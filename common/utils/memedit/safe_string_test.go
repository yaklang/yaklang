package memedit

import "testing"

func TestSafeStringLenCachesRuneCount(t *testing.T) {
	s := NewSafeString("你好abc")
	if got := s.Len(); got != 5 {
		t.Fatalf("Len() = %d, want 5", got)
	}
	if got := s.runeLen.Load(); got != 5 {
		t.Fatalf("runeLen cache = %d, want 5", got)
	}

	// Repeated calls should reuse the cached rune count and stay stable.
	if got := s.Len(); got != 5 {
		t.Fatalf("Len() after cache = %d, want 5", got)
	}
	if got := s.Slice(0, 2); got != "你好" {
		t.Fatalf("Slice() = %q, want %q", got, "你好")
	}
	if got := s.runeLen.Load(); got != 5 {
		t.Fatalf("runeLen cache after Slice = %d, want 5", got)
	}

	sub := s.SafeSlice(1, 4)
	if got := sub.Len(); got != 3 {
		t.Fatalf("SafeSlice().Len() = %d, want 3", got)
	}
	if got := sub.Slice(0, 2); got != "好a" {
		t.Fatalf("SafeSlice().Slice() = %q, want %q", got, "好a")
	}
	if got := sub.runeLen.Load(); got != 3 {
		t.Fatalf("SafeSlice() runeLen cache = %d, want 3", got)
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
	if s.runes.Load() != nil {
		t.Fatalf("ASCII fast path should not materialize rune slice")
	}
	if sub.runes.Load() != nil {
		t.Fatalf("ASCII SafeSlice should not materialize rune slice")
	}
}

func TestSafeString_StringMemoizedAndCorrect(t *testing.T) {
	s := NewSafeString("hello 你好 world")
	first := s.String()
	second := s.String()
	if first != "hello 你好 world" {
		t.Fatalf("String() = %q", first)
	}
	if first != second {
		t.Fatalf("String() not stable across calls")
	}
	if s.strCache.Load() == nil {
		t.Fatalf("String() should have populated the memo cache")
	}
	if cached := s.strCache.Load(); cached != nil && *cached != first {
		t.Fatalf("memoized cache = %q, want %q", *cached, first)
	}

	// SafeSlice produces a distinct SafeString with its own content.
	// "hello 你好 world": rune 6..11 = "你好 wo"
	sub := s.SafeSlice(6, 12)
	if got := sub.String(); got != "你好 wor" {
		t.Fatalf("SafeSlice().String() = %q, want %q", got, "你好 wor")
	}
}
