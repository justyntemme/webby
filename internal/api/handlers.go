package api

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	duplicates    *storage.DuplicateService
}

// NewHandler creates a new handler instance
func NewHandler(db *storage.Database, files *storage.FileStorage) *Handler {
	// Initialize metadata service with Open Library provider
	openLibrary := metadata.NewOpenLibraryProvider()
	metadataService := metadata.NewService(openLibrary, nil) // No fallback for now

	// Initialize comic metadata service with ComicVine provider
	comicVine := metadata.NewComicVineProvider()
	comicMetadataService := metadata.NewComicService(comicVine)

	// Initialize duplicate detection service
	duplicateService := storage.NewDuplicateService(db, files)

	return &Handler{
		db:            db,
		files:         files,
		metadata:      metadataService,
		comicMetadata: comicMetadataService,
		duplicates:    duplicateService,
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

	// Compute file hash for duplicate detection
	fileHash, err := storage.HashFile(filePath)
	if err != nil {
		log.Printf("Warning: failed to compute hash for %s: %v", filePath, err)
		fileHash = "" // Continue without hash
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
			FileHash:        fileHash,
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
			FileHash:        fileHash,
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
		meta, err := cbz.ParseCBZ(filePath, header.Filename)
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
			FileHash:        fileHash,
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
		meta, err := cbz.ParseCBR(filePath, header.Filename)
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
			FileHash:        fileHash,
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
	contentType := c.Query("type")   // "book", "comic", or empty for all
	readStatus := c.Query("status")  // "unread", "reading", "completed", or empty for all
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
		// Filter by content type and read status if specified
		if err == nil && (contentType != "" || readStatus != "") {
			filtered := make([]models.Book, 0)
			for _, b := range books {
				if contentType != "" && b.ContentType != contentType {
					continue
				}
				if readStatus != "" && b.ReadStatus != readStatus {
					continue
				}
				filtered = append(filtered, b)
			}
			books = filtered
		}
	} else {
		books, err = h.db.ListBooksForUserWithFilters(userID, sortBy, order, contentType, readStatus)
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

	// Verify book exists and get current status
	book, err := h.db.GetBook(id)
	if err != nil {
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

	// Auto-update read status to "reading" if currently "unread"
	if book.ReadStatus == "" || book.ReadStatus == models.ReadStatusUnread {
		// Only update if user owns the book or it's a public book
		if book.UserID == "" || book.UserID == userID {
			h.db.UpdateBookReadStatus(id, models.ReadStatusReading, nil)
		}
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

// CreateCollection creates a new collection (static or smart)
func (h *Handler) CreateCollection(c *gin.Context) {
	userID := auth.GetUserID(c)
	var req struct {
		Name      string `json:"name" binding:"required"`
		IsSmart   bool   `json:"is_smart"`
		RuleLogic string `json:"rule_logic"` // AND or OR
		Rules     []struct {
			Field    string `json:"field"`
			Operator string `json:"operator"`
			Value    string `json:"value"`
		} `json:"rules"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required"})
		return
	}

	ruleLogic := req.RuleLogic
	if ruleLogic == "" {
		ruleLogic = "AND"
	}

	collection := &models.Collection{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      req.Name,
		IsSmart:   req.IsSmart,
		RuleLogic: ruleLogic,
		CreatedAt: time.Now(),
	}

	if err := h.db.CreateCollection(collection); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create collection"})
		return
	}

	// Add rules if this is a smart collection
	if req.IsSmart && len(req.Rules) > 0 {
		for _, r := range req.Rules {
			rule := &models.CollectionRule{
				ID:           uuid.New().String(),
				CollectionID: collection.ID,
				Field:        r.Field,
				Operator:     r.Operator,
				Value:        r.Value,
			}
			if err := h.db.CreateCollectionRule(rule); err != nil {
				// Log error but continue
				continue
			}
			collection.Rules = append(collection.Rules, *rule)
		}
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
	userID := auth.GetUserID(c)

	collection, err := h.db.GetCollection(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch collection"})
		return
	}

	// Get rules if it's a smart collection
	if collection.IsSmart {
		rules, err := h.db.GetCollectionRules(id)
		if err == nil {
			collection.Rules = rules
		}
	}

	var books []models.Book
	if collection.IsSmart {
		// For smart collections, get books matching the rules
		books, err = h.db.GetSmartCollectionBooks(id, userID)
	} else {
		// For static collections, get the manually added books
		books, err = h.db.GetBooksInCollection(id)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch books"})
		return
	}

	if books == nil {
		books = []models.Book{}
	}

	collection.BookCount = len(books)
	c.JSON(http.StatusOK, gin.H{"collection": collection, "books": books})
}

// UpdateCollection updates a collection's name and rules
func (h *Handler) UpdateCollection(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Name      string `json:"name" binding:"required"`
		RuleLogic string `json:"rule_logic"`
		Rules     []struct {
			Field    string `json:"field"`
			Operator string `json:"operator"`
			Value    string `json:"value"`
		} `json:"rules"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required"})
		return
	}

	collection, err := h.db.GetCollection(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Collection not found"})
		return
	}

	// Update based on whether it's a smart collection
	if collection.IsSmart {
		ruleLogic := req.RuleLogic
		if ruleLogic == "" {
			ruleLogic = collection.RuleLogic
		}
		if err := h.db.UpdateSmartCollection(id, req.Name, ruleLogic); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update collection"})
			return
		}

		// Replace rules if provided
		if len(req.Rules) > 0 {
			// Delete existing rules
			h.db.DeleteCollectionRules(id)

			// Add new rules
			for _, r := range req.Rules {
				rule := &models.CollectionRule{
					ID:           uuid.New().String(),
					CollectionID: id,
					Field:        r.Field,
					Operator:     r.Operator,
					Value:        r.Value,
				}
				h.db.CreateCollectionRule(rule)
			}
		}
	} else {
		if err := h.db.UpdateCollection(id, req.Name); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update collection"})
			return
		}
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
	year := c.Query("year")

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

	// Filter by year if provided
	if year != "" {
		filtered := make([]metadata.BookMetadata, 0)
		for _, r := range results {
			// Check if publish date contains the year
			if strings.Contains(r.PublishDate, year) {
				filtered = append(filtered, r)
			}
		}
		// Only use filtered results if any matched
		if len(filtered) > 0 {
			results = filtered
		}
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

	// Write metadata to file based on format
	switch book.FileFormat {
	case models.FileFormatEPUB:
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
	case models.FileFormatPDF:
		pdfMeta := &pdf.Metadata{
			Title:    book.Title,
			Author:   book.Author,
			Subject:  book.Description,
			Keywords: result.Subjects,
		}
		if err := pdf.UpdateMetadata(book.FilePath, pdfMeta); err != nil {
			log.Printf("Warning: failed to update PDF metadata for book %s: %v", book.ID, err)
		}
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

	// Parse subjects
	subjects := []string{}
	if book.Subjects != "" {
		subjects = strings.Split(book.Subjects, ",")
		for i := range subjects {
			subjects[i] = strings.TrimSpace(subjects[i])
		}
	}

	// Write metadata to file based on format
	switch book.FileFormat {
	case models.FileFormatEPUB:
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
		}
	case models.FileFormatPDF:
		pdfMeta := &pdf.Metadata{
			Title:    book.Title,
			Author:   book.Author,
			Subject:  book.Description,
			Keywords: subjects,
		}
		if err := pdf.UpdateMetadata(book.FilePath, pdfMeta); err != nil {
			log.Printf("Warning: failed to update PDF metadata for book %s: %v", book.ID, err)
		}
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

	// Re-parse filename for better metadata extraction
	filename := filepath.Base(book.FilePath)
	parsedInfo := cbz.ParseComicFilename(filename)

	// Use parsed data for better search - prefer parsed series over stored if available
	searchSeries := book.Series
	if parsedInfo.Series != "" {
		searchSeries = parsedInfo.Series
	}

	// Get issue number from parsed info or stored SeriesIndex
	issueNumber := parsedInfo.IssueNumber
	if issueNumber == "" && book.SeriesIndex > 0 {
		// Convert float to string, removing trailing zeros
		issueNumber = strconv.FormatFloat(book.SeriesIndex, 'f', -1, 64)
	}

	// Use year from parsed filename for filtering
	year := parsedInfo.Year

	result, err := h.comicMetadata.LookupComic(ctx, searchSeries, issueNumber, book.Title, year)
	if err != nil {
		if err == metadata.ErrNoMatch {
			c.JSON(http.StatusNotFound, gin.H{
				"error":       "No matching comic metadata found",
				"parsed_info": gin.H{
					"series":       searchSeries,
					"issue_number": issueNumber,
					"year":         year,
				},
			})
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

// ReprocessComicFilename re-parses a comic's filename to extract better metadata
func (h *Handler) ReprocessComicFilename(c *gin.Context) {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "This is not a comic"})
		return
	}

	// Re-parse filename
	filename := filepath.Base(book.FilePath)
	parsedInfo := cbz.ParseComicFilename(filename)

	// Update book with parsed metadata
	oldTitle := book.Title
	oldSeries := book.Series
	oldSeriesIndex := book.SeriesIndex

	book.Title = parsedInfo.Title
	book.Series = parsedInfo.Series
	book.SeriesIndex = parsedInfo.IssueFloat

	now := time.Now()
	book.MetadataSource = "filename"
	book.MetadataUpdated = &now

	if err := h.db.UpdateBookMetadata(book); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update metadata"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Comic filename reprocessed successfully",
		"book":    book,
		"changes": gin.H{
			"title":        gin.H{"old": oldTitle, "new": book.Title},
			"series":       gin.H{"old": oldSeries, "new": book.Series},
			"series_index": gin.H{"old": oldSeriesIndex, "new": book.SeriesIndex},
			"year":         parsedInfo.Year,
			"volume":       parsedInfo.Volume,
		},
		"parsed_info": gin.H{
			"raw_filename": parsedInfo.RawFilename,
			"series":       parsedInfo.Series,
			"issue_number": parsedInfo.IssueNumber,
			"volume":       parsedInfo.Volume,
			"year":         parsedInfo.Year,
		},
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
		{"method": "POST", "path": "/api/metadata/bulk-refresh", "description": "Refresh metadata for multiple books", "body": "book_ids, content_type"},

		// Comic Metadata
		{"method": "GET", "path": "/api/metadata/comic/status", "description": "Check if comic metadata service is configured"},
		{"method": "GET", "path": "/api/metadata/comic/search", "description": "Search for comic metadata from ComicVine", "query": "series, issue, title"},
		{"method": "POST", "path": "/api/books/:id/metadata/comic/refresh", "description": "Refresh comic metadata from ComicVine"},

		// Duplicate Detection
		{"method": "GET", "path": "/api/duplicates", "description": "Find duplicate books by file hash"},
		{"method": "GET", "path": "/api/duplicates/status", "description": "Get hash computation status"},
		{"method": "POST", "path": "/api/duplicates/compute", "description": "Compute hashes for books without them"},
		{"method": "POST", "path": "/api/duplicates/merge", "description": "Merge duplicate books", "body": "keep_id, delete_ids"},

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

// BulkRefreshMetadata refreshes metadata for multiple books at once
func (h *Handler) BulkRefreshMetadata(c *gin.Context) {
	userID := auth.GetUserID(c)

	var req struct {
		BookIDs     []string `json:"book_ids"`
		ContentType string   `json:"content_type"` // Optional: "book" or "comic" to refresh all of that type
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// If book_ids is empty but content_type is specified, get all books of that type
	var booksToRefresh []models.Book
	if len(req.BookIDs) == 0 && req.ContentType != "" {
		books, err := h.db.ListBooksForUserWithFilter(userID, "title", "asc", req.ContentType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch books"})
			return
		}
		booksToRefresh = books
	} else if len(req.BookIDs) > 0 {
		for _, id := range req.BookIDs {
			var book *models.Book
			var err error
			if userID != "" {
				book, err = h.db.GetBookForUser(id, userID)
			} else {
				book, err = h.db.GetBook(id)
			}
			if err == nil && book != nil {
				booksToRefresh = append(booksToRefresh, *book)
			}
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Either book_ids or content_type is required"})
		return
	}

	if len(booksToRefresh) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message":   "No books to refresh",
			"processed": 0,
			"succeeded": 0,
			"failed":    0,
		})
		return
	}

	// Limit batch size to prevent timeouts
	maxBatch := 50
	if len(booksToRefresh) > maxBatch {
		booksToRefresh = booksToRefresh[:maxBatch]
	}

	ctx := c.Request.Context()
	results := make([]gin.H, 0)
	succeeded := 0
	failed := 0

	for _, book := range booksToRefresh {
		var result gin.H

		if book.ContentType == models.ContentTypeComic {
			// Use comic metadata service
			if !h.comicMetadata.IsConfigured() {
				result = gin.H{
					"book_id": book.ID,
					"title":   book.Title,
					"status":  "skipped",
					"reason":  "Comic metadata service not configured",
				}
				failed++
			} else {
				// Re-parse filename for better matching
				filename := filepath.Base(book.FilePath)
				parsedInfo := cbz.ParseComicFilename(filename)

				searchSeries := book.Series
				if parsedInfo.Series != "" {
					searchSeries = parsedInfo.Series
				}

				issueNumber := parsedInfo.IssueNumber
				if issueNumber == "" && book.SeriesIndex > 0 {
					issueNumber = strconv.FormatFloat(book.SeriesIndex, 'f', -1, 64)
				}

				comicResult, err := h.comicMetadata.LookupComic(ctx, searchSeries, issueNumber, book.Title, parsedInfo.Year)
				if err != nil || comicResult == nil || comicResult.Confidence < 0.5 {
					result = gin.H{
						"book_id": book.ID,
						"title":   book.Title,
						"status":  "failed",
						"reason":  "No matching metadata found",
					}
					failed++
				} else {
					// Update book metadata
					now := time.Now()
					if comicResult.Title != "" {
						book.Title = comicResult.Title
					}
					if comicResult.Series != "" {
						book.Series = comicResult.Series
					}
					if len(comicResult.Writers) > 0 {
						book.Author = comicResult.Writers[0]
					}
					book.Publisher = comicResult.Publisher
					book.PublishDate = comicResult.ReleaseDate
					book.Description = comicResult.Description
					book.MetadataSource = comicResult.Source
					book.MetadataUpdated = &now

					if err := h.db.UpdateBookMetadata(&book); err != nil {
						result = gin.H{
							"book_id": book.ID,
							"title":   book.Title,
							"status":  "failed",
							"reason":  "Failed to save metadata",
						}
						failed++
					} else {
						result = gin.H{
							"book_id":    book.ID,
							"title":      book.Title,
							"status":     "success",
							"confidence": comicResult.Confidence,
						}
						succeeded++
					}
				}
			}
		} else {
			// Use book metadata service
			bookResult, err := h.metadata.LookupBook(ctx, book.ISBN, book.Title, book.Author)
			if err != nil || bookResult == nil || bookResult.Confidence < 0.5 {
				result = gin.H{
					"book_id": book.ID,
					"title":   book.Title,
					"status":  "failed",
					"reason":  "No matching metadata found",
				}
				failed++
			} else {
				// Update book metadata
				now := time.Now()
				book.Title = bookResult.Title
				if len(bookResult.Authors) > 0 {
					book.Author = bookResult.Authors[0]
				}
				if bookResult.ISBN13 != "" {
					book.ISBN = bookResult.ISBN13
				} else if bookResult.ISBN10 != "" {
					book.ISBN = bookResult.ISBN10
				}
				book.Publisher = bookResult.Publisher
				book.PublishDate = bookResult.PublishDate
				book.Description = bookResult.Description
				book.Language = bookResult.Language
				book.Subjects = strings.Join(bookResult.Subjects, ", ")
				book.MetadataSource = bookResult.Source
				book.MetadataUpdated = &now

				if err := h.db.UpdateBookMetadata(&book); err != nil {
					result = gin.H{
						"book_id": book.ID,
						"title":   book.Title,
						"status":  "failed",
						"reason":  "Failed to save metadata",
					}
					failed++
				} else {
					result = gin.H{
						"book_id":    book.ID,
						"title":      book.Title,
						"status":     "success",
						"confidence": bookResult.Confidence,
					}
					succeeded++
				}
			}
		}

		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Bulk metadata refresh complete",
		"processed": len(booksToRefresh),
		"succeeded": succeeded,
		"failed":    failed,
		"results":   results,
	})
}

// GetDuplicates returns groups of books with the same file hash
func (h *Handler) GetDuplicates(c *gin.Context) {
	userID := auth.GetUserID(c)

	groups, err := h.duplicates.FindDuplicates(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find duplicates"})
		return
	}

	if groups == nil {
		groups = []storage.DuplicateGroup{}
	}

	// Convert to response format
	response := make([]gin.H, 0, len(groups))
	for _, g := range groups {
		response = append(response, gin.H{
			"file_hash": g.FileHash,
			"count":     len(g.Books),
			"books":     g.Books,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"groups": response,
		"count":  len(groups),
	})
}

// GetDuplicatesStatus returns the status of hash computation
func (h *Handler) GetDuplicatesStatus(c *gin.Context) {
	userID := auth.GetUserID(c)

	unhashed, err := h.db.CountBooksWithoutHash(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get status"})
		return
	}

	groups, err := h.duplicates.FindDuplicates(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count duplicates"})
		return
	}

	duplicateCount := 0
	for _, g := range groups {
		duplicateCount += len(g.Books) - 1 // Count extras (all but one per group)
	}

	c.JSON(http.StatusOK, gin.H{
		"books_without_hash": unhashed,
		"duplicate_groups":   len(groups),
		"duplicate_books":    duplicateCount,
		"ready":              unhashed == 0,
	})
}

// ComputeHashes computes missing file hashes for duplicate detection
func (h *Handler) ComputeHashes(c *gin.Context) {
	userID := auth.GetUserID(c)

	progress, err := h.duplicates.ComputeMissingHashes(userID, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to compute hashes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Hash computation complete",
		"total":     progress.Total,
		"processed": progress.Processed,
		"failed":    progress.Failed,
	})
}

// MergeDuplicates merges a group of duplicate books, keeping one and deleting the rest
func (h *Handler) MergeDuplicates(c *gin.Context) {
	userID := auth.GetUserID(c)

	var req struct {
		KeepID    string   `json:"keep_id" binding:"required"`
		DeleteIDs []string `json:"delete_ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "keep_id and delete_ids are required"})
		return
	}

	result, err := h.duplicates.MergeDuplicates(req.KeepID, req.DeleteIDs, userID)
	if err != nil {
		if err == storage.ErrNotOwner {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only merge your own books"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to merge duplicates"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Duplicates merged successfully",
		"kept_book":     result.KeptBook,
		"deleted_books": result.DeletedBooks,
		"files_removed": result.FilesRemoved,
	})
}

// GetBookReadStatus returns the read status for a book
func (h *Handler) GetBookReadStatus(c *gin.Context) {
	id := c.Param("id")
	userID := auth.GetUserID(c)

	// Verify book exists and user has access
	book, err := h.db.GetBookForUser(id, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"book_id":        book.ID,
		"read_status":    book.ReadStatus,
		"date_completed": book.DateCompleted,
	})
}

// UpdateBookReadStatus updates the read status for a book
func (h *Handler) UpdateBookReadStatus(c *gin.Context) {
	id := c.Param("id")
	userID := auth.GetUserID(c)

	var req struct {
		Status string `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status is required"})
		return
	}

	// Validate status value
	if req.Status != models.ReadStatusUnread && req.Status != models.ReadStatusReading && req.Status != models.ReadStatusCompleted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status. Must be 'unread', 'reading', or 'completed'"})
		return
	}

	// Verify book exists and user has access
	book, err := h.db.GetBookForUser(id, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	// Check ownership for modification
	if book.UserID != "" && book.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only update status for your own books"})
		return
	}

	// Set date_completed if marking as completed
	var dateCompleted *time.Time
	if req.Status == models.ReadStatusCompleted {
		now := time.Now()
		dateCompleted = &now
	}

	if err := h.db.UpdateBookReadStatus(id, req.Status, dateCompleted); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update read status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "Read status updated",
		"book_id":        id,
		"read_status":    req.Status,
		"date_completed": dateCompleted,
	})
}

// GetReadStatusCounts returns counts of books by read status
func (h *Handler) GetReadStatusCounts(c *gin.Context) {
	userID := auth.GetUserID(c)

	counts, err := h.db.GetReadStatusCounts(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get status counts"})
		return
	}

	c.JSON(http.StatusOK, counts)
}

// BulkUpdateReadStatus updates read status for multiple books
func (h *Handler) BulkUpdateReadStatus(c *gin.Context) {
	userID := auth.GetUserID(c)

	var req struct {
		BookIDs []string `json:"book_ids" binding:"required"`
		Status  string   `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids and status are required"})
		return
	}

	// Validate status value
	if req.Status != models.ReadStatusUnread && req.Status != models.ReadStatusReading && req.Status != models.ReadStatusCompleted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status. Must be 'unread', 'reading', or 'completed'"})
		return
	}

	// Limit batch size
	if len(req.BookIDs) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Maximum 100 books per batch"})
		return
	}

	// Verify ownership of all books
	var validBookIDs []string
	for _, bookID := range req.BookIDs {
		book, err := h.db.GetBookForUser(bookID, userID)
		if err != nil {
			continue // Skip books that don't exist or user doesn't have access to
		}
		// Only allow updating own books
		if book.UserID == "" || book.UserID == userID {
			validBookIDs = append(validBookIDs, bookID)
		}
	}

	if len(validBookIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No valid books to update"})
		return
	}

	// Set date_completed if marking as completed
	var dateCompleted *time.Time
	if req.Status == models.ReadStatusCompleted {
		now := time.Now()
		dateCompleted = &now
	}

	if err := h.db.BulkUpdateBookReadStatus(validBookIDs, req.Status, dateCompleted); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update read status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Read status updated",
		"updated_count":   len(validBookIDs),
		"requested_count": len(req.BookIDs),
		"status":          req.Status,
	})
}

// ==================== Star Ratings ====================

// GetBookRating returns the star rating for a book
func (h *Handler) GetBookRating(c *gin.Context) {
	id := c.Param("id")
	userID := auth.GetUserID(c)

	// Get book (verify access)
	var book *models.Book
	var err error
	if userID != "" {
		book, err = h.db.GetBookForUser(id, userID)
	} else {
		book, err = h.db.GetBook(id)
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"book_id": book.ID,
		"rating":  book.Rating,
	})
}

// UpdateBookRating updates the star rating for a book (0-5)
func (h *Handler) UpdateBookRating(c *gin.Context) {
	id := c.Param("id")
	userID := auth.GetUserID(c)

	var req struct {
		Rating int `json:"rating" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rating is required"})
		return
	}

	// Validate rating range (0-5)
	if req.Rating < 0 || req.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rating must be between 0 and 5"})
		return
	}

	// Verify book exists and user has access
	book, err := h.db.GetBookForUser(id, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	// Check ownership for modification
	if book.UserID != "" && book.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only rate your own books"})
		return
	}

	if err := h.db.UpdateBookRating(id, req.Rating); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update rating"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Rating updated",
		"book_id": id,
		"rating":  req.Rating,
	})
}

