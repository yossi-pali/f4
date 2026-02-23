package domain

// VehicleClass represents a vehicle/service class.
type VehicleClass struct {
	ClassID    int    `json:"class_id" db:"class_id"`
	Name       string `json:"name" db:"class_name"`
	Vehclasses string `json:"vehclasses" db:"vehclasses"`
	IsMultiPax bool   `json:"-" db:"is_multi_pax"`
	Seats      string `json:"-" db:"seats"`
}
