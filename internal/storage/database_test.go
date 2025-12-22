package storage

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justyntemme/webby/internal/models"
)

func setupTestDB(t *testing.T) (*Database, func()) {
	tmpFile, err := os.CreateTemp("", "webby-test-*.db")
	require.NoError(t, err)
	tmpFile.Close()

	db, err := NewDatabase(tmpFile.Name())
	require.NoError(t, err)

	cleanup := func() {
		db.Close()
		os.Remove(tmpFile.Name())
	}

	return db, cleanup
}

func TestCreateAndGetUser(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	user := &models.User{
		ID:           "test-user-id",
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hashedpassword",
		CreatedAt:    time.Now(),
	}

	err := db.CreateUser(user)
	require.NoError(t, err)

	// Get by ID
	retrieved, err := db.GetUserByID(user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved.ID)
	assert.Equal(t, user.Username, retrieved.Username)
	assert.Equal(t, user.Email, retrieved.Email)

	// Get by username
	retrieved, err = db.GetUserByUsername(user.Username)
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved.ID)

	// Get by email
	retrieved, err = db.GetUserByEmail(user.Email)
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved.ID)
}

func TestUserExists(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	user := &models.User{
		ID:           "test-user-id",
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hashedpassword",
		CreatedAt:    time.Now(),
	}

	// Before creation
	exists, err := db.UserExists(user.Username, user.Email)
	require.NoError(t, err)
	assert.False(t, exists)

	// Create user
	err = db.CreateUser(user)
	require.NoError(t, err)

	// After creation
	exists, err = db.UserExists(user.Username, user.Email)
	require.NoError(t, err)
	assert.True(t, exists)

	// Check with different values
	exists, err = db.UserExists("other", "other@example.com")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestCreateAndGetBook(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	book := &models.Book{
		ID:          "test-book-id",
		UserID:      "test-user-id",
		Title:       "Test Book",
		Author:      "Test Author",
		Series:      "Test Series",
		SeriesIndex: 1,
		FilePath:    "/path/to/book.epub",
		CoverPath:   "/path/to/cover.jpg",
		FileSize:    1024,
		UploadedAt:  time.Now(),
	}

	err := db.CreateBook(book)
	require.NoError(t, err)

	retrieved, err := db.GetBook(book.ID)
	require.NoError(t, err)
	assert.Equal(t, book.ID, retrieved.ID)
	assert.Equal(t, book.UserID, retrieved.UserID)
	assert.Equal(t, book.Title, retrieved.Title)
	assert.Equal(t, book.Author, retrieved.Author)
}

func TestListBooksForUser(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create books for two users
	book1 := &models.Book{
		ID:         "book-1",
		UserID:     "user-1",
		Title:      "Book 1",
		Author:     "Author 1",
		FilePath:   "/path/1.epub",
		UploadedAt: time.Now(),
	}
	book2 := &models.Book{
		ID:         "book-2",
		UserID:     "user-2",
		Title:      "Book 2",
		Author:     "Author 2",
		FilePath:   "/path/2.epub",
		UploadedAt: time.Now(),
	}

	require.NoError(t, db.CreateBook(book1))
	require.NoError(t, db.CreateBook(book2))

	// List for user 1
	books, err := db.ListBooksForUser("user-1", "title", "asc")
	require.NoError(t, err)
	assert.Len(t, books, 1)
	assert.Equal(t, "Book 1", books[0].Title)

	// List for user 2
	books, err = db.ListBooksForUser("user-2", "title", "asc")
	require.NoError(t, err)
	assert.Len(t, books, 1)
	assert.Equal(t, "Book 2", books[0].Title)
}

func TestBookSharing(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create users
	owner := &models.User{ID: "owner-id", Username: "owner", Email: "owner@example.com", PasswordHash: "hash", CreatedAt: time.Now()}
	recipient := &models.User{ID: "recipient-id", Username: "recipient", Email: "recipient@example.com", PasswordHash: "hash", CreatedAt: time.Now()}
	require.NoError(t, db.CreateUser(owner))
	require.NoError(t, db.CreateUser(recipient))

	// Create book
	book := &models.Book{ID: "book-id", UserID: owner.ID, Title: "Shared Book", Author: "Author", FilePath: "/path.epub", UploadedAt: time.Now()}
	require.NoError(t, db.CreateBook(book))

	// Share book
	err := db.ShareBook(book.ID, owner.ID, recipient.ID)
	require.NoError(t, err)

	// Check sharing
	shared, err := db.IsBookSharedWith(book.ID, recipient.ID)
	require.NoError(t, err)
	assert.True(t, shared)

	// Get shared books
	sharedBooks, err := db.GetSharedBooks(recipient.ID)
	require.NoError(t, err)
	assert.Len(t, sharedBooks, 1)
	assert.Equal(t, book.ID, sharedBooks[0].ID)

	// Unshare
	err = db.UnshareBook(book.ID, recipient.ID)
	require.NoError(t, err)

	shared, err = db.IsBookSharedWith(book.ID, recipient.ID)
	require.NoError(t, err)
	assert.False(t, shared)
}

func TestCollections(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	collection := &models.Collection{
		ID:        "collection-id",
		UserID:    "user-id",
		Name:      "My Collection",
		CreatedAt: time.Now(),
	}

	err := db.CreateCollection(collection)
	require.NoError(t, err)

	retrieved, err := db.GetCollection(collection.ID)
	require.NoError(t, err)
	assert.Equal(t, collection.ID, retrieved.ID)
	assert.Equal(t, collection.Name, retrieved.Name)

	// Update
	err = db.UpdateCollection(collection.ID, "Updated Name")
	require.NoError(t, err)

	retrieved, err = db.GetCollection(collection.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", retrieved.Name)

	// Delete
	err = db.DeleteCollection(collection.ID)
	require.NoError(t, err)

	_, err = db.GetCollection(collection.ID)
	assert.Error(t, err)
}

func TestReadingPositionPerUser(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a book
	book := &models.Book{
		ID:         "test-book",
		Title:      "Test Book",
		Author:     "Author",
		FilePath:   "/path/book.epub",
		UploadedAt: time.Now(),
	}
	require.NoError(t, db.CreateBook(book))

	// Save position for user 1
	pos1 := &models.ReadingPosition{
		BookID:   book.ID,
		UserID:   "user-1",
		Chapter:  "5",
		Position: 0.75,
	}
	err := db.SaveReadingPosition(pos1)
	require.NoError(t, err)

	// Save position for user 2 (same book, different position)
	pos2 := &models.ReadingPosition{
		BookID:   book.ID,
		UserID:   "user-2",
		Chapter:  "2",
		Position: 0.25,
	}
	err = db.SaveReadingPosition(pos2)
	require.NoError(t, err)

	// Get position for user 1
	retrieved1, err := db.GetReadingPosition(book.ID, "user-1")
	require.NoError(t, err)
	assert.Equal(t, "5", retrieved1.Chapter)
	assert.Equal(t, 0.75, retrieved1.Position)
	assert.Equal(t, "user-1", retrieved1.UserID)

	// Get position for user 2
	retrieved2, err := db.GetReadingPosition(book.ID, "user-2")
	require.NoError(t, err)
	assert.Equal(t, "2", retrieved2.Chapter)
	assert.Equal(t, 0.25, retrieved2.Position)
	assert.Equal(t, "user-2", retrieved2.UserID)

	// Positions are independent
	assert.NotEqual(t, retrieved1.Chapter, retrieved2.Chapter)
	assert.NotEqual(t, retrieved1.Position, retrieved2.Position)
}

func TestReadingPositionUpdate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a book
	book := &models.Book{
		ID:         "test-book",
		Title:      "Test Book",
		Author:     "Author",
		FilePath:   "/path/book.epub",
		UploadedAt: time.Now(),
	}
	require.NoError(t, db.CreateBook(book))

	// Save initial position
	pos := &models.ReadingPosition{
		BookID:   book.ID,
		UserID:   "user-1",
		Chapter:  "1",
		Position: 0.0,
	}
	err := db.SaveReadingPosition(pos)
	require.NoError(t, err)

	// Update position (same user, same book)
	pos.Chapter = "10"
	pos.Position = 0.5
	err = db.SaveReadingPosition(pos)
	require.NoError(t, err)

	// Verify updated
	retrieved, err := db.GetReadingPosition(book.ID, "user-1")
	require.NoError(t, err)
	assert.Equal(t, "10", retrieved.Chapter)
	assert.Equal(t, 0.5, retrieved.Position)
}

func TestReadingPositionUnauthenticated(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a book
	book := &models.Book{
		ID:         "test-book",
		Title:      "Test Book",
		Author:     "Author",
		FilePath:   "/path/book.epub",
		UploadedAt: time.Now(),
	}
	require.NoError(t, db.CreateBook(book))

	// Save position for unauthenticated user (empty user_id)
	pos := &models.ReadingPosition{
		BookID:   book.ID,
		UserID:   "", // Empty = unauthenticated
		Chapter:  "3",
		Position: 0.33,
	}
	err := db.SaveReadingPosition(pos)
	require.NoError(t, err)

	// Get position for unauthenticated user
	retrieved, err := db.GetReadingPosition(book.ID, "")
	require.NoError(t, err)
	assert.Equal(t, "3", retrieved.Chapter)
	assert.Equal(t, 0.33, retrieved.Position)

	// Authenticated user has separate position
	authPos := &models.ReadingPosition{
		BookID:   book.ID,
		UserID:   "auth-user",
		Chapter:  "7",
		Position: 0.77,
	}
	err = db.SaveReadingPosition(authPos)
	require.NoError(t, err)

	// Verify both positions are independent
	unauthRetrieved, err := db.GetReadingPosition(book.ID, "")
	require.NoError(t, err)
	assert.Equal(t, "3", unauthRetrieved.Chapter)

	authRetrieved, err := db.GetReadingPosition(book.ID, "auth-user")
	require.NoError(t, err)
	assert.Equal(t, "7", authRetrieved.Chapter)
}

func TestReadingPositionNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a book
	book := &models.Book{
		ID:         "test-book",
		Title:      "Test Book",
		Author:     "Author",
		FilePath:   "/path/book.epub",
		UploadedAt: time.Now(),
	}
	require.NoError(t, db.CreateBook(book))

	// Try to get non-existent position
	_, err := db.GetReadingPosition(book.ID, "nonexistent-user")
	assert.Error(t, err)
}

