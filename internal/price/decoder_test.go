package price

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/12go/f4/internal/domain"
)

func TestDecode_TooShort(t *testing.T) {
	_, err := Decode([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error for short input")
	}
}

func TestDecode_NormalFormat(t *testing.T) {
	// 112 bytes of 'A' (0x41) — matches PHP testCreateTripPrice with str_repeat('A', 112)
	data := bytes.Repeat([]byte{0x41}, bytesPriceNormal)

	tp, err := Decode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// flags=0x41 → IsValid=true, IsValidByML=false, etc.
	if !tp.IsValid {
		t.Error("expected IsValid to be true (bit 0 set in 0x41)")
	}
	if tp.PriceLevel != 0x41 {
		t.Errorf("expected PriceLevel=0x41, got %d", tp.PriceLevel)
	}
	// Should have 3 fare types
	for _, ft := range []string{"adult", "child", "infant"} {
		if _, ok := tp.Fares[ft]; !ok {
			t.Errorf("missing fare type %q", ft)
		}
	}
}

func TestDecode_WithOneDiscountDelta(t *testing.T) {
	// 112 + 3*24 = 184 bytes — matches PHP testCreateTripPriceDetailed1
	data := bytes.Repeat([]byte{0x42}, bytesPriceNormal+3*bytesFareDelta)

	tp, err := Decode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With 184 bytes, deltasPerFare should be 1
	if len(tp.DiscountDelta) == 0 {
		t.Error("expected discount deltas to be populated")
	}
	for _, ft := range []string{"adult", "child", "infant"} {
		if _, ok := tp.DiscountDelta[ft]; !ok {
			t.Errorf("missing discount delta for fare type %q", ft)
		}
	}
}

func TestDecode_WithTwoDeltas(t *testing.T) {
	// 112 + 3*48 = 256 bytes — matches PHP testCreateTripPriceDetailed2
	data := bytes.Repeat([]byte{0x43}, bytesPriceNormal+3*2*bytesFareDelta)

	tp, err := Decode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With 256 bytes, deltasPerFare should be 2
	if len(tp.DiscountDelta) == 0 {
		t.Error("expected discount deltas to be populated")
	}
	if len(tp.RuleDelta) == 0 {
		t.Error("expected rule deltas to be populated")
	}
}

func TestDecode_Api4BlockFormat(t *testing.T) {
	// Build binary matching PHP testCreateTripPriceDetailedApi4
	// flags = 0xFF (all bits set, including flagBlockOutput)
	data := make([]byte, 0, 300)
	data = append(data, 0xFF) // flags — block output mode
	data = append(data, bytes.Repeat([]byte{0x43}, bytesPriceNormal-1)...)

	// Footer (20 bytes) is already included in the 112 bytes of normal format
	// Footer is at offset 17 + 3*25 = 92, so bytes 92-111 are footer
	// After the normal 112 bytes, add version block
	version := make([]byte, 4)
	binary.LittleEndian.PutUint32(version, 4) // version = 4
	data = append(data, version...)

	// Block 1: discount delta for adult (type=1, fareIdx=0, len=24)
	blockHeader := []byte{0x01, 0x00}
	blockLen := make([]byte, 2)
	binary.LittleEndian.PutUint16(blockLen, 24)
	data = append(data, blockHeader...)
	data = append(data, blockLen...)
	data = append(data, bytes.Repeat([]byte{0x43}, 24)...)

	// Block 2: rule delta for adult (type=2, fareIdx=0, len=24)
	blockHeader2 := []byte{0x02, 0x00}
	data = append(data, blockHeader2...)
	data = append(data, blockLen...)
	data = append(data, bytes.Repeat([]byte{0x43}, 24)...)

	tp, err := Decode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With block output flag set, should have parsed blocks
	if _, ok := tp.DiscountDelta["adult"]; !ok {
		t.Error("expected adult discount delta from block parsing")
	}
	if _, ok := tp.RuleDelta["adult"]; !ok {
		t.Error("expected adult rule delta from block parsing")
	}
}

func TestDecode_HeaderFields(t *testing.T) {
	data := make([]byte, bytesPriceNormal)

	// Set specific header values (PHP: Cflags/Cavail/Clevel/Creason_id/Vreason_param/Vstamp/Cfxid/Vtotal)
	data[0] = flagValid | flagValidByTTL | flagOutdated // flags
	data[1] = 5                                          // avail
	data[2] = byte(domain.PriceExact)                    // priceLevel
	data[3] = 42                                         // reasonID
	binary.LittleEndian.PutUint32(data[4:8], 100)        // reasonParam (uint32)
	binary.LittleEndian.PutUint32(data[8:12], 1234)      // stamp (uint32)
	data[12] = 3                                         // fxCode index = THB
	binary.LittleEndian.PutUint32(data[13:17], 150050)   // total = 1500.50

	tp, err := Decode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !tp.IsValid {
		t.Error("expected IsValid true")
	}
	if !tp.IsValidByTTL {
		t.Error("expected IsValidByTTL true")
	}
	if tp.IsValidByML {
		t.Error("expected IsValidByML false")
	}
	if !tp.IsOutdated {
		t.Error("expected IsOutdated true")
	}
	if tp.Avail != 5 {
		t.Errorf("expected Avail=5, got %d", tp.Avail)
	}
	if tp.PriceLevel != domain.PriceExact {
		t.Errorf("expected PriceLevel=%d, got %d", domain.PriceExact, tp.PriceLevel)
	}
	if tp.ReasonID != 42 {
		t.Errorf("expected ReasonID=42, got %d", tp.ReasonID)
	}
	if tp.ReasonParam != 100 {
		t.Errorf("expected ReasonParam=100, got %d", tp.ReasonParam)
	}
	if tp.Stamp != 1234 {
		t.Errorf("expected Stamp=1234, got %d", tp.Stamp)
	}
	if tp.FXCode != "THB" {
		t.Errorf("expected FXCode=THB, got %q", tp.FXCode)
	}
	if tp.Total != 1500.50 {
		t.Errorf("expected Total=1500.50, got %f", tp.Total)
	}
}

func TestDecode_FareFields(t *testing.T) {
	data := make([]byte, bytesPriceNormal)
	data[0] = flagValid

	// Set adult fare at offset 17 (after 17-byte header)
	off := bytesHeader
	data[off] = 4                                                   // fullPriceFXCode = USD
	binary.LittleEndian.PutUint32(data[off+1:off+5], 100000)       // fullPrice = 1000.00
	data[off+5] = 3                                                 // netPriceFXCode = THB
	binary.LittleEndian.PutUint32(data[off+6:off+10], 3500000)     // netPrice = 35000.00
	data[off+10] = 4                                                // topupFXCode = USD
	binary.LittleEndian.PutUint32(data[off+11:off+15], 500)        // topup = 5.00
	data[off+15] = 4                                                // sysFeeFXCode = USD
	binary.LittleEndian.PutUint32(data[off+16:off+20], 200)        // sysFee = 2.00
	data[off+20] = 4                                                // agFeeFXCode = USD
	binary.LittleEndian.PutUint32(data[off+21:off+25], 100)        // agFee = 1.00

	tp, err := Decode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	adult := tp.Fares["adult"]
	if adult == nil {
		t.Fatal("adult fare is nil")
	}
	if adult.FullPriceFXCode != "USD" {
		t.Errorf("expected FullPriceFXCode=USD, got %q", adult.FullPriceFXCode)
	}
	if adult.FullPrice != 1000.00 {
		t.Errorf("expected FullPrice=1000.00, got %f", adult.FullPrice)
	}
	if adult.NetPriceFXCode != "THB" {
		t.Errorf("expected NetPriceFXCode=THB, got %q", adult.NetPriceFXCode)
	}
	if adult.NetPrice != 35000.00 {
		t.Errorf("expected NetPrice=35000.00, got %f", adult.NetPrice)
	}
	if adult.Topup != 5.00 {
		t.Errorf("expected Topup=5.00, got %f", adult.Topup)
	}
	if adult.SysFee != 2.00 {
		t.Errorf("expected SysFee=2.00, got %f", adult.SysFee)
	}
	if adult.AgFee != 1.00 {
		t.Errorf("expected AgFee=1.00, got %f", adult.AgFee)
	}
}

func TestDecode_DeltaFareSignedValues(t *testing.T) {
	// Test that negative deltas are decoded correctly via 2's complement
	data := make([]byte, bytesPriceNormal+3*bytesFareDelta)
	data[0] = flagValid // no block output

	// With deltasPerFare=1, fareSize=25+24=49
	// Adult delta starts at offset 17 + 25 = 42
	deltaOff := bytesHeader + bytesFareNormal
	binary.LittleEndian.PutUint32(data[deltaOff:deltaOff+4], 99)  // ID = 99

	// Negative total delta: -50.00 → 2's complement uint32 of -5000
	binary.LittleEndian.PutUint32(data[deltaOff+4:deltaOff+8], i32toU32(-5000))

	tp, err := Decode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	adultDelta := tp.DiscountDelta["adult"]
	if adultDelta == nil {
		t.Fatal("adult discount delta is nil")
	}
	if adultDelta.ID != 99 {
		t.Errorf("expected delta ID=99, got %d", adultDelta.ID)
	}
	if adultDelta.TotalDelta != -50.00 {
		t.Errorf("expected TotalDelta=-50.00, got %f", adultDelta.TotalDelta)
	}
}

func TestFXCodeByIndex(t *testing.T) {
	tests := []struct {
		idx  byte
		want string
	}{
		{0, ""},
		{1, "PCT"},
		{2, "PCD"},
		{3, "THB"},
		{4, "USD"},
		{5, "VND"},
		{8, "EUR"},
		{9, "INR"},
		{171, "AED"},
		{172, ""}, // out of range
		{255, ""}, // out of range
	}
	for _, tt := range tests {
		got := FXCodeByIndex(tt.idx)
		if got != tt.want {
			t.Errorf("FXCodeByIndex(%d) = %q, want %q", tt.idx, got, tt.want)
		}
	}
}

func i32toU32(v int32) uint32 { return uint32(v) }

func TestToSignedFloat(t *testing.T) {
	tests := []struct {
		v    uint32
		want float64
	}{
		{0, 0},
		{100, 1.00},
		{5000, 50.00},
		{i32toU32(-5000), -50.00},
		{i32toU32(-100), -1.00},
	}
	for _, tt := range tests {
		got := toSignedFloat(tt.v)
		if got != tt.want {
			t.Errorf("toSignedFloat(%d) = %f, want %f", tt.v, got, tt.want)
		}
	}
}
