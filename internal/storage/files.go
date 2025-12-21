package storage

import (
	"io"
	"os"
	"path/filepath"
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
