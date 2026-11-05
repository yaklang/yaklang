package sharkcli

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"
)

func httpLatencyMetadata(name string, value any) (string, bool) {
	if name != "Response Latency" {
		return "", false
	}
	// httpEvidence stores elapsed/time.Microsecond. Its original Go value
	// retains time.Duration's named type, but its number already counts us;
	// Duration.String would incorrectly print nanoseconds. DoH copies this
	// exact HTTP exchange observation. JSON workers retain the same count.
	switch v := value.(type) {
	case time.Duration:
		return fmt.Sprintf("%d us", int64(v)), true
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d us", v), true
	case float64:
		if math.Trunc(v) == v && math.Abs(v) <= 1<<53-1 {
			return fmt.Sprintf("%.0f us", v), true
		}
	case json.Number:
		if count, err := strconv.ParseInt(string(v), 10, 64); err == nil {
			return fmt.Sprintf("%d us", count), true
		}
	}
	return "", false
}