// ==================== Reading Lists ====================

// ListReadingLists returns all reading lists for the current user
func (h *Handler) ListReadingLists(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	// Ensure system lists exist
	if err := h.db.EnsureSystemReadingLists(userID); err != nil {
		log.Printf("Warning: Failed to ensure system reading lists: %v", err)
	}

	lists, err := h.db.ListReadingLists(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reading lists"})
		return
	}

	if lists == nil {
		lists = []models.ReadingList{}
	}

	c.JSON(http.StatusOK, gin.H{
		"lists": lists,
		"count": len(lists),
	})
}

// GetReadingList returns a single reading list with its books
func (h *Handler) GetReadingList(c *gin.Context) {
	id := c.Param("id")
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	list, err := h.db.GetReadingList(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reading list not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reading list"})
		return
	}

	// Verify ownership
	if list.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Get books in the list
	books, err := h.db.GetBooksInReadingList(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch books"})
		return
	}

	if books == nil {
		books = []models.Book{}
	}

	c.JSON(http.StatusOK, gin.H{
		"list":  list,
		"books": books,
	})
}

// CreateReadingList creates a new custom reading list
func (h *Handler) CreateReadingList(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	list := &models.ReadingList{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      req.Name,
		ListType:  models.ReadingListCustom,
		CreatedAt: time.Now(),
	}

	if err := h.db.CreateReadingList(list); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create reading list"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Reading list created",
		"list":    list,
	})
}

