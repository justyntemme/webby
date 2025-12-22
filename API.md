# Webby API Documentation

Base URL: `http://localhost:8080`

## Supported Formats

- **EPUB** - Standard ebook format with full reading support
- **PDF** - Portable Document Format with cover extraction
- **CBZ** - Comic Book Archive (ZIP) with page-by-page reading
- **CBR** - Comic Book Archive (RAR) with page-by-page reading

## Authentication

All authenticated endpoints require the `Authorization` header:
```
Authorization: Bearer <jwt_token>
```

### Register User
```
POST /api/auth/register
Content-Type: application/json

{
  "username": "string",
  "email": "string",
  "password": "string"
}

Response 201:
{
  "message": "User registered successfully",
  "user": {
    "id": "uuid",
    "username": "string",
    "email": "string",
    "created_at": "timestamp"
  }
}
```

### Login
```
POST /api/auth/login
Content-Type: application/json

{
  "username": "string",
  "password": "string"
}

Response 200:
{
  "token": "jwt_token",
  "user": {
    "id": "uuid",
    "username": "string",
    "email": "string",
    "created_at": "timestamp"
  }
}
```

### Refresh Token
```
POST /api/auth/refresh
Content-Type: application/json

{
  "token": "existing_jwt_token"
}

Response 200:
{
  "token": "new_jwt_token"
}
```

### Get Current User
```
GET /api/auth/me
Authorization: Bearer <token>

Response 200:
{
  "user": {
    "id": "uuid",
    "username": "string",
    "email": "string",
    "created_at": "timestamp"
  }
}
```

### Search Users
```
GET /api/users/search?q=<query>
Authorization: Bearer <token>

Response 200:
{
  "users": [
    {
      "id": "uuid",
      "username": "string",
      "email": "string"
    }
  ]
}
```

---

## Books

### Upload Book
```
POST /api/books
Content-Type: multipart/form-data

file: <epub_file|pdf_file|cbz_file|cbr_file>

Supported formats: .epub, .pdf, .cbz, .cbr
Max file size: 100MB

Response 201:
{
  "message": "Book uploaded successfully",
  "book": {
    "id": "uuid",
    "user_id": "uuid",
    "title": "string",
    "author": "string",
    "series": "string",
    "series_index": 1.0,
    "file_size": 1024,
    "file_format": "epub|pdf|cbz|cbr",
    "content_type": "book|comic",
    "read_status": "unread|reading|completed",
    "rating": 0,
    "uploaded_at": "timestamp"
  }
}
```

### List Books
```
GET /api/books
GET /api/books?sort=title&order=asc
GET /api/books?search=<query>
GET /api/books?page=1&limit=20
GET /api/books?type=comic

Query Parameters:
- sort: title, author, series, date (default: title)
- order: asc, desc (default: asc)
- search: search in title/author
- page: page number (default: 1)
- limit: items per page (default: 0 = unlimited)
- type: book, comic (filter by content type)

Response 200:
{
  "books": [
    {
      "id": "uuid",
      "title": "string",
      "author": "string",
      "series": "string",
      "series_index": 1.0,
      "file_size": 1024,
      "file_format": "epub|pdf|cbz",
      "content_type": "book|comic",
      "read_status": "unread|reading|completed",
      "rating": 0,
      "uploaded_at": "timestamp"
    }
  ],
  "count": 10,
  "total": 50,
  "page": 1,
  "limit": 20
}
```

### Get Book
```
GET /api/books/:id

Response 200:
{
  "id": "uuid",
  "title": "string",
  "author": "string",
  "series": "string",
  "series_index": 1.0,
  "file_size": 1024,
  "uploaded_at": "timestamp"
}
```

### Delete Book
```
DELETE /api/books/:id

Response 200:
{
  "message": "Book deleted",
  "book": { ... }
}
```

### Books by Author
```
GET /api/books/by-author

Response 200:
{
  "authors": {
    "Author Name": [
      { "id": "uuid", "title": "string", ... }
    ]
  }
}
```

### Books by Series
```
GET /api/books/by-series

Response 200:
{
  "series": {
    "Series Name": [
      { "id": "uuid", "title": "string", "series_index": 1.0, ... }
    ]
  }
}
```

