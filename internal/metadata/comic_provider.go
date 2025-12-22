package metadata

import (
	"context"
)

// ComicMetadata represents enriched comic information from external sources
type ComicMetadata struct {
	Title        string   `json:"title"`
	Series       string   `json:"series"`
	Volume       int      `json:"volume,omitempty"`
	IssueNumber  string   `json:"issue_number,omitempty"`
	Publisher    string   `json:"publisher,omitempty"`
	ReleaseDate  string   `json:"release_date,omitempty"`
	Description  string   `json:"description,omitempty"`
	Writers      []string `json:"writers,omitempty"`
	Artists      []string `json:"artists,omitempty"`
	CoverArtists []string `json:"cover_artists,omitempty"`
	Colorists    []string `json:"colorists,omitempty"`
	Genres       []string `json:"genres,omitempty"`
	CoverURL     string   `json:"cover_url,omitempty"`
	PageCount    int      `json:"page_count,omitempty"`
	Source       string   `json:"source"`
	SourceID     string   `json:"source_id,omitempty"` // External ID for future lookups
	Confidence   float64  `json:"confidence"`          // 0.0 - 1.0
}

// ComicProvider defines the interface for comic metadata lookup services
type ComicProvider interface {
	// Name returns the provider identifier (e.g., "comicvine")
	Name() string

	// SearchBySeriesAndIssue searches for a comic by series name and issue number
	SearchBySeriesAndIssue(ctx context.Context, series string, issueNumber string) ([]ComicMetadata, error)

	// SearchByTitle searches for comics matching a title
	SearchByTitle(ctx context.Context, title string) ([]ComicMetadata, error)

	// GetIssueDetails retrieves full details for a specific issue by source ID
	GetIssueDetails(ctx context.Context, sourceID string) (*ComicMetadata, error)
}