// UpdateReadingList updates a reading list's name
func (h *Handler) UpdateReadingList(c *gin.Context) {
	id := c.Param("id")
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	// Verify list exists and user owns it
	list, err := h.db.GetReadingList(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reading list not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reading list"})
		return
	}

	if list.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	if err := h.db.UpdateReadingList(id, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update reading list"})
		return
	}

	list.Name = req.Name
	c.JSON(http.StatusOK, gin.H{
		"message": "Reading list updated",
		"list":    list,
	})
}

// DeleteReadingList deletes a custom reading list
func (h *Handler) DeleteReadingList(c *gin.Context) {
	id := c.Param("id")
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	// Verify list exists and user owns it
	list, err := h.db.GetReadingList(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reading list not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reading list"})
		return
	}

	if list.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Don't allow deleting system lists
	if list.ListType != models.ReadingListCustom {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete system reading lists"})
		return
	}

	if err := h.db.DeleteReadingList(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete reading list"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Reading list deleted",
	})
}

// AddBookToReadingList adds a book to a reading list
func (h *Handler) AddBookToReadingList(c *gin.Context) {
	listID := c.Param("id")
	bookID := c.Param("bookId")
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	// Verify list exists and user owns it
	list, err := h.db.GetReadingList(listID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reading list not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reading list"})
		return
	}

	if list.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Verify book exists and user has access
	_, err = h.db.GetBookForUser(bookID, userID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch book"})
		return
	}

	if err := h.db.AddBookToReadingList(bookID, listID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add book to list"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Book added to reading list",
		"list_id": listID,
		"book_id": bookID,
	})
}

