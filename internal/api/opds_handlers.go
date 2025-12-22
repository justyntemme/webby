package api

import (
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/justyntemme/webby/internal/auth"
	"github.com/justyntemme/webby/internal/opds"
)

// getBaseURL constructs the base URL from the request
func getBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

// OPDSCatalog serves the root OPDS navigation catalog
func (h *Handler) OPDSCatalog(c *gin.Context) {
	baseURL := getBaseURL(c)
	selfURL := baseURL + "/opds/v1.2/catalog.xml"

	feed := opds.NewNavigationFeed(
		"Webby Library",
		"urn:webby:catalog:root",
		selfURL,
		selfURL,
	)

	// Add search link
	feed.AddSearchLink(baseURL + "/opds/v1.2/search.xml")

	// Add navigation entries
	feed.AddNavigationEntry(
		"All Books",
		"urn:webby:catalog:all",
		baseURL+"/opds/v1.2/books/all.xml",
		"Browse all books in the library",
	)

	feed.AddNavigationEntry(
		"Recent Books",
		"urn:webby:catalog:recent",
		baseURL+"/opds/v1.2/books/recent.xml",
		"Recently added books",
	)

	feed.AddNavigationEntry(
		"By Author",
		"urn:webby:catalog:authors",
		baseURL+"/opds/v1.2/authors.xml",
		"Browse books by author",
	)

	feed.AddNavigationEntry(
		"By Series",
		"urn:webby:catalog:series",
		baseURL+"/opds/v1.2/series.xml",
		"Browse books by series",
	)

	feed.AddNavigationEntry(
		"eBooks",
		"urn:webby:catalog:ebooks",
		baseURL+"/opds/v1.2/books/ebooks.xml",
		"EPUB and PDF books",
	)

	feed.AddNavigationEntry(
		"Comics",
		"urn:webby:catalog:comics",
		baseURL+"/opds/v1.2/books/comics.xml",
		"Comic books (CBZ/CBR)",
	)

	xml, err := feed.ToXML()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate feed"})
		return
	}

	c.Data(http.StatusOK, opds.OPDSCatalogType, xml)
}

// OPDSAllBooks serves an acquisition feed of all books
func (h *Handler) OPDSAllBooks(c *gin.Context) {
	userID := auth.GetUserID(c)
	baseURL := getBaseURL(c)
	selfURL := baseURL + "/opds/v1.2/books/all.xml"
	startURL := baseURL + "/opds/v1.2/catalog.xml"

	books, err := h.db.ListBooksForUser(userID, "title", "asc")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list books"})
		return
	}

	feed := opds.NewAcquisitionFeed(
		"All Books",
		"urn:webby:catalog:all",
		selfURL,
		startURL,
	)

	for _, book := range books {
		feed.Entries = append(feed.Entries, opds.BookToEntry(&book, baseURL))
	}

	xml, err := feed.ToXML()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate feed"})
		return
	}

	c.Data(http.StatusOK, opds.OPDSFeedType, xml)
}

// OPDSRecentBooks serves an acquisition feed of recently added books
func (h *Handler) OPDSRecentBooks(c *gin.Context) {
	userID := auth.GetUserID(c)
	baseURL := getBaseURL(c)
	selfURL := baseURL + "/opds/v1.2/books/recent.xml"
	startURL := baseURL + "/opds/v1.2/catalog.xml"

	books, err := h.db.ListBooksForUser(userID, "uploaded_at", "desc")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list books"})
		return
	}

	// Limit to 50 most recent
	if len(books) > 50 {
		books = books[:50]
	}

	feed := opds.NewAcquisitionFeed(
		"Recent Books",
		"urn:webby:catalog:recent",
		selfURL,
		startURL,
	)

	for _, book := range books {
		feed.Entries = append(feed.Entries, opds.BookToEntry(&book, baseURL))
	}

	xml, err := feed.ToXML()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate feed"})
		return
	}

	c.Data(http.StatusOK, opds.OPDSFeedType, xml)
}

// OPDSEBooks serves an acquisition feed of ebooks only
func (h *Handler) OPDSEBooks(c *gin.Context) {
	userID := auth.GetUserID(c)
	baseURL := getBaseURL(c)
	selfURL := baseURL + "/opds/v1.2/books/ebooks.xml"
	startURL := baseURL + "/opds/v1.2/catalog.xml"

	books, err := h.db.ListBooksForUserWithFilter(userID, "title", "asc", "book")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list books"})
		return
	}

	feed := opds.NewAcquisitionFeed(
		"eBooks",
		"urn:webby:catalog:ebooks",
		selfURL,
		startURL,
	)

	for _, book := range books {
		feed.Entries = append(feed.Entries, opds.BookToEntry(&book, baseURL))
	}

	xml, err := feed.ToXML()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate feed"})
		return
	}

	c.Data(http.StatusOK, opds.OPDSFeedType, xml)
}

