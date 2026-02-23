package domain

// Operator represents a transport operator.
type Operator struct {
	OperatorID    int     `json:"operator_id"`
	Name          string  `json:"name"`
	Slug          string  `json:"slug"`          // computed from name via Slugger
	SellerID      int     `json:"seller_id"`
	MasterID      int     `json:"master_id"`
	Bookable      bool    `json:"-"`
	RatingAvg     float64 `json:"rating_avg,omitempty"`
	RatingCount   int     `json:"rating_count,omitempty"`
	Code          *string `json:"-"`              // operator_code column
	CounterpartID int     `json:"-"`              // from seller table
}

// Seller represents a seller/aggregator company.
type Seller struct {
	SellerID         int    `db:"seller_id"`
	Bookable         bool   `db:"bookable"`
	PriceRestriction string `db:"price_restriction"`
}
