package utils

import (
	"math"
	"math/rand"
	"net"
	"sync"
	"time"
)

func newRnd() *rand.Rand {
	seed := time.Now().UnixNano()
	src := rand.NewSource(seed)
	return rand.New(src)
}

var (
	rnd   = newRnd()
	rndMu sync.Mutex
)

// Return capped exponential backoff with jitter
// http://www.awsarchitectureblog.com/2015/03/backoff.html
func JitterBackoff(min, max time.Duration, attempt int) time.Duration {
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

func randDuration(center time.Duration) time.Duration {
	rndMu.Lock()
	defer rndMu.Unlock()

	ri := int64(center)
	if ri <= 0 {
		return 0
	}
	jitter := rnd.Int63n(ri)
	return time.Duration(math.Abs(float64(ri + jitter)))
}

func isErrorTimeout(err error) bool {
	if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
		return true
	}
	return false
}
