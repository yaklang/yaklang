package utils

import (
	"testing"
	"time"
)

func TestNewFloatSecondsDelayWaiter(t *testing.T) {
	delayer, err := NewFloatSecondsDelayWaiterSingle(1)
	if err != nil {
		panic(err)
	}
	println(time.Now().String())
	count := 0
	for {
		count++
		delayer.Wait()
		println(time.Now().String())
		if count > 3 {
			return
		}
	}
}
