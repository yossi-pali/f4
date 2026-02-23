# f4 — Trip Search Microservice

High-performance Go microservice for transport trip search and aggregation. Migrated from the PHP monolith (`frontend3/`) to a 9-stage pipeline architecture optimized for throughput and latency.

## Architecture

```
                         ┌──────────────────────────────────────────────┐
  HTTP Request           │              SEARCH PIPELINE                │
  GET /api/v1/search     │                                              │
  /{from}/{to}/{date}    │  1. ResolvePlaces                           │
         │               │       ↓                                      │
         ▼               │  2. BuildFilter                             │
    ┌─────────┐          │       ↓                                      │
    │ Router  │────────► │  3. QueryTrips          (regional DB)       │
    │  (Chi)  │          │       ↓                                      │
    └─────────┘          │  4. FilterRawTrips                          │
                         │       ↓                                      │
                         │  ┌────┴────┐                                │
                         │  ▼         ▼         (parallel)             │
                         │  5a. AssembleMultiLeg  5b. EnrichRoundTrips │
                         │  └────┬────┘                                │
                         │       ↓                                      │
                         │  6. CollectRefData     (3 parallel queries) │
                         │       ↓                                      │
                         │  7. HydrateResults                          │
                         │       ↓                                      │
                         │  8. SortAndFinalize                         │
                         │       ↓                                      │
                         │  9. SerializeResponse  → NATS events        │
                         └──────────────────────────────────────────────┘
```

Each stage has explicit input/output contracts via Go generics. Stages 5a and 5b run in parallel using `errgroup`. Stage 6 issues 3 parallel DB queries (operators, stations, classes).

## Project Structure

```
f4/
├── cmd/server/
│   └── main.go                    # Entry point: config, DB, pipeline, HTTP server
├── internal/
│   ├── api/
│   │   ├── router.go              # Chi router with /api/v1 prefix
│   │   ├── handler/
│   │   │   ├── search.go          # GET /api/v1/search/{from}/{to}/{date}
│   │   │   ├── search_by_stations.go  # GET /api/v1/searchByStations/...
│   │   │   ├── health.go          # GET /health
│   │   │   └── admin_search.go    # GET/POST /api/v1/admin/search/...
│   │   ├── middleware/
│   │   │   ├── logging.go         # Request logging (zap)
│   │   │   ├── metrics.go         # Request metrics
│   │   │   ├── recovery.go        # Panic recovery
│   │   │   └── agent.go           # Agent context extraction
│   │   └── response/
│   │       └── response.go        # Response formatting
│   ├── cache/
│   │   ├── cache.go               # Cache interface
│   │   ├── redis.go               # Redis implementation
│   │   └── keys.go                # Cache key generation
│   ├── config/
│   │   └── config.go              # Viper-based config loading
│   ├── db/
│   │   ├── connection.go          # ConnectionManager (default + regional pools)
│   │   ├── region.go              # RegionResolver (station → region mapping)
│   │   └── retry.go               # MySQL deadlock retry (max 5x, 100ms)
│   ├── domain/                    # Domain models (no dependencies)
│   │   ├── trip.go                # RawTrip, TripResult, Segment, TravelOption
│   │   ├── price.go               # TripPrice, PriceFare, PriceDeltaFare
│   │   ├── filter.go              # SearchFilter, SearchParams, AgentContext
│   │   ├── place.go               # Station, Province, Place
│   │   ├── operator.go            # Operator, Seller
│   │   ├── class.go               # VehicleClass
│   │   ├── autopack.go            # AutopackConfig, AutopackRoute, AutopackLeg
│   │   ├── event.go               # Domain event structs
│   │   └── recheck.go             # RecheckCollection, BuyItem
│   ├── event/
│   │   ├── publisher.go           # Publisher interface
│   │   ├── nats.go                # NATS JetStream implementation
│   │   └── noop.go                # No-op fallback
│   ├── feature/
│   │   ├── flags.go               # Feature flag management
│   │   └── provider.go            # Feature provider
│   ├── pipeline/
│   │   ├── pipeline.go            # Run, RunParallelMerge
│   │   ├── stage.go               # Stage[In, Out] interface
│   │   └── context.go             # PipelineContext with stage timings
│   ├── price/
│   │   ├── decoder.go             # Binary price parser (PHP port)
│   │   ├── decoder_test.go        # Decoder unit tests
│   │   └── currency.go            # FX code index → currency string
│   ├── repository/
│   │   ├── trip_pool.go           # Main search query (trip_pool4 + joins)
│   │   ├── station.go             # Station/province resolution
│   │   ├── operator.go            # Operator batch load
│   │   ├── class.go               # Vehicle class batch load
│   │   ├── trip_pool_set.go       # Multi-leg connection sets
│   │   ├── round_trip_price.go    # Round-trip price cache
│   │   ├── autopack.go            # Landing alternatives
│   │   ├── data_sec.go            # Security restrictions
│   │   ├── white_label.go         # White-label partner config
│   │   ├── page_override.go       # Admin page overrides
│   │   └── integration.go         # Integration metadata
│   └── stage/
│       ├── search_pipeline.go     # Pipeline orchestration
│       ├── resolve_places.go      # Stage 1
│       ├── build_filter.go        # Stage 2
│       ├── query_trips.go         # Stage 3
│       ├── filter_raw_trips.go    # Stage 4
│       ├── assemble_multi_leg.go  # Stage 5a
│       ├── enrich_round_trips.go  # Stage 5b
│       ├── enrich_round_trips_test.go
│       ├── collect_ref_data.go    # Stage 6
│       ├── hydrate_results.go     # Stage 7
│       ├── sort_and_finalize.go   # Stage 8
│       └── serialize_response.go  # Stage 9
├── Makefile
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── go.sum
├── .env
└── .dockerignore
```

