package api

import (
	"database/sql"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/justyntemme/webby/internal/auth"
	"github.com/justyntemme/webby/internal/epub"
	"github.com/justyntemme/webby/internal/models"
	"github.com/justyntemme/webby/internal/storage"
)

// Handler contains all HTTP handlers
type Handler struct {
	db    *storage.Database
	files *storage.FileStorage
}

// NewHandler creates a new handler instance
func NewHandler(db *storage.Database, files *storage.FileStorage) *Handler {
	return &Handler{db: db, files: files}
}

// UploadBook handles EPUB file uploads
func (h *Handler) UploadBook(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}
	defer file.Close()

	// Check file size (max 100MB)
	if header.Size > 100*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large (max 100MB)"})
		return
	}

	// Generate unique ID
	bookID := uuid.New().String()

	// Save file temporarily to validate and parse
	filePath, err := h.files.SaveBook(bookID, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Validate EPUB
	if err := epub.ValidateEPUB(filePath); err != nil {
		h.files.DeleteBook(bookID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid EPUB file"})
		return
	}

	// Parse metadata
	meta, err := epub.ParseEPUB(filePath)
	if err != nil {
		h.files.DeleteBook(bookID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse EPUB metadata"})
		return
	}

	// Save cover if present
	var coverPath string
	if len(meta.CoverData) > 0 {
		coverPath, _ = h.files.SaveCover(bookID, meta.CoverData, meta.CoverExt)
	}

	// Get user ID from context (if authenticated)
	userID := auth.GetUserID(c)

	// Create book record
	book := &models.Book{
		ID:          bookID,
		UserID:      userID,
		Title:       meta.Title,
		Author:      meta.Author,
		Series:      meta.Series,
		SeriesIndex: meta.SeriesIndex,
		FilePath:    filePath,
		CoverPath:   coverPath,
		FileSize:    header.Size,
		UploadedAt:  time.Now(),
	}

	if err := h.db.CreateBook(book); err != nil {
		h.files.DeleteBook(bookID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save book metadata"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Book uploaded successfully",
		"book":    book,
	})
}

// ListBooks returns all books with optional sorting and pagination
func (h *Handler) ListBooks(c *gin.Context) {
	sortBy := c.DefaultQuery("sort", "title")
	order := c.DefaultQuery("order", "asc")
	search := c.Query("search")
	userID := auth.GetUserID(c)

	// Pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "0")) // 0 = no limit
	if page < 1 {
		page = 1
	}
	if limit < 0 {
		limit = 0
	}

	var books []models.Book
	var err error

	if search != "" {
		books, err = h.db.SearchBooksForUser(search, userID)
	} else {
		books, err = h.db.ListBooksForUser(userID, sortBy, order)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch books"})
		return
	}

	if books == nil {
		books = []models.Book{}
	}

	totalCount := len(books)

	// Apply pagination if limit is set
	if limit > 0 {
		start := (page - 1) * limit
		end := start + limit
		if start > len(books) {
			books = []models.Book{}
		} else if end > len(books) {
			books = books[start:]
		} else {
			books = books[start:end]
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"books": books,
		"count": len(books),
		"total": totalCount,
		"page":  page,
		"limit": limit,
	})
}

// GetBook returns a single book by ID
func (h *Handler) GetBook(c *gin.Context) {
	id := c.Param("id")
	userID := auth.GetUserID(c)

	var book *models.Book
	var err error

	if userID != "" {
		book, err = h.db.GetBookForUser(id, userID)
	} else {
		book, err = h.db.GetBook(id)
	}

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch book"})
		return
	}

	c.JSON(http.StatusOK, book)
}

// DeleteBook removes a book from the library
func (h *Handler) DeleteBook(c *gin.Context) {
	id := c.Param("id")

	book, err := h.db.GetBook(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch book"})
		return
	}

	// Delete file
	h.files.DeleteBook(id)

	// Delete from database
	if err := h.db.DeleteBook(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete book"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Book deleted", "book": book})
}

