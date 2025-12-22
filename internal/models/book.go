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

// ContentType constants for books vs comics
const (
	ContentTypeBook  = "book"
	ContentTypeComic = "comic"
)

// ReadStatus constants for tracking reading progress
const (
	ReadStatusUnread    = "unread"
	ReadStatusReading   = "reading"
	ReadStatusCompleted = "completed"
)

// FileFormat constants for different file types
const (
	FileFormatEPUB = "epub"
	FileFormatPDF  = "pdf"
	FileFormatCBZ  = "cbz"
	FileFormatCBR  = "cbr"
)

// Book represents a book in the library (EPUB, PDF, or CBZ)
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
	ContentType string    `json:"content_type"`  // "book" or "comic"
	FileFormat  string    `json:"file_format"`   // "epub", "pdf", or "cbz"

	// File hash for duplicate detection
	FileHash string `json:"file_hash,omitempty"`

	// Extended metadata fields
	ISBN            string     `json:"isbn,omitempty"`
	Publisher       string     `json:"publisher,omitempty"`
	PublishDate     string     `json:"publish_date,omitempty"`
	Description     string     `json:"description,omitempty"`
	Language        string     `json:"language,omitempty"`
	Subjects        string     `json:"subjects,omitempty"` // Comma-separated
	MetadataSource  string     `json:"metadata_source,omitempty"`
	MetadataUpdated *time.Time `json:"metadata_updated,omitempty"`

	// Reading status tracking
	ReadStatus    string     `json:"read_status"`              // "unread", "reading", "completed"
	DateCompleted *time.Time `json:"date_completed,omitempty"` // When book was marked completed

	// Star rating (0-5, 0 means no rating)
	Rating int `json:"rating"`
}

// Collection represents a user-defined collection of books
type Collection struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id,omitempty"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`

	// Smart collection fields
	IsSmart   bool              `json:"is_smart"`
	RuleLogic string            `json:"rule_logic,omitempty"` // "AND" or "OR"
	Rules     []CollectionRule  `json:"rules,omitempty"`
	BookCount int               `json:"book_count,omitempty"`
}

// Rule field constants for smart collections
const (
	RuleFieldAuthor      = "author"
	RuleFieldTitle       = "title"
	RuleFieldFormat      = "format"
	RuleFieldYear        = "year"
	RuleFieldSeries      = "series"
	RuleFieldTags        = "tags"
	RuleFieldRating      = "rating"
	RuleFieldReadStatus  = "read_status"
	RuleFieldFileSize    = "file_size"
	RuleFieldContentType = "content_type"
)

// Rule operator constants
const (
	RuleOpEquals      = "equals"
	RuleOpContains    = "contains"
	RuleOpStartsWith  = "starts_with"
	RuleOpGreaterThan = "greater_than"
	RuleOpLessThan    = "less_than"
	RuleOpBetween     = "between"
	RuleOpIn          = "in" // for multi-value fields like tags
)

// CollectionRule defines a single rule for smart collections
type CollectionRule struct {
	ID           string `json:"id"`
	CollectionID string `json:"collection_id"`
	Field        string `json:"field"`    // author, title, format, year, series, tags, rating, read_status, file_size
	Operator     string `json:"operator"` // equals, contains, starts_with, greater_than, less_than, between, in
	Value        string `json:"value"`    // The value to match (JSON for complex values like ranges)
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

// ReadingListType constants for predefined list types
const (
	ReadingListWantToRead = "want_to_read"
	ReadingListFavorites  = "favorites"
	ReadingListCustom     = "custom"
)

// ReadingList represents a user's reading list
type ReadingList struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	ListType  string    `json:"list_type"` // "want_to_read", "favorites", or "custom"
	CreatedAt time.Time `json:"created_at"`
	BookCount int       `json:"book_count,omitempty"`
}

// ReadingListBook represents a book in a reading list
type ReadingListBook struct {
	BookID    string    `json:"book_id"`
	ListID    string    `json:"list_id"`
	AddedAt   time.Time `json:"added_at"`
	Position  int       `json:"position"` // For ordering within the list
}

// Tag represents a custom user-defined tag for organizing books
type Tag struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"created_at"`
	BookCount int       `json:"book_count,omitempty"`
}

// BookTag represents a book's association with a tag
type BookTag struct {
	BookID  string    `json:"book_id"`
	TagID   string    `json:"tag_id"`
	AddedAt time.Time `json:"added_at"`
}

// HighlightColor constants for highlight colors
const (
	HighlightColorYellow = "yellow"
	HighlightColorGreen  = "green"
	HighlightColorBlue   = "blue"
	HighlightColorPink   = "pink"
	HighlightColorOrange = "orange"
)

// Annotation represents a highlight or note on a book
type Annotation struct {
	ID            string    `json:"id"`
	BookID        string    `json:"book_id"`
	UserID        string    `json:"user_id"`
	Chapter       string    `json:"chapter"`                  // Chapter/section identifier
	CFI           string    `json:"cfi,omitempty"`            // EPUB CFI for precise location
	StartOffset   int       `json:"start_offset"`             // Character offset start
	EndOffset     int       `json:"end_offset"`               // Character offset end
	SelectedText  string    `json:"selected_text"`            // The highlighted text
	Note          string    `json:"note,omitempty"`           // User's note/comment
	Color         string    `json:"color"`                    // Highlight color
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
