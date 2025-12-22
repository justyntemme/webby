package api_test

import (
	"bytes"
	"context" // Added for context.Context type
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/justyntemme/webby/internal/api"
	"github.com/justyntemme/webby/internal/metadata"
	"github.com/justyntemme/webby/internal/models"
	"github.com/justyntemme/webby/internal/storage"
)

// MockDatabase is a mock implementation of the storage.Database interface
type MockDatabase struct {
	mock.Mock
}

func (m *MockDatabase) CreateBook(book *models.Book) error {
	args := m.Called(book)
	return args.Error(0)
}

func (m *MockDatabase) GetBook(id string) (*models.Book, error) {
	args := m.Called(id)
	return args.Get(0).(*models.Book), args.Error(1)
}

// Implement other methods of storage.Database as needed for tests
func (m *MockDatabase) ListBooksForUser(userID string) ([]models.Book, error) {
	args := m.Called(userID)
	return args.Get(0).([]models.Book), args.Error(1)
}
func (m *MockDatabase) SearchBooksForUser(query, userID string) ([]models.Book, error) {
	args := m.Called(query, userID)
	return args.Get(0).([]models.Book), args.Error(1)
}
func (m *MockDatabase) ListBooksForUserWithFilters(userID, sortBy, order, contentType, readStatus string) ([]models.Book, error) {
	args := m.Called(userID, sortBy, order, contentType, readStatus)
	return args.Get(0).([]models.Book), args.Error(1)
}
func (m *MockDatabase) GetBookForUser(id, userID string) (*models.Book, error) {
	args := m.Called(id, userID)
	return args.Get(0).(*models.Book), args.Error(1)
}
func (m *MockDatabase) DeleteBook(id string) error {
	args := m.Called(id)
	return args.Error(0)
}
func (m *MockDatabase) GetBooksByAuthorForUser(userID string) (map[string][]models.Book, error) {
	args := m.Called(userID)
	return args.Get(0).(map[string][]models.Book), args.Error(1)
}
func (m *MockDatabase) GetBooksBySeriesForUser(userID string) (map[string][]models.Book, error) {
	args := m.Called(userID)
	return args.Get(0).(map[string][]models.Book), args.Error(1)
}
func (m *MockDatabase) GetReadingPosition(bookID, userID string) (*models.ReadingPosition, error) {
	args := m.Called(bookID, userID)
	return args.Get(0).(*models.ReadingPosition), args.Error(1)
}
func (m *MockDatabase) SaveReadingPosition(pos *models.ReadingPosition) error {
	args := m.Called(pos)
	return args.Error(0)
}
func (m *MockDatabase) UpdateBookReadStatus(bookID, status string, completedAt *time.Time) error {
	args := m.Called(bookID, status, completedAt)
	return args.Error(0)
}
func (m *MockDatabase) CreateCollection(collection *models.Collection) error {
	args := m.Called(collection)
	return args.Error(0)
}
func (m *MockDatabase) ListCollections() ([]models.Collection, error) {
	args := m.Called()
	return args.Get(0).([]models.Collection), args.Error(1)
}
func (m *MockDatabase) GetCollection(id string) (*models.Collection, error) {
	args := m.Called(id)
	return args.Get(0).(*models.Collection), args.Error(1)
}
func (m *MockDatabase) UpdateCollection(id, name string) error {
	args := m.Called(id, name)
	return args.Error(0)
}
func (m *MockDatabase) DeleteCollection(id string) error {
	args := m.Called(id)
	return args.Error(0)
}
func (m *MockDatabase) AddBookToCollection(bookID, collectionID string) error {
	args := m.Called(bookID, collectionID)
	return args.Error(0)
}
func (m *MockDatabase) RemoveBookFromCollection(bookID, collectionID string) error {
	args := m.Called(bookID, collectionID)
	return args.Error(0)
}
func (m *MockDatabase) BulkAddBooksToCollection(bookIDs []string, collectionID string) error {
	args := m.Called(bookIDs, collectionID)
	return args.Error(0)
}
func (m *MockDatabase) GetBooksInCollection(collectionID string) ([]models.Book, error) {
	args := m.Called(collectionID)
	return args.Get(0).([]models.Book), args.Error(1)
}
func (m *MockDatabase) GetCollectionsForBook(bookID string) ([]models.Collection, error) {
	args := m.Called(bookID)
	return args.Get(0).([]models.Collection), args.Error(1)
}
func (m *MockDatabase) UpdateBookMetadata(book *models.Book) error {
	args := m.Called(book)
	return args.Error(0)
}
func (m *MockDatabase) UpdateBookFilePaths(bookID, filePath, coverPath string) error {
	args := m.Called(bookID, filePath, coverPath)
	return args.Error(0)
}
func (m *MockDatabase) GetUserByID(userID string) (*models.User, error) {
	args := m.Called(userID)
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *MockDatabase) ShareBook(bookID, ownerID, targetUserID string) error {
	args := m.Called(bookID, ownerID, targetUserID)
	return args.Error(0)
}
func (m *MockDatabase) UnshareBook(bookID, targetUserID string) error {
	args := m.Called(bookID, targetUserID)
	return args.Error(0)
}
func (m *MockDatabase) GetSharedBooks(userID string) ([]models.Book, error) {
	args := m.Called(userID)
	return args.Get(0).([]models.Book), args.Error(1)
}
func (m *MockDatabase) GetBookShares(bookID string) ([]models.User, error) {
	args := m.Called(bookID)
	return args.Get(0).([]models.User), args.Error(1)
}
func (m *MockDatabase) ListBooksForUserWithFilter(userID, sortBy, order, contentType string) ([]models.Book, error) {
	args := m.Called(userID, sortBy, order, contentType)
	return args.Get(0).([]models.Book), args.Error(1)
}
func (m *MockDatabase) GetBookReadStatus(bookID, userID string) (string, *time.Time, error) {
	args := m.Called(bookID, userID)
	return args.String(0), args.Get(1).(*time.Time), args.Error(2)
}
func (m *MockDatabase) UpdateBookRating(bookID, userID string, rating int) error {
	args := m.Called(bookID, userID, rating)
	return args.Error(0)
}
func (m *MockDatabase) CreateTag(tag *models.Tag) error {
	args := m.Called(tag)
	return args.Error(0)
}
func (m *MockDatabase) ListTagsForUser(userID string) ([]models.Tag, error) {
	args := m.Called(userID)
	return args.Get(0).([]models.Tag), args.Error(1)
}
func (m *MockDatabase) GetTagForUser(id, userID string) (*models.Tag, error) {
	args := m.Called(id, userID)
	return args.Get(0).(*models.Tag), args.Error(1)
}
func (m *MockDatabase) GetTagByNameForUser(name, userID string) (*models.Tag, error) {
	args := m.Called(name, userID)
	return args.Get(0).(*models.Tag), args.Error(1)
}
func (m *MockDatabase) UpdateTag(tag *models.Tag) error {
	args := m.Called(tag)
	return args.Error(0)
}
func (m *MockDatabase) DeleteTag(id string) error {
	args := m.Called(id)
	return args.Error(0)
}
func (m *MockDatabase) GetBooksWithTag(tagID, userID string) ([]models.Book, error) {
	args := m.Called(tagID, userID)
	return args.Get(0).([]models.Book), args.Error(1)
}
func (m *MockDatabase) GetTagsForBook(bookID, userID string) ([]models.Tag, error) {
	args := m.Called(bookID, userID)
	return args.Get(0).([]models.Tag), args.Error(1)
}
func (m *MockDatabase) AddTagToBook(bookID, tagID string) error {
	args := m.Called(bookID, tagID)
	return args.Error(0)
}
func (m *MockDatabase) RemoveTagFromBook(bookID, tagID string) error {
	args := m.Called(bookID, tagID)
	return args.Error(0)
}
func (m *MockDatabase) GetAnnotation(id string) (*models.Annotation, error) {
	args := m.Called(id)
	return args.Get(0).(*models.Annotation), args.Error(1)
}
func (m *MockDatabase) ListAnnotationsForUser(userID string) ([]models.Annotation, error) {
	args := m.Called(userID)
	return args.Get(0).([]models.Annotation), args.Error(1)
}
func (m *MockDatabase) GetAnnotationStatsForUser(userID string) (int, int, error) {
	args := m.Called(userID)
	return args.Int(0), args.Int(1), args.Error(2)
}
func (m *MockDatabase) ListAnnotationsForBook(bookID, userID string) ([]models.Annotation, error) {
	args := m.Called(bookID, userID)
	return args.Get(0).([]models.Annotation), args.Error(1)
}
func (m *MockDatabase) ListAnnotationsForChapter(bookID, chapter, userID string) ([]models.Annotation, error) {
	args := m.Called(bookID, chapter, userID)
	return args.Get(0).([]models.Annotation), args.Error(1)
}
func (m *MockDatabase) CreateAnnotation(annotation *models.Annotation) error {
	args := m.Called(annotation)
	return args.Error(0)
}
func (m *MockDatabase) UpdateAnnotation(annotation *models.Annotation) error {
	args := m.Called(annotation)
	return args.Error(0)
}
func (m *MockDatabase) DeleteAnnotation(id string) error {
	args := m.Called(id)
	return args.Error(0)
}
func (m *MockDatabase) CreateReadingList(list *models.ReadingList) error {
	args := m.Called(list)
	return args.Error(0)
}
func (m *MockDatabase) ListReadingListsForUser(userID string) ([]models.ReadingList, error) {
	args := m.Called(userID)
	return args.Get(0).([]models.ReadingList), args.Error(1)
}
func (m *MockDatabase) GetReadingListForUser(listID, userID string) (*models.ReadingList, error) {
	args := m.Called(listID, userID)
	return args.Get(0).(*models.ReadingList), args.Error(1)
}
func (m *MockDatabase) UpdateReadingList(list *models.ReadingList) error {
	args := m.Called(list)
	return args.Error(0)
}
func (m *MockDatabase) DeleteReadingList(listID, userID string) error {
	args := m.Called(listID, userID)
	return args.Error(0)
}
func (m *MockDatabase) AddBookToReadingList(bookID, listID string) error {
	args := m.Called(bookID, listID)
	return args.Error(0)
}
func (m *MockDatabase) RemoveBookFromReadingList(bookID, listID string) error {
	args := m.Called(bookID, listID)
	return args.Error(0)
}
func (m *MockDatabase) GetBooksInReadingList(listID string) ([]models.Book, error) {
	args := m.Called(listID)
	return args.Get(0).([]models.Book), args.Error(1)
}
func (m *MockDatabase) GetReadingListsForBook(bookID, userID string) ([]models.ReadingList, error) {
	args := m.Called(bookID, userID)
	return args.Get(0).([]models.ReadingList), args.Error(1)
}
func (m *MockDatabase) UpdateReadingListOrder(listID string, bookIDs []string) error {
	args := m.Called(listID, bookIDs)
	return args.Error(0)
}
func (m *MockDatabase) GetOrCreateSystemReadingList(userID, listType string) (*models.ReadingList, error) {
	args := m.Called(userID, listType)
	return args.Get(0).(*models.ReadingList), args.Error(1)
}

