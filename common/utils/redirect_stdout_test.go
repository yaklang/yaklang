package utils

import (
	"testing"
)

func TestSafePanicString(t *testing.T) {
	if got := safePanicString(nil); got != "nil" {
		t.Fatalf("nil: got %q", got)
	}
	if got := safePanicString("boom"); got == "" {
		t.Fatalf("string empty")
	}
	var x any = 1
	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic")
			}
			msg := safePanicString(r)
			if msg == "" {
				t.Fatal("safePanicString empty for type-assert panic")
			}
		}()
		_ = x.(string)
	}()
}