// OPDSComics serves an acquisition feed of comics only
func (h *Handler) OPDSComics(c *gin.Context) {
	userID := auth.GetUserID(c)
	baseURL := getBaseURL(c)
	selfURL := baseURL + "/opds/v1.2/books/comics.xml"
	startURL := baseURL + "/opds/v1.2/catalog.xml"

	books, err := h.db.ListBooksForUserWithFilter(userID, "title", "asc", "comic")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list books"})
		return
	}

	feed := opds.NewAcquisitionFeed(
		"Comics",
		"urn:webby:catalog:comics",
		selfURL,
		startURL,
	)

	for _, book := range books {
		feed.Entries = append(feed.Entries, opds.BookToEntry(&book, baseURL))
	}

	xml, err := feed.ToXML()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate feed"})
		return
	}

	c.Data(http.StatusOK, opds.OPDSFeedType, xml)
}

// OPDSAuthors serves a navigation feed of all authors
func (h *Handler) OPDSAuthors(c *gin.Context) {
	userID := auth.GetUserID(c)
	baseURL := getBaseURL(c)
	selfURL := baseURL + "/opds/v1.2/authors.xml"
	startURL := baseURL + "/opds/v1.2/catalog.xml"

	authorBooks, err := h.db.GetBooksByAuthorForUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get authors"})
		return
	}

	feed := opds.NewNavigationFeed(
		"Authors",
		"urn:webby:catalog:authors",
		selfURL,
		startURL,
	)

	// Get sorted list of authors
	var authors []string
	for author := range authorBooks {
		authors = append(authors, author)
	}
	sort.Strings(authors)

	for _, authorName := range authors {
		displayName := authorName
		if displayName == "" {
			displayName = "Unknown Author"
		}
		// URL-encode the author name for the path
		encodedAuthor := strings.ReplaceAll(authorName, " ", "%20")
		feed.AddNavigationEntry(
			displayName,
			"urn:webby:author:"+authorName,
			baseURL+"/opds/v1.2/authors/"+encodedAuthor+".xml",
			"",
		)
	}

	xml, err := feed.ToXML()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate feed"})
		return
	}

	c.Data(http.StatusOK, opds.OPDSCatalogType, xml)
}

// OPDSAuthorBooks serves an acquisition feed of books by a specific author
func (h *Handler) OPDSAuthorBooks(c *gin.Context) {
	author := c.Param("author")
	// URL decode if needed - Gin should handle this
	author = strings.TrimSuffix(author, ".xml")

	userID := auth.GetUserID(c)
	baseURL := getBaseURL(c)
	selfURL := baseURL + "/opds/v1.2/authors/" + strings.ReplaceAll(author, " ", "%20") + ".xml"
	startURL := baseURL + "/opds/v1.2/catalog.xml"

	books, err := h.db.ListBooksForUser(userID, "title", "asc")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list books"})
		return
	}

	displayAuthor := author
	if displayAuthor == "" {
		displayAuthor = "Unknown Author"
	}

	feed := opds.NewAcquisitionFeed(
		"Books by "+displayAuthor,
		"urn:webby:author:"+author,
		selfURL,
		startURL,
	)

	for _, book := range books {
		if strings.EqualFold(book.Author, author) {
			feed.Entries = append(feed.Entries, opds.BookToEntry(&book, baseURL))
		}
	}

	xml, err := feed.ToXML()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate feed"})
		return
	}

	c.Data(http.StatusOK, opds.OPDSFeedType, xml)
}

// OPDSSeries serves a navigation feed of all series
func (h *Handler) OPDSSeries(c *gin.Context) {
	userID := auth.GetUserID(c)
	baseURL := getBaseURL(c)
	selfURL := baseURL + "/opds/v1.2/series.xml"
	startURL := baseURL + "/opds/v1.2/catalog.xml"

	seriesBooks, err := h.db.GetBooksBySeriesForUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get series"})
		return
	}

	feed := opds.NewNavigationFeed(
		"Series",
		"urn:webby:catalog:series",
		selfURL,
		startURL,
	)

	// Get sorted list of series
	var seriesList []string
	for series := range seriesBooks {
		seriesList = append(seriesList, series)
	}
	sort.Strings(seriesList)

	for _, seriesName := range seriesList {
		displayName := seriesName
		if displayName == "" {
			displayName = "No Series"
		}
		encodedSeries := strings.ReplaceAll(seriesName, " ", "%20")
		feed.AddNavigationEntry(
			displayName,
			"urn:webby:series:"+seriesName,
			baseURL+"/opds/v1.2/series/"+encodedSeries+".xml",
			"",
		)
	}

	xml, err := feed.ToXML()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate feed"})
		return
	}

	c.Data(http.StatusOK, opds.OPDSCatalogType, xml)
}

