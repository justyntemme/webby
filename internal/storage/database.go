package storage

import (
	"database/sql"
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
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
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
	return err
}

// CreateBook inserts a new book into the database
func (d *Database) CreateBook(book *models.Book) error {
	_, err := d.db.Exec(`
		INSERT INTO books (id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		book.ID, book.UserID, book.Title, book.Author, book.Series, book.SeriesIndex,
		book.FilePath, book.CoverPath, book.FileSize, book.UploadedAt,
	)
	return err
}

// GetBook retrieves a book by ID
func (d *Database) GetBook(id string) (*models.Book, error) {
	book := &models.Book{}
	err := d.db.QueryRow(`
		SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at
		FROM books WHERE id = ?`, id,
	).Scan(&book.ID, &book.UserID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
		&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt)
	if err != nil {
		return nil, err
	}
	return book, nil
}

// GetBookForUser retrieves a book by ID if user has access (owner or shared)
func (d *Database) GetBookForUser(id, userID string) (*models.Book, error) {
	book := &models.Book{}
	err := d.db.QueryRow(`
		SELECT b.id, b.user_id, b.title, b.author, b.series, b.series_index, b.file_path, b.cover_path, b.file_size, b.uploaded_at
		FROM books b
		LEFT JOIN book_shares bs ON b.id = bs.book_id AND bs.shared_with_id = ?
		WHERE b.id = ? AND (b.user_id = ? OR b.user_id = '' OR bs.id IS NOT NULL)`, userID, id, userID,
	).Scan(&book.ID, &book.UserID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
		&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt)
	if err != nil {
		return nil, err
	}
	return book, nil
}

// ListBooks returns all books with optional sorting (legacy - no user filter)
func (d *Database) ListBooks(sortBy, order string) ([]models.Book, error) {
	return d.ListBooksForUser("", sortBy, order)
}

// ListBooksForUser returns books for a specific user with optional sorting
func (d *Database) ListBooksForUser(userID, sortBy, order string) ([]models.Book, error) {
	validSort := map[string]string{
		"title":  "title",
		"author": "author",
		"series": "series, series_index",
		"date":   "uploaded_at",
	}

	sortColumn, ok := validSort[sortBy]
	if !ok {
		sortColumn = "title"
	}

	if order != "desc" {
		order = "asc"
	}

	var query string
	var args []interface{}

	if userID != "" {
		query = "SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at FROM books WHERE user_id = ? ORDER BY " + sortColumn + " " + order
		args = append(args, userID)
	} else {
		query = "SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at FROM books WHERE user_id = '' ORDER BY " + sortColumn + " " + order
	}

	rows, err := d.db.Query(query, args...)
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
			SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at
			FROM books
			WHERE user_id = ? AND (title LIKE ? OR author LIKE ? OR series LIKE ?)
			ORDER BY title`,
			userID, searchTerm, searchTerm, searchTerm,
		)
	} else {
		rows, err = d.db.Query(`
			SELECT id, user_id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at
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
			&book.FilePath, &book.CoverPath, &book.FileSize, &book.UploadedAt)
		if err != nil {
			return nil, err
		}
		books = append(books, book)
	}

	return books, nil
}

// GetBooksByAuthor returns books grouped by author
func (d *Database) GetBooksByAuthor() (map[string][]models.Book, error) {
	books, err := d.ListBooks("author", "asc")
	if err != nil {
		return nil, err
	}

	grouped := make(map[string][]models.Book)
	for _, book := range books {
		grouped[book.Author] = append(grouped[book.Author], book)
	}

	return grouped, nil
}

// GetBooksBySeries returns books grouped by series
func (d *Database) GetBooksBySeries() (map[string][]models.Book, error) {
	rows, err := d.db.Query(`
		SELECT id, title, author, series, series_index, file_path, cover_path, file_size, uploaded_at
		FROM books
		WHERE series != ''
		ORDER BY series, series_index`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grouped := make(map[string][]models.Book)
	for rows.Next() {
		var book models.Book
		err := rows.Scan(&book.ID, &book.Title, &book.Author, &book.Series, &book.SeriesIndex,
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

// SaveReadingPosition saves or updates reading position
func (d *Database) SaveReadingPosition(pos *models.ReadingPosition) error {
	_, err := d.db.Exec(`
		INSERT INTO reading_positions (book_id, chapter, position, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(book_id) DO UPDATE SET
			chapter = excluded.chapter,
			position = excluded.position,
			updated_at = excluded.updated_at`,
		pos.BookID, pos.Chapter, pos.Position, time.Now(),
	)
	return err
}

// GetReadingPosition retrieves reading position for a book
func (d *Database) GetReadingPosition(bookID string) (*models.ReadingPosition, error) {
	pos := &models.ReadingPosition{}
	err := d.db.QueryRow(`
		SELECT book_id, chapter, position, updated_at
		FROM reading_positions WHERE book_id = ?`, bookID,
	).Scan(&pos.BookID, &pos.Chapter, &pos.Position, &pos.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return pos, nil
}

// CreateCollection creates a new collection
func (d *Database) CreateCollection(collection *models.Collection) error {
	_, err := d.db.Exec(`
		INSERT INTO collections (id, name, created_at)
		VALUES (?, ?, ?)`,
		collection.ID, collection.Name, collection.CreatedAt,
	)
	return err
}

// GetCollection retrieves a collection by ID
func (d *Database) GetCollection(id string) (*models.Collection, error) {
	collection := &models.Collection{}
	err := d.db.QueryRow(`
		SELECT id, name, created_at FROM collections WHERE id = ?`, id,
	).Scan(&collection.ID, &collection.Name, &collection.CreatedAt)
	if err != nil {
		return nil, err
	}
	return collection, nil
}

// ListCollections returns all collections
func (d *Database) ListCollections() ([]models.Collection, error) {
	rows, err := d.db.Query(`SELECT id, name, created_at FROM collections ORDER BY name`)
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

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}
