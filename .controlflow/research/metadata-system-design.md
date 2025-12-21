# Technical Design: Metadata Lookup System

**Story:** STORY-024
**Status:** Complete
**Date:** 2025-12-21

## Overview

This document defines the architecture for enriching book metadata in Webby using external APIs. The system will automatically lookup missing metadata on upload and allow manual refresh.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              FRONTEND (index.html)                          │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────┐  ┌────────────────┐  │
│  │ Upload Book │  │ Edit Metadata│  │ Refresh Single │  │ Bulk Refresh   │  │
│  └──────┬──────┘  └──────┬───────┘  └───────┬────────┘  └───────┬────────┘  │
└─────────┼────────────────┼──────────────────┼───────────────────┼───────────┘
          │                │                  │                   │
          ▼                ▼                  ▼                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              API LAYER (Gin)                                │
├─────────────────────────────────────────────────────────────────────────────┤
│  POST /api/books           PUT /api/books/:id/metadata                      │
│  POST /api/books/:id/lookup-metadata                                        │
│  POST /api/books/:id/cover                                                  │
│  POST /api/books/bulk-refresh                                               │
│  GET  /api/jobs/:id                                                         │
└─────────────────────────────────────────────────────────────────────────────┘
          │                                                       │
          ▼                                                       ▼
┌─────────────────────────────┐               ┌───────────────────────────────┐
│     Metadata Service        │               │       Job Queue               │
│  ┌───────────────────────┐  │               │  ┌─────────────────────────┐  │
│  │ MetadataProvider      │  │               │  │ BulkRefreshJob          │  │
│  │ interface             │  │               │  │ - Progress tracking     │  │
│  ├───────────────────────┤  │               │  │ - Cancellation          │  │
│  │ OpenLibraryProvider   │  │               │  │ - Rate limiting         │  │
│  │ GoogleBooksProvider   │  │               │  └─────────────────────────┘  │
│  └───────────────────────┘  │               └───────────────────────────────┘
└──────────────┬──────────────┘
               │
               ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              EXTERNAL APIs                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────────────┐              ┌──────────────────────────┐          │
│  │  Open Library       │              │  Google Books            │          │
│  │  (Primary)          │              │  (Fallback)              │          │
│  │  - /search.json     │              │  - /v1/volumes           │          │
│  │  - /isbn/{ISBN}     │              │  - /v1/volumes?q=isbn:   │          │
│  │  - covers API       │              │                          │          │
│  └─────────────────────┘              └──────────────────────────┘          │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Database Schema Changes

### Migration SQL

```sql
-- Add new columns to books table
ALTER TABLE books ADD COLUMN isbn TEXT DEFAULT '';
ALTER TABLE books ADD COLUMN publisher TEXT DEFAULT '';
ALTER TABLE books ADD COLUMN publish_date TEXT DEFAULT '';
ALTER TABLE books ADD COLUMN description TEXT DEFAULT '';
ALTER TABLE books ADD COLUMN language TEXT DEFAULT '';
ALTER TABLE books ADD COLUMN subjects TEXT DEFAULT '';
ALTER TABLE books ADD COLUMN metadata_source TEXT DEFAULT 'epub';
ALTER TABLE books ADD COLUMN metadata_updated_at DATETIME;

-- Add index for ISBN lookups
CREATE INDEX IF NOT EXISTS idx_books_isbn ON books(isbn);
```

### Updated Book Model

```go
// internal/models/book.go
type Book struct {
    ID                string    `json:"id"`
    UserID            string    `json:"user_id,omitempty"`
    Title             string    `json:"title"`
    Author            string    `json:"author"`
    Series            string    `json:"series,omitempty"`
    SeriesIndex       float64   `json:"series_index,omitempty"`
    FilePath          string    `json:"-"`
    CoverPath         string    `json:"-"`
    FileSize          int64     `json:"file_size"`
    UploadedAt        time.Time `json:"uploaded_at"`

    // New metadata fields
    ISBN              string    `json:"isbn,omitempty"`
    Publisher         string    `json:"publisher,omitempty"`
    PublishDate       string    `json:"publish_date,omitempty"`
    Description       string    `json:"description,omitempty"`
    Language          string    `json:"language,omitempty"`
    Subjects          string    `json:"subjects,omitempty"`  // Comma-separated
    MetadataSource    string    `json:"metadata_source,omitempty"`
    MetadataUpdatedAt *time.Time `json:"metadata_updated_at,omitempty"`
}
```

