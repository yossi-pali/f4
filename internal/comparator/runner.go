package comparator

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// FetchResult holds the raw response from one endpoint.
type FetchResult struct {
	Body       []byte
	StatusCode int
	Duration   time.Duration
	Err        error
}

// TestCaseResult holds both endpoint results for a single test case.
type TestCaseResult struct {
	TestCase TestCase
	Legacy   FetchResult
	New      FetchResult
}

// Runner fetches search results from both endpoints.
type Runner struct {
	client *http.Client
	cfg    *Config
}

// NewRunner creates a new Runner.
func NewRunner(cfg *Config) *Runner {
	return &Runner{
		client: &http.Client{Timeout: 5 * time.Minute},
		cfg:    cfg,
	}
}

// RunAll executes all test cases sequentially.
// Legacy and new requests for the same test case run in parallel.
func (r *Runner) RunAll(cases []TestCase, progress func(i, total int, tc TestCase)) []TestCaseResult {
	start := time.Now()
	results := make([]TestCaseResult, len(cases))
	for i, tc := range cases {
		if progress != nil {
			progress(i+1, len(cases), tc)
		}
		results[i] = r.runOne(tc)
	}
	fmt.Printf("\nAll %d test cases completed in %s\n", len(cases), time.Since(start).Round(time.Millisecond))
	return results
}

func (r *Runner) runOne(tc TestCase) TestCaseResult {
	legacy := r.cfg.Endpoints["legacy"]
	newEp := r.cfg.Endpoints["new"]

	legacyURL := buildURL(legacy, tc)
	newURL := buildURL(newEp, tc)
	fmt.Printf("  legacy: %s\n  new:    %s\n", legacyURL, newURL)

	var wg sync.WaitGroup
	var legacyResult, newResult FetchResult

	start := time.Now()
	wg.Add(2)
	go func() {
		defer wg.Done()
		legacyResult = r.fetch(legacyURL, legacy.Headers)
	}()
	go func() {
		defer wg.Done()
		newResult = r.fetch(newURL, newEp.Headers)
	}()
	wg.Wait()
	total := time.Since(start)

	fmt.Printf("  timing: legacy=%s  new=%s  total=%s\n", legacyResult.Duration.Round(time.Millisecond), newResult.Duration.Round(time.Millisecond), total.Round(time.Millisecond))

	return TestCaseResult{
		TestCase: tc,
		Legacy:   legacyResult,
		New:      newResult,
	}
}

func (r *Runner) fetch(rawURL string, headers map[string]string) FetchResult {
	// Parse base URL (everything before '?') and set raw query to avoid re-encoding.
	var base, rawQuery string
	if idx := indexOf(rawURL, '?'); idx >= 0 {
		base = rawURL[:idx]
		rawQuery = rawURL[idx+1:]
	} else {
		base = rawURL
	}

	req, err := http.NewRequest("GET", base, nil)
	if err != nil {
		return FetchResult{Err: fmt.Errorf("build request: %w", err)}
	}
	req.URL.RawQuery = rawQuery

	for k, v := range headers {
		if k == "Host" {
			req.Host = v // Go requires Host on req.Host, not Header
		} else {
			req.Header.Set(k, v)
		}
	}

	start := time.Now()
	resp, err := r.client.Do(req)
	if err != nil {
		return FetchResult{Err: fmt.Errorf("fetch %s: %w", rawURL, err), Duration: time.Since(start)}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	duration := time.Since(start) // includes body read time
	if err != nil {
		return FetchResult{Err: fmt.Errorf("read body: %w", err), StatusCode: resp.StatusCode, Duration: duration}
	}

	return FetchResult{
		Body:       body,
		StatusCode: resp.StatusCode,
		Duration:   duration,
	}
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// buildURL constructs the full URL for a test case and endpoint.
// Query param values are NOT percent-encoded (matching curl behavior);
// some legacy endpoints route based on the raw ref= value.
func buildURL(ep Endpoint, tc TestCase) string {
	var path string
	switch tc.Scenario.Type {
	case "station":
		path = fmt.Sprintf("/api/v1/searchByStations/%s/%s/%s", tc.Scenario.From, tc.Scenario.To, tc.Date)
	default: // "place"
		path = fmt.Sprintf("/api/v1/search/%s/%s/%s", tc.Scenario.From, tc.Scenario.To, tc.Date)
	}

	// Merge endpoint params + scenario params (order: endpoint first, scenario overrides)
	merged := make(map[string]string)
	for k, v := range ep.Params {
		merged[k] = v
	}
	for k, v := range tc.Scenario.Params {
		merged[k] = v
	}

	// Build raw query string without percent-encoding values
	fullURL := ep.BaseURL + path
	if len(merged) > 0 {
		sep := "?"
		for k, v := range merged {
			fullURL += sep + url.QueryEscape(k) + "=" + v
			sep = "&"
		}
	}
	return fullURL
}
