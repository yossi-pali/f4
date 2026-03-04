package comparator

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level comparator configuration.
type Config struct {
	Endpoints      map[string]Endpoint `yaml:"endpoints"`
	Dates          []string            `yaml:"dates"`
	FloatTolerance float64             `yaml:"float_tolerance"`
	Scenarios      []Scenario          `yaml:"scenarios"`
	OutputDir      string              `yaml:"output_dir"`
}

// Endpoint defines one target (legacy or new).
type Endpoint struct {
	BaseURL string            `yaml:"base_url"`
	Headers map[string]string `yaml:"headers"`
	Params  map[string]string `yaml:"params"`
}

// Scenario defines a single search scenario to compare.
type Scenario struct {
	Name   string            `yaml:"name"`
	Type   string            `yaml:"type"` // "place" or "station"
	From   string            `yaml:"from"`
	To     string            `yaml:"to"`
	Params map[string]string `yaml:"params"`
}

// TestCase is a single executable test (scenario + date).
type TestCase struct {
	Scenario Scenario
	Date     string
}

// LoadConfig reads and parses a YAML config file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{
		FloatTolerance: 0.01, // default
		OutputDir:      "./test-results",
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if len(c.Endpoints) < 2 {
		return fmt.Errorf("config must define at least 2 endpoints")
	}
	if _, ok := c.Endpoints["legacy"]; !ok {
		return fmt.Errorf("config missing 'legacy' endpoint")
	}
	if _, ok := c.Endpoints["new"]; !ok {
		return fmt.Errorf("config missing 'new' endpoint")
	}
	if len(c.Dates) == 0 {
		return fmt.Errorf("config must define at least one date")
	}
	if len(c.Scenarios) == 0 {
		return fmt.Errorf("config must define at least one scenario")
	}
	for i, s := range c.Scenarios {
		if s.Type != "place" && s.Type != "station" {
			return fmt.Errorf("scenario %d (%s): type must be 'place' or 'station', got %q", i, s.Name, s.Type)
		}
		if s.From == "" || s.To == "" {
			return fmt.Errorf("scenario %d (%s): from and to are required", i, s.Name)
		}
	}
	return nil
}

// TestCases returns the full matrix of scenario x date.
func (c *Config) TestCases() []TestCase {
	cases := make([]TestCase, 0, len(c.Scenarios)*len(c.Dates))
	for _, s := range c.Scenarios {
		for _, d := range c.Dates {
			cases = append(cases, TestCase{Scenario: s, Date: d})
		}
	}
	return cases
}
