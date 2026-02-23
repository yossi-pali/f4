package domain

// AutopackConfig represents an autopack configuration from landing_alternatives.
type AutopackConfig struct {
	AutopackID int
	FromPlaceID string
	ToPlaceID   string
	IsActive    bool
	Routes      []AutopackRoute
}

// AutopackRoute represents one route option within an autopack.
type AutopackRoute struct {
	RouteIndex       int
	Leg1             AutopackLeg
	Leg2             AutopackLeg
	TransferWaitTime int // minutes
	TotalDuration    int // minutes
}

// AutopackLeg represents one leg of an autopack route.
type AutopackLeg struct {
	VehclassID    string
	ClassID       int
	FromStationID int
	ToStationID   int
	FromCountryID string
	ToCountryID   string
	DepartureTime int // minutes from midnight
	ArrivalTime   int // minutes from midnight
	Duration      int // minutes
}
