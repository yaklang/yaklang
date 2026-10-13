package pcaputil

import (
	"encoding/binary"
	"fmt"
	"math"
)

// WithDNP3ReadPoints selects passive complete-link class READ/header and
// static G1V2/G10V2/G30V1..6 point observations. It does not authenticate an
// endpoint, assemble fragments, decode event objects or certify device success.
func WithDNP3ReadPoints(enabled bool) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.DecodeDNP3ReadPoints = enabled
		return nil
	}
}

// WithSessionDNP3ReadPoints selects the same bounded observation on TCP streams.
func WithSessionDNP3ReadPoints(enabled bool) ProtocolSessionOption {
	return func(s *captureSession) error { s.f.a.config.DecodeDNP3ReadPoints = enabled; return nil }
}

func dnp3ReadClasses(b []byte, limit int) ([]any, byte, error) {
	if len(b) == 0 {
		return nil, 0, protocolError(ErrUnsupportedFeature, "DNP3 selected READ requires class selectors")
	}
	classes := []any{}
	mask := byte(0)
	for len(b) > 0 {
		if len(b) < 3 {
			return nil, 0, protocolError(ErrMalformedMessage, "DNP3 truncated READ class header")
		}
		if b[0] != 60 || b[1] < 1 || b[1] > 4 || b[2] != 6 {
			return nil, 0, protocolError(ErrUnsupportedFeature, "DNP3 READ class variation/qualifier")
		}
		if len(classes) >= limit {
			return nil, 0, protocolError(ErrResourceExceeded, "DNP3 READ class selector count")
		}
		bit := byte(1 << (b[1] - 1))
		if mask&bit != 0 {
			return nil, 0, protocolError(ErrMalformedMessage, "DNP3 duplicate READ class selector")
		}
		mask |= bit
		classes = append(classes, int(b[1]-1))
		b = b[3:]
	}
	return classes, mask, nil
}

