package api

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/justyntemme/webby/internal/auth"
	"github.com/justyntemme/webby/internal/cbz"
	"github.com/justyntemme/webby/internal/epub"
	"github.com/justyntemme/webby/internal/metadata"
	"github.com/justyntemme/webby/internal/models"
	"github.com/justyntemme/webby/internal/pdf"
	"github.com/justyntemme/webby/internal/storage"
)

// Handler contains all HTTP handlers
type Handler struct {
	db            *storage.Database
	files         *storage.FileStorage
	metadata      *metadata.Service
	comicMetadata *metadata.ComicService
}

// NewHandler creates a new handler instance
func NewHandler(db *storage.Database, files *storage.FileStorage) *Handler {
	// Initialize metadata service with Open Library provider
	openLibrary := metadata.NewOpenLibraryProvider()
	metadataService := metadata.NewService(openLibrary, nil) // No fallback for now

	// Initialize comic metadata service with ComicVine provider
	comicVine := metadata.NewComicVineProvider()
	comicMetadataService := metadata.NewComicService(comicVine)

	return &Handler{
		db:            db,
		files:         files,
		metadata:      metadataService,
		comicMetadata: comicMetadataService,
	}
}

// UploadBook handles EPUB and PDF file uploads
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

	// Detect file type from extension
	filename := strings.ToLower(header.Filename)
	var fileFormat string
	var fileExt string

	switch {
	case strings.HasSuffix(filename, ".epub"):
		fileFormat = models.FileFormatEPUB
		fileExt = ".epub"
	case strings.HasSuffix(filename, ".pdf"):
		fileFormat = models.FileFormatPDF
		fileExt = ".pdf"
	case strings.HasSuffix(filename, ".cbz"):
		fileFormat = models.FileFormatCBZ
		fileExt = ".cbz"
	case strings.HasSuffix(filename, ".cbr"):
		fileFormat = models.FileFormatCBR
		fileExt = ".cbr"
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported file format. Please upload EPUB, PDF, CBZ, or CBR files."})
		return
	}

	// Generate unique ID
	bookID := uuid.New().String()

	// Save file with appropriate extension
	filePath, err := h.files.SaveBookWithExt(bookID, file, fileExt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	var book *models.Book
	now := time.Now()
	userID := auth.GetUserID(c)

	if fileFormat == models.FileFormatEPUB {
		// Validate EPUB
		if err := epub.ValidateEPUB(filePath); err != nil {
			h.files.DeleteBook(bookID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid EPUB file"})
			return
		}

		// Parse EPUB metadata
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

		contentType := meta.ContentType
		if contentType == "" {
			contentType = models.ContentTypeBook
		}

		book = &models.Book{
			ID:              bookID,
			UserID:          userID,
			Title:           meta.Title,
			Author:          meta.Author,
			Series:          meta.Series,
			SeriesIndex:     meta.SeriesIndex,
			FilePath:        filePath,
			CoverPath:       coverPath,
			FileSize:        header.Size,
			UploadedAt:      now,
			ContentType:     contentType,
			FileFormat:      models.FileFormatEPUB,
			ISBN:            meta.ISBN,
			Publisher:       meta.Publisher,
			PublishDate:     meta.PublishDate,
			Description:     meta.Description,
			Language:        meta.Language,
			Subjects:        strings.Join(meta.Subjects, ", "),
			MetadataSource:  "epub",
			MetadataUpdated: &now,
		}
	} else if fileFormat == models.FileFormatPDF {
		// Validate PDF
		if err := pdf.ValidatePDF(filePath); err != nil {
			h.files.DeleteBook(bookID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid PDF file"})
			return
		}

		// Parse PDF metadata
		meta, err := pdf.ParsePDF(filePath)
		if err != nil {
			h.files.DeleteBook(bookID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse PDF metadata"})
			return
		}

		// Try to extract cover image from first page
		var coverPath string
		if cover, err := pdf.ExtractCover(filePath); err == nil && len(cover.Data) > 0 {
			coverPath, _ = h.files.SaveCover(bookID, cover.Data, cover.Extension)
		}

		contentType := meta.ContentType
		if contentType == "" {
			contentType = models.ContentTypeBook
		}

		book = &models.Book{
			ID:              bookID,
			UserID:          userID,
			Title:           meta.Title,
			Author:          meta.Author,
			FilePath:        filePath,
			CoverPath:       coverPath,
			FileSize:        header.Size,
			UploadedAt:      now,
			ContentType:     contentType,
			FileFormat:      models.FileFormatPDF,
			Subjects:        strings.Join(meta.Keywords, ", "),
			MetadataSource:  "pdf",
			MetadataUpdated: &now,
		}
	} else if fileFormat == models.FileFormatCBZ {
		// Validate CBZ
		if err := cbz.ValidateCBZ(filePath); err != nil {
			h.files.DeleteBook(bookID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid CBZ file"})
			return
		}

		// Parse CBZ metadata
		meta, err := cbz.ParseCBZ(filePath)
		if err != nil {
			h.files.DeleteBook(bookID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse CBZ metadata"})
			return
		}

		// Extract cover image from first page
		var coverPath string
		if cover, err := cbz.ExtractCover(filePath); err == nil && len(cover.Data) > 0 {
			coverPath, _ = h.files.SaveCover(bookID, cover.Data, cover.Extension)
		}

		book = &models.Book{
			ID:              bookID,
			UserID:          userID,
			Title:           meta.Title,
			Author:          meta.Author,
			Series:          meta.Series,
			SeriesIndex:     meta.SeriesIndex,
			FilePath:        filePath,
			CoverPath:       coverPath,
			FileSize:        header.Size,
			UploadedAt:      now,
			ContentType:     models.ContentTypeComic, // CBZ is always comic
			FileFormat:      models.FileFormatCBZ,
			MetadataSource:  "cbz",
			MetadataUpdated: &now,
		}
	} else if fileFormat == models.FileFormatCBR {
		// Validate CBR
		if err := cbz.ValidateCBR(filePath); err != nil {
			h.files.DeleteBook(bookID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid CBR file"})
			return
		}

		// Parse CBR metadata
		meta, err := cbz.ParseCBR(filePath)
		if err != nil {
			h.files.DeleteBook(bookID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse CBR metadata"})
			return
		}

		// Extract cover image from first page
		var coverPath string
		if cover, err := cbz.ExtractCoverCBR(filePath); err == nil && len(cover.Data) > 0 {
			coverPath, _ = h.files.SaveCover(bookID, cover.Data, cover.Extension)
		}

		book = &models.Book{
			ID:              bookID,
			UserID:          userID,
			Title:           meta.Title,
			Author:          meta.Author,
			Series:          meta.Series,
			SeriesIndex:     meta.SeriesIndex,
			FilePath:        filePath,
			CoverPath:       coverPath,
			FileSize:        header.Size,
			UploadedAt:      now,
			ContentType:     models.ContentTypeComic, // CBR is always comic
			FileFormat:      models.FileFormatCBR,
			MetadataSource:  "cbr",
			MetadataUpdated: &now,
		}
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
	contentType := c.Query("type") // "book", "comic", or empty for all
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
		// Filter by content type if specified
		if contentType != "" && err == nil {
			filtered := make([]models.Book, 0)
			for _, b := range books {
				if b.ContentType == contentType {
					filtered = append(filtered, b)
				}
			}
			books = filtered
		}
	} else {
		books, err = h.db.ListBooksForUserWithFilter(userID, sortBy, order, contentType)
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

// ServeReader serves the web reader HTML page (EPUB or PDF based on book format)
func (h *Handler) ServeReader(c *gin.Context) {
	id := c.Param("id")

	// Get book to determine format
	book, err := h.db.GetBook(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	var readerPath string
	switch book.FileFormat {
	case models.FileFormatPDF:
		readerPath = "web/static/pdf-reader.html"
	case models.FileFormatCBZ, models.FileFormatCBR:
		readerPath = "web/static/cbz-reader.html"
	default:
		readerPath = "web/static/reader.html"
	}

	if _, err := os.Stat(readerPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reader not found"})
		return
	}
	c.File(readerPath)
}

// GetBookFile serves the actual book file (PDF or EPUB) for reading
func (h *Handler) GetBookFile(c *gin.Context) {
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

	// Set appropriate content type
	var contentType string
	switch book.FileFormat {
	case models.FileFormatPDF:
		contentType = "application/pdf"
	case models.FileFormatEPUB:
		contentType = "application/epub+zip"
	case models.FileFormatCBZ:
		contentType = "application/zip"
	case models.FileFormatCBR:
		contentType = "application/x-rar-compressed"
	default:
		contentType = "application/octet-stream"
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", "inline; filename=\""+book.Title+"\"")
	c.File(book.FilePath)
}

// GetCBZPage serves a specific page from a CBZ file
func (h *Handler) GetCBZPage(c *gin.Context) {
	id := c.Param("id")
	pageStr := c.Param("page")
	userID := auth.GetUserID(c)

	pageIndex, err := strconv.Atoi(pageStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page number"})
		return
	}

	var book *models.Book
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

	if book.FileFormat != models.FileFormatCBZ && book.FileFormat != models.FileFormatCBR {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Book is not a comic file (CBZ/CBR)"})
		return
	}

	var data []byte
	var contentType string
	if book.FileFormat == models.FileFormatCBR {
		data, contentType, err = cbz.GetPageCBR(book.FilePath, pageIndex)
	} else {
		data, contentType, err = cbz.GetPage(book.FilePath, pageIndex)
	}
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, contentType, data)
}

// GetCBZInfo returns page count and other info for a CBZ/CBR
func (h *Handler) GetCBZInfo(c *gin.Context) {
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

	if book.FileFormat != models.FileFormatCBZ && book.FileFormat != models.FileFormatCBR {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Book is not a comic file (CBZ/CBR)"})
		return
	}

	var pageCount int
	if book.FileFormat == models.FileFormatCBR {
		pageCount, err = cbz.GetPageCountCBR(book.FilePath)
	} else {
		pageCount, err = cbz.GetPageCount(book.FilePath)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get page count"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"pageCount": pageCount,
		"title":     book.Title,
		"author":    book.Author,
		"series":    book.Series,
	})
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

// SearchMetadata searches for book metadata and returns all matches for selection
func (h *Handler) SearchMetadata(c *gin.Context) {
	isbn := c.Query("isbn")
	title := c.Query("title")
	author := c.Query("author")

	if isbn == "" && title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least isbn or title is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	results, err := h.metadata.SearchBooks(ctx, isbn, title, author)
	if err != nil {
		if err == metadata.ErrNoMatch {
			c.JSON(http.StatusNotFound, gin.H{"error": "No matching metadata found"})
			return
		}
		if err == metadata.ErrRateLimited {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Rate limited, please try again later"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search metadata"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"results": results, "count": len(results)})
}

// LookupMetadata searches for book metadata from external sources
func (h *Handler) LookupMetadata(c *gin.Context) {
	isbn := c.Query("isbn")
	title := c.Query("title")
	author := c.Query("author")

	if isbn == "" && title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least isbn or title is required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	result, err := h.metadata.LookupBook(ctx, isbn, title, author)
	if err != nil {
		if err == metadata.ErrNoMatch {
			c.JSON(http.StatusNotFound, gin.H{"error": "No matching metadata found"})
			return
		}
		if err == metadata.ErrRateLimited {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Rate limited, please try again later"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to lookup metadata"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"metadata": result})
}

// RefreshBookMetadata fetches and updates metadata for an existing book
func (h *Handler) RefreshBookMetadata(c *gin.Context) {
	id := c.Param("id")
	userID := auth.GetUserID(c)

	// Get the book
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

	// Lookup metadata
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	result, err := h.metadata.LookupBook(ctx, book.ISBN, book.Title, book.Author)
	if err != nil {
		if err == metadata.ErrNoMatch {
			c.JSON(http.StatusNotFound, gin.H{"error": "No matching metadata found"})
			return
		}
		if err == metadata.ErrRateLimited {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Rate limited, please try again later"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to lookup metadata"})
		return
	}

	// Only update if confidence is above threshold
	if result.Confidence < 0.5 {
		c.JSON(http.StatusOK, gin.H{
			"message":    "Match confidence too low, metadata not updated",
			"metadata":   result,
			"confidence": result.Confidence,
		})
		return
	}

	// Update book with external metadata
	now := time.Now()
	book.Title = result.Title
	if len(result.Authors) > 0 {
		book.Author = result.Authors[0]
	}
	if result.ISBN13 != "" {
		book.ISBN = result.ISBN13
	} else if result.ISBN10 != "" {
		book.ISBN = result.ISBN10
	}
	book.Publisher = result.Publisher
	book.PublishDate = result.PublishDate
	book.Description = result.Description
	book.Language = result.Language
	book.Subjects = strings.Join(result.Subjects, ", ")
	book.MetadataSource = result.Source
	book.MetadataUpdated = &now

	if err := h.db.UpdateBookMetadata(book); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update metadata"})
		return
	}

	// Write metadata to EPUB file
	epubMeta := &epub.Metadata{
		Title:       book.Title,
		Author:      book.Author,
		Series:      book.Series,
		SeriesIndex: book.SeriesIndex,
		ISBN:        book.ISBN,
		Publisher:   book.Publisher,
		PublishDate: book.PublishDate,
		Language:    book.Language,
		Description: book.Description,
		Subjects:    result.Subjects,
	}
	if err := epub.UpdateMetadata(book.FilePath, epubMeta); err != nil {
		log.Printf("Warning: failed to update EPUB metadata for book %s: %v", book.ID, err)
	}

	// Reorganize book to correct folder structure
	newPaths, err := h.files.ReorganizeBook(book.FilePath, book.CoverPath, book.Author, book.Series, book.Title)
	if err != nil {
		log.Printf("Warning: failed to reorganize book %s: %v", book.ID, err)
	} else if newPaths.BookPath != book.FilePath || newPaths.CoverPath != book.CoverPath {
		if err := h.db.UpdateBookFilePaths(book.ID, newPaths.BookPath, newPaths.CoverPath); err != nil {
			log.Printf("Warning: failed to update file paths for book %s: %v", book.ID, err)
		}
		book.FilePath = newPaths.BookPath
		book.CoverPath = newPaths.CoverPath
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Metadata updated successfully",
		"book":       book,
		"confidence": result.Confidence,
		"source":     result.Source,
	})
}

// UpdateBookMetadata manually updates book metadata fields
func (h *Handler) UpdateBookMetadata(c *gin.Context) {
	id := c.Param("id")
	userID := auth.GetUserID(c)

	// Get the book
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

	// Parse request body
	var req struct {
		Title       string  `json:"title"`
		Author      string  `json:"author"`
		Series      string  `json:"series"`
		SeriesIndex float64 `json:"series_index"`
		ISBN        string  `json:"isbn"`
		Publisher   string  `json:"publisher"`
		PublishDate string  `json:"publish_date"`
		Language    string  `json:"language"`
		Subjects    string  `json:"subjects"`
		Description string  `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Update book fields
	if req.Title != "" {
		book.Title = req.Title
	}
	if req.Author != "" {
		book.Author = req.Author
	}
	book.Series = req.Series
	book.SeriesIndex = req.SeriesIndex
	book.ISBN = req.ISBN
	book.Publisher = req.Publisher
	book.PublishDate = req.PublishDate
	book.Language = req.Language
	book.Subjects = req.Subjects
	book.Description = req.Description
	book.MetadataSource = "manual"
	now := time.Now()
	book.MetadataUpdated = &now

	// Update database metadata
	if err := h.db.UpdateBookMetadata(book); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update metadata"})
		return
	}

	// Write metadata to EPUB file
	subjects := []string{}
	if book.Subjects != "" {
		subjects = strings.Split(book.Subjects, ",")
		for i := range subjects {
			subjects[i] = strings.TrimSpace(subjects[i])
		}
	}
	epubMeta := &epub.Metadata{
		Title:       book.Title,
		Author:      book.Author,
		Series:      book.Series,
		SeriesIndex: book.SeriesIndex,
		ISBN:        book.ISBN,
		Publisher:   book.Publisher,
		PublishDate: book.PublishDate,
		Language:    book.Language,
		Description: book.Description,
		Subjects:    subjects,
	}
	if err := epub.UpdateMetadata(book.FilePath, epubMeta); err != nil {
		log.Printf("Warning: failed to update EPUB metadata for book %s: %v", book.ID, err)
		// Continue anyway - database was updated
	}

	// Reorganize book to correct folder structure
	newPaths, err := h.files.ReorganizeBook(book.FilePath, book.CoverPath, book.Author, book.Series, book.Title)
	if err != nil {
		log.Printf("Warning: failed to reorganize book %s: %v", book.ID, err)
		// Continue anyway - metadata was updated
	} else if newPaths.BookPath != book.FilePath || newPaths.CoverPath != book.CoverPath {
		// Update file paths in database
		if err := h.db.UpdateBookFilePaths(book.ID, newPaths.BookPath, newPaths.CoverPath); err != nil {
			log.Printf("Warning: failed to update file paths for book %s: %v", book.ID, err)
		}
		book.FilePath = newPaths.BookPath
		book.CoverPath = newPaths.CoverPath
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Metadata updated successfully",
		"book":    book,
	})
}

// SearchComicMetadata searches for comic metadata from ComicVine
func (h *Handler) SearchComicMetadata(c *gin.Context) {
	series := c.Query("series")
	issue := c.Query("issue")
	title := c.Query("title")

	if series == "" && title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least series or title is required"})
		return
	}

	if !h.comicMetadata.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "Comic metadata service not configured",
			"message": "Set COMICVINE_API_KEY environment variable to enable comic metadata lookup",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	results, err := h.comicMetadata.SearchComics(ctx, series, issue, title)
	if err != nil {
		if err == metadata.ErrNoMatch {
			c.JSON(http.StatusNotFound, gin.H{"error": "No matching comic metadata found"})
			return
		}
		if err == metadata.ErrRateLimited {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Rate limited, please try again later"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search comic metadata"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"results": results, "count": len(results)})
}

// RefreshComicMetadata fetches and updates metadata for a comic
func (h *Handler) RefreshComicMetadata(c *gin.Context) {
	id := c.Param("id")
	userID := auth.GetUserID(c)

	// Get the book
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

	// Verify this is a comic
	if book.ContentType != models.ContentTypeComic {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This is not a comic. Use the book metadata refresh endpoint."})
		return
	}

	if !h.comicMetadata.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "Comic metadata service not configured",
			"message": "Set COMICVINE_API_KEY environment variable to enable comic metadata lookup",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	// Use series and title as search hints
	result, err := h.comicMetadata.LookupComic(ctx, book.Series, "", book.Title)
	if err != nil {
		if err == metadata.ErrNoMatch {
			c.JSON(http.StatusNotFound, gin.H{"error": "No matching comic metadata found"})
			return
		}
		if err == metadata.ErrRateLimited {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Rate limited, please try again later"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to lookup comic metadata"})
		return
	}

	// Only update if confidence is above threshold
	if result.Confidence < 0.5 {
		c.JSON(http.StatusOK, gin.H{
			"message":    "Match confidence too low, metadata not updated",
			"metadata":   result,
			"confidence": result.Confidence,
		})
		return
	}

	// Update book with comic metadata
	now := time.Now()
	if result.Title != "" {
		book.Title = result.Title
	}
	if result.Series != "" {
		book.Series = result.Series
	}
	// Use first writer as author
	if len(result.Writers) > 0 {
		book.Author = result.Writers[0]
	}
	if result.Publisher != "" {
		book.Publisher = result.Publisher
	}
	if result.ReleaseDate != "" {
		book.PublishDate = result.ReleaseDate
	}
	if result.Description != "" {
		book.Description = result.Description
	}
	// Combine genres as subjects
	if len(result.Genres) > 0 {
		book.Subjects = strings.Join(result.Genres, ", ")
	}
	book.MetadataSource = result.Source
	book.MetadataUpdated = &now

	if err := h.db.UpdateBookMetadata(book); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update metadata"})
		return
	}

	// Reorganize book to correct folder structure
	newPaths, err := h.files.ReorganizeBook(book.FilePath, book.CoverPath, book.Author, book.Series, book.Title)
	if err != nil {
		log.Printf("Warning: failed to reorganize comic %s: %v", book.ID, err)
	} else if newPaths.BookPath != book.FilePath || newPaths.CoverPath != book.CoverPath {
		if err := h.db.UpdateBookFilePaths(book.ID, newPaths.BookPath, newPaths.CoverPath); err != nil {
			log.Printf("Warning: failed to update file paths for comic %s: %v", book.ID, err)
		}
		book.FilePath = newPaths.BookPath
		book.CoverPath = newPaths.CoverPath
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Comic metadata updated successfully",
		"book":       book,
		"confidence": result.Confidence,
		"source":     result.Source,
	})
}

// GetComicMetadataStatus returns whether comic metadata service is configured
func (h *Handler) GetComicMetadataStatus(c *gin.Context) {
	configured := h.comicMetadata.IsConfigured()
	c.JSON(http.StatusOK, gin.H{
		"configured": configured,
		"provider":   "comicvine",
		"message": func() string {
			if configured {
				return "Comic metadata service is ready"
			}
			return "Set COMICVINE_API_KEY environment variable to enable"
		}(),
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
		{"method": "POST", "path": "/api/books", "description": "Upload EPUB/PDF/CBZ", "body": "file (multipart)"},
		{"method": "GET", "path": "/api/books", "description": "List books", "query": "sort, order, search, page, limit, type (book/comic)"},
		{"method": "GET", "path": "/api/books/:id", "description": "Get book by ID"},
		{"method": "DELETE", "path": "/api/books/:id", "description": "Delete book"},
		{"method": "GET", "path": "/api/books/by-author", "description": "Books grouped by author"},
		{"method": "GET", "path": "/api/books/by-series", "description": "Books grouped by series"},

		// Reading
		{"method": "GET", "path": "/api/books/:id/cover", "description": "Get book cover image"},
		{"method": "GET", "path": "/api/books/:id/file", "description": "Get book file (PDF/EPUB/CBZ)"},
		{"method": "GET", "path": "/api/books/:id/toc", "description": "Get table of contents (EPUB only)"},
		{"method": "GET", "path": "/api/books/:id/content/:chapter", "description": "Get chapter HTML content (EPUB only)"},
		{"method": "GET", "path": "/api/books/:id/text/:chapter", "description": "Get chapter plain text (EPUB only, TUI-friendly)"},
		{"method": "GET", "path": "/api/books/:id/cbz/info", "description": "Get CBZ comic info and page count"},
		{"method": "GET", "path": "/api/books/:id/cbz/page/:page", "description": "Get specific page from CBZ"},
		{"method": "GET", "path": "/api/books/:id/position", "description": "Get reading position"},
		{"method": "POST", "path": "/api/books/:id/position", "description": "Save reading position", "body": "chapter, position"},

		// Book Metadata
		{"method": "GET", "path": "/api/metadata/lookup", "description": "Lookup book metadata from external sources", "query": "isbn, title, author"},
		{"method": "GET", "path": "/api/metadata/search", "description": "Search for book metadata and return all matches", "query": "isbn, title, author"},
		{"method": "POST", "path": "/api/books/:id/metadata/refresh", "description": "Refresh book metadata from external sources"},
		{"method": "PUT", "path": "/api/books/:id/metadata", "description": "Manually update book metadata", "body": "title, author, series, series_index, isbn, publisher, publish_date, language, subjects, description"},

		// Comic Metadata
		{"method": "GET", "path": "/api/metadata/comic/status", "description": "Check if comic metadata service is configured"},
		{"method": "GET", "path": "/api/metadata/comic/search", "description": "Search for comic metadata from ComicVine", "query": "series, issue, title"},
		{"method": "POST", "path": "/api/books/:id/metadata/comic/refresh", "description": "Refresh comic metadata from ComicVine"},

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
		"description": "EPUB/PDF/CBZ library API for web and TUI clients",
		"endpoints":   endpoints,
	})
}
