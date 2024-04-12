package memedit

import (
	"errors"
	"strings"
)

type MemEditor struct {
	sourceCode         string
	lineLensMap        map[int]int
	lineStartOffsetMap map[int]int
	cursor             int // 模拟光标位置（指针功能）
}

func NewMemEditor(sourceCode string) *MemEditor {
	editor := &MemEditor{
		sourceCode:         sourceCode,
		lineLensMap:        make(map[int]int),
		lineStartOffsetMap: make(map[int]int),
		cursor:             0,
	}

	currentOffset := 0
	editor.lineStartOffsetMap[0] = 0
	lines := strings.Split(sourceCode, "\n")

	for lineNumber, line := range lines {
		lineLen := len(line)
		editor.lineLensMap[lineNumber] = lineLen
		editor.lineStartOffsetMap[lineNumber+1] = currentOffset + lineLen + 1
		currentOffset += lineLen + 1
	}

	return editor
}

func (ve *MemEditor) GetOffsetByPosition(x, y int) (int, error) {
	if x < 1 || y < 0 {
		return 0, errors.New("line number and column number should be positive")
	}

	x = x - 1
	if _, exists := ve.lineStartOffsetMap[x]; !exists {
		return 0, errors.New("line number out of range")
	}

	if y > ve.lineLensMap[x] {
		return 0, errors.New("column number out of range")
	}

	return ve.lineStartOffsetMap[x] + y, nil
}

func (ve *MemEditor) GetStartOffsetByLine(x int) (int, error) {
	if x < 1 {
		return 0, errors.New("line number should be positive")
	}

	x = x - 1
	if _, exists := ve.lineStartOffsetMap[x]; !exists {
		return 0, errors.New("line number out of range")
	}

	return ve.lineStartOffsetMap[x], nil
}

func (ve *MemEditor) GetEndOffsetByLine(x int) (int, error) {
	if x < 1 {
		return 0, errors.New("line number should be positive")
	}

	x = x - 1
	if _, exists := ve.lineStartOffsetMap[x]; !exists {
		return 0, errors.New("line number out of range")
	}

	return ve.lineStartOffsetMap[x] + ve.lineLensMap[x], nil
}

// 获取指定行的内容
func (ve *MemEditor) GetLine(x int) (string, error) {
	start, err := ve.GetStartOffsetByLine(x)
	if err != nil {
		return "", err
	}
	end, err := ve.GetEndOffsetByLine(x)
	if err != nil {
		return "", err
	}
	return ve.sourceCode[start:end], nil
}

// Select 返回指定范围的文本
func (ve *MemEditor) Select(start, end int) (string, error) {
	if start < 0 || end > len(ve.sourceCode) || start > end {
		return "", errors.New("invalid range for select")
	}
	return ve.sourceCode[start:end], nil
}

// 比较指定范围的文本是否与给定字符串相同
func (ve *MemEditor) CompareRangeWithString(start, end int, compareTo string) (bool, error) {
	selectedText, err := ve.Select(start, end)
	if err != nil {
		return false, err
	}
	return selectedText == compareTo, nil
}

// MoveCursor 移动模拟光标位置
func (ve *MemEditor) MoveCursor(position int) error {
	if position < 0 || position > len(ve.sourceCode) {
		return errors.New("position out of bounds")
	}
	ve.cursor = position
	return nil
}

// GetCurrentLine 返回当前光标所在行的内容
func (ve *MemEditor) GetCurrentLine() (string, error) {
	for lineNumber, startOffset := range ve.lineStartOffsetMap {
		if ve.cursor >= startOffset && ve.cursor <= (startOffset+ve.lineLensMap[lineNumber]) {
			return ve.GetLine(lineNumber + 1)
		}
	}
	return "", errors.New("current position is out of the source code range")
}

// GetPositionByOffset 获取给定偏移量的位置信息
func (ve *MemEditor) GetPositionByOffset(offset int) (PositionIf, error) {
	if offset < 0 || offset >= len(ve.sourceCode) {
		return nil, errors.New("offset out of bounds")
	}
	for line, startOffset := range ve.lineStartOffsetMap {
		if startOffset > offset {
			continue
		}
		lineLength := ve.lineLensMap[line]
		if offset < startOffset+lineLength+1 {
			column := offset - startOffset
			return NewPosition(line+1, column+1, offset), nil
		}
	}
	return nil, errors.New("position not found")
}

// GetTextByRange 根据Range获取文本
func (ve *MemEditor) GetTextByRange(r RangeIf) (string, error) {
	startOffset := r.GetStart().GetOffset()
	endOffset := r.GetEnd().GetOffset()
	if startOffset > endOffset {
		return "", errors.New("start position is after end position")
	}
	return ve.Select(startOffset, endOffset)
}

// UpdateTextByRange 根据Range更新文本
func (ve *MemEditor) UpdateTextByRange(r RangeIf, newText string) error {
	startOffset := r.GetStart().GetOffset()
	endOffset := r.GetEnd().GetOffset()
	if startOffset > endOffset {
		return errors.New("start position is after end position")
	}

	before := ve.sourceCode[:startOffset]
	after := ve.sourceCode[endOffset:]
	ve.sourceCode = before + newText + after

	// Update the lineLensMap and lineStartOffsetMap
	ve.recalculateLineMappings()

	return nil
}

// recalculateLineMappings 重新计算行映射
func (ve *MemEditor) recalculateLineMappings() {
	ve.lineLensMap = make(map[int]int)
	ve.lineStartOffsetMap = make(map[int]int)
	currentOffset := 0
	lines := strings.Split(ve.sourceCode, "\n")
	ve.lineStartOffsetMap[0] = 0
	for lineNumber, line := range lines {
		lineLen := len(line)
		ve.lineLensMap[lineNumber] = lineLen
		ve.lineStartOffsetMap[lineNumber+1] = currentOffset + lineLen + 1
		currentOffset += lineLen + 1
	}
}
