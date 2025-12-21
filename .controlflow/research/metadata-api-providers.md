# Research: Metadata API Providers Evaluation

**Story:** STORY-022
**Status:** Complete
**Date:** 2025-12-21

## Summary

Evaluation of free book metadata APIs for enriching EPUB library data. Recommendation: Use **Open Library** as primary provider with **Google Books** as fallback.

## Provider Comparison Matrix

| Provider | Free | Auth Required | Rate Limit | Database Size | Covers | ISBN Lookup |
|----------|------|---------------|------------|---------------|--------|-------------|
| **Open Library** | Yes | No | 100/5min (covers) | ~30M titles | Yes | Yes |
| **Google Books** | Yes | API Key | ~1000/day | ~40M titles | Yes | Yes |
| **Internet Archive** | Yes | No | Unknown | Large | Yes | Limited |
| **WorldCat** | Restricted | Affiliate | N/A | 586M+ | Yes | Yes |
| **ISBNdb** | No ($14.95+/mo) | API Key | 1-5/sec | 100M+ | Yes | Yes |
| **Amazon** | Restricted | Affiliate | N/A | Large | Yes | Yes |

## Detailed Provider Analysis

### 1. Open Library (Recommended Primary)

**API Documentation:** https://openlibrary.org/developers/api

#### Endpoints

| Endpoint | URL Pattern | Purpose |
|----------|-------------|---------|
| Search | `/search.json?q=` | General search |
| ISBN | `/isbn/{ISBN}.json` | Direct ISBN lookup |
| Works | `/works/{ID}.json` | Work-level data |
| Editions | `/books/{ID}.json` | Edition-specific data |
| Covers | `covers.openlibrary.org/b/isbn/{ISBN}-M.jpg` | Cover images |

#### Available Data Fields

```json
{
  "title": "Book Title",
  "authors": [{"name": "Author Name", "key": "/authors/OL123A"}],
  "publishers": ["Publisher Name"],
  "publish_date": "2020",
  "number_of_pages": 350,
  "subjects": ["Fiction", "Fantasy"],
  "description": "Book summary...",
  "covers": [12345],
  "isbn_13": ["9781234567890"],
  "isbn_10": ["1234567890"]
}
```

#### Rate Limits

- **Covers API:** 100 requests per IP per 5 minutes
- **Other APIs:** No explicit limit, but fair use expected
- **Bulk downloads:** Not allowed via API (use data dumps)

#### Pros
- Completely free, no API key required
- Good data quality for popular books
- ISBN, OCLC, LCCN lookup support
- Cover images available
- Open source, community maintained

#### Cons
- Covers rate limited
- Less comprehensive than paid services
- Some missing metadata for obscure books
- No official SLA

---

### 2. Google Books (Recommended Fallback)

**API Documentation:** https://developers.google.com/books/docs/v1/using

#### Endpoints

| Endpoint | URL Pattern | Purpose |
|----------|-------------|---------|
| Volumes | `/books/v1/volumes?q=` | Search by query |
| ISBN | `/books/v1/volumes?q=isbn:` | ISBN lookup |
| Volume | `/books/v1/volumes/{ID}` | Get specific volume |

#### Available Data Fields

```json
{
  "volumeInfo": {
    "title": "Book Title",
    "subtitle": "Subtitle",
    "authors": ["Author Name"],
    "publisher": "Publisher",
    "publishedDate": "2020-01-15",
    "description": "HTML description...",
    "pageCount": 350,
    "categories": ["Fiction"],
    "imageLinks": {
      "thumbnail": "http://...",
      "small": "http://...",
      "medium": "http://..."
    },
    "industryIdentifiers": [
      {"type": "ISBN_10", "identifier": "1234567890"},
      {"type": "ISBN_13", "identifier": "9781234567890"}
    ]
  }
}
```

#### Rate Limits

- **Default:** ~1,000 requests per day (free tier)
- **With API key:** Quota visible in Google Cloud Console
- **Per-user limits:** Configurable in console

#### Pros
- Large database (~40M titles)
- Good data quality
- Stable, reliable service
- Preview links available
- Detailed publication info

