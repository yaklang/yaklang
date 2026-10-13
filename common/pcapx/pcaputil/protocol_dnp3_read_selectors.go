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
	all         bool
	first, last uint16
}

func dnp3DirectReadHeaders(b []byte, limit int) ([]any, byte, []dnp3StaticSelector, []any, error) {
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
		} else {
			if g != 30 || v != 0 {
				return fail(ErrUnsupportedFeature, "DNP3 direct READ requires G30V0")
			}
			x := dnp3StaticSelector{}
			switch q {
			case 6:
				x.all = true
				h["All"] = true
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
	return classes, mask, normalizeDNP3Selectors(selectors), headers, nil
}
func normalizeDNP3Selectors(a []dnp3StaticSelector) []dnp3StaticSelector {
	for _, x := range a {
		if x.all {
			return []dnp3StaticSelector{{all: true}}
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
