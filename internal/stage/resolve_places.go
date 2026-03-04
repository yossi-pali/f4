package stage

import (
	"context"
	"math"

	"github.com/12go/f4/internal/domain"
	"github.com/12go/f4/internal/pipeline"
	"github.com/12go/f4/internal/repository"
)

// ResolvePlacesInput is the input for Stage 1.
type ResolvePlacesInput struct {
	FromPlaceID  string
	ToPlaceID    string
	FromStations []int // optional explicit station IDs (searchByStations endpoint)
	ToStations   []int
}

// ResolvedPlaces is the output of Stage 1.
type ResolvedPlaces struct {
	FromPlaceID    string
	ToPlaceID      string
	FromStationIDs []int
	ToStationIDs   []int
	FromPlace      domain.Place
	ToPlace        domain.Place
	Distance       float64 // meters
	IsNotPossible  bool
}

// ResolvePlacesStage resolves place IDs to station IDs.
type ResolvePlacesStage struct {
	stationRepo *repository.StationRepo
}

func NewResolvePlacesStage(stationRepo *repository.StationRepo) *ResolvePlacesStage {
	return &ResolvePlacesStage{stationRepo: stationRepo}
}

func (s *ResolvePlacesStage) Name() string { return "resolve_places" }

func (s *ResolvePlacesStage) Execute(ctx context.Context, in ResolvePlacesInput) (ResolvedPlaces, error) {
	out := ResolvedPlaces{
		FromPlaceID: in.FromPlaceID,
		ToPlaceID:   in.ToPlaceID,
	}

	// If explicit station IDs are provided (searchByStations endpoint), use them directly.
	if len(in.FromStations) > 0 && len(in.ToStations) > 0 {
		out.FromStationIDs = in.FromStations
		out.ToStationIDs = in.ToStations
		out.FromPlaceID = domain.UnknownPlace
		out.ToPlaceID = domain.UnknownPlace
		return out, nil
	}

	pc := pipeline.FromContext(ctx)
	const stage = "resolve_places"

	// Resolve from place
	t := pc.StartTimer(stage, "from_resolve")
	fromIDs, err := s.stationRepo.ResolvePlaceToStationIDs(ctx, in.FromPlaceID)
	t.Stop()
	if err != nil {
		return out, err
	}
	if len(fromIDs) == 0 {
		out.IsNotPossible = true
		return out, nil
	}
	out.FromStationIDs = fromIDs

	// Resolve to place
	t = pc.StartTimer(stage, "to_resolve")
	toIDs, err := s.stationRepo.ResolvePlaceToStationIDs(ctx, in.ToPlaceID)
	t.Stop()
	if err != nil {
		return out, err
	}
	if len(toIDs) == 0 {
		out.IsNotPossible = true
		return out, nil
	}
	out.ToStationIDs = toIDs

	// Load place data for from and to
	fromPlace, err := s.stationRepo.GetPlaceData(ctx, in.FromPlaceID)
	if err != nil {
		return out, err
	}
	out.FromPlace = fromPlace

	toPlace, err := s.stationRepo.GetPlaceData(ctx, in.ToPlaceID)
	if err != nil {
		return out, err
	}
	out.ToPlace = toPlace

	// Calculate distance between places
	out.Distance = haversineDistance(fromPlace.Lat, fromPlace.Lng, toPlace.Lat, toPlace.Lng)

	return out, nil
}

// haversineDistance calculates the distance in meters between two lat/lng points.
func haversineDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusM = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}
