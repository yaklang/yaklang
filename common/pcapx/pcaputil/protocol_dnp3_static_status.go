package pcaputil

import (
	"encoding/binary"
	"fmt"
	"math"
)

// WithDNP3StaticStatusPoints selects TCP G3V1/2, G10V1/2 and G40V1..4
// static points and V0/concrete all/range READs, plus existing typed static
// observations. Packed formats do not invent quality. G40 quality bits5/6
// remain literal Uninterpreted Quality Bits: fixed references disagree about
// their meaning. This does not certify device configuration, units or operation.
func WithDNP3StaticStatusPoints(enabled bool) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.DecodeDNP3StaticStatusPoints = enabled
		return nil
	}
}

// WithSessionDNP3StaticStatusPoints selects the same bounded TCP observation.
func WithSessionDNP3StaticStatusPoints(enabled bool) ProtocolSessionOption {
	return func(s *captureSession) error { s.f.a.config.DecodeDNP3StaticStatusPoints = enabled; return nil }
}

func dnp3DoubleBitValue(point map[string]any, value uint8) {
	point["Value"] = value
	point["Value State"] = [4]string{"intermediate", "determined-off", "determined-on", "indeterminate"}[value]
}

func dnp3StatusPoint(index int, group, variation byte, b []byte) map[string]any {
	flags := b[0]
	out := map[string]any{"Index": index, "Flags": int(flags)}
	for bit, key := range []string{"Online", "Restart", "Communication Lost", "Remote Forced", "Local Forced"} {
		out[key] = flags&(1<<uint(bit)) != 0
	}
	if group == 3 {
		out["Chatter Filter"] = flags&32 != 0
		dnp3DoubleBitValue(out, flags>>6)
		return out
	}
	// Preserve both disputed quality bits without choosing an unproven meaning
	// from a dissector or SDK. All flag bytes remain exact wire observations.
	out["Uninterpreted Quality Bits"] = int(flags & 96)
	out["Reserved Flags"] = int(flags & 128)
	switch variation {
	case 1:
		out["Value"] = int32(binary.LittleEndian.Uint32(b[1:5]))
	case 2:
		out["Value"] = int32(int16(binary.LittleEndian.Uint16(b[1:3])))
	case 3, 4:
		dnp3IEEEPointValue(out, b[1:])
	}
	return out
}

// Both layouts are validated before this call. Keep all IEEE bits, including
// signed zero and NaN payloads; numeric Value exists only for finite encodings.
func dnp3IEEEPointValue(point map[string]any, b []byte) {
	var value float64
	if len(b) == 4 {
		bits := binary.LittleEndian.Uint32(b)
		point["Encoding"], point["Raw Bits"] = "binary32", fmt.Sprintf("%08x", bits)
		value = float64(math.Float32frombits(bits))
	} else {
		bits := binary.LittleEndian.Uint64(b)
		point["Encoding"], point["Raw Bits"] = "binary64", fmt.Sprintf("%016x", bits)
		value = math.Float64frombits(bits)
	}
	state := "finite"
	if math.IsNaN(value) {
		state = "nan"
	} else if math.IsInf(value, 1) {
		state = "positive-infinity"
	} else if math.IsInf(value, -1) {
		state = "negative-infinity"
	} else {
		point["Value"] = value
	}
	point["Value Status"] = state
}
