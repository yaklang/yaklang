package memedit

import (
	"testing"
)

func TestNewMemEditor_SMOCKING(t *testing.T) {
	editor := NewMemEditor("Hello\nWorld\nThis is a test")
	if editor == nil {
		t.Fatal("Failed to create MemEditor instance")
	}
}

func TestGetOffsetByPosition(t *testing.T) {
	editor := NewMemEditor("Hello\nWorld\nThis is a test")

	tests := []struct {
		name    string
		x, y    int
		want    int
		wantErr bool
	}{
		{"Start of file", 1, 0, 0, false},
		{"Start of second line", 2, 0, 6, false},
		{"Random position", 2, 1, 7, false},
		{"End of file", 4, 14, 27, false},
		{"Out of range line", 5, 0, 0, true},
		{"Out of range column", 1, 6, 0, true},
		{"Negative line", -1, 0, 0, true},
		{"Negative column", 1, -1, 0, true},
	}

	for _, tt := range tests {
		got, err := editor.GetOffsetByPosition(tt.x, tt.y)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: GetOffsetByPosition() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("%s: GetOffsetByPosition() = %v, want %v", tt.name, got, tt.want)
		}
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
	startPos, _ := editor.GetPositionByOffset(0)
	endPos, _ := editor.GetPositionByOffset(5)

	err := editor.UpdateTextByRange(NewRange(startPos, endPos), "Hi")
	if err != nil {
		t.Fatal("UpdateTextByRange() failed:", err)
	}

	expected := "Hi\nWorld\nThis is a test"
	if editor.sourceCode != expected {
		t.Errorf("UpdateTextByRange() got = %v, want %v", editor.sourceCode, expected)
	}

	// Test out of bounds
	err = editor.UpdateTextByRange(NewRange(startPos, NewPosition(100, 100, 100)), "Hi")
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
