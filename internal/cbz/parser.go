package cbz

import (
	"archive/zip"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nwaples/rardecode/v2"
)

// Metadata contains CBZ metadata
type Metadata struct {
	Title       string
	Author      string
	Series      string
	SeriesIndex float64
	PageCount   int
	ContentType string // Always "comic" for CBZ
}

// CoverImage contains extracted cover image data
type CoverImage struct {
	Data      []byte
	Extension string
}

// imageExtensions lists valid image file extensions
var imageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".bmp":  true,
}

// ParseCBZ parses a CBZ file and extracts metadata
func ParseCBZ(filePath string) (*Metadata, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CBZ: %w", err)
	}
	defer r.Close()

	meta := &Metadata{
		ContentType: "comic",
	}

	// Count image files for page count
	var imageFiles []string
	for _, f := range r.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if imageExtensions[ext] && !strings.HasPrefix(filepath.Base(f.Name), ".") {
			imageFiles = append(imageFiles, f.Name)
		}
	}
	meta.PageCount = len(imageFiles)

	// Try to extract title from filename
	baseName := filepath.Base(filePath)
	meta.Title = strings.TrimSuffix(baseName, filepath.Ext(baseName))

	// Try to parse series info from common naming patterns
	// e.g., "Series Name 001.cbz", "Series Name - 01.cbz", "Series Name #1.cbz"
	meta.Series, meta.SeriesIndex = parseSeriesFromFilename(meta.Title)

	// Look for ComicInfo.xml (standard comic metadata format)
	for _, f := range r.File {
		if strings.EqualFold(filepath.Base(f.Name), "ComicInfo.xml") {
			if info, err := parseComicInfo(f); err == nil {
				if info.Title != "" {
					meta.Title = info.Title
				}
				if info.Series != "" {
					meta.Series = info.Series
				}
				if info.Number > 0 {
					meta.SeriesIndex = info.Number
				}
				if info.Writer != "" {
					meta.Author = info.Writer
				}
			}
			break
		}
	}

	return meta, nil
}

// ValidateCBZ checks if a file is a valid CBZ archive
func ValidateCBZ(filePath string) error {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return fmt.Errorf("invalid CBZ file: %w", err)
	}
	defer r.Close()

	// Check if it contains at least one image
	for _, f := range r.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if imageExtensions[ext] {
			return nil
		}
	}

	return fmt.Errorf("CBZ file contains no images")
}

// ExtractCover extracts the first image from a CBZ as the cover
func ExtractCover(filePath string) (*CoverImage, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CBZ: %w", err)
	}
	defer r.Close()

	// Get sorted list of image files
	var imageFiles []*zip.File
	for _, f := range r.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if imageExtensions[ext] && !strings.HasPrefix(filepath.Base(f.Name), ".") {
			imageFiles = append(imageFiles, f)
		}
	}

	if len(imageFiles) == 0 {
		return nil, fmt.Errorf("no images found in CBZ")
	}

	// Sort by name to get the first page
	sort.Slice(imageFiles, func(i, j int) bool {
		return imageFiles[i].Name < imageFiles[j].Name
	})

	// Read the first image
	firstImage := imageFiles[0]
	rc, err := firstImage.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open image: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(firstImage.Name))
	return &CoverImage{
		Data:      data,
		Extension: ext,
	}, nil
}

// GetPageCount returns the number of image pages in a CBZ
func GetPageCount(filePath string) (int, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return 0, err
	}
	defer r.Close()

	count := 0
	for _, f := range r.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if imageExtensions[ext] && !strings.HasPrefix(filepath.Base(f.Name), ".") {
			count++
		}
	}

	return count, nil
}

// GetPage extracts a specific page (0-indexed) from a CBZ
func GetPage(filePath string, pageIndex int) ([]byte, string, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open CBZ: %w", err)
	}
	defer r.Close()

	// Get sorted list of image files
	var imageFiles []*zip.File
	for _, f := range r.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if imageExtensions[ext] && !strings.HasPrefix(filepath.Base(f.Name), ".") {
			imageFiles = append(imageFiles, f)
		}
	}

	if len(imageFiles) == 0 {
		return nil, "", fmt.Errorf("no images found in CBZ")
	}

	// Sort by name
	sort.Slice(imageFiles, func(i, j int) bool {
		return imageFiles[i].Name < imageFiles[j].Name
	})

	if pageIndex < 0 || pageIndex >= len(imageFiles) {
		return nil, "", fmt.Errorf("page index out of range: %d (total: %d)", pageIndex, len(imageFiles))
	}

	// Read the requested page
	pageFile := imageFiles[pageIndex]
	rc, err := pageFile.Open()
	if err != nil {
		return nil, "", fmt.Errorf("failed to open page: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read page: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(pageFile.Name))
	contentType := getImageContentType(ext)

	return data, contentType, nil
}

// GetPageList returns a list of page filenames in order
func GetPageList(filePath string) ([]string, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CBZ: %w", err)
	}
	defer r.Close()

	var pages []string
	for _, f := range r.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if imageExtensions[ext] && !strings.HasPrefix(filepath.Base(f.Name), ".") {
			pages = append(pages, f.Name)
		}
	}

	sort.Strings(pages)
	return pages, nil
}