// RemoveBookFromReadingList removes a book from a reading list
func (h *Handler) RemoveBookFromReadingList(c *gin.Context) {
	listID := c.Param("id")
	bookID := c.Param("bookId")
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	// Verify list exists and user owns it
	list, err := h.db.GetReadingList(listID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reading list not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reading list"})
		return
	}

	if list.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	if err := h.db.RemoveBookFromReadingList(bookID, listID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove book from list"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Book removed from reading list",
		"list_id": listID,
		"book_id": bookID,
	})
}

// GetBookReadingLists returns the reading lists a book belongs to
func (h *Handler) GetBookReadingLists(c *gin.Context) {
	bookID := c.Param("id")
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	// Verify book exists and user has access
	_, err := h.db.GetBookForUser(bookID, userID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch book"})
		return
	}

	lists, err := h.db.GetReadingListsForBook(bookID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reading lists"})
		return
	}

	if lists == nil {
		lists = []models.ReadingList{}
	}

	c.JSON(http.StatusOK, gin.H{
		"book_id": bookID,
		"lists":   lists,
	})
}

// ToggleBookInReadingList adds or removes a book from a reading list
func (h *Handler) ToggleBookInReadingList(c *gin.Context) {
	listID := c.Param("id")
	bookID := c.Param("bookId")
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	// Verify list exists and user owns it
	list, err := h.db.GetReadingList(listID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reading list not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reading list"})
		return
	}

	if list.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Verify book exists and user has access
	_, err = h.db.GetBookForUser(bookID, userID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch book"})
		return
	}

	// Check if book is already in the list
	inList, err := h.db.IsBookInReadingList(bookID, listID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check list membership"})
		return
	}

	var action string
	if inList {
		if err := h.db.RemoveBookFromReadingList(bookID, listID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove book from list"})
			return
		}
		action = "removed"
	} else {
		if err := h.db.AddBookToReadingList(bookID, listID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add book to list"})
			return
		}
		action = "added"
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Book " + action + " from reading list",
		"action":  action,
		"list_id": listID,
		"book_id": bookID,
		"in_list": !inList,
	})
}

