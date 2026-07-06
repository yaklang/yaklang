package memedit_test

import (
	"github.com/yaklang/yaklang/common/utils/memedit"
	"testing"
	"unicode/utf8"
)

func TestRuneOffsetMap(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		runeTests []struct {
			runeIdx   int
			wantBytes int
			wantOk    bool
		}
		byteTests []struct {
			byteOffset int
			wantRune   int
			wantOk     bool
		}
	}{
		{
			name:  "empty string",
			input: "",
			runeTests: []struct {
				runeIdx   int
				wantBytes int
				wantOk    bool
			}{
				{0, 0, false},
				{-1, 0, false},
			},
			byteTests: []struct {
				byteOffset int
				wantRune   int
				wantOk     bool
			}{
				{0, 0, false},
				{5, 0, false},
			},
		},
		{
			name:  "ASCII only",
			input: "Hello",
			runeTests: []struct {
				runeIdx   int
				wantBytes int
				wantOk    bool
			}{
				{0, 0, true},
				{4, 4, true},
				{5, 0, false}, // 越界
				{-1, 0, false},
			},
			byteTests: []struct {
				byteOffset int
				wantRune   int
				wantOk     bool
			}{
				{0, 0, true},
				{3, 3, true},
				{5, 0, false}, // 越界
				{6, 0, false},
			},
		},
		{
			name:  "multi-byte characters",
			input: "世界café",
			runeTests: []struct {
				runeIdx   int
				wantBytes int
				wantOk    bool
			}{
				{0, 0, true},  // 世
				{1, 3, true},  // 界
				{2, 6, true},  // c
				{5, 9, true},  // é
				{6, 0, false}, // 越界
			},
			byteTests: []struct {
				byteOffset int
				wantRune   int
				wantOk     bool
			}{
				{0, 0, true},   // 世
				{2, 0, true},   // 世 (中间字节)
				{3, 1, true},   // 界
				{6, 2, true},   // c
				{10, 5, true},  // é
				{11, 0, false}, // 越界
				{-1, 0, false}, // 无效偏移
			},
		},
		{
			name:  "mixed characters with emoji",
			input: "Go🚀语言",
			runeTests: []struct {
				runeIdx   int
				wantBytes int
				wantOk    bool
			}{
				{0, 0, true}, // G
				{1, 1, true}, // 🚀 (占用4字节)
				{2, 2, true}, // 语
				{3, 6, true}, // 言
				{4, 9, true},
			},
			byteTests: []struct {
				byteOffset int
				wantRune   int
				wantOk     bool
			}{
				{0, 0, true},   // G
				{1, 1, true},   // o
				{2, 2, true},   // 🚀
				{5, 2, true},   // 🚀 的中间字节
				{6, 3, true},   // 语
				{12, 0, false}, // 越界
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rom := memedit.NewRuneOffsetMap(tt.input)

			// 验证 RuneCount
			if got := rom.RuneCount(); got != utf8.RuneCountInString(tt.input) {
				t.Errorf("RuneCount() = %v, want %v", got, utf8.RuneCountInString(tt.input))
			}

			// 验证 RuneIndexToByteOffset
			for _, rt := range tt.runeTests {
				gotBytes, gotOk := rom.RuneIndexToByteOffset(rt.runeIdx)
				if gotOk != rt.wantOk || gotBytes != rt.wantBytes {
					t.Errorf("RuneIndexToByteOffset(%d) = (%v, %v), want (%v, %v)",
						rt.runeIdx, gotBytes, gotOk, rt.wantBytes, rt.wantOk)
				}
			}

			// 验证 ByteOffsetToRuneIndex
			for _, bt := range tt.byteTests {
				gotRune, gotOk := rom.ByteOffsetToRuneIndex(bt.byteOffset)
				if gotOk != bt.wantOk || gotRune != bt.wantRune {
					t.Errorf("ByteOffsetToRuneIndex(%d) = (%v, %v), want (%v, %v)",
						bt.byteOffset, gotRune, gotOk, bt.wantRune, bt.wantOk)
				}
			}

			// 验证 String() 方法
			if rom.String() != tt.input {
				t.Errorf("String() = %q, want %q", rom.String(), tt.input)
			}
		})
	}
}

// TestMemEditor_GetRuneOffsetMap_Memoize asserts the rune-offset map is
// memoized on the editor (built once, reused) and invalidated when the source
// changes. Before memoization, FileFilter rebuilt NewRuneOffsetMap over the
// full source on every call — ~71GB/20% of alloc on javacms-core.
func TestMemEditor_GetRuneOffsetMap_Memoize(t *testing.T) {
	ed := memedit.NewMemEditor("Hello, 世界")

	// Two calls return the SAME map pointer (memoized, not rebuilt).
	first := ed.GetRuneOffsetMap()
	if first == nil {
		t.Fatal("GetRuneOffsetMap() returned nil")
	}
	second := ed.GetRuneOffsetMap()
	if first != second {
		t.Fatalf("GetRuneOffsetMap() rebuilt the map: first=%p second=%p (should be memoized)", first, second)
	}
	// Correctness on the memoized map (multi-byte): "Hello, 世界" — byte 7 = 世.
	if r, ok := first.ByteOffsetToRuneIndex(7); !ok || r != 7 {
		t.Errorf("ByteOffsetToRuneIndex(7) = (%v,%v), want (7,true)", r, ok)
	}

	// After a source edit (which invalidates), the map is rebuilt — new pointer.
	if err := ed.InsertAtPosition(memedit.NewPosition(1, 1), "X"); err != nil {
		t.Fatalf("InsertAtPosition: %v", err)
	}
	third := ed.GetRuneOffsetMap()
	if third == nil {
		t.Fatal("GetRuneOffsetMap() returned nil after source edit")
	}
	if third == first {
		t.Fatalf("GetRuneOffsetMap() returned stale map after source edit (should rebuild): first=%p third=%p", first, third)
	}
	// And the rebuilt map reflects the new source.
	if r, ok := third.ByteOffsetToRuneIndex(0); !ok || r != 0 {
		t.Errorf("ByteOffsetToRuneIndex(0) on rebuilt map = (%v,%v), want (0,true)", r, ok)
	}
}
