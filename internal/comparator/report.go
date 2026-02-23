package comparator

import (
	"fmt"
	"strings"
)

// RunSummary is the overall summary saved to summary.json.
type RunSummary struct {
	Timestamp      string              `json:"timestamp"`
	FloatTolerance float64             `json:"float_tolerance"`
	TotalCases     int                 `json:"total_cases"`
	Passed         int                 `json:"passed"`
	Diffed         int                 `json:"diffed"`
	Errored        int                 `json:"errored"`
	Cases          []CaseSummary       `json:"cases"`
}

// CaseSummary is a single test case in the summary.
type CaseSummary struct {
	Scenario       string         `json:"scenario"`
	Date           string         `json:"date"`
	TripsLegacy    int            `json:"trips_legacy"`
	TripsNew       int            `json:"trips_new"`
	TripsMatched   int            `json:"trips_matched"`
	FieldsDiff     int            `json:"fields_different"`
	CategoryCounts map[string]int `json:"category_counts,omitempty"`
	Status         string         `json:"status"` // PASS, DIFF, ERROR
}

// PrintReport prints a formatted summary table to stdout.
func PrintReport(summary *RunSummary, outputDir string) {
	fmt.Printf("\n=== Comparison Results (%s) ===\n", summary.Timestamp)
	fmt.Printf("Float tolerance: %.4f\n\n", summary.FloatTolerance)

	// Calculate column widths
	maxName := 8 // "Scenario"
	for _, c := range summary.Cases {
		if len(c.Scenario) > maxName {
			maxName = len(c.Scenario)
		}
	}

	// Header
	fmt.Printf("%-*s  %-10s  %6s  %6s  %6s  %6s  %s\n",
		maxName, "Scenario", "Date", "Legacy", "New", "Match", "Diff", "Status")
	fmt.Println(strings.Repeat("-", maxName+2+10+2+6+2+6+2+6+2+6+2+10))

	// Category display order
	categoryOrder := []string{"classes", "operators", "recheck", "stations", "trips"}

	// Rows
	for _, c := range summary.Cases {
		status := c.Status
		if status == "PASS" && c.TripsLegacy == 0 {
			status = "PASS (empty)"
		}
		fmt.Printf("%-*s  %-10s  %6d  %6d  %6d  %6d  %s\n",
			maxName, c.Scenario, c.Date,
			c.TripsLegacy, c.TripsNew, c.TripsMatched, c.FieldsDiff, status)
		if len(c.CategoryCounts) > 0 {
			var parts []string
			for _, cat := range categoryOrder {
				cnt := c.CategoryCounts[cat] // 0 if missing
				parts = append(parts, fmt.Sprintf("%s: %d", cat, cnt))
			}
			// Include any categories not in the standard order
			for cat, cnt := range c.CategoryCounts {
				found := false
				for _, co := range categoryOrder {
					if co == cat {
						found = true
						break
					}
				}
				if !found && cnt > 0 {
					parts = append(parts, fmt.Sprintf("%s: %d", cat, cnt))
				}
			}
			fmt.Printf("%*s  %s\n", maxName, "", strings.Join(parts, "  "))
		}
	}

	// Footer
	fmt.Printf("\nTotal: %d test cases | %d PASS | %d DIFF | %d ERROR\n",
		summary.TotalCases, summary.Passed, summary.Diffed, summary.Errored)
	fmt.Printf("Results saved to: %s\n\n", outputDir)
}

// BuildSummary constructs a RunSummary from diff results.
func BuildSummary(timestamp string, floatTolerance float64, diffs []*DiffResult) *RunSummary {
	summary := &RunSummary{
		Timestamp:      timestamp,
		FloatTolerance: floatTolerance,
		TotalCases:     len(diffs),
	}

	for _, d := range diffs {
		status := "PASS"
		if len(d.Errors) > 0 {
			status = "ERROR"
			summary.Errored++
		} else if d.Summary.FieldsDifferent > 0 || d.Summary.TripsOnlyLegacy > 0 || d.Summary.TripsOnlyNew > 0 {
			status = "DIFF"
			summary.Diffed++
		} else {
			summary.Passed++
		}

		summary.Cases = append(summary.Cases, CaseSummary{
			Scenario:       d.Scenario,
			Date:           d.Date,
			TripsLegacy:    d.Summary.TripsLegacy,
			TripsNew:       d.Summary.TripsNew,
			TripsMatched:   d.Summary.TripsMatched,
			FieldsDiff:     d.Summary.FieldsDifferent,
			CategoryCounts: d.Summary.CategoryCounts,
			Status:         status,
		})
	}

	return summary
}
