package yakvm

import "fmt"

func normalizeIndex(index, length int) (int, bool) {
	if index < 0 {
		index += length
	}
	return index, index >= 0 && index < length
}

// normalizeSlice preserves strict Yak bounds and copy semantics. Omission is
// distinct from explicit 0/-1. The count formula also handles MinInt steps.
func normalizeSlice(length, start, end, step, omitted int) (int, int) {
	if step == 0 {
		panic("slice call step cannot be 0")
	}
	if omitted&1 != 0 {
		start = 0
		if step < 0 {
			start = length - 1
		}
	} else if start < 0 {
		start += length
	}
	if omitted&2 != 0 {
		end = length
		if step < 0 {
			end = -1
		}
	} else if end < 0 {
		end += length
	}
	if start < 0 && !(length == 0 && omitted&1 != 0) || start > length {
		panic("slice call error, start index out of range")
	}
	if end < 0 && !(end == -1 && omitted&2 != 0 && step < 0) || end > length {
		panic("slice call error, end index out of range")
	}
	count := 0
	if step > 0 && start < end {
		count = (end-start-1)/step + 1
	}
	if step < 0 && start > end {
		stride := uint(-(step + 1)) + 1
		count = int(uint(start-end-1)/stride) + 1
	}
	if count > 0 && (start < 0 || start >= length) {
		panic(fmt.Sprintf("slice call error, start index %d out of range", start))
	}
	return start, count
}
