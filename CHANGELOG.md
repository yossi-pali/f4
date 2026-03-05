# f4 Changelog — Architecture, Instrumentation & Performance

## Phase 10: Architecture Cleanup — PipelineContext + Split Stage 8

**Files:** `internal/pipeline/context.go`, `internal/stage/*.go`, `cmd/server/main.go`

### 10.1: Move passthrough fields to PipelineContext
- **What:** Added `Filter`, `PreFilterRecheckEntries`, `PendingPackRechecks` fields with nil-safe getters/setters to `PipelineContext`. Stage 2 sets Filter, Stage 4 sets PreFilterRecheckEntries, merge function sets PendingPackRechecks.
- **Why:** Eliminated 15 passthrough field copies across 9 intermediate structs (Stages 3-8 input/output).
- **Before:** `Filter` copied through 8 structs, `PreFilterRecheckEntries` through 4, `PendingPackRechecks` through 3.
- **After:** Each field written once to `PipelineContext`, read by later stages via `pc.Filter()` etc.

### 10.2: Split Stage 8 into MergeAndFilter + SortAndFinalize
- **What:** Extracted merge/dedup/filter/recheck logic into `merge_and_filter.go` (Stage 8a, ~300 lines). Simplified `sort_and_finalize.go` (Stage 8b) to sort-only (~60 lines).
- **Why:** Old `sort_and_finalize.go` was 370 lines with 7 operations. Split improves clarity and testability.
- **Pipeline flow:** `... → HydrateResults → MergeAndFilter → SortAndFinalize → SerializeResponse`
- **New files:** `merge_and_filter.go`, `merge_and_filter_test.go`
- **Zero behavioral change:** All existing tests pass, pipeline output identical.

## Phase 9: Cache trip_pool4_set + Parallel Leg Fetches

**Files:** `internal/repository/trip_pool_set.go`, `internal/stage/assemble_multi_leg.go`, `internal/config/config.go`, `cmd/server/main.go`, `internal/db/connection.go`

- **What (cache):** Added in-memory caching of `trip_pool4_set` per region. On startup, all sets are preloaded (149K rows for TH). Controlled by `CACHE_SETS=true` env var with same TTL as other caches. On cache hit, `FindBySetIDs` is a pure map lookup (~0ms instead of ~280ms DB round-trip).
- **What (parallel):** Combined `fetch_missing_keys` (same-day connection legs) and `fetch_multiday` (multi-day legs) into a single `errgroup` so they run in parallel instead of sequentially.
- **Pre-state:** `buildConnections` did 3 sequential DB calls: FindBySetIDs (~280ms) → fetch_missing_keys (~300ms) → fetch_multiday (~300ms). Total `find_sets` timer: ~1,200ms.
- **Post-state:** FindBySetIDs from cache (~0ms), fetch_missing_keys ∥ fetch_multiday (~310ms). Total `find_sets` timer: ~640ms.
- **Why:** `find_sets` was the largest single hotspot. Set data rarely changes (connection configurations), perfect for caching. Same-day and multi-day fetches are independent queries with distinct trip keys.
- **Impact:** `assemble_multi_leg.find_sets` dropped from **1,200ms → 640ms** (~560ms saved).

## Phase 8: Move Province Lookup to Stage 1

**Files:** `internal/stage/resolve_places.go`, `internal/stage/build_filter.go`, `internal/stage/sort_and_finalize.go`, `internal/domain/filter.go`, `cmd/server/main.go`

- **What:** Moved `GetParentProvinceName` from Stage 8 (sequential DB call) to Stage 1 (parallel errgroup). Province name stored in `SearchFilter.ToProvinceName` and flows through the pipeline. Removed `stationRepo` dependency from `SortAndFinalizeStage`.
- **Pre-state:** Stage 8 called `GetParentProvinceName` as a blocking DB query (~280ms).
- **Post-state:** Runs as a 5th parallel goroutine in Stage 1 (0ms additional cost — overlaps with 4 existing calls). Stage 8 reads from filter: **302ms → 4ms**.
- **Why:** Free optimization — the DB call was already happening in Stage 1's time window.

## Phase 7: Parallel Image Queries in Stage 6

**File:** `internal/stage/collect_ref_data.go`