// parseSeriesFromFilename tries to extract series name and index from filename
func parseSeriesFromFilename(filename string) (string, float64) {
	// Common patterns:
	// "Series Name 001" -> "Series Name", 1
	// "Series Name - 01" -> "Series Name", 1
	// "Series Name #1" -> "Series Name", 1
	// "Series Name v01" -> "Series Name", 1

	// Try to find a number at the end
	parts := strings.Fields(filename)
	if len(parts) < 2 {
		return "", 0
	}

	lastPart := parts[len(parts)-1]

	// Remove common prefixes
	lastPart = strings.TrimPrefix(lastPart, "#")
	lastPart = strings.TrimPrefix(lastPart, "v")
	lastPart = strings.TrimPrefix(lastPart, "V")

	// Try to parse as number
	var index float64
	if _, err := fmt.Sscanf(lastPart, "%f", &index); err == nil {
		// Found a number, the rest is the series name
		seriesName := strings.TrimSpace(strings.Join(parts[:len(parts)-1], " "))
		// Remove trailing dash or hyphen
		seriesName = strings.TrimSuffix(seriesName, "-")
		seriesName = strings.TrimSpace(seriesName)
		return seriesName, index
	}

	return "", 0
}

// getImageContentType returns the MIME type for an image extension
func getImageContentType(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return "application/octet-stream"
	}
}

// ComicInfo represents the ComicInfo.xml metadata format
type ComicInfo struct {
	Title  string
	Series string
	Number float64
	Writer string
}

// parseComicInfo parses ComicInfo.xml from a zip file entry
func parseComicInfo(f *zip.File) (*ComicInfo, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	// Simple XML parsing for common fields
	info := &ComicInfo{}
	content := string(data)

	info.Title = extractXMLValue(content, "Title")
	info.Series = extractXMLValue(content, "Series")
	info.Writer = extractXMLValue(content, "Writer")

	if numStr := extractXMLValue(content, "Number"); numStr != "" {
		fmt.Sscanf(numStr, "%f", &info.Number)
	}

	return info, nil
}

// extractXMLValue extracts a simple XML element value
func extractXMLValue(xml, tagName string) string {
	startTag := "<" + tagName + ">"
	endTag := "</" + tagName + ">"

	startIdx := strings.Index(xml, startTag)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(startTag)

	endIdx := strings.Index(xml[startIdx:], endTag)
	if endIdx == -1 {
		return ""
	}

	return strings.TrimSpace(xml[startIdx : startIdx+endIdx])
}

// ============================================
// CBR (RAR Archive) Support
// ============================================

// ParseCBR parses a CBR file and extracts metadata
func ParseCBR(filePath string) (*Metadata, error) {
	r, err := rardecode.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CBR: %w", err)
	}
	defer r.Close()

	meta := &Metadata{
		ContentType: "comic",
	}

	// Count image files for page count
	var imageFiles []string
	var comicInfoData []byte

	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CBR: %w", err)
		}

		ext := strings.ToLower(filepath.Ext(header.Name))
		baseName := filepath.Base(header.Name)

		// Check for ComicInfo.xml
		if strings.EqualFold(baseName, "ComicInfo.xml") {
			comicInfoData, _ = io.ReadAll(r)
		}

		if imageExtensions[ext] && !strings.HasPrefix(baseName, ".") {
			imageFiles = append(imageFiles, header.Name)
		}
	}

	meta.PageCount = len(imageFiles)

	// Try to extract title from filename
	baseName := filepath.Base(filePath)
	meta.Title = strings.TrimSuffix(baseName, filepath.Ext(baseName))

	// Try to parse series info from common naming patterns
	meta.Series, meta.SeriesIndex = parseSeriesFromFilename(meta.Title)

	// Parse ComicInfo.xml if found
	if comicInfoData != nil {
		if info := parseComicInfoData(comicInfoData); info != nil {
			if info.Title != "" {
				meta.Title = info.Title
			}
			if info.Series != "" {
				meta.Series = info.Series
			}
			if info.Number > 0 {
				meta.SeriesIndex = info.Number
			}
			if info.Writer != "" {
				meta.Author = info.Writer
			}
		}
	}

	return meta, nil
}

// ValidateCBR checks if a file is a valid CBR archive
func ValidateCBR(filePath string) error {
	r, err := rardecode.OpenReader(filePath)
	if err != nil {
		return fmt.Errorf("invalid CBR file: %w", err)
	}
	defer r.Close()

	// Check if it contains at least one image
	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read CBR: %w", err)
		}

		ext := strings.ToLower(filepath.Ext(header.Name))
		if imageExtensions[ext] {
			return nil
		}
	}

	return fmt.Errorf("CBR file contains no images")
}

