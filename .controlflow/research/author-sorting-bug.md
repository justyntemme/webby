# Research: Author Sorting Bug Analysis

**Story:** STORY-021
**Status:** Complete
**Date:** 2025-12-21

## Summary

When a user selects "Sort by Author" in the library view, the page displays blank/empty results instead of showing books grouped by author.

## Root Cause

The `GetBooksByAuthor()` database method does not filter by user ID, causing it to return only books with an empty `user_id` field (legacy/public books). Since the authentication system now assigns all uploaded books to a user, no books match this criteria.

## Code Trace

### 1. Frontend Trigger (index.html:654-665)
```javascript
document.getElementById('sortSelect').addEventListener('change', (e) => {
    const val = e.target.value;
    if (val === 'author') {
        groupBy = 'author';
        loadGrouped('author');  // Calls /api/books/by-author
    }
    // ...
});
```

### 2. API Handler (internal/api/handlers.go:217-225)
```go
func (h *Handler) GetBooksByAuthor(c *gin.Context) {
    grouped, err := h.db.GetBooksByAuthor()  // No userID passed!
    // ...
}
```
**Issue:** Handler does not extract user ID from auth context.

### 3. Database Method (internal/storage/database.go:249-261)
```go
func (d *Database) GetBooksByAuthor() (map[string][]models.Book, error) {
    books, err := d.ListBooks("author", "asc")  // Passes empty userID
    // ...
}
```
**Issue:** Method signature doesn't accept userID parameter.

### 4. ListBooks Query (internal/storage/database.go:176-178)
```go
if userID != "" {
    query = "... WHERE user_id = ? ..."
} else {
    query = "... WHERE user_id = '' ..."  // Only returns legacy books!
}
```
**Issue:** Empty userID filters for books with literal empty string, not all books.

## Affected Functions

| Function | File | Issue |
|----------|------|-------|
| `GetBooksByAuthor()` | database.go:249 | No userID parameter |
| `GetBooksBySeries()` | database.go:264 | No userID filter (different bug: returns ALL users' books) |
| `GetBooksByAuthor` handler | handlers.go:217 | Doesn't pass user context |
| `GetBooksBySeries` handler | handlers.go:228 | Doesn't pass user context |

## EPUB Parser Analysis

The EPUB parser correctly extracts author metadata:

```go
// internal/epub/parser.go:110-116
for _, creator := range pkg.Metadata.Creator {
    if creator.Value != "" {
        meta.Author = strings.TrimSpace(creator.Value)
        break
    }
}
```

- Reads `dc:creator` elements from OPF metadata
- Falls back to "Unknown" if no author found
- Author is correctly stored in database during upload

## Database Schema

```sql
-- books table includes author field
CREATE TABLE IF NOT EXISTS books (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    author TEXT NOT NULL DEFAULT 'Unknown',  -- Correctly stores author
    ...
);

CREATE INDEX IF NOT EXISTS idx_books_author ON books(author);  -- Index exists
```

## Recommended Fix

### Option A: Add userID parameter to grouped methods (Recommended)

1. Update `GetBooksByAuthor()` signature:
```go
func (d *Database) GetBooksByAuthorForUser(userID string) (map[string][]models.Book, error) {
    books, err := d.ListBooksForUser(userID, "author", "asc")
    // ...
}
```

2. Update handler to pass user context:
```go
func (h *Handler) GetBooksByAuthor(c *gin.Context) {
    userID := auth.GetUserID(c)
    grouped, err := h.db.GetBooksByAuthorForUser(userID)
    // ...
}
```

### Option B: Include shared books

For a more complete solution, also include books shared with the user:
```sql
SELECT b.* FROM books b
LEFT JOIN book_shares bs ON b.id = bs.book_id AND bs.shared_with_id = ?
WHERE b.user_id = ? OR bs.id IS NOT NULL
ORDER BY b.author
```

## Testing Recommendations

1. Upload book as authenticated user
2. Verify book has author in database: `SELECT author FROM books WHERE id = ?`
3. Call `/api/books/by-author` with auth token
4. Verify response includes user's books grouped by author
5. Test with books that have no author (should appear under "Unknown")

## Related Issues

- `GetBooksBySeries()` has similar but different bug: it doesn't filter by user at all, potentially exposing other users' books (security issue)