- **What:** Within the `images` goroutine, `LoadCustomClassImages` and `LoadRouteImages` now run in parallel via inner `errgroup`. `FindClassImages` still runs first (creates the ImageCollection).
- **Pre-state:** 3 sequential image queries: FindClassImages → LoadCustomClassImages → LoadRouteImages (~900ms = 3 × 300ms).
- **Post-state:** FindClassImages, then (LoadCustomClassImages ∥ LoadRouteImages) (~600ms = 2 round-trips).
- **Why:** Custom and route images write to separate fields in ImageCollection — safe for parallel writes.
- **Impact:** `images` goroutine dropped from **900ms → 600ms**, Stage 6 total from **1,238ms → 880ms**.

## Phase 6.2: Parallel Build Filter

**File:** `internal/stage/build_filter.go`

- **What:** Changed `data_sec` and `white_label` queries from sequential to parallel using `errgroup`.
- **Pre-state:** Sequential: data_sec → white_label (~585ms, though data_sec is fast for agentID=0).
- **Post-state:** Both run in parallel. White-label result is applied after both complete.
- **Why:** Both lookups use only `agentID` — fully independent.
- **Impact:** Minimal for default agent (data_sec ~0ms), but saves ~280ms for authenticated agents.

## Phase 6.1: Parallel Resolve Places

**File:** `internal/stage/resolve_places.go`

- **What:** Changed 4 sequential DB calls (`ResolvePlaceToStationIDs` × 2, `GetPlaceData` × 2) to parallel using `errgroup`.
- **Pre-state:** Sequential: from_resolve → to_resolve → from_place_data → to_place_data (~1,200ms total = 4 × 300ms).
- **Post-state:** All 4 run in parallel (~290ms total = 1 round-trip).
- **Why:** Each call is independent (different place IDs). With ~280ms per DB round-trip to remote MySQL, this saves ~900ms.
- **Impact:** Stage 1 dropped from **1,256ms → 291ms**.

## Phase 5.2: Comparator `gocheck` Command

**Files:** `cmd/comparator/main.go`, `cmd/comparator/config.yaml`, `internal/comparator/config.go`

- **What:** Added `gocheck` subcommand comparing baseline Go (8081) vs dev Go (8080). Added `baseline` endpoint to config.yaml.
- **Why:** Automated regression testing — any diffs between baseline and dev indicate a regression.
- **Makefile target:** `compare-go`.

## Phase 5.1: Baseline Docker Setup

**Files:** `docker-compose.baseline.yml`, `Makefile`

- **What:** Added Docker Compose file to run a frozen Go baseline on port 8081 alongside dev on 8080.
- **Why:** Enables regression detection after every change by comparing baseline vs dev responses.
- **Makefile targets:** `baseline-build`, `baseline-up`, `baseline-down`.

## Phase 4: Parallel Multi-Day Key Fetches

**File:** `internal/stage/assemble_multi_leg.go`

- **What:** Changed sequential multi-day leg fetches to parallel using `errgroup`. Each day offset fetches concurrently with `sync.Mutex` protecting the shared `tripByKey` map.
- **Pre-state:** Sequential loop over day offsets, each calling `FindByTripKeys`.
- **Post-state:** `errgroup.Go` for each day offset in parallel.
- **Why:** Multi-day connections with 2-3 day offsets blocked on sequential DB calls. Now runs in parallel.

## Phase 3: Connection Pool Tuning

**Files:** `internal/config/config.go`, `internal/db/connection.go`

- **What:** Made pool settings configurable via env vars: `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME`, `DB_CONN_MAX_IDLE_TIME`.
- **Pre-state:** Hardcoded: MaxOpen=25, MaxIdle=10, Lifetime=5m, IdleTime=1m.
- **Post-state:** Configurable with better defaults: MaxIdle=25 (matches MaxOpen), IdleTime=5m (matches Lifetime).
- **Why:** MaxIdle=10 was too low for 25 open connections on a remote DB — caused frequent reconnects. IdleTime=1m was too aggressive.

## Phase 2 Fix: RefDataCache Config + Ratings + Station Slug

**Files:** `internal/config/config.go`, `internal/refcache/refcache.go`

