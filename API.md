# Webby API Documentation

Base URL: `http://localhost:8080`

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

file: <epub_file>

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

Query Parameters:
- sort: title, author, series, uploaded_at (default: title)
- order: asc, desc (default: asc)
- search: search in title/author
- page: page number (default: 1)
- limit: items per page (default: 0 = unlimited)

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

### Get Table of Contents
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

2. **Reading Flow:**
   - List books with `/api/books?page=1&limit=20`
   - Get TOC with `/api/books/:id/toc`
   - Get plain text with `/api/books/:id/text/:chapter`
   - Save position with `/api/books/:id/position`

3. **Pagination:**
   - Use `page` and `limit` query params
   - Response includes `total` for calculating pages

4. **Plain Text Content:**
   - Use `/api/books/:id/text/:chapter` for terminal display
   - HTML is stripped, entities decoded
   - Line breaks preserved for readability