// GetBooksByAuthor returns books grouped by author
func (h *Handler) GetBooksByAuthor(c *gin.Context) {
	userID := auth.GetUserID(c)
	grouped, err := h.db.GetBooksByAuthorForUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch books"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"authors": grouped})
}

// GetBooksBySeries returns books grouped by series
func (h *Handler) GetBooksBySeries(c *gin.Context) {
	userID := auth.GetUserID(c)
	grouped, err := h.db.GetBooksBySeriesForUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch books"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"series": grouped})
}

// GetBookCover serves the book's cover image
func (h *Handler) GetBookCover(c *gin.Context) {
	id := c.Param("id")

	book, err := h.db.GetBook(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	if book.CoverPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No cover available"})
		return
	}

	c.File(book.CoverPath)
}

// GetTableOfContents returns the book's table of contents
func (h *Handler) GetTableOfContents(c *gin.Context) {
	id := c.Param("id")

	book, err := h.db.GetBook(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	chapters, err := epub.GetTableOfContents(book.FilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse table of contents"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"chapters": chapters})
}

// GetChapterContent returns the HTML content of a chapter
func (h *Handler) GetChapterContent(c *gin.Context) {
	id := c.Param("id")
	chapterStr := c.Param("chapter")

	chapter, err := strconv.Atoi(chapterStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid chapter number"})
		return
	}

	book, err := h.db.GetBook(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	content, err := epub.GetChapterContent(book.FilePath, chapter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get chapter content"})
		return
	}

	if content == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Chapter not found"})
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, content)
}

// GetReadingPosition returns the saved reading position for a book
func (h *Handler) GetReadingPosition(c *gin.Context) {
	id := c.Param("id")
	userID := auth.GetUserID(c)

	pos, err := h.db.GetReadingPosition(id, userID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusOK, gin.H{"position": nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get position"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"position": pos})
}

// SaveReadingPosition saves the reading position for a book
func (h *Handler) SaveReadingPosition(c *gin.Context) {
	id := c.Param("id")
	userID := auth.GetUserID(c)

	var req struct {
		Chapter  string  `json:"chapter" binding:"required"`
		Position float64 `json:"position"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Verify book exists
	if _, err := h.db.GetBook(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	pos := &models.ReadingPosition{
		BookID:   id,
		UserID:   userID,
		Chapter:  req.Chapter,
		Position: req.Position,
	}

	if err := h.db.SaveReadingPosition(pos); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save position"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Position saved", "position": pos})
}

// HealthCheck returns server health status
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now()})
}

// ServeReader serves the web reader HTML page
func (h *Handler) ServeReader(c *gin.Context) {
	readerPath := "web/static/reader.html"
	if _, err := os.Stat(readerPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reader not found"})
		return
	}
	c.File(readerPath)
}

// CreateCollection creates a new collection
func (h *Handler) CreateCollection(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required"})
		return
	}

	collection := &models.Collection{
		ID:        uuid.New().String(),
		Name:      req.Name,
		CreatedAt: time.Now(),
	}

	if err := h.db.CreateCollection(collection); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create collection"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Collection created", "collection": collection})
}

// ListCollections returns all collections
func (h *Handler) ListCollections(c *gin.Context) {
	collections, err := h.db.ListCollections()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch collections"})
		return
	}

	if collections == nil {
		collections = []models.Collection{}
	}

	c.JSON(http.StatusOK, gin.H{"collections": collections, "count": len(collections)})
}

// GetCollection returns a collection with its books
func (h *Handler) GetCollection(c *gin.Context) {
	id := c.Param("id")

	collection, err := h.db.GetCollection(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch collection"})
		return
	}

	books, err := h.db.GetBooksInCollection(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch books"})
		return
	}

	if books == nil {
		books = []models.Book{}
	}

	c.JSON(http.StatusOK, gin.H{"collection": collection, "books": books})
}

// UpdateCollection updates a collection's name
func (h *Handler) UpdateCollection(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required"})
		return
	}

	if _, err := h.db.GetCollection(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
		return
	}

	if err := h.db.UpdateCollection(id, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update collection"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Collection updated"})
}

// DeleteCollection removes a collection
func (h *Handler) DeleteCollection(c *gin.Context) {
	id := c.Param("id")

	if _, err := h.db.GetCollection(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
		return
	}

	if err := h.db.DeleteCollection(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete collection"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Collection deleted"})
}

// AddBookToCollection adds a book to a collection
func (h *Handler) AddBookToCollection(c *gin.Context) {
	collectionID := c.Param("id")
	bookID := c.Param("bookId")

	if _, err := h.db.GetCollection(collectionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
		return
	}

	if _, err := h.db.GetBook(bookID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	if err := h.db.AddBookToCollection(bookID, collectionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add book to collection"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Book added to collection"})
}

// RemoveBookFromCollection removes a book from a collection
func (h *Handler) RemoveBookFromCollection(c *gin.Context) {
	collectionID := c.Param("id")
	bookID := c.Param("bookId")

	if err := h.db.RemoveBookFromCollection(bookID, collectionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove book from collection"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Book removed from collection"})
}

// BulkAddToCollection adds multiple books to a collection
func (h *Handler) BulkAddToCollection(c *gin.Context) {
	collectionID := c.Param("id")

	var req struct {
		BookIDs []string `json:"book_ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids is required"})
		return
	}

	if _, err := h.db.GetCollection(collectionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
		return
	}

	if err := h.db.BulkAddBooksToCollection(req.BookIDs, collectionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add books to collection"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Books added to collection", "count": len(req.BookIDs)})
}

// GetBookCollections returns all collections a book belongs to
func (h *Handler) GetBookCollections(c *gin.Context) {
	bookID := c.Param("id")

	if _, err := h.db.GetBook(bookID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	collections, err := h.db.GetCollectionsForBook(bookID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch collections"})
		return
	}

	if collections == nil {
		collections = []models.Collection{}
	}

	c.JSON(http.StatusOK, gin.H{"collections": collections})
}

// ShareBook shares a book with another user
func (h *Handler) ShareBook(c *gin.Context) {
	bookID := c.Param("id")
	targetUserID := c.Param("userId")
	currentUserID := auth.GetUserID(c)

	if currentUserID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	// Check book ownership
	book, err := h.db.GetBook(bookID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	if book.UserID != currentUserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only share your own books"})
		return
	}

	// Check target user exists
	if _, err := h.db.GetUserByID(targetUserID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if err := h.db.ShareBook(bookID, currentUserID, targetUserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to share book"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Book shared successfully"})
}

// UnshareBook removes a book share
func (h *Handler) UnshareBook(c *gin.Context) {
	bookID := c.Param("id")
	targetUserID := c.Param("userId")
	currentUserID := auth.GetUserID(c)

	if currentUserID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	// Check book ownership
	book, err := h.db.GetBook(bookID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	if book.UserID != currentUserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only unshare your own books"})
		return
	}

	if err := h.db.UnshareBook(bookID, targetUserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unshare book"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Book unshared successfully"})
}

// GetSharedBooks returns books shared with the current user
func (h *Handler) GetSharedBooks(c *gin.Context) {
	userID := auth.GetUserID(c)

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	books, err := h.db.GetSharedBooks(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch shared books"})
		return
	}

	if books == nil {
		books = []models.Book{}
	}

	c.JSON(http.StatusOK, gin.H{"books": books, "count": len(books)})
}

// GetBookShares returns users a book is shared with
func (h *Handler) GetBookShares(c *gin.Context) {
	bookID := c.Param("id")
	currentUserID := auth.GetUserID(c)

	// Check book ownership
	book, err := h.db.GetBook(bookID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	if book.UserID != currentUserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only view shares for your own books"})
		return
	}

	users, err := h.db.GetBookShares(bookID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch shares"})
		return
	}

	if users == nil {
		users = []models.User{}
	}

	c.JSON(http.StatusOK, gin.H{"shared_with": users})
}

// GetChapterText returns plain text content of a chapter (for TUI clients)
func (h *Handler) GetChapterText(c *gin.Context) {
	id := c.Param("id")
	chapterStr := c.Param("chapter")

	chapter, err := strconv.Atoi(chapterStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid chapter number"})
		return
	}

	book, err := h.db.GetBook(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	content, err := epub.GetChapterText(book.FilePath, chapter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get chapter content"})
		return
	}

	if content == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Chapter not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"book_id":      id,
		"chapter":      chapter,
		"content":      content,
		"content_type": "text/plain",
	})
}

