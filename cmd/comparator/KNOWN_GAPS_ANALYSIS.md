# Known Gaps Analysis — PHP vs Go Search API

**Date:** 2026-03-04
**Test Run:** `2026-03-04T13-20-58` (golive mode)
**Result:** 15/15 PASS, 0 field diffs, 112 known gaps total

```
known_gap_totals:
  orphan_station:      43
  orphan_operator:     30
  orphan_class:        24
  orphan_recheck_url:   3
  pack_recheck_urls:   12
```

---

## Table of Contents

1. [Orphan Stations (43)](#1-orphan-stations-43)
2. [Orphan Operators (30)](#2-orphan-operators-30)
3. [Orphan Classes (24)](#3-orphan-classes-24)
4. [Orphan Recheck URLs (3)](#4-orphan-recheck-urls-3)
5. [Pack Recheck URLs (12)](#5-pack-recheck-urls-12)
6. [Cross-Gap Correlations](#6-cross-gap-correlations)
7. [Recommendations](#7-recommendations)

---

## 1. Orphan Stations (43)

### What

Station entries present in PHP's `stations` dictionary but missing from Go's response.
These stations are not referenced by any visible trip in the response.

### Distribution Across Scenarios

| Scenario               | Dates | Orphan Stations per Date | IDs |
|------------------------|-------|--------------------------|-----|
| Bangkok → Chiang Mai   | 3     | 1 each                  | 5943 |
| Chiang Mai → Bangkok   | 3     | 1–2 each                | 5943, 5955 |
| Surat Thani → Chiang Mai | 3  | 9 each                  | 5936, 5937, 5943, 4055, 4057, 36429, 106904, 165135, 280287 |
| Bangkok → Phuket       | 3     | 3 each                  | 5939, 5943, 5955 |
| Surat Thani → Koh Phangan | 3  | 0                       | — |

**Station 5943 appears in 12/15 cases** — it is the most common orphan.

### Root Cause

**PHP collects station IDs from connection legs (transit1/transit2); Go does not.**

#### PHP — `frontend3/src/TripSearch/Component/Search.php` (prepareRawTrips, lines 712-721)

```php
$this->collectors->stationCollector->addById((int)$rawTrip['dep_station_id']);
$this->collectors->stationCollector->addById((int)$rawTrip['arr_station_id']);
if ($rawTrip['transit1_trip_key']) {
    $this->collectors->stationCollector->addById((int)$rawTrip['transit1_dep_station_id']);
    $this->collectors->stationCollector->addById((int)$rawTrip['transit1_arr_station_id']);
    if (!empty($rawTrip['transit2_trip_key'])) {
        $this->collectors->stationCollector->addById((int)$rawTrip['transit2_dep_station_id']);
        $this->collectors->stationCollector->addById((int)$rawTrip['transit2_arr_station_id']);
    }
}
```

PHP iterates **ALL raw trips** (including connections with `set_id > 0`) and collects:
- dep/arr station from the main trip
- dep/arr stations from transit1 leg
- dep/arr stations from transit2 leg

This happens **before** any filtering (meta, daytrip, connection assembly).

#### Go — `internal/stage/filter_raw_trips.go` (lines 44-60) + `internal/stage/collect_ref_data.go` (lines 92-120)

Go's station collection has two sources:

1. **Pre-filter stations** (filter_raw_trips.go:48-56): Collects dep/arr from all non-hidden trips BEFORE meta/daytrip filtering. But only collects the **head trip's** dep/arr stations — NOT transit1/transit2 legs.

2. **Post-filter stations** (collect_ref_data.go:105-106): Collects dep/arr from `AllTrips` (filtered trips only).

**The gap:** Go never reads `transit1_dep_station_id`, `transit1_arr_station_id`, `transit2_dep_station_id`, `transit2_arr_station_id` for station collection. These intermediate-leg station IDs end up in PHP's dictionary but not Go's.

### Is the "Known Gap" Classification Correct?

**Yes, with a caveat.** The orphan stations in PHP are unreferenced by any visible trip — no trip in the response uses these station IDs as dep/arr. They exist purely because PHP's collector pattern is greedy (collects from connection legs that get reassembled or filtered out).

**However:** The client never needs these stations for the initial response. They are dead entries in the dictionary. When a recheck URL is triggered later, the recheck response provides its own station dictionary. So these orphan stations are **unnecessary overhead in PHP's response**, not a gap in Go.

**Flag: The current classification is correct.** Go's behavior is actually cleaner — it only includes stations that are referenced by visible trips or recheck URLs. PHP includes extra stations that serve no purpose.

---

## 2. Orphan Operators (30)

### What

Operator entries present in PHP's `operators` dictionary but missing from Go's response.

### Distribution

| Scenario               | Dates | Orphan Operators per Date | IDs |
|------------------------|-------|---------------------------|-----|
| Bangkok → Chiang Mai   | 3     | 0                        | — |
| Chiang Mai → Bangkok   | 3     | 0                        | — |
| Surat Thani → Chiang Mai | 3  | 8 each                   | 1, 4, 42, 3125, 3178, 27053, 28424, 30884 |
| Bangkok → Phuket       | 3     | 2 each                   | 4, 44139 |
| Surat Thani → Koh Phangan | 3  | 0                       | — |

### Root Cause

**PHP collects operator IDs from connection legs AND from trips that are later filtered out.
Go explicitly excludes connection trips from operator collection.**

#### PHP — `Search.php` (lines 724-739)

```php
$effectiveOperatorId = !empty($rawTrip['effective_operator_id'])
    ? $rawTrip['effective_operator_id']
    : $rawTrip['operator_id'];
$this->collectors->operatorCollector->addById((int)$effectiveOperatorId);
if ($rawTrip['transit1_trip_key']) {
    // Also collects transit1 operator
    $effectiveOperatorId1 = ...;
    $this->collectors->operatorCollector->addById((int)$effectiveOperatorId1);
    // And transit2 operator if present
}
```

PHP collects operators from ALL raw trips including connection legs (transit1/transit2 operators).

#### Go — `collect_ref_data.go` (lines 92-104)

```go
for _, t := range in.AllTrips {
    isConnection := t.SetID != nil && *t.SetID > 0
    if !isConnection {
        operatorIDSet[t.OperatorID] = struct{}{}  // Only direct trips
        classIDSet[t.ClassID] = struct{}{}
    }
    // stations are collected from ALL trips (including connections)
}
```

Go **intentionally** excludes connection trips (`set_id > 0`) from operator collection.
This is documented in the code comment: "Assembled connections may include operators that PHP filters out during connection assembly."

### Is the "Known Gap" Classification Correct?

**Yes.** The orphan operators are from connection legs or meta trips. No visible trip in the response references these operator IDs. The same reasoning as stations applies — these are dead entries in PHP's dictionary.

**Interesting detail for Surat Thani → Chiang Mai:** This route has **0 visible trips** in both PHP and Go responses, yet PHP returns 8 orphan operators. This means PHP's raw query returns connection/meta trips for this route, collects their operators, then filters them ALL out. The operators exist in the response dictionary with zero trips referencing them.

---

## 3. Orphan Classes (24)

### What

Vehicle class entries present in PHP's `classes` (vehclasses) dictionary but missing from Go's.

### Distribution

| Scenario               | Dates | Orphan Classes per Date | IDs |
|------------------------|-------|-------------------------|-----|
| Bangkok → Chiang Mai   | 3     | 0                      | — |
| Chiang Mai → Bangkok   | 3     | 0                      | — |
| Surat Thani → Chiang Mai | 3  | 7 each                 | 3, 1721, 2485, 3329, 3796, 4385, 7525 |
| Bangkok → Phuket       | 3     | 1 each                 | 21 |
| Surat Thani → Koh Phangan | 3  | 0                     | — |

### Root Cause

**Identical to operators.** PHP collects class IDs from connection legs (transit1_class_id, transit2_class_id). Go excludes connections from class collection.

#### PHP — `Search.php` (lines 770-776)

```php
$this->collectors->classCollector->addById((int)$rawTrip['class_id']);
if (!empty($rawTrip['transit1_class_id'])) {
    $this->collectors->classCollector->addById((int)$rawTrip['transit1_class_id']);
    if (!empty($rawTrip['transit2_class_id'])) {
        $this->collectors->classCollector->addById((int)$rawTrip['transit2_class_id']);
    }
}
```

#### Go — Same exclusion as operators (collect_ref_data.go:100-102)

### Is the "Known Gap" Classification Correct?

**Yes.** Same as operators — dead entries from connection legs. Class 21 (Bangkok→Phuket) and classes 3, 1721, 2485, etc. (Surat→CM) belong to transit legs that are never shown as standalone trips.

---

## 4. Orphan Recheck URLs (3)

### What

One extra `/searchr` recheck URL per date in PHP's response for the Bangkok → Phuket route.
Go does not generate this URL.

### Concrete Data

All 3 orphan URLs have the same station pair, differing only by date:

```
/searchr?l=en&f=5936&t=5957&d=2026-03-23&sa=1&sc=0&si=0&a=1&search_url=...
/searchr?l=en&f=5936&t=5957&d=2026-04-15&sa=1&sc=0&si=0&a=1&search_url=...
/searchr?l=en&f=5936&t=5957&d=2026-05-20&sa=1&sc=0&si=0&a=1&search_url=...
```

**Station pair:** `f=5936` → `t=5957` (Bangkok Khaosan → Phuket Town, likely)

### Root Cause

**PHP generates recheck URLs from trips that Go doesn't have in its recheck pool.**

#### How Recheck URLs Work

The search response includes recheck URLs for trip "slots" where the price is invalid (expired, needs refresh). The client can call these URLs to get fresh pricing:

1. Trips with invalid prices are grouped by integration + station pair
2. Each group gets a `/searchr` URL with `f=` (from stations) and `t=` (to stations)
3. The client triggers these URLs in the background to fill in missing availability

#### PHP — `frontend3/src/TripSearch/Service/Rechecker.php` (lines 115-161)

PHP's recheck builder iterates through `originalCollection->items` — these come from ALL pre-filter trips (direct, non-connection) with invalid prices. The key is `originalCollection`: PHP populates it during `prepareRawTrips()` which runs before trip-level filtering.

#### Go — `internal/stage/filter_raw_trips.go` (lines 67-89)

Go's `PreFilterRecheckEntries` also collects from all pre-filter trips (excluding connections and hidden). So Go should capture the same invalid-price trips.

#### Why the Extra URL?

The extra URL (f=5936&t=5957) suggests PHP's trip pool query returns at least one raw trip with:
- `dep_station_id=5936`, `arr_station_id=5957`
- Invalid price
- Not a connection (`set_id` is 0 or null)

Go either:
1. Doesn't receive this trip from its query (different SQL or parameters), OR
2. Receives it but filters it differently (meta, daytrip, hidden)

**Note:** Station 5936 also appears as an orphan station in Surat→CM, suggesting it's a transit hub (Bangkok area) that shows up in various routes. This specific station pair (5936→5957) doesn't appear in any visible trip for Bangkok→Phuket, confirming it's from a pre-filtered trip.

### Is the "Known Gap" Classification Correct?

**Partially.** This is a genuine functionality gap — PHP generates a recheck URL that Go doesn't. When the client calls this URL, it could discover additional available trips (Bangkok Khaosan → Phuket) that Go's response doesn't offer to recheck.

**However, the impact is low:**
- It's one extra recheck URL per date for one route
- The station pair (5936→5957) is likely a marginal route variant
- The main station pairs for Bangkok→Phuket are covered by Go's recheck URLs
- If this specific pair matters, it can be investigated further in the trip pool query

**Recommendation:** Investigate whether Go's trip pool query returns the `5936→5957` trip rows. If it does, check why they're being filtered before reaching the recheck pool. If it doesn't, it may be a query-level difference that affects coverage.

---

## 5. Pack Recheck URLs (12)

### What

PHP generates `/searchpm` (pack manual) recheck URLs. Go generates 0.
This gap appears only for Surat Thani → Chiang Mai (4 pack URLs x 3 dates = 12 total).

### What Are Manual Packs?

Manual packs are **multi-leg trip combinations** assembled by the system:
- A "head trip" (e.g., Surat Thani → Bangkok by train) combined with connection legs (e.g., Bangkok → Chiang Mai by bus)
- Distinguished from "autopacks" which have a predefined `autopack_id` in the database
- Manual packs: `isPack() == true` AND `getAutopackId() == null`

### How PHP Generates Pack Recheck URLs

#### `frontend3/src/TripSearch/Service/RecheckBuilder.php` (lines 29-41)

```php
if (!$trip->isPack()) {
    // Regular trip → goes to items collection → /searchr URL
    $collection->items[$trip->getGroupKey()][$key][] = $buyItem;
} elseif (!$trip->getAutopackId()) {
    // Manual pack → goes to manualPacks → /searchpm URL
    $collection->manualPacks[$trip->getGroupKey()][$key][] = $buyItem;
} else {
    // Autopack → goes to autoPacks → (separate handling)
}
```

#### `frontend3/src/TripSearch/Service/Rechecker.php` (lines 162-191)

```php
foreach ($recheck->manualPacks as $recheckGroup) {
    foreach ($recheckGroup as $headTripKey => $recheckTripsData) {
        $recheckRequestData[] = implode(' ', [
            $headTripKey,
            $recheckTripData->tripKey,
            substr($recheckTripData->godate, 0, 10),
        ]);
    }
    $urls[] = $recheckPackUrl . '?t=' . implode(',', $recheckRequestData) . '&l=...';
}
```

URL format: `/searchpm?t=HEAD_KEY LEG_KEY DATE,HEAD_KEY LEG_KEY DATE&l=en&d=...`

### Why Go Generates 0

Go has the **code infrastructure** for pack recheck URLs:

- `internal/stage/sort_and_finalize.go` (lines 137-145): `PackRecheckGroup` and `PackRecheckEntry` types defined
- `internal/stage/sort_and_finalize.go` (lines 231-254): Logic to populate `packRecheckGroupMap` when a pack has invalid price
- `internal/stage/serialize_response.go` (lines 119-170): `buildPackManualRecheckURLs()` function exists

**But** Go's pipeline never creates trips with `IsPack=true` that have `PackLegs` populated, because:

1. **AssembleMultiLeg stage** (`internal/stage/assemble_multi_leg.go`) handles connection assembly but doesn't create manual pack trips yet
2. Without assembled pack trips, `packRecheckGroupMap` stays empty
3. No `/searchpm` URLs are generated

### Is the "Known Gap" Classification Correct?

**Yes, but this is a genuine missing feature, not just cosmetic overhead.**

Unlike orphan stations/operators/classes (which are dead dictionary entries), pack recheck URLs represent **real functionality**:
- When the client calls `/searchpm`, PHP can check availability of multi-leg combinations
- Without these URLs, Go users won't see availability for manual pack routes on Surat Thani → Chiang Mai
- This is a **multi-segment journey** (e.g., Surat Thani → Bangkok → Chiang Mai via train+bus) where individual legs may exist but the combined pack pricing needs verification

**Impact:** Medium. The Surat Thani → Chiang Mai route returns 0 trips in both PHP and Go (all prices invalid), so the entire availability for this route depends on recheck. Missing pack recheck URLs means some multi-leg options will never be rechecked in Go.

---

## 6. Cross-Gap Correlations

### Station 5943 — The Universal Orphan

Station 5943 appears as an orphan in **12 out of 15 test cases** across all 4 routes that have gaps. This is likely a major Bangkok transit station (Don Mueang or Mo Chit) that appears in connection legs for many routes but is never a direct dep/arr station for visible trips.

### Station 5936 — Recheck + Orphan Overlap

Station 5936 appears in TWO gap types:
- **orphan_station** in Surat Thani → Chiang Mai (all 3 dates)
- **orphan_recheck_url** f= parameter in Bangkok → Phuket (all 3 dates)

This is the same station used differently: as a connection leg station (Surat→CM) and as a recheck departure station (BKK→Phuket). This confirms it's a Bangkok-area hub station.

### Surat Thani → Chiang Mai — The Gap Hotspot

This route has the most gaps (9 stations + 8 operators + 7 classes + 4 pack URLs per date) because:
- It's a long-distance multi-segment route requiring connections through Bangkok
- ALL trips have invalid prices (0 trips shown in both PHP and Go)
- PHP's connection assembly processes many transit legs, collecting their ref data
- Manual packs are the primary way to serve this route

---

## 7. Recommendations

### A. Orphan Stations/Operators/Classes — NO ACTION NEEDED

**Classification: Correct as "known gap" — safe to keep as-is.**

These are genuinely unreferenced dictionary entries in PHP's response. The client doesn't use them because no visible trip references them. Go's behavior is actually **more correct** — it only includes ref data for entities that appear in the response. PHP's greedy collection is legacy behavior from its collector pattern.

Rationale:
- The search response returns what is currently available
- Ref data dictionaries exist to render those available trips
- Orphan ref data serves no rendering purpose
- When recheck URLs are triggered, the recheck response provides its own ref data

**If strict byte-for-byte parity is needed:** Collect operators/classes from connection legs too in `collect_ref_data.go`. But this adds DB queries and response size for no functional benefit.

### B. Orphan Recheck URLs — INVESTIGATE (Low Priority)

**Classification: Correct as "known gap" but warrants investigation.**

The extra recheck URL (f=5936&t=5957) for Bangkok→Phuket represents a minor coverage gap. One station pair won't be rechecked in Go.

**Suggested action:**
1. Check if Go's trip pool query returns trips with `dep_station_id=5936, arr_station_id=5957` for the Bangkok→Phuket search
2. If yes: find where they're being filtered before reaching `PreFilterRecheckEntries`
3. If no: it may be a query-level difference (PHP may use broader station expansion)
4. Assess whether this station pair generates real bookings — if not, it's noise

### C. Pack Recheck URLs — IMPLEMENT (Medium Priority)

**Classification: Correct as "known gap" — but represents real missing functionality.**

This is the only gap that affects user-visible behavior. Without `/searchpm` URLs, multi-leg pack trips (like Surat Thani → Bangkok → Chiang Mai) cannot be rechecked in Go.

**Implementation path:**
1. `AssembleMultiLeg` stage needs to create manual pack trips (trips with `IsPack=true`, `PackLegs` populated)
2. When these pack trips have invalid prices, they'll flow through the existing `packRecheckGroupMap` logic in `sort_and_finalize.go`
3. `buildPackManualRecheckURLs()` in `serialize_response.go` will then generate the `/searchpm` URLs
4. The Go code infrastructure is already in place — only pack assembly is missing

**Dependencies:** This requires implementing the pack assembly logic in Stage 5a, which is tracked separately as a feature flag (`FEATURE_AUTOPACKS`).

---

## Summary Table

| Gap Type | Count | Source | Client Impact | Correct Classification? | Action |
|----------|-------|--------|---------------|------------------------|--------|
| orphan_station | 43 | Connection leg stations | None — unreferenced | YES | No action |
| orphan_operator | 30 | Connection leg operators | None — unreferenced | YES | No action |
| orphan_class | 24 | Connection leg classes | None — unreferenced | YES | No action |
| orphan_recheck_url | 3 | Extra trip in PHP's pool | Low — 1 marginal pair | YES | Investigate |
| pack_recheck_urls | 12 | Pack assembly not impl. | Medium — multi-leg gaps | YES (feature gap) | Implement |
