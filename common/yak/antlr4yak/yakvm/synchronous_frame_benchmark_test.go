package yakvm

import (
	"context"
	"testing"
)

// One operation creates nested frames and performs native callback/current-frame
// lookup. The opt-in case assumes privately owned, entirely synchronous execution.
func BenchmarkSynchronousFrameLifecycle(b *testing.B) {
	for _, synchronous := range []bool{false, true} {
		name := "ordinary"
		if synchronous {
			name = "private-synchronous"
		}
		b.Run(name, func(b *testing.B) {
			vm := New()
			vm.GetConfig().SetSynchronousExecution(synchronous)
			ctx := context.Background()
			child := func(f *Frame) {
				if vm.CurrentFM() != f || f.nativeCallbackFrame() != f {
					b.Fatal("incorrect child frame")
				}
			}
			parent := func(f *Frame) {
				if vm.CurrentFM() != f || f.nativeCallbackFrame() != f {
					b.Fatal("incorrect parent frame")
				}
				if err := vm.Exec(ctx, child, Sub); err != nil {
					b.Fatal(err)
				}
				if vm.CurrentFM() != f {
					b.Fatal("parent was not restored")
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := vm.Exec(ctx, parent); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			if vm.CurrentFM() != nil {
				b.Fatal("retained frame")
			}
		})
	}
}
