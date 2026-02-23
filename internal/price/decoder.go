package price

import (
	"encoding/binary"
	"fmt"

	"github.com/12go/f4/internal/domain"
)

const (
	bytesFareNormal  = 25
	bytesFareDelta   = 24
	bytesHeader      = 17 // flags(1)+avail(1)+level(1)+reason_id(1)+reason_param(4)+stamp(4)+fxid(1)+total(4)
	bytesPriceNormal = bytesHeader + 3*bytesFareNormal + 20 // 17 + 75 + 20 = 112

	flagValid       = 0b00000001
	flagValidByTTL  = 0b00000010
	flagValidByML   = 0b00000100
	flagExperiment  = 0b00001000
	flagOutdated    = 0b00010000
	flagBlockOutput = 0b10000000
)

var fareTypes = [3]string{"adult", "child", "infant"}

// Decode parses a binary price string into a TripPrice.
// Ported from PHP PriceBinaryParser::parseTripPrice().
//
// PHP header format: Cflags/Cavail/Clevel/Creason_id/Vreason_param/Vstamp/Cfxid/Vtotal
// where C=uint8, V=uint32_LE. Total header = 17 bytes.
// The stored function price_5_6_pool() may return a compact (header-only) format
// when detailed fare data is unavailable.
func Decode(data []byte) (domain.TripPrice, error) {
	if len(data) < bytesHeader {
		return domain.TripPrice{}, fmt.Errorf("price binary too short: %d bytes (need %d)", len(data), bytesHeader)
	}

	tp := domain.TripPrice{
		Fares:         make(map[string]*domain.PriceFare, 3),
		DiscountDelta: make(map[string]*domain.PriceDeltaFare),
		RuleDelta:     make(map[string]*domain.PriceDeltaFare),
	}

	// Header: 17 bytes (PHP: Cflags/Cavail/Clevel/Creason_id/Vreason_param/Vstamp/Cfxid/Vtotal)
	flags := data[0]
	tp.IsValid = flags&flagValid != 0
	tp.IsValidByTTL = flags&flagValidByTTL != 0
	tp.IsValidByML = flags&flagValidByML != 0
	tp.IsExperiment = flags&flagExperiment != 0
	tp.IsOutdated = flags&flagOutdated != 0

	tp.Avail = int(data[1])
	tp.PriceLevel = int(data[2])
	tp.ReasonID = int(data[3])
	tp.ReasonParam = int(binary.LittleEndian.Uint32(data[4:8]))
	tp.Stamp = int(binary.LittleEndian.Uint32(data[8:12]))
	tp.FXCode = FXCodeByIndex(data[12])
	tp.Total = float64(binary.LittleEndian.Uint32(data[13:17])) / 100.0

	dataLen := len(data)

	// Short format: header only (e.g., 24 bytes from price_5_6_pool compact output).
	// No fare breakdown available, but header fields (flags, total, avail) are valid.
	if dataLen < bytesPriceNormal {
		return tp, nil
	}

	// Determine format based on length and flags
	hasBlocks := flags&flagBlockOutput != 0

	// Fares: 3 × 25 bytes starting at offset 17
	fareOffset := bytesHeader
	fareSize := bytesFareNormal

	// Detect detailed format
	deltasPerFare := 0
	if !hasBlocks {
		// 184 bytes = normal + 1 delta per fare
		if dataLen == bytesPriceNormal+3*bytesFareDelta {
			deltasPerFare = 1
			fareSize = bytesFareNormal + bytesFareDelta
		}
		// 256 bytes = normal + 2 deltas per fare
		if dataLen == bytesPriceNormal+3*2*bytesFareDelta {
			deltasPerFare = 2
			fareSize = bytesFareNormal + 2*bytesFareDelta
		}
	}

	for i, fareType := range fareTypes {
		offset := fareOffset + i*fareSize
		if offset+bytesFareNormal > dataLen {
			break
		}
		fare := decodeFare(data[offset : offset+bytesFareNormal])
		tp.Fares[fareType] = &fare

		// Parse deltas if present
		deltaOffset := offset + bytesFareNormal
		if deltasPerFare >= 1 && deltaOffset+bytesFareDelta <= dataLen {
			delta := decodeDeltaFare(data[deltaOffset : deltaOffset+bytesFareDelta])
			tp.DiscountDelta[fareType] = &delta
		}
		if deltasPerFare >= 2 && deltaOffset+2*bytesFareDelta <= dataLen {
			delta := decodeDeltaFare(data[deltaOffset+bytesFareDelta : deltaOffset+2*bytesFareDelta])
			tp.RuleDelta[fareType] = &delta
		}
	}

	// Footer: 20 bytes after all fares
	footerOffset := fareOffset + 3*fareSize
	if footerOffset+20 <= dataLen {
		tp.MLScore = float64(binary.LittleEndian.Uint32(data[footerOffset:footerOffset+4])) / 100000.0
		tp.Duration = int(binary.LittleEndian.Uint32(data[footerOffset+4 : footerOffset+8]))
		tp.AdvHour = int(binary.LittleEndian.Uint32(data[footerOffset+8 : footerOffset+12]))
		tp.LegacyRouteID = int(binary.LittleEndian.Uint32(data[footerOffset+12 : footerOffset+16]))
		tp.LegacyTripID = int(binary.LittleEndian.Uint32(data[footerOffset+16 : footerOffset+20]))
	}

	// API v4+ block parsing
	if hasBlocks {
		blockOffset := footerOffset + 20
		if blockOffset+4 <= dataLen {
			// Skip 4-byte version
			blockOffset += 4
			parseBlocks(data[blockOffset:], &tp)
		}
	}

	return tp, nil
}

