package metadata

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode"
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
			result.Confidence = 1.0 // Exact ISBN match
			return result, nil
		}
		// Try fallback
		if s.fallback != nil {
			s.rateLimit.Wait()
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
			s.rateLimit.Wait()
			results, err = s.fallback.Search(ctx, title, author)
			if err == nil && len(results) > 0 {
				return s.selectBestMatch(results, title, author), nil
			}
		}
	}

	return nil, ErrNoMatch
}

// SearchBooks searches for metadata and returns all results with confidence scores
func (s *Service) SearchBooks(ctx context.Context, isbn, title, author string) ([]BookMetadata, error) {
	s.rateLimit.Wait()

	// Try ISBN lookup first (most accurate) - returns single result
	if isbn != "" {
		if result, err := s.primary.LookupByISBN(ctx, isbn); err == nil && result != nil {
			result.Confidence = 1.0
			return []BookMetadata{*result}, nil
		}
		if s.fallback != nil {
			s.rateLimit.Wait()
			if result, err := s.fallback.LookupByISBN(ctx, isbn); err == nil && result != nil {
				result.Confidence = 1.0
				return []BookMetadata{*result}, nil
			}
		}
	}

	// Search by title/author and return all results
	if title != "" {
		results, err := s.primary.Search(ctx, title, author)
		if err == nil && len(results) > 0 {
			return s.rankResults(results, title, author), nil
		}
		if s.fallback != nil {
			s.rateLimit.Wait()
			results, err = s.fallback.Search(ctx, title, author)
			if err == nil && len(results) > 0 {
				return s.rankResults(results, title, author), nil
			}
		}
	}

	return nil, ErrNoMatch
}

// rankResults calculates confidence scores for all results and sorts by confidence
func (s *Service) rankResults(results []BookMetadata, title, author string) []BookMetadata {
	for i := range results {
		results[i].Confidence = s.calculateConfidence(&results[i], title, author)
	}
	// Sort by confidence descending
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Confidence > results[i].Confidence {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	return results
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
		// Find best matching author
		normalizedAuthor := normalize(author)
		for _, a := range meta.Authors {
			score := stringSimilarity(normalize(a), normalizedAuthor)
			if score > authorScore {
				authorScore = score
			}
		}
	} else if author == "" {
		// No author to compare, don't penalize
		authorScore = 1.0
	}

	// Weight: 60% title, 40% author
	return titleScore*0.6 + authorScore*0.4
}

// normalize prepares a string for comparison
func normalize(s string) string {
	s = strings.ToLower(s)
	// Remove common prefixes
	s = strings.TrimPrefix(s, "the ")
	s = strings.TrimPrefix(s, "a ")
	s = strings.TrimPrefix(s, "an ")
	// Remove punctuation
	var result strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) {
			result.WriteRune(r)
		}
	}
	// Collapse whitespace
	return strings.Join(strings.Fields(result.String()), " ")
}

// stringSimilarity calculates similarity between two strings (0.0 - 1.0)
// Uses a simple token overlap algorithm
func stringSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}

	tokensA := strings.Fields(a)
	tokensB := strings.Fields(b)

	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0.0
	}

	// Count matching tokens
	matches := 0
	for _, ta := range tokensA {
		for _, tb := range tokensB {
			if ta == tb {
				matches++
				break
			}
		}
	}

	// Jaccard-like similarity
	total := len(tokensA) + len(tokensB) - matches
	if total == 0 {
		return 0.0
	}
	return float64(matches) / float64(total)
}

// RateLimiter provides simple rate limiting
type RateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	lastCall time.Time
}

// NewRateLimiter creates a rate limiter with the given minimum interval
func NewRateLimiter(interval time.Duration) *RateLimiter {
	return &RateLimiter{
		interval: interval,
	}
}

// Wait blocks until it's safe to make another request
func (r *RateLimiter) Wait() {
	r.mu.Lock()
	defer r.mu.Unlock()

	since := time.Since(r.lastCall)
	if since < r.interval {
		time.Sleep(r.interval - since)
	}
	r.lastCall = time.Now()
}
