package atomicbool

import (
	"sync"
	"testing"
)

func TestBool(t *testing.T) {
	v := NewBool(true)
	if !v.IsSet() {
		t.Fatal("NewValue(true) failed")
	}

	v = NewBool(false)
	if v.IsSet() {
		t.Fatal("NewValue(false) failed")
	}

	v = New()
	if v.IsSet() {
		t.Fatal("Empty value of AtomicBool should be false")
	}

	v.Set()
	if !v.IsSet() {
		t.Fatal("AtomicBool.Set() failed")
	}

	v.UnSet()
	if v.IsSet() {
		t.Fatal("AtomicBool.UnSet() failed")
	}

	v.SetTo(true)
	if !v.IsSet() {
		t.Fatal("AtomicBool.SetTo(true) failed")
	}

	v.SetTo(false)
	if v.IsSet() {
		t.Fatal("AtomicBool.SetTo(false) failed")
	}

	if set := v.SetToIf(true, false); set || v.IsSet() {
		t.Fatal("AtomicBool.SetTo(true, false) failed")
	}

	if set := v.SetToIf(false, true); !set || !v.IsSet() {
		t.Fatal("AtomicBool.SetTo(false, true) failed")
	}
}

func TestRace(t *testing.T) {
	repeat := 10000
	var wg sync.WaitGroup
	wg.Add(repeat * 3)
	v := New()

	// Writer
	go func() {
		for i := 0; i < repeat; i++ {
			v.Set()
			wg.Done()
		}
	}()

	// Reader
	go func() {
		for i := 0; i < repeat; i++ {
			v.IsSet()
			wg.Done()
		}
	}()

	// Writer
	go func() {
		for i := 0; i < repeat; i++ {
			v.UnSet()
			wg.Done()
		}
	}()
	wg.Wait()
}

func ExampleAtomicBool() {
	cond := New()    // default to false
	cond.Set()       // set to true
	cond.IsSet()     // returns true
	cond.UnSet()     // set to false
	cond.SetTo(true) // set to whatever you want
}

func TestZeroValueAndCASWinner(t *testing.T) {
	var value AtomicBool
	if value.IsSet() {
		t.Fatal("zero value must be false")
	}
	winners := make(chan bool, 32)
	for i := 0; i < 32; i++ {
		go func() { winners <- value.SetToIf(false, true) }()
	}
	count := 0
	for i := 0; i < 32; i++ {
		if <-winners {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("CAS winners: %d", count)
	}
	if !value.SetToIf(true, true) || value.SetToIf(false, false) {
		t.Fatal("same-value CAS mismatch")
	}
}