---

## Reading

### Get Book Cover
```
GET /api/books/:id/cover

Response 200: image/jpeg or image/png binary
Response 404: { "error": "No cover available" }
```

### Get Book File
```
GET /api/books/:id/file

Response 200: Binary file with appropriate Content-Type
- application/epub+zip (EPUB)
- application/pdf (PDF)
- application/zip (CBZ)
- application/x-rar-compressed (CBR)
```

### Get Table of Contents (EPUB only)
```
GET /api/books/:id/toc

Response 200:
{
  "chapters": [
    {
      "index": 0,
      "id": "chapter1",
      "href": "OEBPS/chapter1.xhtml",
      "title": "Chapter 1: Introduction"
    }
  ]
}
```

---

## CBZ/CBR Comic Reading

These endpoints work for both CBZ (ZIP) and CBR (RAR) comic archives.

### Get Comic Info
```
GET /api/books/:id/cbz/info

Response 200:
{
  "pageCount": 24,
  "title": "Comic Title",
  "author": "Artist Name",
  "series": "Series Name"
}
```

### Get Comic Page
```
GET /api/books/:id/cbz/page/:pageIndex

pageIndex: 0-based page number

Response 200: image/jpeg or image/png binary
Response 404: { "error": "Page not found" }
```

### Get Chapter Content (HTML)
```
GET /api/books/:id/content/:chapter

Response 200:
Content-Type: text/html; charset=utf-8

<html>...</html>
```

### Get Chapter Content (Plain Text) - TUI Friendly
```
GET /api/books/:id/text/:chapter

Response 200:
{
  "book_id": "uuid",
  "chapter": 0,
  "content": "Plain text content with HTML stripped...",
  "content_type": "text/plain"
}
```

### Get Reading Position
```
GET /api/books/:id/position

Response 200:
{
  "position": {
    "book_id": "uuid",
    "chapter": "0",
    "position": 0.5,
    "updated_at": "timestamp"
  }
}

Response 200 (no position saved):
{
  "position": null
}
```

### Save Reading Position
```
POST /api/books/:id/position
Content-Type: application/json

{
  "chapter": "0",
  "position": 0.5
}

Response 200:
{
  "message": "Position saved",
  "position": { ... }
}
```

---

## Book Metadata

### Lookup Metadata
```
GET /api/metadata/lookup?title=<title>&author=<author>&isbn=<isbn>

Query Parameters (at least one required):
- isbn: ISBN-10 or ISBN-13
- title: Book title
- author: Author name

Response 200:
{
  "metadata": {
    "title": "string",
    "authors": ["string"],
    "publisher": "string",
    "publish_date": "string",
    "description": "string",
    "isbn_10": "string",
    "isbn_13": "string",
    "subjects": ["string"],
    "cover_url": "string",
    "source": "openlibrary",
    "confidence": 0.95
  }
}
```

### Search Metadata
```
GET /api/metadata/search?title=<title>&author=<author>&isbn=<isbn>

Returns multiple results for selection.

Response 200:
{
  "results": [ ... ],
  "count": 5
}
```

### Refresh Book Metadata
```
POST /api/books/:id/metadata/refresh

Automatically fetches metadata from external sources.

Response 200:
{
  "message": "Metadata updated successfully",
  "book": { ... },
  "confidence": 0.85,
  "source": "openlibrary"
}
```

### Update Book Metadata (Manual)
```
PUT /api/books/:id/metadata
Content-Type: application/json

{
  "title": "string",
  "author": "string",
  "series": "string",
  "series_index": 1.0,
  "isbn": "string",
  "publisher": "string",
  "publish_date": "string",
  "language": "string",
  "subjects": "comma, separated, tags",
  "description": "string"
}

Response 200:
{
  "message": "Metadata updated successfully",
  "book": { ... }
}
```

