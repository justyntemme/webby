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
	authHandler := api.NewAuthHandler(db)

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

			// Reading
			booksGroup.GET("/books/:id/cover", handler.GetBookCover)
			booksGroup.GET("/books/:id/toc", handler.GetTableOfContents)
			booksGroup.GET("/books/:id/content/:chapter", handler.GetChapterContent)
			booksGroup.GET("/books/:id/text/:chapter", handler.GetChapterText)

			// Reading position
			booksGroup.GET("/books/:id/position", handler.GetReadingPosition)
			booksGroup.POST("/books/:id/position", handler.SaveReadingPosition)

			// Book collections (for a specific book)
			booksGroup.GET("/books/:id/collections", handler.GetBookCollections)

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

	// Serve static files for web reader
	r.Static("/static", "web/static")
	r.GET("/reader/:id", handler.ServeReader)

	// Serve auth page
	r.GET("/auth", func(c *gin.Context) {
		c.File("web/static/auth.html")
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
