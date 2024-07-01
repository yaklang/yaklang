package utils

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"time"
)

func Retry(times int, f func() error) error {
	var err error
	for i := 0; i < times; i++ {
		err = f()
		if err == nil {
			return nil
		}
	}
	return err
}

// retry 对第二个参数作为函数的情况，重试N次，如果第二个参数返回值是 true，则重试，否则就结束，如果遇到错误，停止重试
// Example:
// ```
// count = 0
//
//	retry(100, () => {
//	   defer recover()
//
//	   count++
//	   if count > 3 {
//	       die(111)
//	   }
//	   return true
//	})
//
// assert count == 4, f`${count}`
//
// count = 0
//
//	retry(100, () => {
//	   defer recover()
//
//	   count++
//	   if count > 3 {
//	       return false
//	   }
//	   return true
//	})
//
// assert count == 4, f`${count}`
//
// count = 0
//
//	retry(100, () => {
//	   count++
//	})
//
// assert count == 1, f`${count}`
//
// count = 0
//
//	retry(100, () => {
//	   count++
//	   return true
//	})
//
// assert count == 100, f`${count}`
// ```
func Retry2(i int, handler func() bool) {
	wrapperHandler := func() (ret bool) {
		if err := recover(); err != nil {
			log.Warnf("retry handler failed: %v", err)
			ret = false
		}
		ret = handler()
		return
	}
	// retry until handler's result is true
	for i > 0 {
		if !wrapperHandler() {
			return
		}
		i--
	}
}

func RetryWithExpBackOff(f func() error) error {
	return RetryWithExpBackOffEx(5, 300, f)
}

func RetryWithExpBackOffEx(times int, begin int, f func() error) error {
	var err error
	for i := 0; i < times; i++ {
		err = f()
		if err == nil {
			return nil
		}
		time.Sleep(time.Duration(begin*(1<<i)) * time.Millisecond)
	}
	return err
}

func AttemptWithDelay(maxIteration int, delay time.Duration, f func() error) error {
	_, _, err := lo.AttemptWithDelay(maxIteration, delay, func(index int, duration time.Duration) error {
		return f()
	})
	return err
}

func AttemptWithDelayFast(f func() error) error {
	return AttemptWithDelay(3, 300*time.Millisecond, f)
}