// MockFileStorage is a mock implementation of the storage.FileStorage interface
type MockFileStorage struct {
	mock.Mock
	TempDir string
}

func (m *MockFileStorage) SaveBookWithExt(bookID string, file io.Reader, ext string) (string, error) {
	args := m.Called(bookID, file, ext)
	// Simulate saving a file to a temporary location
	tempPath := filepath.Join(m.TempDir, bookID+ext)
	data, _ := io.ReadAll(file)
	os.WriteFile(tempPath, data, 0644)
	return tempPath, args.Error(0)
}

func (m *MockFileStorage) DeleteBook(bookID string) error {
	args := m.Called(bookID)
	return args.Error(0)
}

func (m *MockFileStorage) SaveCover(bookID string, imageData []byte, ext string) (string, error) {
	args := m.Called(bookID, imageData, ext)
	return args.String(0), args.Error(1)
}
func (m *MockFileStorage) DeleteCover(bookID string) error {
	args := m.Called(bookID)
	return args.Error(0)
}
func (m *MockFileStorage) BookExists(bookID string) (bool, error) {
	args := m.Called(bookID)
	return args.Bool(0), args.Error(1)
}
func (m *MockFileStorage) ReorganizeBook(bookFilePath, coverFilePath, author, series, title string) (*storage.BookPaths, error) {
	args := m.Called(bookFilePath, coverFilePath, author, series, title)
	return args.Get(0).(*storage.BookPaths), args.Error(1)
}

