package memedit

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
)

func TestNewMemEditor_SMOCKING(t *testing.T) {
	editor := NewMemEditor("Hello\nWorld\nThis is a test")
	if editor == nil {
		t.Fatal("Failed to create MemEditor instance")
	}
}

func TestGetLine(t *testing.T) {
	editor := NewMemEditor("Hello\nWorld\nThis is a test")

	line, err := editor.GetLine(2)
	if err != nil {
		t.Fatal("GetLine() failed:", err)
	}
	if line != "World" {
		t.Errorf("GetLine() got = %v, want %v", line, "World")
	}

	_, err = editor.GetLine(5)
	if err == nil {
		t.Error("GetLine() should fail for out of range line")
	}
}

func TestSelect(t *testing.T) {
	editor := NewMemEditor("Hello\nWorld\nThis is a test")

	selected, err := editor.Select(0, 5)
	if err != nil {
		t.Fatal("Select() failed:", err)
	}
	if selected != "Hello" {
		t.Errorf("Select() got = %v, want %v", selected, "Hello")
	}

	_, err = editor.Select(0, 100)
	if err == nil {
		t.Error("Select() should fail for out of range selection")
	}
}

func TestUpdateTextByRange(t *testing.T) {
	editor := NewMemEditor("Hello\nWorld\nThis is a test")
	startPos := NewPosition(1, 1)
	endPos := NewPosition(1, 6)
	err := editor.UpdateTextByRange(NewRange(startPos, endPos), "Hi")
	if err != nil {
		t.Fatal("UpdateTextByRange() failed:", err)
	}

	expected := "Hi\nWorld\nThis is a test"
	if editor.GetSourceCode() != expected {
		t.Errorf("UpdateTextByRange() got = %v, want %v", editor.GetSourceCode(), expected)
	}

	// Test out of bounds
	err = editor.UpdateTextByRange(NewRange(startPos, NewPosition(100, 100)), "Hi")
	if err == nil {
		t.Error("UpdateTextByRange() should fail for out of range range")
	}
}

func TestMoveCursorAndGetCurrectLine(t *testing.T) {
	editor := NewMemEditor("Hello\nWorld\nThis is a test")

	err := editor.MoveCursor(7) // Move to 'W' in "World"
	if err != nil {
		t.Fatal("MoveCursor() failed:", err)
	}

	line, err := editor.GetCurrentLine()
	if err != nil {
		t.Fatal("GetCurrentLine() failed:", err)
	}
	if line != "World" {
		t.Errorf("GetCurrentLine() got = %v, want %v", line, "World")
	}

	// Move cursor out of bounds
	err = editor.MoveCursor(100)
	if err == nil {
		t.Error("MoveCursor() should fail when moving out of bounds")
	}
}

func TestVirtualEditor_OffsetMode(t *testing.T) {
	source := "Hello\nWorld\nThis is a test"
	editor := NewMemEditor(source)

	// 测试 GetLine
	line, err := editor.GetLine(2)
	if err != nil || line != "World" {
		t.Errorf("Expected 'World', got '%s'", line)
	}

	// 测试 Select
	selected, err := editor.Select(6, 11)
	if err != nil || selected != "World" {
		t.Errorf("Expected 'World', got '%s'", selected)
	}

	// 测试 CompareRangeWithString
	match, err := editor.CompareRangeWithString(6, 11, "World")
	if err != nil || !match {
		t.Errorf("Expected true, got false")
	}

	// 测试 MoveCursor and GetCurrentLine
	err = editor.MoveCursor(7)
	if err != nil {
		t.Errorf("Moving cursor failed: %s", err)
	}

	currentLine, err := editor.GetCurrentLine()
	if err != nil || currentLine != "World" {
		t.Errorf("Expected 'World', got '%s'", currentLine)
	}
}

// 测试错误的选择操作
func TestSelect_Errors(t *testing.T) {
	editor := NewMemEditor("Hello\nWorld\nThis is a test")

	_, err := editor.Select(5, 0) // start大于end
	if err == nil {
		t.Error("Select() should fail when start index is greater than end index")
	}

	_, err = editor.Select(-1, 10) // 负的起始索引
	if err == nil {
		t.Error("Select() should fail for negative start index")
	}

	_, err = editor.Select(10, 1000) // 结束索引越界
	if err == nil {
		t.Error("Select() should fail for end index out of bounds")
	}
}

