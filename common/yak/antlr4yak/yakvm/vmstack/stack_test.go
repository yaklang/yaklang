package vmstack

import "testing"

func TestPeekNBoundsAndShadowRestore(t *testing.T) {
	s := New()
	s.Push(1)
	s.Push(2)
	for n, want := range map[int]any{-1: nil, 0: 2, 1: 1, 2: nil, 3: nil} {
		if got := s.PeekN(n); got != want {
			t.Fatalf("PeekN(%d)=%v, want %v", n, got, want)
		}
	}
	restore := s.CreateShadowStack()
	s.Pop()
	s.Pop()
	s.Push(3)
	restore()
	if s.Pop() != 2 || s.Pop() != 1 {
		t.Fatal("shadow did not restore popped values")
	}
}
