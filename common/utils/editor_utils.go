package utils

import "strings"

// VirtualEditor 定义了一个虚拟编辑器的结构体
type VirtualEditor struct {
	sourceCode         string      // 源代码
	lineLensMap        map[int]int // 每一行代码的长度
	lineStartOffsetMap map[int]int // 每一行代码的起始偏移量
}

// NewVirtualEditor 是一个工厂函数，用于创建VirtualEditor实例
func NewVirtualEditor(sourceCode string) *VirtualEditor {
	editor := &VirtualEditor{
		sourceCode:         sourceCode,
		lineLensMap:        make(map[int]int),
		lineStartOffsetMap: make(map[int]int),
	}

	// 初始化累积偏移量为0
	currentOffset := 0
	editor.lineStartOffsetMap[0] = 0

	// 分割源代码到不同的行
	lines := strings.Split(sourceCode, "\n")

	// 遍历每一行，计算长度并存入lineLensMap，同时更新累积偏移量
	for lineNumber, line := range lines {
		editor.lineLensMap[lineNumber] = len(line)
		currentOffset += len(line) + 1 // 加上行长度和换行符长度
		editor.lineStartOffsetMap[lineNumber+1] = currentOffset
	}

	return editor
}

// GetOffsetByPosition 是一个指针方法，用于获取指定行号和列号在sourceCode中的偏移量
// 行号为了对齐antlr4的行号，从1开始计数
func (ve *VirtualEditor) GetOffsetByPosition(x, y int) (int, error) {
	x = x - 1
	// 行号和列号应该是正数
	if x < 0 || y < 0 {
		return 0, Errorf("line number and column number should be positive")
	}

	offset := 0
	if _, exists := ve.lineStartOffsetMap[x]; !exists {
		return 0, Errorf("line %d out of range", x)
	}

	// 如果列号超出了当前行的长度
	if y > ve.lineLensMap[x] {
		return 0, Errorf("line %d out of range", x)
	}

	// 加上列号
	offset = ve.lineStartOffsetMap[x] + y

	return offset, nil
}

// GetStartOffsetByLine 是一个指针方法，用于获取指定行号的起始偏移量
// 行号为了对齐antlr4的行号和列号，从1开始计数
func (ve *VirtualEditor) GetStartOffsetByLine(x int) (int, error) {
	x = x - 1
	if x < 0 {
		return 0, Errorf("line number should be positive")
	}

	offset := 0
	if _, exists := ve.lineStartOffsetMap[x]; !exists {
		return 0, Errorf("line %d out of range", x)
	}

	// 加上列号
	offset = ve.lineStartOffsetMap[x]

	return offset, nil
}

// GetEndOffsetByLine 是一个指针方法，用于获取指定行号的结束偏移量
// 行号为了对齐antlr4的行号，从1开始计数
func (ve *VirtualEditor) GetEndOffsetByLine(x int) (int, error) {
	x = x - 1
	if x < 0 {
		return 0, Errorf("line number should be positive")
	}

	offset := 0
	if _, exists := ve.lineStartOffsetMap[x]; !exists {
		return 0, Errorf("line %d out of range", x)
	}

	// 加上列号
	offset = ve.lineStartOffsetMap[x] + ve.lineLensMap[x]

	return offset, nil
}