// MockMetadataService is a mock for metadata.Service
type MockMetadataService struct {
	mock.Mock
}

func (m *MockMetadataService) LookupBook(ctx context.Context, isbn, title, author string) (*metadata.LookupResult, error) {
	args := m.Called(ctx, isbn, title, author)
	return args.Get(0).(*metadata.LookupResult), args.Error(1)
}
func (m *MockMetadataService) SearchBooks(ctx context.Context, isbn, title, author string) ([]metadata.LookupResult, error) {
	args := m.Called(ctx, isbn, title, author)
	return args.Get(0).([]metadata.LookupResult), args.Error(1)
}

// MockComicMetadataService is a mock for metadata.ComicService
type MockComicMetadataService struct {
	mock.Mock
}

func (m *MockComicMetadataService) IsConfigured() bool {
	args := m.Called()
	return args.Bool(0)
}
func (m *MockComicMetadataService) LookupComic(ctx context.Context, series, issue, title string, year int) (*metadata.ComicLookupResult, error) {
	args := m.Called(ctx, series, issue, title, year)
	return args.Get(0).(*metadata.ComicLookupResult), args.Error(1)
}
func (m *MockComicMetadataService) SearchComics(ctx context.Context, series, issue, title string) ([]metadata.ComicLookupResult, error) {
	args := m.Called(ctx, series, issue, title)
	return args.Get(0).([]metadata.ComicLookupResult), args.Error(1)
}

