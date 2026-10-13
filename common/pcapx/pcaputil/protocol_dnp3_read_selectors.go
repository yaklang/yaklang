package pcaputil

import (
	"encoding/binary"
	"sort"
)

// WithDNP3DirectStaticReads selects bounded TCP G30V0 all/range and class READ
// observations. Range coverage is checked before a response retires its request.
// Other groups, specific requested variations and index-prefix READs remain
// separate capabilities; this does not certify outstation behavior or completeness.
func WithDNP3DirectStaticReads(enabled bool) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.DecodeDNP3DirectStaticReads = enabled
		return nil
	}
}

// WithSessionDNP3DirectStaticReads selects the same bounded observation on TCP.
func WithSessionDNP3DirectStaticReads(enabled bool) ProtocolSessionOption {
	return func(s *captureSession) error { s.f.a.config.DecodeDNP3DirectStaticReads = enabled; return nil }
}

type dnp3StaticSelector struct {
	all              bool
	first, last      uint16
	group, variation byte
	count            uint16 // zero means unbounded; count READs reject zero on the wire
}

func dnp3DirectReadHeaders(b []byte, limit int, typed, counters, events, counts, statuses, binaries bool) ([]any, byte, []dnp3StaticSelector, []any, error) {
	classes := []any{}
	mask := byte(0)
	selectors := []dnp3StaticSelector{}
	headers := []any{}
	fail := func(kind ProtocolErrorKind, why string) ([]any, byte, []dnp3StaticSelector, []any, error) {
		return nil, 0, nil, nil, protocolError(kind, why)
	}
	if len(b) == 0 {
		return fail(ErrUnsupportedFeature, "DNP3 selected READ requires selectors")
	}
	for len(b) > 0 {
		if len(b) < 3 {
			return fail(ErrMalformedMessage, "DNP3 truncated READ selector header")
		}
		if len(headers) >= limit {
			return fail(ErrResourceExceeded, "DNP3 READ selector count")
		}
		g, v, q := b[0], b[1], b[2]
		b = b[3:]
		h := map[string]any{"Group": int(g), "Variation": int(v), "Qualifier": int(q)}
		if g == 60 && v >= 1 && v <= 4 && q == 6 {
			bit := byte(1 << (v - 1))
			if mask&bit != 0 {
				return fail(ErrMalformedMessage, "DNP3 duplicate READ class selector")
			}
			mask |= bit
			classes = append(classes, int(v-1))
			h["Class"] = int(v - 1)
			if typed && v == 1 {
				for _, g := range dnp3StaticReadGroups(counters, statuses) {
					selectors = append(selectors, dnp3StaticSelector{all: true, group: g})
				}
			}
			if events && v > 1 {
				for _, group := range dnp3ObservedEventGroups(binaries) {
					selectors = append(selectors, dnp3StaticSelector{all: true, group: group})
				}
			}
		} else {
			valid := g == 30 && v == 0
			if typed {
				valid = (g == 1 && v <= 2) || (g == 10 && (v == 0 || v == 2)) || (g == 30 && v <= 6)
				if counters && (g == 20 || g == 21) {
					valid = v == 0 || dnp3CounterWidth(g, v) > 0
				}
			}
			if statuses {
				valid = valid || (g == 3 && v <= 2) || (g == 10 && v == 1) || (g == 40 && v <= 4)
			}
			isEvent := events && (g == 22 || g == 23) || binaries && (g == 2 || g == 4 || g == 11)
			if isEvent {
				valid = v == 0 || dnp3CounterEventWidth(g, v) > 0 || binaries && dnp3BinaryEventWidth(g, v) > 0
				if q != 6 && (!counts || (q != 7 && q != 8)) {
					return fail(ErrUnsupportedFeature, "DNP3 event READ requires all-object qualifier")
				}
			}
			if !valid {
				if typed {
					return fail(ErrUnsupportedFeature, "DNP3 unsupported selected static READ group/variation")
				}
				return fail(ErrUnsupportedFeature, "DNP3 direct READ requires G30V0")
			}
			x := dnp3StaticSelector{group: g, variation: v}
			if isEvent {
				h["Event"] = true
			}
			switch q {
			case 6:
				x.all = true
				h["All"] = true
			case 7, 8:
				if !isEvent || !counts {
					return fail(ErrUnsupportedFeature, "DNP3 unsupported direct READ qualifier")
				}
				n := 1
				if q == 8 {
					n = 2
				}
				if len(b) < n {
					return fail(ErrMalformedMessage, "DNP3 truncated READ selector count")
				}
				x.count = uint16(b[0])
				if n == 2 {
					x.count = binary.LittleEndian.Uint16(b)
				}
				b = b[n:]
				if x.count == 0 {
					return fail(ErrMalformedMessage, "DNP3 empty READ selector count")
				}
				h["Count"] = int(x.count)
			case 0, 1:
				n := 1
				if q == 1 {
					n = 2
				}
				if len(b) < 2*n {
					return fail(ErrMalformedMessage, "DNP3 truncated READ selector range")
				}
				if n == 1 {
					x.first, x.last = uint16(b[0]), uint16(b[1])
				} else {
					x.first, x.last = binary.LittleEndian.Uint16(b), binary.LittleEndian.Uint16(b[2:])
				}
				b = b[2*n:]
				if x.last < x.first {
					return fail(ErrMalformedMessage, "DNP3 descending READ selector range")
				}
				h["Start"], h["Stop"] = int(x.first), int(x.last)
			default:
				return fail(ErrUnsupportedFeature, "DNP3 unsupported direct READ qualifier")
			}
			// Retained slice capacity is reflected by memoryTarget before insertion.
			selectors = append(selectors, x)
		}
		headers = append(headers, h)
	}
	if counts {
		hasCount := false
		for _, x := range selectors {
			hasCount = hasCount || x.count != 0
		}
		// Event class membership is absent from wire points. A limited type
		// selection mixed with class selection cannot prove ordered coverage.
		if hasCount && mask&14 != 0 {
			return fail(ErrUnsupportedFeature, "DNP3 counted event READ with class selectors requires class context")
		}
	}
	if typed {
		return classes, mask, normalizeDNP3TypedSelectors(selectors), headers, nil
	}
	return classes, mask, normalizeDNP3Selectors(selectors), headers, nil
}
func normalizeDNP3Selectors(a []dnp3StaticSelector) []dnp3StaticSelector {
	for _, x := range a {
		if x.all {
			return []dnp3StaticSelector{{all: true, group: 30}}
		}
	}
	sort.Slice(a, func(i, j int) bool {
		if a[i].first == a[j].first {
			return a[i].last < a[j].last
		}
		return a[i].first < a[j].first
	})
	n := 0
	for _, x := range a {
		if n > 0 && int(x.first) <= int(a[n-1].last)+1 {
			if x.last > a[n-1].last {
				a[n-1].last = x.last
			}
		} else {
			a[n] = x
			n++
		}
	}
	return a[:n]
}
func dnp3SelectorsCover(selectors []dnp3StaticSelector, objects []any) bool {
	for _, o := range objects {
		obj := o.(map[string]any)
		if obj["Group"] != 30 {
			return false
		}
		for _, p := range obj["Points"].([]any) {
			index := p.(map[string]any)["Index"].(int)
			if len(selectors) > 0 && selectors[0].all {
				continue
			}
			i := sort.Search(len(selectors), func(i int) bool { return int(selectors[i].last) >= index })
			if i == len(selectors) || int(selectors[i].first) > index {
				return false
			}
		}
	}
	return true
}
