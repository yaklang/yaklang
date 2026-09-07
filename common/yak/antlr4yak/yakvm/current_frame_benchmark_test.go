package yakvm

import (
	"context"
	"testing"
)

var benchmarkFrameSink *Frame

func BenchmarkExecFrameLifecycle(b *testing.B) {
	vm := New()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := vm.Exec(ctx, func(frame *Frame) {
			benchmarkFrameSink = frame
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNestedFrameLifecycle(b *testing.B) {
	vm := New()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := vm.Exec(ctx, func(parent *Frame) {
			if err := vm.exec(ctx, parent, func(child *Frame) {
				benchmarkFrameSink = child
			}, Sub); err != nil {
				b.Fatal(err)
			}
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIdleGetVar(b *testing.B) {
	vm := New()
	vm.SetVars(map[string]any{"hotpatch": 1})
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		value, ok := vm.GetVar("hotpatch")
		if !ok || value != 1 {
			b.Fatalf("unexpected value: %v, %v", value, ok)
		}
	}
}
