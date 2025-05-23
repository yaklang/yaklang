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