func TestGetBooksByAuthorForUser(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create books for two different users with same author
	book1 := &models.Book{
		ID:         "book-1",
		UserID:     "user-1",
		Title:      "Book 1",
		Author:     "Author A",
		FilePath:   "/path/1.epub",
		UploadedAt: time.Now(),
	}
	book2 := &models.Book{
		ID:         "book-2",
		UserID:     "user-1",
		Title:      "Book 2",
		Author:     "Author A",
		FilePath:   "/path/2.epub",
		UploadedAt: time.Now(),
	}
	book3 := &models.Book{
		ID:         "book-3",
		UserID:     "user-2",
		Title:      "Book 3",
		Author:     "Author A",
		FilePath:   "/path/3.epub",
		UploadedAt: time.Now(),
	}
	book4 := &models.Book{
		ID:         "book-4",
		UserID:     "user-1",
		Title:      "Book 4",
		Author:     "Author B",
		FilePath:   "/path/4.epub",
		UploadedAt: time.Now(),
	}

	require.NoError(t, db.CreateBook(book1))
	require.NoError(t, db.CreateBook(book2))
	require.NoError(t, db.CreateBook(book3))
	require.NoError(t, db.CreateBook(book4))

	// Get books by author for user-1
	grouped, err := db.GetBooksByAuthorForUser("user-1")
	require.NoError(t, err)

	// Should have 2 authors for user-1
	assert.Len(t, grouped, 2)

	// Author A should have 2 books for user-1
	assert.Len(t, grouped["Author A"], 2)

	// Author B should have 1 book for user-1
	assert.Len(t, grouped["Author B"], 1)

	// Get books by author for user-2
	grouped2, err := db.GetBooksByAuthorForUser("user-2")
	require.NoError(t, err)

	// Should have 1 author for user-2
	assert.Len(t, grouped2, 1)
	assert.Len(t, grouped2["Author A"], 1)
}

