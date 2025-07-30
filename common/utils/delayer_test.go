package utils

import (
	"testing"
	"time"
)

func TestNewFloatSecondsDelayWaiter(t *testing.T) {
	// 将延迟时间从1秒减少到0.01秒（10毫秒）以优化测试性能
	delayer, err := NewFloatSecondsDelayWaiterSingle(0.01)
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
