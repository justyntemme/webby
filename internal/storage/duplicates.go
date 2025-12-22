package storage

import (
	"log"
	"os"
	"sync"

	"github.com/justyntemme/webby/internal/models"
)

// DuplicateService handles duplicate detection and management
type DuplicateService struct {
	db    *Database
	files *FileStorage
	mu    sync.Mutex
}

// NewDuplicateService creates a new duplicate detection service
func NewDuplicateService(db *Database, files *FileStorage) *DuplicateService {
	return &DuplicateService{
		db:    db,
		files: files,
	}
}

// DuplicateCheckResult contains the result of checking for duplicates
type DuplicateCheckResult struct {
	IsDuplicate bool
	FileHash    string
	Duplicates  []models.Book
}

// CheckForDuplicate checks if a file would be a duplicate before upload
func (s *DuplicateService) CheckForDuplicate(filePath, userID string) (*DuplicateCheckResult, error) {
	hash, err := HashFile(filePath)
	if err != nil {
		return nil, err
	}

	existingBooks, err := s.db.GetBooksByHash(hash)
	if err != nil {
		return nil, err
	}

	// Filter to user's books if userID is provided
	var userBooks []models.Book
	for _, book := range existingBooks {
		if userID == "" || book.UserID == userID {
			userBooks = append(userBooks, book)
		}
	}

	return &DuplicateCheckResult{
		IsDuplicate: len(userBooks) > 0,
		FileHash:    hash,
		Duplicates:  userBooks,
	}, nil
}

// ComputeHashForBook computes and stores the hash for a single book
func (s *DuplicateService) ComputeHashForBook(book *models.Book) (string, error) {
	// Check if file exists
	if _, err := os.Stat(book.FilePath); os.IsNotExist(err) {
		return "", err
	}

	hash, err := HashFile(book.FilePath)
	if err != nil {
		return "", err
	}

	// Update the database
	if err := s.db.UpdateBookFileHash(book.ID, hash); err != nil {
		return "", err
	}

	return hash, nil
}

// HashProgress tracks the progress of bulk hash computation
type HashProgress struct {
	Total     int `json:"total"`
	Processed int `json:"processed"`
	Failed    int `json:"failed"`
}

// ComputeMissingHashes computes hashes for books that don't have them
// Returns progress channel and error channel
func (s *DuplicateService) ComputeMissingHashes(userID string, batchSize int) (*HashProgress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get count first
	total, err := s.db.CountBooksWithoutHash(userID)
	if err != nil {
		return nil, err
	}

	progress := &HashProgress{
		Total:     total,
		Processed: 0,
		Failed:    0,
	}

	if total == 0 {
		return progress, nil
	}

	// Process in batches
	for {
		books, err := s.db.GetBooksWithoutHash(userID, batchSize)
		if err != nil {
			return progress, err
		}

		if len(books) == 0 {
			break
		}

		for _, book := range books {
			_, err := s.ComputeHashForBook(&book)
			if err != nil {
				log.Printf("Failed to compute hash for book %s: %v", book.ID, err)
				progress.Failed++
			} else {
				progress.Processed++
			}
		}
	}

	return progress, nil
}

// FindDuplicates returns all duplicate groups for a user
func (s *DuplicateService) FindDuplicates(userID string) ([]DuplicateGroup, error) {
	return s.db.FindDuplicateBooks(userID)
}

// MergeResult contains the result of merging duplicates
type MergeResult struct {
	KeptBook     *models.Book `json:"kept_book"`
	DeletedBooks []string     `json:"deleted_books"`
	FilesRemoved int          `json:"files_removed"`
}

// MergeDuplicates keeps one book and deletes the others
// keepBookID is the ID of the book to keep, others in the group are deleted
func (s *DuplicateService) MergeDuplicates(keepBookID string, deleteBookIDs []string, userID string) (*MergeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get the book to keep
	keptBook, err := s.db.GetBook(keepBookID)
	if err != nil {
		return nil, err
	}

	// Verify ownership if userID is provided
	if userID != "" && keptBook.UserID != userID {
		return nil, ErrNotOwner
	}

	result := &MergeResult{
		KeptBook:     keptBook,
		DeletedBooks: make([]string, 0),
		FilesRemoved: 0,
	}

	for _, bookID := range deleteBookIDs {
		if bookID == keepBookID {
			continue // Don't delete the book we're keeping
		}

		book, err := s.db.GetBook(bookID)
		if err != nil {
			log.Printf("Failed to get book %s for deletion: %v", bookID, err)
			continue
		}

		// Verify ownership
		if userID != "" && book.UserID != userID {
			log.Printf("Cannot delete book %s: not owner", bookID)
			continue
		}

		// Verify same hash
		if book.FileHash != keptBook.FileHash {
			log.Printf("Book %s has different hash, skipping", bookID)
			continue
		}

		// Delete from database first
		if err := s.db.DeleteBook(bookID); err != nil {
			log.Printf("Failed to delete book %s from database: %v", bookID, err)
			continue
		}

		// Delete files directly since we have the full paths
		filesDeleted := 0
		if book.FilePath != "" {
			if err := os.Remove(book.FilePath); err != nil && !os.IsNotExist(err) {
				log.Printf("Failed to delete book file for %s: %v", bookID, err)
			} else {
				filesDeleted++
			}
		}
		if book.CoverPath != "" {
			if err := os.Remove(book.CoverPath); err != nil && !os.IsNotExist(err) {
				log.Printf("Failed to delete cover file for %s: %v", bookID, err)
			} else {
				filesDeleted++
			}
		}
		if filesDeleted > 0 {
			result.FilesRemoved++
		}

		result.DeletedBooks = append(result.DeletedBooks, bookID)
	}

	return result, nil
}

// Error types for duplicate service
var (
	ErrNotOwner = &DuplicateError{Message: "not the owner of this book"}
)

// DuplicateError represents a duplicate service error
type DuplicateError struct {
	Message string
}

func (e *DuplicateError) Error() string {
	return e.Message
}
