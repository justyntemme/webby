package opds

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/justyntemme/webby/internal/models"
)

const (
	// OPDS Link Relations
	OPDSLinkRelNavigation  = "http://opds-spec.org/navigation"
	OPDSLinkRelAcquisition = "http://opds-spec.org/acquisition"
	OPDSLinkRelImage       = "http://opds-spec.org/image"
	OPDSLinkRelThumbnail   = "http://opds-spec.org/image/thumbnail"
	OPDSLinkRelSearch      = "search"

	// OPDS Content Types
	OPDSCatalogType = "application/atom+xml;profile=opds-catalog;kind=navigation"
	OPDSFeedType    = "application/atom+xml;profile=opds-catalog;kind=acquisition"
	OPDSSearchType  = "application/opensearchdescription+xml"

	// MIME types for book formats
	MIMETypeEPUB = "application/epub+zip"
	MIMETypePDF  = "application/pdf"
	MIMETypeCBZ  = "application/vnd.comicbook+zip"
	MIMETypeCBR  = "application/vnd.comicbook-rar"
)

// Feed represents an OPDS Atom feed
type Feed struct {
	XMLName   xml.Name  `xml:"feed"`
	Xmlns     string    `xml:"xmlns,attr"`
	XmlnsDC   string    `xml:"xmlns:dc,attr,omitempty"`
	XmlnsOpds string    `xml:"xmlns:opds,attr,omitempty"`
	ID        string    `xml:"id"`
	Title     string    `xml:"title"`
	Updated   time.Time `xml:"updated"`
	Author    *Author   `xml:"author,omitempty"`
	Links     []Link    `xml:"link"`
	Entries   []Entry   `xml:"entry"`
}

// Entry represents an OPDS feed entry
type Entry struct {
	ID        string    `xml:"id"`
	Title     string    `xml:"title"`
	Updated   time.Time `xml:"updated"`
	Published time.Time `xml:"published,omitempty"`
	Author    *Author   `xml:"author,omitempty"`
	Content   *Content  `xml:"content,omitempty"`
	Summary   *Summary  `xml:"summary,omitempty"`
	Links     []Link    `xml:"link"`

	// Dublin Core metadata
	DCPublisher string `xml:"dc:publisher,omitempty"`
	DCLanguage  string `xml:"dc:language,omitempty"`
	DCIssued    string `xml:"dc:issued,omitempty"`
}

// Author represents an Atom author element
type Author struct {
	Name string `xml:"name"`
	URI  string `xml:"uri,omitempty"`
}

// Link represents an Atom link element
type Link struct {
	Rel   string `xml:"rel,attr,omitempty"`
	Href  string `xml:"href,attr"`
	Type  string `xml:"type,attr,omitempty"`
	Title string `xml:"title,attr,omitempty"`
}

// Content represents content with type attribute
type Content struct {
	Type  string `xml:"type,attr,omitempty"`
	Value string `xml:",chardata"`
}

// Summary represents a summary element
type Summary struct {
	Type  string `xml:"type,attr,omitempty"`
	Value string `xml:",chardata"`
}

// NewNavigationFeed creates a new OPDS navigation feed
func NewNavigationFeed(title, id, selfURL, startURL string) *Feed {
	return &Feed{
		Xmlns:     "http://www.w3.org/2005/Atom",
		XmlnsDC:   "http://purl.org/dc/terms/",
		XmlnsOpds: "http://opds-spec.org/2010/catalog",
		ID:        id,
		Title:     title,
		Updated:   time.Now().UTC(),
		Author:    &Author{Name: "Webby Library"},
		Links: []Link{
			{Rel: "self", Href: selfURL, Type: OPDSCatalogType},
			{Rel: "start", Href: startURL, Type: OPDSCatalogType},
		},
		Entries: []Entry{},
	}
}

