package memedit

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
)

// 测试基本的插入功能
func TestInsertAtPosition_Basic(t *testing.T) {
	tests := []struct {
		name       string
		initial    string
		line       int
		column     int
		text       string
		expected   string
		shouldFail bool
	}{
		{
			name:     "insert at beginning",
			initial:  "Hello\nWorld",
			line:     1,
			column:   1,
			text:     ">>> ",
			expected: ">>> Hello\nWorld",
		},
		{
			name:     "insert in middle of line",
			initial:  "Hello\nWorld",
			line:     1,
			column:   3,
			text:     "XXX",
			expected: "HeXXXllo\nWorld",
		},
		{
			name:     "insert at end of line",
			initial:  "Hello\nWorld",
			line:     1,
			column:   6,
			text:     " there",
			expected: "Hello there\nWorld",
		},
		{
			name:     "insert at beginning of second line",
			initial:  "Hello\nWorld",
			line:     2,
			column:   1,
			text:     ">>> ",
			expected: "Hello\n>>> World",
		},
		{
			name:       "invalid position - zero line",
			initial:    "Hello\nWorld",
			line:       0,
			column:     1,
			text:       "test",
			shouldFail: true,
		},
		{
			name:       "invalid position - zero column",
			initial:    "Hello\nWorld",
			line:       1,
			column:     0,
			text:       "test",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor(tt.initial)
			pos := NewPosition(tt.line, tt.column)
			err := editor.InsertAtPosition(pos, tt.text)

			if tt.shouldFail {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, editor.GetSourceCode())
		})
	}
}

// 测试 UTF-8 字符的插入
func TestInsertAtPosition_UTF8(t *testing.T) {
	tests := []struct {
		name     string
		initial  string
		line     int
		column   int
		text     string
		expected string
	}{
		{
			name:     "insert chinese characters",
			initial:  "你好\n世界",
			line:     1,
			column:   2,
			text:     "很",
			expected: "你很好\n世界",
		},
		{
			name:     "insert emoji",
			initial:  "Hello 😀\nWorld 🌍",
			line:     1,
			column:   7,
			text:     "😊",
			expected: "Hello 😊😀\nWorld 🌍",
		},
		{
			name:     "insert multi-byte at end",
			initial:  "Test\nLine",
			line:     1,
			column:   5,
			text:     " 测试",
			expected: "Test 测试\nLine",
		},
		{
			name:     "insert at unicode boundary",
			initial:  "🚀🎯🌟",
			line:     1,
			column:   2,
			text:     "⭐",
			expected: "🚀⭐🎯🌟",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor(tt.initial)
			pos := NewPosition(tt.line, tt.column)
			err := editor.InsertAtPosition(pos, tt.text)

			assert.NoError(t, err)
			result := editor.GetSourceCode()
			assert.Equal(t, tt.expected, result)

			// 验证 UTF-8 有效性
			assert.True(t, utf8.ValidString(result))
		})
	}
}

// 测试行插入功能
func TestInsertAtLine_Basic(t *testing.T) {
	tests := []struct {
		name       string
		initial    string
		line       int
		text       string
		expected   string
		shouldFail bool
	}{
		{
			name:     "insert at first line",
			initial:  "Hello\nWorld",
			line:     1,
			text:     ">>> ",
			expected: ">>> Hello\nWorld",
		},
		{
			name:     "insert at second line",
			initial:  "Hello\nWorld",
			line:     2,
			text:     ">>> ",
			expected: "Hello\n>>> World",
		},
		{
			name:     "insert at line beyond range - should extend",
			initial:  "Hello\nWorld",
			line:     5,
			text:     "New line",
			expected: "Hello\nWorld\n\n\nNew line",
		},
		{
			name:     "insert in empty file",
			initial:  "",
			line:     1,
			text:     "First line",
			expected: "First line",
		},
		{
			name:       "invalid line number",
			initial:    "Hello\nWorld",
			line:       0,
			text:       "test",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor(tt.initial)
			err := editor.InsertAtLine(tt.line, tt.text)

			if tt.shouldFail {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, editor.GetSourceCode())
		})
	}
}

