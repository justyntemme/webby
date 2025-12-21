# Research: EPUB Metadata Extraction Analysis

**Story:** STORY-023
**Status:** Complete
**Date:** 2025-12-21

## Summary

Analysis of the current EPUB parser implementation and gap analysis for metadata enhancement.

## Current Implementation

### Extracted Fields (internal/epub/parser.go)

| Field | Source | Default |
|-------|--------|---------|
| Title | `dc:title` | "Unknown" |
| Author | `dc:creator` | "Unknown" |
| Series | `calibre:series` meta or `belongs-to-collection` (EPUB 3) | "" |
| SeriesIndex | `calibre:series_index` meta or `group-position` (EPUB 3) | 0 |
| CoverData | Extracted from manifest cover image | nil |
| CoverExt | File extension of cover | "" |

### Metadata Struct Definition

```go
type Metadata struct {
    Title       string
    Author      string
    Series      string
    SeriesIndex float64
    CoverData   []byte
    CoverExt    string
}
```

### OPF Package Parsing

```go
type Package struct {
    Metadata struct {
        Title   []string `xml:"title"`
        Creator []struct {
            Value string `xml:",chardata"`
            Role  string `xml:"role,attr"`
        } `xml:"creator"`
        Meta []struct {
            Name     string `xml:"name,attr"`
            Content  string `xml:"content,attr"`
            Property string `xml:"property,attr"`
            Refines  string `xml:"refines,attr"`
            Value    string `xml:",chardata"`
        } `xml:"meta"`
    } `xml:"metadata"`
    // ...
}
```

## Available EPUB Metadata (Not Currently Extracted)

Based on Dublin Core and EPUB OPF standards:

### Dublin Core Elements (dc:*)

| Element | Description | Priority |
|---------|-------------|----------|
| `dc:identifier` | ISBN, UUID, DOI, etc. | **High** - needed for API lookups |
| `dc:publisher` | Publisher name | Medium |
| `dc:description` | Book summary/abstract | **High** - useful for display |
| `dc:date` | Publication date (YYYY-MM-DD) | Medium |
| `dc:language` | Language code (ISO 639-1) | Low |
| `dc:subject` | Categories/genres | Medium |
| `dc:rights` | Copyright info | Low |
| `dc:contributor` | Editor, illustrator, etc. | Low |

### OPF Meta Elements

| Element | Description | Priority |
|---------|-------------|----------|
| `opf:file-as` | Sort name for author | Medium |
| `opf:role` | Creator role (aut, edt, ill) | Low |

### EPUB 3 Specific

| Property | Description | Priority |
|----------|-------------|----------|
| `dcterms:modified` | Last modification date | Low |
| `media:duration` | For audio books | Low |

## Gap Analysis

### Fields We Extract vs Fields Available in EPUB

```
EPUB Available          Currently Extracted     Gap
─────────────────────   ───────────────────    ──────────
dc:title               ✓ Title                 -
dc:creator             ✓ Author                -
calibre:series         ✓ Series                -
calibre:series_index   ✓ SeriesIndex           -
cover image            ✓ CoverData/CoverExt    -
dc:identifier          ✗                       HIGH
dc:publisher           ✗                       MEDIUM
dc:description         ✗                       HIGH
dc:date                ✗                       MEDIUM
dc:language            ✗                       LOW
dc:subject             ✗                       MEDIUM
```

### Fields We Need vs Fields We Have

For metadata lookup to work effectively, we need:

| Need | Have | Gap |
|------|------|-----|
| ISBN for API lookup | ✗ | Extract from `dc:identifier` |
| Title for fuzzy match | ✓ | - |
| Author for matching | ✓ | - |
| Description for display | ✗ | Extract from `dc:description` |
| Publisher for matching | ✗ | Extract from `dc:publisher` |
| Publish date | ✗ | Extract from `dc:date` |

## Database Schema Gaps

### Current books table

```sql
CREATE TABLE IF NOT EXISTS books (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    author TEXT NOT NULL DEFAULT 'Unknown',
    series TEXT DEFAULT '',
    series_index REAL DEFAULT 0,
    file_path TEXT NOT NULL,
    cover_path TEXT DEFAULT '',
    file_size INTEGER DEFAULT 0,
    uploaded_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Missing columns needed

```sql
-- Proposed additions
isbn TEXT DEFAULT '',              -- For API lookups
publisher TEXT DEFAULT '',         -- Publisher name
publish_date TEXT DEFAULT '',      -- Publication date
description TEXT DEFAULT '',       -- Book description/abstract
language TEXT DEFAULT '',          -- ISO 639-1 language code
subjects TEXT DEFAULT '',          -- Comma-separated categories
metadata_source TEXT DEFAULT 'epub',  -- 'epub', 'openlibrary', 'manual'
metadata_updated_at DATETIME       -- When metadata was last refreshed
```

## Recommended Parser Enhancements

### 1. Extended Metadata Struct

```go
type Metadata struct {
    // Existing
    Title       string
    Author      string
    Series      string
    SeriesIndex float64
    CoverData   []byte
    CoverExt    string

    // New fields
    ISBN        string   // From dc:identifier with scheme="ISBN"
    Publisher   string   // From dc:publisher
    Description string   // From dc:description
    PublishDate string   // From dc:date
    Language    string   // From dc:language
    Subjects    []string // From dc:subject elements
}
```

### 2. Enhanced OPF Parsing

```go
type Package struct {
    Metadata struct {
        Title       []string `xml:"title"`
        Creator     []CreatorElement `xml:"creator"`
        Identifier  []IdentifierElement `xml:"identifier"`  // NEW
        Publisher   []string `xml:"publisher"`              // NEW
        Description []string `xml:"description"`            // NEW
        Date        []string `xml:"date"`                   // NEW
        Language    []string `xml:"language"`               // NEW
        Subject     []string `xml:"subject"`                // NEW
        Meta        []MetaElement `xml:"meta"`
    } `xml:"metadata"`
}

type IdentifierElement struct {
    Value  string `xml:",chardata"`
    Scheme string `xml:"scheme,attr"`  // "ISBN", "UUID", "DOI"
    ID     string `xml:"id,attr"`
}
```

### 3. ISBN Extraction Logic

```go
func extractISBN(identifiers []IdentifierElement) string {
    for _, id := range identifiers {
        if strings.EqualFold(id.Scheme, "ISBN") {
            return normalizeISBN(id.Value)
        }
        // Also check for urn:isbn: format
        if strings.HasPrefix(strings.ToLower(id.Value), "urn:isbn:") {
            return normalizeISBN(strings.TrimPrefix(id.Value, "urn:isbn:"))
        }
    }
    return ""
}
```

## Priority Recommendations

1. **High Priority** - Extract for API lookup capability:
   - `dc:identifier` (ISBN) - essential for accurate API matching
   - `dc:description` - useful fallback when API lookup fails

2. **Medium Priority** - Extract for better metadata:
   - `dc:publisher` - helpful for matching
   - `dc:date` - publication date
   - `dc:subject` - categories/genres

3. **Low Priority** - Nice to have:
   - `dc:language` - multi-language support
   - `opf:file-as` - author sort name

## Test Coverage

Current tests (parser_test.go) cover:
- Basic metadata extraction (title, author, series)
- Table of contents parsing
- Chapter content extraction
- HTML stripping

Missing test coverage:
- ISBN extraction from various formats
- Description extraction
- Publisher/date extraction
- Multiple author handling
- EPUB 3 metadata properties
