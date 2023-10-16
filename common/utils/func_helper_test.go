package utils

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
	"time"
)

func TestNewDebounce(t *testing.T) {
	var count int
	debounceCaller := NewDebounce(1)
	lastIndex := 0
	for i := 0; i < 240; i++ {
		time.Sleep(10 * time.Millisecond)
		debounceCaller(func() {
			count++
			lastIndex = i
			t.Log("debounce")
		})
	}
	time.Sleep(FloatSecondDuration(1.1))
	if count != 1 {
		t.Fatal("debounce failed")
	}
	if lastIndex != 240 {
		spew.Dump(lastIndex)
		t.Fatal("debounce failed")
	}
}

func TestNewThrottle(t *testing.T) {
	var count int
	caller := NewThrottle(1)
	var indexes []int
	for i := 0; i < 240; i++ {
		time.Sleep(10 * time.Millisecond)
		caller(func() {
			count++
			t.Log("throttle")
			spew.Dump(i)
			indexes = append(indexes, i)
		})
	}
	spew.Dump(count)
	if count != 2 {
		t.Fatal("throttle failed")
	}

	if indexes[0] < 80 {
		t.Fatal("throttle failed")
	}
	spew.Dump(indexes)
}
