package base

func InterfaceToUint64(d any) (uint64, bool) {
	switch ret := d.(type) {
	case uint64:
		return ret, true
	case uint32:
		return uint64(ret), true
	case uint16:
		return uint64(ret), true
	case uint8:
		return uint64(ret), true
	case int64:
		return uint64(ret), true
	case int32:
		return uint64(ret), true
	case int16:
		return uint64(ret), true
	case int8:
		return uint64(ret), true
	case int:
		return uint64(ret), true
	case float64:
		return uint64(ret), true
	case float32:
		return uint64(ret), true
	}
	return 0, false
}
