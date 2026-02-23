package domain

// PriceLevel indicates the confidence level of a price.
const (
	PriceNone        = 0
	PriceExact       = 1
	PriceAnyOnDate   = 2
	PricePredict     = 3
)

// RecheckLevel controls how aggressively to recheck prices.
const (
	RecheckNever        = 0
	RecheckNotPredicted = 1
	RecheckPredicted    = 2
	RecheckByCacheTTL   = 3
	RecheckNotExact     = 4
	RecheckAlways       = 5
	RecheckBySettings   = 6
)

// PriceMode bit flags.
const (
	PriceModeExactFXCode = 1 << 6 // 0b01000000
	PriceModeWhiteLabel  = 1 << 5 // 0b00100000
	PriceModeNoDiscount  = 1 << 4 // 0b00010000
)

// TripPrice represents a decoded price for a trip.
type TripPrice struct {
	IsValid      bool
	IsValidByTTL bool
	IsValidByML  bool
	IsExperiment bool
	IsOutdated   bool
	Avail        int
	PriceLevel   int
	ReasonID     int
	ReasonParam  int
	Stamp        int
	FXCode       string
	Total        float64
	Fares        map[string]*PriceFare // "adult", "child", "infant"
	MLScore      float64
	Duration     int // seconds
	AdvHour      int
	LegacyRouteID int
	LegacyTripID  int
	// Extended (API v2+)
	DiscountDelta map[string]*PriceDeltaFare
	RuleDelta     map[string]*PriceDeltaFare
}

// PriceFare represents pricing for one passenger type.
type PriceFare struct {
	FullPriceFXCode string
	FullPrice       float64
	NetPriceFXCode  string
	NetPrice        float64
	TopupFXCode     string
	Topup           float64
	SysFeeFXCode    string
	SysFee          float64
	AgFeeFXCode     string
	AgFee           float64
}

// PriceDeltaFare represents a price delta (discount or rule adjustment).
type PriceDeltaFare struct {
	ID             int
	TotalDelta     float64
	NetPriceDelta  float64
	TopupDelta     float64
	SysFeeDelta    float64
	AgFeeDelta     float64
}