- **What:**
  1. Fixed Viper `BindEnv` not propagating to nested struct via `Unmarshal` — added explicit `v.GetBool()` reads for all `ref_cache.*` keys.
  2. Fixed `loadAllRatings` query — was referencing non-existent `rating` column. Now uses same JSON_EXTRACT formula as `OperatorRepo.FindOperatorRatings`.
  3. Fixed cached stations missing `StationSlug` — added `slugifyName(st.StationName)` in `loadAllStations`.
  4. Increased refresh timeout from 30s to 2m (263K stations from remote DB needs more time).
- **Why:** Cache was silently disabled due to Viper bug; ratings query failed; station slugs were empty causing diffs.
- **Verified:** 15/15 PASS comparing master baseline vs perf with all caches enabled.

## Phase 2: RefDataCache (Per-Entity In-Memory Cache)

**Files:** `internal/refcache/refcache.go` (new), `internal/config/config.go`, `internal/stage/collect_ref_data.go`, `cmd/server/main.go`

- **What:** Created `RefDataCache` with per-entity toggle via env vars. Each entity cache (operators, stations, classes, integration) can be independently enabled. Background goroutine refreshes at configurable TTL.
- **Env vars:** `CACHE_OPERATORS`, `CACHE_STATIONS`, `CACHE_CLASSES`, `CACHE_INTEGRATION` (all default `false`), `CACHE_REFRESH_TTL` (default `5m`).
- **Stage 6 integration:** Each goroutine checks `refCache.GetX()` first. If cached, uses it; if not, falls back to existing DB query.
- **Why:** Stage 6 makes 9+ parallel DB queries for data that rarely changes. Caching eliminates ~500ms of DB round-trips.
- **Impact:** All caches off = identical behavior to before. Zero risk to enable incrementally.
- **NOT cached:** Images (per operator+class pair, large), weight overrides (per page_url), reasons (language-dependent).

## Phase 1.5: pprof Endpoint

**File:** `cmd/server/main.go`

- **What:** Added `import _ "net/http/pprof"` and started debug HTTP server on `:6060`.
- **Why:** Enables CPU/memory profiling via `go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30`.
- **Impact:** Separate port, no effect on main server.

## Phase 1.4: Structured Logging with zap

**Files:** `internal/api/handler/search.go`, `search_by_stations.go`, `internal/api/router.go`

- **What:** Replaced `log.Printf` with `zap.Logger`. Injected logger into search handlers via constructor. Each request logs: from, to, date, trip count, total duration, and all stage times as structured fields.
- **Why:** Structured JSON logs enable automated analysis and alerting.
- **Pre-state:** `log.Printf("pipeline error: %v", err)`.
- **Post-state:** `logger.Info("search", zap.String("from", ...), zap.Duration("total", ...), zap.Any("stages", ...))`.

## Phase 1.3: HTTP Response Timing Headers

**Files:** `internal/api/handler/search.go`, `search_by_stations.go`

- **What:** After `pipeline.Execute()`, sets `X-Stage-{name}: {ms}ms` and `X-Total-Time: {ms}ms` headers.
- **Why:** Enables performance analysis via `curl -v | grep X-Stage` without log parsing.

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

## Phase 1.1: PipelineContext Sub-Stage Timer

**File:** `internal/pipeline/context.go`

- **What:** Added `RecordSubStageTime(stage, operation, duration)`, `StartTimer(stage, operation)` → `Timer.Stop()` helper. Nil-safe (works when PipelineContext is nil).
- **Why:** Enables fine-grained timing within each pipeline stage using dotted keys like `"query_trips.sql_execute"`.
- **Pre-state:** Only stage-level timing via `RecordStageTime`.
- **Post-state:** Sub-stage timing with zero-allocation Timer helper.

---

## Performance Comparison: Master vs Perf Branch

**Comparison date:** 2026-03-04 | **15/15 PASS, 0 diffs**

| Route | Master (baseline) | Perf (optimized) | Saved |
|-------|------------------|-----------------|-------|
| Bangkok→Chiang Mai | 6.4-7.5s | 3.4s | **~3.3s (48%)** |
| Chiang Mai→Bangkok | 6.0-6.4s | 3.3s | **~2.9s (47%)** |
| Surat Thani→Koh Phangan | 5.0-5.2s | 2.5s | **~2.6s (51%)** |
| Surat Thani→Chiang Mai | 5.8-6.0s | 2.9s | **~2.9s (50%)** |
| Bangkok→Phuket | 6.4-6.7s | 3.3s | **~3.3s (50%)** |

