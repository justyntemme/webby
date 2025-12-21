# =============================================================================
# Webby EPUB Library - Dockerfile
# =============================================================================
#
# Build and run:
#   docker build -t webby .
#   docker run -p 8080:8080 -v /path/to/your/data:/app/data webby
#
# =============================================================================

# Build stage
FROM golang:latest AS builder

# Install build dependencies for CGO (required by go-sqlite3)
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy dependency files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o webby ./cmd/webby

# Production stage
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/webby .

# Copy static web files
COPY --from=builder /app/web ./web

# =============================================================================
# DATA STORAGE LOCATION
# =============================================================================
# The application stores all data in /app/data including:
#   - webby.db      : SQLite database (users, books metadata, collections)
#   - books/        : Uploaded EPUB files
#   - covers/       : Extracted book cover images
#
# To persist data, mount a volume to /app/data:
#   docker run -v /your/local/path:/app/data webby
#   docker run -v webby-data:/app/data webby  (named volume)
#
# Example with all options:
#   docker run -d \
#     --name webby \
#     -p 8080:8080 \
#     -v /home/user/webby-data:/app/data \
#     -e WEBBY_JWT_SECRET=your-secret-key \
#     -e WEBBY_DISABLE_REGISTRATION=true \
#     webby
# =============================================================================
RUN mkdir -p /app/data

# Environment variables
# WEBBY_DATA_DIR          : Directory for database and files (default: /app/data)
# WEBBY_PORT              : Server port (default: 8080)
# WEBBY_JWT_SECRET        : Secret key for JWT tokens (CHANGE IN PRODUCTION!)
# WEBBY_DISABLE_REGISTRATION : Set to "true" to disable new user signups
ENV WEBBY_DATA_DIR=/app/data
ENV WEBBY_PORT=8080

# Expose the default port
EXPOSE 8080

# Run as non-root user for security
RUN useradd -u 1000 webby && chown -R webby:webby /app
USER webby

# Command-line flags available:
#   --url <address>           : Bind to specific address (e.g., :8080, 0.0.0.0:3000)
#   --disable-registration    : Disable new user registration
ENTRYPOINT ["./webby"]
