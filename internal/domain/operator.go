package domain

// Operator represents a transport operator.
type Operator struct {
	OperatorID int    `json:"operator_id" db:"operator_id"`
	Name       string `json:"name" db:"operator_name"`
	Slug       string `json:"slug" db:"slug"`
	SellerID   int    `json:"seller_id" db:"seller_id"`
	MasterID   int    `json:"master_id" db:"master_id"`
	Bookable   bool   `json:"-" db:"bookable"`
	LogoURL    string `json:"logo_url,omitempty"`
	RatingAvg  float64 `json:"rating_avg,omitempty"`
	RatingCount int    `json:"rating_count,omitempty"`
}

// Seller represents a seller/aggregator company.
type Seller struct {
	SellerID         int    `db:"seller_id"`
	Bookable         bool   `db:"bookable"`
	PriceRestriction string `db:"price_restriction"`
}
