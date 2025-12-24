package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"github.com/justyntemme/webby/internal/api"
	"github.com/justyntemme/webby/internal/auth"
	"github.com/justyntemme/webby/internal/storage"
)

func main() {
	// Command-line flags
	urlFlag := flag.String("url", "", "Server bind address (e.g., :8080 or 0.0.0.0:8080)")
	disableRegFlag := flag.Bool("disable-registration", false, "Disable new user registration")
	flag.Parse()

	// Configuration
	dataDir := getEnv("WEBBY_DATA_DIR", "./data")
	dbPath := filepath.Join(dataDir, "webby.db")
	port := getEnv("WEBBY_PORT", "8080")

	// Determine bind address: flag takes precedence, then env, then default
	bindAddr := ":" + port
	if *urlFlag != "" {
		bindAddr = *urlFlag
	}

	// Check if registration is disabled (flag or env var)
	disableRegistration := *disableRegFlag || getEnv("WEBBY_DISABLE_REGISTRATION", "") == "true"

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Initialize database
	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize file storage
	files, err := storage.NewFileStorage(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize file storage: %v", err)
	}

	// Initialize handlers
	handler := api.NewHandler(db, files)
	authHandler := api.NewAuthHandler(db, disableRegistration)

	// Set up Gin router
	r := gin.Default()

	// Enable CORS for mobile access
	r.Use(corsMiddleware())

	// Health check
	r.GET("/health", handler.HealthCheck)

	// API routes
	apiGroup := r.Group("/api")
	{
		// API documentation (for TUI clients)
		apiGroup.GET("", handler.APIInfo)

		// Auth routes (public)
		authGroup := apiGroup.Group("/auth")
		{
			authGroup.GET("/status", authHandler.GetAuthStatus)
			authGroup.POST("/register", authHandler.Register)
			authGroup.POST("/login", authHandler.Login)
			authGroup.POST("/refresh", authHandler.RefreshToken)
		}

		// Protected routes (require authentication)
		protected := apiGroup.Group("")
		protected.Use(auth.AuthMiddleware())
		{
			// Current user
			protected.GET("/auth/me", authHandler.GetCurrentUser)
			protected.GET("/users/search", authHandler.SearchUsers)

			// Reading Lists
			protected.GET("/reading-lists", handler.ListReadingLists)
			protected.POST("/reading-lists", handler.CreateReadingList)
			protected.GET("/reading-lists/:id", handler.GetReadingList)
			protected.PUT("/reading-lists/:id", handler.UpdateReadingList)
			protected.DELETE("/reading-lists/:id", handler.DeleteReadingList)
			protected.POST("/reading-lists/:id/books/:bookId", handler.AddBookToReadingList)
			protected.DELETE("/reading-lists/:id/books/:bookId", handler.RemoveBookFromReadingList)
			protected.PUT("/reading-lists/:id/books/:bookId/toggle", handler.ToggleBookInReadingList)
			protected.PUT("/reading-lists/:id/reorder", handler.ReorderReadingList)
			protected.GET("/books/:id/reading-lists", handler.GetBookReadingLists)

			// Custom Tags
			protected.GET("/tags", handler.ListTags)
			protected.POST("/tags", handler.CreateTag)
			protected.GET("/tags/:id", handler.GetTag)
			protected.PUT("/tags/:id", handler.UpdateTag)
			protected.DELETE("/tags/:id", handler.DeleteTag)
			protected.GET("/tags/:id/books", handler.GetBooksByTag)
			protected.GET("/books/:id/tags", handler.GetBookTags)
			protected.POST("/books/:id/tags/:tagId", handler.AddTagToBook)
			protected.DELETE("/books/:id/tags/:tagId", handler.RemoveTagFromBook)
			protected.PUT("/books/:id/tags/:tagId/toggle", handler.ToggleBookTag)

			// Annotations & Highlights
			protected.GET("/annotations", handler.ListAllAnnotations)
			protected.GET("/annotations/stats", handler.GetAnnotationStats)
			protected.GET("/books/:id/annotations", handler.ListAnnotationsForBook)
			protected.GET("/books/:id/annotations/chapter/:chapter", handler.ListAnnotationsForChapter)
			protected.POST("/books/:id/annotations", handler.CreateAnnotation)
			protected.GET("/books/:id/annotations/:annotationId", handler.GetAnnotation)
			protected.PUT("/books/:id/annotations/:annotationId", handler.UpdateAnnotation)
			protected.DELETE("/books/:id/annotations/:annotationId", handler.DeleteAnnotation)

			// Reading Statistics
			protected.GET("/stats", handler.GetUserStatistics)
			protected.GET("/stats/summary", handler.GetStatsSummary)
			protected.GET("/stats/daily", handler.GetDailyStats)
			protected.GET("/stats/sessions", handler.GetRecentSessions)
			protected.POST("/stats/sessions", handler.StartReadingSession)
			protected.PUT("/stats/sessions/:id", handler.EndReadingSession)
			protected.PUT("/books/:id/reading-session", handler.UpdateReadingSessionProgress)
			protected.GET("/books/:id/stats", handler.GetBookReadingStats)
		}

		// Book routes - use optional auth for backward compatibility
		// When auth is present, operations are scoped to user
		booksGroup := apiGroup.Group("")
		booksGroup.Use(auth.OptionalAuthMiddleware())
		{
			// Books
			booksGroup.POST("/books", handler.UploadBook)
			booksGroup.GET("/books", handler.ListBooks)
			booksGroup.GET("/books/:id", handler.GetBook)
			booksGroup.DELETE("/books/:id", handler.DeleteBook)

			// Grouping
			booksGroup.GET("/books/by-author", handler.GetBooksByAuthor)
			booksGroup.GET("/books/by-series", handler.GetBooksBySeries)

			// Similar books recommendations
			booksGroup.GET("/books/:id/similar", handler.GetSimilarBooks)

			// Reading
			booksGroup.GET("/books/:id/cover", handler.GetBookCover)
			booksGroup.GET("/books/:id/file", handler.GetBookFile)
			booksGroup.GET("/books/:id/toc", handler.GetTableOfContents)
			booksGroup.GET("/books/:id/content/:chapter", handler.GetChapterContent)
			booksGroup.GET("/books/:id/text/:chapter", handler.GetChapterText)
			booksGroup.GET("/books/:id/resource/*path", handler.GetBookResource)

			// CBZ comic reading
			booksGroup.GET("/books/:id/cbz/info", handler.GetCBZInfo)
			booksGroup.GET("/books/:id/cbz/page/:page", handler.GetCBZPage)

			// Reading position
			booksGroup.GET("/books/:id/position", handler.GetReadingPosition)
			booksGroup.POST("/books/:id/position", handler.SaveReadingPosition)

			// Read status tracking
			booksGroup.GET("/books/status/counts", handler.GetReadStatusCounts)
			booksGroup.GET("/books/:id/status", handler.GetBookReadStatus)
			booksGroup.PUT("/books/:id/status", handler.UpdateBookReadStatus)
			booksGroup.POST("/books/status/bulk", handler.BulkUpdateReadStatus)

			// Star ratings
			booksGroup.GET("/books/:id/rating", handler.GetBookRating)
			booksGroup.PUT("/books/:id/rating", handler.UpdateBookRating)

			// Book collections (for a specific book)
			booksGroup.GET("/books/:id/collections", handler.GetBookCollections)

			// Book Metadata
			booksGroup.GET("/metadata/lookup", handler.LookupMetadata)
			booksGroup.GET("/metadata/search", handler.SearchMetadata)
			booksGroup.POST("/books/:id/metadata/refresh", handler.RefreshBookMetadata)
			booksGroup.PUT("/books/:id/metadata", handler.UpdateBookMetadata)
			booksGroup.POST("/metadata/bulk-refresh", handler.BulkRefreshMetadata)

			// Comic Metadata
			booksGroup.GET("/metadata/comic/status", handler.GetComicMetadataStatus)
			booksGroup.GET("/metadata/comic/search", handler.SearchComicMetadata)
			booksGroup.POST("/books/:id/metadata/comic/refresh", handler.RefreshComicMetadata)
			booksGroup.POST("/books/:id/metadata/comic/reprocess", handler.ReprocessComicFilename)

			// Duplicate Detection
			booksGroup.GET("/duplicates", handler.GetDuplicates)
			booksGroup.GET("/duplicates/status", handler.GetDuplicatesStatus)
			booksGroup.POST("/duplicates/compute", handler.ComputeHashes)
			booksGroup.POST("/duplicates/merge", handler.MergeDuplicates)

			// Book sharing
			booksGroup.GET("/books/shared", handler.GetSharedBooks)
			booksGroup.GET("/books/:id/shares", handler.GetBookShares)
			booksGroup.POST("/books/:id/share/:userId", handler.ShareBook)
			booksGroup.DELETE("/books/:id/share/:userId", handler.UnshareBook)

			// Collections
			booksGroup.POST("/collections", handler.CreateCollection)
			booksGroup.GET("/collections", handler.ListCollections)
			booksGroup.GET("/collections/:id", handler.GetCollection)
			booksGroup.PUT("/collections/:id", handler.UpdateCollection)
			booksGroup.DELETE("/collections/:id", handler.DeleteCollection)
			booksGroup.POST("/collections/:id/books/:bookId", handler.AddBookToCollection)
			booksGroup.DELETE("/collections/:id/books/:bookId", handler.RemoveBookFromCollection)
			booksGroup.POST("/collections/:id/books", handler.BulkAddToCollection)
		}
	}

	// OPDS routes for e-reader apps
	opdsGroup := r.Group("/opds/v1.2")
	opdsGroup.Use(auth.OptionalAuthMiddleware())
	{
		// Root catalog
		opdsGroup.GET("/catalog.xml", handler.OPDSCatalog)

		// Acquisition feeds
		opdsGroup.GET("/books/all.xml", handler.OPDSAllBooks)
		opdsGroup.GET("/books/recent.xml", handler.OPDSRecentBooks)
		opdsGroup.GET("/books/ebooks.xml", handler.OPDSEBooks)
		opdsGroup.GET("/books/comics.xml", handler.OPDSComics)

		// Navigation feeds
		opdsGroup.GET("/authors.xml", handler.OPDSAuthors)
		opdsGroup.GET("/authors/:author", handler.OPDSAuthorBooks)
		opdsGroup.GET("/series.xml", handler.OPDSSeries)
		opdsGroup.GET("/series/:series", handler.OPDSSeriesBooks)

		// Search
		opdsGroup.GET("/search.xml", handler.OPDSSearch)

		// Book download
		opdsGroup.GET("/books/:id/download", handler.OPDSDownload)
	}

	// Serve static files for web reader
	r.Static("/static", "web/static")
	r.GET("/reader/:id", handler.ServeReader)

	// Serve auth page
	r.GET("/auth", func(c *gin.Context) {
		c.File("web/static/auth.html")
	})

	// Serve duplicates page
	r.GET("/duplicates", func(c *gin.Context) {
		c.File("web/static/duplicates.html")
	})

	// Serve library index at root
	r.GET("/", func(c *gin.Context) {
		c.File("web/static/index.html")
	})

	// Start server
	log.Printf("Webby server starting on %s", bindAddr)
	log.Printf("Data directory: %s", dataDir)
	if err := r.Run(bindAddr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
