package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FileStorage handles file system operations for EPUBs
type FileStorage struct {
	basePath   string
	booksDir   string
	coversDir  string
}

// NewFileStorage creates a new file storage handler
func NewFileStorage(basePath string) (*FileStorage, error) {
	fs := &FileStorage{
		basePath:  basePath,
		booksDir:  filepath.Join(basePath, "books"),
		coversDir: filepath.Join(basePath, "covers"),
	}

	// Create directories if they don't exist
	if err := os.MkdirAll(fs.booksDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(fs.coversDir, 0755); err != nil {
		return nil, err
	}

	return fs, nil
}

// SaveBook saves an EPUB file and returns the file path
func (fs *FileStorage) SaveBook(id string, reader io.Reader) (string, error) {
	filePath := filepath.Join(fs.booksDir, id+".epub")

	file, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	if err != nil {
		os.Remove(filePath)
		return "", err
	}

	return filePath, nil
}

// SaveCover saves a cover image and returns the file path
func (fs *FileStorage) SaveCover(id string, data []byte, ext string) (string, error) {
	if ext == "" {
		ext = ".jpg"
	}
	filePath := filepath.Join(fs.coversDir, id+ext)

	err := os.WriteFile(filePath, data, 0644)
	if err != nil {
		return "", err
	}

	return filePath, nil
}

// GetBookPath returns the path to a book file
func (fs *FileStorage) GetBookPath(id string) string {
	return filepath.Join(fs.booksDir, id+".epub")
}

// GetCoverPath returns the path to a cover file
func (fs *FileStorage) GetCoverPath(id string) string {
	// Try common extensions
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif"} {
		path := filepath.Join(fs.coversDir, id+ext)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// DeleteBook removes a book file
func (fs *FileStorage) DeleteBook(id string) error {
	bookPath := fs.GetBookPath(id)
	if err := os.Remove(bookPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Also remove cover if exists
	coverPath := fs.GetCoverPath(id)
	if coverPath != "" {
		os.Remove(coverPath)
	}

	return nil
}

// OpenBook opens a book file for reading
func (fs *FileStorage) OpenBook(id string) (*os.File, error) {
	return os.Open(fs.GetBookPath(id))
}

// ReorganizedPaths contains the new file paths after reorganization
type ReorganizedPaths struct {
	BookPath  string
	CoverPath string
}

// ReorganizeBook moves a book to the correct folder structure based on metadata
// Structure: Author/Series/Title.epub or Author/Title.epub (if no series)
func (fs *FileStorage) ReorganizeBook(currentBookPath, currentCoverPath, author, series, title string) (*ReorganizedPaths, error) {
	// Sanitize names for filesystem
	author = sanitizeFileName(author)
	series = sanitizeFileName(series)
	title = sanitizeFileName(title)

	if author == "" {
		author = "Unknown Author"
	}
	if title == "" {
		title = "Unknown Title"
	}

	// Build directory path: Author/Series or just Author
	var dirPath string
	if series != "" {
		dirPath = filepath.Join(fs.booksDir, author, series)
	} else {
		dirPath = filepath.Join(fs.booksDir, author)
	}

	// Create directory structure
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, err
	}

	// Build new book path
	newBookPath := filepath.Join(dirPath, title+".epub")

	// Handle filename conflicts by adding a number suffix
	newBookPath = resolveConflict(newBookPath, currentBookPath)

	// Move the book file if paths are different
	if currentBookPath != newBookPath {
		if err := moveFile(currentBookPath, newBookPath); err != nil {
			return nil, err
		}
	}

	result := &ReorganizedPaths{
		BookPath: newBookPath,
	}

	// Move cover if it exists
	if currentCoverPath != "" {
		ext := filepath.Ext(currentCoverPath)
		newCoverPath := filepath.Join(dirPath, title+ext)
		newCoverPath = resolveConflict(newCoverPath, currentCoverPath)

		if currentCoverPath != newCoverPath {
			if err := moveFile(currentCoverPath, newCoverPath); err != nil {
				// Cover move failed, but book moved successfully - not fatal
				result.CoverPath = currentCoverPath
			} else {
				result.CoverPath = newCoverPath
			}
		} else {
			result.CoverPath = currentCoverPath
		}
	}

	// Clean up empty directories from old location
	cleanEmptyDirs(filepath.Dir(currentBookPath), fs.booksDir)

	return result, nil
}

// sanitizeFileName removes or replaces characters that are invalid in filenames
func sanitizeFileName(name string) string {
	if name == "" {
		return ""
	}

	// Replace characters that are problematic on various filesystems
	// Windows: \ / : * ? " < > |
	// Unix: /
	// Also replace control characters
	re := regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]`)
	name = re.ReplaceAllString(name, "_")

	// Replace multiple underscores/spaces with single space
	spaceRe := regexp.MustCompile(`[_\s]+`)
	name = spaceRe.ReplaceAllString(name, " ")

	// Trim leading/trailing spaces and dots (problematic on Windows)
	name = strings.Trim(name, " .")

	// Limit length to avoid filesystem issues (max 255 bytes, leave room for extension)
	if len(name) > 200 {
		name = name[:200]
	}

	return name
}

// resolveConflict adds a numeric suffix if the target path already exists
// and is not the same as the source
func resolveConflict(targetPath, sourcePath string) string {
	// If source and target are the same, no conflict
	if targetPath == sourcePath {
		return targetPath
	}

	// If target doesn't exist, no conflict
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return targetPath
	}

	// Add numeric suffix
	ext := filepath.Ext(targetPath)
	base := strings.TrimSuffix(targetPath, ext)

	for i := 2; i < 1000; i++ {
		newPath := base + fmt.Sprintf(" (%d)", i) + ext
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}

	return targetPath
}

// moveFile moves a file from src to dst, handling cross-device moves
func moveFile(src, dst string) error {
	// Try rename first (fast, same filesystem)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fall back to copy + delete for cross-device moves
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(dst)
		return err
	}

	// Close files before deleting source
	srcFile.Close()
	dstFile.Close()

	return os.Remove(src)
}

// cleanEmptyDirs removes empty directories up to the stop directory
func cleanEmptyDirs(dir, stopDir string) {
	for dir != stopDir && dir != "." && dir != "/" {
		// Try to remove directory (will fail if not empty)
		if err := os.Remove(dir); err != nil {
			break
		}
		dir = filepath.Dir(dir)
	}
}