### Bulk Refresh Metadata
```
POST /api/metadata/bulk-refresh
Authorization: Bearer <token>
Content-Type: application/json

{
  "book_ids": ["uuid1", "uuid2", "uuid3"],
  "content_type": "book"
}

Either book_ids or content_type can be specified:
- book_ids: Specific books to refresh
- content_type: "book" or "comic" to refresh all books of that type (max 50 per request)

Response 200:
{
  "message": "Bulk metadata refresh complete",
  "processed": 10,
  "succeeded": 8,
  "failed": 2,
  "results": [
    {
      "book_id": "uuid",
      "title": "string",
      "status": "success",
      "confidence": 0.85
    },
    {
      "book_id": "uuid2",
      "title": "string",
      "status": "failed",
      "reason": "No matching metadata found"
    }
  ]
}
```

---

## Comic Metadata

Requires `COMICVINE_API_KEY` environment variable.

### Check Comic Metadata Status
```
GET /api/metadata/comic/status

Response 200:
{
  "configured": true,
  "provider": "comicvine",
  "message": "Comic metadata service is ready"
}
```

### Search Comic Metadata
```
GET /api/metadata/comic/search?series=<series>&issue=<issue>&title=<title>

Query Parameters (at least series or title required):
- series: Comic series name
- issue: Issue number
- title: Comic title

Response 200:
{
  "results": [
    {
      "title": "string",
      "series": "string",
      "issue_number": "string",
      "publisher": "string",
      "release_date": "string",
      "description": "string",
      "writers": ["string"],
      "artists": ["string"],
      "cover_url": "string",
      "source": "comicvine",
      "confidence": 0.9
    }
  ],
  "count": 5
}
```

### Refresh Comic Metadata
```
POST /api/books/:id/metadata/comic/refresh

Only works for books with content_type="comic".
Uses intelligent filename parsing to extract series, issue number, and year for better ComicVine matching.

Response 200:
{
  "message": "Comic metadata updated successfully",
  "book": { ... },
  "confidence": 0.85,
  "source": "comicvine"
}

Response 404 (no match):
{
  "error": "No matching comic metadata found",
  "parsed_info": {
    "series": "Batman",
    "issue_number": "001",
    "year": 2020
  }
}

Response 503 (not configured):
{
  "error": "Comic metadata service not configured",
  "message": "Set COMICVINE_API_KEY environment variable to enable"
}
```

### Reprocess Comic Filename
```
POST /api/books/:id/metadata/comic/reprocess

Re-parses the comic filename to extract cleaner metadata without external API lookup.
Useful for fixing titles that weren't properly parsed on initial upload.

Common filename patterns recognized:
- "Series Name 001 (2020).cbz"
- "Series Name #1 (2020) (Digital).cbr"
- "Series Name v01 - Issue Title (2020).cbz"

Response 200:
{
  "message": "Comic filename reprocessed successfully",
  "book": { ... },
  "changes": {
    "title": {"old": "Batman 001 (2020) (Digital)", "new": "Batman #001 (2020)"},
    "series": {"old": "", "new": "Batman"},
    "series_index": {"old": 0, "new": 1}
  },
  "parsed_info": {
    "raw_filename": "Batman 001 (2020) (Digital).cbz",
    "series": "Batman",
    "issue_number": "001",
    "volume": 0,
    "year": 2020
  }
}
```

---

## Read Status Tracking

Track reading progress with status values: `unread`, `reading`, `completed`.

### Get Book Read Status
```
GET /api/books/:id/status
Authorization: Bearer <token>

Response 200:
{
  "book_id": "uuid",
  "read_status": "reading",
  "date_completed": null
}
```

### Update Book Read Status
```
PUT /api/books/:id/status
Authorization: Bearer <token>
Content-Type: application/json

{
  "status": "completed"  // "unread", "reading", or "completed"
}

Response 200:
{
  "message": "Read status updated",
  "book_id": "uuid",
  "read_status": "completed",
  "date_completed": "2025-01-15T12:00:00Z"
}
```

### Get Read Status Counts
```
GET /api/books/status/counts
Authorization: Bearer <token>

Response 200:
{
  "unread": 50,
  "reading": 5,
  "completed": 20,
  "total": 75
}
```

### Bulk Update Read Status
```
POST /api/books/status/bulk
Authorization: Bearer <token>
Content-Type: application/json

{
  "book_ids": ["uuid-1", "uuid-2", "uuid-3"],
  "status": "completed"
}

Response 200:
{
  "message": "Read status updated",
  "updated_count": 3,
  "requested_count": 3,
  "status": "completed"
}
```

