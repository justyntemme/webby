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

// Metadata contains extracted EPUB metadata
type Metadata struct {
	Title       string
	Author      string
	Series      string
	SeriesIndex float64
	CoverData   []byte
	CoverExt    string
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

	return meta, nil
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
