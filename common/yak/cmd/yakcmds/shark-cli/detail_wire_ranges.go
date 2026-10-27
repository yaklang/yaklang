package sharkcli

import "math"

// Range metadata can arrive directly from a native decoder or through worker
// JSON. Keep the same exact nonnegative integer contract in both cases.
func detailRangeOffset(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, v >= 0
	case int64:
		n := int(v)
		return n, n >= 0 && int64(n) == v
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		n := int(v)
		return n, n >= 0 && uint32(n) == v
	case uint64:
		n := int(v)
		return n, n >= 0 && uint64(n) == v
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v != math.Trunc(v) {
			return 0, false
		}
		n := int(v)
		return n, n >= 0 && float64(n) == v
	default:
		return 0, false
	}
}

func detailWireRanges(value any) map[string][]int {
	out := map[string][]int{}
	put := func(name string, first, last any) {
		a, aOK := detailRangeOffset(first)
		b, bOK := detailRangeOffset(last)
		if aOK && bOK && b >= a {
			out[name] = []int{a, b}
		}
	}
	switch ranges := value.(type) {
	case map[string][]int:
		for name, span := range ranges {
			if len(span) == 2 {
				put(name, span[0], span[1])
			}
		}
	case map[string]any:
		for name, value := range ranges {
			switch span := value.(type) {
			case []int:
				if len(span) == 2 {
					put(name, span[0], span[1])
				}
			case []uint64:
				if len(span) == 2 {
					put(name, span[0], span[1])
				}
			case []any:
				if len(span) == 2 {
					put(name, span[0], span[1])
				}
			}
		}
	}
	return out
}