// MockDuplicateService is a mock for storage.DuplicateService
type MockDuplicateService struct {
	mock.Mock
}

func (m *MockDuplicateService) FindDuplicates(userID string) ([]storage.DuplicateGroup, error) {
	args := m.Called(userID)
	return args.Get(0).([]storage.DuplicateGroup), args.Error(1)
}
func (m *MockDuplicateService) ComputeMissingHashes(userID string) (int, int, error) {
	args := m.Called(userID)
	return args.Int(0), args.Int(1), args.Error(2)
}

// Helper to create a dummy CBZ file
func createDummyCBZ(t *testing.T, filename string) string {
	zipfile, err := os.Create(filename)
	assert.NoError(t, err)
	defer zipfile.Close()

	w := multipart.NewWriter(zipfile)
	defer w.Close()

	// Add a dummy image file
	imgWriter, err := w.CreateFormFile("image", "001.jpg")
	assert.NoError(t, err)
	_, err = imgWriter.Write([]byte("dummy image data"))
	assert.NoError(t, err)

	return zipfile.Name()
}

func TestUploadBook_CBZ_FilenameParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup mocks
	mockDB := new(MockDatabase)
	mockFS := &MockFileStorage{TempDir: t.TempDir()} // Use testify's TempDir
	mockMetadata := new(MockMetadataService)
	mockComicMetadata := new(MockComicMetadataService)
	mockDuplicates := new(MockDuplicateService)

	h := &api.Handler{
		DB:            mockDB,
		Files:         mockFS,
		Metadata:      mockMetadata,
		ComicMetadata: mockComicMetadata,
		Duplicates:    mockDuplicates,
	}

	// Create a dummy CBZ file with a descriptive name
	originalCBZFilename := "The Amazing Spider-Man #700 (2012) (Digital).cbz"
	dummyCBZPath := createDummyCBZ(t, filepath.Join(t.TempDir(), originalCBZFilename))
	defer os.Remove(dummyCBZPath)

	// Simulate file upload
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", originalCBZFilename)
	assert.NoError(t, err)

	file, err := os.Open(dummyCBZPath)
	assert.NoError(t, err)
	defer file.Close()

	_, err = io.Copy(part, file)
	assert.NoError(t, err)
	writer.Close()

	// Mock file storage saving and deletion
	mockFS.On("SaveBookWithExt", mock.AnythingOfType("string"), mock.Anything, ".cbz").Return(
		func(bookID string, file io.Reader, ext string) string {
			// Simulate saving the file, but don't actually process CBZ contents here.
			// The actual CBZ parsing is done by the cbz package which will use the path.
			// We just need a valid path for the mock.
			return filepath.Join(mockFS.TempDir, bookID+ext)
		},
		nil,
	)
	mockFS.On("DeleteBook", mock.AnythingOfType("string")).Return(nil)
	mockFS.On("SaveCover", mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("string")).Return("path/to/cover.jpg", nil)

	// Mock database operations
	mockDB.On("CreateBook", mock.AnythingOfType("*models.Book")).Return(nil).Run(func(args mock.Arguments) {
		// Assert that the Title is correctly parsed from the original filename
		book := args.Get(0).(*models.Book)
		assert.Equal(t, "The Amazing Spider-Man #700 (2012)", book.Title, "Book title should be parsed from original filename")
		assert.Equal(t, "The Amazing Spider-Man", book.Series, "Book series should be parsed from original filename")
		assert.Equal(t, 700.0, book.SeriesIndex, "Book series index should be parsed from original filename")
	})

	// Create a Gin context and recorder
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/books", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	// Add dummy UserID to context (required by handler, but not central to this test)
	c.Set("userID", "test-user-id")

	// Call the handler
	h.UploadBook(c)

	// Assert HTTP response
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), "Book uploaded successfully")

	// Assert that mocks were called as expected
	mockDB.AssertExpectations(t)
	mockFS.AssertExpectations(t)
}

