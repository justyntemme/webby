package storage

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/justyntemme/webby/internal/models"
)

// Database handles all database operations
type Database struct {
	db *sql.DB
}

// NewDatabase creates and initializes the SQLite database
func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	d := &Database{db: db}
	if err := d.migrate(); err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Database) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS books (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL,
		author TEXT NOT NULL DEFAULT 'Unknown',
		series TEXT DEFAULT '',
		series_index REAL DEFAULT 0,
		file_path TEXT NOT NULL,
		cover_path TEXT DEFAULT '',
		file_size INTEGER DEFAULT 0,
		uploaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS collections (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		is_smart INTEGER DEFAULT 0,
		rule_logic TEXT DEFAULT 'AND',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS collection_rules (
		id TEXT PRIMARY KEY,
		collection_id TEXT NOT NULL,
		field TEXT NOT NULL,
		operator TEXT NOT NULL,
		value TEXT NOT NULL,
		FOREIGN KEY (collection_id) REFERENCES collections(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS book_collections (
		book_id TEXT NOT NULL,
		collection_id TEXT NOT NULL,
		PRIMARY KEY (book_id, collection_id),
		FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
		FOREIGN KEY (collection_id) REFERENCES collections(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS reading_positions (
		book_id TEXT NOT NULL,
		user_id TEXT NOT NULL DEFAULT '',
		chapter TEXT NOT NULL,
		position REAL DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (book_id, user_id),
		FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS book_shares (
		id TEXT PRIMARY KEY,
		book_id TEXT NOT NULL,
		owner_id TEXT NOT NULL,
		shared_with_id TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(book_id, shared_with_id),
		FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
		FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (shared_with_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_books_author ON books(author);
	CREATE INDEX IF NOT EXISTS idx_books_series ON books(series);
	CREATE INDEX IF NOT EXISTS idx_books_user ON books(user_id);
	CREATE INDEX IF NOT EXISTS idx_collections_user ON collections(user_id);
	CREATE INDEX IF NOT EXISTS idx_book_shares_shared_with ON book_shares(shared_with_id);
	`

	_, err := d.db.Exec(schema)
	if err != nil {
		return err
	}

	// Add new metadata columns if they don't exist (migration for existing databases)
	metadataColumns := []string{
		"ALTER TABLE books ADD COLUMN isbn TEXT DEFAULT ''",
		"ALTER TABLE books ADD COLUMN publisher TEXT DEFAULT ''",
		"ALTER TABLE books ADD COLUMN publish_date TEXT DEFAULT ''",
		"ALTER TABLE books ADD COLUMN description TEXT DEFAULT ''",
		"ALTER TABLE books ADD COLUMN language TEXT DEFAULT ''",
		"ALTER TABLE books ADD COLUMN subjects TEXT DEFAULT ''",
		"ALTER TABLE books ADD COLUMN metadata_source TEXT DEFAULT 'epub'",
		"ALTER TABLE books ADD COLUMN metadata_updated DATETIME",
		"ALTER TABLE books ADD COLUMN content_type TEXT DEFAULT 'book'",
		"ALTER TABLE books ADD COLUMN file_format TEXT DEFAULT 'epub'",
	}

	for _, col := range metadataColumns {
		// Ignore errors - column may already exist
		d.db.Exec(col)
	}

	// Add file_hash column for duplicate detection
	d.db.Exec("ALTER TABLE books ADD COLUMN file_hash TEXT DEFAULT ''")

	// Add read status tracking columns
	d.db.Exec("ALTER TABLE books ADD COLUMN read_status TEXT DEFAULT 'unread'")
	d.db.Exec("ALTER TABLE books ADD COLUMN date_completed DATETIME")

	// Add star rating column (0-5, 0 means no rating)
	d.db.Exec("ALTER TABLE books ADD COLUMN rating INTEGER DEFAULT 0")

	// Add smart collections support
	d.db.Exec("ALTER TABLE collections ADD COLUMN is_smart INTEGER DEFAULT 0")
	d.db.Exec("ALTER TABLE collections ADD COLUMN rule_logic TEXT DEFAULT 'AND'")

	// Create collection_rules table if it doesn't exist
	d.db.Exec(`
		CREATE TABLE IF NOT EXISTS collection_rules (
			id TEXT PRIMARY KEY,
			collection_id TEXT NOT NULL,
			field TEXT NOT NULL,
			operator TEXT NOT NULL,
			value TEXT NOT NULL,
			FOREIGN KEY (collection_id) REFERENCES collections(id) ON DELETE CASCADE
		)
	`)
	d.db.Exec("CREATE INDEX IF NOT EXISTS idx_collection_rules_collection ON collection_rules(collection_id)")

	// Add indexes
	d.db.Exec("CREATE INDEX IF NOT EXISTS idx_books_isbn ON books(isbn)")
	d.db.Exec("CREATE INDEX IF NOT EXISTS idx_books_content_type ON books(content_type)")
	d.db.Exec("CREATE INDEX IF NOT EXISTS idx_books_file_format ON books(file_format)")
	d.db.Exec("CREATE INDEX IF NOT EXISTS idx_books_file_hash ON books(file_hash)")
	d.db.Exec("CREATE INDEX IF NOT EXISTS idx_books_read_status ON books(read_status)")

	// Create reading lists tables
	readingListsSchema := `
	CREATE TABLE IF NOT EXISTS reading_lists (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		list_type TEXT NOT NULL DEFAULT 'custom',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS book_reading_list (
		book_id TEXT NOT NULL,
		list_id TEXT NOT NULL,
		added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		position INTEGER DEFAULT 0,
		PRIMARY KEY (book_id, list_id),
		FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
		FOREIGN KEY (list_id) REFERENCES reading_lists(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_reading_lists_user ON reading_lists(user_id);
	CREATE INDEX IF NOT EXISTS idx_reading_lists_type ON reading_lists(list_type);
	CREATE INDEX IF NOT EXISTS idx_book_reading_list_list ON book_reading_list(list_id);
	`
	d.db.Exec(readingListsSchema)

	// Create custom tags tables
	tagsSchema := `
	CREATE TABLE IF NOT EXISTS tags (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		color TEXT DEFAULT '#3b82f6',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, name),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS book_tags (
		book_id TEXT NOT NULL,
		tag_id TEXT NOT NULL,
		added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (book_id, tag_id),
		FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
		FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_tags_user ON tags(user_id);
	CREATE INDEX IF NOT EXISTS idx_tags_name ON tags(name);
	CREATE INDEX IF NOT EXISTS idx_book_tags_tag ON book_tags(tag_id);
	`
	d.db.Exec(tagsSchema)

	// Create annotations table for highlights and notes
	annotationsSchema := `
	CREATE TABLE IF NOT EXISTS annotations (
		id TEXT PRIMARY KEY,
		book_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		chapter TEXT NOT NULL,
		cfi TEXT DEFAULT '',
		start_offset INTEGER DEFAULT 0,
		end_offset INTEGER DEFAULT 0,
		selected_text TEXT NOT NULL,
		note TEXT DEFAULT '',
		color TEXT DEFAULT 'yellow',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_annotations_book ON annotations(book_id);
	CREATE INDEX IF NOT EXISTS idx_annotations_user ON annotations(user_id);
	CREATE INDEX IF NOT EXISTS idx_annotations_book_user ON annotations(book_id, user_id);
	CREATE INDEX IF NOT EXISTS idx_annotations_chapter ON annotations(chapter);
	`
	d.db.Exec(annotationsSchema)

	// Create reading sessions and statistics tables
	readingStatsSchema := `
	CREATE TABLE IF NOT EXISTS reading_sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		book_id TEXT NOT NULL,
		start_time DATETIME NOT NULL,
		end_time DATETIME,
		pages_read INTEGER DEFAULT 0,
		chapters_read INTEGER DEFAULT 0,
		duration_seconds INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS user_statistics (
		user_id TEXT PRIMARY KEY,
		total_books_read INTEGER DEFAULT 0,
		total_pages_read INTEGER DEFAULT 0,
		total_chapters_read INTEGER DEFAULT 0,
		total_time_seconds INTEGER DEFAULT 0,
		current_streak INTEGER DEFAULT 0,
		longest_streak INTEGER DEFAULT 0,
		last_reading_date DATE,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS daily_reading_stats (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		reading_date DATE NOT NULL,
		pages_read INTEGER DEFAULT 0,
		chapters_read INTEGER DEFAULT 0,
		time_seconds INTEGER DEFAULT 0,
		books_touched INTEGER DEFAULT 0,
		UNIQUE(user_id, reading_date),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_reading_sessions_user ON reading_sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_reading_sessions_book ON reading_sessions(book_id);
	CREATE INDEX IF NOT EXISTS idx_reading_sessions_start ON reading_sessions(start_time);
	CREATE INDEX IF NOT EXISTS idx_daily_stats_user ON daily_reading_stats(user_id);
	CREATE INDEX IF NOT EXISTS idx_daily_stats_date ON daily_reading_stats(reading_date);
	`
	d.db.Exec(readingStatsSchema)

	return nil
}

// CreateBook inserts a new book into the database
func (d *Database) CreateBook(book *models.Book) error {
	// Default to "book" if content type not set
	contentType := book.ContentType
	if contentType == "" {
		contentType = models.ContentTypeBook
	}
	// Default to "epub" if file format not set
	fileFormat := book.FileFormat
	if fileFormat == "" {
		fileFormat = models.FileFormatEPUB
	}
	// Default to "unread" if read status not set
	readStatus := book.ReadStatus
	if readStatus == "" {
		readStatus = models.ReadStatusUnread
	}
	_, err := d.db.Exec(`
		INSERT INTO books (id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at,
			isbn, publisher, publish_date, description, language, subjects, metadata_source, metadata_updated, content_type, file_format, file_hash, read_status, date_completed, rating)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		book.ID, book.UserID, book.Title, book.Author, book.Series, book.SeriesIndex,
		book.FilePath, book.CoverPath, book.FileSize, book.UploadedAt,
		book.ISBN, book.Publisher, book.PublishDate, book.Description,
		book.Language, book.Subjects, book.MetadataSource, book.MetadataUpdated, contentType, fileFormat, book.FileHash, readStatus, book.DateCompleted, book.Rating,
	)
	return err
}

// UpdateBookMetadata updates the metadata fields for a book
func (d *Database) UpdateBookMetadata(book *models.Book) error {
	_, err := d.db.Exec(`
		UPDATE books SET
			title = ?, author = ?, series = ?, series_index = ?,
			isbn = ?, publisher = ?, publish_date = ?, description = ?,
			language = ?, subjects = ?, metadata_source = ?, metadata_updated = ?
		WHERE id = ?`,
		book.Title, book.Author, book.Series, book.SeriesIndex,
		book.ISBN, book.Publisher, book.PublishDate, book.Description,
		book.Language, book.Subjects, book.MetadataSource, book.MetadataUpdated,
		book.ID,
	)
	return err
}

// UpdateBookFilePaths updates the file paths for a book after reorganization
func (d *Database) UpdateBookFilePaths(bookID, filePath, coverPath string) error {
	_, err := d.db.Exec(`
		UPDATE books SET file_path = ?, cover_path = ? WHERE id = ?`,
		filePath, coverPath, bookID,
	)
	return err
}

// GetBook retrieves a book by ID
func (d *Database) GetBook(id string) (*models.Book, error) {
	book := &models.Book{}
	err := d.db.QueryRow(`
		SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at,
			COALESCE(isbn, ''), COALESCE(publisher, ''), COALESCE(publish_date, ''), COALESCE(description, ''),
			COALESCE(language, ''), COALESCE(subjects, ''), COALESCE(metadata_source, 'epub'), metadata_updated,
			COALESCE(content_type, 'book'), COALESCE(file_format, 'epub'), COALESCE(file_hash, ''),
			COALESCE(read_status, 'unread'), date_completed, COALESCE(rating, 0)
		FROM books WHERE id = ?`, id,
	).Scan(&book.ID, &book.UserID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
		&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt,
		&book.ISBN, &book.Publisher, &book.PublishDate, &book.Description,
		&book.Language, &book.Subjects, &book.MetadataSource, &book.MetadataUpdated, &book.ContentType, &book.FileFormat, &book.FileHash,
		&book.ReadStatus, &book.DateCompleted, &book.Rating)
	if err != nil {
		return nil, err
	}
	return book, nil
}

// GetBookForUser retrieves a book by ID if user has access (owner or shared)
func (d *Database) GetBookForUser(id, userID string) (*models.Book, error) {
	book := &models.Book{}
	err := d.db.QueryRow(`
		SELECT b.id, b.user_id, b.title, b.author, b.series, b.series_index, b.file_path, b.cover_path, b.file_size, b.uploaded_at,
			COALESCE(b.isbn, ''), COALESCE(b.publisher, ''), COALESCE(b.publish_date, ''), COALESCE(b.description, ''),
			COALESCE(b.language, ''), COALESCE(b.subjects, ''), COALESCE(b.metadata_source, 'epub'), b.metadata_updated,
			COALESCE(b.content_type, 'book'), COALESCE(b.file_format, 'epub'), COALESCE(b.file_hash, ''),
			COALESCE(b.read_status, 'unread'), b.date_completed, COALESCE(b.rating, 0)
		FROM books b
		LEFT JOIN book_shares bs ON b.id = bs.book_id AND bs.shared_with_id = ?
		WHERE b.id = ? AND (b.user_id = ? OR b.user_id = '' OR bs.id IS NOT NULL)`, userID, id, userID,
	).Scan(&book.ID, &book.UserID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
		&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt,
		&book.ISBN, &book.Publisher, &book.PublishDate, &book.Description,
		&book.Language, &book.Subjects, &book.MetadataSource, &book.MetadataUpdated, &book.ContentType, &book.FileFormat, &book.FileHash,
		&book.ReadStatus, &book.DateCompleted, &book.Rating)
	if err != nil {
		return nil, err
	}
	return book, nil
}

// ListBooks returns all books with optional sorting (legacy - no user filter)
func (d *Database) ListBooks(sortBy, order string) ([]models.Book, error) {
	return d.ListBooksForUserWithFilter("", sortBy, order, "")
}

// ListBooksForUser returns books for a specific user with optional sorting
func (d *Database) ListBooksForUser(userID, sortBy, order string) ([]models.Book, error) {
	return d.ListBooksForUserWithFilter(userID, sortBy, order, "")
}

// ListBooksForUserWithFilter returns books for a specific user with optional sorting and content type filter
func (d *Database) ListBooksForUserWithFilter(userID, sortBy, order, contentType string) ([]models.Book, error) {
	return d.ListBooksForUserWithFilters(userID, sortBy, order, contentType, "")
}

// ListBooksForUserWithFilters returns books for a specific user with optional sorting, content type, and read status filters
func (d *Database) ListBooksForUserWithFilters(userID, sortBy, order, contentType, readStatus string) ([]models.Book, error) {
	// Define sort columns - each column needs order applied
	// Using COALESCE to handle NULL/empty authors - sort them at the end
	validSort := map[string][]string{
		"title":  {"title"},
		"author": {"CASE WHEN author = '' OR author IS NULL THEN 1 ELSE 0 END", "author", "series", "series_index", "title"},
		"series": {"series", "series_index", "title"},
		"date":   {"uploaded_at"},
	}

	sortColumns, ok := validSort[sortBy]
	if !ok {
		// Default: sort by author, then series, then title
		sortColumns = validSort["author"]
	}

	if order != "desc" {
		order = "asc"
	}

	// Build ORDER BY clause with order applied to each column
	var orderParts []string
	for _, col := range sortColumns {
		orderParts = append(orderParts, col+" "+order)
	}
	orderBy := " ORDER BY " + strings.Join(orderParts, ", ")

	var query string
	var args []interface{}

	baseSelect := "SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at, COALESCE(content_type, 'book'), COALESCE(file_format, 'epub'), COALESCE(read_status, 'unread') FROM books WHERE "

	if userID != "" {
		query = baseSelect + "user_id = ?"
		args = append(args, userID)
	} else {
		query = baseSelect + "user_id = ''"
	}

	// Add content type filter if specified
	if contentType != "" && (contentType == models.ContentTypeBook || contentType == models.ContentTypeComic) {
		query += " AND COALESCE(content_type, 'book') = ?"
		args = append(args, contentType)
	}

	// Add read status filter if specified
	if readStatus != "" && (readStatus == models.ReadStatusUnread || readStatus == models.ReadStatusReading || readStatus == models.ReadStatusCompleted) {
		query += " AND COALESCE(read_status, 'unread') = ?"
		args = append(args, readStatus)
	}

	query += orderBy

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var book models.Book
		err := rows.Scan(&book.ID, &book.UserID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
			&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt, &book.ContentType, &book.FileFormat, &book.ReadStatus)
		if err != nil {
			return nil, err
		}
		books = append(books, book)
	}

	return books, nil
}

// SearchBooks searches books by title, author, or series
func (d *Database) SearchBooks(query string) ([]models.Book, error) {
	return d.SearchBooksForUser(query, "")
}

// SearchBooksForUser searches books for a specific user
func (d *Database) SearchBooksForUser(query, userID string) ([]models.Book, error) {
	searchTerm := "%" + query + "%"
	var rows *sql.Rows
	var err error

	if userID != "" {
		rows, err = d.db.Query(`
			SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at, COALESCE(content_type, 'book'), COALESCE(file_format, 'epub')
			FROM books
			WHERE user_id = ? AND (title LIKE ? OR author LIKE ? OR series LIKE ?)
			ORDER BY title`,
			userID, searchTerm, searchTerm, searchTerm,
		)
	} else {
		rows, err = d.db.Query(`
			SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at, COALESCE(content_type, 'book'), COALESCE(file_format, 'epub')
			FROM books
			WHERE user_id = '' AND (title LIKE ? OR author LIKE ? OR series LIKE ?)
			ORDER BY title`,
			searchTerm, searchTerm, searchTerm,
		)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var book models.Book
		err := rows.Scan(&book.ID, &book.UserID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
			&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt, &book.ContentType, &book.FileFormat)
		if err != nil {
			return nil, err
		}
		books = append(books, book)
	}

	return books, nil
}

// GetBooksByAuthor returns books grouped by author (legacy - no user filter)
func (d *Database) GetBooksByAuthor() (map[string][]models.Book, error) {
	return d.GetBooksByAuthorForUser("")
}

// GetBooksByAuthorForUser returns books grouped by author for a specific user
func (d *Database) GetBooksByAuthorForUser(userID string) (map[string][]models.Book, error) {
	books, err := d.ListBooksForUser(userID, "author", "asc")
	if err != nil {
		return nil, err
	}

	grouped := make(map[string][]models.Book)
	for _, book := range books {
		grouped[book.Author] = append(grouped[book.Author], book)
	}

	return grouped, nil
}

// GetBooksBySeries returns books grouped by series (legacy - no user filter)
func (d *Database) GetBooksBySeries() (map[string][]models.Book, error) {
	return d.GetBooksBySeriesForUser("")
}

// GetBooksBySeriesForUser returns books grouped by series for a specific user
func (d *Database) GetBooksBySeriesForUser(userID string) (map[string][]models.Book, error) {
	var rows *sql.Rows
	var err error

	if userID != "" {
		rows, err = d.db.Query(`
			SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at
			FROM books
			WHERE user_id = ? AND series != ''
			ORDER BY series, series_index`, userID)
	} else {
		rows, err = d.db.Query(`
			SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at
			FROM books
			WHERE user_id = '' AND series != ''
			ORDER BY series, series_index`)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grouped := make(map[string][]models.Book)
	for rows.Next() {
		var book models.Book
		err := rows.Scan(&book.ID, &book.UserID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
			&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt)
		if err != nil {
			return nil, err
		}
		grouped[book.Series] = append(grouped[book.Series], book)
	}

	return grouped, nil
}

// DeleteBook removes a book from the database
func (d *Database) DeleteBook(id string) error {
	_, err := d.db.Exec("DELETE FROM books WHERE id = ?", id)
	return err
}

// SaveReadingPosition saves or updates reading position for a user
func (d *Database) SaveReadingPosition(pos *models.ReadingPosition) error {
	_, err := d.db.Exec(`
		INSERT INTO reading_positions (book_id, user_id, chapter, position, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(book_id, user_id) DO UPDATE SET
			chapter = excluded.chapter,
			position = excluded.position,
			updated_at = excluded.updated_at`,
		pos.BookID, pos.UserID, pos.Chapter, pos.Position, time.Now(),
	)
	return err
}

// GetReadingPosition retrieves reading position for a book and user
func (d *Database) GetReadingPosition(bookID, userID string) (*models.ReadingPosition, error) {
	pos := &models.ReadingPosition{}
	err := d.db.QueryRow(`
		SELECT book_id, user_id, chapter, position, updated_at
		FROM reading_positions WHERE book_id = ? AND user_id = ?`, bookID, userID,
	).Scan(&pos.BookID, &pos.UserID, &pos.Chapter, &pos.Position, &pos.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return pos, nil
}

// CreateCollection creates a new collection
func (d *Database) CreateCollection(collection *models.Collection) error {
	isSmart := 0
	if collection.IsSmart {
		isSmart = 1
	}
	ruleLogic := collection.RuleLogic
	if ruleLogic == "" {
		ruleLogic = "AND"
	}
	_, err := d.db.Exec(`
		INSERT INTO collections (id, user_id, name, is_smart, rule_logic, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		collection.ID, collection.UserID, collection.Name, isSmart, ruleLogic, collection.CreatedAt,
	)
	return err
}

// GetCollection retrieves a collection by ID
func (d *Database) GetCollection(id string) (*models.Collection, error) {
	collection := &models.Collection{}
	var isSmart int
	var userID, ruleLogic sql.NullString
	err := d.db.QueryRow(`
		SELECT id, user_id, name, COALESCE(is_smart, 0), COALESCE(rule_logic, 'AND'), created_at
		FROM collections WHERE id = ?`, id,
	).Scan(&collection.ID, &userID, &collection.Name, &isSmart, &ruleLogic, &collection.CreatedAt)
	if err != nil {
		return nil, err
	}
	collection.IsSmart = isSmart == 1
	if userID.Valid {
		collection.UserID = userID.String
	}
	if ruleLogic.Valid {
		collection.RuleLogic = ruleLogic.String
	} else {
		collection.RuleLogic = "AND"
	}
	return collection, nil
}

// ListCollections returns all collections
func (d *Database) ListCollections() ([]models.Collection, error) {
	rows, err := d.db.Query(`
		SELECT id, user_id, name, COALESCE(is_smart, 0), COALESCE(rule_logic, 'AND'), created_at
		FROM collections ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collections []models.Collection
	for rows.Next() {
		var c models.Collection
		var isSmart int
		var userID, ruleLogic sql.NullString
		if err := rows.Scan(&c.ID, &userID, &c.Name, &isSmart, &ruleLogic, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.IsSmart = isSmart == 1
		if userID.Valid {
			c.UserID = userID.String
		}
		if ruleLogic.Valid {
			c.RuleLogic = ruleLogic.String
		} else {
			c.RuleLogic = "AND"
		}
		collections = append(collections, c)
	}
	return collections, nil
}

// UpdateCollection updates a collection's name
func (d *Database) UpdateCollection(id, name string) error {
	_, err := d.db.Exec(`UPDATE collections SET name = ? WHERE id = ?`, name, id)
	return err
}

// DeleteCollection removes a collection
func (d *Database) DeleteCollection(id string) error {
	_, err := d.db.Exec(`DELETE FROM collections WHERE id = ?`, id)
	return err
}

// AddBookToCollection adds a book to a collection
func (d *Database) AddBookToCollection(bookID, collectionID string) error {
	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO book_collections (book_id, collection_id)
		VALUES (?, ?)`, bookID, collectionID,
	)
	return err
}

// RemoveBookFromCollection removes a book from a collection
func (d *Database) RemoveBookFromCollection(bookID, collectionID string) error {
	_, err := d.db.Exec(`
		DELETE FROM book_collections WHERE book_id = ? AND collection_id = ?`,
		bookID, collectionID,
	)
	return err
}

// GetBooksInCollection returns all books in a collection
func (d *Database) GetBooksInCollection(collectionID string) ([]models.Book, error) {
	rows, err := d.db.Query(`
		SELECT b.id, b.title, b.author, b.series, b.series_index, b.file_path, b.cover_path, b.file_size, b.uploaded_at
		FROM books b
		JOIN book_collections bc ON b.id = bc.book_id
		WHERE bc.collection_id = ?
		ORDER BY b.title`, collectionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var book models.Book
		err := rows.Scan(&book.ID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
			&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt)
		if err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, nil
}

// GetCollectionsForBook returns all collections a book belongs to
func (d *Database) GetCollectionsForBook(bookID string) ([]models.Collection, error) {
	rows, err := d.db.Query(`
		SELECT c.id, c.name, c.created_at
		FROM collections c
		JOIN book_collections bc ON c.id = bc.collection_id
		WHERE bc.book_id = ?
		ORDER BY c.name`, bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collections []models.Collection
	for rows.Next() {
		var c models.Collection
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		collections = append(collections, c)
	}
	return collections, nil
}

// BulkAddBooksToCollection adds multiple books to a collection
func (d *Database) BulkAddBooksToCollection(bookIDs []string, collectionID string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO book_collections (book_id, collection_id) VALUES (?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, bookID := range bookIDs {
		if _, err := stmt.Exec(bookID, collectionID); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// UpdateSmartCollection updates a smart collection's settings
func (d *Database) UpdateSmartCollection(id, name, ruleLogic string) error {
	_, err := d.db.Exec(`UPDATE collections SET name = ?, rule_logic = ? WHERE id = ?`, name, ruleLogic, id)
	return err
}

// CreateCollectionRule adds a rule to a collection
func (d *Database) CreateCollectionRule(rule *models.CollectionRule) error {
	_, err := d.db.Exec(`
		INSERT INTO collection_rules (id, collection_id, field, operator, value)
		VALUES (?, ?, ?, ?, ?)`,
		rule.ID, rule.CollectionID, rule.Field, rule.Operator, rule.Value,
	)
	return err
}

// GetCollectionRules returns all rules for a collection
func (d *Database) GetCollectionRules(collectionID string) ([]models.CollectionRule, error) {
	rows, err := d.db.Query(`
		SELECT id, collection_id, field, operator, value
		FROM collection_rules WHERE collection_id = ?`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.CollectionRule
	for rows.Next() {
		var r models.CollectionRule
		if err := rows.Scan(&r.ID, &r.CollectionID, &r.Field, &r.Operator, &r.Value); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// DeleteCollectionRules removes all rules for a collection
func (d *Database) DeleteCollectionRules(collectionID string) error {
	_, err := d.db.Exec(`DELETE FROM collection_rules WHERE collection_id = ?`, collectionID)
	return err
}

// GetSmartCollectionBooks returns books matching a smart collection's rules
func (d *Database) GetSmartCollectionBooks(collectionID, userID string) ([]models.Book, error) {
	// Get collection and its rules
	collection, err := d.GetCollection(collectionID)
	if err != nil {
		return nil, err
	}
	if !collection.IsSmart {
		// For non-smart collections, return normal book list
		return d.GetBooksInCollection(collectionID)
	}

	rules, err := d.GetCollectionRules(collectionID)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return []models.Book{}, nil
	}

	// Build dynamic query based on rules
	query := `SELECT DISTINCT b.id, b.title, b.author, b.series, b.series_index,
		b.file_path, b.cover_path, b.file_size, b.uploaded_at,
		COALESCE(b.isbn, ''), COALESCE(b.publisher, ''), COALESCE(b.publish_date, ''),
		COALESCE(b.description, ''), COALESCE(b.language, ''), COALESCE(b.subjects, ''),
		COALESCE(b.content_type, 'book'), COALESCE(b.file_format, 'epub'),
		COALESCE(b.read_status, 'unread'), COALESCE(b.rating, 0)
		FROM books b
		LEFT JOIN book_tags bt ON b.id = bt.book_id
		LEFT JOIN tags t ON bt.tag_id = t.id
		WHERE b.user_id = ?`

	args := []interface{}{userID}
	conditions := []string{}

	for _, rule := range rules {
		cond, ruleArgs := d.buildRuleCondition(rule)
		if cond != "" {
			conditions = append(conditions, cond)
			args = append(args, ruleArgs...)
		}
	}

	if len(conditions) > 0 {
		joiner := " AND "
		if collection.RuleLogic == "OR" {
			joiner = " OR "
		}
		query += " AND (" + strings.Join(conditions, joiner) + ")"
	}

	query += " ORDER BY b.title"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var book models.Book
		err := rows.Scan(&book.ID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
			&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt,
			&book.ISBN, &book.Publisher, &book.PublishDate, &book.Description,
			&book.Language, &book.Subjects, &book.ContentType, &book.FileFormat,
			&book.ReadStatus, &book.Rating)
		if err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, nil
}

// buildRuleCondition builds a SQL condition for a single rule
func (d *Database) buildRuleCondition(rule models.CollectionRule) (string, []interface{}) {
	var args []interface{}

	switch rule.Field {
	case models.RuleFieldAuthor:
		switch rule.Operator {
		case models.RuleOpEquals:
			return "LOWER(b.author) = LOWER(?)", []interface{}{rule.Value}
		case models.RuleOpContains:
			return "LOWER(b.author) LIKE LOWER(?)", []interface{}{"%" + rule.Value + "%"}
		case models.RuleOpStartsWith:
			return "LOWER(b.author) LIKE LOWER(?)", []interface{}{rule.Value + "%"}
		}

	case models.RuleFieldTitle:
		switch rule.Operator {
		case models.RuleOpEquals:
			return "LOWER(b.title) = LOWER(?)", []interface{}{rule.Value}
		case models.RuleOpContains:
			return "LOWER(b.title) LIKE LOWER(?)", []interface{}{"%" + rule.Value + "%"}
		case models.RuleOpStartsWith:
			return "LOWER(b.title) LIKE LOWER(?)", []interface{}{rule.Value + "%"}
		}

	case models.RuleFieldFormat:
		return "b.file_format = ?", []interface{}{rule.Value}

	case models.RuleFieldContentType:
		return "b.content_type = ?", []interface{}{rule.Value}

	case models.RuleFieldYear:
		switch rule.Operator {
		case models.RuleOpEquals:
			return "b.publish_date LIKE ?", []interface{}{rule.Value + "%"}
		case models.RuleOpGreaterThan:
			return "CAST(SUBSTR(b.publish_date, 1, 4) AS INTEGER) > ?", []interface{}{rule.Value}
		case models.RuleOpLessThan:
			return "CAST(SUBSTR(b.publish_date, 1, 4) AS INTEGER) < ?", []interface{}{rule.Value}
		}

	case models.RuleFieldSeries:
		switch rule.Operator {
		case models.RuleOpEquals:
			return "LOWER(b.series) = LOWER(?)", []interface{}{rule.Value}
		case models.RuleOpContains:
			return "LOWER(b.series) LIKE LOWER(?)", []interface{}{"%" + rule.Value + "%"}
		}

	case models.RuleFieldTags:
		return "LOWER(t.name) = LOWER(?)", []interface{}{rule.Value}

	case models.RuleFieldRating:
		switch rule.Operator {
		case models.RuleOpEquals:
			return "b.rating = ?", []interface{}{rule.Value}
		case models.RuleOpGreaterThan:
			return "b.rating > ?", []interface{}{rule.Value}
		case models.RuleOpLessThan:
			return "b.rating < ?", []interface{}{rule.Value}
		}

	case models.RuleFieldReadStatus:
		return "b.read_status = ?", []interface{}{rule.Value}

	case models.RuleFieldFileSize:
		switch rule.Operator {
		case models.RuleOpGreaterThan:
			return "b.file_size > ?", []interface{}{rule.Value}
		case models.RuleOpLessThan:
			return "b.file_size < ?", []interface{}{rule.Value}
		}
	}

	return "", args
}

// ListSmartCollections returns all smart collections for a user
func (d *Database) ListSmartCollections(userID string) ([]models.Collection, error) {
	rows, err := d.db.Query(`
		SELECT id, user_id, name, COALESCE(is_smart, 0), COALESCE(rule_logic, 'AND'), created_at
		FROM collections WHERE is_smart = 1 AND user_id = ? ORDER BY name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collections []models.Collection
	for rows.Next() {
		var c models.Collection
		var isSmart int
		var uID, ruleLogic sql.NullString
		if err := rows.Scan(&c.ID, &uID, &c.Name, &isSmart, &ruleLogic, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.IsSmart = isSmart == 1
		if uID.Valid {
			c.UserID = uID.String
		}
		if ruleLogic.Valid {
			c.RuleLogic = ruleLogic.String
		} else {
			c.RuleLogic = "AND"
		}
		collections = append(collections, c)
	}
	return collections, nil
}

// CreateUser creates a new user
func (d *Database) CreateUser(user *models.User) error {
	_, err := d.db.Exec(`
		INSERT INTO users (id, username, email, password_hash, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		user.ID, user.Username, user.Email, user.PasswordHash, user.CreatedAt,
	)
	return err
}

// GetUserByID retrieves a user by ID
func (d *Database) GetUserByID(id string) (*models.User, error) {
	user := &models.User{}
	err := d.db.QueryRow(`
		SELECT id, username, email, password_hash, created_at
		FROM users WHERE id = ?`, id,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByUsername retrieves a user by username
func (d *Database) GetUserByUsername(username string) (*models.User, error) {
	user := &models.User{}
	err := d.db.QueryRow(`
		SELECT id, username, email, password_hash, created_at
		FROM users WHERE username = ?`, username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByEmail retrieves a user by email
func (d *Database) GetUserByEmail(email string) (*models.User, error) {
	user := &models.User{}
	err := d.db.QueryRow(`
		SELECT id, username, email, password_hash, created_at
		FROM users WHERE email = ?`, email,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UserExists checks if a username or email is already taken
func (d *Database) UserExists(username, email string) (bool, error) {
	var count int
	err := d.db.QueryRow(`
		SELECT COUNT(*) FROM users WHERE username = ? OR email = ?`,
		username, email,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// SearchUsers searches for users by username (for sharing)
func (d *Database) SearchUsers(query string, excludeUserID string) ([]models.User, error) {
	searchTerm := "%" + query + "%"
	rows, err := d.db.Query(`
		SELECT id, username, email, created_at
		FROM users
		WHERE username LIKE ? AND id != ?
		LIMIT 10`,
		searchTerm, excludeUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

// ShareBook shares a book with another user
func (d *Database) ShareBook(bookID, ownerID, sharedWithID string) error {
	id := sharedWithID + "-" + bookID // Simple composite ID
	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO book_shares (id, book_id, owner_id, shared_with_id, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		id, bookID, ownerID, sharedWithID, time.Now(),
	)
	return err
}

// UnshareBook removes a book share
func (d *Database) UnshareBook(bookID, sharedWithID string) error {
	_, err := d.db.Exec(`
		DELETE FROM book_shares WHERE book_id = ? AND shared_with_id = ?`,
		bookID, sharedWithID,
	)
	return err
}

// GetSharedBooks returns books shared with a user
func (d *Database) GetSharedBooks(userID string) ([]models.Book, error) {
	rows, err := d.db.Query(`
		SELECT b.id, b.user_id, b.title, b.author, b.series, b.series_index, b.file_path, b.cover_path, b.file_size, b.uploaded_at
		FROM books b
		JOIN book_shares bs ON b.id = bs.book_id
		WHERE bs.shared_with_id = ?
		ORDER BY b.title`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var book models.Book
		err := rows.Scan(&book.ID, &book.UserID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
			&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt)
		if err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, nil
}

// GetBookShares returns users a book is shared with
func (d *Database) GetBookShares(bookID string) ([]models.User, error) {
	rows, err := d.db.Query(`
		SELECT u.id, u.username, u.email, u.created_at
		FROM users u
		JOIN book_shares bs ON u.id = bs.shared_with_id
		WHERE bs.book_id = ?`, bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

// IsBookSharedWith checks if a book is shared with a user
func (d *Database) IsBookSharedWith(bookID, userID string) (bool, error) {
	var count int
	err := d.db.QueryRow(`
		SELECT COUNT(*) FROM book_shares WHERE book_id = ? AND shared_with_id = ?`,
		bookID, userID,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// UpdateBookFileHash updates the file hash for a book
func (d *Database) UpdateBookFileHash(bookID, fileHash string) error {
	_, err := d.db.Exec(`UPDATE books SET file_hash = ? WHERE id = ?`, fileHash, bookID)
	return err
}

// GetBooksByHash returns all books with a given file hash
func (d *Database) GetBooksByHash(fileHash string) ([]models.Book, error) {
	if fileHash == "" {
		return nil, nil
	}
	rows, err := d.db.Query(`
		SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at,
			COALESCE(content_type, 'book'), COALESCE(file_format, 'epub'), COALESCE(file_hash, '')
		FROM books WHERE file_hash = ?
		ORDER BY uploaded_at`, fileHash,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var book models.Book
		err := rows.Scan(&book.ID, &book.UserID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
			&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt, &book.ContentType, &book.FileFormat, &book.FileHash)
		if err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, nil
}

// DuplicateGroup represents a group of books with the same file hash
type DuplicateGroup struct {
	FileHash string
	Books    []models.Book
}

// FindDuplicateBooks returns groups of books that have the same file hash
func (d *Database) FindDuplicateBooks(userID string) ([]DuplicateGroup, error) {
	// First find all hashes that appear more than once
	var hashQuery string
	var args []interface{}

	if userID != "" {
		hashQuery = `
			SELECT file_hash, COUNT(*) as cnt
			FROM books
			WHERE user_id = ? AND file_hash != ''
			GROUP BY file_hash
			HAVING cnt > 1
			ORDER BY cnt DESC`
		args = append(args, userID)
	} else {
		hashQuery = `
			SELECT file_hash, COUNT(*) as cnt
			FROM books
			WHERE file_hash != ''
			GROUP BY file_hash
			HAVING cnt > 1
			ORDER BY cnt DESC`
	}

	rows, err := d.db.Query(hashQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var duplicateHashes []string
	for rows.Next() {
		var hash string
		var count int
		if err := rows.Scan(&hash, &count); err != nil {
			return nil, err
		}
		duplicateHashes = append(duplicateHashes, hash)
	}

	// Now get the books for each duplicate hash
	var groups []DuplicateGroup
	for _, hash := range duplicateHashes {
		books, err := d.GetBooksByHash(hash)
		if err != nil {
			return nil, err
		}
		// Filter by user if needed
		if userID != "" {
			var filtered []models.Book
			for _, b := range books {
				if b.UserID == userID {
					filtered = append(filtered, b)
				}
			}
			books = filtered
		}
		if len(books) > 1 {
			groups = append(groups, DuplicateGroup{
				FileHash: hash,
				Books:    books,
			})
		}
	}

	return groups, nil
}

// GetBooksWithoutHash returns books that don't have a file hash computed yet
func (d *Database) GetBooksWithoutHash(userID string, limit int) ([]models.Book, error) {
	var query string
	var args []interface{}

	if userID != "" {
		query = `
			SELECT id, user_id, title, author, file_path, file_size, uploaded_at,
				COALESCE(content_type, 'book'), COALESCE(file_format, 'epub')
			FROM books
			WHERE user_id = ? AND (file_hash IS NULL OR file_hash = '')
			ORDER BY uploaded_at DESC
			LIMIT ?`
		args = append(args, userID, limit)
	} else {
		query = `
			SELECT id, user_id, title, author, file_path, file_size, uploaded_at,
				COALESCE(content_type, 'book'), COALESCE(file_format, 'epub')
			FROM books
			WHERE file_hash IS NULL OR file_hash = ''
			ORDER BY uploaded_at DESC
			LIMIT ?`
		args = append(args, limit)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var book models.Book
		err := rows.Scan(&book.ID, &book.UserID, &book.Title, &book.Author,
			&book.FilePath, &book.FileSize, &book.UploadedAt, &book.ContentType, &book.FileFormat)
		if err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, nil
}

// CountBooksWithoutHash returns the count of books without file hashes
func (d *Database) CountBooksWithoutHash(userID string) (int, error) {
	var count int
	var err error

	if userID != "" {
		err = d.db.QueryRow(`
			SELECT COUNT(*) FROM books
			WHERE user_id = ? AND (file_hash IS NULL OR file_hash = '')`, userID,
		).Scan(&count)
	} else {
		err = d.db.QueryRow(`
			SELECT COUNT(*) FROM books
			WHERE file_hash IS NULL OR file_hash = ''`,
		).Scan(&count)
	}
	return count, err
}

// UpdateBookReadStatus updates the read status for a book
func (d *Database) UpdateBookReadStatus(bookID, status string, dateCompleted *time.Time) error {
	_, err := d.db.Exec(`
		UPDATE books SET read_status = ?, date_completed = ? WHERE id = ?`,
		status, dateCompleted, bookID,
	)
	return err
}

// GetBookReadStatus returns the read status for a book
func (d *Database) GetBookReadStatus(bookID string) (string, *time.Time, error) {
	var status string
	var dateCompleted *time.Time
	err := d.db.QueryRow(`
		SELECT COALESCE(read_status, 'unread'), date_completed FROM books WHERE id = ?`, bookID,
	).Scan(&status, &dateCompleted)
	if err != nil {
		return "", nil, err
	}
	return status, dateCompleted, nil
}

// BulkUpdateBookReadStatus updates read status for multiple books
func (d *Database) BulkUpdateBookReadStatus(bookIDs []string, status string, dateCompleted *time.Time) error {
	if len(bookIDs) == 0 {
		return nil
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`UPDATE books SET read_status = ?, date_completed = ? WHERE id = ?`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, bookID := range bookIDs {
		if _, err := stmt.Exec(status, dateCompleted, bookID); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// ReadStatusCounts holds counts of books by read status
type ReadStatusCounts struct {
	Unread    int `json:"unread"`
	Reading   int `json:"reading"`
	Completed int `json:"completed"`
	Total     int `json:"total"`
}

// UpdateBookRating updates the star rating for a book (0-5)
func (d *Database) UpdateBookRating(bookID string, rating int) error {
	if rating < 0 || rating > 5 {
		rating = 0
	}
	_, err := d.db.Exec(`UPDATE books SET rating = ? WHERE id = ?`, rating, bookID)
	return err
}

// GetReadStatusCounts returns counts of books by read status
func (d *Database) GetReadStatusCounts(userID string) (*ReadStatusCounts, error) {
	counts := &ReadStatusCounts{}

	var query string
	var args []interface{}

	if userID != "" {
		query = `
			SELECT
				COUNT(*) FILTER (WHERE COALESCE(read_status, 'unread') = 'unread') as unread,
				COUNT(*) FILTER (WHERE read_status = 'reading') as reading,
				COUNT(*) FILTER (WHERE read_status = 'completed') as completed,
				COUNT(*) as total
			FROM books WHERE user_id = ?`
		args = append(args, userID)
	} else {
		query = `
			SELECT
				COUNT(*) FILTER (WHERE COALESCE(read_status, 'unread') = 'unread') as unread,
				COUNT(*) FILTER (WHERE read_status = 'reading') as reading,
				COUNT(*) FILTER (WHERE read_status = 'completed') as completed,
				COUNT(*) as total
			FROM books WHERE user_id = ''`
	}

	// SQLite doesn't support FILTER, use CASE instead
	if userID != "" {
		query = `
			SELECT
				SUM(CASE WHEN COALESCE(read_status, 'unread') = 'unread' THEN 1 ELSE 0 END) as unread,
				SUM(CASE WHEN read_status = 'reading' THEN 1 ELSE 0 END) as reading,
				SUM(CASE WHEN read_status = 'completed' THEN 1 ELSE 0 END) as completed,
				COUNT(*) as total
			FROM books WHERE user_id = ?`
	} else {
		query = `
			SELECT
				SUM(CASE WHEN COALESCE(read_status, 'unread') = 'unread' THEN 1 ELSE 0 END) as unread,
				SUM(CASE WHEN read_status = 'reading' THEN 1 ELSE 0 END) as reading,
				SUM(CASE WHEN read_status = 'completed' THEN 1 ELSE 0 END) as completed,
				COUNT(*) as total
			FROM books WHERE user_id = ''`
	}

	err := d.db.QueryRow(query, args...).Scan(&counts.Unread, &counts.Reading, &counts.Completed, &counts.Total)
	if err != nil {
		return nil, err
	}

	return counts, nil
}

// CreateReadingList creates a new reading list for a user
func (d *Database) CreateReadingList(list *models.ReadingList) error {
	_, err := d.db.Exec(`
		INSERT INTO reading_lists (id, user_id, name, list_type, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		list.ID, list.UserID, list.Name, list.ListType, list.CreatedAt,
	)
	return err
}

// GetReadingList retrieves a reading list by ID
func (d *Database) GetReadingList(id string) (*models.ReadingList, error) {
	list := &models.ReadingList{}
	err := d.db.QueryRow(`
		SELECT rl.id, rl.user_id, rl.name, rl.list_type, rl.created_at,
			(SELECT COUNT(*) FROM book_reading_list brl WHERE brl.list_id = rl.id) as book_count
		FROM reading_lists rl WHERE rl.id = ?`, id,
	).Scan(&list.ID, &list.UserID, &list.Name, &list.ListType, &list.CreatedAt, &list.BookCount)
	if err != nil {
		return nil, err
	}
	return list, nil
}

// GetReadingListByType retrieves a user's reading list by type (for system lists)
func (d *Database) GetReadingListByType(userID, listType string) (*models.ReadingList, error) {
	list := &models.ReadingList{}
	err := d.db.QueryRow(`
		SELECT rl.id, rl.user_id, rl.name, rl.list_type, rl.created_at,
			(SELECT COUNT(*) FROM book_reading_list brl WHERE brl.list_id = rl.id) as book_count
		FROM reading_lists rl WHERE rl.user_id = ? AND rl.list_type = ?`, userID, listType,
	).Scan(&list.ID, &list.UserID, &list.Name, &list.ListType, &list.CreatedAt, &list.BookCount)
	if err != nil {
		return nil, err
	}
	return list, nil
}

// ListReadingLists returns all reading lists for a user
func (d *Database) ListReadingLists(userID string) ([]models.ReadingList, error) {
	rows, err := d.db.Query(`
		SELECT rl.id, rl.user_id, rl.name, rl.list_type, rl.created_at,
			(SELECT COUNT(*) FROM book_reading_list brl WHERE brl.list_id = rl.id) as book_count
		FROM reading_lists rl
		WHERE rl.user_id = ?
		ORDER BY CASE rl.list_type
			WHEN 'want_to_read' THEN 1
			WHEN 'favorites' THEN 2
			ELSE 3
		END, rl.name`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lists []models.ReadingList
	for rows.Next() {
		var list models.ReadingList
		if err := rows.Scan(&list.ID, &list.UserID, &list.Name, &list.ListType, &list.CreatedAt, &list.BookCount); err != nil {
			return nil, err
		}
		lists = append(lists, list)
	}
	return lists, nil
}

// UpdateReadingList updates a reading list's name
func (d *Database) UpdateReadingList(id, name string) error {
	_, err := d.db.Exec(`UPDATE reading_lists SET name = ? WHERE id = ?`, name, id)
	return err
}

// DeleteReadingList removes a reading list
func (d *Database) DeleteReadingList(id string) error {
	_, err := d.db.Exec(`DELETE FROM reading_lists WHERE id = ?`, id)
	return err
}

// AddBookToReadingList adds a book to a reading list
func (d *Database) AddBookToReadingList(bookID, listID string) error {
	// Get the max position in this list
	var maxPos sql.NullInt64
	d.db.QueryRow(`SELECT MAX(position) FROM book_reading_list WHERE list_id = ?`, listID).Scan(&maxPos)
	nextPos := 0
	if maxPos.Valid {
		nextPos = int(maxPos.Int64) + 1
	}

	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO book_reading_list (book_id, list_id, added_at, position)
		VALUES (?, ?, ?, ?)`, bookID, listID, time.Now(), nextPos,
	)
	return err
}

// RemoveBookFromReadingList removes a book from a reading list
func (d *Database) RemoveBookFromReadingList(bookID, listID string) error {
	_, err := d.db.Exec(`
		DELETE FROM book_reading_list WHERE book_id = ? AND list_id = ?`,
		bookID, listID,
	)
	return err
}

// GetBooksInReadingList returns all books in a reading list
func (d *Database) GetBooksInReadingList(listID string) ([]models.Book, error) {
	rows, err := d.db.Query(`
		SELECT b.id, b.user_id, b.title, b.author, b.series, b.series_index,
			b.file_path, b.cover_path, b.file_size, b.uploaded_at,
			COALESCE(b.content_type, 'book'), COALESCE(b.file_format, 'epub'),
			COALESCE(b.read_status, 'unread')
		FROM books b
		JOIN book_reading_list brl ON b.id = brl.book_id
		WHERE brl.list_id = ?
		ORDER BY brl.position, brl.added_at`, listID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var book models.Book
		err := rows.Scan(&book.ID, &book.UserID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
			&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt,
			&book.ContentType, &book.FileFormat, &book.ReadStatus)
		if err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, nil
}

// GetReadingListsForBook returns all reading lists a book belongs to
func (d *Database) GetReadingListsForBook(bookID, userID string) ([]models.ReadingList, error) {
	rows, err := d.db.Query(`
		SELECT rl.id, rl.user_id, rl.name, rl.list_type, rl.created_at
		FROM reading_lists rl
		JOIN book_reading_list brl ON rl.id = brl.list_id
		WHERE brl.book_id = ? AND rl.user_id = ?
		ORDER BY rl.list_type, rl.name`, bookID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lists []models.ReadingList
	for rows.Next() {
		var list models.ReadingList
		if err := rows.Scan(&list.ID, &list.UserID, &list.Name, &list.ListType, &list.CreatedAt); err != nil {
			return nil, err
		}
		lists = append(lists, list)
	}
	return lists, nil
}

// IsBookInReadingList checks if a book is in a reading list
func (d *Database) IsBookInReadingList(bookID, listID string) (bool, error) {
	var count int
	err := d.db.QueryRow(`
		SELECT COUNT(*) FROM book_reading_list WHERE book_id = ? AND list_id = ?`,
		bookID, listID,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// EnsureSystemReadingLists creates the system reading lists for a user if they don't exist
func (d *Database) EnsureSystemReadingLists(userID string) error {
	systemLists := []struct {
		listType string
		name     string
	}{
		{models.ReadingListWantToRead, "Want to Read"},
		{models.ReadingListFavorites, "Favorites"},
	}

	for _, sl := range systemLists {
		// Check if list exists
		var exists int
		d.db.QueryRow(`SELECT COUNT(*) FROM reading_lists WHERE user_id = ? AND list_type = ?`, userID, sl.listType).Scan(&exists)
		if exists == 0 {
			id := userID + "-" + sl.listType
			_, err := d.db.Exec(`
				INSERT INTO reading_lists (id, user_id, name, list_type, created_at)
				VALUES (?, ?, ?, ?, ?)`,
				id, userID, sl.name, sl.listType, time.Now(),
			)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// ReorderReadingList updates positions of books in a reading list
func (d *Database) ReorderReadingList(listID string, bookIDs []string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`UPDATE book_reading_list SET position = ? WHERE book_id = ? AND list_id = ?`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for i, bookID := range bookIDs {
		if _, err := stmt.Exec(i, bookID, listID); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// ==================== Tag Methods ====================

// CreateTag creates a new tag for a user
func (d *Database) CreateTag(tag *models.Tag) error {
	_, err := d.db.Exec(`
		INSERT INTO tags (id, user_id, name, color, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		tag.ID, tag.UserID, tag.Name, tag.Color, tag.CreatedAt,
	)
	return err
}

// GetTag returns a tag by ID
func (d *Database) GetTag(tagID string) (*models.Tag, error) {
	tag := &models.Tag{}
	err := d.db.QueryRow(`
		SELECT t.id, t.user_id, t.name, t.color, t.created_at,
			(SELECT COUNT(*) FROM book_tags WHERE tag_id = t.id) as book_count
		FROM tags t WHERE t.id = ?`, tagID).Scan(
		&tag.ID, &tag.UserID, &tag.Name, &tag.Color, &tag.CreatedAt, &tag.BookCount,
	)
	if err != nil {
		return nil, err
	}
	return tag, nil
}

// GetTagByName returns a tag by name for a specific user
func (d *Database) GetTagByName(userID, name string) (*models.Tag, error) {
	tag := &models.Tag{}
	err := d.db.QueryRow(`
		SELECT t.id, t.user_id, t.name, t.color, t.created_at,
			(SELECT COUNT(*) FROM book_tags WHERE tag_id = t.id) as book_count
		FROM tags t WHERE t.user_id = ? AND t.name = ?`, userID, name).Scan(
		&tag.ID, &tag.UserID, &tag.Name, &tag.Color, &tag.CreatedAt, &tag.BookCount,
	)
	if err != nil {
		return nil, err
	}
	return tag, nil
}

// ListTags returns all tags for a user
func (d *Database) ListTags(userID string) ([]*models.Tag, error) {
	rows, err := d.db.Query(`
		SELECT t.id, t.user_id, t.name, t.color, t.created_at,
			(SELECT COUNT(*) FROM book_tags WHERE tag_id = t.id) as book_count
		FROM tags t
		WHERE t.user_id = ?
		ORDER BY t.name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []*models.Tag
	for rows.Next() {
		tag := &models.Tag{}
		if err := rows.Scan(&tag.ID, &tag.UserID, &tag.Name, &tag.Color, &tag.CreatedAt, &tag.BookCount); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// UpdateTag updates a tag's name and/or color
func (d *Database) UpdateTag(tagID, name, color string) error {
	_, err := d.db.Exec(`UPDATE tags SET name = ?, color = ? WHERE id = ?`, name, color, tagID)
	return err
}

// DeleteTag removes a tag and all its book associations
func (d *Database) DeleteTag(tagID string) error {
	_, err := d.db.Exec(`DELETE FROM tags WHERE id = ?`, tagID)
	return err
}

// AddTagToBook adds a tag to a book
func (d *Database) AddTagToBook(bookID, tagID string) error {
	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO book_tags (book_id, tag_id, added_at)
		VALUES (?, ?, ?)`,
		bookID, tagID, time.Now(),
	)
	return err
}

// RemoveTagFromBook removes a tag from a book
func (d *Database) RemoveTagFromBook(bookID, tagID string) error {
	_, err := d.db.Exec(`DELETE FROM book_tags WHERE book_id = ? AND tag_id = ?`, bookID, tagID)
	return err
}

// GetBookTags returns all tags for a specific book
func (d *Database) GetBookTags(bookID string) ([]*models.Tag, error) {
	rows, err := d.db.Query(`
		SELECT t.id, t.user_id, t.name, t.color, t.created_at
		FROM tags t
		INNER JOIN book_tags bt ON t.id = bt.tag_id
		WHERE bt.book_id = ?
		ORDER BY t.name ASC`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []*models.Tag
	for rows.Next() {
		tag := &models.Tag{}
		if err := rows.Scan(&tag.ID, &tag.UserID, &tag.Name, &tag.Color, &tag.CreatedAt); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// GetBooksByTag returns all books with a specific tag
func (d *Database) GetBooksByTag(tagID string) ([]*models.Book, error) {
	rows, err := d.db.Query(`
		SELECT b.id, b.user_id, b.title, b.author, b.series, b.series_index, b.file_path, b.cover_path,
			b.file_size, b.uploaded_at, b.content_type, b.file_format, b.read_status, b.rating
		FROM books b
		INNER JOIN book_tags bt ON b.id = bt.book_id
		WHERE bt.tag_id = ?
		ORDER BY b.title ASC`, tagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []*models.Book
	for rows.Next() {
		book := &models.Book{}
		err := rows.Scan(&book.ID, &book.UserID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
			&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt,
			&book.ContentType, &book.FileFormat, &book.ReadStatus, &book.Rating)
		if err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

// ToggleBookTag toggles a tag on a book (adds if not present, removes if present)
func (d *Database) ToggleBookTag(bookID, tagID string) (bool, error) {
	// Check if tag is currently on the book
	var count int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM book_tags WHERE book_id = ? AND tag_id = ?`, bookID, tagID).Scan(&count)
	if err != nil {
		return false, err
	}

	if count > 0 {
		// Remove tag
		_, err = d.db.Exec(`DELETE FROM book_tags WHERE book_id = ? AND tag_id = ?`, bookID, tagID)
		return false, err
	}

	// Add tag
	_, err = d.db.Exec(`INSERT INTO book_tags (book_id, tag_id, added_at) VALUES (?, ?, ?)`, bookID, tagID, time.Now())
	return true, err
}

// ==================== Annotation Methods ====================

// CreateAnnotation creates a new annotation/highlight
func (d *Database) CreateAnnotation(ann *models.Annotation) error {
	_, err := d.db.Exec(`
		INSERT INTO annotations (id, book_id, user_id, chapter, cfi, start_offset, end_offset, selected_text, note, color, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ann.ID, ann.BookID, ann.UserID, ann.Chapter, ann.CFI, ann.StartOffset, ann.EndOffset,
		ann.SelectedText, ann.Note, ann.Color, ann.CreatedAt, ann.UpdatedAt,
	)
	return err
}

// GetAnnotation returns an annotation by ID
func (d *Database) GetAnnotation(annotationID string) (*models.Annotation, error) {
	ann := &models.Annotation{}
	err := d.db.QueryRow(`
		SELECT id, book_id, user_id, chapter, cfi, start_offset, end_offset, selected_text, note, color, created_at, updated_at
		FROM annotations WHERE id = ?`, annotationID).Scan(
		&ann.ID, &ann.BookID, &ann.UserID, &ann.Chapter, &ann.CFI, &ann.StartOffset, &ann.EndOffset,
		&ann.SelectedText, &ann.Note, &ann.Color, &ann.CreatedAt, &ann.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return ann, nil
}

// GetAnnotationsForBook returns all annotations for a book by a user
func (d *Database) GetAnnotationsForBook(bookID, userID string) ([]*models.Annotation, error) {
	rows, err := d.db.Query(`
		SELECT id, book_id, user_id, chapter, cfi, start_offset, end_offset, selected_text, note, color, created_at, updated_at
		FROM annotations
		WHERE book_id = ? AND user_id = ?
		ORDER BY chapter ASC, start_offset ASC`, bookID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var annotations []*models.Annotation
	for rows.Next() {
		ann := &models.Annotation{}
		if err := rows.Scan(&ann.ID, &ann.BookID, &ann.UserID, &ann.Chapter, &ann.CFI, &ann.StartOffset, &ann.EndOffset,
			&ann.SelectedText, &ann.Note, &ann.Color, &ann.CreatedAt, &ann.UpdatedAt); err != nil {
			return nil, err
		}
		annotations = append(annotations, ann)
	}
	return annotations, rows.Err()
}

// GetAnnotationsForChapter returns annotations for a specific chapter
func (d *Database) GetAnnotationsForChapter(bookID, userID, chapter string) ([]*models.Annotation, error) {
	rows, err := d.db.Query(`
		SELECT id, book_id, user_id, chapter, cfi, start_offset, end_offset, selected_text, note, color, created_at, updated_at
		FROM annotations
		WHERE book_id = ? AND user_id = ? AND chapter = ?
		ORDER BY start_offset ASC`, bookID, userID, chapter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var annotations []*models.Annotation
	for rows.Next() {
		ann := &models.Annotation{}
		if err := rows.Scan(&ann.ID, &ann.BookID, &ann.UserID, &ann.Chapter, &ann.CFI, &ann.StartOffset, &ann.EndOffset,
			&ann.SelectedText, &ann.Note, &ann.Color, &ann.CreatedAt, &ann.UpdatedAt); err != nil {
			return nil, err
		}
		annotations = append(annotations, ann)
	}
	return annotations, rows.Err()
}

// GetAllAnnotationsForUser returns all annotations across all books for a user
func (d *Database) GetAllAnnotationsForUser(userID string) ([]*models.Annotation, error) {
	rows, err := d.db.Query(`
		SELECT id, book_id, user_id, chapter, cfi, start_offset, end_offset, selected_text, note, color, created_at, updated_at
		FROM annotations
		WHERE user_id = ?
		ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var annotations []*models.Annotation
	for rows.Next() {
		ann := &models.Annotation{}
		if err := rows.Scan(&ann.ID, &ann.BookID, &ann.UserID, &ann.Chapter, &ann.CFI, &ann.StartOffset, &ann.EndOffset,
			&ann.SelectedText, &ann.Note, &ann.Color, &ann.CreatedAt, &ann.UpdatedAt); err != nil {
			return nil, err
		}
		annotations = append(annotations, ann)
	}
	return annotations, rows.Err()
}

// UpdateAnnotation updates an annotation's note and/or color
func (d *Database) UpdateAnnotation(annotationID, note, color string) error {
	_, err := d.db.Exec(`UPDATE annotations SET note = ?, color = ?, updated_at = ? WHERE id = ?`,
		note, color, time.Now(), annotationID)
	return err
}

// DeleteAnnotation removes an annotation
func (d *Database) DeleteAnnotation(annotationID string) error {
	_, err := d.db.Exec(`DELETE FROM annotations WHERE id = ?`, annotationID)
	return err
}

// DeleteAnnotationsForBook removes all annotations for a book by a user
func (d *Database) DeleteAnnotationsForBook(bookID, userID string) error {
	_, err := d.db.Exec(`DELETE FROM annotations WHERE book_id = ? AND user_id = ?`, bookID, userID)
	return err
}

// GetAnnotationCount returns the number of annotations for a book by a user
func (d *Database) GetAnnotationCount(bookID, userID string) (int, error) {
	var count int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM annotations WHERE book_id = ? AND user_id = ?`, bookID, userID).Scan(&count)
	return count, err
}

// GetAnnotationStats returns annotation statistics for a user
func (d *Database) GetAnnotationStats(userID string) (totalAnnotations int, booksWithAnnotations int, err error) {
	err = d.db.QueryRow(`SELECT COUNT(*) FROM annotations WHERE user_id = ?`, userID).Scan(&totalAnnotations)
	if err != nil {
		return 0, 0, err
	}
	err = d.db.QueryRow(`SELECT COUNT(DISTINCT book_id) FROM annotations WHERE user_id = ?`, userID).Scan(&booksWithAnnotations)
	return totalAnnotations, booksWithAnnotations, err
}

// SimilarBook represents a book with a similarity score
type SimilarBook struct {
	Book    *models.Book `json:"book"`
	Score   int          `json:"score"`   // Similarity score (higher is more similar)
	Reasons []string     `json:"reasons"` // Why this book is similar
}

// GetSimilarBooks finds books similar to the given book based on various criteria
func (d *Database) GetSimilarBooks(bookID, userID string, limit int) ([]*SimilarBook, error) {
	// First, get the source book
	book, err := d.GetBook(bookID)
	if err != nil {
		return nil, err
	}

	// Map to accumulate scores for each book
	similarBooks := make(map[string]*SimilarBook)

	// Helper to add a book to the results
	addBook := func(b *models.Book, score int, reason string) {
		if sb, exists := similarBooks[b.ID]; exists {
			sb.Score += score
			// Check if reason already exists
			found := false
			for _, r := range sb.Reasons {
				if r == reason {
					found = true
					break
				}
			}
			if !found {
				sb.Reasons = append(sb.Reasons, reason)
			}
		} else {
			similarBooks[b.ID] = &SimilarBook{
				Book:    b,
				Score:   score,
				Reasons: []string{reason},
			}
		}
	}

	// 1. Find books by same author (weight: 30)
	if book.Author != "" {
		rows, err := d.db.Query(`
			SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size,
				   uploaded_at, content_type, file_format, read_status, rating
			FROM books
			WHERE author = ? AND id != ? AND (user_id = ? OR user_id = '')
			LIMIT 20`, book.Author, bookID, userID)
		if err == nil {
			for rows.Next() {
				b := &models.Book{}
				err := rows.Scan(&b.ID, &b.UserID, &b.Title, &b.Author, &b.Series, &b.SeriesIndex,
					&b.FilePath, &b.CoverPath, &b.FileSize, &b.UploadedAt,
					&b.ContentType, &b.FileFormat, &b.ReadStatus, &b.Rating)
				if err != nil {
					continue
				}
				addBook(b, 30, "same author")
			}
			rows.Close()
		}
	}

	// 2. Find books in same series (weight: 50)
	if book.Series != "" {
		rows, err := d.db.Query(`
			SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size,
				   uploaded_at, content_type, file_format, read_status, rating
			FROM books
			WHERE series = ? AND id != ? AND (user_id = ? OR user_id = '')
			ORDER BY series_index ASC
			LIMIT 20`, book.Series, bookID, userID)
		if err == nil {
			for rows.Next() {
				b := &models.Book{}
				err := rows.Scan(&b.ID, &b.UserID, &b.Title, &b.Author, &b.Series, &b.SeriesIndex,
					&b.FilePath, &b.CoverPath, &b.FileSize, &b.UploadedAt,
					&b.ContentType, &b.FileFormat, &b.ReadStatus, &b.Rating)
				if err != nil {
					continue
				}
				addBook(b, 50, "same series")
			}
			rows.Close()
		}
	}

	// 3. Find books with overlapping subjects (weight: 20 per matching subject)
	if book.Subjects != "" {
		subjects := strings.Split(book.Subjects, ",")
		for _, subject := range subjects {
			subject = strings.TrimSpace(subject)
			if subject == "" {
				continue
			}
			rows, err := d.db.Query(`
				SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size,
					   uploaded_at, content_type, file_format, read_status, rating
				FROM books
				WHERE subjects LIKE ? AND id != ? AND (user_id = ? OR user_id = '')
				LIMIT 20`, "%"+subject+"%", bookID, userID)
			if err == nil {
				for rows.Next() {
					b := &models.Book{}
					err := rows.Scan(&b.ID, &b.UserID, &b.Title, &b.Author, &b.Series, &b.SeriesIndex,
						&b.FilePath, &b.CoverPath, &b.FileSize, &b.UploadedAt,
						&b.ContentType, &b.FileFormat, &b.ReadStatus, &b.Rating)
					if err != nil {
						continue
					}
					addBook(b, 20, "similar subjects")
				}
				rows.Close()
			}
		}
	}

	// 4. Find books with same tags (weight: 15 per matching tag)
	tagRows, err := d.db.Query(`
		SELECT DISTINCT bt2.book_id
		FROM book_tags bt1
		JOIN book_tags bt2 ON bt1.tag_id = bt2.tag_id
		JOIN books b ON bt2.book_id = b.id
		WHERE bt1.book_id = ? AND bt2.book_id != ? AND (b.user_id = ? OR b.user_id = '')
		LIMIT 50`, bookID, bookID, userID)
	if err == nil {
		for tagRows.Next() {
			var relatedBookID string
			if err := tagRows.Scan(&relatedBookID); err != nil {
				continue
			}
			if sb, exists := similarBooks[relatedBookID]; exists {
				sb.Score += 15
				found := false
				for _, r := range sb.Reasons {
					if r == "shared tags" {
						found = true
						break
					}
				}
				if !found {
					sb.Reasons = append(sb.Reasons, "shared tags")
				}
			} else {
				// Fetch the book
				relatedBook, err := d.GetBook(relatedBookID)
				if err == nil {
					similarBooks[relatedBookID] = &SimilarBook{
						Book:    relatedBook,
						Score:   15,
						Reasons: []string{"shared tags"},
					}
				}
			}
		}
		tagRows.Close()
	}

	// 5. Find books with same content type (weight: 5)
	rows, err := d.db.Query(`
		SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size,
			   uploaded_at, content_type, file_format, read_status, rating
		FROM books
		WHERE content_type = ? AND id != ? AND (user_id = ? OR user_id = '')
		LIMIT 50`, book.ContentType, bookID, userID)
	if err == nil {
		for rows.Next() {
			b := &models.Book{}
			err := rows.Scan(&b.ID, &b.UserID, &b.Title, &b.Author, &b.Series, &b.SeriesIndex,
				&b.FilePath, &b.CoverPath, &b.FileSize, &b.UploadedAt,
				&b.ContentType, &b.FileFormat, &b.ReadStatus, &b.Rating)
			if err != nil {
				continue
			}
			addBook(b, 5, "same type")
		}
		rows.Close()
	}

	// Convert map to slice and sort by score
	result := make([]*SimilarBook, 0, len(similarBooks))
	for _, sb := range similarBooks {
		// Only include books with score > 5 (just same type isn't enough)
		if sb.Score > 5 {
			result = append(result, sb)
		}
	}

	// Sort by score descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	// Apply limit
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// ============================================================================
// Reading Statistics Methods
// ============================================================================

// CreateReadingSession creates a new reading session
func (d *Database) CreateReadingSession(session *models.ReadingSession) error {
	_, err := d.db.Exec(`
		INSERT INTO reading_sessions (id, user_id, book_id, start_time, end_time, pages_read, chapters_read, duration_seconds, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.BookID, session.StartTime, session.EndTime,
		session.PagesRead, session.ChaptersRead, session.DurationSeconds, session.CreatedAt,
	)
	return err
}

// UpdateReadingSession updates an existing reading session (e.g., when ending it)
func (d *Database) UpdateReadingSession(session *models.ReadingSession) error {
	_, err := d.db.Exec(`
		UPDATE reading_sessions SET
			end_time = ?, pages_read = ?, chapters_read = ?, duration_seconds = ?
		WHERE id = ?`,
		session.EndTime, session.PagesRead, session.ChaptersRead, session.DurationSeconds, session.ID,
	)
	return err
}

// GetActiveReadingSession gets an active (not ended) reading session for a user and book
func (d *Database) GetActiveReadingSession(userID, bookID string) (*models.ReadingSession, error) {
	session := &models.ReadingSession{}
	err := d.db.QueryRow(`
		SELECT id, user_id, book_id, start_time, end_time, pages_read, chapters_read, duration_seconds, created_at
		FROM reading_sessions
		WHERE user_id = ? AND book_id = ? AND end_time IS NULL
		ORDER BY start_time DESC LIMIT 1`,
		userID, bookID,
	).Scan(&session.ID, &session.UserID, &session.BookID, &session.StartTime, &session.EndTime,
		&session.PagesRead, &session.ChaptersRead, &session.DurationSeconds, &session.CreatedAt)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// GetRecentReadingSessions returns recent reading sessions for a user
func (d *Database) GetRecentReadingSessions(userID string, limit int) ([]models.ReadingSession, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := d.db.Query(`
		SELECT rs.id, rs.user_id, rs.book_id, rs.start_time, rs.end_time,
			rs.pages_read, rs.chapters_read, rs.duration_seconds, rs.created_at,
			b.title, b.author
		FROM reading_sessions rs
		JOIN books b ON rs.book_id = b.id
		WHERE rs.user_id = ? AND rs.end_time IS NOT NULL
		ORDER BY rs.start_time DESC
		LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.ReadingSession
	for rows.Next() {
		var s models.ReadingSession
		if err := rows.Scan(&s.ID, &s.UserID, &s.BookID, &s.StartTime, &s.EndTime,
			&s.PagesRead, &s.ChaptersRead, &s.DurationSeconds, &s.CreatedAt,
			&s.BookTitle, &s.BookAuthor); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// GetOrCreateUserStatistics gets or creates user statistics record
func (d *Database) GetOrCreateUserStatistics(userID string) (*models.UserStatistics, error) {
	stats := &models.UserStatistics{UserID: userID}
	err := d.db.QueryRow(`
		SELECT user_id, total_books_read, total_pages_read, total_chapters_read,
			total_time_seconds, current_streak, longest_streak, last_reading_date, updated_at
		FROM user_statistics WHERE user_id = ?`, userID,
	).Scan(&stats.UserID, &stats.TotalBooksRead, &stats.TotalPagesRead, &stats.TotalChaptersRead,
		&stats.TotalTimeSeconds, &stats.CurrentStreak, &stats.LongestStreak,
		&stats.LastReadingDate, &stats.UpdatedAt)

	if err == sql.ErrNoRows {
		// Create new record
		now := time.Now()
		_, err = d.db.Exec(`
			INSERT INTO user_statistics (user_id, total_books_read, total_pages_read, total_chapters_read,
				total_time_seconds, current_streak, longest_streak, updated_at)
			VALUES (?, 0, 0, 0, 0, 0, 0, ?)`, userID, now)
		if err != nil {
			return nil, err
		}
		stats.UpdatedAt = now
		return stats, nil
	}
	if err != nil {
		return nil, err
	}

	// Calculate average pace
	if stats.TotalPagesRead > 0 && stats.TotalTimeSeconds > 0 {
		stats.AveragePaceMinutes = float64(stats.TotalTimeSeconds) / 60.0 / float64(stats.TotalPagesRead)
	}

	// Format total time
	stats.TotalTimeFormatted = formatDuration(stats.TotalTimeSeconds)

	return stats, nil
}

// UpdateUserStatistics updates the user's aggregated statistics
func (d *Database) UpdateUserStatistics(stats *models.UserStatistics) error {
	_, err := d.db.Exec(`
		UPDATE user_statistics SET
			total_books_read = ?, total_pages_read = ?, total_chapters_read = ?,
			total_time_seconds = ?, current_streak = ?, longest_streak = ?,
			last_reading_date = ?, updated_at = ?
		WHERE user_id = ?`,
		stats.TotalBooksRead, stats.TotalPagesRead, stats.TotalChaptersRead,
		stats.TotalTimeSeconds, stats.CurrentStreak, stats.LongestStreak,
		stats.LastReadingDate, time.Now(), stats.UserID,
	)
	return err
}

// GetDailyReadingStats returns daily reading statistics for a date range
func (d *Database) GetDailyReadingStats(userID string, startDate, endDate time.Time) ([]models.DailyReadingStats, error) {
	rows, err := d.db.Query(`
		SELECT id, user_id, reading_date, pages_read, chapters_read, time_seconds, books_touched
		FROM daily_reading_stats
		WHERE user_id = ? AND reading_date >= ? AND reading_date <= ?
		ORDER BY reading_date ASC`, userID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.DailyReadingStats
	for rows.Next() {
		var s models.DailyReadingStats
		var dateStr string
		if err := rows.Scan(&s.ID, &s.UserID, &dateStr, &s.PagesRead, &s.ChaptersRead, &s.TimeSeconds, &s.BooksTouched); err != nil {
			return nil, err
		}
		s.ReadingDate, _ = time.Parse("2006-01-02", dateStr)
		stats = append(stats, s)
	}
	return stats, nil
}

// UpdateDailyStats updates or creates daily reading stats
func (d *Database) UpdateDailyStats(userID string, date time.Time, pagesRead, chaptersRead, timeSeconds int, bookID string) error {
	dateStr := date.Format("2006-01-02")

	// Check if record exists
	var existingID string
	var existingPages, existingChapters, existingTime, existingBooks int
	err := d.db.QueryRow(`
		SELECT id, pages_read, chapters_read, time_seconds, books_touched
		FROM daily_reading_stats WHERE user_id = ? AND reading_date = ?`,
		userID, dateStr).Scan(&existingID, &existingPages, &existingChapters, &existingTime, &existingBooks)

	if err == sql.ErrNoRows {
		// Create new record
		id := generateUUID()
		_, err = d.db.Exec(`
			INSERT INTO daily_reading_stats (id, user_id, reading_date, pages_read, chapters_read, time_seconds, books_touched)
			VALUES (?, ?, ?, ?, ?, ?, 1)`,
			id, userID, dateStr, pagesRead, chaptersRead, timeSeconds)
		return err
	}
	if err != nil {
		return err
	}

	// Update existing record
	_, err = d.db.Exec(`
		UPDATE daily_reading_stats SET
			pages_read = pages_read + ?, chapters_read = chapters_read + ?, time_seconds = time_seconds + ?
		WHERE id = ?`,
		pagesRead, chaptersRead, timeSeconds, existingID)
	return err
}

// CalculateStreak calculates the current reading streak for a user
func (d *Database) CalculateStreak(userID string) (current, longest int, err error) {
	// Get distinct reading dates in descending order
	rows, err := d.db.Query(`
		SELECT DISTINCT DATE(start_time) as reading_date
		FROM reading_sessions
		WHERE user_id = ? AND end_time IS NOT NULL
		ORDER BY reading_date DESC`, userID)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var dateStr string
		if err := rows.Scan(&dateStr); err != nil {
			return 0, 0, err
		}
		d, _ := time.Parse("2006-01-02", dateStr)
		dates = append(dates, d)
	}

	if len(dates) == 0 {
		return 0, 0, nil
	}

	today := time.Now().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)
	lastReading := dates[0].Truncate(24 * time.Hour)

	// If last reading wasn't today or yesterday, streak is broken
	if !lastReading.Equal(today) && !lastReading.Equal(yesterday) {
		// Get longest streak from history
		longest = d.calculateLongestStreak(dates)
		return 0, longest, nil
	}

	// Count current streak
	current = 1
	for i := 0; i < len(dates)-1; i++ {
		curr := dates[i].Truncate(24 * time.Hour)
		prev := dates[i+1].Truncate(24 * time.Hour)
		if curr.AddDate(0, 0, -1).Equal(prev) {
			current++
		} else {
			break
		}
	}

	// Calculate longest streak
	longest = d.calculateLongestStreak(dates)
	if current > longest {
		longest = current
	}

	return current, longest, nil
}

func (d *Database) calculateLongestStreak(dates []time.Time) int {
	if len(dates) == 0 {
		return 0
	}

	longest := 1
	currentStreak := 1

	for i := 0; i < len(dates)-1; i++ {
		curr := dates[i].Truncate(24 * time.Hour)
		prev := dates[i+1].Truncate(24 * time.Hour)
		if curr.AddDate(0, 0, -1).Equal(prev) {
			currentStreak++
			if currentStreak > longest {
				longest = currentStreak
			}
		} else {
			currentStreak = 1
		}
	}

	return longest
}

// GetReadingStatsForBook returns reading statistics for a specific book
func (d *Database) GetReadingStatsForBook(userID, bookID string) (totalTime, pagesRead, sessionsCount int, err error) {
	err = d.db.QueryRow(`
		SELECT COALESCE(SUM(duration_seconds), 0), COALESCE(SUM(pages_read), 0), COUNT(*)
		FROM reading_sessions
		WHERE user_id = ? AND book_id = ? AND end_time IS NOT NULL`,
		userID, bookID).Scan(&totalTime, &pagesRead, &sessionsCount)
	return
}

// GetCompletedBooksCount returns the count of books marked as completed
func (d *Database) GetCompletedBooksCount(userID string) (int, error) {
	var count int
	err := d.db.QueryRow(`
		SELECT COUNT(*) FROM books WHERE user_id = ? AND read_status = 'completed'`, userID).Scan(&count)
	return count, err
}

// Helper function to format duration
func formatDuration(seconds int) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return "< 1m"
}

// Helper to generate UUID - uses the same pattern as elsewhere in the codebase
func generateUUID() string {
	// Simple UUID generation - in production, use google/uuid
	return time.Now().Format("20060102150405") + "-" + strings.ReplaceAll(time.Now().String()[20:29], ".", "")
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}
