package comparator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Storage manages test result files on disk.
type Storage struct {
	baseDir string
}

// NewStorage creates a new Storage rooted at baseDir.
func NewStorage(baseDir string) *Storage {
	return &Storage{baseDir: baseDir}
}

// RunDir returns the directory for a specific run timestamp.
func (s *Storage) RunDir(ts time.Time) string {
	return filepath.Join(s.baseDir, ts.Format("2006-01-02T15-04-05"))
}

// SaveRaw writes a raw JSON response file.
func (s *Storage) SaveRaw(runDir string, tc TestCase, endpoint string, data []byte) error {
	dir := filepath.Join(runDir, "raw")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	filename := fmt.Sprintf("%s_%s-%s.json", Slugify(tc.Scenario.Name), tc.Date, endpoint)
	return os.WriteFile(filepath.Join(dir, filename), prettyJSON(data), 0o644)
}

// SaveDiff writes a diff result file.
func (s *Storage) SaveDiff(runDir string, tc TestCase, diff *DiffResult) error {
	dir := filepath.Join(runDir, "diff")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(diff, "", "  ")
	if err != nil {
		return err
	}
	filename := fmt.Sprintf("%s_%s.json", Slugify(tc.Scenario.Name), tc.Date)
	return os.WriteFile(filepath.Join(dir, filename), data, 0o644)
}

// SaveSummary writes the overall run summary.
func (s *Storage) SaveSummary(runDir string, summary any) error {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir, "summary.json"), data, 0o644)
}

// ListRuns returns all run directories sorted by name (oldest first).
func (s *Storage) ListRuns() ([]string, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

// Clean removes old test runs.
// keep: number of most recent runs to keep (0 = delete all).
// rawOnly/diffOnly: if true, only delete that subfolder.
func (s *Storage) Clean(keep int, rawOnly, diffOnly bool) (int, error) {
	runs, err := s.ListRuns()
	if err != nil {
		return 0, err
	}

	if len(runs) == 0 {
		return 0, nil
	}

	// Default: keep 1 latest
	toDelete := runs
	if keep > 0 && keep < len(runs) {
		toDelete = runs[:len(runs)-keep]
	} else if keep >= len(runs) {
		return 0, nil // nothing to delete
	}

	deleted := 0
	for _, run := range toDelete {
		runPath := filepath.Join(s.baseDir, run)

		if rawOnly || diffOnly {
			// Selective deletion
			if rawOnly {
				if err := os.RemoveAll(filepath.Join(runPath, "raw")); err != nil && !os.IsNotExist(err) {
					return deleted, err
				}
			}
			if diffOnly {
				if err := os.RemoveAll(filepath.Join(runPath, "diff")); err != nil && !os.IsNotExist(err) {
					return deleted, err
				}
			}
			// If both raw and diff are gone, remove the run dir too
			remaining, _ := os.ReadDir(runPath)
			if len(remaining) <= 1 { // only summary.json or empty
				os.RemoveAll(runPath)
			}
		} else {
			// Delete entire run directory
			if err := os.RemoveAll(runPath); err != nil {
				return deleted, err
			}
		}
		deleted++
	}
	return deleted, nil
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a name to a filesystem-safe slug.
// Slugify converts a scenario name to a filesystem-safe slug.
// "Bangkok to Chiang Mai" → "bangkok-to-chiang-mai"
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// prettyJSON re-formats raw JSON with indentation. Returns original if parsing fails.
func prettyJSON(data []byte) []byte {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return data
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return data
	}
	return pretty
}
