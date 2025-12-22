package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// ComicVineProvider implements the ComicProvider interface for ComicVine API
type ComicVineProvider struct {
	client  *http.Client
	baseURL string
	apiKey  string
}

// NewComicVineProvider creates a new ComicVine provider
// Reads API key from COMICVINE_API_KEY environment variable
func NewComicVineProvider() *ComicVineProvider {
	apiKey := os.Getenv("COMICVINE_API_KEY")
	return &ComicVineProvider{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		baseURL: "https://comicvine.gamespot.com/api",
		apiKey:  apiKey,
	}
}

// NewComicVineProviderWithKey creates a provider with explicit API key
func NewComicVineProviderWithKey(apiKey string) *ComicVineProvider {
	return &ComicVineProvider{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		baseURL: "https://comicvine.gamespot.com/api",
		apiKey:  apiKey,
	}
}

// IsConfigured returns true if API key is set
func (p *ComicVineProvider) IsConfigured() bool {
	return p.apiKey != ""
}

// Name returns the provider identifier
func (p *ComicVineProvider) Name() string {
	return "comicvine"
}

// ComicVine API response structures
type cvResponse struct {
	Error      string `json:"error"`
	StatusCode int    `json:"status_code"`
	// Results can be array or single object depending on endpoint
}

type cvSearchResponse struct {
	Error            string        `json:"error"`
	StatusCode       int           `json:"status_code"`
	NumberOfResults  int           `json:"number_of_total_results"`
	NumberOfPageRes  int           `json:"number_of_page_results"`
	Results          []cvIssueData `json:"results"`
}

type cvVolumeSearchResponse struct {
	Error            string         `json:"error"`
	StatusCode       int            `json:"status_code"`
	NumberOfResults  int            `json:"number_of_total_results"`
	Results          []cvVolumeData `json:"results"`
}

type cvIssueResponse struct {
	Error      string      `json:"error"`
	StatusCode int         `json:"status_code"`
	Results    cvIssueData `json:"results"`
}

type cvIssueData struct {
	ID           int           `json:"id"`
	Name         string        `json:"name"`
	IssueNumber  string        `json:"issue_number"`
	Description  string        `json:"description"`
	CoverDate    string        `json:"cover_date"`
	StoreDate    string        `json:"store_date"`
	Image        cvImage       `json:"image"`
	Volume       cvVolumeRef   `json:"volume"`
	PersonCredits []cvPerson   `json:"person_credits"`
}

type cvVolumeData struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	StartYear   string    `json:"start_year"`
	Publisher   cvPublisher `json:"publisher"`
	Image       cvImage   `json:"image"`
	CountOfIssues int     `json:"count_of_issues"`
}

type cvVolumeRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type cvImage struct {
	IconURL       string `json:"icon_url"`
	MediumURL     string `json:"medium_url"`
	ScreenURL     string `json:"screen_url"`
	ScreenLargeURL string `json:"screen_large_url"`
	SmallURL      string `json:"small_url"`
	SuperURL      string `json:"super_url"`
	ThumbURL      string `json:"thumb_url"`
	TinyURL       string `json:"tiny_url"`
	OriginalURL   string `json:"original_url"`
}

type cvPerson struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

type cvPublisher struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// SearchBySeriesAndIssue searches for a comic by series name and issue number
func (p *ComicVineProvider) SearchBySeriesAndIssue(ctx context.Context, series string, issueNumber string) ([]ComicMetadata, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("ComicVine API key not configured")
	}

	// First, search for the volume (series)
	volumes, err := p.searchVolumes(ctx, series)
	if err != nil {
		return nil, err
	}

	if len(volumes) == 0 {
		return nil, ErrNoMatch
	}

	// For each matching volume, try to find the specific issue
	var results []ComicMetadata
	for _, vol := range volumes {
		issues, err := p.searchIssuesInVolume(ctx, vol.ID, issueNumber)
		if err != nil {
			continue
		}
		for _, issue := range issues {
			meta := p.convertIssueToMetadata(&issue, &vol)
			meta.Confidence = p.calculateComicConfidence(meta, series, issueNumber)
			results = append(results, meta)
		}
		// Limit results
		if len(results) >= 10 {
			break
		}
	}

	if len(results) == 0 {
		return nil, ErrNoMatch
	}

	return results, nil
}

