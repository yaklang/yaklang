package chanx

import (
	"fmt"
	"runtime"
)

func PrintCurrentGoroutineRuntimeStack() {
	var buf [4096]byte
	n := runtime.Stack(buf[:], false)
	fmt.Printf("Current goroutine call stack:\n%s\n", buf[:n])
}