## Perf Branch: Caches ON vs Caches OFF

Same binary, same code (parallelized stages + all optimizations). Only difference: `CACHE_*=true` vs `CACHE_*=false`.

### Server Load Time & Memory Footprint

| Metric | Caches OFF | Caches ON | Notes |
|--------|-----------|----------|-------|
| **Startup time** | <1s | ~36s | Loads 41K operators, 263K stations, 5.3K classes, 149K sets from remote DB |
| **Heap in-use** | 9 MB | 247 MB | +238 MB for cached data |
| **Working set (OS)** | 23 MB | 283 MB | +260 MB RSS |
| **Heap allocated** | 8 MB | 240 MB | Go runtime `HeapAlloc` |
| **Sys (total from OS)** | 19 MB | 299 MB | Includes Go runtime overhead |

**Breakdown of cache memory** (estimated from entry counts × avg struct size):
- Stations (263K entries): ~180 MB (largest — each station has name, slug, full name, coords, province, timezone)
- Sets (149K entries): ~30 MB
- Operators (41K entries): ~20 MB
- Ratings (41K entries): ~5 MB
- Classes (5.3K entries): ~2 MB

**Trade-off:** 260 MB extra memory buys 200-776ms faster responses per request and eliminates 4+ DB round-trips per search.

### Response Time Comparison

**Warm requests, single run per route, date: 2026-03-22**

#### Total Response Time

| Route | Caches OFF | Caches ON | Saved |
|-------|-----------|----------|-------|
| Bangkok→Chiang Mai | 3,599ms | 3,391ms | **208ms** |
| Chiang Mai→Bangkok | 3,758ms | 3,314ms | **444ms** |
| Surat Thani→Koh Phangan | 2,803ms | 2,482ms | **321ms** |
| Surat Thani→Chiang Mai | 3,243ms | 2,923ms | **320ms** |
| Bangkok→Phuket | 4,056ms | 3,280ms | **776ms** |

### Stage Breakdown

| Stage | Caches OFF | Caches ON | Notes |
|-------|-----------|----------|-------|
| **resolve_places** | 285-376ms | 283-327ms | Parallel 5 DB calls; ~1 round-trip |
| **query_trips** | 656-1,354ms | 679-1,100ms | Main SQL; not cached |
| **assemble_multi_leg** | 284-1,367ms | 280-1,011ms | Set cache + parallel fetches |
| → find_sets | 909-1,064ms | 586-698ms | **~330ms saved** (set cache hit) |
| **collect_ref_data** | 928-1,318ms | 927-1,101ms | Operators/stations/classes from cache |
| **sort_and_finalize** | 0-9ms | 0-13ms | Province pre-resolved in Stage 1 |

### Cache Impact by Entity

| Cache | What it eliminates | Entries | Impact |
|-------|-------------------|---------|--------|
| `CACHE_OPERATORS` | operator + seller JOIN in Stage 6 | 41K | ~280ms per search |
| `CACHE_STATIONS` | station + province + timezone queries in Stage 6 | 263K | ~280ms per search |
| `CACHE_CLASSES` | class query in Stage 6 | 5.3K | ~280ms per search |
| `CACHE_INTEGRATION` | integration subquery in Stage 6 | 1 value | minimal |
| `CACHE_SETS` | `FindBySetIDs` DB call in Stage 5a | 149K (TH) | **~280ms per search** |

**Note:** With caches ON, Stage 6 (`collect_ref_data`) still takes ~1s because **images**, **weight_overrides**, **reasons**, and **translations** are NOT cached (per-trip or language-dependent data).

**Remaining hotspots** (Bangkok→Chiang Mai, warm, caches ON):
- `query_trips.sql_execute`: 1,100ms — main trip pool query (10+ JOINs, `price_5_6_pool()` stored function)
- `assemble_multi_leg.find_sets`: 586ms — parallel `fetch_missing_keys` + `fetch_multiday` (1 round-trip each)
- `collect_ref_data.images`: 640ms — 2 sequential round-trips (FindClassImages + parallel load)
- `collect_ref_data.translate`: 280ms — single DB round-trip
- `collect_ref_data.weight_overrides`: 310ms — single DB round-trip