// OPDSSeriesBooks serves an acquisition feed of books in a specific series
func (h *Handler) OPDSSeriesBooks(c *gin.Context) {
	series := c.Param("series")
	series = strings.TrimSuffix(series, ".xml")

	userID := auth.GetUserID(c)
	baseURL := getBaseURL(c)
	selfURL := baseURL + "/opds/v1.2/series/" + strings.ReplaceAll(series, " ", "%20") + ".xml"
	startURL := baseURL + "/opds/v1.2/catalog.xml"

	books, err := h.db.ListBooksForUser(userID, "series_index", "asc")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list books"})
		return
	}

	displaySeries := series
	if displaySeries == "" {
		displaySeries = "No Series"
	}

	feed := opds.NewAcquisitionFeed(
		displaySeries+" Series",
		"urn:webby:series:"+series,
		selfURL,
		startURL,
	)

	for _, book := range books {
		if strings.EqualFold(book.Series, series) || (series == "" && book.Series == "") {
			feed.Entries = append(feed.Entries, opds.BookToEntry(&book, baseURL))
		}
	}

	xml, err := feed.ToXML()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate feed"})
		return
	}

	c.Data(http.StatusOK, opds.OPDSFeedType, xml)
}

// OPDSSearch serves the OpenSearch description document
func (h *Handler) OPDSSearch(c *gin.Context) {
	baseURL := getBaseURL(c)

	// Check if this is a search query
	query := c.Query("q")
	if query != "" {
		h.OPDSSearchResults(c, query)
		return
	}

	// Return OpenSearch description document
	searchURL := baseURL + "/opds/v1.2/search.xml"
	xml := opds.OpenSearchDescription(baseURL, searchURL)
	c.Data(http.StatusOK, opds.OPDSSearchType, []byte(xml))
}

// OPDSSearchResults serves search results as an acquisition feed
func (h *Handler) OPDSSearchResults(c *gin.Context, query string) {
	userID := auth.GetUserID(c)
	baseURL := getBaseURL(c)
	selfURL := baseURL + "/opds/v1.2/search.xml?q=" + strings.ReplaceAll(query, " ", "%20")
	startURL := baseURL + "/opds/v1.2/catalog.xml"

	books, err := h.db.ListBooksForUser(userID, "title", "asc")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list books"})
		return
	}

	feed := opds.NewAcquisitionFeed(
		"Search Results: "+query,
		"urn:webby:search:"+query,
		selfURL,
		startURL,
	)

	queryLower := strings.ToLower(query)
	for _, book := range books {
		// Search in title, author, series, and description
		if strings.Contains(strings.ToLower(book.Title), queryLower) ||
			strings.Contains(strings.ToLower(book.Author), queryLower) ||
			strings.Contains(strings.ToLower(book.Series), queryLower) ||
			strings.Contains(strings.ToLower(book.Description), queryLower) {
			feed.Entries = append(feed.Entries, opds.BookToEntry(&book, baseURL))
		}
	}

	xml, err := feed.ToXML()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate feed"})
		return
	}

	c.Data(http.StatusOK, opds.OPDSFeedType, xml)
}

// OPDSDownload serves a book file for download via OPDS
func (h *Handler) OPDSDownload(c *gin.Context) {
	bookID := c.Param("id")
	userID := auth.GetUserID(c)

	book, err := h.db.GetBookForUser(bookID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
		return
	}

	// Check if file exists
	bookPath := h.files.GetBookPath(bookID)
	if bookPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Set headers for download
	filename := book.Title
	if book.Author != "" {
		filename = book.Author + " - " + filename
	}
	filename = strings.ReplaceAll(filename, "/", "-")
	filename = strings.ReplaceAll(filename, "\\", "-")

	ext := filepath.Ext(book.FilePath)
	if ext == "" {
		ext = "." + book.FileFormat
	}

	c.Header("Content-Disposition", "attachment; filename=\""+filename+ext+"\"")
	c.Header("Content-Type", opds.GetMIMEType(book.FileFormat))
	c.File(book.FilePath)
}
