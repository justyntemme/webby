# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies for CGO (required by go-sqlite3)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy dependency files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o webby ./cmd/webby

# Production stage
FROM alpine:3.21

# Install runtime dependencies for SQLite
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/webby .

# Copy static web files
COPY --from=builder /app/web ./web

# Create data directory for database and file storage
RUN mkdir -p /app/data

# Set environment variables
ENV WEBBY_DATA_DIR=/app/data
ENV WEBBY_PORT=8080

# Expose the default port
EXPOSE 8080

# Run as non-root user for security
RUN adduser -D -u 1000 webby && chown -R webby:webby /app
USER webby

ENTRYPOINT ["./webby"]
