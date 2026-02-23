package main

import (
	"flag"
	"fmt"
	"os"
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
	fmt.Fprintln(os.Stderr, "  run     Run comparison tests")
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