// 测试行替换功能
func TestReplaceLine_Basic(t *testing.T) {
	tests := []struct {
		name       string
		initial    string
		line       int
		text       string
		expected   string
		shouldFail bool
	}{
		{
			name:     "replace first line",
			initial:  "Hello\nWorld\nTest",
			line:     1,
			text:     "Goodbye",
			expected: "Goodbye\nWorld\nTest",
		},
		{
			name:     "replace middle line",
			initial:  "Hello\nWorld\nTest",
			line:     2,
			text:     "Universe",
			expected: "Hello\nUniverse\nTest",
		},
		{
			name:     "replace last line",
			initial:  "Hello\nWorld\nTest",
			line:     3,
			text:     "Final",
			expected: "Hello\nWorld\nFinal",
		},
		{
			name:     "replace with empty string",
			initial:  "Hello\nWorld\nTest",
			line:     2,
			text:     "",
			expected: "Hello\n\nTest",
		},
		{
			name:       "replace line out of range",
			initial:    "Hello\nWorld",
			line:       5,
			text:       "test",
			shouldFail: true,
		},
		{
			name:       "invalid line number",
			initial:    "Hello\nWorld",
			line:       0,
			text:       "test",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor(tt.initial)
			err := editor.ReplaceLine(tt.line, tt.text)

			if tt.shouldFail {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, editor.GetSourceCode())
		})
	}
}

