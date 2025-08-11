package utils

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"reflect"
	"runtime"
)

func PrintCurrentGoroutineRuntimeStack() {
	var buf [4096]byte
	n := runtime.Stack(buf[:], false)
	fmt.Printf("Current goroutine call stack:\n%s\n", buf[:n])
}

func TryWriteChannel[T any](c chan T, data T) (ret bool) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("write channel failed: %v", err)
			ret = false
		}
	}()
	c <- data
	return true
}

func TryCloseChannel(i any) {
	if i == nil {
		return
	}
	defer func() {
		if err := recover(); err != nil {
			Debug(func() {
				log.Infof("close channel failed (maybe already closed): %v", err)
			})
		}
	}()

	if reflect.TypeOf(i).Kind() == reflect.Chan {
		reflect.ValueOf(i).Close()
	}
}
