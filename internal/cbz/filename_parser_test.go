package cbz

import (
	"testing"
)

func TestParseComicFilename(t *testing.T) {
	tests := []struct {
		filename    string
		wantSeries  string
		wantIssue   string
		wantVolume  int
		wantYear    int
	}{
		// Basic patterns
		{
			filename:   "Batman 001.cbz",
			wantSeries: "Batman",
			wantIssue:  "001",
		},
		{
			filename:   "Batman #1.cbz",
			wantSeries: "Batman",
			wantIssue:  "1",
		},
		{
			filename:   "Amazing Spider-Man #500.cbz",
			wantSeries: "Amazing Spider-Man",
			wantIssue:  "500",
		},

		// With year
		{
			filename:   "Batman 001 (2020).cbz",
			wantSeries: "Batman",
			wantIssue:  "001",
			wantYear:   2020,
		},
		{
			filename:   "Batman #1 (Jan 2020).cbz",
			wantSeries: "Batman",
			wantIssue:  "1",
			wantYear:   2020,
		},

		// With volume
		{
			filename:   "Batman Vol. 2 #1.cbz",
			wantSeries: "Batman",
			wantIssue:  "1",
			wantVolume: 2,
		},
		{
			filename:   "Batman v3 001.cbz",
			wantSeries: "Batman",
			wantIssue:  "001",
			wantVolume: 3,
		},

		// With digital tags
		{
			filename:   "Batman 001 (2020) (Digital) (Zone-Empire).cbz",
			wantSeries: "Batman",
			wantIssue:  "001",
			wantYear:   2020,
		},
		{
			filename:   "The Walking Dead 100 (2012) (Digital-Empire).cbz",
			wantSeries: "The Walking Dead",
			wantIssue:  "100",
			wantYear:   2012,
		},

		// Complex real-world examples
		{
			filename:   "Amazing Spider-Man v5 001 (2018) (Digital) (Zone-Empire).cbz",
			wantSeries: "Amazing Spider-Man",
			wantIssue:  "001",
			wantVolume: 5,
			wantYear:   2018,
		},
		{
			filename:   "X-Men - Red #1 (2022).cbr",
			wantSeries: "X-Men - Red",
			wantIssue:  "1",
			wantYear:   2022,
		},
		{
			filename:   "Saga 054 (2018) (Digital) (Mephisto-Empire).cbz",
			wantSeries: "Saga",
			wantIssue:  "054",
			wantYear:   2018,
		},

		// Edge cases
		{
			filename:   "Y - The Last Man 001 (2002).cbz",
			wantSeries: "Y - The Last Man",
			wantIssue:  "001",
			wantYear:   2002,
		},
		{
			filename:   "100 Bullets 001.cbz",
			wantSeries: "100 Bullets",
			wantIssue:  "001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			info := ParseComicFilename(tt.filename)

			if info.Series != tt.wantSeries {
				t.Errorf("Series = %q, want %q", info.Series, tt.wantSeries)
			}
			if info.IssueNumber != tt.wantIssue {
				t.Errorf("IssueNumber = %q, want %q", info.IssueNumber, tt.wantIssue)
			}
			if info.Volume != tt.wantVolume {
				t.Errorf("Volume = %d, want %d", info.Volume, tt.wantVolume)
			}
			if info.Year != tt.wantYear {
				t.Errorf("Year = %d, want %d", info.Year, tt.wantYear)
			}
		})
	}
}

func TestParseComicFilenameTitle(t *testing.T) {
	tests := []struct {
		filename  string
		wantTitle string
	}{
		{
			filename:  "Batman 001 (2020) (Digital).cbz",
			wantTitle: "Batman #001 (2020)",
		},
		{
			filename:  "Amazing Spider-Man Vol. 2 #15 (2019) (Digital-Empire).cbz",
			wantTitle: "Amazing Spider-Man Vol. 2 #15 (2019)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			info := ParseComicFilename(tt.filename)

			if info.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", info.Title, tt.wantTitle)
			}
		})
	}
}