func TestUploadBook_CBR_FilenameParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup mocks
	mockDB := new(MockDatabase)
	mockFS := &MockFileStorage{TempDir: t.TempDir()} // Use testify's TempDir
	mockMetadata := new(MockMetadataService)
	mockComicMetadata := new(MockComicMetadataService)
	mockDuplicates := new(MockDuplicateService)

	h := &api.Handler{
		DB:            mockDB,
		Files:         mockFS,
		Metadata:      mockMetadata,
		ComicMetadata: mockComicMetadata,
		Duplicates:    mockDuplicates,
	}

	// Create a dummy CBR file with a descriptive name (RAR is complex, simulate with ZIP for test)
	// For actual CBR, rardecode would be used. Here, we just need a file to exist.
	originalCBRFilename := "X-Men - Red #1 (2022) (Digital).cbr"
	dummyCBRPath := createDummyCBZ(t, filepath.Join(t.TempDir(), originalCBRFilename)) // Reuse CBZ helper, content doesn't matter for filename parsing
	defer os.Remove(dummyCBRPath)

	// Simulate file upload
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", originalCBRFilename)
	assert.NoError(t, err)

	file, err := os.Open(dummyCBRPath)
	assert.NoError(t, err)
	defer file.Close()

	_, err = io.Copy(part, file)
	assert.NoError(t, err)
	writer.Close()

	// Mock file storage saving and deletion
	mockFS.On("SaveBookWithExt", mock.AnythingOfType("string"), mock.Anything, ".cbr").Return(
		func(bookID string, file io.Reader, ext string) string {
			return filepath.Join(mockFS.TempDir, bookID+ext)
		},
		nil,
	)
	mockFS.On("DeleteBook", mock.AnythingOfType("string")).Return(nil)
	mockFS.On("SaveCover", mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("string")).Return("path/to/cover.jpg", nil)

	// Mock database operations
	mockDB.On("CreateBook", mock.AnythingOfType("*models.Book")).Return(nil).Run(func(args mock.Arguments) {
		// Assert that the Title is correctly parsed from the original filename
		book := args.Get(0).(*models.Book)
		assert.Equal(t, "X-Men - Red #1 (2022)", book.Title, "Book title should be parsed from original filename")
		assert.Equal(t, "X-Men - Red", book.Series, "Book series should be parsed from original filename")
		assert.Equal(t, 1.0, book.SeriesIndex, "Book series index should be parsed from original filename")
	})

	// Create a Gin context and recorder
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/books", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	// Add dummy UserID to context
	c.Set("userID", "test-user-id")

	// Call the handler
	h.UploadBook(c)

	// Assert HTTP response
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), "Book uploaded successfully")

	// Assert that mocks were called as expected
	mockDB.AssertExpectations(t)
	mockFS.AssertExpectations(t)
}
