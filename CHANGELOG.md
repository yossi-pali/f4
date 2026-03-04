# f4 Changelog — Architecture, Instrumentation & Performance

## Phase 5.1: Baseline Docker Setup

**Files:** `docker-compose.baseline.yml`, `Makefile`

- **What:** Added Docker Compose file to run a frozen Go baseline on port 8081 alongside dev on 8080.
- **Why:** Enables regression detection after every change by comparing baseline vs dev responses.
- **Makefile targets:** `baseline-build`, `baseline-up`, `baseline-down`.

## Phase 5.2: Comparator `gocheck` Command

**Files:** `cmd/comparator/main.go`, `cmd/comparator/config.yaml`, `internal/comparator/config.go`

- **What:** Added `gocheck` subcommand comparing baseline Go (8081) vs dev Go (8080). Added `baseline` endpoint to config.yaml.
- **Why:** Automated regression testing — any diffs between baseline and dev indicate a regression.
- **Makefile target:** `compare-go`.

## Phase 1.1: PipelineContext Sub-Stage Timer

**File:** `internal/pipeline/context.go`

- **What:** Added `RecordSubStageTime(stage, operation, duration)`, `StartTimer(stage, operation)` → `Timer.Stop()` helper. Nil-safe (works when PipelineContext is nil).
- **Why:** Enables fine-grained timing within each pipeline stage using dotted keys like `"query_trips.sql_execute"`.
- **Pre-state:** Only stage-level timing via `RecordStageTime`.
- **Post-state:** Sub-stage timing with zero-allocation Timer helper.

## Phase 1.2: Stage Instrumentation

**Files:** All 10 stage files in `internal/stage/`

- **What:** Added `pc.StartTimer(stage, operation)` / `t.Stop()` calls in each stage:
  - Stage 1: `from_resolve`, `to_resolve`
  - Stage 2: `data_sec`, `white_label`
  - Stage 3: `sql_execute` (replaces manual `time.Now()`)
  - Stage 4: `station_collect`, `filter_loop`
  - Stage 5a: `find_sets`, `fetch_missing_keys`, `fetch_multiday`, `assembly`, `autopacks`
  - Stage 5b: `price_lookup`, `discount_apply`
  - Stage 6: Per-goroutine timing for `operators`, `ratings`, `stations`, `classes`, `reasons`, `integration`, `logos`, `images`, `weight_overrides`, then `translate`
  - Stage 7: `hydrate_loop`
  - Stage 8: `province_lookup`, `merge_loop`, `sort`, `recheck_groups`
  - Stage 9: `recheck_urls`, `events`
- **Why:** Identifies exactly where time is spent within each stage.
- **Impact:** No behavior change. Adds ~100ns overhead per timer call.

## Phase 1.3: HTTP Response Timing Headers

**Files:** `internal/api/handler/search.go`, `search_by_stations.go`

- **What:** After `pipeline.Execute()`, sets `X-Stage-{name}: {ms}ms` and `X-Total-Time: {ms}ms` headers.
- **Why:** Enables performance analysis via `curl -v | grep X-Stage` without log parsing.

## Phase 1.4: Structured Logging with zap

**Files:** `internal/api/handler/search.go`, `search_by_stations.go`, `internal/api/router.go`

- **What:** Replaced `log.Printf` with `zap.Logger`. Injected logger into search handlers via constructor. Each request logs: from, to, date, trip count, total duration, and all stage times as structured fields.
- **Why:** Structured JSON logs enable automated analysis and alerting.
- **Pre-state:** `log.Printf("pipeline error: %v", err)`.
- **Post-state:** `logger.Info("search", zap.String("from", ...), zap.Duration("total", ...), zap.Any("stages", ...))`.

## Phase 1.5: pprof Endpoint

**File:** `cmd/server/main.go`

- **What:** Added `import _ "net/http/pprof"` and started debug HTTP server on `:6060`.
- **Why:** Enables CPU/memory profiling via `go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30`.
- **Impact:** Separate port, no effect on main server.

## Phase 3: Connection Pool Tuning

**Files:** `internal/config/config.go`, `internal/db/connection.go`

- **What:** Made pool settings configurable via env vars: `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME`, `DB_CONN_MAX_IDLE_TIME`.
- **Pre-state:** Hardcoded: MaxOpen=25, MaxIdle=10, Lifetime=5m, IdleTime=1m.
- **Post-state:** Configurable with better defaults: MaxIdle=25 (matches MaxOpen), IdleTime=5m (matches Lifetime).
- **Why:** MaxIdle=10 was too low for 25 open connections on a remote DB — caused frequent reconnects. IdleTime=1m was too aggressive.

## Phase 2: RefDataCache (Per-Entity In-Memory Cache)

**Files:** `internal/refcache/refcache.go` (new), `internal/config/config.go`, `internal/stage/collect_ref_data.go`, `cmd/server/main.go`

- **What:** Created `RefDataCache` with per-entity toggle via env vars. Each entity cache (operators, stations, classes, integration) can be independently enabled. Background goroutine refreshes at configurable TTL.
- **Env vars:** `CACHE_OPERATORS`, `CACHE_STATIONS`, `CACHE_CLASSES`, `CACHE_INTEGRATION` (all default `false`), `CACHE_REFRESH_TTL` (default `5m`).
- **Stage 6 integration:** Each goroutine checks `refCache.GetX()` first. If cached, uses it; if not, falls back to existing DB query.
- **Why:** Stage 6 makes 9+ parallel DB queries for data that rarely changes. Caching eliminates ~500ms of DB round-trips.
- **Impact:** All caches off = identical behavior to before. Zero risk to enable incrementally.
- **NOT cached:** Images (per operator+class pair, large), weight overrides (per page_url), reasons (language-dependent).

## Phase 4: Parallel Multi-Day Key Fetches

**File:** `internal/stage/assemble_multi_leg.go`

- **What:** Changed sequential multi-day leg fetches to parallel using `errgroup`. Each day offset fetches concurrently with `sync.Mutex` protecting the shared `tripByKey` map.
- **Pre-state:** Sequential loop over day offsets, each calling `FindByTripKeys`.
- **Post-state:** `errgroup.Go` for each day offset in parallel.
- **Why:** Multi-day connections with 2-3 day offsets blocked on sequential DB calls. Now runs in parallel.