// SearchByTitle searches for comics matching a title
func (p *ComicVineProvider) SearchByTitle(ctx context.Context, title string) ([]ComicMetadata, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("ComicVine API key not configured")
	}

	// Search issues directly by name
	params := url.Values{}
	params.Set("api_key", p.apiKey)
	params.Set("format", "json")
	params.Set("resources", "issue")
	params.Set("query", title)
	params.Set("limit", "10")
	params.Set("field_list", "id,name,issue_number,description,cover_date,image,volume")

	searchURL := fmt.Sprintf("%s/search/?%s", p.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Webby/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var data cvSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if data.StatusCode != 1 {
		if data.StatusCode == 107 {
			return nil, ErrRateLimited
		}
		return nil, fmt.Errorf("API error: %s", data.Error)
	}

	if len(data.Results) == 0 {
		return nil, ErrNoMatch
	}

	var results []ComicMetadata
	for _, issue := range data.Results {
		meta := p.convertIssueToMetadata(&issue, nil)
		meta.Confidence = p.calculateComicConfidence(meta, title, "")
		results = append(results, meta)
	}

	return results, nil
}

// GetIssueDetails retrieves full details for a specific issue
func (p *ComicVineProvider) GetIssueDetails(ctx context.Context, sourceID string) (*ComicMetadata, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("ComicVine API key not configured")
	}

	params := url.Values{}
	params.Set("api_key", p.apiKey)
	params.Set("format", "json")
	params.Set("field_list", "id,name,issue_number,description,cover_date,store_date,image,volume,person_credits")

	issueURL := fmt.Sprintf("%s/issue/4000-%s/?%s", p.baseURL, sourceID, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", issueURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Webby/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, ErrRateLimited
	}
	if resp.StatusCode == 404 {
		return nil, ErrNoMatch
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var data cvIssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if data.StatusCode != 1 {
		return nil, fmt.Errorf("API error: %s", data.Error)
	}

	meta := p.convertIssueToMetadata(&data.Results, nil)
	meta.Confidence = 1.0 // Direct ID lookup
	return &meta, nil
}