## Go Interface Definitions

### Metadata Provider Interface

```go
// internal/metadata/provider.go
package metadata

import "context"

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
    Publisher   string   `json:"publisher"`
    PublishDate string   `json:"publish_date"`
    Description string   `json:"description"`
    ISBN10      string   `json:"isbn_10,omitempty"`
    ISBN13      string   `json:"isbn_13,omitempty"`
    PageCount   int      `json:"page_count,omitempty"`
    Subjects    []string `json:"subjects,omitempty"`
    CoverURL    string   `json:"cover_url,omitempty"`
    Language    string   `json:"language,omitempty"`
    Source      string   `json:"source"`
    Confidence  float64  `json:"confidence"`  // 0.0 - 1.0
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
```

### Metadata Service

```go
// internal/metadata/service.go
package metadata

import (
    "context"
    "time"
)

// Service orchestrates metadata lookups across providers
type Service struct {
    primary   Provider
    fallback  Provider
    rateLimit *RateLimiter
}

// NewService creates a metadata service with primary and fallback providers
func NewService(primary, fallback Provider) *Service {
    return &Service{
        primary:   primary,
        fallback:  fallback,
        rateLimit: NewRateLimiter(500 * time.Millisecond),
    }
}

// LookupBook attempts to find metadata using ISBN first, then title/author
func (s *Service) LookupBook(ctx context.Context, isbn, title, author string) (*BookMetadata, error) {
    s.rateLimit.Wait()

    // Try ISBN lookup first (most accurate)
    if isbn != "" {
        if result, err := s.primary.LookupByISBN(ctx, isbn); err == nil && result != nil {
            result.Confidence = 1.0  // Exact ISBN match
            return result, nil
        }
        // Try fallback
        if s.fallback != nil {
            if result, err := s.fallback.LookupByISBN(ctx, isbn); err == nil && result != nil {
                result.Confidence = 1.0
                return result, nil
            }
        }
    }

    // Fall back to title/author search
    if title != "" {
        results, err := s.primary.Search(ctx, title, author)
        if err == nil && len(results) > 0 {
            return s.selectBestMatch(results, title, author), nil
        }
        // Try fallback
        if s.fallback != nil {
            results, err = s.fallback.Search(ctx, title, author)
            if err == nil && len(results) > 0 {
                return s.selectBestMatch(results, title, author), nil
            }
        }
    }

    return nil, ErrNoMatch
}

// selectBestMatch calculates confidence scores and returns best result
func (s *Service) selectBestMatch(results []BookMetadata, title, author string) *BookMetadata {
    var best *BookMetadata
    var bestScore float64

    for i := range results {
        score := s.calculateConfidence(&results[i], title, author)
        results[i].Confidence = score
        if score > bestScore {
            bestScore = score
            best = &results[i]
        }
    }
    return best
}

// calculateConfidence computes match confidence based on title/author similarity
func (s *Service) calculateConfidence(meta *BookMetadata, title, author string) float64 {
    titleScore := stringSimilarity(normalize(meta.Title), normalize(title))
    authorScore := 0.0
    if author != "" && len(meta.Authors) > 0 {
        authorScore = stringSimilarity(normalize(meta.Authors[0]), normalize(author))
    }
    // Weight: 60% title, 40% author
    return titleScore*0.6 + authorScore*0.4
}
```

### Open Library Provider