func TestGetBooksBySeriesForUser(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create books with series for two users
	book1 := &models.Book{
		ID:          "book-1",
		UserID:      "user-1",
		Title:       "Book 1",
		Author:      "Author",
		Series:      "Series X",
		SeriesIndex: 1,
		FilePath:    "/path/1.epub",
		UploadedAt:  time.Now(),
	}
	book2 := &models.Book{
		ID:          "book-2",
		UserID:      "user-1",
		Title:       "Book 2",
		Author:      "Author",
		Series:      "Series X",
		SeriesIndex: 2,
		FilePath:    "/path/2.epub",
		UploadedAt:  time.Now(),
	}
	book3 := &models.Book{
		ID:          "book-3",
		UserID:      "user-2",
		Title:       "Book 3",
		Author:      "Author",
		Series:      "Series X",
		SeriesIndex: 3,
		FilePath:    "/path/3.epub",
		UploadedAt:  time.Now(),
	}
	book4 := &models.Book{
		ID:         "book-4",
		UserID:     "user-1",
		Title:      "Book 4",
		Author:     "Author",
		Series:     "",  // No series
		FilePath:   "/path/4.epub",
		UploadedAt: time.Now(),
	}

	require.NoError(t, db.CreateBook(book1))
	require.NoError(t, db.CreateBook(book2))
	require.NoError(t, db.CreateBook(book3))
	require.NoError(t, db.CreateBook(book4))

	// Get books by series for user-1
	grouped, err := db.GetBooksBySeriesForUser("user-1")
	require.NoError(t, err)

	// Should have 1 series for user-1 (book4 has no series)
	assert.Len(t, grouped, 1)

	// Series X should have 2 books for user-1
	assert.Len(t, grouped["Series X"], 2)

	// Verify series order
	assert.Equal(t, float64(1), grouped["Series X"][0].SeriesIndex)
	assert.Equal(t, float64(2), grouped["Series X"][1].SeriesIndex)

	// Get books by series for user-2
	grouped2, err := db.GetBooksBySeriesForUser("user-2")
	require.NoError(t, err)

	// Should have 1 series for user-2
	assert.Len(t, grouped2, 1)
	assert.Len(t, grouped2["Series X"], 1)
}

