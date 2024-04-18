package memedit

import (
	"errors"
	"regexp"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/utils"
)

var defaultMemEditorPool sync.Pool = sync.Pool{
	New: func() interface{} {
		return new(MemEditor)
	},
}

type MemEditor struct {
	sourceCodeCtxStack []string

	sourceCodeMd5      string
	sourceCodeSha1     string
	sourceCodeSha256   string
	sourceCode         string
	lineLensMap        map[int]int
	lineStartOffsetMap map[int]int
	cursor             int // 模拟光标位置（指针功能）
}

func NewMemEditor(sourceCode string) *MemEditor {
	editor := defaultMemEditorPool.Get().(*MemEditor)

	editor.sourceCode = sourceCode
	editor.lineLensMap = make(map[int]int)
	editor.lineStartOffsetMap = make(map[int]int)

	editor.recalculateLineMappings()
	return editor
}

func (ve *MemEditor) Release() {
	ve.sourceCode = ""
	ve.lineLensMap = nil
	ve.lineStartOffsetMap = nil
	ve.cursor = 0
	defaultMemEditorPool.Put(ve)
}

func (ve *MemEditor) PushSourceCodeContext(i any) {
	ve.ResetSourceCodeHash()

	ve.sourceCodeCtxStack = append(ve.sourceCodeCtxStack, codec.AnyToString(i))
}

func (ve *MemEditor) GetOffsetByPositionRaw(line, col int) int {
	offset, _ := ve.GetOffsetByPositionWithError(line, col)
	return offset
}

