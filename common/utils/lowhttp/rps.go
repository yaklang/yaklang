package lowhttp

import (
	"sync/atomic"
	"time"
)

var currentRPS atomic.Int64
var lastRPS atomic.Int64

func GetLowhttpRPS() int64 {
	return lastRPS.Load()
}

func init() {
	go func() {
		rpsTicker := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-rpsTicker.C:
				lastRPS.Store(currentRPS.Load())
				currentRPS.Store(0)
			}
		}
	}()
}
