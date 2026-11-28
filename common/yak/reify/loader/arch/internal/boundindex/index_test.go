package boundindex

import "testing"

func TestIndexNextAndFlush(t *testing.T) {
	var idx Index
	for _, addr := range []uint64{0x400, 0x100, 0x300, 0x200} {
		idx.Add(addr)
	}
	if got := idx.Next(0x50); got != 0x100 {
		t.Fatalf("Next(0x50) = 0x%x, want 0x100", got)
	}
	if got := idx.Next(0x100); got != 0x200 {
		t.Fatalf("Next(0x100) = 0x%x, want 0x200", got)
	}
	if got := idx.Next(0x400); got != 0 {
		t.Fatalf("Next(0x400) = 0x%x, want 0", got)
	}

	// Exceed flushThreshold to exercise automatic sort+compact and mixed pending.
	for i := uint64(0); i < 100; i++ {
		idx.Add(0x1000 + i*0x10)
	}
	idx.Add(0x150)
	if got := idx.Next(0x100); got != 0x150 {
		t.Fatalf("Next(0x100) with pending = 0x%x, want 0x150", got)
	}
	if got := idx.Next(0x1000); got != 0x1010 {
		t.Fatalf("Next(0x1000) = 0x%x, want 0x1010", got)
	}
	sorted := idx.Sorted()
	if len(sorted) != 105 || sorted[0] != 0x100 || sorted[len(sorted)-1] != 0x1000+99*0x10 {
		t.Fatalf("unexpected sorted len=%d", len(sorted))
	}
}
