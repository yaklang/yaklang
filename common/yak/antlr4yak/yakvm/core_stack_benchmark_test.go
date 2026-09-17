package yakvm

import "testing"

func BenchmarkCoreOperandStack(b *testing.B) {
	f := NewFrame(New())
	v := NewAutoValue(1)
	f.push(v)
	f.pop()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.push(v)
		f.pop()
	}
}
