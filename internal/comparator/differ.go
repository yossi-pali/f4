package comparator

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// KnownGap represents a known architectural difference tracked but not counted as a diff.
type KnownGap struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
	Count  int    `json:"count"`
}

// DiffResult is the output of comparing two JSON responses for one test case.
type DiffResult struct {
	Scenario       string                   `json:"scenario"`
	Date           string                   `json:"date"`
	LegacyStatus   int                      `json:"legacy_status"`
	NewStatus      int                      `json:"new_status"`
	Summary        DiffSummary              `json:"summary"`
	OnlyInLegacy   []string                 `json:"only_in_legacy,omitempty"`
	OnlyInNew      []string                 `json:"only_in_new,omitempty"`
	Differences    map[string]CategoryDiff  `json:"differences,omitempty"`
	KnownGaps      []KnownGap               `json:"known_gaps,omitempty"`
	Errors         []string                 `json:"errors,omitempty"`
	rawDiffs       []FieldDiff              // internal accumulator during comparison
}

// DiffSummary contains aggregate stats.
type DiffSummary struct {
	TripsLegacy      int            `json:"trips_legacy"`
	TripsNew         int            `json:"trips_new"`
	TripsMatched     int            `json:"trips_matched"`
	TripsOnlyLegacy  int            `json:"trips_only_in_legacy"`
	TripsOnlyNew     int            `json:"trips_only_in_new"`
	FieldsCompared   int            `json:"fields_compared"`
	FieldsDifferent  int            `json:"fields_different"`
	CategoryCounts   map[string]int `json:"category_counts,omitempty"`
}

// FieldDiff represents a single field difference.
type FieldDiff struct {
	TripKey string `json:"trip_key,omitempty"`
	Path    string `json:"path"`
	Legacy  any    `json:"legacy"`
	New     any    `json:"new"`
}

// CategoryDiff groups field differences under a top-level response category.
type CategoryDiff struct {
	Count       int         `json:"count"`
	Differences []FieldDiff `json:"differences"`
}

// Differ compares two JSON search responses.
type Differ struct {
	floatTolerance float64
}

// NewDiffer creates a new Differ with the given float tolerance.
func NewDiffer(floatTolerance float64) *Differ {
	return &Differ{floatTolerance: floatTolerance}
}

// Compare diffs two raw JSON responses for a test case.
func (d *Differ) Compare(tc TestCase, legacyBody, newBody []byte, legacyStatus, newStatus int) *DiffResult {
	result := &DiffResult{
		Scenario:     tc.Scenario.Name,
		Date:         tc.Date,
		LegacyStatus: legacyStatus,
		NewStatus:    newStatus,
	}

	var legacyData, newData map[string]any
	if err := json.Unmarshal(legacyBody, &legacyData); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("parse legacy JSON: %v", err))
		return result
	}
	if err := json.Unmarshal(newBody, &newData); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("parse new JSON: %v", err))
		return result
	}

	// Compare trips (matched by trip_key)
	d.compareTrips(result, legacyData, newData)

	// Compare reference dictionaries
	d.compareDicts(result, legacyData, newData, "stations")
	d.compareDicts(result, legacyData, newData, "operators")
	d.compareDicts(result, legacyData, newData, "classes")

	// Compare simple top-level fields
	d.compareField(result, "", "provinceName", legacyData["provinceName"], newData["provinceName"])

	// Compare recheck URLs — filter out scan URLs (/searchs) as known gaps
	legacyRecheck := toStringSlice(legacyData["recheck"])
	newRecheck := toStringSlice(newData["recheck"])
	legacySearchr, legacyScan := splitScanURLs(legacyRecheck)
	newSearchr, newScan := splitScanURLs(newRecheck)
	d.compareSortedStringSlices(result, "recheck", legacySearchr, newSearchr)
	if len(legacyScan) > 0 || len(newScan) > 0 {
		result.KnownGaps = append(result.KnownGaps, KnownGap{
			Type:   "scan_urls",
			Detail: fmt.Sprintf("legacy=%d new=%d", len(legacyScan), len(newScan)),
			Count:  len(legacyScan),
		})
	}

	// Move orphan station diffs (legacy-only) to known gaps
	result.extractOrphanStationGaps()

	result.groupDifferences()
	return result
}

