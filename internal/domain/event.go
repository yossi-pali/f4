package domain

import "time"

// Event topics.
const (
	TopicSearchCompleted      = "search.completed"
	TopicSearchNeedsRecheck   = "search.needs_recheck"
	TopicSearchNeedsRoundTrip = "search.needs_round_trip_prices"
)

// SearchCompletedEvent is fired after every search response.
type SearchCompletedEvent struct {
	FromPlaceID string    `json:"from_place_id"`
	ToPlaceID   string    `json:"to_place_id"`
	Date        string    `json:"date"`
	ResultCount int       `json:"result_count"`
	LatencyMs   int64     `json:"latency_ms"`
	Region      string    `json:"region"`
	AgentID     int       `json:"agent_id"`
	Timestamp   time.Time `json:"timestamp"`
}

// SearchNeedsRecheckEvent is fired when results have invalid/predicted prices.
type SearchNeedsRecheckEvent struct {
	TripKeys         []string `json:"trip_keys"`
	IntegrationCodes []string `json:"integration_codes"`
	FromStationIDs   []int    `json:"from_station_ids"`
	ToStationIDs     []int    `json:"to_station_ids"`
	Date             string   `json:"date"`
	SeatsAdult       int      `json:"seats_adult"`
	SeatsChild       int      `json:"seats_child"`
	SeatsInfant      int      `json:"seats_infant"`
	FXCode           string   `json:"fxcode"`
	Lang             string   `json:"lang"`
	AgentID          int      `json:"agent_id"`
}

// SearchNeedsRoundTripPricesEvent is fired when round trip cache misses.
type SearchNeedsRoundTripPricesEvent struct {
	OutboundTripKey string `json:"outbound_trip_key"`
	OutboundGodate  string `json:"outbound_godate"`
	InboundDate     string `json:"inbound_date"`
	IntegrationCode string `json:"integration_code"`
	SeatsAdult      int    `json:"seats_adult"`
	SeatsChild      int    `json:"seats_child"`
	SeatsInfant     int    `json:"seats_infant"`
}
