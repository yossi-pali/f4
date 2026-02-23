package stage

import (
	"math"
	"testing"
)

func TestHaversineDistance_BangkokToChiangMai(t *testing.T) {
	// Bangkok: lat=13.756429208208447, lng=100.57920577819823
	// Chiang Mai: lat=18.801005483236665, lng=99.01602569335935
	// PHP uses Pythagorean approx (sqrt(dLng²+dLat²)*111111 = 586801.60),
	// but our haversine is more accurate (~585200m).
	got := haversineDistance(
		13.756429208208447, 100.57920577819823,
		18.801005483236665, 99.01602569335935,
	)
	// Haversine result should be ~585 km (±1 km tolerance for earth radius variations)
	if got < 584000 || got > 586500 {
		t.Errorf("haversineDistance(Bangkok, ChiangMai) = %f, want ~585200", got)
	}
}

func TestHaversineDistance_SamePoint(t *testing.T) {
	got := haversineDistance(13.756, 100.579, 13.756, 100.579)
	if got != 0 {
		t.Errorf("haversineDistance(same, same) = %f, want 0", got)
	}
}

func TestHaversineDistance_Antipodal(t *testing.T) {
	// North pole to south pole ~ 20015.09 km
	got := haversineDistance(90, 0, -90, 0)
	expected := math.Pi * 6371000.0 // half circumference
	if math.Abs(got-expected) > 1000 {
		t.Errorf("haversineDistance(NP, SP) = %f, want ~%f", got, expected)
	}
}

func TestHaversineDistance_KnownCities(t *testing.T) {
	// London to Paris ~ 343.5 km
	got := haversineDistance(51.5074, -0.1278, 48.8566, 2.3522)
	if got < 340000 || got > 350000 {
		t.Errorf("haversineDistance(London, Paris) = %f, want ~343500", got)
	}
}
