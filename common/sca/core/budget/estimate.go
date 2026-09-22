package budget

import "github.com/yaklang/yaklang/common/sca/core/scanerr"

// Logical size units for result-memory accounting. These are conservative
// working-set estimates, not Go heap, GC, or RSS measurements.
const (
	SizeObject int64 = 64
	SizeMap    int64 = 48
	SizeSlice  int64 = 24
	SizeString int64 = 16
	SizePtr    int64 = 8
)

func SizeOfString(s string) int64 { return SizeString + int64(len(s)) }

// SizeOfStrings is a conservative slice+header charge for copying extra
// string evidence during merge. An empty input is free.
func SizeOfStrings(ss []string) int64 {
	if len(ss) == 0 {
		return 0
	}
	n := SizeSlice
	for _, s := range ss {
		n += SizeOfString(s)
	}
	return n
}

func SizeOfBytes(n int) int64 {
	if n < 0 {
		return SizeSlice
	}
	return SizeSlice + int64(n)
}

func SizeOfNode(raw int) int64 { return SizeObject + SizeMap + SizeSlice + int64(max(0, raw)) }

func SizeOfPackage(name, version, extra string) int64 {
	return SizeObject*6 + SizeOfString(name) + SizeOfString(version) + SizeOfString(extra) + 256
}

func SizeOfComponent(name, version string) int64 {
	return SizeObject + SizeOfString(name) + SizeOfString(version) + 128
}

func SizeOfObservation() int64 { return 192 }

func SizeOfEdge() int64 { return 128 }

func SizeOfJob() int64 { return 96 }

func SizeOfRecord() int64 { return 160 }

// SizeDecoderScratch is a conservative allowance for encoding/json and
// encoding/xml internal buffers. It is not a measurement of those decoders.
const SizeDecoderScratch int64 = 4096

// SizeReadScratch is the pipeline snapshot read buffer.
const SizeReadScratch int64 = 32 << 10

func SizeOfSortIndex(n int) int64 {
	if n < 0 {
		return 0
	}
	v, err := SizeMul(n, SizePtr)
	if err != nil {
		return -1
	}
	return v
}

// SizeAdd sums working-set charges. A negative part is a failed prior
// multiply/overflow, not a value that may cancel into a positive total.
func SizeAdd(parts ...int64) (int64, error) {
	var sum int64
	max := int64(^uint64(0) >> 1)
	for _, p := range parts {
		if p < 0 {
			return 0, scanerr.New(scanerr.ResourceLimit, "negative size add")
		}
		if p > 0 && sum > max-p {
			return 0, scanerr.New(scanerr.ResourceLimit, "size add overflow")
		}
		sum += p
	}
	return sum, nil
}

// SizeOfJSONString is a conservative escaped JSON string working copy:
// quotes plus six bytes per source byte (\u00XX), not a measured encoder.
func SizeOfJSONString(s string) (int64, error) {
	esc, err := SizeMul(len(s), 6)
	if err != nil {
		return 0, err
	}
	return SizeAdd(2, esc)
}

// SizeMul is a saturating-safe product for capacity charges. Overflow is a
// resource_limit, not a wrapped integer.
func SizeMul(n int, unit int64) (int64, error) {
	if n < 0 || unit < 0 {
		return 0, scanerr.New(scanerr.ResourceLimit, "negative size multiply")
	}
	if n == 0 || unit == 0 {
		return 0, nil
	}
	max := int64(^uint64(0) >> 1)
	if unit != 0 && int64(n) > max/unit {
		return 0, scanerr.New(scanerr.ResourceLimit, "size multiply overflow")
	}
	return int64(n) * unit, nil
}
