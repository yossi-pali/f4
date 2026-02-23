package stage

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// TestAutopackKeyGeneration tests the autopack trip key format "P-{id}-{routeIndex}".
// Ported from PHP AutopackManagerTest::testGenerateAutopackKey and testExtractAutopackIdFromKey.
func TestAutopackKeyGeneration(t *testing.T) {
	tests := []struct {
		autopackID int
		routeIndex int
		expected   string
	}{
		{42, 3, "P-42-3"},
		{100, 0, "P-100-0"},
	}
	for _, tt := range tests {
		key := fmt.Sprintf("P-%d-%d", tt.autopackID, tt.routeIndex)
		if key != tt.expected {
			t.Errorf("autopack key = %q, want %q", key, tt.expected)
		}
	}
}

func TestExtractAutopackIDFromKey(t *testing.T) {
	// Ported from PHP AutopackManagerTest::testExtractAutopackIdFromKey
	tests := []struct {
		key      string
		expected *int
	}{
		{"P-42-3", intPtr(42)},
		{"P-100-0", intPtr(100)},
		{"P-23-5-64-4gdf", intPtr(23)},
		{"invalid", nil},
		{"P-", intPtr(0)},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := extractAutopackID(tt.key)
			if tt.expected == nil {
				if got != nil {
					t.Errorf("extractAutopackID(%q) = %d, want nil", tt.key, *got)
				}
			} else {
				if got == nil {
					t.Errorf("extractAutopackID(%q) = nil, want %d", tt.key, *tt.expected)
				} else if *got != *tt.expected {
					t.Errorf("extractAutopackID(%q) = %d, want %d", tt.key, *got, *tt.expected)
				}
			}
		})
	}
}

// extractAutopackID extracts the autopack ID from a key like "P-42-3".
// Returns nil if the key format is invalid (doesn't start with "P-").
// Matches PHP intval() behavior: empty string → 0.
func extractAutopackID(key string) *int {
	if !strings.HasPrefix(key, "P-") {
		return nil
	}
	rest := key[2:]
	parts := strings.SplitN(rest, "-", 2)
	if parts[0] == "" {
		zero := 0
		return &zero
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil
	}
	return &id
}

func intPtr(v int) *int { return &v }