```go
// internal/metadata/openlibrary.go
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

type OpenLibraryProvider struct {
    client  *http.Client
    baseURL string
}

func NewOpenLibraryProvider() *OpenLibraryProvider {
    return &OpenLibraryProvider{
        client: &http.Client{
            Timeout: 10 * time.Second,
        },
        baseURL: "https://openlibrary.org",
    }
}

func (p *OpenLibraryProvider) Name() string {
    return "openlibrary"
}

func (p *OpenLibraryProvider) LookupByISBN(ctx context.Context, isbn string) (*BookMetadata, error) {
    // Normalize ISBN (remove hyphens)
    isbn = strings.ReplaceAll(isbn, "-", "")

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
        return nil, nil  // Not found
    }
    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }

    var data olEdition
    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
        return nil, err
    }

    return p.convertEdition(&data), nil
}

func (p *OpenLibraryProvider) Search(ctx context.Context, title, author string) ([]BookMetadata, error) {
    params := url.Values{}
    params.Set("title", title)
    if author != "" {
        params.Set("author", author)
    }
    params.Set("limit", "5")
    params.Set("fields", "key,title,author_name,publisher,first_publish_year,cover_i,isbn")

    url := fmt.Sprintf("%s/search.json?%s", p.baseURL, params.Encode())
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    resp, err := p.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var data olSearchResponse
    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
        return nil, err
    }

    var results []BookMetadata
    for _, doc := range data.Docs {
        results = append(results, p.convertSearchDoc(&doc))
    }
    return results, nil
}

func (p *OpenLibraryProvider) GetCoverURL(isbn string, size CoverSize) string {
    return fmt.Sprintf("https://covers.openlibrary.org/b/isbn/%s-%s.jpg", isbn, size)
}
```

## API Endpoint Contracts

### POST /api/books/:id/lookup-metadata

Triggers metadata lookup for a specific book.

**Request:**
```json
{
    "force": false  // If true, ignores existing metadata
}
```

**Response (200 OK):**
```json
{
    "found": true,
    "metadata": {
        "title": "The Great Gatsby",
        "authors": ["F. Scott Fitzgerald"],
        "publisher": "Scribner",
        "publish_date": "1925",
        "description": "A novel about the decadence...",
        "isbn_13": "9780743273565",
        "subjects": ["Fiction", "Classics"],
        "cover_url": "https://covers.openlibrary.org/b/isbn/9780743273565-M.jpg",
        "source": "openlibrary",
        "confidence": 0.95
    },
    "message": "Metadata found with 95% confidence"
}
```

**Response (404 Not Found):**
```json
{
    "found": false,
    "message": "No matching metadata found"
}
```

### PUT /api/books/:id/metadata

Updates book metadata (manual edit).

**Request:**
```json
{
    "title": "Updated Title",
    "author": "Updated Author",
    "publisher": "Updated Publisher",
    "publish_date": "2020",
    "description": "Updated description...",
    "series": "Series Name",
    "series_index": 1
}
```

**Response (200 OK):**
```json
{
    "message": "Metadata updated",
    "book": { ... }
}
```

### POST /api/books/:id/cover

Uploads custom cover image.

**Request:** `multipart/form-data` with `cover` file

**Response (200 OK):**
```json
{
    "message": "Cover updated",
    "cover_url": "/api/books/{id}/cover"
}
```

### POST /api/books/bulk-refresh

Starts bulk metadata refresh job.

**Request:**
```json
{
    "book_ids": ["id1", "id2"],  // Optional, empty = all books
    "force": false,              // Refresh even if metadata exists
    "incomplete_only": true      // Only books missing key fields
}
```

**Response (202 Accepted):**
```json
{
    "job_id": "job-uuid",
    "total_books": 50,
    "message": "Bulk refresh started"
}
```

### GET /api/jobs/:id

Gets job status.

**Response (200 OK):**
```json
{
    "id": "job-uuid",
    "status": "running",  // "pending", "running", "completed", "failed", "cancelled"
    "progress": {
        "total": 50,
        "completed": 25,
        "failed": 2,
        "current_book": "Book Title"
    },
    "started_at": "2025-12-21T15:00:00Z",
    "completed_at": null
}
```

## Data Flow: Upload to Metadata Enrichment