### Filter Books by Read Status
```
GET /api/books?status=reading
Authorization: Bearer <token>

Query Parameters:
- status: "unread", "reading", or "completed" (optional)

Note: The status filter can be combined with other filters (type, search, sort).
```

**Auto-Status Updates:**
- When a reading position is saved and the book's status is "unread", it automatically updates to "reading"

---

## Star Ratings

Rate books from 1-5 stars. A rating of 0 means no rating.

### Get Book Rating
```
GET /api/books/:id/rating
Authorization: Bearer <token>

Response 200:
{
  "book_id": "uuid",
  "rating": 4
}
```

### Update Book Rating
```
PUT /api/books/:id/rating
Authorization: Bearer <token>
Content-Type: application/json

{
  "rating": 5
}

Response 200:
{
  "message": "Rating updated",
  "rating": 5
}

Notes:
- Rating must be between 0 and 5
- Rating of 0 clears the rating
- Users can only rate books they own or have been shared with them
```

---

## Custom Tags

Create and manage custom tags to organize your library. Tags have a name and optional color.

### List All Tags
```
GET /api/tags
Authorization: Bearer <token>

Response 200:
{
  "tags": [
    {
      "id": "uuid",
      "user_id": "uuid",
      "name": "Sci-Fi",
      "color": "#3b82f6",
      "created_at": "timestamp",
      "book_count": 5
    }
  ],
  "count": 1
}
```

### Create Tag
```
POST /api/tags
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Sci-Fi",
  "color": "#3b82f6"
}

Response 201:
{
  "message": "Tag created",
  "tag": {
    "id": "uuid",
    "user_id": "uuid",
    "name": "Sci-Fi",
    "color": "#3b82f6",
    "created_at": "timestamp"
  }
}

Notes:
- Name is required, color defaults to #3b82f6 if not provided
- Tag names must be unique per user
```

### Get Tag
```
GET /api/tags/:id
Authorization: Bearer <token>

Response 200:
{
  "id": "uuid",
  "user_id": "uuid",
  "name": "Sci-Fi",
  "color": "#3b82f6",
  "created_at": "timestamp",
  "book_count": 5
}
```

### Update Tag
```
PUT /api/tags/:id
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Science Fiction",
  "color": "#ef4444"
}

Response 200:
{
  "message": "Tag updated",
  "tag": { ... }
}
```

### Delete Tag
```
DELETE /api/tags/:id
Authorization: Bearer <token>

Response 200:
{
  "message": "Tag deleted",
  "tag_id": "uuid"
}

Note: Deleting a tag removes it from all books.
```

### Get Books by Tag
```
GET /api/tags/:id/books
Authorization: Bearer <token>

Response 200:
{
  "tag": { ... },
  "books": [ ... ],
  "count": 5
}
```

### Get Tags for a Book
```
GET /api/books/:id/tags
Authorization: Bearer <token>

Response 200:
{
  "book_id": "uuid",
  "tags": [ ... ],
  "count": 2
}
```

### Add Tag to Book
```
POST /api/books/:id/tags/:tagId
Authorization: Bearer <token>

Response 200:
{
  "message": "Tag added to book",
  "book_id": "uuid",
  "tag_id": "uuid"
}
```

### Remove Tag from Book
```
DELETE /api/books/:id/tags/:tagId
Authorization: Bearer <token>

Response 200:
{
  "message": "Tag removed from book",
  "book_id": "uuid",
  "tag_id": "uuid"
}
```

### Toggle Tag on Book
```
PUT /api/books/:id/tags/:tagId/toggle
Authorization: Bearer <token>

Response 200:
{
  "book_id": "uuid",
  "tag_id": "uuid",
  "in_tag": true
}

Note: Adds the tag if not present, removes it if already applied.
```

---

## Annotations & Highlights

Create and manage annotations (highlights and notes) on your books. Annotations support multiple highlight colors and optional notes.

### Highlight Colors

Available colors:
- `yellow` (default)
- `green`
- `blue`
- `pink`
- `orange`

