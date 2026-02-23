package comparator

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// DiffResult is the output of comparing two JSON responses for one test case.
type DiffResult struct {
	Scenario       string          `json:"scenario"`
	Date           string          `json:"date"`
	LegacyStatus   int             `json:"legacy_status"`
	NewStatus      int             `json:"new_status"`
	Summary        DiffSummary     `json:"summary"`
	OnlyInLegacy   []string        `json:"only_in_legacy,omitempty"`
	OnlyInNew      []string        `json:"only_in_new,omitempty"`
	Differences    []FieldDiff     `json:"differences,omitempty"`
	Errors         []string        `json:"errors,omitempty"`
}

// DiffSummary contains aggregate stats.
type DiffSummary struct {
	TripsLegacy      int `json:"trips_legacy"`
	TripsNew         int `json:"trips_new"`
	TripsMatched     int `json:"trips_matched"`
	TripsOnlyLegacy  int `json:"trips_only_in_legacy"`
	TripsOnlyNew     int `json:"trips_only_in_new"`
	FieldsCompared   int `json:"fields_compared"`
	FieldsDifferent  int `json:"fields_different"`
}

// FieldDiff represents a single field difference.
type FieldDiff struct {
	TripKey string `json:"trip_key,omitempty"`
	Path    string `json:"path"`
	Legacy  any    `json:"legacy"`
	New     any    `json:"new"`
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

	// Compare recheck URLs as sorted sets
	d.compareStringArrays(result, "recheck", legacyData["recheck"], newData["recheck"])

	result.Summary.FieldsDifferent = len(result.Differences)
	return result
}

func (d *Differ) compareTrips(result *DiffResult, legacy, newData map[string]any) {
	legacyTrips := toSlice(legacy["trips"])
	newTrips := toSlice(newData["trips"])

	result.Summary.TripsLegacy = len(legacyTrips)
	result.Summary.TripsNew = len(newTrips)

	// Index by trip_key
	legacyByKey := indexByField(legacyTrips, "trip_key")
	newByKey := indexByField(newTrips, "trip_key")

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
	// Compare scalar trip fields
	for _, field := range []string{"group_key", "is_bookable", "has_valid_price", "is_connection", "rank_score", "special_deal", "new_trip"} {
		d.compareField(result, tripKey, field, legacy[field], newTrip[field])
	}

	// Compare tags
	d.compareStringArrays(result, fmt.Sprintf("trips[%s].tags", tripKey), legacy["tags"], newTrip["tags"])

	// Compare segments by index
	legacySegs := toSlice(legacy["segments"])
	newSegs := toSlice(newTrip["segments"])
	maxSegs := len(legacySegs)
	if len(newSegs) > maxSegs {
		maxSegs = len(newSegs)
	}
	if len(legacySegs) != len(newSegs) {
		result.Differences = append(result.Differences, FieldDiff{
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
		for _, field := range []string{"from_station_id", "to_station_id", "departure", "arrival", "duration", "operator_id", "class_id", "vehclass_id", "type"} {
			d.compareField(result, tripKey, prefix+"."+field, legSeg[field], newSeg[field])
		}
	}

	// Compare travel_options matched by trip_key + integration_code
	d.compareTravelOptions(result, tripKey, legacy["travel_options"], newTrip["travel_options"])
}

func (d *Differ) compareTravelOptions(result *DiffResult, tripKey string, legacyRaw, newRaw any) {
	legacyOpts := toSlice(legacyRaw)
	newOpts := toSlice(newRaw)

	legacyByKey := indexByCompoundKey(legacyOpts, "trip_key", "integration_code")
	newByKey := indexByCompoundKey(newOpts, "trip_key", "integration_code")

	if len(legacyOpts) != len(newOpts) {
		result.Differences = append(result.Differences, FieldDiff{
			TripKey: tripKey,
			Path:    "travel_options.length",
			Legacy:  len(legacyOpts),
			New:     len(newOpts),
		})
	}

	for key, legOpt := range legacyByKey {
		newOpt, ok := newByKey[key]
		if !ok {
			result.Differences = append(result.Differences, FieldDiff{
				TripKey: tripKey,
				Path:    fmt.Sprintf("travel_options[%s]", key),
				Legacy:  "present",
				New:     "missing",
			})
			continue
		}

		prefix := fmt.Sprintf("travel_options[%s]", key)
		d.compareField(result, tripKey, prefix+".available_seats", legOpt["available_seats"], newOpt["available_seats"])
		d.compareField(result, tripKey, prefix+".departure_time", legOpt["departure_time"], newOpt["departure_time"])

		// Compare price
		legPrice := toMap(legOpt["price"])
		newPrice := toMap(newOpt["price"])
		pricePrefix := prefix + ".price"
		for _, field := range []string{"total", "fxcode", "price_level", "is_valid"} {
			d.compareField(result, tripKey, pricePrefix+"."+field, legPrice[field], newPrice[field])
		}

		// Compare fares
		d.compareFares(result, tripKey, pricePrefix, legPrice["fares"], newPrice["fares"])
	}

	// Check for travel options only in new
	for key := range newByKey {
		if _, ok := legacyByKey[key]; !ok {
			result.Differences = append(result.Differences, FieldDiff{
				TripKey: tripKey,
				Path:    fmt.Sprintf("travel_options[%s]", key),
				Legacy:  "missing",
				New:     "present",
			})
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
			result.Differences = append(result.Differences, FieldDiff{
				TripKey: tripKey, Path: farePrefix, Legacy: nil, New: "present",
			})
			continue
		}
		if legFare != nil && newFare == nil {
			result.Differences = append(result.Differences, FieldDiff{
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
			result.Differences = append(result.Differences, FieldDiff{
				Path: prefix, Legacy: nil, New: "present",
			})
			continue
		}
		if legEntry != nil && newEntry == nil {
			result.Differences = append(result.Differences, FieldDiff{
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

	result.Differences = append(result.Differences, FieldDiff{
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
		result.Differences = append(result.Differences, FieldDiff{
			Path: path + ".length", Legacy: len(legacyArr), New: len(newArr),
		})
		result.Summary.FieldsCompared++
		return
	}

	for i := range legacyArr {
		result.Summary.FieldsCompared++
		if legacyArr[i] != newArr[i] {
			result.Differences = append(result.Differences, FieldDiff{
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
