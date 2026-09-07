package yakvm

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func valueNativeLiteralTarget() {}

func TestValueDerivedDataIsConcurrentSafe(t *testing.T) {
	const workers = 32
	nativeLiteral := runtime.FuncForPC(reflect.ValueOf(valueNativeLiteralTarget).Pointer()).Name()
	waitGroup := new(sync.WaitGroup)
	mutex := new(sync.Mutex)
	values := []struct {
		value   *Value
		literal string
	}{
		{value: NewBoolValue(true), literal: "true"},
		{value: NewIntValue(42), literal: "42"},
		{value: NewAutoValue(float64(3.5)), literal: "3.5000"},
		{value: NewStringValue("你好yak"), literal: `"你好yak"`},
		{value: NewAutoValue(waitGroup), literal: fmt.Sprint(waitGroup)},
		{value: NewAutoValue(mutex), literal: fmt.Sprint(mutex)},
		{value: NewAutoValue(valueNativeLiteralTarget), literal: nativeLiteral},
	}
	stringValue := values[3].value
	frames := make([]*Frame, workers)
	for i := range frames {
		frames[i] = &Frame{}
	}

	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(frame *Frame) {
			defer wg.Done()
			<-start
			for iteration := 0; iteration < 100; iteration++ {
				for _, testCase := range values {
					if got := testCase.value.GetLiteral(); got != testCase.literal {
						errs <- fmt.Errorf("GetLiteral() = %q, want %q", got, testCase.literal)
						return
					}
				}
				if got := string(frame.cacheRunes(stringValue)); got != "你好yak" {
					errs <- fmt.Errorf("cacheRunes() = %q, want %q", got, "你好yak")
					return
				}
			}
		}(frames[worker])
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	for _, testCase := range values {
		if testCase.value.Literal != "" {
			t.Fatalf("GetLiteral mutated shared Literal to %q", testCase.value.Literal)
		}
	}
}

func assertRuneCacheReleased(t *testing.T, frame *Frame) {
	t.Helper()
	if frame.runeCache != nil || frame.runeCacheBytes != 0 {
		t.Fatalf("rune cache retained %d entries / %d bytes", len(frame.runeCache), frame.runeCacheBytes)
	}
}

func primeRuneCache(t *testing.T, frame *Frame) {
	t.Helper()
	value := NewStringValue("你好 yakvm")
	if got := string(frame.cacheRunes(value)); got != "你好 yakvm" {
		t.Fatalf("cacheRunes() = %q", got)
	}
	if len(frame.runeCache) != 1 || frame.runeCacheBytes == 0 {
		t.Fatalf("rune cache was not populated: entries=%d bytes=%d", len(frame.runeCache), frame.runeCacheBytes)
	}
}

func TestFrameRuneCacheIsBoundedAndReleased(t *testing.T) {
	frame := NewFrame(New())
	for i := 0; i < 512; i++ {
		raw := fmt.Sprintf("%04d-%s", i, strings.Repeat("界", 8<<10))
		value := NewStringValue(raw)
		if got := string(frame.cacheRunes(value)); got != raw {
			t.Fatalf("cacheRunes iteration %d changed the string", i)
		}
		if entries := len(frame.runeCache); entries > frameRuneCacheMaxEntries {
			t.Fatalf("rune cache grew to %d entries, max %d", entries, frameRuneCacheMaxEntries)
		}
		if frame.runeCacheBytes > frameRuneCacheMaxBytes {
			t.Fatalf("rune cache retained %d bytes, max %d", frame.runeCacheBytes, frameRuneCacheMaxBytes)
		}
	}

	entries, retainedBytes := len(frame.runeCache), frame.runeCacheBytes
	large := strings.Repeat("界", frameRuneCacheMaxBytes)
	if got := string(frame.cacheRunes(NewStringValue(large))); got != large {
		t.Fatal("oversized uncached conversion changed the string")
	}
	if len(frame.runeCache) != entries || frame.runeCacheBytes != retainedBytes {
		t.Fatalf("oversized value entered cache: before=%d/%d after=%d/%d",
			entries, retainedBytes, len(frame.runeCache), frame.runeCacheBytes)
	}

	t.Run("normal", func(t *testing.T) {
		frame := NewFrame(New())
		primeRuneCache(t, frame)
		frame.Exec(nil)
		assertRuneCacheReleased(t, frame)
	})

	t.Run("canceled", func(t *testing.T) {
		frame := NewFrame(New())
		primeRuneCache(t, frame)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		frame.ctx = ctx
		frame.Exec([]*Code{{Opcode: OpPush, Op1: NewIntValue(1)}})
		assertRuneCacheReleased(t, frame)
	})

	t.Run("panic", func(t *testing.T) {
		vm := New()
		vm.GetConfig().SetStopRecover(true)
		frame := NewFrame(vm)
		frame.ctx = context.Background()
		primeRuneCache(t, frame)
		gotPanic := false
		func() {
			defer func() {
				if recover() != nil {
					gotPanic = true
				}
			}()
			frame.Exec([]*Code{
				{Opcode: OpPush, Op1: NewIntValue(1)},
				{Opcode: OpExit, Op1: NewStringValue("cache release panic")},
			})
		}()
		if !gotPanic {
			t.Fatal("expected VM panic")
		}
		assertRuneCacheReleased(t, frame)
	})
}

func TestRetainedTraceFrameDoesNotKeepRuneCacheBetweenInlineRuns(t *testing.T) {
	vm := New()
	ctx := context.Background()
	var retained *Frame
	if err := vm.Exec(ctx, func(frame *Frame) {
		retained = frame
		primeRuneCache(t, frame)
		frame.Exec(nil)
	}, Trace); err != nil {
		t.Fatal(err)
	}
	assertRuneCacheReleased(t, retained)

	for i := 0; i < 100; i++ {
		if err := vm.Exec(ctx, func(frame *Frame) {
			if frame != retained {
				t.Fatalf("inline run %d changed retained frame", i)
			}
			value := NewStringValue(fmt.Sprintf("inline-%d-%s", i, strings.Repeat("界", 1024)))
			if runes := frame.cacheRunes(value); len(runes) == 0 {
				t.Fatalf("inline run %d did not convert string", i)
			}
			frame.Exec(nil)
			assertRuneCacheReleased(t, frame)
		}, Inline); err != nil {
			t.Fatalf("inline run %d failed: %v", i, err)
		}
	}

	if popped := vm.popCurrentFrame(retained); popped != retained {
		t.Fatalf("failed to pop retained trace frame: %#v", popped)
	}
}