// 测试光标移动的错误情况
func TestMoveCursor_Errors(t *testing.T) {
	editor := NewMemEditor("Hello\nWorld\nThis is a test")

	err := editor.MoveCursor(-1) // 负的光标位置
	if err == nil {
		t.Error("MoveCursor() should fail for negative position")
	}

	err = editor.MoveCursor(len(editor.GetSourceCode()) + 1) // 超过文本长度的光标位置
	if err == nil {
		t.Error("MoveCursor() should fail for position out of bounds")
	}
}

// 测试GetLine的错误情况
func TestGetLine_Errors(t *testing.T) {
	editor := NewMemEditor("Hello\nWorld\nThis is a test")

	_, err := editor.GetLine(0) // 不存在的行号
	if err == nil {
		t.Error("GetLine() should fail for line 0 which does not exist")
	}

	_, err = editor.GetLine(10) // 越界的行号
	if err == nil {
		t.Error("GetLine() should fail for out of range line number")
	}
}

func TestTextByRangeGetEditor(t *testing.T) {
	editor := NewMemEditor(`0123456789
0123456789
0123456789
0123456789
0123456789
0123456789`)

	tests := []struct {
		name     string
		startPos *Position
		endPos   *Position
		want     string
		wantErr  bool
	}{
		{
			name:     "Select part of a single line",
			startPos: NewPosition(1, 1),
			endPos:   NewPosition(1, 5),
			want:     "0123",
			wantErr:  false,
		},
		{
			name:     "Select entire single line",
			startPos: NewPosition(2, 1),
			endPos:   NewPosition(2, 11),
			want:     "0123456789",
			wantErr:  false,
		},
		{
			name:     "Select text across multiple lines",
			startPos: NewPosition(1, 1),
			endPos:   NewPosition(3, 6),
			want:     "0123456789\n0123456789\n01234",
			wantErr:  false,
		},
		{
			name:     "Column number out of single line's length",
			startPos: NewPosition(1, 1),
			endPos:   NewPosition(1, 10000),
			want:     "0123456789",
			wantErr:  false,
		},
		{
			name:     "Line number out of text's total lines",
			startPos: NewPosition(1, 1),
			endPos:   NewPosition(10, 1),
			want:     "0123456789\n0123456789\n0123456789\n0123456789\n0123456789\n0123456789",
			wantErr:  true,
		},
		{
			name:     "Start and end positions are the same",
			startPos: NewPosition(2, 3),
			endPos:   NewPosition(2, 3),
			want:     "",
			wantErr:  false,
		},
		{
			name:     "Start position is after end position",
			startPos: NewPosition(3, 6),
			endPos:   NewPosition(3, 3),
			want:     "234",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := editor.GetTextFromRangeWithError(NewRange(tt.startPos, tt.endPos))
			if (err != nil) != tt.wantErr {
				t.Errorf("expected error: %v, got error: %v", tt.wantErr, err)
			}
			if err == nil && result != tt.want {
				t.Errorf("expected result: %q, got result: %q", tt.want, result)
			}
		})
	}
}

func TestSourceCodeContext(t *testing.T) {
	e := NewMemEditor(`code1`)
	if e.SourceCodeMd5() == utils.CalcMd5("code1") {
		t.Log("SourceCodeMd5() passed")
	} else {
		spew.Dump(e.SourceCodeMd5())
		spew.Dump("code1: md5", utils.CalcMd5("code1"))
		t.Fatal("SourceCodeMd5() failed")
	}

	e.PushSourceCodeContext("abc")
	if e.SourceCodeMd5() == utils.CalcMd5("code1") {
		t.Fatal("SourceCode MD5 is not updated after PushSourceCodeContext()")
	}
	spew.Dump(e.SourceCodeMd5())
}

func TestRunesSupporting(t *testing.T) {
	e := NewMemEditor("\n\n\n" + `你好？世界,OOO` + "\n" + `Hello,World`)
	var result string
	result = e.GetTextFromRange(e.GetRangeOffset(4, 5))
	assert.Equal(t, "好", result)
	result = e.GetTextFromRange(e.GetRangeOffset(5, 7))
	assert.Equal(t, "？世", result)

	result = e.GetTextFromPositionInt(4, 3, 4, 6)
	assert.Equal(t, "？世界", result)

	result = e.GetTextFromPositionInt(4, 6, 4, 8)
	assert.Equal(t, ",O", result)

	result = e.GetTextFromPositionInt(4, 6, 5, 3)
	assert.Equal(t, ",OOO\nHe", result)
	println(strconv.Quote(result))
}
