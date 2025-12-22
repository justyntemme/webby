package epub

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Content type constants
const (
	ContentTypeBook  = "book"
	ContentTypeComic = "comic"
)

// Metadata contains extracted EPUB metadata
type Metadata struct {
	Title       string
	Author      string
	Series      string
	SeriesIndex float64
	CoverData   []byte
	CoverExt    string
	ContentType string // "book" or "comic"

	// Extended metadata fields
	ISBN        string
	Description string
	Publisher   string
	Language    string
	PublishDate string
	Subjects    []string
}

// Container represents the META-INF/container.xml structure
type Container struct {
	XMLName   xml.Name `xml:"container"`
	RootFiles []struct {
		FullPath  string `xml:"full-path,attr"`
		MediaType string `xml:"media-type,attr"`
	} `xml:"rootfiles>rootfile"`
}

// Package represents the OPF package document
type Package struct {
	XMLName  xml.Name `xml:"package"`
	Metadata struct {
		Title   []string `xml:"title"`
		Creator []struct {
			Value string `xml:",chardata"`
			Role  string `xml:"role,attr"`
		} `xml:"creator"`
		Meta []struct {
			Name     string `xml:"name,attr"`
			Content  string `xml:"content,attr"`
			Property string `xml:"property,attr"`
			Refines  string `xml:"refines,attr"`
			Value    string `xml:",chardata"`
		} `xml:"meta"`
		// Dublin Core elements
		Identifier []struct {
			Value  string `xml:",chardata"`
			Scheme string `xml:"scheme,attr"`
			ID     string `xml:"id,attr"`
		} `xml:"identifier"`
		Description []string `xml:"description"`
		Publisher   []string `xml:"publisher"`
		Language    []string `xml:"language"`
		Date        []string `xml:"date"`
		Subject     []string `xml:"subject"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		Items []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

// ParseEPUB extracts metadata from an EPUB file
func ParseEPUB(filePath string) (*Metadata, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	// Find container.xml
	containerFile, err := findFile(&r.Reader, "META-INF/container.xml")
	if err != nil {
		return nil, err
	}

	container := &Container{}
	if err := parseXML(containerFile, container); err != nil {
		return nil, err
	}

	if len(container.RootFiles) == 0 {
		return &Metadata{Title: "Unknown", Author: "Unknown"}, nil
	}

	// Parse OPF file
	opfPath := container.RootFiles[0].FullPath
	opfFile, err := findFile(&r.Reader, opfPath)
	if err != nil {
		return nil, err
	}

	pkg := &Package{}
	if err := parseXML(opfFile, pkg); err != nil {
		return nil, err
	}

	meta := &Metadata{
		Title:  "Unknown",
		Author: "Unknown",
	}

	// Extract title
	if len(pkg.Metadata.Title) > 0 {
		meta.Title = strings.TrimSpace(pkg.Metadata.Title[0])
	}

	// Extract author
	for _, creator := range pkg.Metadata.Creator {
		if creator.Value != "" {
			meta.Author = strings.TrimSpace(creator.Value)
			break
		}
	}

	// Extract ISBN from identifiers
	for _, ident := range pkg.Metadata.Identifier {
		value := strings.TrimSpace(ident.Value)
		scheme := strings.ToUpper(ident.Scheme)

		// Check for ISBN scheme or pattern in value
		if scheme == "ISBN" || strings.HasPrefix(strings.ToUpper(value), "URN:ISBN:") {
			meta.ISBN = normalizeISBN(value)
			break
		}
		// Look for ISBN pattern in the value itself
		if isbn := extractISBN(value); isbn != "" && meta.ISBN == "" {
			meta.ISBN = isbn
		}
	}

	// Extract description
	if len(pkg.Metadata.Description) > 0 {
		// Clean HTML from description
		desc := strings.TrimSpace(pkg.Metadata.Description[0])
		meta.Description = StripHTML(desc)
	}

	// Extract publisher
	if len(pkg.Metadata.Publisher) > 0 {
		meta.Publisher = strings.TrimSpace(pkg.Metadata.Publisher[0])
	}

	// Extract language
	if len(pkg.Metadata.Language) > 0 {
		meta.Language = strings.TrimSpace(pkg.Metadata.Language[0])
	}

	// Extract publish date
	if len(pkg.Metadata.Date) > 0 {
		meta.PublishDate = strings.TrimSpace(pkg.Metadata.Date[0])
	}

	// Extract subjects
	for _, subj := range pkg.Metadata.Subject {
		if trimmed := strings.TrimSpace(subj); trimmed != "" {
			meta.Subjects = append(meta.Subjects, trimmed)
		}
	}

	// Extract series info from meta tags (Calibre format)
	for _, m := range pkg.Metadata.Meta {
		switch m.Name {
		case "calibre:series":
			meta.Series = m.Content
		case "calibre:series_index":
			if idx, err := strconv.ParseFloat(m.Content, 64); err == nil {
				meta.SeriesIndex = idx
			}
		}
		// EPUB 3 format
		if m.Property == "belongs-to-collection" {
			meta.Series = m.Value
		}
		if m.Property == "group-position" {
			if idx, err := strconv.ParseFloat(m.Value, 64); err == nil {
				meta.SeriesIndex = idx
			}
		}
	}

	// Extract cover
	coverID := findCoverID(pkg)
	if coverID != "" {
		opfDir := path.Dir(opfPath)
		for _, item := range pkg.Manifest.Items {
			if item.ID == coverID {
				coverPath := item.Href
				if opfDir != "." {
					coverPath = path.Join(opfDir, coverPath)
				}
				if coverFile, err := findFile(&r.Reader, coverPath); err == nil {
					meta.CoverData, _ = io.ReadAll(coverFile)
					meta.CoverExt = path.Ext(coverPath)
				}
				break
			}
		}
	}

	// Detect content type (book vs comic)
	meta.ContentType = detectContentType(pkg, meta)

	return meta, nil
}

// detectContentType determines if the EPUB is a book or comic
func detectContentType(pkg *Package, meta *Metadata) string {
	// Check subjects for comic-related terms
	comicTerms := []string{
		"comic", "comics", "graphic novel", "graphic novels",
		"manga", "manhwa", "manhua", "bande dessinée",
		"sequential art", "comic book", "comic books",
	}

	for _, subj := range meta.Subjects {
		subjLower := strings.ToLower(subj)
		for _, term := range comicTerms {
			if strings.Contains(subjLower, term) {
				return ContentTypeComic
			}
		}
	}

	// Check Calibre tags and custom metadata
	for _, m := range pkg.Metadata.Meta {
		// Check for calibre:user_categories or tags containing comic terms
		if m.Name == "calibre:user_categories" || m.Name == "calibre:tags" {
			contentLower := strings.ToLower(m.Content)
			for _, term := range comicTerms {
				if strings.Contains(contentLower, term) {
					return ContentTypeComic
				}
			}
		}
	}

	// Check publisher for common comic publishers
	comicPublishers := []string{
		"marvel", "dc comics", "dark horse", "image comics",
		"idw", "boom! studios", "dynamite", "valiant",
		"oni press", "viz media", "kodansha", "shueisha",
		"seven seas", "yen press", "tokyopop",
	}

	if meta.Publisher != "" {
		pubLower := strings.ToLower(meta.Publisher)
		for _, pub := range comicPublishers {
			if strings.Contains(pubLower, pub) {
				return ContentTypeComic
			}
		}
	}

	// Check title for common comic patterns
	titleLower := strings.ToLower(meta.Title)
	if strings.Contains(titleLower, " vol.") ||
		strings.Contains(titleLower, " vol ") ||
		strings.Contains(titleLower, " issue ") ||
		strings.Contains(titleLower, " #") {
		// Could be a comic, but also could be a book series
		// Check for high image ratio as additional signal
		imageCount := 0
		totalItems := 0
		for _, item := range pkg.Manifest.Items {
			if strings.HasPrefix(item.MediaType, "image/") {
				imageCount++
			}
			// Count content items (not just any item)
			if strings.HasPrefix(item.MediaType, "application/xhtml") ||
				strings.HasPrefix(item.MediaType, "text/html") ||
				strings.HasPrefix(item.MediaType, "image/") {
				totalItems++
			}
		}

		// If more than 70% of content items are images, likely a comic
		if totalItems > 0 && float64(imageCount)/float64(totalItems) > 0.7 {
			return ContentTypeComic
		}
	}

	// Check for very high image-to-spine ratio (image-heavy EPUBs)
	imageCount := 0
	for _, item := range pkg.Manifest.Items {
		if strings.HasPrefix(item.MediaType, "image/") {
			imageCount++
		}
	}
	spineCount := len(pkg.Spine.Items)

	// If there are many images per spine item, likely a comic (images embedded per page)
	if spineCount > 0 && imageCount > spineCount*2 {
		// Check subjects more loosely
		for _, subj := range meta.Subjects {
			subjLower := strings.ToLower(subj)
			if strings.Contains(subjLower, "fiction") ||
				strings.Contains(subjLower, "novel") {
				// Has fiction/novel subject, probably not a comic
				return ContentTypeBook
			}
		}
		// High image ratio without fiction subjects, could be comic
		if float64(imageCount)/float64(spineCount) > 5 {
			return ContentTypeComic
		}
	}

	return ContentTypeBook
}

// GetTableOfContents returns the book's table of contents
func GetTableOfContents(filePath string) ([]Chapter, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	// Find container.xml
	containerFile, err := findFile(&r.Reader, "META-INF/container.xml")
	if err != nil {
		return nil, err
	}

	container := &Container{}
	if err := parseXML(containerFile, container); err != nil {
		return nil, err
	}

	if len(container.RootFiles) == 0 {
		return nil, nil
	}

	// Parse OPF file
	opfPath := container.RootFiles[0].FullPath
	opfFile, err := findFile(&r.Reader, opfPath)
	if err != nil {
		return nil, err
	}

	pkg := &Package{}
	if err := parseXML(opfFile, pkg); err != nil {
		return nil, err
	}

	// Build manifest lookup
	manifest := make(map[string]string)
	for _, item := range pkg.Manifest.Items {
		manifest[item.ID] = item.Href
	}

	// Build chapters from spine
	opfDir := path.Dir(opfPath)
	var chapters []Chapter
	for i, item := range pkg.Spine.Items {
		href := manifest[item.IDRef]
		if href == "" {
			continue
		}

		fullPath := href
		if opfDir != "." {
			fullPath = path.Join(opfDir, href)
		}

		chapters = append(chapters, Chapter{
			Index: i,
			ID:    item.IDRef,
			Href:  fullPath,
			Title: extractChapterTitle(&r.Reader, fullPath, i),
		})
	}

	return chapters, nil
}

// Chapter represents a chapter in the EPUB
type Chapter struct {
	Index int    `json:"index"`
	ID    string `json:"id"`
	Href  string `json:"href"`
	Title string `json:"title"`
}

// GetChapterContent returns the HTML content of a specific chapter
func GetChapterContent(filePath string, chapterIndex int) (string, error) {
	chapters, err := GetTableOfContents(filePath)
	if err != nil {
		return "", err
	}

	if chapterIndex < 0 || chapterIndex >= len(chapters) {
		return "", nil
	}

	r, err := zip.OpenReader(filePath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	chapter := chapters[chapterIndex]
	file, err := findFile(&r.Reader, chapter.Href)
	if err != nil {
		return "", err
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func findFile(r *zip.Reader, name string) (io.ReadCloser, error) {
	for _, f := range r.File {
		if f.Name == name || strings.EqualFold(f.Name, name) {
			return f.Open()
		}
	}
	return nil, os.ErrNotExist
}

func parseXML(r io.Reader, v interface{}) error {
	decoder := xml.NewDecoder(r)
	return decoder.Decode(v)
}

func findCoverID(pkg *Package) string {
	// Check meta for cover
	for _, m := range pkg.Metadata.Meta {
		if m.Name == "cover" {
			return m.Content
		}
	}

	// Check manifest for cover-image property (EPUB 3)
	for _, item := range pkg.Manifest.Items {
		if strings.Contains(item.Properties, "cover-image") {
			return item.ID
		}
	}

	// Look for common cover IDs
	for _, item := range pkg.Manifest.Items {
		if strings.Contains(strings.ToLower(item.ID), "cover") &&
			strings.HasPrefix(item.MediaType, "image/") {
			return item.ID
		}
	}

	return ""
}

func extractChapterTitle(r *zip.Reader, href string, fallbackIndex int) string {
	file, err := findFile(r, href)
	if err != nil {
		return "Chapter " + strconv.Itoa(fallbackIndex+1)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "Chapter " + strconv.Itoa(fallbackIndex+1)
	}

	// Try to extract title from HTML
	titleRe := regexp.MustCompile(`<title[^>]*>([^<]+)</title>`)
	if matches := titleRe.FindSubmatch(content); len(matches) > 1 {
		title := strings.TrimSpace(string(matches[1]))
		if title != "" && title != "Unknown" {
			return title
		}
	}

	// Try h1
	h1Re := regexp.MustCompile(`<h1[^>]*>([^<]+)</h1>`)
	if matches := h1Re.FindSubmatch(content); len(matches) > 1 {
		title := strings.TrimSpace(string(matches[1]))
		if title != "" {
			return title
		}
	}

	return "Chapter " + strconv.Itoa(fallbackIndex+1)
}

// normalizeISBN cleans an ISBN string
func normalizeISBN(isbn string) string {
	// Remove URN prefix if present
	isbn = strings.TrimPrefix(strings.ToLower(isbn), "urn:isbn:")
	isbn = strings.ToUpper(isbn)
	// Remove hyphens, spaces, and other separators
	isbn = strings.ReplaceAll(isbn, "-", "")
	isbn = strings.ReplaceAll(isbn, " ", "")
	isbn = strings.ReplaceAll(isbn, ".", "")
	return strings.TrimSpace(isbn)
}

// extractISBN attempts to find an ISBN pattern in a string
func extractISBN(s string) string {
	// ISBN-13 pattern: 978 or 979 prefix followed by 10 digits
	isbn13Re := regexp.MustCompile(`(?:978|979)[-\s]?(?:\d[-\s]?){9}\d`)
	if match := isbn13Re.FindString(s); match != "" {
		return normalizeISBN(match)
	}

	// ISBN-10 pattern: 9 digits followed by digit or X
	isbn10Re := regexp.MustCompile(`\d[-\s]?(?:\d[-\s]?){8}[\dXx]`)
	if match := isbn10Re.FindString(s); match != "" {
		return normalizeISBN(match)
	}

	return ""
}

// ValidateEPUB checks if a file is a valid EPUB
func ValidateEPUB(filePath string) error {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return err
	}
	defer r.Close()

	// Check for required files
	_, err = findFile(&r.Reader, "META-INF/container.xml")
	if err != nil {
		return err
	}

	return nil
}

// GetChapterText returns the plain text content of a specific chapter (HTML stripped)
func GetChapterText(filePath string, chapterIndex int) (string, error) {
	html, err := GetChapterContent(filePath, chapterIndex)
	if err != nil {
		return "", err
	}
	return StripHTML(html), nil
}

// StripHTML removes HTML tags and returns plain text
func StripHTML(html string) string {
	// Remove script and style elements entirely
	scriptRe := regexp.MustCompile(`(?i)<script[^>]*>[\s\S]*?</script>`)
	html = scriptRe.ReplaceAllString(html, "")
	styleRe := regexp.MustCompile(`(?i)<style[^>]*>[\s\S]*?</style>`)
	html = styleRe.ReplaceAllString(html, "")

	// Replace block elements with newlines
	blockRe := regexp.MustCompile(`(?i)</(p|div|br|h[1-6]|li|tr)>`)
	html = blockRe.ReplaceAllString(html, "\n")
	brRe := regexp.MustCompile(`(?i)<br\s*/?>`)
	html = brRe.ReplaceAllString(html, "\n")

	// Remove all remaining HTML tags
	tagRe := regexp.MustCompile(`<[^>]+>`)
	html = tagRe.ReplaceAllString(html, "")

	// Decode common HTML entities
	html = strings.ReplaceAll(html, "&nbsp;", " ")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&apos;", "'")

	// Clean up whitespace
	spaceRe := regexp.MustCompile(`[ \t]+`)
	html = spaceRe.ReplaceAllString(html, " ")
	newlineRe := regexp.MustCompile(`\n\s*\n+`)
	html = newlineRe.ReplaceAllString(html, "\n\n")

	return strings.TrimSpace(html)
}
