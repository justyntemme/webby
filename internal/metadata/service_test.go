package metadata

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase", "THE GREAT GATSBY", "great gatsby"},
		{"removes article a", "A Tale of Two Cities", "tale of two cities"},
		{"removes article an", "An American Tragedy", "american tragedy"},
		{"removes article the", "The Hobbit", "hobbit"},
		{"removes punctuation", "Hello, World!", "hello world"},
		{"collapses whitespace", "Hello   World", "hello world"},
		{"mixed case and punctuation", "The Lord of the Rings: Fellowship", "lord of the rings fellowship"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalize(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStringSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected float64
	}{
		{"identical", "hello world", "hello world", 1.0},
		{"empty a", "", "hello", 0.0},
		{"empty b", "hello", "", 0.0},
		{"partial match", "hello world", "hello there", 0.33}, // 1 match out of 3 unique tokens
		{"no match", "hello world", "foo bar", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringSimilarity(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, 0.1, "similarity for %q and %q", tt.a, tt.b)
		})
	}
}

func TestSelectBestMatch(t *testing.T) {
	service := NewService(nil, nil)

	results := []BookMetadata{
		{Title: "The Great Gatsby", Authors: []string{"F. Scott Fitzgerald"}},
		{Title: "Great Expectations", Authors: []string{"Charles Dickens"}},
		{Title: "Gatsby", Authors: []string{"F. Scott Fitzgerald"}},
	}

	best := service.selectBestMatch(results, "The Great Gatsby", "F. Scott Fitzgerald")

	assert.NotNil(t, best)
	assert.Equal(t, "The Great Gatsby", best.Title)
	assert.True(t, best.Confidence > 0.5)
}

func TestCalculateConfidence(t *testing.T) {
	service := NewService(nil, nil)

	tests := []struct {
		name       string
		meta       *BookMetadata
		title      string
		author     string
		minScore   float64
		maxScore   float64
	}{
		{
			name:     "exact match",
			meta:     &BookMetadata{Title: "The Hobbit", Authors: []string{"J.R.R. Tolkien"}},
			title:    "The Hobbit",
			author:   "J.R.R. Tolkien",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "title only no author",
			meta:     &BookMetadata{Title: "The Hobbit", Authors: []string{"J.R.R. Tolkien"}},
			title:    "The Hobbit",
			author:   "",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "partial title match",
			meta:     &BookMetadata{Title: "The Hobbit or There and Back Again", Authors: []string{"J.R.R. Tolkien"}},
			title:    "The Hobbit",
			author:   "J.R.R. Tolkien",
			minScore: 0.4,
			maxScore: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := service.calculateConfidence(tt.meta, tt.title, tt.author)
			assert.GreaterOrEqual(t, score, tt.minScore, "score too low")
			assert.LessOrEqual(t, score, tt.maxScore, "score too high")
		})
	}
}

// MockProvider implements Provider interface for testing
type MockProvider struct {
	lookupResult *BookMetadata
	lookupErr    error
	searchResult []BookMetadata
	searchErr    error
}

func (m *MockProvider) Name() string { return "mock" }

func (m *MockProvider) LookupByISBN(ctx context.Context, isbn string) (*BookMetadata, error) {
	return m.lookupResult, m.lookupErr
}

func (m *MockProvider) Search(ctx context.Context, title, author string) ([]BookMetadata, error) {
	return m.searchResult, m.searchErr
}

func (m *MockProvider) GetCoverURL(isbn string, size CoverSize) string {
	return ""
}

func TestLookupBookByISBN(t *testing.T) {
	mock := &MockProvider{
		lookupResult: &BookMetadata{
			Title:   "Test Book",
			Authors: []string{"Test Author"},
			ISBN13:  "9780123456789",
		},
	}

	service := NewService(mock, nil)

	result, err := service.LookupBook(context.Background(), "9780123456789", "", "")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "Test Book", result.Title)
	assert.Equal(t, 1.0, result.Confidence) // ISBN match is exact
}

func TestLookupBookByTitleAuthor(t *testing.T) {
	mock := &MockProvider{
		lookupErr: ErrNoMatch,
		searchResult: []BookMetadata{
			{Title: "The Great Gatsby", Authors: []string{"F. Scott Fitzgerald"}},
		},
	}

	service := NewService(mock, nil)

	result, err := service.LookupBook(context.Background(), "", "The Great Gatsby", "F. Scott Fitzgerald")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "The Great Gatsby", result.Title)
}

func TestLookupBookNoMatch(t *testing.T) {
	mock := &MockProvider{
		lookupErr: ErrNoMatch,
		searchErr: ErrNoMatch,
	}

	service := NewService(mock, nil)

	result, err := service.LookupBook(context.Background(), "", "Nonexistent Book", "")

	assert.ErrorIs(t, err, ErrNoMatch)
	assert.Nil(t, result)
}

func TestLookupBookWithFallback(t *testing.T) {
	primary := &MockProvider{
		lookupErr: ErrNoMatch,
		searchErr: ErrNoMatch,
	}

	fallback := &MockProvider{
		searchResult: []BookMetadata{
			{Title: "Found in Fallback", Authors: []string{"Fallback Author"}},
		},
	}

	service := NewService(primary, fallback)

	result, err := service.LookupBook(context.Background(), "", "Found in Fallback", "Fallback Author")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "Found in Fallback", result.Title)
}

func TestRateLimiter(t *testing.T) {
	// Just test that it doesn't panic and works
	limiter := NewRateLimiter(1) // 1ms interval for fast test

	// Multiple rapid calls should still work
	for i := 0; i < 3; i++ {
		limiter.Wait()
	}
}
