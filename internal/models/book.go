package models

import "time"

// User represents a registered user
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// Book represents an EPUB book in the library
type Book struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id,omitempty"`
	Title       string    `json:"title"`
	Author      string    `json:"author"`
	Series      string    `json:"series,omitempty"`
	SeriesIndex float64   `json:"series_index,omitempty"`
	FilePath    string    `json:"-"`
	CoverPath   string    `json:"-"`
	FileSize    int64     `json:"file_size"`
	UploadedAt  time.Time `json:"uploaded_at"`

	// Extended metadata fields
	ISBN            string     `json:"isbn,omitempty"`
	Publisher       string     `json:"publisher,omitempty"`
	PublishDate     string     `json:"publish_date,omitempty"`
	Description     string     `json:"description,omitempty"`
	Language        string     `json:"language,omitempty"`
	Subjects        string     `json:"subjects,omitempty"` // Comma-separated
	MetadataSource  string     `json:"metadata_source,omitempty"`
	MetadataUpdated *time.Time `json:"metadata_updated,omitempty"`
}

// Collection represents a user-defined collection of books
type Collection struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id,omitempty"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ReadingPosition tracks user's reading progress
type ReadingPosition struct {
	BookID    string    `json:"book_id"`
	UserID    string    `json:"user_id,omitempty"`
	Chapter   string    `json:"chapter"`
	Position  float64   `json:"position"` // Percentage through chapter
	UpdatedAt time.Time `json:"updated_at"`
}

// BookShare represents a book shared with another user
type BookShare struct {
	ID           string    `json:"id"`
	BookID       string    `json:"book_id"`
	OwnerID      string    `json:"owner_id"`
	SharedWithID string    `json:"shared_with_id"`
	CreatedAt    time.Time `json:"created_at"`
}
