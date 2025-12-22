package cbz

import (
	"regexp"
	"strconv"
	"strings"
)

// ComicFilenameInfo contains parsed metadata from a comic filename
type ComicFilenameInfo struct {
	Series      string  // Cleaned series name
	Title       string  // Full cleaned title (series + issue info)
	IssueNumber string  // Issue number as string (could be "1", "001", "1.5", "Annual 1")
	IssueFloat  float64 // Issue number as float for sorting
	Volume      int     // Volume number if present
	Year        int     // Publication year if found
	RawFilename string  // Original filename for reference
}

// Common digital release group tags to strip
var digitalTags = []string{
	"(Digital)",
	"(digital)",
	"(Digital-Empire)",
	"(Digital-Empire-HD)",
	"(Minutemen-DTs)",
	"(Minutemen)",
	"(DTs)",
	"(Zone-Empire)",
	"(Glorith-HD)",
	"(Glorith)",
	"(DCP)",
	"(DR & Quinch-Empire)",
	"(Empire)",
	"(Oroboros-DCP)",
	"(KG-Empire)",
	"(Renegades-DCP)",
	"(GreenGiant-DCP)",
	"(2013)",
	"(2014)",
	"(2015)",
	"(2016)",
	"(2017)",
	"(2018)",
	"(2019)",
	"(2020)",
	"(2021)",
	"(2022)",
	"(2023)",
	"(2024)",
	"(2025)",
	"[Digital]",
	"[digital]",
}

// Regex patterns for parsing
var (
	// Match year in parentheses or brackets: (2020), [2020], (Jan 2020), (January 2020)
	yearPattern = regexp.MustCompile(`[\(\[](?:(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\.?\s+)?(\d{4})[\)\]]`)

	// Match issue number patterns: #1, #001, # 1, No. 1, No 1, Issue 1
	issuePatterns = []*regexp.Regexp{
		regexp.MustCompile(`#\s*(\d+(?:\.\d+)?)`),                           // #1, #001, #1.5, # 1
		regexp.MustCompile(`(?i)(?:No\.?|Issue)\s*(\d+(?:\.\d+)?)`),         // No. 1, No 1, Issue 1
		regexp.MustCompile(`\s(\d{3})(?:\s|$|\(|\[)`),                       // Space followed by 3 digits (001)
		regexp.MustCompile(`\s(\d{1,2})(?:\s+[\(\[]|\s*$)`),                 // Space followed by 1-2 digits at end or before (
		regexp.MustCompile(`(?i)Annual\s*#?\s*(\d+)`),                       // Annual 1, Annual #1
	}

	// Match volume patterns: Vol. 1, Vol 1, Volume 1, v1, v01
	volumePattern = regexp.MustCompile(`(?i)(?:Vol(?:ume)?\.?\s*|v)(\d+)`)

	// Match content in parentheses or brackets (for removal)
	parenPattern = regexp.MustCompile(`[\(\[][^\)\]]*[\)\]]`)

	// Match multiple spaces
	multiSpacePattern = regexp.MustCompile(`\s+`)

	// Match trailing dash/hyphen with optional spaces
	trailingDashPattern = regexp.MustCompile(`\s*[-–—]\s*$`)
)

// ParseComicFilename extracts metadata from a comic filename
func ParseComicFilename(filename string) *ComicFilenameInfo {
	info := &ComicFilenameInfo{
		RawFilename: filename,
	}

	// Remove file extension
	name := filename
	for _, ext := range []string{".cbz", ".cbr", ".CBZ", ".CBR"} {
		name = strings.TrimSuffix(name, ext)
	}

	// Extract year before stripping parenthetical content
	if matches := yearPattern.FindStringSubmatch(name); len(matches) > 1 {
		if y, err := strconv.Atoi(matches[1]); err == nil && y >= 1900 && y <= 2100 {
			info.Year = y
		}
	}

	// Extract volume before stripping
	if matches := volumePattern.FindStringSubmatch(name); len(matches) > 1 {
		if v, err := strconv.Atoi(matches[1]); err == nil {
			info.Volume = v
		}
	}

	// Extract issue number (try multiple patterns)
	for _, pattern := range issuePatterns {
		if matches := pattern.FindStringSubmatch(name); len(matches) > 1 {
			info.IssueNumber = matches[1]
			if f, err := strconv.ParseFloat(matches[1], 64); err == nil {
				info.IssueFloat = f
			}
			break
		}
	}

	// Strip digital tags
	cleanedName := name
	for _, tag := range digitalTags {
		cleanedName = strings.ReplaceAll(cleanedName, tag, "")
	}

	// Remove all remaining parenthetical/bracketed content
	cleanedName = parenPattern.ReplaceAllString(cleanedName, "")

	// Remove volume notation for series name extraction
	seriesName := volumePattern.ReplaceAllString(cleanedName, "")

	// Remove issue number patterns for series name
	for _, pattern := range issuePatterns {
		seriesName = pattern.ReplaceAllString(seriesName, " ")
	}

	// Clean up the series name
	seriesName = multiSpacePattern.ReplaceAllString(seriesName, " ")
	seriesName = trailingDashPattern.ReplaceAllString(seriesName, "")
	seriesName = strings.TrimSpace(seriesName)

	info.Series = seriesName

	// Build a clean title
	title := seriesName
	if info.Volume > 0 {
		title += " Vol. " + strconv.Itoa(info.Volume)
	}
	if info.IssueNumber != "" {
		title += " #" + info.IssueNumber
	}
	if info.Year > 0 {
		title += " (" + strconv.Itoa(info.Year) + ")"
	}
	info.Title = title

	return info
}

// ParseComicFilenameSimple returns just the cleaned series name and issue number
// for backward compatibility with existing code
func ParseComicFilenameSimple(filename string) (series string, issueNum float64) {
	info := ParseComicFilename(filename)
	return info.Series, info.IssueFloat
}
