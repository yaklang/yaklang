package memedit

import "testing"

func TestByteOffsetToRuneIndexSkipsTableForASCII(t *testing.T) {
	ed := NewMemEditor("alpha\nbeta")
	got, ok := ed.ByteOffsetToRuneIndex(6)
	if !ok || got != 6 {
		t.Fatalf("ascii offset: got %d ok=%v", got, ok)
	}
	if ed.runeOffsetMap.Load() != nil {
		t.Fatal("ascii conversion built a rune offset map")
	}
}

func TestByteOffsetToRuneIndexMultibyte(t *testing.T) {
	// a \n 你 : rune indexes 0, 1, 2. 你 starts at byte 2.
	ed := NewMemEditor("a\n你")
	cases := []struct {
		byteOff int
		want    int
	}{
		{0, 0},
		{1, 1},
		{2, 2},
		{5, 3}, // exclusive end
	}
	for _, tc := range cases {
		got, ok := ed.ByteOffsetToRuneIndex(tc.byteOff)
		if !ok || got != tc.want {
			t.Fatalf("byte %d: got %d ok=%v want %d", tc.byteOff, got, ok, tc.want)
		}
	}
	if ed.runeOffsetMap.Load() != nil {
		t.Fatal("line index conversion built a rune offset map")
	}
}