// ReorderReadingList updates the order of books in a reading list
func (h *Handler) ReorderReadingList(c *gin.Context) {
	listID := c.Param("id")
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req struct {
		BookIDs []string `json:"book_ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids is required"})
		return
	}

	// Verify list exists and user owns it
	list, err := h.db.GetReadingList(listID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Reading list not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch reading list"})
		return
	}

	if list.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	if err := h.db.ReorderReadingList(listID, req.BookIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reorder reading list"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Reading list reordered",
		"list_id": listID,
	})
}

// ==================== Tag Handlers ====================

// ListTags returns all tags for the authenticated user
func (h *Handler) ListTags(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	tags, err := h.db.ListTags(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tags"})
		return
	}

	if tags == nil {
		tags = []*models.Tag{}
	}

	c.JSON(http.StatusOK, gin.H{
		"tags":  tags,
		"count": len(tags),
	})
}

// CreateTag creates a new tag
func (h *Handler) CreateTag(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req struct {
		Name  string `json:"name" binding:"required"`
		Color string `json:"color"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required"})
		return
	}

	// Validate name
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tag name cannot be empty"})
		return
	}

	// Default color if not provided
	if req.Color == "" {
		req.Color = "#3b82f6"
	}

	// Check if tag already exists
	existing, _ := h.db.GetTagByName(userID, req.Name)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Tag already exists", "tag": existing})
		return
	}

	tag := &models.Tag{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      req.Name,
		Color:     req.Color,
		CreatedAt: time.Now(),
	}

	if err := h.db.CreateTag(tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create tag"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Tag created",
		"tag":     tag,
	})
}

