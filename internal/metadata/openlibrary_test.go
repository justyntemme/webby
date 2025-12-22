package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeISBN(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"clean ISBN-10", "0123456789", "0123456789"},
		{"clean ISBN-13", "9780123456789", "9780123456789"},
		{"with hyphens", "978-0-12-345678-9", "9780123456789"},
		{"with spaces", "978 0 12 345678 9", "9780123456789"},
		{"URN format", "urn:isbn:9780123456789", "9780123456789"},
		{"URN uppercase", "URN:ISBN:9780123456789", "9780123456789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeISBN(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFirstOrEmpty(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{"empty slice", []string{}, ""},
		{"nil slice", nil, ""},
		{"single element", []string{"first"}, "first"},
		{"multiple elements", []string{"first", "second", "third"}, "first"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := firstOrEmpty(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOpenLibraryProviderName(t *testing.T) {
	provider := NewOpenLibraryProvider()
	assert.Equal(t, "openlibrary", provider.Name())
}

func TestOpenLibraryProviderGetCoverURL(t *testing.T) {
	provider := NewOpenLibraryProvider()

	tests := []struct {
		name     string
		isbn     string
		size     CoverSize
		expected string
	}{
		{
			name:     "small cover",
			isbn:     "9780123456789",
			size:     CoverSmall,
			expected: "https://covers.openlibrary.org/b/isbn/9780123456789-S.jpg",
		},
		{
			name:     "medium cover",
			isbn:     "9780123456789",
			size:     CoverMedium,
			expected: "https://covers.openlibrary.org/b/isbn/9780123456789-M.jpg",
		},
		{
			name:     "large cover",
			isbn:     "9780123456789",
			size:     CoverLarge,
			expected: "https://covers.openlibrary.org/b/isbn/9780123456789-L.jpg",
		},
		{
			name:     "with hyphens",
			isbn:     "978-0-12-345678-9",
			size:     CoverMedium,
			expected: "https://covers.openlibrary.org/b/isbn/9780123456789-M.jpg",
		},
		{
			name:     "empty isbn",
			isbn:     "",
			size:     CoverMedium,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.GetCoverURL(tt.isbn, tt.size)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertSearchDoc(t *testing.T) {
	provider := NewOpenLibraryProvider()

	doc := &olSearchDoc{
		Key:              "/works/OL123",
		Title:            "Test Book",
		AuthorName:       []string{"Author One", "Author Two"},
		Publisher:        []string{"Publisher A", "Publisher B"},
		FirstPublishYear: 2020,
		ISBN:             []string{"9780123456789", "0123456789"},
		CoverI:           12345,
		Subject:          []string{"Fiction", "Adventure", "Fantasy", "Sci-Fi", "Horror", "Extra"},
	}

	result := provider.convertSearchDoc(doc)

	assert.Equal(t, "Test Book", result.Title)
	assert.Equal(t, []string{"Author One", "Author Two"}, result.Authors)
	assert.Equal(t, "Publisher A", result.Publisher)
	assert.Equal(t, "2020", result.PublishDate)
	assert.Equal(t, "9780123456789", result.ISBN13)
	assert.Equal(t, "0123456789", result.ISBN10)
	assert.Len(t, result.Subjects, 5) // Limited to 5
	assert.Equal(t, "openlibrary", result.Source)
	assert.Contains(t, result.CoverURL, "12345")
}

func TestConvertEdition(t *testing.T) {
	provider := NewOpenLibraryProvider()

	edition := &olEdition{
		Title:       "Test Book",
		Publishers:  []string{"Test Publisher"},
		PublishDate: "January 1, 2020",
		ISBN10:      []string{"0123456789"},
		ISBN13:      []string{"9780123456789"},
		Covers:      []int{12345},
		NumberPages: 300,
		Subjects:    []string{"Fiction", "Adventure"},
		Description: "A great book about testing.",
	}

	result := provider.convertEdition(edition, "9780123456789")

	assert.Equal(t, "Test Book", result.Title)
	assert.Equal(t, "Test Publisher", result.Publisher)
	assert.Equal(t, "January 1, 2020", result.PublishDate)
	assert.Equal(t, "0123456789", result.ISBN10)
	assert.Equal(t, "9780123456789", result.ISBN13)
	assert.Equal(t, 300, result.PageCount)
	assert.Equal(t, []string{"Fiction", "Adventure"}, result.Subjects)
	assert.Equal(t, "A great book about testing.", result.Description)
	assert.Equal(t, 1.0, result.Confidence) // ISBN match is exact
	assert.Equal(t, "openlibrary", result.Source)
}

func TestConvertEditionDescriptionObject(t *testing.T) {
	provider := NewOpenLibraryProvider()

	// Description can be an object with value field
	edition := &olEdition{
		Title:       "Test Book",
		Description: map[string]any{"type": "/type/text", "value": "Description from object"},
	}

	result := provider.convertEdition(edition, "")

	assert.Equal(t, "Description from object", result.Description)
}
