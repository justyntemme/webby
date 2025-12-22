package epub

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// UpdateMetadata updates the metadata inside an EPUB file
func UpdateMetadata(filePath string, meta *Metadata) error {
	// Read the original EPUB
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return fmt.Errorf("failed to open epub: %w", err)
	}
	defer r.Close()

	// Find the OPF path from container.xml
	containerFile, err := findFile(&r.Reader, "META-INF/container.xml")
	if err != nil {
		return fmt.Errorf("failed to find container.xml: %w", err)
	}

	container := &Container{}
	if err := parseXML(containerFile, container); err != nil {
		return fmt.Errorf("failed to parse container.xml: %w", err)
	}

	if len(container.RootFiles) == 0 {
		return fmt.Errorf("no rootfile found in container.xml")
	}

	opfPath := container.RootFiles[0].FullPath

	// Read the original OPF content
	opfFile, err := findFile(&r.Reader, opfPath)
	if err != nil {
		return fmt.Errorf("failed to find OPF file: %w", err)
	}
	opfContent, err := io.ReadAll(opfFile)
	if err != nil {
		return fmt.Errorf("failed to read OPF file: %w", err)
	}

	// Update OPF content with new metadata
	updatedOPF := updateOPFContent(string(opfContent), meta)

	// Create a temporary file for the new EPUB
	tmpFile, err := os.CreateTemp("", "epub-update-*.epub")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Create new ZIP writer
	w := zip.NewWriter(tmpFile)

	// Copy all files, replacing the OPF
	for _, f := range r.File {
		var content []byte

		if f.Name == opfPath {
			// Use updated OPF content
			content = []byte(updatedOPF)
		} else {
			// Copy original file
			rc, err := f.Open()
			if err != nil {
				w.Close()
				tmpFile.Close()
				os.Remove(tmpPath)
				return fmt.Errorf("failed to open file %s: %w", f.Name, err)
			}
			content, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				w.Close()
				tmpFile.Close()
				os.Remove(tmpPath)
				return fmt.Errorf("failed to read file %s: %w", f.Name, err)
			}
		}

		// Create new file in ZIP with same header
		header := &zip.FileHeader{
			Name:   f.Name,
			Method: f.Method,
		}
		header.SetModTime(f.Modified)

		writer, err := w.CreateHeader(header)
		if err != nil {
			w.Close()
			tmpFile.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("failed to create file %s: %w", f.Name, err)
		}

		if _, err := writer.Write(content); err != nil {
			w.Close()
			tmpFile.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("failed to write file %s: %w", f.Name, err)
		}
	}

	if err := w.Close(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close zip writer: %w", err)
	}
	tmpFile.Close()

	// Replace original file with updated one
	if err := os.Rename(tmpPath, filePath); err != nil {
		// If rename fails (cross-device), try copy
		if err := copyFile(tmpPath, filePath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("failed to replace original file: %w", err)
		}
		os.Remove(tmpPath)
	}

	return nil
}

// updateOPFContent modifies the OPF XML with new metadata values
func updateOPFContent(opf string, meta *Metadata) string {
	// Update title
	if meta.Title != "" {
		opf = replaceOrInsertDCElement(opf, "title", meta.Title)
	}

	// Update creator/author
	if meta.Author != "" {
		opf = replaceOrInsertDCElement(opf, "creator", meta.Author)
	}

	// Update identifier with ISBN
	if meta.ISBN != "" {
		opf = updateISBNIdentifier(opf, meta.ISBN)
	}

	// Update publisher
	if meta.Publisher != "" {
		opf = replaceOrInsertDCElement(opf, "publisher", meta.Publisher)
	}

	// Update language
	if meta.Language != "" {
		opf = replaceOrInsertDCElement(opf, "language", meta.Language)
	}

	// Update date
	if meta.PublishDate != "" {
		opf = replaceOrInsertDCElement(opf, "date", meta.PublishDate)
	}

	// Update description
	if meta.Description != "" {
		opf = replaceOrInsertDCElement(opf, "description", escapeXML(meta.Description))
	}

	// Update series (Calibre format)
	if meta.Series != "" {
		opf = updateCalibreMeta(opf, "calibre:series", meta.Series)
		opf = updateCalibreMeta(opf, "calibre:series_index", fmt.Sprintf("%.1f", meta.SeriesIndex))
	}

	// Update subjects
	if len(meta.Subjects) > 0 {
		opf = updateSubjects(opf, meta.Subjects)
	}

	return opf
}