// GetTag returns a specific tag
func (h *Handler) GetTag(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	tagID := c.Param("id")
	tag, err := h.db.GetTag(tagID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tag not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tag"})
		return
	}

	// Verify ownership
	if tag.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, tag)
}

// UpdateTag updates a tag's name and/or color
func (h *Handler) UpdateTag(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	tagID := c.Param("id")
	tag, err := h.db.GetTag(tagID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tag not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tag"})
		return
	}

	// Verify ownership
	if tag.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Use existing values if not provided
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = tag.Name
	}
	color := req.Color
	if color == "" {
		color = tag.Color
	}

	// Check if new name conflicts with existing tag
	if name != tag.Name {
		existing, _ := h.db.GetTagByName(userID, name)
		if existing != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Tag with this name already exists"})
			return
		}
	}

	if err := h.db.UpdateTag(tagID, name, color); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update tag"})
		return
	}

	tag.Name = name
	tag.Color = color

	c.JSON(http.StatusOK, gin.H{
		"message": "Tag updated",
		"tag":     tag,
	})
}

// DeleteTag deletes a tag
func (h *Handler) DeleteTag(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	tagID := c.Param("id")
	tag, err := h.db.GetTag(tagID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tag not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tag"})
		return
	}

	// Verify ownership
	if tag.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	if err := h.db.DeleteTag(tagID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete tag"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Tag deleted",
		"tag_id":  tagID,
	})
}

