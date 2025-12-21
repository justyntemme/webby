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