## Pipeline Stages

| # | Stage | Input | Output | Description |
|---|-------|-------|--------|-------------|
| 1 | **ResolvePlaces** | Place IDs (e.g. `1p`, `44s`) | Station IDs, place data, distance | Resolves place/station identifiers to station ID arrays. Calculates haversine distance. |
| 2 | **BuildFilter** | Resolved places + HTTP params | `SearchFilter` | Merges security restrictions (`data_sec`), white-label filters, feature flags, agent context, and query params into a complete filter. |
| 3 | **QueryTrips** | `SearchFilter` | Raw trips + binary prices | Executes main SQL against regional `trip_pool4` with `STRAIGHT_JOIN`. Calls `price_5_6_pool()` stored function. Decodes binary price strings. |
| 4 | **FilterRawTrips** | Raw trips | Direct trips + connection set IDs | Removes hidden departures, meta operators, daytrip duplicates. Separates connections (set_id > 0) from direct trips. |
| 5a | **AssembleMultiLeg** | Direct trips + set IDs | Connections + autopacks | Builds 2-3 leg connections from `trip_pool4_set`. Matches autopack legs from `landing_alternatives`. Validates transit times. |
| 5b | **EnrichRoundTrips** | Direct trips + outbound ref | Discounted inbound trips | Looks up round-trip prices from cache. Calculates discount % and adjusts inbound total. Publishes event on cache miss. |
| 6 | **CollectRefData** | All trips | Trips + reference maps | Batch-loads operators, stations, and vehicle classes in 3 parallel goroutines (`errgroup`). |
| 7 | **HydrateResults** | Enriched trips | `TripResult` DTOs | Builds segments, travel options, tags (amenities, ticket type, baggage, refundable, special deal). |
| 8 | **SortAndFinalize** | Hydrated results | Deduped + sorted results | Merges duplicate travel options, filters invalid prices (unless admin), sorts by: bookable → valid price → special deal → rank score. |
| 9 | **SerializeResponse** | Final results | JSON response + events | Builds recheck URLs (batched, max 50 keys). Publishes NATS events: `search.completed`, `search.needs_recheck`, `search.needs_round_trip_prices`. |