func (d *Differ) compareTrips(result *DiffResult, legacy, newData map[string]any) {
	legacyTrips := toSlice(legacy["trips"])
	newTrips := toSlice(newData["trips"])

	result.Summary.TripsLegacy = len(legacyTrips)
	result.Summary.TripsNew = len(newTrips)

	// Index by "id" field (MD5 hash in V1 format)
	legacyByKey := indexByField(legacyTrips, "id")
	newByKey := indexByField(newTrips, "id")

	// Find trips only in legacy
	for key := range legacyByKey {
		if _, ok := newByKey[key]; !ok {
			result.OnlyInLegacy = append(result.OnlyInLegacy, key)
		}
	}
	sort.Strings(result.OnlyInLegacy)
	result.Summary.TripsOnlyLegacy = len(result.OnlyInLegacy)

	// Find trips only in new
	for key := range newByKey {
		if _, ok := legacyByKey[key]; !ok {
			result.OnlyInNew = append(result.OnlyInNew, key)
		}
	}
	sort.Strings(result.OnlyInNew)
	result.Summary.TripsOnlyNew = len(result.OnlyInNew)

	// Compare matched trips
	matched := 0
	for key, legacyTrip := range legacyByKey {
		newTrip, ok := newByKey[key]
		if !ok {
			continue
		}
		matched++
		d.compareTripFields(result, key, legacyTrip, newTrip)
	}
	result.Summary.TripsMatched = matched
}

func (d *Differ) compareTripFields(result *DiffResult, tripKey string, legacy, newTrip map[string]any) {
	// Compare scalar trip-level fields (V1 format)
	for _, field := range []string{
		"chunk_key", "route_name", "show_map", "transfer_id",
		"score_sorting", "sales_sorting", "bookings_last_month",
		"is_solo_traveler", "is_boosted", "connected_with",
	} {
		d.compareField(result, tripKey, field, legacy[field], newTrip[field])
	}

	// Compare params sub-object
	legParams := toMap(legacy["params"])
	newParams := toMap(newTrip["params"])
	if legParams != nil && newParams != nil {
		for _, field := range []string{
			"from", "to", "dep_time", "arr_time", "duration", "stops",
			"bookable", "status", "is_bookable", "disabled", "hide", "date",
		} {
			d.compareField(result, tripKey, "params."+field, legParams[field], newParams[field])
		}
		// Compare params.min_price
		legPrice := toMap(legParams["min_price"])
		newPrice := toMap(newParams["min_price"])
		if legPrice != nil || newPrice != nil {
			if legPrice == nil || newPrice == nil {
				d.compareField(result, tripKey, "params.min_price", legParams["min_price"], newParams["min_price"])
			} else {
				d.compareField(result, tripKey, "params.min_price.value", legPrice["value"], newPrice["value"])
				d.compareField(result, tripKey, "params.min_price.fxcode", legPrice["fxcode"], newPrice["fxcode"])
			}
		}
		d.compareField(result, tripKey, "params.min_rating", legParams["min_rating"], newParams["min_rating"])
		d.compareField(result, tripKey, "params.rating_count", legParams["rating_count"], newParams["rating_count"])
		d.compareField(result, tripKey, "params.reason", legParams["reason"], newParams["reason"])
		// Compare params.vehclasses array
		d.compareAnyArrays(result, tripKey, "params.vehclasses", legParams["vehclasses"], newParams["vehclasses"])
		// Compare params.operators array
		d.compareAnyArrays(result, tripKey, "params.operators", legParams["operators"], newParams["operators"])
	}

	// Compare tags
	d.compareAnyArrays(result, tripKey, "tags", legacy["tags"], newTrip["tags"])

	// Compare segments by index (V1 format)
	legacySegs := toSlice(legacy["segments"])
	newSegs := toSlice(newTrip["segments"])
	maxSegs := len(legacySegs)
	if len(newSegs) > maxSegs {
		maxSegs = len(newSegs)
	}
	if len(legacySegs) != len(newSegs) {
		result.rawDiffs = append(result.rawDiffs, FieldDiff{
			TripKey: tripKey,
			Path:    "segments.length",
			Legacy:  len(legacySegs),
			New:     len(newSegs),
		})
	}
	for i := 0; i < maxSegs; i++ {
		if i >= len(legacySegs) || i >= len(newSegs) {
			break
		}
		legSeg := toMap(legacySegs[i])
		newSeg := toMap(newSegs[i])
		prefix := fmt.Sprintf("segments[%d]", i)
		for _, field := range []string{
			"type", "trip_id", "official_id", "from", "to",
			"dep_time", "arr_time", "duration", "class", "operator",
			"rating", "show_map", "search_results_marker", "price",
		} {
			d.compareField(result, tripKey, prefix+"."+field, legSeg[field], newSeg[field])
		}
	}

	// Compare travel_options matched by "id" field (V1 format)
	d.compareTravelOptions(result, tripKey, legacy["travel_options"], newTrip["travel_options"])
}