// 测试行范围替换功能
func TestReplaceLineRange_Basic(t *testing.T) {
	tests := []struct {
		name       string
		initial    string
		startLine  int
		endLine    int
		text       string
		expected   string
		shouldFail bool
	}{
		{
			name:      "replace single line range",
			initial:   "Hello\nWorld\nTest\nFinal",
			startLine: 2,
			endLine:   2,
			text:      "Universe",
			expected:  "Hello\nUniverse\nTest\nFinal",
		},
		{
			name:      "replace multiple lines",
			initial:   "Hello\nWorld\nTest\nFinal",
			startLine: 2,
			endLine:   3,
			text:      "New content",
			expected:  "Hello\nNew content\nFinal",
		},
		{
			name:      "replace all lines",
			initial:   "Hello\nWorld\nTest",
			startLine: 1,
			endLine:   3,
			text:      "Everything new",
			expected:  "Everything new",
		},
		{
			name:      "replace with multiline text",
			initial:   "Hello\nWorld\nTest",
			startLine: 2,
			endLine:   2,
			text:      "Line1\nLine2\nLine3",
			expected:  "Hello\nLine1\nLine2\nLine3\nTest",
		},
		{
			name:       "invalid start line",
			initial:    "Hello\nWorld",
			startLine:  0,
			endLine:    1,
			text:       "test",
			shouldFail: true,
		},
		{
			name:       "start line > end line",
			initial:    "Hello\nWorld",
			startLine:  2,
			endLine:    1,
			text:       "test",
			shouldFail: true,
		},
		{
			name:       "line out of range",
			initial:    "Hello\nWorld",
			startLine:  1,
			endLine:    5,
			text:       "test",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor(tt.initial)
			err := editor.ReplaceLineRange(tt.startLine, tt.endLine, tt.text)

			if tt.shouldFail {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			result := editor.GetSourceCode()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// 测试删除行功能
func TestDeleteLine_Basic(t *testing.T) {
	tests := []struct {
		name       string
		initial    string
		line       int
		expected   string
		shouldFail bool
	}{
		{
			name:     "delete first line",
			initial:  "Hello\nWorld\nTest",
			line:     1,
			expected: "World\nTest",
		},
		{
			name:     "delete middle line",
			initial:  "Hello\nWorld\nTest",
			line:     2,
			expected: "Hello\nTest",
		},
		{
			name:     "delete last line",
			initial:  "Hello\nWorld\nTest",
			line:     3,
			expected: "Hello\nWorld",
		},
		{
			name:     "delete only line",
			initial:  "OnlyLine",
			line:     1,
			expected: "",
		},
		{
			name:     "delete from two line file - first",
			initial:  "First\nSecond",
			line:     1,
			expected: "Second",
		},
		{
			name:     "delete from two line file - second",
			initial:  "First\nSecond",
			line:     2,
			expected: "First",
		},
		{
			name:       "delete line out of range",
			initial:    "Hello\nWorld",
			line:       5,
			shouldFail: true,
		},
		{
			name:       "invalid line number",
			initial:    "Hello\nWorld",
			line:       0,
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor(tt.initial)
			err := editor.DeleteLine(tt.line)

			if tt.shouldFail {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, editor.GetSourceCode())
		})
	}
}

// 测试删除行范围功能
func TestDeleteLineRange_Basic(t *testing.T) {
	tests := []struct {
		name       string
		initial    string
		startLine  int
		endLine    int
		expected   string
		shouldFail bool
	}{
		{
			name:      "delete single line range",
			initial:   "Hello\nWorld\nTest\nFinal",
			startLine: 2,
			endLine:   2,
			expected:  "Hello\nTest\nFinal",
		},
		{
			name:      "delete multiple lines",
			initial:   "Hello\nWorld\nTest\nFinal",
			startLine: 2,
			endLine:   3,
			expected:  "Hello\nFinal",
		},
		{
			name:      "delete all lines",
			initial:   "Hello\nWorld\nTest",
			startLine: 1,
			endLine:   3,
			expected:  "",
		},
		{
			name:      "delete first two lines",
			initial:   "Hello\nWorld\nTest\nFinal",
			startLine: 1,
			endLine:   2,
			expected:  "Test\nFinal",
		},
		{
			name:      "delete last two lines",
			initial:   "Hello\nWorld\nTest\nFinal",
			startLine: 3,
			endLine:   4,
			expected:  "Hello\nWorld",
		},
		{
			name:       "invalid start line",
			initial:    "Hello\nWorld",
			startLine:  0,
			endLine:    1,
			shouldFail: true,
		},
		{
			name:       "start line > end line",
			initial:    "Hello\nWorld",
			startLine:  2,
			endLine:    1,
			shouldFail: true,
		},
		{
			name:       "line out of range",
			initial:    "Hello\nWorld",
			startLine:  1,
			endLine:    5,
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor(tt.initial)
			err := editor.DeleteLineRange(tt.startLine, tt.endLine)

			if tt.shouldFail {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			result := editor.GetSourceCode()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// 测试添加和插入行功能
func TestAppendPrependLine(t *testing.T) {
	tests := []struct {
		name     string
		initial  string
		text     string
		method   string // "append" or "prepend"
		expected string
	}{
		{
			name:     "append to non-empty file",
			initial:  "Hello\nWorld",
			text:     "New line",
			method:   "append",
			expected: "Hello\nWorld\nNew line",
		},
		{
			name:     "append to file ending with newline",
			initial:  "Hello\nWorld\n",
			text:     "New line",
			method:   "append",
			expected: "Hello\nWorld\nNew line",
		},
		{
			name:     "append to empty file",
			initial:  "",
			text:     "First line",
			method:   "append",
			expected: "First line",
		},
		{
			name:     "prepend to non-empty file",
			initial:  "Hello\nWorld",
			text:     "First line",
			method:   "prepend",
			expected: "First line\nHello\nWorld",
		},
		{
			name:     "prepend to empty file",
			initial:  "",
			text:     "Only line",
			method:   "prepend",
			expected: "Only line\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor(tt.initial)

			var err error
			if tt.method == "append" {
				err = editor.AppendLine(tt.text)
			} else {
				err = editor.PrependLine(tt.text)
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, editor.GetSourceCode())
		})
	}
}

// 测试复杂的 UTF-8 场景
func TestComplexUTF8Scenarios(t *testing.T) {
	tests := []struct {
		name        string
		initial     string
		operations  []func(*MemEditor) error
		expected    string
		description string
	}{
		{
			name:    "mixed utf8 insert and replace",
			initial: "Hello 世界\n你好 World\n🌍 Earth",
			operations: []func(*MemEditor) error{
				func(e *MemEditor) error {
					return e.InsertAtPosition(NewPosition(1, 7), "美丽的")
				},
				func(e *MemEditor) error {
					return e.ReplaceLine(2, "大家好 Universe")
				},
				func(e *MemEditor) error {
					return e.InsertAtPosition(NewPosition(3, 1), "🚀")
				},
			},
			expected:    "Hello 美丽的世界\n大家好 Universe\n🚀🌍 Earth",
			description: "Mixed UTF-8 operations",
		},
		{
			name:    "emoji manipulation",
			initial: "😀😃😄\n😁😆😅\n😂🤣😭",
			operations: []func(*MemEditor) error{
				func(e *MemEditor) error {
					return e.InsertAtPosition(NewPosition(1, 2), "👍")
				},
				func(e *MemEditor) error {
					return e.DeleteLine(2)
				},
				func(e *MemEditor) error {
					return e.AppendLine("🎉🎊🎈")
				},
			},
			expected:    "😀👍😃😄\n😂🤣😭\n🎉🎊🎈",
			description: "Emoji character manipulation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor(tt.initial)

			for i, op := range tt.operations {
				err := op(editor)
				assert.NoError(t, err, "Operation %d failed", i)
			}

			result := editor.GetSourceCode()
			assert.Equal(t, tt.expected, result)
			assert.True(t, utf8.ValidString(result), "Result should be valid UTF-8")

			// 验证行数映射正确性
			lines := strings.Split(result, "\n")
			assert.Equal(t, len(lines), editor.GetLineCount(), "Line count should match")
		})
	}
}

// 测试边界条件
func TestEdgeCases(t *testing.T) {
	t.Run("empty file operations", func(t *testing.T) {
		editor := NewMemEditor("")

		// 测试在空文件中插入
		err := editor.InsertAtPosition(NewPosition(1, 1), "test")
		assert.NoError(t, err)
		assert.Equal(t, "test", editor.GetSourceCode())

		// 测试删除唯一行
		err = editor.DeleteLine(1)
		assert.NoError(t, err)
		assert.Equal(t, "", editor.GetSourceCode())
	})

	t.Run("single character file", func(t *testing.T) {
		editor := NewMemEditor("a")

		// 插入在字符前
		err := editor.InsertAtPosition(NewPosition(1, 1), "X")
		assert.NoError(t, err)
		assert.Equal(t, "Xa", editor.GetSourceCode())

		// 重置并插入在字符后
		editor = NewMemEditor("a")
		err = editor.InsertAtPosition(NewPosition(1, 2), "X")
		assert.NoError(t, err)
		assert.Equal(t, "aX", editor.GetSourceCode())
	})

	t.Run("file with only newlines", func(t *testing.T) {
		editor := NewMemEditor("\n\n\n")

		// 在第二个空行插入
		err := editor.InsertAtLine(2, "content")
		assert.NoError(t, err)
		assert.Equal(t, "\ncontent\n\n", editor.GetSourceCode())

		// 删除第一行
		err = editor.DeleteLine(1)
		assert.NoError(t, err)
		assert.Equal(t, "content\n\n", editor.GetSourceCode())
	})

	t.Run("very long lines", func(t *testing.T) {
		longLine := strings.Repeat("a", 10000)
		editor := NewMemEditor(longLine + "\nshort")

		// 在长行中间插入
		err := editor.InsertAtPosition(NewPosition(1, 5000), "INSERT")
		assert.NoError(t, err)

		result := editor.GetSourceCode()
		assert.Contains(t, result, "INSERT")
		assert.True(t, len(result) > 10000)
	})
}

// 测试操作的一致性（确保操作后数据结构保持一致）
func TestOperationConsistency(t *testing.T) {
	tests := []struct {
		name       string
		initial    string
		operations []func(*MemEditor) error
	}{
		{
			name:    "multiple inserts",
			initial: "Line1\nLine2\nLine3",
			operations: []func(*MemEditor) error{
				func(e *MemEditor) error { return e.InsertAtLine(2, "New: ") },
				func(e *MemEditor) error { return e.InsertAtPosition(NewPosition(1, 1), "Start: ") },
				func(e *MemEditor) error { return e.AppendLine("End line") },
			},
		},
		{
			name:    "mixed operations",
			initial: "A\nB\nC\nD\nE",
			operations: []func(*MemEditor) error{
				func(e *MemEditor) error { return e.DeleteLine(3) },                      // 删除C，剩下A\nB\nD\nE
				func(e *MemEditor) error { return e.ReplaceLine(2, "NewB") },             // 替换B为NewB
				func(e *MemEditor) error { return e.InsertAtLine(1, "Prefix: ") },        // 在第一行插入
				func(e *MemEditor) error { return e.ReplaceLineRange(3, 4, "Combined") }, // 替换D和E为Combined
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor(tt.initial)

			for i, op := range tt.operations {
				err := op(editor)
				assert.NoError(t, err, "Operation %d failed", i)

				// 验证数据结构一致性
				validateEditorConsistency(t, editor)
			}
		})
	}
}

// 验证编辑器数据结构的一致性
func validateEditorConsistency(t *testing.T, editor *MemEditor) {
	sourceCode := editor.GetSourceCode()
	lines := strings.Split(sourceCode, "\n")

	// 验证行数一致
	assert.Equal(t, len(lines), editor.GetLineCount(), "Line count should match actual lines")

	// 验证每行的内容和长度
	for i := 1; i <= editor.GetLineCount(); i++ {
		line, err := editor.GetLine(i)
		assert.NoError(t, err, "Should be able to get line %d", i)

		expectedLine := lines[i-1]
		assert.Equal(t, expectedLine, line, "Line %d content should match", i)

		// 验证位置计算
		startOffset, err := editor.GetStartOffsetByLine(i)
		assert.NoError(t, err, "Should be able to get start offset for line %d", i)

		endOffset, err := editor.GetEndOffsetByLine(i)
		assert.NoError(t, err, "Should be able to get end offset for line %d", i)

		// 验证偏移量计算的文本与 GetLine 结果一致
		offsetText := editor.GetTextFromOffset(startOffset, endOffset)
		assert.Equal(t, line, offsetText, "Offset-based text should match GetLine for line %d", i)
	}

	// 验证 UTF-8 有效性
	assert.True(t, utf8.ValidString(sourceCode), "Source code should be valid UTF-8")
}

// 性能测试 - 确保编辑操作在合理时间内完成
func TestEditPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	// 创建一个较大的文件进行测试
	lines := make([]string, 1000)
	for i := range lines {
		lines[i] = fmt.Sprintf("Line %d with some content", i+1)
	}
	initial := strings.Join(lines, "\n")

	editor := NewMemEditor(initial)

	// 执行多次编辑操作
	operations := []func(){
		func() { editor.InsertAtLine(500, "Inserted line") },
		func() { editor.ReplaceLine(600, "Replaced line content") },
		func() { editor.DeleteLine(700) },
		func() { editor.ReplaceLineRange(800, 805, "Multi-line replacement") },
		func() { editor.AppendLine("New end line") },
		func() { editor.PrependLine("New start line") },
	}

	// 这个测试主要确保操作能在合理时间内完成，不会有性能回归
	for i, op := range operations {
		t.Run(fmt.Sprintf("operation_%d", i), func(t *testing.T) {
			op()
			// 验证编辑器仍然一致
			validateEditorConsistency(t, editor)
		})
	}
}

// 测试 nil 和空值处理
func TestNilAndEmptyHandling(t *testing.T) {
	editor := NewMemEditor("test")

	// 测试 nil position
	err := editor.InsertAtPosition(nil, "text")
	assert.Error(t, err)

	// 测试空文本插入（应该成功但不改变内容）
	original := editor.GetSourceCode()
	err = editor.InsertAtPosition(NewPosition(1, 1), "")
	assert.NoError(t, err)
	assert.Equal(t, original, editor.GetSourceCode())
}

// 确保向后兼容性的测试
func TestBackwardCompatibility(t *testing.T) {
	t.Run("existing functionality unchanged", func(t *testing.T) {
		// 这个测试确保现有的功能仍然正常工作
		editor := NewMemEditor("Hello\nWorld\nTest")

		// 测试现有的只读功能
		assert.Equal(t, 3, editor.GetLineCount())

		line, err := editor.GetLine(2)
		assert.NoError(t, err)
		assert.Equal(t, "World", line)

		selected, err := editor.Select(0, 5)
		assert.NoError(t, err)
		assert.Equal(t, "Hello", selected)

		pos := editor.GetPositionByOffset(6)
		assert.Equal(t, 2, pos.GetLine())
		assert.Equal(t, 1, pos.GetColumn())

		offset := editor.GetOffsetByPositionRaw(2, 1)
		assert.Equal(t, 6, offset)

		// 在使用新的编辑功能后，确保只读功能仍然正常
		err = editor.InsertAtLine(2, ">>> ")
		assert.NoError(t, err)

		// 重新验证只读功能
		line, err = editor.GetLine(2)
		assert.NoError(t, err)
		assert.Equal(t, ">>> World", line)

		assert.Equal(t, 3, editor.GetLineCount())
	})
}

func init() {
	// 设置日志级别，避免测试时的调试输出干扰
	log.SetLevel(log.ErrorLevel)
}