// searchVolumes searches for comic volumes (series)
func (p *ComicVineProvider) searchVolumes(ctx context.Context, name string) ([]cvVolumeData, error) {
	params := url.Values{}
	params.Set("api_key", p.apiKey)
	params.Set("format", "json")
	params.Set("resources", "volume")
	params.Set("query", name)
	params.Set("limit", "5")
	params.Set("field_list", "id,name,description,start_year,publisher,image,count_of_issues")

	searchURL := fmt.Sprintf("%s/search/?%s", p.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Webby/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var data cvVolumeSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if data.StatusCode != 1 {
		return nil, fmt.Errorf("API error: %s", data.Error)
	}

	return data.Results, nil
}

// searchIssuesInVolume finds issues in a specific volume
func (p *ComicVineProvider) searchIssuesInVolume(ctx context.Context, volumeID int, issueNumber string) ([]cvIssueData, error) {
	params := url.Values{}
	params.Set("api_key", p.apiKey)
	params.Set("format", "json")
	params.Set("filter", fmt.Sprintf("volume:%d", volumeID))
	if issueNumber != "" {
		params.Set("filter", fmt.Sprintf("volume:%d,issue_number:%s", volumeID, issueNumber))
	}
	params.Set("limit", "5")
	params.Set("field_list", "id,name,issue_number,description,cover_date,image,volume,person_credits")

	issuesURL := fmt.Sprintf("%s/issues/?%s", p.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", issuesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Webby/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var data cvSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return data.Results, nil
}

// convertIssueToMetadata converts ComicVine issue data to our metadata format
func (p *ComicVineProvider) convertIssueToMetadata(issue *cvIssueData, volume *cvVolumeData) ComicMetadata {
	meta := ComicMetadata{
		Source:      p.Name(),
		SourceID:    fmt.Sprintf("%d", issue.ID),
		IssueNumber: issue.IssueNumber,
	}

	// Title: use issue name if available, otherwise "Series #Issue"
	if issue.Name != "" {
		meta.Title = issue.Name
	} else if issue.Volume.Name != "" {
		meta.Title = fmt.Sprintf("%s #%s", issue.Volume.Name, issue.IssueNumber)
	}

	// Series from volume
	if issue.Volume.Name != "" {
		meta.Series = issue.Volume.Name
	} else if volume != nil {
		meta.Series = volume.Name
	}

	// Description - strip HTML tags
	if issue.Description != "" {
		meta.Description = stripHTML(issue.Description)
	}

	// Release date
	if issue.StoreDate != "" {
		meta.ReleaseDate = issue.StoreDate
	} else if issue.CoverDate != "" {
		meta.ReleaseDate = issue.CoverDate
	}

	// Cover image
	if issue.Image.MediumURL != "" {
		meta.CoverURL = issue.Image.MediumURL
	} else if issue.Image.SmallURL != "" {
		meta.CoverURL = issue.Image.SmallURL
	}

	// Publisher from volume data
	if volume != nil && volume.Publisher.Name != "" {
		meta.Publisher = volume.Publisher.Name
	}

	// Extract creator credits by role
	for _, person := range issue.PersonCredits {
		role := strings.ToLower(person.Role)
		if strings.Contains(role, "writer") {
			meta.Writers = appendUnique(meta.Writers, person.Name)
		}
		if strings.Contains(role, "artist") || strings.Contains(role, "penciler") || strings.Contains(role, "inker") {
			meta.Artists = appendUnique(meta.Artists, person.Name)
		}
		if strings.Contains(role, "cover") {
			meta.CoverArtists = appendUnique(meta.CoverArtists, person.Name)
		}
		if strings.Contains(role, "colorist") {
			meta.Colorists = appendUnique(meta.Colorists, person.Name)
		}
	}

	return meta
}

// calculateComicConfidence computes match confidence
func (p *ComicVineProvider) calculateComicConfidence(meta ComicMetadata, searchSeries, searchIssue string) float64 {
	score := 0.0

	// Series/title match (60% weight)
	if searchSeries != "" {
		seriesScore := stringSimilarity(normalize(meta.Series), normalize(searchSeries))
		titleScore := stringSimilarity(normalize(meta.Title), normalize(searchSeries))
		score += max(seriesScore, titleScore) * 0.6
	} else {
		score += 0.3 // Base score if no series to compare
	}

	// Issue number match (40% weight)
	if searchIssue != "" {
		if normalizeIssueNumber(meta.IssueNumber) == normalizeIssueNumber(searchIssue) {
			score += 0.4
		} else {
			score += 0.1 // Partial credit
		}
	} else {
		score += 0.2 // No issue to compare
	}

	return score
}

// normalizeIssueNumber normalizes issue numbers for comparison
func normalizeIssueNumber(issue string) string {
	// Remove leading zeros and common prefixes
	issue = strings.TrimSpace(issue)
	issue = strings.TrimLeft(issue, "0")
	issue = strings.TrimPrefix(strings.ToLower(issue), "#")
	issue = strings.TrimPrefix(issue, "no.")
	issue = strings.TrimPrefix(issue, "no")
	return strings.TrimSpace(issue)
}

// stripHTML removes HTML tags from a string
func stripHTML(s string) string {
	// Simple regex to remove HTML tags
	re := regexp.MustCompile(`<[^>]*>`)
	s = re.ReplaceAllString(s, "")
	// Clean up whitespace
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	return strings.TrimSpace(s)
}

// appendUnique appends a value to slice if not already present
func appendUnique(slice []string, val string) []string {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}

// max returns the larger of two float64 values
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