// GetBookTags returns all tags for a specific book
func (h *Handler) GetBookTags(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	bookID := c.Param("id")

	// Verify book exists and user has access
	book, err := h.db.GetBookForUser(bookID, userID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch book"})
		return
	}

	if book.UserID != userID {
		// Check if shared
		shared, _ := h.db.IsBookSharedWith(bookID, userID)
		if !shared {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}
	}

	tags, err := h.db.GetBookTags(bookID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tags"})
		return
	}

	if tags == nil {
		tags = []*models.Tag{}
	}

	c.JSON(http.StatusOK, gin.H{
		"book_id": bookID,
		"tags":    tags,
		"count":   len(tags),
	})
}

// AddTagToBook adds a tag to a book
func (h *Handler) AddTagToBook(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	bookID := c.Param("id")
	tagID := c.Param("tagId")

	// Verify book exists and user owns it
	book, err := h.db.GetBookForUser(bookID, userID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch book"})
		return
	}

	if book.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Can only tag your own books"})
		return
	}

	// Verify tag exists and user owns it
	tag, err := h.db.GetTag(tagID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tag not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tag"})
		return
	}

	if tag.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Can only use your own tags"})
		return
	}

	if err := h.db.AddTagToBook(bookID, tagID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add tag to book"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Tag added to book",
		"book_id": bookID,
		"tag_id":  tagID,
	})
}

// RemoveTagFromBook removes a tag from a book
func (h *Handler) RemoveTagFromBook(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	bookID := c.Param("id")
	tagID := c.Param("tagId")

	// Verify book exists and user owns it
	book, err := h.db.GetBookForUser(bookID, userID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch book"})
		return
	}

	if book.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Can only modify tags on your own books"})
		return
	}

	if err := h.db.RemoveTagFromBook(bookID, tagID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove tag from book"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Tag removed from book",
		"book_id": bookID,
		"tag_id":  tagID,
	})
}

// ToggleBookTag toggles a tag on a book
func (h *Handler) ToggleBookTag(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	bookID := c.Param("id")
	tagID := c.Param("tagId")

	// Verify book exists and user owns it
	book, err := h.db.GetBookForUser(bookID, userID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch book"})
		return
	}

	if book.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Can only tag your own books"})
		return
	}

	// Verify tag exists and user owns it
	tag, err := h.db.GetTag(tagID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tag not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tag"})
		return
	}

	if tag.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Can only use your own tags"})
		return
	}

	inTag, err := h.db.ToggleBookTag(bookID, tagID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle tag"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"book_id": bookID,
		"tag_id":  tagID,
		"in_tag":  inTag,
	})
}

// GetBooksByTag returns all books with a specific tag
func (h *Handler) GetBooksByTag(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	tagID := c.Param("id")

	// Verify tag exists and user owns it
	tag, err := h.db.GetTag(tagID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tag not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tag"})
		return
	}

	if tag.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	books, err := h.db.GetBooksByTag(tagID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch books"})
		return
	}

	if books == nil {
		books = []*models.Book{}
	}

	c.JSON(http.StatusOK, gin.H{
		"tag":   tag,
		"books": books,
		"count": len(books),
	})
}

// ==================== Annotations API ====================

// ListAnnotationsForBook returns all annotations for a book
func (h *Handler) ListAnnotationsForBook(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	bookID := c.Param("id")

	// Verify book exists and user has access
	book, err := h.db.GetBook(bookID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch book"})
		return
	}

	// Check access (owner or shared with)
	if book.UserID != userID {
		shared, _ := h.db.IsBookSharedWith(bookID, userID)
		if !shared {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}
	}

	annotations, err := h.db.GetAnnotationsForBook(bookID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch annotations"})
		return
	}

	if annotations == nil {
		annotations = []*models.Annotation{}
	}

	c.JSON(http.StatusOK, gin.H{
		"annotations": annotations,
		"count":       len(annotations),
	})
}

