package utils

import (
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