func TestCreateAndGetBookWithMetadata(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	now := time.Now()
	book := &models.Book{
		ID:              "test-book-id",
		UserID:          "test-user-id",
		Title:           "Test Book",
		Author:          "Test Author",
		Series:          "Test Series",
		SeriesIndex:     1,
		FilePath:        "/path/to/book.epub",
		CoverPath:       "/path/to/cover.jpg",
		FileSize:        1024,
		UploadedAt:      now,
		ISBN:            "9780123456789",
		Publisher:       "Test Publisher",
		PublishDate:     "2023-05-15",
		Description:     "A great book about testing.",
		Language:        "en",
		Subjects:        "Fiction, Testing, Unit Tests",
		MetadataSource:  "openlibrary",
		MetadataUpdated: &now,
	}

	err := db.CreateBook(book)
	require.NoError(t, err)

	retrieved, err := db.GetBook(book.ID)
	require.NoError(t, err)

	assert.Equal(t, book.ID, retrieved.ID)
	assert.Equal(t, book.Title, retrieved.Title)
	assert.Equal(t, book.Author, retrieved.Author)
	assert.Equal(t, book.ISBN, retrieved.ISBN)
	assert.Equal(t, book.Publisher, retrieved.Publisher)
	assert.Equal(t, book.PublishDate, retrieved.PublishDate)
	assert.Equal(t, book.Description, retrieved.Description)
	assert.Equal(t, book.Language, retrieved.Language)
	assert.Equal(t, book.Subjects, retrieved.Subjects)
	assert.Equal(t, book.MetadataSource, retrieved.MetadataSource)
	assert.NotNil(t, retrieved.MetadataUpdated)
}

