# Comparator — Legacy vs f4 Search Comparison Tool

CLI tool that compares search results between the legacy PHP endpoint and the new Go f4 endpoint, field by field.

## Quick Start

```bash
# Run all test cases (scenarios × dates)
make compare

# Or with a custom config
go run ./cmd/comparator run --config path/to/config.yaml

# Clean old results (keep latest)
make compare-clean

# Clean everything
go run ./cmd/comparator clean --keep 0
```

## Commands

### `run`

Executes all test cases and produces raw JSON responses + field-by-field diffs.

```
go run ./cmd/comparator run [--config path]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `cmd/comparator/config.yaml` | Path to YAML config file |

Each test case = one scenario × one date. Legacy and new endpoint requests run in parallel.

### `clean`

Removes old test result directories.

```
go run ./cmd/comparator clean [--keep N] [--raw] [--diff]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--keep` | `1` | Number of recent runs to keep. `--keep 0` deletes all. |
| `--raw` | `false` | Only delete `raw/` folders |
| `--diff` | `false` | Only delete `diff/` folders |

## Configuration

See [config.yaml](config.yaml) for the full example. Key sections:

### `endpoints`

Two named endpoints: `legacy` and `new`. Each has:
- `base_url` — server address
- `headers` — HTTP headers added to every request (e.g., `Host`)
- `params` — query params appended to every request (e.g., `ref`)

### `dates`

Array of dates to test. Each scenario runs once per date.

### `float_tolerance`

Absolute tolerance for float comparisons (prices, ratings, coordinates). Default `0.01`.

### `scenarios`

Each scenario defines:
- `name` — human-readable label (also used for filenames)
- `type` — `"place"` uses `/api/v1/search/{from}/{to}/{date}`, `"station"` uses `/api/v1/searchByStations/{from}/{to}/{date}`
- `from`, `to` — place IDs (e.g., `1p`, `44p`) or station IDs (e.g., `123-456`)
- `params` — scenario-specific query params (e.g., `seats: "1"`)

## Output Structure

```
test-results/
  2026-02-23T14-30-00/
    raw/
      bangkok-to-chiang-mai_2026-03-23-legacy.json
      bangkok-to-chiang-mai_2026-03-23-new.json
      ...
    diff/
      bangkok-to-chiang-mai_2026-03-23.json
      ...
    summary.json
```

### Diff Format

Each diff file contains:

```json
{
  "scenario": "Bangkok to Chiang Mai",
  "date": "2026-03-23",
  "legacy_status": 200,
  "new_status": 200,
  "summary": {
    "trips_legacy": 45,
    "trips_new": 43,
    "trips_matched": 42,
    "trips_only_in_legacy": 3,
    "trips_only_in_new": 1,
    "fields_compared": 850,
    "fields_different": 12
  },
  "only_in_legacy": ["trip_key_1", "trip_key_2"],
  "only_in_new": ["trip_key_3"],
  "differences": [
    {
      "trip_key": "abc-123",
      "path": "travel_options[abc-123|OTA].price.total",
      "legacy": 29.50,
      "new": 29.49
    }
  ]
}
```

## Matching Strategy

- **Trips** — matched by `trip_key` (not array index)
- **Travel options** — matched by `trip_key` + `integration_code`
- **Segments** — matched by array index (order is meaningful)
- **Stations / operators / classes** — matched by dictionary key (ID)
- **Recheck URLs** — compared as sorted string sets