// ListAnnotationsForChapter returns annotations for a specific chapter
func (h *Handler) ListAnnotationsForChapter(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	bookID := c.Param("id")
	chapter := c.Param("chapter")

	// Verify book exists and user has access
	book, err := h.db.GetBook(bookID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch book"})
		return
	}

	// Check access
	if book.UserID != userID {
		shared, _ := h.db.IsBookSharedWith(bookID, userID)
		if !shared {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}
	}

	annotations, err := h.db.GetAnnotationsForChapter(bookID, userID, chapter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch annotations"})
		return
	}

	if annotations == nil {
		annotations = []*models.Annotation{}
	}

	c.JSON(http.StatusOK, gin.H{
		"annotations": annotations,
		"count":       len(annotations),
	})
}

// CreateAnnotation creates a new annotation/highlight
func (h *Handler) CreateAnnotation(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	bookID := c.Param("id")

	// Verify book exists and user has access
	book, err := h.db.GetBook(bookID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch book"})
		return
	}

	// Check access
	if book.UserID != userID {
		shared, _ := h.db.IsBookSharedWith(bookID, userID)
		if !shared {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}
	}

	var req struct {
		Chapter      string `json:"chapter" binding:"required"`
		CFI          string `json:"cfi"`
		StartOffset  int    `json:"start_offset"`
		EndOffset    int    `json:"end_offset"`
		SelectedText string `json:"selected_text" binding:"required"`
		Note         string `json:"note"`
		Color        string `json:"color"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chapter and selected_text are required"})
		return
	}

	// Validate color
	validColors := map[string]bool{
		models.HighlightColorYellow: true,
		models.HighlightColorGreen:  true,
		models.HighlightColorBlue:   true,
		models.HighlightColorPink:   true,
		models.HighlightColorOrange: true,
	}
	if req.Color == "" {
		req.Color = models.HighlightColorYellow
	} else if !validColors[req.Color] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid highlight color. Use: yellow, green, blue, pink, or orange"})
		return
	}

	now := time.Now()
	annotation := &models.Annotation{
		ID:           uuid.New().String(),
		BookID:       bookID,
		UserID:       userID,
		Chapter:      req.Chapter,
		CFI:          req.CFI,
		StartOffset:  req.StartOffset,
		EndOffset:    req.EndOffset,
		SelectedText: req.SelectedText,
		Note:         req.Note,
		Color:        req.Color,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := h.db.CreateAnnotation(annotation); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create annotation"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":    "Annotation created",
		"annotation": annotation,
	})
}

// GetAnnotation returns a specific annotation
func (h *Handler) GetAnnotation(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	annotationID := c.Param("annotationId")

	annotation, err := h.db.GetAnnotation(annotationID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Annotation not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch annotation"})
		return
	}

	// Verify ownership
	if annotation.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, annotation)
}

// UpdateAnnotation updates an annotation's note and/or color
func (h *Handler) UpdateAnnotation(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	annotationID := c.Param("annotationId")

	annotation, err := h.db.GetAnnotation(annotationID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Annotation not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch annotation"})
		return
	}

	// Verify ownership
	if annotation.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	var req struct {
		Note  string `json:"note"`
		Color string `json:"color"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Use existing values if not provided
	note := req.Note
	color := req.Color
	if color == "" {
		color = annotation.Color
	} else {
		// Validate color if provided
		validColors := map[string]bool{
			models.HighlightColorYellow: true,
			models.HighlightColorGreen:  true,
			models.HighlightColorBlue:   true,
			models.HighlightColorPink:   true,
			models.HighlightColorOrange: true,
		}
		if !validColors[color] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid highlight color"})
			return
		}
	}

	if err := h.db.UpdateAnnotation(annotationID, note, color); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update annotation"})
		return
	}

	annotation.Note = note
	annotation.Color = color
	annotation.UpdatedAt = time.Now()

	c.JSON(http.StatusOK, gin.H{
		"message":    "Annotation updated",
		"annotation": annotation,
	})
}

// DeleteAnnotation removes an annotation
func (h *Handler) DeleteAnnotation(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	annotationID := c.Param("annotationId")

	annotation, err := h.db.GetAnnotation(annotationID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Annotation not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch annotation"})
		return
	}

	// Verify ownership
	if annotation.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	if err := h.db.DeleteAnnotation(annotationID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete annotation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Annotation deleted"})
}

// ListAllAnnotations returns all annotations for the current user
func (h *Handler) ListAllAnnotations(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	annotations, err := h.db.GetAllAnnotationsForUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch annotations"})
		return
	}

	if annotations == nil {
		annotations = []*models.Annotation{}
	}

	c.JSON(http.StatusOK, gin.H{
		"annotations": annotations,
		"count":       len(annotations),
	})
}

// GetAnnotationStats returns annotation statistics for the current user
func (h *Handler) GetAnnotationStats(c *gin.Context) {
	userID := auth.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	totalAnnotations, booksWithAnnotations, err := h.db.GetAnnotationStats(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch annotation stats"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total_annotations":      totalAnnotations,
		"books_with_annotations": booksWithAnnotations,
	})
}
