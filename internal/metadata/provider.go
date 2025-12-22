package metadata

import (
	"context"
	"errors"
)

// Common errors
var (
	ErrNoMatch      = errors.New("no matching metadata found")
	ErrRateLimited  = errors.New("rate limited by provider")
	ErrProviderDown = errors.New("metadata provider unavailable")
)

// CoverSize represents cover image size options
type CoverSize string

const (
	CoverSmall  CoverSize = "S"
	CoverMedium CoverSize = "M"
	CoverLarge  CoverSize = "L"
)

// BookMetadata represents enriched book information from external sources
type BookMetadata struct {
	Title       string   `json:"title"`
	Authors     []string `json:"authors"`
	Publisher   string   `json:"publisher,omitempty"`
	PublishDate string   `json:"publish_date,omitempty"`
	Description string   `json:"description,omitempty"`
	ISBN10      string   `json:"isbn_10,omitempty"`
	ISBN13      string   `json:"isbn_13,omitempty"`
	PageCount   int      `json:"page_count,omitempty"`
	Subjects    []string `json:"subjects,omitempty"`
	CoverURL    string   `json:"cover_url,omitempty"`
	Language    string   `json:"language,omitempty"`
	Source      string   `json:"source"`
	Confidence  float64  `json:"confidence"` // 0.0 - 1.0
}

// Provider defines the interface for metadata lookup services
type Provider interface {
	// Name returns the provider identifier (e.g., "openlibrary", "googlebooks")
	Name() string

	// LookupByISBN searches for a book by ISBN (10 or 13)
	LookupByISBN(ctx context.Context, isbn string) (*BookMetadata, error)

	// Search finds books matching title and optional author
	Search(ctx context.Context, title, author string) ([]BookMetadata, error)

	// GetCoverURL returns URL for book cover image
	GetCoverURL(isbn string, size CoverSize) string
}