## API Endpoints

### Search by Place

```
GET /api/v1/search/{fromPlaceID}/{toPlaceID}/{date}
```

Place IDs use suffix `p` (province) or `s` (station): `1p`, `44p`, `123s`.

**Query Parameters:**

| Param | Default | Description |
|-------|---------|-------------|
| `a` or `seats` | 1 | Adult seats |
| `c` | 0 | Child seats |
| `i` | 0 | Infant seats |
| `l` | — | Language code |
| `fxcode` | — | Currency (e.g. `USD`, `THB`) |
| `direct` | — | `1` for direct trips only |
| `r` | — | `1` for recheck mode |
| `recheck_amount` | 0 | Number of trips to recheck |
| `cart_hash` | — | Cart hash for round-trip |
| `outbound_trip_ref` | — | Outbound trip key for round-trip |
| `only_pairs` | — | `1` to limit to requested station pairs |
| `vehclass_id` | — | Filter by vehicle class |
| `integration_code` | — | Filter by integration |
| `with_non_bookable` | — | `1` to include non-bookable trips |
| `extended` | — | `1` for extended response format |

**Example:**

```bash
curl "http://localhost:8080/api/v1/search/1p/44p/2026-03-22?seats=1&fxcode=USD"
```

**Response:**

```json
{
  "trips": [
    {
      "TripKey": "TH013p030l...",
      "GroupKey": "4055-11553-1245-600",
      "Segments": [
        {
          "FromStationID": 4055,
          "ToStationID": 11553,
          "Departure": "2026-03-22 20:45:00",
          "Arrival": "2026-03-23 06:45:00",
          "Duration": 600,
          "OperatorID": 1754,
          "ClassID": 4,
          "VehclassID": "bus",
          "Type": "route"
        }
      ],
      "TravelOptions": [
        {
          "Price": {
            "IsValid": true,
            "Total": 25.50,
            "FXCode": "USD",
            "Avail": 5,
            "PriceLevel": 1
          },
          "AvailableSeats": 5,
          "TripKey": "TH013p030l...",
          "IntegrationCode": "thairoute",
          "DepartureTime": 1245
        }
      ],
      "Tags": ["Aircon", "TV", "ticket:show_on_screen"],
      "IsBookable": true,
      "HasValidPrice": true,
      "RankScore": 1016.12
    }
  ],
  "operators": { "1754": { "ID": 1754, "Name": "Thai Route" } },
  "stations": { "4055": { "ID": 4055, "Name": "Bangkok" } },
  "classes": { "4": { "ID": 4, "Name": "VIP" } },
  "stageTimes": {
    "resolve_places": 0.002,
    "query_trips": 3.41,
    "hydrate_results": 0.001
  }
}
```

### Search by Stations

```
GET /api/v1/searchByStations/{fromStations}/{toStations}/{date}
```

Station IDs are dash-separated: `/api/v1/searchByStations/100-200/300-400/2026-03-22`

### Health Check

```
GET /health
```

Returns `{"status":"ok"}`.

### Admin Search

```
GET  /api/v1/admin/search/{fromPlaceID}/{toPlaceID}/{date}
POST /api/v1/admin/search/{fromPlaceID}/{toPlaceID}
```

## Database

### Regional Sharding

Trip data is sharded across regional MySQL databases. The region is resolved from station country at runtime:

| Region | Env Variable | Countries |
|--------|-------------|-----------|
| Default | `DB_DEFAULT_DSN` | — (metadata: stations, operators, classes) |
| Thailand | `DB_TRIPPOOL_TH_DSN` | TH, LA, KH, MM, VN |
| India | `DB_TRIPPOOL_IN_DSN` | IN, NP, LK, BD |
| Europe | `DB_TRIPPOOL_EU_DSN` | EU countries |
| Asia 1 | `DB_TRIPPOOL_ASIA1_DSN` | MY, SG, ID, PH |
| Asia 2 | `DB_TRIPPOOL_ASIA2_DSN` | CN, JP, KR, TW |