### List All Annotations
```
GET /api/annotations
Authorization: Bearer <token>

Response 200:
{
  "annotations": [
    {
      "id": "uuid",
      "book_id": "uuid",
      "user_id": "uuid",
      "chapter": "chapter1",
      "cfi": "/6/4[chap01ref]!/4/2/2/1:0",
      "start_offset": 0,
      "end_offset": 50,
      "selected_text": "The highlighted text",
      "note": "My personal note",
      "color": "yellow",
      "created_at": "timestamp",
      "updated_at": "timestamp"
    }
  ],
  "count": 1
}
```

### Get Annotation Statistics
```
GET /api/annotations/stats
Authorization: Bearer <token>

Response 200:
{
  "total_annotations": 42,
  "books_with_annotations": 5
}
```

### List Annotations for a Book
```
GET /api/books/:id/annotations
Authorization: Bearer <token>

Response 200:
{
  "annotations": [ ... ],
  "count": 5
}

Note: Returns annotations ordered by chapter and start_offset.
```

### List Annotations for a Chapter
```
GET /api/books/:id/annotations/chapter/:chapter
Authorization: Bearer <token>

Response 200:
{
  "annotations": [ ... ],
  "count": 2
}

Note: Returns only annotations for the specified chapter.
```

### Create Annotation
```
POST /api/books/:id/annotations
Authorization: Bearer <token>
Content-Type: application/json

{
  "chapter": "chapter1",
  "cfi": "/6/4[chap01ref]!/4/2/2/1:0",
  "start_offset": 100,
  "end_offset": 150,
  "selected_text": "The text to highlight",
  "note": "Optional note about this highlight",
  "color": "yellow"
}

Response 201:
{
  "message": "Annotation created",
  "annotation": {
    "id": "uuid",
    "book_id": "uuid",
    "user_id": "uuid",
    "chapter": "chapter1",
    "cfi": "/6/4[chap01ref]!/4/2/2/1:0",
    "start_offset": 100,
    "end_offset": 150,
    "selected_text": "The text to highlight",
    "note": "Optional note about this highlight",
    "color": "yellow",
    "created_at": "timestamp",
    "updated_at": "timestamp"
  }
}

Required fields:
- chapter: Chapter/section identifier
- selected_text: The highlighted text

Optional fields:
- cfi: EPUB CFI for precise location
- start_offset / end_offset: Character offsets
- note: User's note/comment
- color: Highlight color (defaults to "yellow")
```

### Get Annotation
```
GET /api/books/:id/annotations/:annotationId
Authorization: Bearer <token>

Response 200:
{
  "id": "uuid",
  "book_id": "uuid",
  "user_id": "uuid",
  "chapter": "chapter1",
  "selected_text": "The highlighted text",
  "note": "My note",
  "color": "yellow",
  "created_at": "timestamp",
  "updated_at": "timestamp"
}
```

### Update Annotation
```
PUT /api/books/:id/annotations/:annotationId
Authorization: Bearer <token>
Content-Type: application/json

{
  "note": "Updated note",
  "color": "green"
}

Response 200:
{
  "message": "Annotation updated",
  "annotation": { ... }
}

Note: Only note and color can be updated. Text selection cannot be changed.
```

### Delete Annotation
```
DELETE /api/books/:id/annotations/:annotationId
Authorization: Bearer <token>

Response 200:
{
  "message": "Annotation deleted"
}
```

---

## Duplicate Detection

Duplicate detection uses SHA256 file hashes to identify identical books in your library.

### Get Duplicate Status
```
GET /api/duplicates/status
Authorization: Bearer <token>

Response 200:
{
  "books_without_hash": 0,
  "duplicate_groups": 3,
  "duplicate_books": 5,
  "ready": true
}
```

### Find Duplicates
```
GET /api/duplicates
Authorization: Bearer <token>

Response 200:
{
  "groups": [
    {
      "file_hash": "sha256hash...",
      "count": 2,
      "books": [
        {
          "id": "uuid",
          "title": "string",
          "author": "string",
          "uploaded_at": "timestamp"
        }
      ]
    }
  ],
  "count": 3
}
```