// NewAcquisitionFeed creates a new OPDS acquisition feed
func NewAcquisitionFeed(title, id, selfURL, startURL string) *Feed {
	return &Feed{
		Xmlns:     "http://www.w3.org/2005/Atom",
		XmlnsDC:   "http://purl.org/dc/terms/",
		XmlnsOpds: "http://opds-spec.org/2010/catalog",
		ID:        id,
		Title:     title,
		Updated:   time.Now().UTC(),
		Author:    &Author{Name: "Webby Library"},
		Links: []Link{
			{Rel: "self", Href: selfURL, Type: OPDSFeedType},
			{Rel: "start", Href: startURL, Type: OPDSCatalogType},
		},
		Entries: []Entry{},
	}
}

// AddNavigationEntry adds a navigation entry to the feed
func (f *Feed) AddNavigationEntry(title, id, href, summary string) {
	entry := Entry{
		ID:      id,
		Title:   title,
		Updated: time.Now().UTC(),
		Links: []Link{
			{Rel: "subsection", Href: href, Type: OPDSCatalogType},
		},
	}
	if summary != "" {
		entry.Content = &Content{Type: "text", Value: summary}
	}
	f.Entries = append(f.Entries, entry)
}

// AddSearchLink adds an OpenSearch link to the feed
func (f *Feed) AddSearchLink(href string) {
	f.Links = append(f.Links, Link{
		Rel:  OPDSLinkRelSearch,
		Href: href,
		Type: OPDSSearchType,
	})
}

// BookToEntry converts a Book model to an OPDS entry
func BookToEntry(book *models.Book, baseURL string) Entry {
	downloadURL := fmt.Sprintf("%s/opds/v1.2/books/%s/download", baseURL, book.ID)
	coverURL := fmt.Sprintf("%s/api/books/%s/cover", baseURL, book.ID)

	entry := Entry{
		ID:      fmt.Sprintf("urn:uuid:%s", book.ID),
		Title:   book.Title,
		Updated: book.UploadedAt,
		Links: []Link{
			// Acquisition link for download
			{
				Rel:  OPDSLinkRelAcquisition,
				Href: downloadURL,
				Type: GetMIMEType(book.FileFormat),
			},
			// Cover image
			{
				Rel:  OPDSLinkRelImage,
				Href: coverURL,
				Type: "image/jpeg",
			},
			// Thumbnail
			{
				Rel:  OPDSLinkRelThumbnail,
				Href: coverURL,
				Type: "image/jpeg",
			},
		},
	}

	if book.Author != "" {
		entry.Author = &Author{Name: book.Author}
	}

	if book.Description != "" {
		entry.Summary = &Summary{Type: "text", Value: book.Description}
	}

	if book.Publisher != "" {
		entry.DCPublisher = book.Publisher
	}

	if book.Language != "" {
		entry.DCLanguage = book.Language
	}

	if book.PublishDate != "" {
		entry.DCIssued = book.PublishDate
	}

	return entry
}

// GetMIMEType returns the MIME type for a given file format
func GetMIMEType(format string) string {
	switch strings.ToLower(format) {
	case "epub":
		return MIMETypeEPUB
	case "pdf":
		return MIMETypePDF
	case "cbz":
		return MIMETypeCBZ
	case "cbr":
		return MIMETypeCBR
	default:
		return "application/octet-stream"
	}
}

// ToXML converts the feed to XML bytes
func (f *Feed) ToXML() ([]byte, error) {
	output, err := xml.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), output...), nil
}

// OpenSearchDescription generates an OpenSearch description document
func OpenSearchDescription(baseURL, searchURL string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/">
  <ShortName>Webby Library</ShortName>
  <Description>Search the Webby ebook library</Description>
  <InputEncoding>UTF-8</InputEncoding>
  <OutputEncoding>UTF-8</OutputEncoding>
  <Url type="%s" template="%s?q={searchTerms}"/>
</OpenSearchDescription>`, OPDSFeedType, searchURL)
}
