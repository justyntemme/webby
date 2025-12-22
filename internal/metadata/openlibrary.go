package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OpenLibraryProvider implements the Provider interface for Open Library API
type OpenLibraryProvider struct {
	client  *http.Client
	baseURL string
}

// NewOpenLibraryProvider creates a new Open Library provider
func NewOpenLibraryProvider() *OpenLibraryProvider {
	return &OpenLibraryProvider{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: "https://openlibrary.org",
	}
}

// Name returns the provider identifier
func (p *OpenLibraryProvider) Name() string {
	return "openlibrary"
}

// olEdition represents an Open Library edition response
type olEdition struct {
	Title       string   `json:"title"`
	Authors     []olRef  `json:"authors"`
	Publishers  []string `json:"publishers"`
	PublishDate string   `json:"publish_date"`
	ISBN10      []string `json:"isbn_10"`
	ISBN13      []string `json:"isbn_13"`
	Covers      []int    `json:"covers"`
	NumberPages int      `json:"number_of_pages"`
	Subjects    []string `json:"subjects"`
	Description any      `json:"description"` // Can be string or {type, value}
}

// olRef represents a reference to another Open Library entity
type olRef struct {
	Key string `json:"key"`
}

// olSearchResponse represents an Open Library search response
type olSearchResponse struct {
	NumFound int           `json:"numFound"`
	Docs     []olSearchDoc `json:"docs"`
}

// olSearchDoc represents a document in search results
type olSearchDoc struct {
	Key              string   `json:"key"`
	Title            string   `json:"title"`
	AuthorName       []string `json:"author_name"`
	Publisher        []string `json:"publisher"`
	FirstPublishYear int      `json:"first_publish_year"`
	ISBN             []string `json:"isbn"`
	CoverI           int      `json:"cover_i"`
	Subject          []string `json:"subject"`
}

// LookupByISBN searches for a book by ISBN
func (p *OpenLibraryProvider) LookupByISBN(ctx context.Context, isbn string) (*BookMetadata, error) {
	// Normalize ISBN (remove hyphens and spaces)
	isbn = normalizeISBN(isbn)
	if isbn == "" {
		return nil, ErrNoMatch
	}

	url := fmt.Sprintf("%s/isbn/%s.json", p.baseURL, isbn)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, ErrNoMatch
	}
	if resp.StatusCode == 429 {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var edition olEdition
	if err := json.NewDecoder(resp.Body).Decode(&edition); err != nil {
		return nil, err
	}

	return p.convertEdition(&edition, isbn), nil
}

// Search finds books matching title and optional author
func (p *OpenLibraryProvider) Search(ctx context.Context, title, author string) ([]BookMetadata, error) {
	params := url.Values{}
	params.Set("title", title)
	if author != "" {
		params.Set("author", author)
	}
	params.Set("limit", "5")
	params.Set("fields", "key,title,author_name,publisher,first_publish_year,isbn,cover_i,subject")

	searchURL := fmt.Sprintf("%s/search.json?%s", p.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var data olSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if data.NumFound == 0 {
		return nil, ErrNoMatch
	}

	var results []BookMetadata
	for _, doc := range data.Docs {
		results = append(results, p.convertSearchDoc(&doc))
	}
	return results, nil
}

// GetCoverURL returns URL for book cover image
func (p *OpenLibraryProvider) GetCoverURL(isbn string, size CoverSize) string {
	isbn = normalizeISBN(isbn)
	if isbn == "" {
		return ""
	}
	return fmt.Sprintf("https://covers.openlibrary.org/b/isbn/%s-%s.jpg", isbn, size)
}

// convertEdition converts an Open Library edition to BookMetadata
func (p *OpenLibraryProvider) convertEdition(e *olEdition, isbn string) *BookMetadata {
	meta := &BookMetadata{
		Title:       e.Title,
		Publisher:   firstOrEmpty(e.Publishers),
		PublishDate: e.PublishDate,
		PageCount:   e.NumberPages,
		Subjects:    e.Subjects,
		Source:      p.Name(),
		Confidence:  1.0, // ISBN match is exact
	}

	// Set ISBNs
	if len(e.ISBN10) > 0 {
		meta.ISBN10 = e.ISBN10[0]
	}
	if len(e.ISBN13) > 0 {
		meta.ISBN13 = e.ISBN13[0]
	}

	// Handle description (can be string or object)
	switch desc := e.Description.(type) {
	case string:
		meta.Description = desc
	case map[string]any:
		if val, ok := desc["value"].(string); ok {
			meta.Description = val
		}
	}

	// Set cover URL if available
	if len(e.Covers) > 0 {
		meta.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-M.jpg", e.Covers[0])
	} else if isbn != "" {
		meta.CoverURL = p.GetCoverURL(isbn, CoverMedium)
	}

	return meta
}

// convertSearchDoc converts a search result to BookMetadata
func (p *OpenLibraryProvider) convertSearchDoc(doc *olSearchDoc) BookMetadata {
	meta := BookMetadata{
		Title:   doc.Title,
		Authors: doc.AuthorName,
		Source:  p.Name(),
	}

	if len(doc.Publisher) > 0 {
		meta.Publisher = doc.Publisher[0]
	}

	if doc.FirstPublishYear > 0 {
		meta.PublishDate = fmt.Sprintf("%d", doc.FirstPublishYear)
	}

	// Extract ISBNs
	for _, isbn := range doc.ISBN {
		normalized := normalizeISBN(isbn)
		if len(normalized) == 10 && meta.ISBN10 == "" {
			meta.ISBN10 = normalized
		} else if len(normalized) == 13 && meta.ISBN13 == "" {
			meta.ISBN13 = normalized
		}
	}

	// Limit subjects
	if len(doc.Subject) > 5 {
		meta.Subjects = doc.Subject[:5]
	} else {
		meta.Subjects = doc.Subject
	}

	// Set cover URL
	if doc.CoverI > 0 {
		meta.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-M.jpg", doc.CoverI)
	} else if meta.ISBN13 != "" {
		meta.CoverURL = p.GetCoverURL(meta.ISBN13, CoverMedium)
	} else if meta.ISBN10 != "" {
		meta.CoverURL = p.GetCoverURL(meta.ISBN10, CoverMedium)
	}

	return meta
}

// normalizeISBN removes hyphens and spaces from ISBN
func normalizeISBN(isbn string) string {
	isbn = strings.ReplaceAll(isbn, "-", "")
	isbn = strings.ReplaceAll(isbn, " ", "")
	// Handle URN format
	isbn = strings.TrimPrefix(strings.ToLower(isbn), "urn:isbn:")
	return strings.TrimSpace(isbn)
}

// firstOrEmpty returns the first element or empty string
func firstOrEmpty(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}