// APIInfo returns API documentation for TUI/programmatic clients
func (h *Handler) APIInfo(c *gin.Context) {
	endpoints := []gin.H{
		{"method": "GET", "path": "/health", "description": "Health check"},
		{"method": "GET", "path": "/api", "description": "API documentation"},

		// Auth
		{"method": "POST", "path": "/api/auth/register", "description": "Register new user", "body": "username, email, password"},
		{"method": "POST", "path": "/api/auth/login", "description": "Login", "body": "username, password"},
		{"method": "POST", "path": "/api/auth/refresh", "description": "Refresh JWT token", "body": "token"},
		{"method": "GET", "path": "/api/auth/me", "description": "Get current user", "auth": true},
		{"method": "GET", "path": "/api/users/search", "description": "Search users", "query": "q", "auth": true},

		// Books
		{"method": "POST", "path": "/api/books", "description": "Upload EPUB", "body": "file (multipart)"},
		{"method": "GET", "path": "/api/books", "description": "List books", "query": "sort, order, search, page, limit"},
		{"method": "GET", "path": "/api/books/:id", "description": "Get book by ID"},
		{"method": "DELETE", "path": "/api/books/:id", "description": "Delete book"},
		{"method": "GET", "path": "/api/books/by-author", "description": "Books grouped by author"},
		{"method": "GET", "path": "/api/books/by-series", "description": "Books grouped by series"},

		// Reading
		{"method": "GET", "path": "/api/books/:id/cover", "description": "Get book cover image"},
		{"method": "GET", "path": "/api/books/:id/toc", "description": "Get table of contents"},
		{"method": "GET", "path": "/api/books/:id/content/:chapter", "description": "Get chapter HTML content"},
		{"method": "GET", "path": "/api/books/:id/text/:chapter", "description": "Get chapter plain text (TUI-friendly)"},
		{"method": "GET", "path": "/api/books/:id/position", "description": "Get reading position"},
		{"method": "POST", "path": "/api/books/:id/position", "description": "Save reading position", "body": "chapter, position"},

		// Sharing
		{"method": "GET", "path": "/api/books/shared", "description": "Get books shared with you", "auth": true},
		{"method": "GET", "path": "/api/books/:id/shares", "description": "Get users book is shared with", "auth": true},
		{"method": "POST", "path": "/api/books/:id/share/:userId", "description": "Share book with user", "auth": true},
		{"method": "DELETE", "path": "/api/books/:id/share/:userId", "description": "Unshare book", "auth": true},

		// Collections
		{"method": "POST", "path": "/api/collections", "description": "Create collection", "body": "name"},
		{"method": "GET", "path": "/api/collections", "description": "List collections"},
		{"method": "GET", "path": "/api/collections/:id", "description": "Get collection with books"},
		{"method": "PUT", "path": "/api/collections/:id", "description": "Update collection", "body": "name"},
		{"method": "DELETE", "path": "/api/collections/:id", "description": "Delete collection"},
		{"method": "POST", "path": "/api/collections/:id/books/:bookId", "description": "Add book to collection"},
		{"method": "DELETE", "path": "/api/collections/:id/books/:bookId", "description": "Remove book from collection"},
		{"method": "POST", "path": "/api/collections/:id/books", "description": "Bulk add books", "body": "book_ids"},
		{"method": "GET", "path": "/api/books/:id/collections", "description": "Get collections for book"},
	}

	c.JSON(http.StatusOK, gin.H{
		"name":        "Webby API",
		"version":     "1.0.0",
		"description": "EPUB library API for web and TUI clients",
		"endpoints":   endpoints,
	})
}