// ExtractCoverCBR extracts the first image from a CBR as the cover
func ExtractCoverCBR(filePath string) (*CoverImage, error) {
	r, err := rardecode.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CBR: %w", err)
	}
	defer r.Close()

	// First pass: collect all image file names
	type imageEntry struct {
		name string
	}
	var imageFiles []imageEntry

	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CBR: %w", err)
		}

		ext := strings.ToLower(filepath.Ext(header.Name))
		if imageExtensions[ext] && !strings.HasPrefix(filepath.Base(header.Name), ".") {
			imageFiles = append(imageFiles, imageEntry{name: header.Name})
		}
	}

	if len(imageFiles) == 0 {
		return nil, fmt.Errorf("no images found in CBR")
	}

	// Sort by name to get the first page
	sort.Slice(imageFiles, func(i, j int) bool {
		return imageFiles[i].name < imageFiles[j].name
	})

	// Re-open and find the first image
	r2, err := rardecode.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to reopen CBR: %w", err)
	}
	defer r2.Close()

	targetName := imageFiles[0].name
	for {
		header, err := r2.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CBR: %w", err)
		}

		if header.Name == targetName {
			data, err := io.ReadAll(r2)
			if err != nil {
				return nil, fmt.Errorf("failed to read image: %w", err)
			}

			ext := strings.ToLower(filepath.Ext(header.Name))
			return &CoverImage{
				Data:      data,
				Extension: ext,
			}, nil
		}
	}

	return nil, fmt.Errorf("cover image not found")
}

// GetPageCountCBR returns the number of image pages in a CBR
func GetPageCountCBR(filePath string) (int, error) {
	r, err := rardecode.OpenReader(filePath)
	if err != nil {
		return 0, err
	}
	defer r.Close()

	count := 0
	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}

		ext := strings.ToLower(filepath.Ext(header.Name))
		if imageExtensions[ext] && !strings.HasPrefix(filepath.Base(header.Name), ".") {
			count++
		}
	}

	return count, nil
}

// GetPageCBR extracts a specific page (0-indexed) from a CBR
func GetPageCBR(filePath string, pageIndex int) ([]byte, string, error) {
	r, err := rardecode.OpenReader(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open CBR: %w", err)
	}
	defer r.Close()

	// First pass: collect all image file names
	var imageFiles []string

	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("failed to read CBR: %w", err)
		}

		ext := strings.ToLower(filepath.Ext(header.Name))
		if imageExtensions[ext] && !strings.HasPrefix(filepath.Base(header.Name), ".") {
			imageFiles = append(imageFiles, header.Name)
		}
	}

	if len(imageFiles) == 0 {
		return nil, "", fmt.Errorf("no images found in CBR")
	}

	// Sort by name
	sort.Strings(imageFiles)

	if pageIndex < 0 || pageIndex >= len(imageFiles) {
		return nil, "", fmt.Errorf("page index out of range: %d (total: %d)", pageIndex, len(imageFiles))
	}

	// Re-open and find the target page
	r2, err := rardecode.OpenReader(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to reopen CBR: %w", err)
	}
	defer r2.Close()

	targetName := imageFiles[pageIndex]
	for {
		header, err := r2.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("failed to read CBR: %w", err)
		}

		if header.Name == targetName {
			data, err := io.ReadAll(r2)
			if err != nil {
				return nil, "", fmt.Errorf("failed to read page: %w", err)
			}

			ext := strings.ToLower(filepath.Ext(header.Name))
			contentType := getImageContentType(ext)
			return data, contentType, nil
		}
	}

	return nil, "", fmt.Errorf("page not found")
}

// GetPageListCBR returns a list of page filenames in order
func GetPageListCBR(filePath string) ([]string, error) {
	r, err := rardecode.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CBR: %w", err)
	}
	defer r.Close()

	var pages []string
	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CBR: %w", err)
		}

		ext := strings.ToLower(filepath.Ext(header.Name))
		if imageExtensions[ext] && !strings.HasPrefix(filepath.Base(header.Name), ".") {
			pages = append(pages, header.Name)
		}
	}

	sort.Strings(pages)
	return pages, nil
}

// parseComicInfoData parses ComicInfo.xml from raw bytes
func parseComicInfoData(data []byte) *ComicInfo {
	info := &ComicInfo{}
	content := string(data)

	info.Title = extractXMLValue(content, "Title")
	info.Series = extractXMLValue(content, "Series")
	info.Writer = extractXMLValue(content, "Writer")

	if numStr := extractXMLValue(content, "Number"); numStr != "" {
		fmt.Sscanf(numStr, "%f", &info.Number)
	}

	return info
}
