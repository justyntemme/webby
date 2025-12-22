package pdf

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Metadata contains extracted PDF metadata
type Metadata struct {
	Title       string
	Author      string
	Subject     string
	Keywords    []string
	Creator     string
	Producer    string
	PageCount   int
	ContentType string // "book" or "comic"
}

// ParsePDF extracts metadata from a PDF file
func ParsePDF(filePath string) (*Metadata, error) {
	meta := &Metadata{
		Title:       extractTitleFromFilename(filePath),
		Author:      "Unknown",
		ContentType: "book",
	}

	// Open the file
	f, err := os.Open(filePath)
	if err != nil {
		return meta, nil // Return default metadata on error
	}
	defer f.Close()

	// Use PDFInfo to get metadata
	info, err := api.PDFInfo(f, filePath, nil, false, model.NewDefaultConfiguration())
	if err != nil {
		return meta, nil // Return default metadata on error
	}

	// Extract metadata from info
	if info.PageCount > 0 {
		meta.PageCount = info.PageCount
	}
	if info.Title != "" {
		meta.Title = info.Title
	}
	if info.Author != "" {
		meta.Author = info.Author
	}
	if info.Subject != "" {
		meta.Subject = info.Subject
	}
	if len(info.Keywords) > 0 {
		meta.Keywords = info.Keywords
	}
	if info.Creator != "" {
		meta.Creator = info.Creator
	}
	if info.Producer != "" {
		meta.Producer = info.Producer
	}

	// Detect content type
	meta.ContentType = detectPDFContentType(meta)

	return meta, nil
}

// ValidatePDF checks if a file is a valid PDF
func ValidatePDF(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	return api.Validate(f, model.NewDefaultConfiguration())
}

// GetPageCount returns the number of pages in a PDF
func GetPageCount(filePath string) (int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	info, err := api.PDFInfo(f, filePath, nil, false, model.NewDefaultConfiguration())
	if err != nil {
		return 0, err
	}

	return info.PageCount, nil
}

// extractTitleFromFilename extracts a title from the filename
func extractTitleFromFilename(filePath string) string {
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// detectPDFContentType determines if the PDF is a book or comic
func detectPDFContentType(meta *Metadata) string {
	comicTerms := []string{
		"comic", "comics", "graphic novel", "manga",
		"manhwa", "manhua", "sequential art",
	}

	// Check subject
	subjectLower := strings.ToLower(meta.Subject)
	for _, term := range comicTerms {
		if strings.Contains(subjectLower, term) {
			return "comic"
		}
	}

	// Check keywords
	for _, kw := range meta.Keywords {
		kwLower := strings.ToLower(kw)
		for _, term := range comicTerms {
			if strings.Contains(kwLower, term) {
				return "comic"
			}
		}
	}

	// Check title
	titleLower := strings.ToLower(meta.Title)
	if strings.Contains(titleLower, " vol.") ||
		strings.Contains(titleLower, " vol ") ||
		strings.Contains(titleLower, " issue ") ||
		strings.Contains(titleLower, " #") {
		for _, term := range comicTerms {
			if strings.Contains(titleLower, term) {
				return "comic"
			}
		}
	}

	return "book"
}

// CoverImage contains extracted cover image data
type CoverImage struct {
	Data      []byte
	Extension string // ".jpg", ".png", etc.
}

// ExtractCover attempts to extract a cover image from the first page of a PDF
// It tries to find embedded images on page 1 and returns the largest one
func ExtractCover(filePath string) (*CoverImage, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()

	// Extract images from first page only
	// Returns []map[int]model.Image - slice of maps (per page), indexed by object number
	pageMaps, err := api.ExtractImagesRaw(f, []string{"1"}, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to extract images: %w", err)
	}

	if len(pageMaps) == 0 {
		return nil, fmt.Errorf("no images found on first page")
	}

	// Find the largest image (likely the cover/main content)
	var bestImage *CoverImage
	var bestSize int

	for _, pageMap := range pageMaps {
		for _, img := range pageMap {
			// model.Image embeds io.Reader directly
			data, err := io.ReadAll(img)
			if err != nil {
				continue
			}

			size := len(data)
			if size > bestSize {
				bestSize = size
				ext := getImageExtension(img.FileType)
				bestImage = &CoverImage{
					Data:      data,
					Extension: ext,
				}
			}
		}
	}

	if bestImage == nil {
		return nil, fmt.Errorf("failed to read any images")
	}

	return bestImage, nil
}

// ExtractCoverFromReader extracts cover from a PDF provided as io.ReadSeeker
func ExtractCoverFromReader(r io.ReadSeeker) (*CoverImage, error) {
	conf := model.NewDefaultConfiguration()

	pageMaps, err := api.ExtractImagesRaw(r, []string{"1"}, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to extract images: %w", err)
	}

	if len(pageMaps) == 0 {
		return nil, fmt.Errorf("no images found on first page")
	}

	var bestImage *CoverImage
	var bestSize int

	for _, pageMap := range pageMaps {
		for _, img := range pageMap {
			data, err := io.ReadAll(img)
			if err != nil {
				continue
			}

			size := len(data)
			if size > bestSize {
				bestSize = size
				ext := getImageExtension(img.FileType)
				bestImage = &CoverImage{
					Data:      data,
					Extension: ext,
				}
			}
		}
	}

	if bestImage == nil {
		return nil, fmt.Errorf("failed to read any images")
	}

	return bestImage, nil
}

// getImageExtension returns the file extension for an image type
func getImageExtension(imageType string) string {
	switch strings.ToLower(imageType) {
	case "jpeg", "jpg":
		return ".jpg"
	case "png":
		return ".png"
	case "gif":
		return ".gif"
	case "tiff", "tif":
		return ".tiff"
	case "bmp":
		return ".bmp"
	case "webp":
		return ".webp"
	default:
		return ".jpg" // Default to jpg
	}
}

// HasImages checks if the first page of a PDF contains any images
func HasImages(filePath string) bool {
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	pageMaps, err := api.ExtractImagesRaw(f, []string{"1"}, conf)
	if err != nil {
		return false
	}

	// Check if any page has images
	for _, pageMap := range pageMaps {
		if len(pageMap) > 0 {
			return true
		}
	}
	return false
}

// UpdateMetadata writes metadata to a PDF file
func UpdateMetadata(filePath string, meta *Metadata) error {
	// Build properties map for pdfcpu
	properties := make(map[string]string)

	if meta.Title != "" {
		properties["Title"] = meta.Title
	}
	if meta.Author != "" {
		properties["Author"] = meta.Author
	}
	if meta.Subject != "" {
		properties["Subject"] = meta.Subject
	}
	if len(meta.Keywords) > 0 {
		properties["Keywords"] = strings.Join(meta.Keywords, ", ")
	}

	if len(properties) == 0 {
		return nil // Nothing to update
	}

	// pdfcpu AddPropertiesFile adds/updates properties
	// When outFile is empty, it modifies the file in place
	conf := model.NewDefaultConfiguration()
	return api.AddPropertiesFile(filePath, "", properties, conf)
}

// SetMetadata is an alias for UpdateMetadata for consistency with other packages
func SetMetadata(filePath, title, author, subject string, keywords []string) error {
	meta := &Metadata{
		Title:    title,
		Author:   author,
		Subject:  subject,
		Keywords: keywords,
	}
	return UpdateMetadata(filePath, meta)
}
