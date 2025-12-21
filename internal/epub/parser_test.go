package epub

import (
	"archive/zip"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestEPUB(t *testing.T) string {
	tmpFile, err := os.CreateTemp("", "test-*.epub")
	require.NoError(t, err)
	tmpFile.Close()

	zipWriter, err := os.Create(tmpFile.Name())
	require.NoError(t, err)
	defer zipWriter.Close()

	w := zip.NewWriter(zipWriter)

	// Add container.xml
	containerWriter, err := w.Create("META-INF/container.xml")
	require.NoError(t, err)
	containerWriter.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

	// Add content.opf
	opfWriter, err := w.Create("OEBPS/content.opf")
	require.NoError(t, err)
	opfWriter.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Test Book Title</dc:title>
    <dc:creator>Test Author Name</dc:creator>
    <meta name="calibre:series" content="Test Series"/>
    <meta name="calibre:series_index" content="2"/>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter1"/>
  </spine>
</package>`))

	// Add chapter
	chapterWriter, err := w.Create("OEBPS/chapter1.xhtml")
	require.NoError(t, err)
	chapterWriter.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body>
<h1>Chapter 1: Introduction</h1>
<p>This is the first chapter.</p>
</body>
</html>`))

	w.Close()
	return tmpFile.Name()
}

func TestParseEPUB(t *testing.T) {
	epubPath := createTestEPUB(t)
	defer os.Remove(epubPath)

	meta, err := ParseEPUB(epubPath)
	require.NoError(t, err)

	assert.Equal(t, "Test Book Title", meta.Title)
	assert.Equal(t, "Test Author Name", meta.Author)
	assert.Equal(t, "Test Series", meta.Series)
	assert.Equal(t, float64(2), meta.SeriesIndex)
}

func TestValidateEPUB(t *testing.T) {
	epubPath := createTestEPUB(t)
	defer os.Remove(epubPath)

	err := ValidateEPUB(epubPath)
	assert.NoError(t, err)
}

func TestValidateEPUB_Invalid(t *testing.T) {
	// Create a non-EPUB file
	tmpFile, err := os.CreateTemp("", "invalid-*.epub")
	require.NoError(t, err)
	tmpFile.WriteString("not a zip file")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	err = ValidateEPUB(tmpFile.Name())
	assert.Error(t, err)
}

func TestGetTableOfContents(t *testing.T) {
	epubPath := createTestEPUB(t)
	defer os.Remove(epubPath)

	chapters, err := GetTableOfContents(epubPath)
	require.NoError(t, err)

	assert.Len(t, chapters, 1)
	assert.Equal(t, 0, chapters[0].Index)
	assert.Equal(t, "chapter1", chapters[0].ID)
}

func TestGetChapterContent(t *testing.T) {
	epubPath := createTestEPUB(t)
	defer os.Remove(epubPath)

	content, err := GetChapterContent(epubPath, 0)
	require.NoError(t, err)

	assert.Contains(t, content, "Chapter 1: Introduction")
	assert.Contains(t, content, "This is the first chapter.")
}

func TestGetChapterContent_InvalidIndex(t *testing.T) {
	epubPath := createTestEPUB(t)
	defer os.Remove(epubPath)

	content, err := GetChapterContent(epubPath, 999)
	require.NoError(t, err)
	assert.Empty(t, content)
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic paragraph",
			input:    "<p>Hello world</p>",
			expected: "Hello world",
		},
		{
			name:     "multiple paragraphs",
			input:    "<p>First paragraph</p><p>Second paragraph</p>",
			expected: "First paragraph\nSecond paragraph",
		},
		{
			name:     "heading and paragraph",
			input:    "<h1>Title</h1><p>Content</p>",
			expected: "Title\nContent",
		},
		{
			name:     "script removal",
			input:    "<p>Before</p><script>alert('hi')</script><p>After</p>",
			expected: "Before\nAfter",
		},
		{
			name:     "style removal",
			input:    "<style>.foo{color:red}</style><p>Text</p>",
			expected: "Text",
		},
		{
			name:     "html entities",
			input:    "<p>Tom &amp; Jerry &lt;3</p>",
			expected: "Tom & Jerry <3",
		},
		{
			name:     "line breaks",
			input:    "<p>Line 1<br/>Line 2</p>",
			expected: "Line 1\nLine 2",
		},
		{
			name:     "nested tags",
			input:    "<div><p><strong>Bold</strong> and <em>italic</em></p></div>",
			expected: "Bold and italic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripHTML(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetChapterText(t *testing.T) {
	epubPath := createTestEPUB(t)
	defer os.Remove(epubPath)

	text, err := GetChapterText(epubPath, 0)
	require.NoError(t, err)

	// Should contain text without HTML tags
	assert.Contains(t, text, "Chapter 1: Introduction")
	assert.Contains(t, text, "This is the first chapter.")
	assert.NotContains(t, text, "<h1>")
	assert.NotContains(t, text, "<p>")
}