### Compute Missing Hashes
```
POST /api/duplicates/compute
Authorization: Bearer <token>

Computes file hashes for books uploaded before duplicate detection was enabled.

Response 200:
{
  "message": "Hash computation complete",
  "total": 100,
  "processed": 98,
  "failed": 2
}
```

### Merge Duplicates
```
POST /api/duplicates/merge
Authorization: Bearer <token>
Content-Type: application/json

{
  "keep_id": "uuid-of-book-to-keep",
  "delete_ids": ["uuid-to-delete-1", "uuid-to-delete-2"]
}

Response 200:
{
  "message": "Duplicates merged successfully",
  "kept_book": { ... },
  "deleted_books": ["uuid-1", "uuid-2"],
  "files_removed": 2
}
```

---

## Book Sharing

### Get Shared Books
```
GET /api/books/shared
Authorization: Bearer <token>

Response 200:
{
  "books": [ ... ],
  "count": 5
}
```

### Get Book Shares
```
GET /api/books/:id/shares
Authorization: Bearer <token>

Response 200:
{
  "shared_with": [
    {
      "id": "uuid",
      "username": "string",
      "email": "string"
    }
  ]
}
```

### Share Book
```
POST /api/books/:id/share/:userId
Authorization: Bearer <token>

Response 200:
{
  "message": "Book shared successfully"
}
```

### Unshare Book
```
DELETE /api/books/:id/share/:userId
Authorization: Bearer <token>

Response 200:
{
  "message": "Book unshared successfully"
}
```

---

## Collections

### Create Collection
```
POST /api/collections
Content-Type: application/json

{
  "name": "string"
}

Response 201:
{
  "message": "Collection created",
  "collection": {
    "id": "uuid",
    "name": "string",
    "created_at": "timestamp"
  }
}
```

### List Collections
```
GET /api/collections

Response 200:
{
  "collections": [
    {
      "id": "uuid",
      "name": "string",
      "created_at": "timestamp"
    }
  ],
  "count": 5
}
```

### Get Collection
```
GET /api/collections/:id

Response 200:
{
  "collection": {
    "id": "uuid",
    "name": "string",
    "created_at": "timestamp"
  },
  "books": [ ... ]
}
```

### Update Collection
```
PUT /api/collections/:id
Content-Type: application/json

{
  "name": "new name"
}

Response 200:
{
  "message": "Collection updated"
}
```

### Delete Collection
```
DELETE /api/collections/:id

Response 200:
{
  "message": "Collection deleted"
}
```

### Add Book to Collection
```
POST /api/collections/:id/books/:bookId

Response 200:
{
  "message": "Book added to collection"
}
```

### Remove Book from Collection
```
DELETE /api/collections/:id/books/:bookId

Response 200:
{
  "message": "Book removed from collection"
}
```

### Bulk Add Books to Collection
```
POST /api/collections/:id/books
Content-Type: application/json

{
  "book_ids": ["uuid1", "uuid2", "uuid3"]
}

Response 200:
{
  "message": "Books added to collection",
  "count": 3
}
```

### Get Collections for Book
```
GET /api/books/:id/collections

Response 200:
{
  "collections": [ ... ]
}
```

---

## Reading Lists

Reading lists are user-curated lists for organizing books. System lists ("Want to Read", "Favorites") are auto-created for each user.

### List Reading Lists
```
GET /api/reading-lists
Authorization: Bearer <token>

Response 200:
{
  "lists": [
    {
      "id": "uuid",
      "user_id": "uuid",
      "name": "Want to Read",
      "list_type": "want_to_read",
      "created_at": "timestamp",
      "book_count": 5
    },
    {
      "id": "uuid",
      "user_id": "uuid",
      "name": "Favorites",
      "list_type": "favorites",
      "created_at": "timestamp",
      "book_count": 3
    }
  ],
  "count": 2
}
```

### Get Reading List
```
GET /api/reading-lists/:id
Authorization: Bearer <token>

Response 200:
{
  "list": {
    "id": "uuid",
    "name": "Want to Read",
    "list_type": "want_to_read",
    "book_count": 5
  },
  "books": [ ... ]
}
```

### Create Custom Reading List
```
POST /api/reading-lists
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "My Summer Reads"
}

Response 201:
{
  "message": "Reading list created",
  "list": {
    "id": "uuid",
    "name": "My Summer Reads",
    "list_type": "custom",
    "created_at": "timestamp"
  }
}
```