func TestUpdateBookMetadata(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create initial book
	book := &models.Book{
		ID:             "test-book-id",
		UserID:         "test-user-id",
		Title:          "Original Title",
		Author:         "Original Author",
		FilePath:       "/path/to/book.epub",
		UploadedAt:     time.Now(),
		MetadataSource: "epub",
	}

	err := db.CreateBook(book)
	require.NoError(t, err)

	// Update metadata
	now := time.Now()
	book.Title = "Updated Title"
	book.Author = "Updated Author"
	book.ISBN = "9780123456789"
	book.Publisher = "New Publisher"
	book.PublishDate = "2024-01-01"
	book.Description = "Updated description"
	book.Language = "en-US"
	book.Subjects = "Updated, Subjects"
	book.MetadataSource = "openlibrary"
	book.MetadataUpdated = &now

	err = db.UpdateBookMetadata(book)
	require.NoError(t, err)

	// Verify updates
	retrieved, err := db.GetBook(book.ID)
	require.NoError(t, err)

	assert.Equal(t, "Updated Title", retrieved.Title)
	assert.Equal(t, "Updated Author", retrieved.Author)
	assert.Equal(t, "9780123456789", retrieved.ISBN)
	assert.Equal(t, "New Publisher", retrieved.Publisher)
	assert.Equal(t, "2024-01-01", retrieved.PublishDate)
	assert.Equal(t, "Updated description", retrieved.Description)
	assert.Equal(t, "en-US", retrieved.Language)
	assert.Equal(t, "Updated, Subjects", retrieved.Subjects)
	assert.Equal(t, "openlibrary", retrieved.MetadataSource)
	assert.NotNil(t, retrieved.MetadataUpdated)
}

func TestGetBookForUserWithMetadata(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create users
	user := &models.User{ID: "user-id", Username: "user", Email: "user@example.com", PasswordHash: "hash", CreatedAt: time.Now()}
	require.NoError(t, db.CreateUser(user))

	now := time.Now()
	book := &models.Book{
		ID:              "book-id",
		UserID:          user.ID,
		Title:           "User's Book",
		Author:          "Author",
		FilePath:        "/path.epub",
		UploadedAt:      now,
		ISBN:            "9780123456789",
		Publisher:       "Publisher",
		Description:     "A description",
		MetadataSource:  "epub",
		MetadataUpdated: &now,
	}
	require.NoError(t, db.CreateBook(book))

	// Get book for owner
	retrieved, err := db.GetBookForUser(book.ID, user.ID)
	require.NoError(t, err)

	assert.Equal(t, book.ID, retrieved.ID)
	assert.Equal(t, book.ISBN, retrieved.ISBN)
	assert.Equal(t, book.Publisher, retrieved.Publisher)
	assert.Equal(t, book.Description, retrieved.Description)
	assert.Equal(t, book.MetadataSource, retrieved.MetadataSource)
}

func TestBookMetadataDefaultValues(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create book without metadata fields
	book := &models.Book{
		ID:         "test-book-id",
		Title:      "Test Book",
		Author:     "Test Author",
		FilePath:   "/path/to/book.epub",
		UploadedAt: time.Now(),
	}

	err := db.CreateBook(book)
	require.NoError(t, err)

	retrieved, err := db.GetBook(book.ID)
	require.NoError(t, err)

	// Metadata fields should have empty defaults
	assert.Equal(t, "", retrieved.ISBN)
	assert.Equal(t, "", retrieved.Publisher)
	assert.Equal(t, "", retrieved.PublishDate)
	assert.Equal(t, "", retrieved.Description)
	assert.Equal(t, "", retrieved.Language)
	assert.Equal(t, "", retrieved.Subjects)
	// MetadataSource defaults to empty when not set
	assert.Equal(t, "", retrieved.MetadataSource)
}

