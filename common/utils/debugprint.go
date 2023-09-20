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

func TryCloseChannel(i any) {
	if i == nil {
		return
	}
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("close channel failed: %v", err)
		}
	}()

	if reflect.TypeOf(i).Kind() == reflect.Chan {
		reflect.ValueOf(i).Close()
	}
}