func (d *Differ) compareTravelOptions(result *DiffResult, tripKey string, legacyRaw, newRaw any) {
	legacyOpts := toSlice(legacyRaw)
	newOpts := toSlice(newRaw)

	// V1 format: match travel options by "id" field
	legacyByKey := indexByField(legacyOpts, "id")
	newByKey := indexByField(newOpts, "id")

	if len(legacyOpts) != len(newOpts) {
		result.rawDiffs = append(result.rawDiffs, FieldDiff{
			TripKey: tripKey,
			Path:    "travel_options.length",
			Legacy:  len(legacyOpts),
			New:     len(newOpts),
		})
	}

	for key, legOpt := range legacyByKey {
		newOpt, ok := newByKey[key]
		if !ok {
			result.rawDiffs = append(result.rawDiffs, FieldDiff{
				TripKey: tripKey,
				Path:    fmt.Sprintf("travel_options[%s]", key),
				Legacy:  "present",
				New:     "missing",
			})
			continue
		}

		prefix := fmt.Sprintf("travel_options[%s]", key)

		// Compare V1 travel option fields
		for _, field := range []string{
			"bookable", "class", "ticket_type",
			"confirmation_time", "confirmation_minutes", "confirmation_message",
			"cancellation", "full_refund_until", "cancellation_message",
			"rating", "rating_count", "is_bookable", "reason",
			"booking_uri", "bookings_last_month", "sales_sorting",
		} {
			d.compareField(result, tripKey, prefix+"."+field, legOpt[field], newOpt[field])
		}

		// Compare price
		legPrice := toMap(legOpt["price"])
		newPrice := toMap(newOpt["price"])
		if legPrice != nil || newPrice != nil {
			if legPrice == nil || newPrice == nil {
				d.compareField(result, tripKey, prefix+".price", legOpt["price"], newOpt["price"])
			} else {
				d.compareField(result, tripKey, prefix+".price.value", legPrice["value"], newPrice["value"])
				d.compareField(result, tripKey, prefix+".price.fxcode", legPrice["fxcode"], newPrice["fxcode"])
			}
		}

		// Compare array fields
		d.compareAnyArrays(result, tripKey, prefix+".amenities", legOpt["amenities"], newOpt["amenities"])
		d.compareAnyArrays(result, tripKey, prefix+".labels", legOpt["labels"], newOpt["labels"])

		// Compare buy items
		d.compareBuyItems(result, tripKey, prefix, legOpt["buy"], newOpt["buy"])
	}

	// Check for travel options only in new
	for key := range newByKey {
		if _, ok := legacyByKey[key]; !ok {
			result.rawDiffs = append(result.rawDiffs, FieldDiff{
				TripKey: tripKey,
				Path:    fmt.Sprintf("travel_options[%s]", key),
				Legacy:  "missing",
				New:     "present",
			})
		}
	}
}

// compareAnyArrays compares two JSON arrays element-by-element using fmt.Sprintf for comparison.
func (d *Differ) compareAnyArrays(result *DiffResult, tripKey, path string, legacyRaw, newRaw any) {
	legacyArr := toSlice(legacyRaw)
	newArr := toSlice(newRaw)

	if len(legacyArr) != len(newArr) {
		result.rawDiffs = append(result.rawDiffs, FieldDiff{
			TripKey: tripKey,
			Path:    path + ".length",
			Legacy:  len(legacyArr),
			New:     len(newArr),
		})
		result.Summary.FieldsCompared++
		return
	}

	for i := range legacyArr {
		result.Summary.FieldsCompared++
		if !d.valuesEqual(legacyArr[i], newArr[i]) {
			result.rawDiffs = append(result.rawDiffs, FieldDiff{
				TripKey: tripKey,
				Path:    fmt.Sprintf("%s[%d]", path, i),
				Legacy:  legacyArr[i],
				New:     newArr[i],
			})
		}
	}
}

