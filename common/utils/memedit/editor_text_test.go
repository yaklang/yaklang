package memedit

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestGetTextFromOffsetComplexSamples(t *testing.T) {
	// 多行文本，包含空行
	multiLineSource := "Hello, world!\n\nThis is a test string.\nAnother line.\n"
	multiLineEditor := NewMemEditor(multiLineSource)

	// 单行文本，不包含换行符
	singleLineSource := "Hello, single line world without new lines!"
	singleLineEditor := NewMemEditor(singleLineSource)

	// 只有换行符的文本
	newLinesSource := "\n\n\n\n"
	newLinesEditor := NewMemEditor(newLinesSource)

	tests := []struct {
		name     string
		editor   *MemEditor
		offset1  int
		offset2  int
		expected string
	}{
		// 多行文本测试
		{"Span multiple lines", multiLineEditor, 14, 50, "\nThis is a test string.\nAnother line"},
		{"Span multiple lines Boundary", multiLineEditor, 14, 51, "\nThis is a test string.\nAnother line."},
		{"Span multiple lines Boundary1", multiLineEditor, 14, 52, "\nThis is a test string.\nAnother line.\n"},
		{"Span multiple lines Out of Boundary", multiLineEditor, 14, 54444, "\nThis is a test string.\nAnother line.\n"},
		{"Start at empty line", multiLineEditor, 13, 29, "\n\nThis is a test"},
		{"End at new line", multiLineEditor, 0, 13, "Hello, world!"},
		{"Entire text with empty lines", multiLineEditor, 0, len(multiLineSource), multiLineSource},

		// 单行文本测试 "Hello, (7)single line world withou(31)t new lines!"
		{"Middle of single line", singleLineEditor, 7, 24, "single line world"},
		{"Single line start to end", singleLineEditor, 0, len(singleLineSource), singleLineSource},
		{"Start of single line", singleLineEditor, 0, 5, "Hello"},
		{"End of single line", singleLineEditor, 31, 47, "t new lines!"},
		{"End of single line 2", singleLineEditor, 31, 11111, "t new lines!"},
		{"Middle of single line", singleLineEditor, 7, 24, "single line world"},
		{"Reverse indices", singleLineEditor, 24, 7, "single line world"},
		{"Single line start to end", singleLineEditor, 0, len(singleLineSource), singleLineSource},
		{"Single line out of upper bound", singleLineEditor, 14, 100, "line world without new lines!"},
		{"Single line negative start index", singleLineEditor, -10, 13, "Hello, single"},

		// 只有换行符的文本测试
		{"Only new lines middle", newLinesEditor, 1, 3, "\n\n"},
		{"Only new lines full", newLinesEditor, 0, 4, "\n\n\n\n"},
		// 只有换行符的文本测试 - 检查完全没有文字的情况
		{"Only new lines single", newLinesEditor, 2, 3, "\n"},
		{"Only new lines middle", newLinesEditor, 1, 3, "\n\n"},
		{"Only new lines full", newLinesEditor, 0, 4, "\n\n\n\n"},
		{"Only new lines out of bound", newLinesEditor, 0, 10, "\n\n\n\n"},
		{"Only new lines negative start", newLinesEditor, -1, 3, "\n\n\n"},
		{"Only new lines reverse indices", newLinesEditor, 3, 1, "\n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.editor.GetTextFromOffset(tt.offset1, tt.offset2)
			if result != tt.expected {
				t.Errorf("GetTextFromOffset(%d, %d) = %v, want %v", tt.offset1, tt.offset2, spew.Sdump(result), spew.Sdump(tt.expected))
			}
		})
	}
}

func TestGetTextFromOffset(t *testing.T) {
	sourceCode := "Hello, world! This is a test string."
	editor := NewMemEditor(sourceCode)

	tests := []struct {
		name     string
		offset1  int
		offset2  int
		expected string
	}{
		{"Normal case, correct order", 0, 5, "Hello"},
		{"Normal case, reverse order", 5, 0, "Hello"},
		{"End of string", 7, len(sourceCode), "world! This is a test string."},
		{"Start to end of string", 0, len(sourceCode), sourceCode},
		{"Negative start offset", -1, 5, "Hello"},
		{"Negative end offset", 0, -10, ""},
		{"Both offsets negative", -10, -2, ""},
		{"Offset beyond string length", 10, 1000, "ld! This is a test string."},
		{"Reverse order with valid indices", len(sourceCode), 7, "world! This is a test string."},
		{"Start equals end", 5, 5, ""},
		{"Offsets out of order beyond string length", 1000, 10, "ld! This is a test string."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := editor.GetTextFromOffset(tt.offset1, tt.offset2)
			if result != tt.expected {
				t.Errorf("GetTextFromOffset(%d, %d) = %v, want %v", tt.offset1, tt.offset2, result, tt.expected)
			}
		})
	}
}

func TestGetTextFromOffsetEdgeCases(t *testing.T) {
	// 空文本
	emptySource := ""
	emptyEditor := NewMemEditor(emptySource)

	// 只有一个换行符的文本
	oneNewLineSource := "\n"
	oneNewLineEditor := NewMemEditor(oneNewLineSource)

	tests := []struct {
		name     string
		editor   *MemEditor
		offset1  int
		offset2  int
		expected string
	}{
		// 空文本测试
		{"Empty text, any indices", emptyEditor, 0, 1, ""},
		{"Empty text, negative start", emptyEditor, -1, 1, ""},
		{"Empty text, out of bounds", emptyEditor, 0, 100, ""},
		{"Empty text, reverse indices", emptyEditor, 5, 0, ""},

		// 只有一个换行符的文本测试
		{"One new line, correct indices", oneNewLineEditor, 0, 1, "\n"},
		{"One new line, out of bounds", oneNewLineEditor, 0, 2, "\n"},
		{"One new line, negative start", oneNewLineEditor, -1, 1, "\n"},
		{"One new line, reverse indices", oneNewLineEditor, 1, 0, "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.editor.GetTextFromOffset(tt.offset1, tt.offset2)
			if result != tt.expected {
				t.Errorf("GetTextFromOffset(%d, %d) = %q, want %q", tt.offset1, tt.offset2, result, tt.expected)
			}
		})
	}
}
