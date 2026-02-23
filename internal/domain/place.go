package domain

// Place represents either a station or a province.
type Place struct {
	ID         int
	Type       PlaceType // "s" for station, "p" for province
	Name       string
	Lat        float64
	Lng        float64
	ProvinceID int
	ParentID   int // for provinces: parent province
}

type PlaceType string

const (
	PlaceTypeStation  PlaceType = "s"
	PlaceTypeProvince PlaceType = "p"
)

const UnknownPlace = "unknown"

// Station represents a transport station.
type Station struct {
	StationID            int     `json:"station_id" db:"station_id"`
	StationName          string  `json:"station_name" db:"station_name"`
	StationCode          string  `json:"station_code,omitempty" db:"station_code"`
	StationSlug          string  `json:"station_slug,omitempty"`
	Lat                  float64 `json:"lat" db:"lat"`
	Lng                  float64 `json:"lng" db:"lng"`
	ProvinceID           int     `json:"province_id" db:"province_id"`
	CountryID            string  `json:"country_id" db:"country_id"`
	VehclassID           string  `json:"vehclass_id" db:"vehclass_id"`
	TimezoneID           int     `json:"-" db:"timezone_id"`
	TimezoneName         string  `json:"-" db:"timezone_name"`
	Weight               int     `json:"-" db:"weight_from"`
	CoordinatesAccurate  bool    `json:"-" db:"coordinates_accurate"`
}

// Province represents a geographic province/region.
type Province struct {
	ProvinceID   int     `json:"province_id" db:"province_id"`
	ProvinceName string  `json:"province_name" db:"province_name"`
	Lat          float64 `json:"lat" db:"lat"`
	Lng          float64 `json:"lng" db:"lng"`
	ParentID     *int    `json:"parent_id" db:"parent_id"`
	CountryID    string  `json:"country_id" db:"country_id"`
}