// compareBuyItems compares buy item arrays in travel options.
func (d *Differ) compareBuyItems(result *DiffResult, tripKey, prefix string, legacyRaw, newRaw any) {
	legacyItems := toSlice(legacyRaw)
	newItems := toSlice(newRaw)

	if len(legacyItems) != len(newItems) {
		result.rawDiffs = append(result.rawDiffs, FieldDiff{
			TripKey: tripKey,
			Path:    prefix + ".buy.length",
			Legacy:  len(legacyItems),
			New:     len(newItems),
		})
		return
	}

	for i := range legacyItems {
		legItem := toMap(legacyItems[i])
		newItem := toMap(newItems[i])
		if legItem == nil || newItem == nil {
			continue
		}
		buyPrefix := fmt.Sprintf("%s.buy[%d]", prefix, i)
		for _, field := range []string{"trip_id", "from_id", "to_id", "date", "date2", "date3"} {
			d.compareField(result, tripKey, buyPrefix+"."+field, legItem[field], newItem[field])
		}
	}
}

func (d *Differ) compareFares(result *DiffResult, tripKey, prefix string, legacyRaw, newRaw any) {
	legacyFares := toMap(legacyRaw)
	newFares := toMap(newRaw)

	allKeys := make(map[string]bool)
	for k := range legacyFares {
		allKeys[k] = true
	}
	for k := range newFares {
		allKeys[k] = true
	}

	for fareType := range allKeys {
		legFare := toMap(legacyFares[fareType])
		newFare := toMap(newFares[fareType])
		farePrefix := fmt.Sprintf("%s.fares.%s", prefix, fareType)

		if legFare == nil && newFare != nil {
			result.rawDiffs = append(result.rawDiffs, FieldDiff{
				TripKey: tripKey, Path: farePrefix, Legacy: nil, New: "present",
			})
			continue
		}
		if legFare != nil && newFare == nil {
			result.rawDiffs = append(result.rawDiffs, FieldDiff{
				TripKey: tripKey, Path: farePrefix, Legacy: "present", New: nil,
			})
			continue
		}

		for _, field := range []string{"full_price", "fxcode", "net_price", "topup", "sys_fee"} {
			d.compareField(result, tripKey, farePrefix+"."+field, legFare[field], newFare[field])
		}
	}
}

func (d *Differ) compareDicts(result *DiffResult, legacy, newData map[string]any, dictName string) {
	legacyDict := toMap(legacy[dictName])
	newDict := toMap(newData[dictName])

	allKeys := make(map[string]bool)
	for k := range legacyDict {
		allKeys[k] = true
	}
	for k := range newDict {
		allKeys[k] = true
	}

	for key := range allKeys {
		prefix := fmt.Sprintf("%s.%s", dictName, key)
		legEntry := toMap(legacyDict[key])
		newEntry := toMap(newDict[key])

		if legEntry == nil && newEntry != nil {
			result.rawDiffs = append(result.rawDiffs, FieldDiff{
				Path: prefix, Legacy: nil, New: "present",
			})
			continue
		}
		if legEntry != nil && newEntry == nil {
			result.rawDiffs = append(result.rawDiffs, FieldDiff{
				Path: prefix, Legacy: "present", New: nil,
			})
			continue
		}

		for field, legVal := range legEntry {
			d.compareField(result, "", prefix+"."+field, legVal, newEntry[field])
		}
	}
}

func (d *Differ) compareField(result *DiffResult, tripKey, path string, legacy, newVal any) {
	result.Summary.FieldsCompared++

	if d.valuesEqual(legacy, newVal) {
		return
	}

	result.rawDiffs = append(result.rawDiffs, FieldDiff{
		TripKey: tripKey,
		Path:    path,
		Legacy:  legacy,
		New:     newVal,
	})
}

func (d *Differ) compareStringArrays(result *DiffResult, path string, legacyRaw, newRaw any) {
	legacyArr := toStringSlice(legacyRaw)
	newArr := toStringSlice(newRaw)

	sort.Strings(legacyArr)
	sort.Strings(newArr)

	if len(legacyArr) != len(newArr) {
		result.rawDiffs = append(result.rawDiffs, FieldDiff{
			Path: path + ".length", Legacy: len(legacyArr), New: len(newArr),
		})
		result.Summary.FieldsCompared++
		return
	}

	for i := range legacyArr {
		result.Summary.FieldsCompared++
		if legacyArr[i] != newArr[i] {
			result.rawDiffs = append(result.rawDiffs, FieldDiff{
				Path: fmt.Sprintf("%s[%d]", path, i), Legacy: legacyArr[i], New: newArr[i],
			})
		}
	}
}

func (d *Differ) valuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Float tolerance
	aFloat, aIsFloat := toFloat(a)
	bFloat, bIsFloat := toFloat(b)
	if aIsFloat && bIsFloat {
		return math.Abs(aFloat-bFloat) <= d.floatTolerance
	}

	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// standardCategories are the 5 top-level response sections always reported.
