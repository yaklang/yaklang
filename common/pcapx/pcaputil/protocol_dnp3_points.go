package pcaputil

import (
	"encoding/binary"
	"fmt"
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

func dnp3StaticPoints(b []byte, limit, depth int, packed, counters, events, statuses bool) ([]any, error) {
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
		isPacked := packed && variation == 1 && (group == 1 || statuses && (group == 3 || group == 10))
		packedBits := 1
		if group == 3 {
			packedBits = 2
		}
		isStatus := statuses && ((group == 3 && variation == 2) || (group == 40 && variation >= 1 && variation <= 4))
		isCounter := counters && dnp3CounterWidth(group, variation) > 0
		isEvent := events && dnp3CounterEventWidth(group, variation) > 0
		if isPacked {
			width = 1
		} else if isStatus {
			width = 1
			if group == 40 {
				width = [5]int{0, 5, 3, 5, 9}[variation]
			}
		} else if isEvent {
			width = dnp3CounterEventWidth(group, variation)
			if qualifier != 23 && qualifier != 40 {
				return nil, protocolError(ErrUnsupportedFeature, "DNP3 counter events require indexed qualifiers")
			}
		} else if isCounter {
			width = dnp3CounterWidth(group, variation)
		} else if (group == 1 || group == 10) && variation == 2 {
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
		if isPacked {
			if prefix != 0 {
				return nil, protocolError(ErrUnsupportedFeature, "DNP3 packed binary requires contiguous range qualifier")
			}
			if (count*packedBits+7)/8 > len(b) {
				return nil, protocolError(ErrMalformedMessage, "DNP3 truncated packed binary values")
			}
		} else if count > len(b)/(width+prefix) {
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
			if !isEvent && seen[key] {
				return nil, protocolError(ErrMalformedMessage, "DNP3 duplicate static point index")
			}
			if !isEvent {
				seen[key] = true
			}
			if isEvent {
				point := dnp3CounterPoint(index, 21, variation, b[:width])
				point["Event Class"] = "not-reported"
				points = append(points, point)
				b = b[width:]
				continue
			}
			point := map[string]any{"Index": index}
			if isCounter {
				points = append(points, dnp3CounterPoint(index, group, variation, b[:width]))
				b = b[width:]
				continue
			}
			if isStatus {
				points = append(points, dnp3StatusPoint(index, group, variation, b[:width]))
				b = b[width:]
				continue
			}
			if isPacked {
				point["Quality"] = "not-reported"
				if group == 3 {
					dnp3DoubleBitValue(point, (b[i/4]>>uint(2*(i%4)))&3)
				} else {
					point["Value"] = b[i/8]&(1<<uint(i%8)) != 0
				}
				points = append(points, point)
				continue
			}
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
					dnp3IEEEPointValue(point, b[1:width])
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
		if isPacked {
			b = b[(count*packedBits+7)/8:]
		}
		object := map[string]any{"Group": int(group), "Variation": int(variation), "Qualifier": int(qualifier), "Points": points}
		if isEvent {
			object["Event"] = true
		}
		objects = append(objects, object)
		total += count + 1
	}
	return objects, nil
}

// Select before pending mutation. Failed object decoding must not consume a
// request and cannot leave a partly accepted field tree or allocation behind.
func (s *binDNP3) observePoints(out map[string]any, dir int, src, dest uint16, user []byte) (byte, []dnp3StaticSelector, error) {
	ac, fc := user[1], user[2]
	key := dnp3RequestKey{dir, src, dest, ac & 15, out["Control"].(int)&0x80 != 0}
	refuse := func(err error) (byte, []dnp3StaticSelector, error) {
		if fc == 1 || fc == 2 {
			delete(s.pending, key)
		}
		if pe, ok := err.(*ProtocolError); ok && pe.Kind != ErrResourceExceeded {
			out["Error Scope"] = "application"
		}
		return 0, nil, err
	}
	switch fc {
	case 0:
		return 0, nil, nil
	case 1:
		if s.directReads {
			classes, mask, selectors, headers, err := dnp3DirectReadHeaders(user[3:], sessionCollectionLimit(s.maxPending), s.typedReads, s.counterPoints, s.counterEvents, s.eventCountReads, s.statusPoints)
			if err != nil {
				return refuse(err)
			}
			if len(classes) > 0 {
				out["Class Selectors"] = classes
			}
			out["Read Selectors"] = headers
			return mask, selectors, nil
		}
		classes, mask, err := dnp3ReadClasses(user[3:], sessionCollectionLimit(s.maxPending))
		if err != nil {
			return refuse(err)
		}
		out["Class Selectors"] = classes
		return mask, nil, nil
	case 0x81, 0x82:
		if fc == 0x82 && !s.counterEvents {
			return refuse(protocolError(ErrUnsupportedFeature, "DNP3 selected READ points do not decode function 130"))
		}
		objects, err := dnp3StaticPoints(user[5:], sessionCollectionLimit(s.maxPending), s.maxDepth, s.typedReads, s.counterPoints, s.counterEvents, s.statusPoints)
		if err != nil {
			return refuse(err)
		}
		hasStatic, hasEvents := false, false
		for _, o := range objects {
			if o.(map[string]any)["Event"] == true {
				hasEvents = true
			} else {
				hasStatic = true
			}
		}
		if fc == 0x82 && hasStatic {
			return refuse(protocolError(ErrUnsupportedFeature, "DNP3 selected unsolicited response requires counter events"))
		}
		req, known := s.pending[dnp3RequestKey{1 - dir, dest, src, ac & 15, out["Control"].(int)&0x80 == 0}]
		if fc == 0x82 {
			known = false
		}
		if known && s.directReads && !dnp3StaticSnapshotUnique(objects) {
			return refuse(protocolError(ErrContextRequired, "DNP3 static points are outside observed READ selectors"))
		}
		if known && len(objects) > 0 && s.typedReads {
			if !dnp3TypedSelectorsCover(req.selectors, objects) {
				return refuse(protocolError(ErrContextRequired, "DNP3 static points are outside observed READ selectors"))
			}
		} else if known && len(objects) > 0 && req.classes&1 == 0 {
			if !s.directReads || len(req.selectors) == 0 {
				return refuse(protocolError(ErrContextRequired, "DNP3 static points are not covered by observed READ class selectors"))
			}
			if !dnp3SelectorsCover(req.selectors, objects) {
				return refuse(protocolError(ErrContextRequired, "DNP3 static points are outside observed READ selectors"))
			}
		}
		out["Objects"] = objects
		out["Class Context"] = "unobserved"
		if known && (!s.directReads || req.classes != 0) {
			out["Class Context"] = "observed"
		}
		if s.directReads {
			out["Read Context"], out["Read Coverage"] = "unobserved", "unobserved"
			if known {
				out["Read Context"] = "observed"
				if len(objects) == 0 {
					out["Read Coverage"] = "no-static-data"
				} else if hasEvents {
					out["Read Coverage"] = "selected-events"
					if hasStatic {
						out["Read Coverage"] = "selected-static-and-events"
					}
				} else if s.typedReads {
					out["Read Coverage"] = "selected-static"
				} else if req.classes&1 != 0 {
					out["Read Coverage"] = "class0"
				} else {
					out["Read Coverage"] = "direct-static"
				}
			}
		}
		return 0, nil, nil
	default:
		return refuse(protocolError(ErrUnsupportedFeature, fmt.Sprintf("DNP3 selected READ points do not decode function %d", fc)))
	}
}

// A selected static snapshot has one record per group/index. Different
// variations cannot turn the same point into two independently selected points.
// Unassociated wire observations remain literal; ordered event records can
// legitimately repeat an index and are excluded from this static invariant.
func dnp3StaticSnapshotUnique(objects []any) bool {
	seen := map[uint32]struct{}{}
	for _, o := range objects {
		object := o.(map[string]any)
		if object["Event"] == true {
			continue
		}
		group := uint32(object["Group"].(int))
		for _, p := range object["Points"].([]any) {
			key := group<<16 | uint32(p.(map[string]any)["Index"].(int))
			if _, duplicate := seen[key]; duplicate {
				return false
			}
			seen[key] = struct{}{}
		}
	}
	return true
}
