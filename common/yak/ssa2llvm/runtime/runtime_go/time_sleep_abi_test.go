package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/aotlib"
)

// TestTimeSleepIntegerWordIsSeconds locks the ABI of yak `time.Sleep(1)`.
// The compiler passes the integer 1 as a raw word. The parameter must be
// float64 seconds, not time.Duration nanoseconds.
func TestTimeSleepIntegerWordIsSeconds(t *testing.T) {
	fn := reflect.ValueOf(aotlib.TimeSleep)
	if fn.Type().In(0).Kind() != reflect.Float64 {
		t.Fatalf("time.Sleep parameter kind = %s, want float64 seconds", fn.Type().In(0).Kind())
	}
	arg, err := runtimeDecodeArg(1, fn.Type().In(0))
	if err != nil {
		t.Fatal(err)
	}
	if arg.Float() != 1 {
		t.Fatalf("decoded time.Sleep(1) = %v, want 1 second", arg.Float())
	}
	start := time.Now()
	fn.Call([]reflect.Value{arg})
	elapsed := time.Since(start)
	if elapsed < 900*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("time.Sleep(1) slept %s", elapsed)
	}
}
