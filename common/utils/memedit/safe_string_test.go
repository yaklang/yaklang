package memedit

import "testing"

func TestSafeStringLenCachesRuneCount(t *testing.T) {
	s := NewSafeString("你好abc")
	if got := s.Len(); got != 5 {
		t.Fatalf("Len() = %d, want 5", got)
	}
	if s.runeLen != 5 {
		t.Fatalf("runeLen cache = %d, want 5", s.runeLen)
	}

	// Repeated calls should reuse the cached rune count and stay stable.
	if got := s.Len(); got != 5 {
		t.Fatalf("Len() after cache = %d, want 5", got)
	}
	if got := s.Slice(0, 2); got != "你好" {
		t.Fatalf("Slice() = %q, want %q", got, "你好")
	}
	if s.runeLen != 5 {
		t.Fatalf("runeLen cache after Slice = %d, want 5", s.runeLen)
	}

	sub := s.SafeSlice(1, 4)
	if got := sub.Len(); got != 3 {
		t.Fatalf("SafeSlice().Len() = %d, want 3", got)
	}
	if got := sub.Slice(0, 2); got != "好a" {
		t.Fatalf("SafeSlice().Slice() = %q, want %q", got, "好a")
	}
	if sub.runeLen != 3 {
		t.Fatalf("SafeSlice() runeLen cache = %d, want 3", sub.runeLen)
	}
}

func TestSafeStringASCIIPathAvoidsRuneMaterialization(t *testing.T) {
	s := NewSafeString("hello world")
	if got := s.Len(); got != 11 {
		t.Fatalf("Len() = %d, want 11", got)
	}
	if !s.isASCII() {
		t.Fatalf("isASCII() = false, want true")
	}
	if got := s.Slice(0, 5); got != "hello" {
		t.Fatalf("Slice() = %q, want %q", got, "hello")
	}
	if got := s.SliceBeforeStart(5); got != "hello" {
		t.Fatalf("SliceBeforeStart() = %q, want %q", got, "hello")
	}
	sub := s.SafeSlice(6, 11)
	if got := sub.Slice(0, sub.Len()); got != "world" {
		t.Fatalf("SafeSlice().Slice() = %q, want %q", got, "world")
	}
	if s.runes != nil {
		t.Fatalf("ASCII fast path should not materialize rune slice")
	}
	if sub.runes != nil {
		t.Fatalf("ASCII SafeSlice should not materialize rune slice")
	}
}