```
┌──────────────────────────────────────────────────────────────────┐
│ 1. User uploads EPUB                                             │
└───────────────────────────────┬──────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ 2. Parse EPUB metadata (existing)                                │
│    - Extract: title, author, series, cover                       │
│    - NEW: Extract ISBN from dc:identifier                        │
│    - NEW: Extract description from dc:description                │
└───────────────────────────────┬──────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ 3. Save book to database                                         │
│    - All extracted metadata                                      │
│    - metadata_source = 'epub'                                    │
└───────────────────────────────┬──────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ 4. Check metadata completeness                                   │
│    - Has ISBN? Has description? Has publisher?                   │
│    - If incomplete → trigger async lookup                        │
└───────────────────────────────┬──────────────────────────────────┘
                                │ (async goroutine)
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ 5. Metadata Service lookup                                       │
│    a. Try Open Library by ISBN                                   │
│    b. If no ISBN, search by title + author                       │
│    c. If Open Library fails, try Google Books                    │
│    d. Calculate confidence score                                 │
└───────────────────────────────┬──────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ 6. Handle result                                                 │
│    - If confidence >= 0.8: Auto-apply metadata                   │
│    - If confidence < 0.8: Flag for user review                   │
│    - Update metadata_source = 'openlibrary'/'googlebooks'        │
│    - Download and save cover if better quality                   │
└──────────────────────────────────────────────────────────────────┘
```

## Error Handling Strategy

| Error | Retry | Fallback | User Notification |
|-------|-------|----------|-------------------|
| Network timeout | 1x with backoff | Switch provider | Silent, log warning |
| 429 Rate Limited | Wait + exponential backoff | Switch provider | Silent for single, show for bulk |
| 404 Not Found | No | Try search instead of ISBN | "No match found" |
| 500 Server Error | 2x with backoff | Switch provider | "Service unavailable" |
| Low confidence match | No | N/A | Show preview for confirmation |

### Retry with Exponential Backoff

```go
func withRetry(fn func() error, maxAttempts int) error {
    var lastErr error
    for attempt := 0; attempt < maxAttempts; attempt++ {
        if err := fn(); err != nil {
            lastErr = err
            if attempt < maxAttempts-1 {
                time.Sleep(time.Duration(1<<attempt) * time.Second)
            }
            continue
        }
        return nil
    }
    return lastErr
}
```

## File Structure

```
internal/
├── metadata/
│   ├── provider.go          # Interface definitions
│   ├── service.go           # Orchestration logic
│   ├── openlibrary.go       # Open Library implementation
│   ├── googlebooks.go       # Google Books implementation
│   ├── matching.go          # Confidence calculation
│   ├── ratelimit.go         # Rate limiting
│   └── errors.go            # Error types
├── jobs/
│   ├── queue.go             # In-memory job queue
│   ├── bulk_refresh.go      # Bulk refresh job
│   └── status.go            # Job status tracking
```

## Configuration

```go
// Environment variables
WEBBY_METADATA_ENABLED=true              // Enable/disable auto-lookup
WEBBY_METADATA_AUTO_THRESHOLD=0.8        // Auto-apply confidence threshold
WEBBY_METADATA_RATE_LIMIT_MS=500         // Delay between API calls
WEBBY_GOOGLE_BOOKS_API_KEY=              // Optional: Google Books API key
```

## Implementation Order

1. **Phase 1: Database & Parser** (STORY-017 fix + schema)
   - Fix author sorting bug
   - Add new columns to books table
   - Extend EPUB parser to extract ISBN, description

2. **Phase 2: Provider Interface** (STORY-018 core)
   - Implement MetadataProvider interface
   - Implement OpenLibraryProvider
   - Add rate limiting

3. **Phase 3: Manual Editing** (STORY-019)
   - PUT /api/books/:id/metadata endpoint
   - POST /api/books/:id/cover endpoint
   - Edit UI modal

4. **Phase 4: Auto-lookup** (STORY-018 continued)
   - Metadata service orchestration
   - POST /api/books/:id/lookup-metadata
   - Integration with upload flow

5. **Phase 5: Bulk Operations** (STORY-020)
   - Job queue implementation
   - POST /api/books/bulk-refresh
   - Progress tracking UI