func TestCreateAndGetAnnotation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create user and book
	user := &models.User{ID: "user-id", Username: "user", Email: "user@example.com", PasswordHash: "hash", CreatedAt: time.Now()}
	require.NoError(t, db.CreateUser(user))

	book := &models.Book{ID: "book-id", UserID: user.ID, Title: "Test Book", Author: "Author", FilePath: "/path.epub", UploadedAt: time.Now()}
	require.NoError(t, db.CreateBook(book))

	// Create annotation
	now := time.Now()
	ann := &models.Annotation{
		ID:           "ann-id",
		BookID:       book.ID,
		UserID:       user.ID,
		Chapter:      "chapter-1",
		CFI:          "/6/4[chap01]!/4/2/1:0",
		StartOffset:  100,
		EndOffset:    150,
		SelectedText: "This is the highlighted text",
		Note:         "My note about this",
		Color:        "yellow",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	err := db.CreateAnnotation(ann)
	require.NoError(t, err)

	// Get annotation
	retrieved, err := db.GetAnnotation(ann.ID)
	require.NoError(t, err)
	assert.Equal(t, ann.ID, retrieved.ID)
	assert.Equal(t, ann.BookID, retrieved.BookID)
	assert.Equal(t, ann.UserID, retrieved.UserID)
	assert.Equal(t, ann.Chapter, retrieved.Chapter)
	assert.Equal(t, ann.CFI, retrieved.CFI)
	assert.Equal(t, ann.StartOffset, retrieved.StartOffset)
	assert.Equal(t, ann.EndOffset, retrieved.EndOffset)
	assert.Equal(t, ann.SelectedText, retrieved.SelectedText)
	assert.Equal(t, ann.Note, retrieved.Note)
	assert.Equal(t, ann.Color, retrieved.Color)
}

func TestGetAnnotationsForBook(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create user and book
	user := &models.User{ID: "user-id", Username: "user", Email: "user@example.com", PasswordHash: "hash", CreatedAt: time.Now()}
	require.NoError(t, db.CreateUser(user))

	book := &models.Book{ID: "book-id", UserID: user.ID, Title: "Test Book", Author: "Author", FilePath: "/path.epub", UploadedAt: time.Now()}
	require.NoError(t, db.CreateBook(book))

	now := time.Now()

	// Create multiple annotations
	ann1 := &models.Annotation{ID: "ann-1", BookID: book.ID, UserID: user.ID, Chapter: "chapter-1", StartOffset: 100, EndOffset: 150, SelectedText: "Text 1", Color: "yellow", CreatedAt: now, UpdatedAt: now}
	ann2 := &models.Annotation{ID: "ann-2", BookID: book.ID, UserID: user.ID, Chapter: "chapter-1", StartOffset: 200, EndOffset: 250, SelectedText: "Text 2", Color: "green", CreatedAt: now, UpdatedAt: now}
	ann3 := &models.Annotation{ID: "ann-3", BookID: book.ID, UserID: user.ID, Chapter: "chapter-2", StartOffset: 50, EndOffset: 100, SelectedText: "Text 3", Color: "blue", CreatedAt: now, UpdatedAt: now}

	require.NoError(t, db.CreateAnnotation(ann1))
	require.NoError(t, db.CreateAnnotation(ann2))
	require.NoError(t, db.CreateAnnotation(ann3))

	// Get all annotations for book
	annotations, err := db.GetAnnotationsForBook(book.ID, user.ID)
	require.NoError(t, err)
	assert.Len(t, annotations, 3)

	// Should be sorted by chapter then offset
	assert.Equal(t, "ann-1", annotations[0].ID)
	assert.Equal(t, "ann-2", annotations[1].ID)
	assert.Equal(t, "ann-3", annotations[2].ID)
}

func TestGetAnnotationsForChapter(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create user and book
	user := &models.User{ID: "user-id", Username: "user", Email: "user@example.com", PasswordHash: "hash", CreatedAt: time.Now()}
	require.NoError(t, db.CreateUser(user))

	book := &models.Book{ID: "book-id", UserID: user.ID, Title: "Test Book", Author: "Author", FilePath: "/path.epub", UploadedAt: time.Now()}
	require.NoError(t, db.CreateBook(book))

	now := time.Now()

	// Create annotations in different chapters
	ann1 := &models.Annotation{ID: "ann-1", BookID: book.ID, UserID: user.ID, Chapter: "chapter-1", StartOffset: 100, EndOffset: 150, SelectedText: "Text 1", Color: "yellow", CreatedAt: now, UpdatedAt: now}
	ann2 := &models.Annotation{ID: "ann-2", BookID: book.ID, UserID: user.ID, Chapter: "chapter-1", StartOffset: 200, EndOffset: 250, SelectedText: "Text 2", Color: "green", CreatedAt: now, UpdatedAt: now}
	ann3 := &models.Annotation{ID: "ann-3", BookID: book.ID, UserID: user.ID, Chapter: "chapter-2", StartOffset: 50, EndOffset: 100, SelectedText: "Text 3", Color: "blue", CreatedAt: now, UpdatedAt: now}

	require.NoError(t, db.CreateAnnotation(ann1))
	require.NoError(t, db.CreateAnnotation(ann2))
	require.NoError(t, db.CreateAnnotation(ann3))

	// Get annotations for chapter-1 only
	annotations, err := db.GetAnnotationsForChapter(book.ID, user.ID, "chapter-1")
	require.NoError(t, err)
	assert.Len(t, annotations, 2)

	// Get annotations for chapter-2 only
	annotations, err = db.GetAnnotationsForChapter(book.ID, user.ID, "chapter-2")
	require.NoError(t, err)
	assert.Len(t, annotations, 1)
	assert.Equal(t, "ann-3", annotations[0].ID)
}

