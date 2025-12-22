package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justyntemme/webby/internal/models"
	"github.com/justyntemme/webby/internal/storage"
)

// setupTestHandler creates a test handler with a temporary database
func setupTestHandler(t *testing.T) (*Handler, func()) {
	// Create temp dir
	tmpDir, err := os.MkdirTemp("", "webby-test-*")
	require.NoError(t, err)

	// Create database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.NewDatabase(dbPath)
	require.NoError(t, err)

	// Create file storage
	files, err := storage.NewFileStorage(tmpDir)
	require.NoError(t, err)

	// Create handler
	handler := NewHandler(db, files)

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return handler, cleanup
}

// setupTestUser creates a test user and returns the user ID
func setupTestUser(t *testing.T, handler *Handler) string {
	user := &models.User{
		ID:           uuid.New().String(),
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hashedpassword",
		CreatedAt:    time.Now(),
	}
	err := handler.db.CreateUser(user)
	require.NoError(t, err)
	return user.ID
}

// setupTestBook creates a test book for a user and returns the book ID
func setupTestBook(t *testing.T, handler *Handler, userID string) string {
	book := &models.Book{
		ID:          uuid.New().String(),
		UserID:      userID,
		Title:       "Test Book",
		Author:      "Test Author",
		FilePath:    "/tmp/test.epub",
		FileSize:    1024,
		UploadedAt:  time.Now(),
		ContentType: models.ContentTypeBook,
		FileFormat:  models.FileFormatEPUB,
		ReadStatus:  models.ReadStatusUnread,
	}
	err := handler.db.CreateBook(book)
	require.NoError(t, err)
	return book.ID
}

// createAuthenticatedContext creates a gin context with auth middleware
func createAuthenticatedContext(userID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", userID)
	return c, w
}