func dnp3StaticPoints(b []byte, limit, depth int) ([]any, error) {
	if len(b) > 0 && depth < 3 {
		return nil, protocolError(ErrResourceExceeded, "DNP3 point tree depth")
	}
	objects := []any{}
	total := 0
	seen := map[[3]uint32]bool{}
	for len(b) > 0 {
		if len(b) < 3 {
			return nil, protocolError(ErrMalformedMessage, "DNP3 truncated object header")
		}
		group, variation, qualifier := b[0], b[1], b[2]
		b = b[3:]
		width := 0
		if (group == 1 || group == 10) && variation == 2 {
			width = 1
		} else if group == 30 && variation >= 1 && variation <= 6 {
			width = [7]int{0, 5, 3, 4, 2, 5, 9}[variation]
		} else {
			return nil, protocolError(ErrUnsupportedFeature, "DNP3 unsupported static object group/variation")
		}
		start, count, prefix := 0, 0, 0
		switch qualifier {
		case 0, 1:
			n := 1
			if qualifier == 1 {
				n = 2
			}
			if len(b) < 2*n {
				return nil, protocolError(ErrMalformedMessage, "DNP3 truncated object range")
			}
			stop := 0
			if n == 1 {
				start, stop = int(b[0]), int(b[1])
			} else {
				start, stop = int(binary.LittleEndian.Uint16(b)), int(binary.LittleEndian.Uint16(b[2:]))
			}
			b = b[2*n:]
			if stop < start {
				return nil, protocolError(ErrMalformedMessage, "DNP3 descending object range")
			}
			count = stop - start + 1
		case 0x17, 0x28:
			prefix = 1
			if qualifier == 0x28 {
				prefix = 2
			}
			if len(b) < prefix {
				return nil, protocolError(ErrMalformedMessage, "DNP3 truncated object count")
			}
			count = int(b[0])
			if prefix == 2 {
				count = int(binary.LittleEndian.Uint16(b))
			}
			b = b[prefix:]
			if count == 0 {
				return nil, protocolError(ErrMalformedMessage, "DNP3 empty indexed object count")
			}
		default:
			return nil, protocolError(ErrUnsupportedFeature, "DNP3 unsupported static object qualifier")
		}
		if total >= limit || count > limit-total-1 {
			return nil, protocolError(ErrResourceExceeded, "DNP3 object/point collection count")
		}
		if count > len(b)/(width+prefix) {
			return nil, protocolError(ErrMalformedMessage, "DNP3 truncated point values")
		}
		points := make([]any, 0, count)
		for i := 0; i < count; i++ {
			index := start + i
			if prefix != 0 {
				index = int(b[0])
				if prefix == 2 {
					index = int(binary.LittleEndian.Uint16(b))
				}
				b = b[prefix:]
			}
			key := [3]uint32{uint32(group), uint32(variation), uint32(index)}
			if seen[key] {
				return nil, protocolError(ErrMalformedMessage, "DNP3 duplicate static point index")
			}
			seen[key] = true
			point := map[string]any{"Index": index}
			if group == 30 && (variation == 3 || variation == 4) {
				// These layouts contain no quality octet. Do not fabricate online
				// or cleared flags from the low byte of the signed value.
				point["Quality"] = "not-reported"
				if variation == 3 {
					point["Value"] = int32(binary.LittleEndian.Uint32(b[:4]))
				} else {
					point["Value"] = int32(int16(binary.LittleEndian.Uint16(b[:2])))
				}
				b = b[width:]
				points = append(points, point)
				continue
			}
			flags := b[0]
			point["Flags"] = int(flags)
			point["Online"] = flags&1 != 0
			point["Restart"] = flags&2 != 0
			point["Communication Lost"] = flags&4 != 0
			point["Remote Forced"] = flags&8 != 0
			point["Local Forced"] = flags&16 != 0
			if group == 30 {
				point["Reserved Flags"] = int(flags & 128)
				switch variation {
				case 1:
					point["Value"] = int32(binary.LittleEndian.Uint32(b[1:5]))
				case 2:
					point["Value"] = int32(int16(binary.LittleEndian.Uint16(b[1:3])))
				case 5, 6:
					var value float64
					if variation == 5 {
						bits := binary.LittleEndian.Uint32(b[1:5])
						point["Encoding"], point["Raw Bits"] = "binary32", fmt.Sprintf("%08x", bits)
						value = float64(math.Float32frombits(bits))
					} else {
						bits := binary.LittleEndian.Uint64(b[1:9])
						point["Encoding"], point["Raw Bits"] = "binary64", fmt.Sprintf("%016x", bits)
						value = math.Float64frombits(bits)
					}
					// Keep all IEEE bits, including NaN payloads and signed zero.
					// Nonfinite values remain explicit, JSON-safe observations.
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
				point["Over Range"] = flags&32 != 0
				point["Reference Error"] = flags&64 != 0
			} else {
				point["Value"] = flags&128 != 0
				point["Reserved Flags"] = int(flags & 96)
				if group == 1 {
					point["Reserved Flags"] = int(flags & 64)
				}
				if group == 1 {
					point["Chatter Filter"] = flags&32 != 0
				}
			}
			b = b[width:]
			points = append(points, point)
		}
		objects = append(objects, map[string]any{"Group": int(group), "Variation": int(variation), "Qualifier": int(qualifier), "Points": points})
		total += count + 1
	}
	return objects, nil
}

// Select before pending mutation. Failed object decoding must not consume a
// request and cannot leave a partly accepted field tree or allocation behind.
func (s *binDNP3) observePoints(out map[string]any, dir int, src, dest uint16, user []byte) (byte, error) {
	ac, fc := user[1], user[2]
	key := dnp3RequestKey{dir, src, dest, ac & 15}
	refuse := func(err error) (byte, error) {
		if fc == 1 || fc == 2 {
			delete(s.pending, key)
		}
		if pe, ok := err.(*ProtocolError); ok && pe.Kind != ErrResourceExceeded {
			out["Error Scope"] = "application"
		}
		return 0, err
	}
	switch fc {
	case 0:
		return 0, nil
	case 1:
		classes, mask, err := dnp3ReadClasses(user[3:], sessionCollectionLimit(s.maxPending))
		if err != nil {
			return refuse(err)
		}
		out["Class Selectors"] = classes
		return mask, nil
	case 0x81:
		objects, err := dnp3StaticPoints(user[5:], sessionCollectionLimit(s.maxPending), s.maxDepth)
		if err != nil {
			return refuse(err)
		}
		req, known := s.pending[dnp3RequestKey{1 - dir, dest, src, ac & 15}]
		if known && len(objects) > 0 && req.classes&1 == 0 {
			return refuse(protocolError(ErrContextRequired, "DNP3 static points are not covered by observed READ class selectors"))
		}
		out["Objects"] = objects
		out["Class Context"] = "unobserved"
		if known {
			out["Class Context"] = "observed"
		}
		return 0, nil
	default:
		return refuse(protocolError(ErrUnsupportedFeature, fmt.Sprintf("DNP3 selected READ points do not decode function %d", fc)))
	}
}
