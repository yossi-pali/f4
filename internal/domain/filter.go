package domain

import "time"

// SearchFilter holds all parameters for a trip search query.
type SearchFilter struct {
	FromPlaceID    string
	ToPlaceID      string
	FromStationIDs []int
	ToStationIDs   []int
	Date           time.Time

	// Passengers
	SeatsAdult  int
	SeatsChild  int
	SeatsInfant int

	// Pricing
	FXCode       string
	PriceMode    int
	RecheckLevel int

	// Localization
	Lang    string
	Locale  string
	AgentID int

	// Include filters
	OperatorIDs  []int
	SellerIDs    []int
	VehclassIDs  []string
	ClassIDs     []int
	CountryIDs   []string

	// Exclude filters
	ExcludeOperatorIDs  []int
	ExcludeSellerIDs    []int
	ExcludeVehclassIDs  []string
	ExcludeClassIDs     []int
	ExcludeCountryIDs   []string

	// Search behavior
	IntegrationCode string
	OnlyDirect      bool
	OnlyPairs       bool
	WithAutopacks   bool
	WithNonBookable bool
	IsNotPossible   bool
	RecheckAmount   int
	TripKeys        []string

	// Round trip
	OutboundTrip                *TripPlain
	OnlyExactRoundTripDiscounts bool

	// Admin
	WithAdminLinks bool
	IsBot          bool

	// Page URL for station weight overrides (e.g., "travel/bangkok/chiang-mai")
	PageURL string

	// Price visibility (computed from agent permissions, matching PHP TravelOptionBaseFactory)
	NeedPassTopup              bool // agent logged in OR reseller → include agfee + price_restriction
	NeedPassNetpriceAndSysfee  bool // agent has api_pass_netprice_sysfee permission → include netprice + sysfee
}

// SearchParams holds the raw query parameters parsed from an HTTP request.
type SearchParams struct {
	Lang            string
	SeatsAdult      int
	SeatsChild      int
	SeatsInfant     int
	FXCode          string
	Direct          bool
	RecheckAmount   int
	IsRecheck       bool // "r" param
	CartHash        string
	OutboundTripRef string
	OnlyPairs       bool
	VehclassID      string
	IntegrationCode string
	WithNonBookable bool
	ExtendedFormat  bool
	Referer         string
	FromStations    []int // for searchByStations endpoint
	ToStations      []int
}

// AgentContext holds information about the API caller.
type AgentContext struct {
	AgentID    int
	APIKey     string
	Role       string
	Referer    string
	IsBot      bool
	IsLoggedIn bool
	IsAdmin    bool
	UserID     int
}
