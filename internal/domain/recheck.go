package domain

// RecheckCollection groups trips that need price rechecking.
type RecheckCollection struct {
	Items       map[string]map[string][]BuyItem // groupKey → tripKey → items
	ManualPacks map[string]map[string][]BuyItem // groupKey → headTripKey → items
	AutoPacks   map[string]map[string][]BuyItem // groupKey → key → items
}

// NewRecheckCollection creates an empty RecheckCollection.
func NewRecheckCollection() *RecheckCollection {
	return &RecheckCollection{
		Items:       make(map[string]map[string][]BuyItem),
		ManualPacks: make(map[string]map[string][]BuyItem),
		AutoPacks:   make(map[string]map[string][]BuyItem),
	}
}

// BuyItem represents a single bookable item for rechecking.
type BuyItem struct {
	TripKey          string
	FromID           int
	ToID             int
	Godate           string
	Godate2          string
	Godate3          string
	OperatorID       int
	MasterOperatorID int
	ClassID          int
	OfficialID       string
	LegacyRouteID    int
}
