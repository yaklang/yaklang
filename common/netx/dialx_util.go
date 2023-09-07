package netx

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

// Return capped exponential backoff with jitter
// http://www.awsarchitectureblog.com/2015/03/backoff.html
func jitterBackoff(min, max time.Duration, attempt int) time.Duration {
	base := float64(min)
	capLevel := float64(max)

	temp := math.Min(capLevel, base*math.Exp2(float64(attempt)))
	ri := time.Duration(temp / 2)
	result := randDuration(ri)

	if result < min {
		result = min
	}

	return result
}

func newRnd() *rand.Rand {
	var seed = time.Now().UnixNano()
	var src = rand.NewSource(seed)
	return rand.New(src)
}

var rnd = newRnd()
var rndMu sync.Mutex

func randDuration(center time.Duration) time.Duration {
	rndMu.Lock()
	defer rndMu.Unlock()

	var ri = int64(center)
	if ri <= 0 {
		return 0
	}
	var jitter = rnd.Int63n(ri)
	return time.Duration(math.Abs(float64(ri + jitter)))
}