// decodeFare parses 25 bytes into a PriceFare.
func decodeFare(data []byte) domain.PriceFare {
	return domain.PriceFare{
		FullPriceFXCode: FXCodeByIndex(data[0]),
		FullPrice:       float64(binary.LittleEndian.Uint32(data[1:5])) / 100.0,
		NetPriceFXCode:  FXCodeByIndex(data[5]),
		NetPrice:        float64(binary.LittleEndian.Uint32(data[6:10])) / 100.0,
		TopupFXCode:     FXCodeByIndex(data[10]),
		Topup:           float64(binary.LittleEndian.Uint32(data[11:15])) / 100.0,
		SysFeeFXCode:    FXCodeByIndex(data[15]),
		SysFee:          float64(binary.LittleEndian.Uint32(data[16:20])) / 100.0,
		AgFeeFXCode:     FXCodeByIndex(data[20]),
		AgFee:           float64(binary.LittleEndian.Uint32(data[21:25])) / 100.0,
	}
}

// decodeDeltaFare parses 24 bytes into a PriceDeltaFare.
func decodeDeltaFare(data []byte) domain.PriceDeltaFare {
	return domain.PriceDeltaFare{
		ID:            int(binary.LittleEndian.Uint32(data[0:4])),
		TotalDelta:    toSignedFloat(binary.LittleEndian.Uint32(data[4:8])),
		NetPriceDelta: toSignedFloat(binary.LittleEndian.Uint32(data[8:12])),
		TopupDelta:    toSignedFloat(binary.LittleEndian.Uint32(data[12:16])),
		SysFeeDelta:   toSignedFloat(binary.LittleEndian.Uint32(data[16:20])),
		AgFeeDelta:    toSignedFloat(binary.LittleEndian.Uint32(data[20:24])),
	}
}

// toSignedFloat converts a uint32 to a signed float64 (divide by 100).
// Values >= 0x80000000 are negative (2's complement).
func toSignedFloat(v uint32) float64 {
	if v >= 0x80000000 {
		return float64(int32(v)) / 100.0
	}
	return float64(v) / 100.0
}

const (
	blockTypeDeltaFareDiscount = 0x01
	blockTypeDeltaFareRule     = 0x02
)

// parseBlocks parses API v4+ extended blocks.
func parseBlocks(data []byte, tp *domain.TripPrice) {
	offset := 0
	for offset+4 <= len(data) {
		blockType := data[offset]
		fareIdx := data[offset+1]
		blockLen := int(binary.LittleEndian.Uint16(data[offset+2 : offset+4]))
		offset += 4

		if offset+blockLen > len(data) {
			break
		}

		if fareIdx >= 3 || blockLen < bytesFareDelta {
			offset += blockLen
			continue
		}

		fareType := fareTypes[fareIdx]
		delta := decodeDeltaFare(data[offset : offset+bytesFareDelta])

		switch blockType {
		case blockTypeDeltaFareDiscount:
			tp.DiscountDelta[fareType] = &delta
		case blockTypeDeltaFareRule:
			tp.RuleDelta[fareType] = &delta
		}

		offset += blockLen
	}
}