### Key Tables

**Default DB:**

| Table | Purpose |
|-------|---------|
| `station` | Station master data (name, lat/lng, timezone, country) |
| `province` | Province/region hierarchy |
| `operator` | Transport operator details |
| `class` | Vehicle class definitions |
| `search_station`, `search_province` | Place ID → station ID mapping |
| `data_sec`, `data_sec_role`, `usr` | Agent security restrictions |
| `whitelabel` | White-label partner config |
| `integration` | Integration partner metadata |
| `country_region` | Country → region mapping |

**Regional Trip Pool DB:**

| Table | Purpose |
|-------|---------|
| `trip_pool4` | Trip definitions (trip_key, operator, class, stations, times) |
| `trip_pool4_price` | Date-specific pricing (binary encoded via `price_5_6_pool()`) |
| `trip_pool4_extra` | Trip metadata (amenities, ticket type, baggage, refundable) |
| `trip_pool4_departure_extra` | Departure-specific rank scores |
| `trip_pool4_set` | Multi-leg connection definitions |
| `trip_pool4_round_trip_price` | Round-trip price cache (20 min TTL) |
| `landing_alternatives` | Autopack configurations (JSON routes) |

### Connection Pooling

```
MaxOpenConns:    25
MaxIdleConns:    10
ConnMaxLifetime: 5 minutes
ConnMaxIdleTime: 1 minute
```

Automatic retry on MySQL deadlocks (error codes 1205, 1213): max 5 retries, 100ms delay.

## Price Decoder

Prices are stored as binary strings produced by MySQL stored function `price_5_6_pool()`. The Go decoder (`internal/price/decoder.go`) is a port of PHP's `PriceBinaryParser`.

### Binary Format

**Header (17 bytes):**

```
Offset  Size  Type    Field
0       1     uint8   flags (valid, validByTTL, validByML, experiment, outdated, blockOutput)
1       1     uint8   avail (available seats)
2       1     uint8   priceLevel (0=none, 1=exact, 2=anyOnDate, 3=predict)
3       1     uint8   reasonID
4       4     uint32  reasonParam (LE)
8       4     uint32  stamp (LE)
12      1     uint8   fxCode index (maps to currency string via 171-entry table)
13      4     uint32  total in cents (LE, divide by 100)
```

**Fare (25 bytes × 3 = 75 bytes):** adult, child, infant — each with fullPrice, netPrice, topup, sysFee, agFee (currency index + uint32 value).

**Footer (20 bytes):** mlScore, duration, advHour, legacyRouteID, legacyTripID.

**Total normal format:** 17 + 75 + 20 = **112 bytes**.

**Short format:** Only 17-byte header (from `price_5_6_pool()` compact output when fare breakdown unavailable).

**Extended formats:** 184 bytes (1 delta per fare), 256 bytes (2 deltas per fare), or block-output format (version 4+).

## Configuration

All configuration via environment variables (loaded by Viper from `.env`):

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_PORT` | `8080` | HTTP server port |
| `SERVER_READ_TIMEOUT` | `30s` | HTTP read timeout |
| `SERVER_WRITE_TIMEOUT` | `60s` | HTTP write timeout |
| `DB_DEFAULT_DSN` | — | Default MySQL DSN (metadata) |
| `DB_TRIPPOOL_TH_DSN` | — | Thailand trip pool DSN |
| `DB_TRIPPOOL_IN_DSN` | — | India trip pool DSN |
| `DB_TRIPPOOL_EU_DSN` | — | Europe trip pool DSN |
| `DB_TRIPPOOL_ASIA1_DSN` | — | Asia 1 trip pool DSN |
| `DB_TRIPPOOL_ASIA2_DSN` | — | Asia 2 trip pool DSN |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `REDIS_PASSWORD` | — | Redis password |
| `REDIS_DB` | `0` | Redis database number |
| `NATS_URL` | `nats://localhost:4222` | NATS event bus URL |
| `RECHECK_BASE_URL` | — | Recheck service base URL |
| `FEATURE_ROUND_TRIPS` | `true` | Enable round-trip pricing |
| `FEATURE_AUTOPACKS` | `true` | Enable autopack assembly |
| `FEATURE_MULTISELLER` | `true` | Enable multi-seller handling |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `METRICS_PORT` | `9090` | Prometheus metrics port |
| `OTEL_EXPORTER_ENDPOINT` | `localhost:4317` | OpenTelemetry exporter |