func TestUpdateAnnotation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create user and book
	user := &models.User{ID: "user-id", Username: "user", Email: "user@example.com", PasswordHash: "hash", CreatedAt: time.Now()}
	require.NoError(t, db.CreateUser(user))

	book := &models.Book{ID: "book-id", UserID: user.ID, Title: "Test Book", Author: "Author", FilePath: "/path.epub", UploadedAt: time.Now()}
	require.NoError(t, db.CreateBook(book))

	now := time.Now()
	ann := &models.Annotation{ID: "ann-id", BookID: book.ID, UserID: user.ID, Chapter: "chapter-1", StartOffset: 100, EndOffset: 150, SelectedText: "Text", Note: "Original note", Color: "yellow", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, db.CreateAnnotation(ann))

	// Update annotation
	err := db.UpdateAnnotation(ann.ID, "Updated note", "green")
	require.NoError(t, err)

	// Verify update
	retrieved, err := db.GetAnnotation(ann.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated note", retrieved.Note)
	assert.Equal(t, "green", retrieved.Color)
}

func TestDeleteAnnotation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create user and book
	user := &models.User{ID: "user-id", Username: "user", Email: "user@example.com", PasswordHash: "hash", CreatedAt: time.Now()}
	require.NoError(t, db.CreateUser(user))

	book := &models.Book{ID: "book-id", UserID: user.ID, Title: "Test Book", Author: "Author", FilePath: "/path.epub", UploadedAt: time.Now()}
	require.NoError(t, db.CreateBook(book))

	now := time.Now()
	ann := &models.Annotation{ID: "ann-id", BookID: book.ID, UserID: user.ID, Chapter: "chapter-1", StartOffset: 100, EndOffset: 150, SelectedText: "Text", Color: "yellow", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, db.CreateAnnotation(ann))

	// Delete annotation
	err := db.DeleteAnnotation(ann.ID)
	require.NoError(t, err)

	// Verify deleted
	_, err = db.GetAnnotation(ann.ID)
	assert.Error(t, err)
}