// replaceOrInsertDCElement replaces or inserts a Dublin Core element
func replaceOrInsertDCElement(opf, element, value string) string {
	// Pattern to match existing element (with or without namespace prefix)
	patterns := []string{
		fmt.Sprintf(`(?i)(<dc:%s[^>]*>)[^<]*(</dc:%s>)`, element, element),
		fmt.Sprintf(`(?i)(<%s[^>]*>)[^<]*(</%s>)`, element, element),
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if re.MatchString(opf) {
			return re.ReplaceAllString(opf, "${1}"+value+"${2}")
		}
	}

	// Element doesn't exist, insert it after <metadata...>
	metaRe := regexp.MustCompile(`(?i)(<metadata[^>]*>)`)
	if metaRe.MatchString(opf) {
		replacement := fmt.Sprintf("${1}\n    <dc:%s>%s</dc:%s>", element, value, element)
		return metaRe.ReplaceAllString(opf, replacement)
	}

	return opf
}

// updateISBNIdentifier updates or adds ISBN identifier
func updateISBNIdentifier(opf, isbn string) string {
	// Try to find existing ISBN identifier
	isbnRe := regexp.MustCompile(`(?i)(<dc:identifier[^>]*scheme=["']ISBN["'][^>]*>)[^<]*(</dc:identifier>)`)
	if isbnRe.MatchString(opf) {
		return isbnRe.ReplaceAllString(opf, "${1}"+isbn+"${2}")
	}

	// Try URN:ISBN format
	urnRe := regexp.MustCompile(`(?i)(<dc:identifier[^>]*>)urn:isbn:[^<]*(</dc:identifier>)`)
	if urnRe.MatchString(opf) {
		return urnRe.ReplaceAllString(opf, "${1}urn:isbn:"+isbn+"${2}")
	}

	// Insert new ISBN identifier
	metaRe := regexp.MustCompile(`(?i)(<metadata[^>]*>)`)
	if metaRe.MatchString(opf) {
		replacement := fmt.Sprintf("${1}\n    <dc:identifier scheme=\"ISBN\">%s</dc:identifier>", isbn)
		return metaRe.ReplaceAllString(opf, replacement)
	}

	return opf
}

// updateCalibreMeta updates or adds Calibre-style meta tags
func updateCalibreMeta(opf, name, value string) string {
	// Pattern to match existing meta
	pattern := fmt.Sprintf(`(?i)(<meta\s+name=["']%s["']\s+content=["'])[^"']*(["'][^>]*/>)`, regexp.QuoteMeta(name))
	re := regexp.MustCompile(pattern)
	if re.MatchString(opf) {
		return re.ReplaceAllString(opf, "${1}"+value+"${2}")
	}

	// Also try with attributes in different order
	pattern2 := fmt.Sprintf(`(?i)(<meta\s+content=["'])[^"']*(["']\s+name=["']%s["'][^>]*/>)`, regexp.QuoteMeta(name))
	re2 := regexp.MustCompile(pattern2)
	if re2.MatchString(opf) {
		return re2.ReplaceAllString(opf, "${1}"+value+"${2}")
	}

	// Insert new meta before </metadata>
	endMetaRe := regexp.MustCompile(`(?i)(</metadata>)`)
	if endMetaRe.MatchString(opf) {
		replacement := fmt.Sprintf("    <meta name=\"%s\" content=\"%s\"/>\n  ${1}", name, value)
		return endMetaRe.ReplaceAllString(opf, replacement)
	}

	return opf
}

// updateSubjects replaces all subject elements
func updateSubjects(opf string, subjects []string) string {
	// Remove existing subjects
	subjectRe := regexp.MustCompile(`(?i)\s*<dc:subject[^>]*>[^<]*</dc:subject>`)
	opf = subjectRe.ReplaceAllString(opf, "")

	// Insert new subjects before </metadata>
	var subjectTags bytes.Buffer
	for _, subj := range subjects {
		subjectTags.WriteString(fmt.Sprintf("    <dc:subject>%s</dc:subject>\n", escapeXML(subj)))
	}

	endMetaRe := regexp.MustCompile(`(?i)(</metadata>)`)
	if endMetaRe.MatchString(opf) {
		return endMetaRe.ReplaceAllString(opf, subjectTags.String()+"  ${1}")
	}

	return opf
}

// escapeXML escapes special XML characters
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	dest, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dest.Close()

	_, err = io.Copy(dest, source)
	return err
}
