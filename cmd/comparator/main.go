package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/12go/f4/internal/comparator"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "rediff":
		cmdRediff(os.Args[2:])
	case "golive":
		cmdGoLive(os.Args[2:])
	case "clean":
		cmdClean(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: comparator <command> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  run     Run comparison tests (both endpoints live)")
	fmt.Fprintln(os.Stderr, "  golive  Cached legacy + live Go fetch, then diff")
	fmt.Fprintln(os.Stderr, "  rediff  Re-diff raw responses from a previous run")
	fmt.Fprintln(os.Stderr, "  clean   Clean old test results")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Use 'comparator <command> --help' for details.")
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "cmd/comparator/config.yaml", "path to config file")
	fs.Parse(args)

	cfg, err := comparator.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	cases := cfg.TestCases()
	if len(cases) == 0 {
		fmt.Fprintln(os.Stderr, "No test cases to run (check scenarios and dates in config)")
		os.Exit(1)
	}

	fmt.Printf("Running %d test cases (%d scenarios x %d dates)...\n\n",
		len(cases), len(cfg.Scenarios), len(cfg.Dates))

	// Run all test cases
	runner := comparator.NewRunner(cfg)
	results := runner.RunAll(cases, func(i, total int, tc comparator.TestCase) {
		fmt.Printf("[%d/%d] %s @ %s ...\n", i, total, tc.Scenario.Name, tc.Date)
	})

	// Process results: save raw + compute diffs
	now := time.Now()
	storage := comparator.NewStorage(cfg.OutputDir)
	runDir := storage.RunDir(now)
	differ := comparator.NewDiffer(cfg.FloatTolerance)

	var diffs []*comparator.DiffResult
	for _, r := range results {
		// Save raw responses
		if r.Legacy.Err == nil {
			if err := storage.SaveRaw(runDir, r.TestCase, "legacy", r.Legacy.Body); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save legacy raw: %v\n", err)
			}
		}
		if r.New.Err == nil {
			if err := storage.SaveRaw(runDir, r.TestCase, "new", r.New.Body); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save new raw: %v\n", err)
			}
		}

		// Compute diff
		var diff *comparator.DiffResult
		if r.Legacy.Err != nil || r.New.Err != nil {
			diff = &comparator.DiffResult{
				Scenario:     r.TestCase.Scenario.Name,
				Date:         r.TestCase.Date,
				LegacyStatus: r.Legacy.StatusCode,
				NewStatus:    r.New.StatusCode,
			}
			if r.Legacy.Err != nil {
				diff.Errors = append(diff.Errors, fmt.Sprintf("legacy: %v", r.Legacy.Err))
			}
			if r.New.Err != nil {
				diff.Errors = append(diff.Errors, fmt.Sprintf("new: %v", r.New.Err))
			}
		} else {
			diff = differ.Compare(r.TestCase, r.Legacy.Body, r.New.Body, r.Legacy.StatusCode, r.New.StatusCode)
		}

		if err := storage.SaveDiff(runDir, r.TestCase, diff); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save diff: %v\n", err)
		}
		diffs = append(diffs, diff)
	}

	// Build and save summary
	timestamp := now.Format("2006-01-02T15:04:05")
	summary := comparator.BuildSummary(timestamp, cfg.FloatTolerance, diffs)
	if err := storage.SaveSummary(runDir, summary); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save summary: %v\n", err)
	}

	// Print report
	comparator.PrintReport(summary, runDir)
}

// rawEntry holds a cached raw response file for one endpoint+scenario+date.
type rawEntry struct {
	scenarioName string
	date         string
	legacy       []byte
	new          []byte
}

