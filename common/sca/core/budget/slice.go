package budget

// Grow reserves an explicitly sized backing array before allocating it. The
// caller supplies a conservative element size for its fixed record type. Old
// backing arrays remain charged, covering both growth copies and retained aliases.
func Grow[T any](st *State, values []T, extra int, elementBytes int64) ([]T, error) {
	needed, err := SizeAdd(int64(len(values)), int64(extra))
	if err != nil {
		return nil, err
	}
	if needed <= int64(cap(values)) {
		return values, nil
	}
	capacity := needed
	if int64(cap(values)) <= int64(int(^uint(0)>>1))/2 {
		capacity = max(capacity, int64(cap(values))*2)
	}
	// SizeMul also rejects an overflowing int conversion.
	bytes, err := SizeMul(int(capacity), elementBytes)
	if err != nil {
		return nil, err
	}
	bytes, err = SizeAdd(bytes, SizeSlice)
	if err != nil {
		return nil, err
	}
	if err = st.Working(bytes); err != nil {
		return nil, err
	}
	next := make([]T, len(values), int(capacity))
	copy(next, values)
	return next, nil
}