#### Cons
- Requires API key setup
- Daily quota limits
- No bulk download option
- Some books restricted by region

---

### 3. Internet Archive

**API Documentation:** https://archive.org/developers/

#### Pros
- Free and open
- Large collection of public domain books
- Full text available for many books

#### Cons
- Limited to archived books
- Less metadata for modern publications
- Complex API structure

---

### 4. ISBNdb (Paid Option)

**Website:** https://isbndb.com/

#### Pricing
- Basic: $14.95/month (1 req/sec)
- Premium: $29.95/month (3 req/sec)
- Pro: $74.95/month (5 req/sec)

#### Pros
- 100M+ titles
- 19 data points per book
- Fast and reliable
- Excellent coverage

#### Cons
- Monthly subscription required
- Overkill for small personal library

---

## Recommended Architecture

### Primary: Open Library

```
Search Strategy:
1. ISBN lookup: /isbn/{ISBN}.json (fastest, most accurate)
2. Title+Author: /search.json?title=X&author=Y
3. Title only: /search.json?title=X (fallback)
```

### Fallback: Google Books

```
When to use:
- Open Library returns no results
- Open Library rate limited
- Need additional data (description, categories)
```

### Implementation Flow

```
┌─────────────────┐
│ EPUB Upload     │
│ (extract ISBN)  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌─────────────────┐
│ Open Library    │──No─▶│ Google Books    │
│ ISBN Lookup     │     │ Fallback        │
└────────┬────────┘     └────────┬────────┘
         │Yes                    │
         ▼                       ▼
┌─────────────────────────────────────────┐
│ Match Confidence Score                   │
│ - Title similarity                       │
│ - Author match                           │
│ - ISBN exact match = 100%               │
└────────────────────┬────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────┐
│ User Preview/Confirm                     │
│ (for low confidence matches)            │
└─────────────────────────────────────────┘
```

## Rate Limiting Strategy

### Open Library

```go
const (
    OL_COVER_RATE_LIMIT = 100    // per 5 minutes
    OL_COVER_WINDOW     = 5 * time.Minute
    OL_REQUEST_DELAY    = 500 * time.Millisecond  // Between requests
)
```

### Google Books

```go
const (
    GB_DAILY_LIMIT    = 1000    // per day
    GB_REQUEST_DELAY  = 1 * time.Second  // Conservative spacing
)
```

### Bulk Operations

For bulk metadata refresh:
```go
// Process one book every 2 seconds = 30 books/minute
// = 1,800 books/hour (well within limits)
const BULK_DELAY = 2 * time.Second
```

## Error Handling

| Scenario | Action |
|----------|--------|
| 429 Too Many Requests | Exponential backoff, switch to fallback |
| 404 Not Found | Try alternate search (title instead of ISBN) |
| 5xx Server Error | Retry with backoff, log for monitoring |
| Timeout | Retry once, then skip |
| No Match Found | Mark as "needs manual entry" |

## API Client Interface Design

```go
type MetadataProvider interface {
    // LookupByISBN searches for a book by ISBN
    LookupByISBN(isbn string) (*BookMetadata, error)

    // Search finds books matching title and optional author
    Search(title, author string) ([]BookMetadata, error)

    // GetCoverURL returns URL for book cover image
    GetCoverURL(isbn string, size CoverSize) string

    // Name returns provider identifier
    Name() string
}

type BookMetadata struct {
    Title       string
    Authors     []string
    Publisher   string
    PublishDate string
    Description string
    ISBN10      string
    ISBN13      string
    PageCount   int
    Subjects    []string
    CoverURL    string
    Language    string
    Source      string  // "openlibrary", "googlebooks"
    Confidence  float64 // 0.0 - 1.0 match confidence
}
```

## Sources

- [Open Library Books API](https://openlibrary.org/dev/docs/api/books)
- [Open Library Search API](https://openlibrary.org/dev/docs/api/search)
- [Google Books API](https://developers.google.com/books/docs/v1/using)
- [Top 9 Book APIs in 2025 - ISBNdb](https://isbndb.com/blog/book-api/)
- [Free and Paid APIs for ISBN - Vinzius](https://www.vinzius.com/post/free-and-paid-api-isbn/)