// loadRawFiles reads raw response files from a previous run directory.
// Returns a map keyed by "slug_date" with the loaded data.
func loadRawFiles(rawDir string) (map[string]*rawEntry, error) {
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		return nil, fmt.Errorf("reading raw dir %s: %w", rawDir, err)
	}

	pairs := make(map[string]*rawEntry)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		name = strings.TrimSuffix(name, ".json")
		var endpoint string
		if strings.HasSuffix(name, "-legacy") {
			endpoint = "legacy"
			name = strings.TrimSuffix(name, "-legacy")
		} else if strings.HasSuffix(name, "-new") {
			endpoint = "new"
			name = strings.TrimSuffix(name, "-new")
		} else {
			continue
		}

		lastUnderscore := strings.LastIndex(name, "_")
		if lastUnderscore < 0 {
			continue
		}
		slug := name[:lastUnderscore]
		date := name[lastUnderscore+1:]

		key := slug + "_" + date
		p, ok := pairs[key]
		if !ok {
			words := strings.Split(slug, "-")
			for i, w := range words {
				if len(w) > 0 {
					words[i] = strings.ToUpper(w[:1]) + w[1:]
				}
			}
			p = &rawEntry{scenarioName: strings.Join(words, " "), date: date}
			pairs[key] = p
		}
		data, err := os.ReadFile(filepath.Join(rawDir, e.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to read %s: %v\n", e.Name(), err)
			continue
		}
		if endpoint == "legacy" {
			p.legacy = data
		} else {
			p.new = data
		}
	}
	return pairs, nil
}

// resolveRunDir finds the run directory: explicit name or latest.
func resolveRunDir(storage *comparator.Storage, outputDir, runName string) (string, error) {
	if runName != "" {
		return filepath.Join(outputDir, runName), nil
	}
	runs, err := storage.ListRuns()
	if err != nil || len(runs) == 0 {
		return "", fmt.Errorf("no previous runs found")
	}
	sort.Strings(runs)
	return filepath.Join(outputDir, runs[len(runs)-1]), nil
}

func cmdRediff(args []string) {
	fs := flag.NewFlagSet("rediff", flag.ExitOnError)
	configPath := fs.String("config", "cmd/comparator/config.yaml", "path to config file")
	runName := fs.String("run", "", "run directory name (e.g. 2026-03-04T12-12-12); default: latest")
	fs.Parse(args)

	cfg, err := comparator.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	storage := comparator.NewStorage(cfg.OutputDir)
	runDir, err := resolveRunDir(storage, cfg.OutputDir, *runName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	pairs, err := loadRawFiles(filepath.Join(runDir, "raw"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("Re-diffing %d test cases from %s...\n\n", len(pairs), filepath.Base(runDir))

	differ := comparator.NewDiffer(cfg.FloatTolerance)
	var diffs []*comparator.DiffResult
	var keys []string
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		p := pairs[key]
		if p.legacy == nil || p.new == nil {
			fmt.Fprintf(os.Stderr, "Skipping %s (missing legacy or new)\n", key)
			continue
		}
		tc := comparator.TestCase{
			Scenario: comparator.Scenario{Name: p.scenarioName},
			Date:     p.date,
		}
		fmt.Printf("  %s @ %s ...\n", p.scenarioName, p.date)
		diff := differ.Compare(tc, p.legacy, p.new, 200, 200)
		if err := storage.SaveDiff(runDir, tc, diff); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save diff: %v\n", err)
		}
		diffs = append(diffs, diff)
	}

	timestamp := filepath.Base(runDir)
	summary := comparator.BuildSummary(timestamp, cfg.FloatTolerance, diffs)
	if err := storage.SaveSummary(runDir, summary); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save summary: %v\n", err)
	}

	comparator.PrintReport(summary, runDir)
}

