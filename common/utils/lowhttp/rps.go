package lowhttp

import (
	"sync/atomic"
	"time"
)

var currentRPS atomic.Int64
var lastRPS int64

func GetLowhttpRPS() int64 {
	return (lastRPS + currentRPS.Load()) / 2
}

func init() {
	go func() {
		for {
			time.Sleep(time.Second)
			lastRPS = currentRPS.Load()
			currentRPS.Store(0)
		}
	}()
}