### Update Reading List
```
PUT /api/reading-lists/:id
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "New List Name"
}

Response 200:
{
  "message": "Reading list updated",
  "list": { ... }
}
```

### Delete Reading List
```
DELETE /api/reading-lists/:id
Authorization: Bearer <token>

Note: System lists (want_to_read, favorites) cannot be deleted.

Response 200:
{
  "message": "Reading list deleted"
}
```

### Add Book to Reading List
```
POST /api/reading-lists/:id/books/:bookId
Authorization: Bearer <token>

Response 200:
{
  "message": "Book added to reading list",
  "list_id": "uuid",
  "book_id": "uuid"
}
```

### Remove Book from Reading List
```
DELETE /api/reading-lists/:id/books/:bookId
Authorization: Bearer <token>

Response 200:
{
  "message": "Book removed from reading list",
  "list_id": "uuid",
  "book_id": "uuid"
}
```

### Toggle Book in Reading List
```
PUT /api/reading-lists/:id/books/:bookId/toggle
Authorization: Bearer <token>

Adds the book if not in list, removes if already in list.

Response 200:
{
  "message": "Book added from reading list",
  "action": "added",
  "list_id": "uuid",
  "book_id": "uuid",
  "in_list": true
}
```

### Get Reading Lists for Book
```
GET /api/books/:id/reading-lists
Authorization: Bearer <token>

Response 200:
{
  "book_id": "uuid",
  "lists": [
    {
      "id": "uuid",
      "name": "Want to Read",
      "list_type": "want_to_read"
    }
  ]
}
```

### Reorder Reading List
```
PUT /api/reading-lists/:id/reorder
Authorization: Bearer <token>
Content-Type: application/json

{
  "book_ids": ["uuid-1", "uuid-2", "uuid-3"]
}

Response 200:
{
  "message": "Reading list reordered",
  "list_id": "uuid"
}
```

---

## Utility

### Health Check
```
GET /health

Response 200:
{
  "status": "ok",
  "time": "timestamp"
}
```

### API Documentation
```
GET /api

Response 200:
{
  "name": "Webby API",
  "version": "1.0.0",
  "description": "EPUB library API for web and TUI clients",
  "endpoints": [ ... ]
}
```

---

## Error Responses

All errors return JSON:
```json
{
  "error": "Error message description"
}
```

Common HTTP status codes:
- `400` - Bad Request (invalid input)
- `401` - Unauthorized (missing/invalid token)
- `403` - Forbidden (not owner)
- `404` - Not Found
- `500` - Internal Server Error

---

## TUI Client Tips

1. **Authentication Flow:**
   - Login with `/api/auth/login`
   - Store the JWT token
   - Include in all requests as `Authorization: Bearer <token>`
   - Refresh before expiry with `/api/auth/refresh`

2. **Reading Flow (EPUB):**
   - List books with `/api/books?page=1&limit=20`
   - Get TOC with `/api/books/:id/toc`
   - Get plain text with `/api/books/:id/text/:chapter`
   - Save position with `/api/books/:id/position`

3. **Reading Flow (CBZ/CBR Comics):**
   - Same endpoints work for both CBZ (ZIP) and CBR (RAR) formats
   - Get comic info with `/api/books/:id/cbz/info`
   - Fetch pages with `/api/books/:id/cbz/page/:pageIndex`
   - Page index is 0-based (0 to pageCount-1)

4. **Reading Flow (PDF):**
   - Download file with `/api/books/:id/file`
   - Use a PDF library for rendering

5. **Filtering Content:**
   - Books only: `/api/books?type=book`
   - Comics only: `/api/books?type=comic`

6. **Pagination:**
   - Use `page` and `limit` query params
   - Response includes `total` for calculating pages

7. **Plain Text Content:**
   - Use `/api/books/:id/text/:chapter` for terminal display
   - HTML is stripped, entities decoded
   - Line breaks preserved for readability

8. **Metadata Sources:**
   - Books: OpenLibrary (automatic)
   - Comics: ComicVine (requires COMICVINE_API_KEY)