func TestAnnotationCount(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create user and book
	user := &models.User{ID: "user-id", Username: "user", Email: "user@example.com", PasswordHash: "hash", CreatedAt: time.Now()}
	require.NoError(t, db.CreateUser(user))

	book := &models.Book{ID: "book-id", UserID: user.ID, Title: "Test Book", Author: "Author", FilePath: "/path.epub", UploadedAt: time.Now()}
	require.NoError(t, db.CreateBook(book))

	now := time.Now()

	// Initially no annotations
	count, err := db.GetAnnotationCount(book.ID, user.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Add annotations
	ann1 := &models.Annotation{ID: "ann-1", BookID: book.ID, UserID: user.ID, Chapter: "chapter-1", StartOffset: 100, EndOffset: 150, SelectedText: "Text 1", Color: "yellow", CreatedAt: now, UpdatedAt: now}
	ann2 := &models.Annotation{ID: "ann-2", BookID: book.ID, UserID: user.ID, Chapter: "chapter-1", StartOffset: 200, EndOffset: 250, SelectedText: "Text 2", Color: "green", CreatedAt: now, UpdatedAt: now}

	require.NoError(t, db.CreateAnnotation(ann1))
	require.NoError(t, db.CreateAnnotation(ann2))

	count, err = db.GetAnnotationCount(book.ID, user.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestAnnotationStats(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create user and books
	user := &models.User{ID: "user-id", Username: "user", Email: "user@example.com", PasswordHash: "hash", CreatedAt: time.Now()}
	require.NoError(t, db.CreateUser(user))

	book1 := &models.Book{ID: "book-1", UserID: user.ID, Title: "Book 1", Author: "Author", FilePath: "/path1.epub", UploadedAt: time.Now()}
	book2 := &models.Book{ID: "book-2", UserID: user.ID, Title: "Book 2", Author: "Author", FilePath: "/path2.epub", UploadedAt: time.Now()}
	require.NoError(t, db.CreateBook(book1))
	require.NoError(t, db.CreateBook(book2))

	now := time.Now()

	// Add annotations to different books
	ann1 := &models.Annotation{ID: "ann-1", BookID: book1.ID, UserID: user.ID, Chapter: "ch1", StartOffset: 100, EndOffset: 150, SelectedText: "Text 1", Color: "yellow", CreatedAt: now, UpdatedAt: now}
	ann2 := &models.Annotation{ID: "ann-2", BookID: book1.ID, UserID: user.ID, Chapter: "ch2", StartOffset: 200, EndOffset: 250, SelectedText: "Text 2", Color: "green", CreatedAt: now, UpdatedAt: now}
	ann3 := &models.Annotation{ID: "ann-3", BookID: book2.ID, UserID: user.ID, Chapter: "ch1", StartOffset: 50, EndOffset: 100, SelectedText: "Text 3", Color: "blue", CreatedAt: now, UpdatedAt: now}

	require.NoError(t, db.CreateAnnotation(ann1))
	require.NoError(t, db.CreateAnnotation(ann2))
	require.NoError(t, db.CreateAnnotation(ann3))

	// Get stats
	totalAnnotations, booksWithAnnotations, err := db.GetAnnotationStats(user.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, totalAnnotations)
	assert.Equal(t, 2, booksWithAnnotations)
}

func TestAnnotationsPerUser(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create two users
	user1 := &models.User{ID: "user-1", Username: "user1", Email: "user1@example.com", PasswordHash: "hash", CreatedAt: time.Now()}
	user2 := &models.User{ID: "user-2", Username: "user2", Email: "user2@example.com", PasswordHash: "hash", CreatedAt: time.Now()}
	require.NoError(t, db.CreateUser(user1))
	require.NoError(t, db.CreateUser(user2))

	// Create a shared book
	book := &models.Book{ID: "book-id", UserID: user1.ID, Title: "Shared Book", Author: "Author", FilePath: "/path.epub", UploadedAt: time.Now()}
	require.NoError(t, db.CreateBook(book))

	now := time.Now()

	// Each user creates their own annotations on the same book
	ann1 := &models.Annotation{ID: "ann-1", BookID: book.ID, UserID: user1.ID, Chapter: "ch1", StartOffset: 100, EndOffset: 150, SelectedText: "User 1 highlight", Color: "yellow", CreatedAt: now, UpdatedAt: now}
	ann2 := &models.Annotation{ID: "ann-2", BookID: book.ID, UserID: user2.ID, Chapter: "ch1", StartOffset: 100, EndOffset: 150, SelectedText: "User 2 highlight", Color: "green", CreatedAt: now, UpdatedAt: now}

	require.NoError(t, db.CreateAnnotation(ann1))
	require.NoError(t, db.CreateAnnotation(ann2))

	// User 1 only sees their annotations
	annotations1, err := db.GetAnnotationsForBook(book.ID, user1.ID)
	require.NoError(t, err)
	assert.Len(t, annotations1, 1)
	assert.Equal(t, "User 1 highlight", annotations1[0].SelectedText)

	// User 2 only sees their annotations
	annotations2, err := db.GetAnnotationsForBook(book.ID, user2.ID)
	require.NoError(t, err)
	assert.Len(t, annotations2, 1)
	assert.Equal(t, "User 2 highlight", annotations2[0].SelectedText)
}