func TestListAnnotationsForBook(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	userID := setupTestUser(t, handler)
	bookID := setupTestBook(t, handler, userID)

	// Create test annotations
	now := time.Now()
	annotations := []*models.Annotation{
		{
			ID:           uuid.New().String(),
			BookID:       bookID,
			UserID:       userID,
			Chapter:      "chapter1",
			CFI:          "/6/4[chap01ref]!/4/2/2/1:0",
			StartOffset:  0,
			EndOffset:    50,
			SelectedText: "First highlight",
			Note:         "Important note",
			Color:        models.HighlightColorYellow,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           uuid.New().String(),
			BookID:       bookID,
			UserID:       userID,
			Chapter:      "chapter2",
			CFI:          "/6/6[chap02ref]!/4/2/2/1:0",
			StartOffset:  10,
			EndOffset:    30,
			SelectedText: "Second highlight",
			Color:        models.HighlightColorGreen,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}

	for _, ann := range annotations {
		err := handler.db.CreateAnnotation(ann)
		require.NoError(t, err)
	}

	// Test listing annotations
	c, w := createAuthenticatedContext(userID)
	c.Params = []gin.Param{{Key: "id", Value: bookID}}
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/books/"+bookID+"/annotations", nil)

	handler.ListAnnotationsForBook(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Annotations []*models.Annotation `json:"annotations"`
		Count       int                  `json:"count"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 2, response.Count)
	assert.Len(t, response.Annotations, 2)
}

func TestListAnnotationsForChapter(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	userID := setupTestUser(t, handler)
	bookID := setupTestBook(t, handler, userID)

	// Create annotations in different chapters
	now := time.Now()
	ann1 := &models.Annotation{
		ID:           uuid.New().String(),
		BookID:       bookID,
		UserID:       userID,
		Chapter:      "chapter1",
		SelectedText: "In chapter 1",
		Color:        models.HighlightColorYellow,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	ann2 := &models.Annotation{
		ID:           uuid.New().String(),
		BookID:       bookID,
		UserID:       userID,
		Chapter:      "chapter2",
		SelectedText: "In chapter 2",
		Color:        models.HighlightColorBlue,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	require.NoError(t, handler.db.CreateAnnotation(ann1))
	require.NoError(t, handler.db.CreateAnnotation(ann2))

	// Request chapter1 only
	c, w := createAuthenticatedContext(userID)
	c.Params = []gin.Param{
		{Key: "id", Value: bookID},
		{Key: "chapter", Value: "chapter1"},
	}
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/books/"+bookID+"/annotations/chapter/chapter1", nil)

	handler.ListAnnotationsForChapter(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Annotations []*models.Annotation `json:"annotations"`
		Count       int                  `json:"count"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 1, response.Count)
	assert.Equal(t, "In chapter 1", response.Annotations[0].SelectedText)
}

func TestCreateAnnotation(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	userID := setupTestUser(t, handler)
	bookID := setupTestBook(t, handler, userID)

	// Create annotation request
	reqBody := map[string]interface{}{
		"chapter":       "chapter3",
		"cfi":           "/6/8[chap03ref]!/4/2/2/1:0",
		"start_offset":  100,
		"end_offset":    150,
		"selected_text": "This is highlighted text",
		"note":          "My personal note",
		"color":         "blue",
	}
	body, _ := json.Marshal(reqBody)

	c, w := createAuthenticatedContext(userID)
	c.Params = []gin.Param{{Key: "id", Value: bookID}}
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/books/"+bookID+"/annotations", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.CreateAnnotation(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response struct {
		Message    string            `json:"message"`
		Annotation *models.Annotation `json:"annotation"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Annotation created", response.Message)
	assert.Equal(t, "chapter3", response.Annotation.Chapter)
	assert.Equal(t, "This is highlighted text", response.Annotation.SelectedText)
	assert.Equal(t, "My personal note", response.Annotation.Note)
	assert.Equal(t, models.HighlightColorBlue, response.Annotation.Color)
}

func TestCreateAnnotation_DefaultColor(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	userID := setupTestUser(t, handler)
	bookID := setupTestBook(t, handler, userID)

	// Create annotation without specifying color
	reqBody := map[string]interface{}{
		"chapter":       "chapter1",
		"selected_text": "Highlighted without color",
	}
	body, _ := json.Marshal(reqBody)

	c, w := createAuthenticatedContext(userID)
	c.Params = []gin.Param{{Key: "id", Value: bookID}}
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/books/"+bookID+"/annotations", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.CreateAnnotation(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response struct {
		Annotation *models.Annotation `json:"annotation"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	// Should default to yellow
	assert.Equal(t, models.HighlightColorYellow, response.Annotation.Color)
}

func TestCreateAnnotation_InvalidColor(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	userID := setupTestUser(t, handler)
	bookID := setupTestBook(t, handler, userID)

	// Try invalid color
	reqBody := map[string]interface{}{
		"chapter":       "chapter1",
		"selected_text": "Test",
		"color":         "purple", // Invalid
	}
	body, _ := json.Marshal(reqBody)

	c, w := createAuthenticatedContext(userID)
	c.Params = []gin.Param{{Key: "id", Value: bookID}}
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/books/"+bookID+"/annotations", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.CreateAnnotation(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetAnnotation(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	userID := setupTestUser(t, handler)
	bookID := setupTestBook(t, handler, userID)

	// Create annotation
	now := time.Now()
	ann := &models.Annotation{
		ID:           uuid.New().String(),
		BookID:       bookID,
		UserID:       userID,
		Chapter:      "chapter1",
		SelectedText: "Test text",
		Note:         "Test note",
		Color:        models.HighlightColorPink,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, handler.db.CreateAnnotation(ann))

	// Get annotation
	c, w := createAuthenticatedContext(userID)
	c.Params = []gin.Param{
		{Key: "id", Value: bookID},
		{Key: "annotationId", Value: ann.ID},
	}
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/books/"+bookID+"/annotations/"+ann.ID, nil)

	handler.GetAnnotation(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.Annotation
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, ann.ID, response.ID)
	assert.Equal(t, "Test text", response.SelectedText)
}

func TestGetAnnotation_NotFound(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	userID := setupTestUser(t, handler)

	c, w := createAuthenticatedContext(userID)
	c.Params = []gin.Param{
		{Key: "id", Value: "nonexistent-book"},
		{Key: "annotationId", Value: "nonexistent-annotation"},
	}
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/books/x/annotations/y", nil)

	handler.GetAnnotation(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateAnnotation(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	userID := setupTestUser(t, handler)
	bookID := setupTestBook(t, handler, userID)

	// Create annotation
	now := time.Now()
	ann := &models.Annotation{
		ID:           uuid.New().String(),
		BookID:       bookID,
		UserID:       userID,
		Chapter:      "chapter1",
		SelectedText: "Original text",
		Note:         "Original note",
		Color:        models.HighlightColorYellow,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, handler.db.CreateAnnotation(ann))

	// Update annotation
	reqBody := map[string]interface{}{
		"note":  "Updated note",
		"color": "green",
	}
	body, _ := json.Marshal(reqBody)

	c, w := createAuthenticatedContext(userID)
	c.Params = []gin.Param{
		{Key: "id", Value: bookID},
		{Key: "annotationId", Value: ann.ID},
	}
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/books/"+bookID+"/annotations/"+ann.ID, bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAnnotation(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Message    string            `json:"message"`
		Annotation *models.Annotation `json:"annotation"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Annotation updated", response.Message)
	assert.Equal(t, "Updated note", response.Annotation.Note)
	assert.Equal(t, models.HighlightColorGreen, response.Annotation.Color)
}

func TestDeleteAnnotation(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	userID := setupTestUser(t, handler)
	bookID := setupTestBook(t, handler, userID)

	// Create annotation
	now := time.Now()
	ann := &models.Annotation{
		ID:           uuid.New().String(),
		BookID:       bookID,
		UserID:       userID,
		Chapter:      "chapter1",
		SelectedText: "To be deleted",
		Color:        models.HighlightColorOrange,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, handler.db.CreateAnnotation(ann))

	// Delete annotation
	c, w := createAuthenticatedContext(userID)
	c.Params = []gin.Param{
		{Key: "id", Value: bookID},
		{Key: "annotationId", Value: ann.ID},
	}
	c.Request, _ = http.NewRequest(http.MethodDelete, "/api/books/"+bookID+"/annotations/"+ann.ID, nil)

	handler.DeleteAnnotation(c)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify it's deleted
	_, err := handler.db.GetAnnotation(ann.ID)
	assert.Error(t, err)
}

func TestListAllAnnotations(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	userID := setupTestUser(t, handler)
	bookID1 := setupTestBook(t, handler, userID)
	bookID2 := setupTestBook(t, handler, userID)

	// Create annotations across different books
	now := time.Now()
	annotations := []*models.Annotation{
		{
			ID:           uuid.New().String(),
			BookID:       bookID1,
			UserID:       userID,
			Chapter:      "ch1",
			SelectedText: "Book 1 highlight",
			Color:        models.HighlightColorYellow,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           uuid.New().String(),
			BookID:       bookID2,
			UserID:       userID,
			Chapter:      "ch1",
			SelectedText: "Book 2 highlight",
			Color:        models.HighlightColorBlue,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}

	for _, ann := range annotations {
		require.NoError(t, handler.db.CreateAnnotation(ann))
	}

	c, w := createAuthenticatedContext(userID)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/annotations", nil)

	handler.ListAllAnnotations(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Annotations []*models.Annotation `json:"annotations"`
		Count       int                  `json:"count"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 2, response.Count)
}

func TestGetAnnotationStats(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	userID := setupTestUser(t, handler)
	bookID1 := setupTestBook(t, handler, userID)
	bookID2 := setupTestBook(t, handler, userID)

	// Create annotations across different books
	now := time.Now()
	annotations := []*models.Annotation{
		{
			ID:           uuid.New().String(),
			BookID:       bookID1,
			UserID:       userID,
			Chapter:      "ch1",
			SelectedText: "Highlight 1",
			Color:        models.HighlightColorYellow,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           uuid.New().String(),
			BookID:       bookID1,
			UserID:       userID,
			Chapter:      "ch2",
			SelectedText: "Highlight 2",
			Color:        models.HighlightColorGreen,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           uuid.New().String(),
			BookID:       bookID2,
			UserID:       userID,
			Chapter:      "ch1",
			SelectedText: "Highlight 3",
			Color:        models.HighlightColorBlue,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}

	for _, ann := range annotations {
		require.NoError(t, handler.db.CreateAnnotation(ann))
	}

	c, w := createAuthenticatedContext(userID)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/annotations/stats", nil)

	handler.GetAnnotationStats(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		TotalAnnotations     int `json:"total_annotations"`
		BooksWithAnnotations int `json:"books_with_annotations"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 3, response.TotalAnnotations)
	assert.Equal(t, 2, response.BooksWithAnnotations)
}

func TestAnnotationAccessControl(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	// Create two users
	user1ID := setupTestUser(t, handler)

	user2 := &models.User{
		ID:           uuid.New().String(),
		Username:     "user2",
		Email:        "user2@example.com",
		PasswordHash: "hashedpassword",
		CreatedAt:    time.Now(),
	}
	require.NoError(t, handler.db.CreateUser(user2))
	user2ID := user2.ID

	// User1 creates a book
	bookID := setupTestBook(t, handler, user1ID)

	// User1 creates an annotation
	now := time.Now()
	ann := &models.Annotation{
		ID:           uuid.New().String(),
		BookID:       bookID,
		UserID:       user1ID,
		Chapter:      "ch1",
		SelectedText: "User1's private annotation",
		Color:        models.HighlightColorYellow,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, handler.db.CreateAnnotation(ann))

	// User2 tries to access User1's annotation - should be forbidden
	c, w := createAuthenticatedContext(user2ID)
	c.Params = []gin.Param{
		{Key: "id", Value: bookID},
		{Key: "annotationId", Value: ann.ID},
	}
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/books/"+bookID+"/annotations/"+ann.ID, nil)

	handler.GetAnnotation(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAnnotation_Unauthenticated(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	// Create context without authentication
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// Don't set user_id - simulates unauthenticated request
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/annotations", nil)

	handler.ListAllAnnotations(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAllHighlightColors(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	userID := setupTestUser(t, handler)
	bookID := setupTestBook(t, handler, userID)

	colors := []string{
		models.HighlightColorYellow,
		models.HighlightColorGreen,
		models.HighlightColorBlue,
		models.HighlightColorPink,
		models.HighlightColorOrange,
	}

	for _, color := range colors {
		t.Run("color_"+color, func(t *testing.T) {
			reqBody := map[string]interface{}{
				"chapter":       "chapter1",
				"selected_text": "Test for " + color,
				"color":         color,
			}
			body, _ := json.Marshal(reqBody)

			c, w := createAuthenticatedContext(userID)
			c.Params = []gin.Param{{Key: "id", Value: bookID}}
			c.Request, _ = http.NewRequest(http.MethodPost, "/api/books/"+bookID+"/annotations", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")

			handler.CreateAnnotation(c)

			assert.Equal(t, http.StatusCreated, w.Code)

			var response struct {
				Annotation *models.Annotation `json:"annotation"`
			}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			assert.Equal(t, color, response.Annotation.Color)
		})
	}
}
