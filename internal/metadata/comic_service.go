package metadata

import (
	"context"
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
func (s *ComicService) LookupComic(ctx context.Context, series, issueNumber, title string) (*ComicMetadata, error) {
	s.rateLimit.Wait()

	// Try series + issue lookup first (most accurate for comics)
	if series != "" {
		results, err := s.provider.SearchBySeriesAndIssue(ctx, series, issueNumber)
		if err == nil && len(results) > 0 {
			return s.selectBestMatch(results, series, issueNumber), nil
		}
	}

	// Fall back to title search
	if title != "" {
		s.rateLimit.Wait()
		results, err := s.provider.SearchByTitle(ctx, title)
		if err == nil && len(results) > 0 {
			return s.selectBestMatch(results, title, issueNumber), nil
		}
	}

	// If we have a series but no results, try searching by series as title
	if series != "" && title == "" {
		s.rateLimit.Wait()
		results, err := s.provider.SearchByTitle(ctx, series)
		if err == nil && len(results) > 0 {
			return s.selectBestMatch(results, series, issueNumber), nil
		}
	}

	return nil, ErrNoMatch
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
