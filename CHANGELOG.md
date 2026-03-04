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

## Phase 2 Fix: RefDataCache Config + Ratings + Station Slug

**Files:** `internal/config/config.go`, `internal/refcache/refcache.go`

- **What:**
  1. Fixed Viper `BindEnv` not propagating to nested struct via `Unmarshal` — added explicit `v.GetBool()` reads for all `ref_cache.*` keys.
  2. Fixed `loadAllRatings` query — was referencing non-existent `rating` column. Now uses same JSON_EXTRACT formula as `OperatorRepo.FindOperatorRatings`.
  3. Fixed cached stations missing `StationSlug` — added `slugifyName(st.StationName)` in `loadAllStations`.
  4. Increased refresh timeout from 30s to 2m (263K stations from remote DB needs more time).
- **Why:** Cache was silently disabled due to Viper bug; ratings query failed; station slugs were empty causing diffs.
- **Verified:** 15/15 PASS comparing master baseline vs perf with all caches enabled.

## Phase 6: Parallel Resolve Places

**File:** `internal/stage/resolve_places.go`

- **What:** Changed 4 sequential DB calls (`ResolvePlaceToStationIDs` × 2, `GetPlaceData` × 2) to parallel using `errgroup`.
- **Pre-state:** Sequential: from_resolve → to_resolve → from_place_data → to_place_data (~1,200ms total = 4 × 300ms).
- **Post-state:** All 4 run in parallel (~290ms total = 1 round-trip).
- **Why:** Each call is independent (different place IDs). With ~280ms per DB round-trip to remote MySQL, this saves ~900ms.
- **Impact:** Stage 1 dropped from **1,256ms → 291ms**.

## Phase 6: Parallel Build Filter

**File:** `internal/stage/build_filter.go`

- **What:** Changed `data_sec` and `white_label` queries from sequential to parallel using `errgroup`.
- **Pre-state:** Sequential: data_sec → white_label (~585ms, though data_sec is fast for agentID=0).
- **Post-state:** Both run in parallel. White-label result is applied after both complete.
- **Why:** Both lookups use only `agentID` — fully independent.
- **Impact:** Minimal for default agent (data_sec ~0ms), but saves ~280ms for authenticated agents.

## Phase 7: Parallel Image Queries in Stage 6

**File:** `internal/stage/collect_ref_data.go`

- **What:** Within the `images` goroutine, `LoadCustomClassImages` and `LoadRouteImages` now run in parallel via inner `errgroup`. `FindClassImages` still runs first (creates the ImageCollection).
- **Pre-state:** 3 sequential image queries: FindClassImages → LoadCustomClassImages → LoadRouteImages (~900ms = 3 × 300ms).
- **Post-state:** FindClassImages, then (LoadCustomClassImages ∥ LoadRouteImages) (~600ms = 2 round-trips).
- **Why:** Custom and route images write to separate fields in ImageCollection — safe for parallel writes.
- **Impact:** `images` goroutine dropped from **900ms → 600ms**, Stage 6 total from **1,238ms → 880ms**.

## Phase 8: Move Province Lookup to Stage 1

**Files:** `internal/stage/resolve_places.go`, `internal/stage/build_filter.go`, `internal/stage/sort_and_finalize.go`, `internal/domain/filter.go`, `cmd/server/main.go`

- **What:** Moved `GetParentProvinceName` from Stage 8 (sequential DB call) to Stage 1 (parallel errgroup). Province name stored in `SearchFilter.ToProvinceName` and flows through the pipeline. Removed `stationRepo` dependency from `SortAndFinalizeStage`.
- **Pre-state:** Stage 8 called `GetParentProvinceName` as a blocking DB query (~280ms).
- **Post-state:** Runs as a 5th parallel goroutine in Stage 1 (0ms additional cost — overlaps with 4 existing calls). Stage 8 reads from filter: **302ms → 4ms**.
- **Why:** Free optimization — the DB call was already happening in Stage 1's time window.

## Performance Comparison: Master vs Perf Branch

**Comparison date:** 2026-03-04 | **15/15 PASS, 0 diffs**

| Route | Master (baseline) | Perf (optimized) | Saved |
|-------|------------------|-----------------|-------|
| Bangkok→Chiang Mai | 5.7-6.7s | 4.1-4.3s | **~1.7s** |
| Chiang Mai→Bangkok | 5.7-6.2s | 4.2-4.4s | **~1.6s** |
| Surat Thani→Koh Phangan | 4.5-4.6s | 3.0-3.1s | **~1.5s** |
| Surat Thani→Chiang Mai | 5.4-5.5s | 3.8-4.0s | **~1.5s** |
| Bangkok→Phuket | 5.9-6.0s | 4.4-4.6s | **~1.5s** |

**Remaining hotspots** (Bangkok→Chiang Mai, warm):
- `assemble_multi_leg.find_sets`: 1,200ms — largest single query
- `query_trips.sql_execute`: 950ms — main trip pool query
- `collect_ref_data.images`: 600ms — 2 sequential round-trips (create + parallel load)
- `build_filter.white_label`: 290ms — single DB round-trip