## Getting Started

### Prerequisites

- Go 1.26+
- MySQL database access (trip pool + metadata)

### Setup

1. Clone the repository
2. Copy and configure environment:
   ```bash
   cp .env.example .env
   # Edit .env with your database DSNs
   ```
3. Build and run:
   ```bash
   make run
   ```
4. Test:
   ```bash
   curl "http://localhost:8080/api/v1/search/1p/44p/2026-03-22?seats=1"
   ```

### Makefile Targets

| Target | Command | Description |
|--------|---------|-------------|
| `make build` | `go build -o f4 ./cmd/server` | Build binary |
| `make run` | Build + `./f4` | Build and run |
| `make test` | `go test ./...` | Run all tests |
| `make test-v` | `go test -v -count=1 ./...` | Verbose, no cache |
| `make test-fresh` | `go test -count=1 ./...` | No cache |
| `make vet` | `go vet ./...` | Static analysis |
| `make clean` | `rm -f f4` | Remove binary |

## Docker

### Build and Run

```bash
docker compose up --build
```

The Dockerfile uses a multi-stage build:

1. **Builder stage** — `golang:1.26-alpine`, compiles static binary
2. **Runtime stage** — `alpine:3.21` with `ca-certificates` and `tzdata`

Final image is ~15MB.

### Manual Docker Build

```bash
docker build -t f4 .
docker run --rm -p 8080:8080 --env-file .env f4
```

## Testing

```bash
make test          # Run all tests
make test-fresh    # Run without cache
```

### Test Coverage

| Package | Test File | Coverage |
|---------|-----------|----------|
| `internal/price` | `decoder_test.go` | Binary price decoder: header fields, fare fields, delta fares, signed values, FX code index, short/normal/block formats |
| `internal/stage` | `enrich_round_trips_test.go` | Round-trip enrichment: no outbound passthrough, cache miss event publishing, discount calculation |

## Load Testing

The k6 load test at `docs/k6/burst_test.js` tests 5 route pairs with burst traffic:

```
Route pairs: 1↔44, 44↔1, 73↔78, 73↔44, 1↔88
```

**Burst pattern:**

| Phase | VUs | Duration |
|-------|-----|----------|
| Warmup | 5 | 30s |
| Baseline | 10 | 20s |
| Burst 1 | 200 | 20s |
| Recovery | 10 | 20s |
| Burst 2 | 350 | 20s |
| Recovery | 10 | 20s |
| Burst 3 | 500 | 20s |
| Cooldown | 10 | 20s |

**Thresholds:**

- p95 latency < 2000ms
- Error rate < 1%

```bash
k6 run docs/k6/burst_test.js --env BASE_URL=http://localhost:8080 --env API_KEY=your_key
```

## Dependencies

| Library | Purpose |
|---------|---------|
| [chi](https://github.com/go-chi/chi) | Lightweight HTTP router |
| [sqlx](https://github.com/jmoiron/sqlx) | SQL extensions (named params, struct scanning) |
| [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql) | MySQL driver |
| [go-redis](https://github.com/redis/go-redis) | Redis client |
| [nats.go](https://github.com/nats-io/nats.go) | NATS event bus client |
| [viper](https://github.com/spf13/viper) | Configuration management |
| [zap](https://go.uber.org/zap) | Structured logging |
| [x/sync](https://golang.org/x/sync) | errgroup for parallel execution |