var standardCategories = []string{"classes", "operators", "recheck", "stations", "trips"}

// groupDifferences converts the flat rawDiffs into a categorized map.
// All 5 standard categories are always present, even with count 0.
func (r *DiffResult) groupDifferences() {
	cats := make(map[string][]FieldDiff)
	for _, d := range r.rawDiffs {
		cat := categorize(d)
		cats[cat] = append(cats[cat], d)
	}
	r.Differences = make(map[string]CategoryDiff)
	r.Summary.CategoryCounts = make(map[string]int)
	// Always include all standard categories
	for _, cat := range standardCategories {
		r.Differences[cat] = CategoryDiff{Count: 0, Differences: []FieldDiff{}}
		r.Summary.CategoryCounts[cat] = 0
	}
	// Fill in actual counts
	for cat, diffs := range cats {
		r.Differences[cat] = CategoryDiff{Count: len(diffs), Differences: diffs}
		r.Summary.CategoryCounts[cat] = len(diffs)
	}
	r.Summary.FieldsDifferent = len(r.rawDiffs)
}

// categorize determines which top-level response category a diff belongs to.
func categorize(d FieldDiff) string {
	if d.TripKey != "" {
		return "trips"
	}
	path := d.Path
	if strings.HasPrefix(path, "trips") {
		return "trips"
	}
	if strings.HasPrefix(path, "stations") {
		return "stations"
	}
	if strings.HasPrefix(path, "operators") {
		return "operators"
	}
	if strings.HasPrefix(path, "classes") {
		return "classes"
	}
	if strings.HasPrefix(path, "recheck") {
		return "recheck"
	}
	return "other"
}

// splitScanURLs separates place-based scan URLs (/searchs) from station-based recheck URLs (/searchr).
func splitScanURLs(urls []string) (searchr, scan []string) {
	for _, u := range urls {
		if strings.Contains(u, "/searchs") {
			scan = append(scan, u)
		} else {
			searchr = append(searchr, u)
		}
	}
	return
}

// compareSortedStringSlices compares two pre-split string slices after sorting.
func (d *Differ) compareSortedStringSlices(result *DiffResult, path string, legacy, newSlice []string) {
	sort.Strings(legacy)
	sort.Strings(newSlice)

	if len(legacy) != len(newSlice) {
		result.rawDiffs = append(result.rawDiffs, FieldDiff{
			Path: path + ".length", Legacy: len(legacy), New: len(newSlice),
		})
		result.Summary.FieldsCompared++
		return
	}

	for i := range legacy {
		result.Summary.FieldsCompared++
		if legacy[i] != newSlice[i] {
			result.rawDiffs = append(result.rawDiffs, FieldDiff{
				Path: fmt.Sprintf("%s[%d]", path, i), Legacy: legacy[i], New: newSlice[i],
			})
		}
	}
}

// extractOrphanStationGaps moves station diffs where legacy has the station but new doesn't
// to KnownGaps (these are transit stations from PHP connection legs that Go doesn't collect).
func (r *DiffResult) extractOrphanStationGaps() {
	var filtered []FieldDiff
	for _, d := range r.rawDiffs {
		if strings.HasPrefix(d.Path, "stations.") && d.Legacy == "present" && d.New == nil {
			r.KnownGaps = append(r.KnownGaps, KnownGap{
				Type:   "orphan_station",
				Detail: d.Path,
				Count:  1,
			})
		} else {
			filtered = append(filtered, d)
		}
	}
	r.rawDiffs = filtered
}

// --- helpers ---

func toSlice(v any) []any {
	if v == nil {
		return nil
	}
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func toMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func toStringSlice(v any) []string {
	arr := toSlice(v)
	if arr == nil {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func indexByField(items []any, field string) map[string]map[string]any {
	index := make(map[string]map[string]any, len(items))
	for _, item := range items {
		m := toMap(item)
		if m == nil {
			continue
		}
		if key, ok := m[field].(string); ok {
			index[key] = m
		}
	}
	return index
}

func indexByCompoundKey(items []any, fields ...string) map[string]map[string]any {
	index := make(map[string]map[string]any, len(items))
	for _, item := range items {
		m := toMap(item)
		if m == nil {
			continue
		}
		parts := make([]string, len(fields))
		for i, f := range fields {
			parts[i] = fmt.Sprintf("%v", m[f])
		}
		key := ""
		for i, p := range parts {
			if i > 0 {
				key += "|"
			}
			key += p
		}
		index[key] = m
	}
	return index
}