func (ve *MemEditor) GetOffsetByPositionWithError(line, col int) (int, error) {
	if line < 1 || col < 0 {
		return 0, errors.New("line number and column number must be positive")
	}

	// 调整line为内部索引使用，从0开始
	adjustedLine := line - 1

	// 检查行号是否超出范围
	if adjustedLine >= len(ve.lineStartOffsetMap) {
		return len(ve.sourceCode), errors.New("line number out of range")
	}

	// 检查列号是否超出当前行的长度
	if col > ve.lineLensMap[adjustedLine] {
		col = ve.lineLensMap[adjustedLine] // Clamp the column to the maximum length of the line
	}

	lineStartOffset := ve.lineStartOffsetMap[adjustedLine]
	if adjustedLine < len(ve.lineLensMap)-1 {
		return lineStartOffset + col, nil
	} else {
		// For the last line, we need to ensure we do not exceed the length of the source code
		if lineStartOffset+col >= len(ve.sourceCode) {
			return len(ve.sourceCode), nil
		} else {
			return lineStartOffset + col, nil
		}
	}
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

func (ve *MemEditor) GetPositionByOffset(offset int) PositionIf {
	result, _ := ve.GetPositionByOffsetWithError(offset)
	return result
}

func (ve *MemEditor) GetPositionByOffsetWithError(offset int) (PositionIf, error) {
	if offset < 0 {
		// 偏移量为负，返回最初位置
		return NewPosition(1, 0), errors.New("offset is negative")
	}
	if offset >= len(ve.sourceCode) {
		// 偏移量超出最大范围，返回最后位置
		lastLine := len(ve.lineStartOffsetMap) - 1 // 最后一行的索引（从0开始）
		lastLineStart := ve.lineStartOffsetMap[lastLine]
		lastLineLen := ve.lineLensMap[lastLine]
		outOfRange := utils.Errorf("offset %d is out of range", offset)
		if offset == len(ve.sourceCode) && lastLineLen == 0 {
			// 特殊情况，最后一行无内容
			return NewPosition(lastLine+1, 0), outOfRange
		}
		return NewPosition(lastLine+1, utils.Min(offset-lastLineStart, lastLineLen)), outOfRange
	}

	// 使用二分查找定位行
	low, high := 0, len(ve.lineStartOffsetMap)-1
	for low <= high {
		mid := low + (high-low)/2
		startOffset := ve.lineStartOffsetMap[mid]

		if startOffset == offset {
			return NewPosition(mid+1, 0), nil
		} else if startOffset < offset {
			if mid == high || ve.lineStartOffsetMap[mid+1] > offset {
				column := offset - startOffset
				return NewPosition(mid+1, column), nil
			}
			low = mid + 1
		} else {
			high = mid - 1
		}
	}

	// 理论上不应该执行到这里
	return NewPosition(1, 0), errors.New("position not found")
}

// GetTextFromRangeWithError 根据Range获取文本，优先使用Offset，其次使用Line和Column
func (ve *MemEditor) GetTextFromRangeWithError(r RangeIf) (string, error) {
	start := r.GetStart()
	end := r.GetEnd()

	var startOffset, endOffset int
	// 使用Line和Column计算Offset
	var err error
	startOffset, err = ve.GetOffsetByPositionWithError(start.GetLine(), start.GetColumn())
	if err != nil {
		return "", err
	}
	endOffset, err = ve.GetOffsetByPositionWithError(end.GetLine(), end.GetColumn())
	if err != nil {
		return "", err
	}

	if startOffset > endOffset {
		return "", errors.New("start position is after end position")
	}
	return ve.Select(startOffset, endOffset)
}

// UpdateTextByRange 根据Range更新文本，优先使用Offset，其次使用Line和Column
func (ve *MemEditor) UpdateTextByRange(r RangeIf, newText string) error {
	start := r.GetStart()
	end := r.GetEnd()

	var startOffset, endOffset int
	var err error
	// 使用Line和Column计算Offset
	startOffset, err = ve.GetOffsetByPositionWithError(start.GetLine(), start.GetColumn())
	if err != nil {
		return err // 如果计算偏移出错，返回错误
	}
	endOffset, err = ve.GetOffsetByPositionWithError(end.GetLine(), end.GetColumn())
	if err != nil {
		return err // 如果计算偏移出错，返回错误
	}

	// 检查偏移范围是否有效
	if startOffset > endOffset {
		return errors.New("start position is after end position")
	}
	if endOffset > len(ve.sourceCode) {
		return errors.New("end offset is out of bounds")
	}

	// 使用安全的字符串分割方式防止越界
	before := ve.sourceCode[:startOffset] // 取起始偏移之前的文本
	after := ""                           // 默认后续文本为空
	if endOffset < len(ve.sourceCode) {
		after = ve.sourceCode[endOffset:] // 取结束偏移之后的文本
	}

	ve.sourceCode = before + newText + after // 构造新的源代码

	// 更新行信息映射
	ve.recalculateLineMappings()
	return nil
}

func (ve *MemEditor) ResetSourceCodeHash() {
	if ve == nil {
		return
	}
	ve.sourceCodeMd5 = ""
	ve.sourceCodeSha1 = ""
	ve.sourceCodeSha256 = ""
}

// recalculateLineMappings 重新计算行映射
func (ve *MemEditor) recalculateLineMappings() {
	ve.ResetSourceCodeHash()
	ve.SourceCodeMd5()
	ve.SourceCodeSha1()
	ve.SourceCodeSha256()

	ve.lineLensMap = make(map[int]int)
	ve.lineStartOffsetMap = make(map[int]int)
	currentOffset := 0
	lines := strings.Split(ve.sourceCode, "\n")
	ve.lineStartOffsetMap[0] = 0
	for lineNumber, line := range lines {
		lineLen := len(line)
		ve.lineLensMap[lineNumber] = lineLen
		if lineNumber+1 < len(lines) {
			ve.lineStartOffsetMap[lineNumber+1] = currentOffset + lineLen + 1
		}
		currentOffset += lineLen + 1
	}
}

func (ve *MemEditor) GetTextFromOffset(offset1, offset2 int) string {
	start, end := utils.Min(offset1, offset2), utils.Max(offset1, offset2)
	if start < 0 {
		start = 0
	}
	if end > len(ve.sourceCode) {
		end = len(ve.sourceCode)
	}
	if end <= 0 {
		end = 0
	}
	return ve.sourceCode[start:end]
}

func (ve *MemEditor) GetOffsetByPosition(p PositionIf) int {
	return ve.GetOffsetByPositionRaw(p.GetLine(), p.GetColumn())
}

func (ve *MemEditor) GetTextFromPosition(p1, p2 PositionIf) string {
	return ve.GetTextFromOffset(ve.GetOffsetByPositionRaw(p1.GetLine(), p1.GetColumn()), ve.GetOffsetByPositionRaw(p2.GetLine(), p2.GetColumn()))
}

func (ve *MemEditor) GetTextFromRange(i RangeIf) string {
	return ve.GetTextFromPosition(i.GetEnd(), i.GetStart())
}

func (ve *MemEditor) boundary(c rune) bool {
	return !('a' <= c && c <= 'z') && !('A' <= c && c <= 'Z') && !('0' <= c && c <= '9')
}

func (ve *MemEditor) ExpandWordTextRange(i RangeIf) RangeIf {
	startPos := i.GetStart()
	endPos := i.GetEnd()

	startOffset, _ := ve.GetOffsetByPositionWithError(startPos.GetLine(), startPos.GetColumn())
	endOffset, _ := ve.GetOffsetByPositionWithError(endPos.GetLine(), endPos.GetColumn())

	// 定义单词边界符，这里使用非字母数字作为分隔符，可根据实际需求调整
	boundary := func(c rune) bool {
		return !('a' <= c && c <= 'z') && !('A' <= c && c <= 'Z') && !('0' <= c && c <= '9')
	}

	// 扩展起始偏移到前一个单词边界
	startWordOffset := startOffset
	for startWordOffset > 0 && !boundary(rune(ve.sourceCode[startWordOffset-1])) {
		startWordOffset--
	}

	// 扩展结束偏移到后一个单词边界
	endWordOffset := endOffset
	for endWordOffset < len(ve.sourceCode) && !boundary(rune(ve.sourceCode[endWordOffset])) {
		endWordOffset++
	}
	return NewRange(ve.GetPositionByOffset(startWordOffset), ve.GetPositionByOffset(endWordOffset))
}

func (ve *MemEditor) GetWordTextFromRange(i RangeIf) string {
	i = ve.ExpandWordTextRange(i)

	return ve.GetTextFromRange(i)
}

func (ve *MemEditor) IsOffsetValid(offset int) bool {
	return offset >= 0 && offset <= len(ve.sourceCode)
}

func (ve *MemEditor) IsValidPosition(line, col int) bool {
	if line < 1 || col < 0 {
		return false
	}
	adjustedLine := line - 1
	if adjustedLine >= len(ve.lineStartOffsetMap) {
		return false
	}
	return col <= ve.lineLensMap[adjustedLine]
}

func (ve *MemEditor) FindStringRange(feature string, callback func(RangeIf) error) error {
	startIndex := 0
	for {
		index := strings.Index(ve.sourceCode[startIndex:], feature)
		if index == -1 {
			break // No more matches found
		}

		absoluteIndex := startIndex + index
		startPos, _ := ve.GetPositionByOffsetWithError(absoluteIndex)
		endPos, _ := ve.GetPositionByOffsetWithError(absoluteIndex + len(feature))
		err := callback(NewRange(startPos, endPos))
		if err != nil {
			return err // Return error if callback fails
		}

		startIndex = absoluteIndex + len(feature) // Move past this feature occurrence
	}
	return nil
}

func (ve *MemEditor) FindRegexpRange(patternStr string, callback func(RangeIf) error) error {
	pattern, err := regexp.Compile(patternStr)
	if err != nil {
		return err // 处理正则表达式编译错误
	}

	text := ve.sourceCode
	offset := 0 // 维护当前的搜索起点，逐步推进

	for {
		matches := pattern.FindStringIndex(text[offset:])
		if matches == nil {
			break // 如果没有找到匹配项，退出循环
		}

		// 调整matches的索引，使其相对于整个文本
		matchStart := offset + matches[0]
		matchEnd := offset + matches[1]

		startPos, _ := ve.GetPositionByOffsetWithError(matchStart)
		endPos, _ := ve.GetPositionByOffsetWithError(matchEnd)
		err = callback(NewRange(startPos, endPos))
		if err != nil {
			return err // 如果回调函数出错，提前退出
		}

		offset = matchEnd // 更新搜索起点，推进到当前找到的匹配项之后
	}

	return nil
}

func (ve *MemEditor) GetMinAndMaxOffset(pos ...PositionIf) (int, int) {
	minOffset := len(ve.sourceCode)
	maxOffset := 0
	for _, p := range pos {
		offset := ve.GetOffsetByPosition(p)
		minOffset = utils.Min(minOffset, offset)
		maxOffset = utils.Max(maxOffset, offset)
	}
	return minOffset, maxOffset
}

func (ve *MemEditor) GetContextAroundRange(startPos, endPos PositionIf, n int) (string, error) {
	start, end := ve.GetMinAndMaxOffset(startPos, endPos)
	if start < 0 || end > len(ve.sourceCode) || start > end {
		return "", errors.New("invalid range")
	}

	startLine, _ := ve.GetPositionByOffsetWithError(start)
	endLine, _ := ve.GetPositionByOffsetWithError(end)

	startContextLine := utils.Max(startLine.GetLine()-n, 1)
	endContextLine := utils.Min(endLine.GetLine()+n, len(ve.lineStartOffsetMap))

	var contextBuilder strings.Builder
	for i := startContextLine; i <= endContextLine; i++ {
		lineText, _ := ve.GetLine(i)
		contextBuilder.WriteString(lineText)
		contextBuilder.WriteString("\n")
	}

	return contextBuilder.String(), nil
}

func (ve *MemEditor) GetTextFromRangeContext(i RangeIf, lineNum int) string {
	startPos := i.GetStart()
	endPos := i.GetEnd()
	context, _ := ve.GetContextAroundRange(startPos, endPos, lineNum)
	return context
}

func (ve *MemEditor) GetCurrentSourceCodeContextText() string {
	var salt string
	if len(ve.sourceCodeCtxStack) > 0 {
		salt = strings.Join(ve.sourceCodeCtxStack, "\n")
	}
	return salt
}

func (ve *MemEditor) SourceCodeMd5() string {
	if ve.sourceCodeMd5 == "" {
		ve.sourceCodeMd5 = utils.CalcMd5(ve.sourceCode, ve.GetCurrentSourceCodeContextText())
	}
	return ve.sourceCodeMd5
}

func (ve *MemEditor) SourceCodeSha1() string {
	if ve.sourceCodeSha1 == "" {
		ve.sourceCodeSha1 = utils.CalcSha1(ve.sourceCode, ve.GetCurrentSourceCodeContextText())
	}
	return ve.sourceCodeSha1
}

func (ve *MemEditor) SourceCodeSha256() string {
	if ve.sourceCodeSha256 == "" {
		ve.sourceCodeSha256 = utils.CalcSha256(ve.sourceCode, ve.GetCurrentSourceCodeContextText())
	}
	return ve.sourceCodeSha256
}

func (ve *MemEditor) GetSourceCode() string {
	return ve.sourceCode
}
