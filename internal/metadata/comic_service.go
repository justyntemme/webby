package metadata

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// ComicService orchestrates comic metadata lookups
type ComicService struct {
	provider  ComicProvider
	rateLimit *RateLimiter
}

// NewComicService creates a comic metadata service
func NewComicService(provider ComicProvider) *ComicService {
	return &ComicService{
		provider:  provider,
		rateLimit: NewRateLimiter(1 * time.Second), // ComicVine has stricter rate limits
	}
}

// LookupComic attempts to find metadata using series/issue or title
// year is optional (0 means ignore) and used to filter/rank results
func (s *ComicService) LookupComic(ctx context.Context, series, issueNumber, title string, year int) (*ComicMetadata, error) {
	s.rateLimit.Wait()

	// Try series + issue lookup first (most accurate for comics)
	if series != "" {
		results, err := s.provider.SearchBySeriesAndIssue(ctx, series, issueNumber)
		if err == nil && len(results) > 0 {
			results = s.filterAndRankByYear(results, year)
			if len(results) > 0 {
				return s.selectBestMatch(results, series, issueNumber), nil
			}
		}
	}

	// Fall back to title search
	if title != "" {
		s.rateLimit.Wait()
		results, err := s.provider.SearchByTitle(ctx, title)
		if err == nil && len(results) > 0 {
			results = s.filterAndRankByYear(results, year)
			if len(results) > 0 {
				return s.selectBestMatch(results, title, issueNumber), nil
			}
		}
	}

	// If we have a series but no results, try searching by series as title
	if series != "" && title == "" {
		s.rateLimit.Wait()
		results, err := s.provider.SearchByTitle(ctx, series)
		if err == nil && len(results) > 0 {
			results = s.filterAndRankByYear(results, year)
			if len(results) > 0 {
				return s.selectBestMatch(results, series, issueNumber), nil
			}
		}
	}

	return nil, ErrNoMatch
}

// filterAndRankByYear filters results by year and boosts confidence for matches
func (s *ComicService) filterAndRankByYear(results []ComicMetadata, year int) []ComicMetadata {
	if year == 0 {
		return results // No year to filter by
	}

	var filtered []ComicMetadata
	for i := range results {
		// Extract year from release date (format: YYYY-MM-DD or YYYY)
		resultYear := extractYearFromDate(results[i].ReleaseDate)

		if resultYear > 0 {
			// Boost confidence for year match, reduce for mismatch
			yearDiff := abs(resultYear - year)
			if yearDiff == 0 {
				results[i].Confidence += 0.15 // Boost for exact year match
			} else if yearDiff <= 1 {
				results[i].Confidence += 0.05 // Small boost for ±1 year
			} else if yearDiff > 5 {
				results[i].Confidence -= 0.1 // Penalty for >5 year difference
			}
		}
		filtered = append(filtered, results[i])
	}

	return filtered
}

// extractYearFromDate extracts year from various date formats
func extractYearFromDate(date string) int {
	if date == "" {
		return 0
	}
	// Try YYYY-MM-DD format
	parts := strings.Split(date, "-")
	if len(parts) >= 1 {
		if y, err := strconv.Atoi(parts[0]); err == nil && y >= 1900 && y <= 2100 {
			return y
		}
	}
	return 0
}

// abs returns absolute value of an int
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// SearchComics searches for metadata and returns all results with confidence scores
func (s *ComicService) SearchComics(ctx context.Context, series, issueNumber, title string) ([]ComicMetadata, error) {
	s.rateLimit.Wait()

	var results []ComicMetadata

	// Try series + issue search first
	if series != "" {
		seriesResults, err := s.provider.SearchBySeriesAndIssue(ctx, series, issueNumber)
		if err == nil && len(seriesResults) > 0 {
			results = append(results, seriesResults...)
		}
	}

	// Also search by title if provided
	if title != "" && title != series {
		s.rateLimit.Wait()
		titleResults, err := s.provider.SearchByTitle(ctx, title)
		if err == nil {
			results = append(results, titleResults...)
		}
	}

	if len(results) == 0 {
		return nil, ErrNoMatch
	}

	// Remove duplicates and sort by confidence
	results = s.deduplicateResults(results)
	return s.rankResults(results, series, issueNumber), nil
}

// GetIssueDetails retrieves full details for a specific issue
func (s *ComicService) GetIssueDetails(ctx context.Context, sourceID string) (*ComicMetadata, error) {
	s.rateLimit.Wait()
	return s.provider.GetIssueDetails(ctx, sourceID)
}

// IsConfigured returns true if the comic provider is configured
func (s *ComicService) IsConfigured() bool {
	if cvp, ok := s.provider.(*ComicVineProvider); ok {
		return cvp.IsConfigured()
	}
	return true
}

// selectBestMatch returns the result with highest confidence
func (s *ComicService) selectBestMatch(results []ComicMetadata, series, issueNumber string) *ComicMetadata {
	if len(results) == 0 {
		return nil
	}

	best := &results[0]
	for i := 1; i < len(results); i++ {
		if results[i].Confidence > best.Confidence {
			best = &results[i]
		}
	}
	return best
}

// rankResults sorts results by confidence
func (s *ComicService) rankResults(results []ComicMetadata, series, issueNumber string) []ComicMetadata {
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

// deduplicateResults removes duplicate entries based on source ID
func (s *ComicService) deduplicateResults(results []ComicMetadata) []ComicMetadata {
	seen := make(map[string]bool)
	var unique []ComicMetadata

	for _, r := range results {
		key := r.SourceID
		if key == "" {
			key = r.Series + r.IssueNumber
		}
		if !seen[key] {
			seen[key] = true
			unique = append(unique, r)
		}
	}
	return unique
}
