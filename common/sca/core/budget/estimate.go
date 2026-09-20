package budget

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
