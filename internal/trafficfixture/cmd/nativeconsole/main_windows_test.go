package main

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestNativeHexOffsetColumnCompleteTokens(t *testing.T) {
	for _, digits := range []int{6, 8} {
		for _, shift := range []int{-4, 0, 2} {
			prefix := strings.Repeat(" ", 20+shift)
			line := utf16.Encode([]rune(prefix + strings.Repeat("0", digits) + "  00 ff 73 4d 42"))
			x, width := nativeHexOffsetColumn(line, 20)
			if x != 20+shift || width != digits {
				t.Fatalf("digits=%d shift=%d: x=%d width=%d", digits, shift, x, width)
			}
		}
	}
	for _, token := range []string{"00000", "0000000", "000000000", "0000000000", "000001", "00000001", "00000g", "00000000x"} {
		x, width := nativeHexOffsetColumn(utf16.Encode([]rune("  "+token+"  ff")), 2)
		if x != -1 || width != 0 {
			t.Fatalf("accepted partial/nonzero/invalid token %q at %d width%d", token, x, width)
		}
	}
	for _, line := range []string{"", " ", "000000", "00000000"} {
		if x, _ := nativeHexOffsetColumn(utf16.Encode([]rune(line)), 0); x != -1 {
			t.Fatalf("accepted unterminated %q", line)
		}
	}
}