func cmdGoLive(args []string) {
	fs := flag.NewFlagSet("golive", flag.ExitOnError)
	configPath := fs.String("config", "cmd/comparator/config.yaml", "path to config file")
	runName := fs.String("run", "", "run with cached legacy responses (default: latest)")
	fs.Parse(args)

	cfg, err := comparator.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	storage := comparator.NewStorage(cfg.OutputDir)

	// Load cached legacy responses
	srcRunDir, err := resolveRunDir(storage, cfg.OutputDir, *runName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	pairs, err := loadRawFiles(filepath.Join(srcRunDir, "raw"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Build test cases from config (we need the Scenario struct for URL building)
	cases := cfg.TestCases()
	if len(cases) == 0 {
		fmt.Fprintln(os.Stderr, "No test cases in config")
		os.Exit(1)
	}

	// Index cached legacy by scenario slug + date
	// (slugify must match how storage.SaveRaw names the files)
	legacyByKey := make(map[string][]byte)
	for key, p := range pairs {
		if p.legacy != nil {
			legacyByKey[key] = p.legacy
		}
	}

	fmt.Printf("Go-live: cached legacy from %s, fetching new from Go (%d cases)...\n\n",
		filepath.Base(srcRunDir), len(cases))

	runner := comparator.NewRunner(cfg)
	differ := comparator.NewDiffer(cfg.FloatTolerance)
	now := time.Now()
	runDir := storage.RunDir(now)

	var diffs []*comparator.DiffResult
	for i, tc := range cases {
		// Build the key that matches the raw file naming
		slug := comparator.Slugify(tc.Scenario.Name)
		key := slug + "_" + tc.Date

		legacy, ok := legacyByKey[key]
		if !ok {
			fmt.Fprintf(os.Stderr, "[%d/%d] %s @ %s — no cached legacy, skipping\n", i+1, len(cases), tc.Scenario.Name, tc.Date)
			continue
		}

		fmt.Printf("[%d/%d] %s @ %s ...\n", i+1, len(cases), tc.Scenario.Name, tc.Date)
		newResult := runner.FetchEndpoint(tc, "new")

		// Save new raw response
		if newResult.Err == nil {
			if err := storage.SaveRaw(runDir, tc, "new", newResult.Body); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save new raw: %v\n", err)
			}
		}
		// Copy legacy raw from source run
		if err := storage.SaveRaw(runDir, tc, "legacy", legacy); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save legacy raw: %v\n", err)
		}

		var diff *comparator.DiffResult
		if newResult.Err != nil {
			diff = &comparator.DiffResult{
				Scenario:     tc.Scenario.Name,
				Date:         tc.Date,
				LegacyStatus: 200,
				NewStatus:    newResult.StatusCode,
				Errors:       []string{fmt.Sprintf("new: %v", newResult.Err)},
			}
		} else {
			diff = differ.Compare(tc, legacy, newResult.Body, 200, newResult.StatusCode)
		}

		if err := storage.SaveDiff(runDir, tc, diff); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save diff: %v\n", err)
		}
		diffs = append(diffs, diff)
	}

	timestamp := now.Format("2006-01-02T15:04:05")
	summary := comparator.BuildSummary(timestamp, cfg.FloatTolerance, diffs)
	if err := storage.SaveSummary(runDir, summary); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save summary: %v\n", err)
	}

	comparator.PrintReport(summary, runDir)
}


func cmdClean(args []string) {
	fs := flag.NewFlagSet("clean", flag.ExitOnError)
	configPath := fs.String("config", "cmd/comparator/config.yaml", "path to config file")
	keep := fs.Int("keep", 1, "number of recent runs to keep (0 = delete all)")
	rawOnly := fs.Bool("raw", false, "only clean raw result folders")
	diffOnly := fs.Bool("diff", false, "only clean diff result folders")
	fs.Parse(args)

	cfg, err := comparator.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	storage := comparator.NewStorage(cfg.OutputDir)
	deleted, err := storage.Clean(*keep, *rawOnly, *diffOnly)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error cleaning: %v\n", err)
		os.Exit(1)
	}

	if deleted == 0 {
		fmt.Println("Nothing to clean.")
	} else {
		fmt.Printf("Cleaned %d test run(s).\n", deleted)
	}
}
