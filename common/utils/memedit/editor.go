package memedit

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type MemEditor struct {
	sourceCodeCtxStack []string

	// hash
	sourceCodeMd5    string
	sourceCodeSha1   string
	sourceCodeSha256 string

	// fileUrl and source
	fileUrl        string
	safeSourceCode *SafeString

	// editor
	lineLensMap        map[int]int
	lineStartOffsetMap map[int]int
	cursor             int // 模拟光标位置（指针功能）
}

func NewMemEditor(sourceCode string) *MemEditor {
	editor := &MemEditor{
		safeSourceCode:     NewSafeString(sourceCode),
		lineLensMap:        make(map[int]int),
		lineStartOffsetMap: make(map[int]int),
	}
	editor.recalculateLineMappings()
	return editor
}

func NewMemEditorWithFileUrl(sourceCode string, fileUrl string) *MemEditor {
	editor := &MemEditor{
		safeSourceCode:     NewSafeString(sourceCode),
		lineLensMap:        make(map[int]int),
		lineStartOffsetMap: make(map[int]int),
		fileUrl:            fileUrl,
	}
	editor.recalculateLineMappings()
	return editor
}

func (ve *MemEditor) CodeLength() int {
	return ve.safeSourceCode.Len()
}

func (ve *MemEditor) SetUrl(url string) {
	ve.fileUrl = url
}

// GetIrSourceHash 使用程序名称、路径和源代码计算哈希值
func (ve *MemEditor) GetIrSourceHash(programName string) string {
	return codec.Md5(programName + ve.GetFilename() + ve.GetSourceCode())
}

func (ve *MemEditor) GetFormatedUrl() string {
	u := ve.fileUrl
	if strings.HasPrefix(u, "file://") {
		return u
	}

	if filepath.IsAbs(ve.fileUrl) {
		return "file://" + ve.fileUrl
	}

	raw, err := filepath.Abs(ve.fileUrl)
	if err != nil {
		if strings.HasPrefix(ve.fileUrl, "./") {
			return ve.fileUrl
		}
		return "./" + ve.fileUrl
	}
	return "file://" + raw
}

func (ve *MemEditor) GetFilename() string {
	return ve.fileUrl
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
	if line < 1 || col < 1 {
		return 0, errors.New("line number and column number must be positive")
	}

	// 调整line为内部索引使用，从0开始
	adjustedLine := line - 1
	adjustedCol := col - 1

	// 检查行号是否超出范围
	if adjustedLine >= len(ve.lineStartOffsetMap) {
		return ve.safeSourceCode.Len(), errors.New("line number out of range")
	}

	// 检查列号是否超出当前行的长度
	if adjustedCol > ve.lineLensMap[adjustedLine] {
		adjustedCol = ve.lineLensMap[adjustedLine] // Clamp the column to the maximum length of the line
	}

	lineStartOffset := ve.lineStartOffsetMap[adjustedLine]
	if adjustedLine < len(ve.lineLensMap)-1 {
		return lineStartOffset + adjustedCol, nil
	} else {
		// For the last line, we need to ensure we do not exceed the length of the source code
		if lineStartOffset+adjustedCol >= ve.safeSourceCode.Len() {
			return ve.safeSourceCode.Len(), nil
		} else {
			return lineStartOffset + adjustedCol, nil
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
	// return ve.sourceCode[start:end], nil
	return ve.safeSourceCode.Slice2(start, end), nil
}

// Select 返回指定范围的文本
func (ve *MemEditor) Select(start, end int) (string, error) {
	if start < 0 || end > ve.safeSourceCode.Len() || start > end {
		return "", errors.New("invalid range for select")
	}
	return ve.safeSourceCode.Slice2(start, end), nil
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
	if position < 0 || position > ve.safeSourceCode.Len() {
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

func (ve *MemEditor) GetPositionByLine(line, column int) PositionIf {
	return NewPosition(line, column)
}

func (ve *MemEditor) GetPositionByOffsetWithError(offset int) (PositionIf, error) {
	if offset < 0 {
		// 偏移量为负，返回最初位置
		return NewPosition(1, 1), errors.New("offset is negative")
	}
	if offset >= ve.safeSourceCode.Len() {
		// 偏移量超出最大范围，返回最后位置
		lastLine := len(ve.lineStartOffsetMap) - 1 // 最后一行的索引（从0开始）
		lastLineStart := ve.lineStartOffsetMap[lastLine]
		lastLineLen := ve.lineLensMap[lastLine]
		outOfRange := utils.Errorf("offset %d is out of range", offset)
		if offset == ve.safeSourceCode.Len() && lastLineLen == 0 {
			// 特殊情况，最后一行无内容
			return NewPosition(lastLine+1, 1), outOfRange
		}
		return NewPosition(lastLine+1, utils.Min(offset-lastLineStart, lastLineLen)+1), outOfRange
	}

	// 使用二分查找定位行
	low, high := 0, len(ve.lineStartOffsetMap)-1
	for low <= high {
		mid := low + (high-low)/2
		startOffset := ve.lineStartOffsetMap[mid]

		if startOffset == offset {
			return NewPosition(mid+1, 1), nil
		} else if startOffset < offset {
			if mid == high || ve.lineStartOffsetMap[mid+1] > offset {
				column := offset - startOffset
				return NewPosition(mid+1, column+1), nil
			}
			low = mid + 1
		} else {
			high = mid - 1
		}
	}

	// 理论上不应该执行到这里
	return NewPosition(1, 1), errors.New("position not found")
}

func (ve *MemEditor) GetRangeOffset(start, end int) RangeIf {
	ret := NewRange(ve.GetPositionByOffset(start), ve.GetPositionByOffset(end))
	ret.SetEditor(ve)
	return ret
}

func (ve *MemEditor) GetRangeByPosition(start, end PositionIf) RangeIf {
	ret := NewRange(start, end)
	ret.SetEditor(ve)
	return ret
}

func (ve *MemEditor) GetFullRange() RangeIf {
	return ve.GetRangeOffset(0, ve.safeSourceCode.Len())
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
	if endOffset > ve.safeSourceCode.Len() {
		return errors.New("end offset is out of bounds")
	}

	// 使用安全的字符串分割方式防止越界
	before := ve.safeSourceCode.SliceBeforeStart(startOffset) // 取起始偏移之前的文本
	after := ""                                               // 默认后续文本为空
	if endOffset < ve.safeSourceCode.Len() {
		after = ve.safeSourceCode.Slice2(endOffset, ve.safeSourceCode.Len()) // 取结束偏移之后的文本
	}

	ve.safeSourceCode = NewSafeString(before + newText + after) // 构造新的源代码

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
	lines := strings.Split(ve.safeSourceCode.String(), "\n")
	ve.lineStartOffsetMap[0] = 0
	for lineNumber, line := range lines {
		lineLen := 0
		if utf8.ValidString(line) {
			lineLen = len([]rune(line))
		} else {
			lineLen = len(line)
		}
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
	if end > ve.safeSourceCode.Len() {
		end = ve.safeSourceCode.Len()
	}
	if end <= 0 {
		end = 0
	}
	// return ve.sourceCode[start:end]
	return ve.safeSourceCode.Slice2(start, end)
}

func (ve *MemEditor) GetOffsetByPosition(p PositionIf) int {
	return ve.GetOffsetByPositionRaw(p.GetLine(), p.GetColumn())
}

func (ve *MemEditor) GetTextFromPosition(p1, p2 PositionIf) string {
	return ve.GetTextFromOffset(ve.GetOffsetByPositionRaw(p1.GetLine(), p1.GetColumn()), ve.GetOffsetByPositionRaw(p2.GetLine(), p2.GetColumn()))
}

func (ve *MemEditor) GetTextFromPositionInt(startLine, startCol, endLine, endCol int) string {
	return ve.GetTextFromOffset(ve.GetOffsetByPositionRaw(startLine, startCol), ve.GetOffsetByPositionRaw(endLine, endCol))
}

func (ve *MemEditor) GetTextFromRange(i RangeIf) string {
	return ve.GetTextFromPosition(i.GetEnd(), i.GetStart())
}

func (ve *MemEditor) boundary(c rune) bool {
	return !('a' <= c && c <= 'z') && !('A' <= c && c <= 'Z') && !('0' <= c && c <= '9')
}

func (ve *MemEditor) ExpandWordTextOffset(startOffset, endOffset int) (int, int) {
	// 扩展起始偏移到前一个单词边界
	startWordOffset := startOffset
	for startWordOffset > 0 && !ve.boundary(rune(ve.safeSourceCode.Slice1(startWordOffset-1))) {
		startWordOffset--
	}

	// 扩展结束偏移到后一个单词边界
	endWordOffset := endOffset
	for endWordOffset < ve.safeSourceCode.Len() && !ve.boundary(ve.safeSourceCode.Slice1(endWordOffset)) {
		endWordOffset++
	}
	return startWordOffset, endWordOffset
}

func (ve *MemEditor) ExpandWordTextRange(i RangeIf) RangeIf {
	startPos := i.GetStart()
	endPos := i.GetEnd()

	startOffset, _ := ve.GetOffsetByPositionWithError(startPos.GetLine(), startPos.GetColumn())
	endOffset, _ := ve.GetOffsetByPositionWithError(endPos.GetLine(), endPos.GetColumn())

	startWordOffset, endWordOffset := ve.ExpandWordTextOffset(startOffset, endOffset)

	return ve.GetRangeByPosition(ve.GetPositionByOffset(startWordOffset), ve.GetPositionByOffset(endWordOffset))
}

func (ve *MemEditor) GetWordTextFromOffset(start, end int) string {
	start, end = ve.ExpandWordTextOffset(start, end)

	return ve.GetTextFromOffset(start, end)
}

func (ve *MemEditor) GetWordTextFromRange(i RangeIf) string {
	i = ve.ExpandWordTextRange(i)

	return ve.GetTextFromRange(i)
}

func (ve *MemEditor) IsOffsetValid(offset int) bool {
	return offset >= 0 && offset <= ve.safeSourceCode.Len()
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
		index := strings.Index(
			ve.safeSourceCode.SliceToEnd(startIndex),
			feature,
		)
		if index == -1 {
			break // No more matches found
		}

		absoluteIndex := startIndex + index
		startPos, _ := ve.GetPositionByOffsetWithError(absoluteIndex)
		endPos, _ := ve.GetPositionByOffsetWithError(absoluteIndex + len(feature))
		err := callback(ve.GetRangeByPosition(startPos, endPos))
		if err != nil {
			return err // Return error if callback fails
		}

		startIndex = absoluteIndex + len(feature) // Move past this feature occurrence
	}
	return nil
}

func (ve *MemEditor) FindStringRangeIndexFirst(startIndex int, feature string, callback func(RangeIf)) (end int, ok bool) {
	index := strings.Index(ve.safeSourceCode.SliceToEnd(startIndex), feature)
	if index == -1 {
		return startIndex, false
	}

	absoluteIndex := startIndex + index
	startPos, _ := ve.GetPositionByOffsetWithError(absoluteIndex)
	endPos, _ := ve.GetPositionByOffsetWithError(absoluteIndex + len(feature))
	callback(ve.GetRangeByPosition(startPos, endPos))
	return startIndex + len(feature), true
}

func (ve *MemEditor) FindRegexpRange(patternStr string, callback func(RangeIf) error) error {
	pattern, err := regexp.Compile(patternStr)
	if err != nil {
		return err // 处理正则表达式编译错误
	}

	text := ve.safeSourceCode.String()
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
		err = callback(ve.GetRangeByPosition(startPos, endPos))
		if err != nil {
			return err // 如果回调函数出错，提前退出
		}

		offset = matchEnd // 更新搜索起点，推进到当前找到的匹配项之后
	}

	return nil
}

func (ve *MemEditor) GetMinAndMaxOffset(pos ...PositionIf) (int, int) {
	minOffset := ve.safeSourceCode.Len()
	maxOffset := 0
	for _, p := range pos {
		offset := ve.GetOffsetByPosition(p)
		minOffset = utils.Min(minOffset, offset)
		maxOffset = utils.Max(maxOffset, offset)
	}
	return minOffset, maxOffset
}

func (ve *MemEditor) GetContextAroundRange(startPos, endPos PositionIf, n int, prefix ...func(i int) string) (string, error) {
	var prefixFunc func(i int) string
	if len(prefix) > 0 && prefix[0] != nil {
		prefixFunc = prefix[0]
	}
	return ve.GetContextAroundRangeEx(startPos, endPos, n, prefixFunc, nil)
}

func (ve *MemEditor) GetContextAroundRangeEx(startPos, endPos PositionIf, n int, prefix func(i int) string, suffix func(i int) string) (string, error) {
	start, end := ve.GetMinAndMaxOffset(startPos, endPos)
	if start < 0 || end > ve.safeSourceCode.Len() || start > end {
		return "", errors.New("invalid range")
	}

	startLine, _ := ve.GetPositionByOffsetWithError(start)
	endLine, _ := ve.GetPositionByOffsetWithError(end)

	startContextLine := utils.Max(startLine.GetLine()-n, 1)
	endContextLine := utils.Min(endLine.GetLine()+n, len(ve.lineStartOffsetMap))

	var contextBuilder strings.Builder
	for i := startContextLine; i <= endContextLine; i++ {
		lineText, _ := ve.GetLine(i)
		if prefix != nil {
			contextBuilder.WriteString(prefix(i))
		}
		contextBuilder.WriteString(lineText)
		if suffix != nil {
			contextBuilder.WriteString(suffix(i))
		}
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

func (ve *MemEditor) getCurrentSourceCodeContextText() string {
	salt := ve.safeSourceCode.String()
	if len(ve.sourceCodeCtxStack) > 0 {
		salt += strings.Join(ve.sourceCodeCtxStack, "\n")
	}
	return salt
}

func (ve *MemEditor) SourceCodeMd5() string {
	if ve.sourceCodeMd5 == "" {
		ve.sourceCodeMd5 = utils.CalcMd5(ve.getCurrentSourceCodeContextText())
	}
	return ve.sourceCodeMd5
}

func (ve *MemEditor) GetPureSourceHash() string {
	return codec.Sha256(ve.safeSourceCode.String())
}

func (ve *MemEditor) SourceCodeSha1() string {
	if ve.sourceCodeSha1 == "" {
		ve.sourceCodeSha1 = utils.CalcSha1(ve.getCurrentSourceCodeContextText())
	}
	return ve.sourceCodeSha1
}

func (ve *MemEditor) SourceCodeSha256() string {
	if ve.sourceCodeSha256 == "" {
		ve.sourceCodeSha256 = utils.CalcSha256(ve.getCurrentSourceCodeContextText())
	}
	return ve.sourceCodeSha256
}

func (ve *MemEditor) GetSourceCode() string {
	return ve.safeSourceCode.String()
}

func (e *MemEditor) GetTextContextWithPrompt(p RangeIf, n int, msg ...string) string {
	start := p.GetStart()
	end := p.GetEnd()

	const prefixTemplate = "%4d | "
	const prefixHitTemplate = "%4d > "
	const suffixTemplate = "       "

	endMessage := strings.Join(msg, " ")
	endMessage = strings.ReplaceAll(endMessage, "\n", " ")

	raw, err := e.GetContextAroundRangeEx(start, end, n, func(i int) string {
		if i >= start.GetLine() && i <= end.GetLine() {
			return fmt.Sprintf(prefixHitTemplate, i)
		} else {
			return fmt.Sprintf(prefixTemplate, i)
		}
	}, func(i int) string {
		if i > end.GetLine() || i < start.GetLine() {
			return ""
		}

		var buf bytes.Buffer
		buf.WriteByte('\n')
		buf.WriteString(suffixTemplate)

		if start.GetLine() == end.GetLine() {
			line, _ := e.GetLine(i)
			for j := 0; j < len(line); j++ {
				if j < start.GetColumn() {
					buf.WriteByte(' ')
				} else if j == start.GetColumn() {
					buf.WriteByte('^')
				} else if j > start.GetColumn() && j <= end.GetColumn() {
					buf.WriteByte('~')
				} else {
					buf.WriteByte(' ')
				}
			}
			if strings.TrimSpace(endMessage) != "" {
				buf.WriteString(" -- " + endMessage)
			}
			return buf.String()
		}

		if start.GetLine() > end.GetLine() {
			return ""
		}

		if i < end.GetLine() && i > start.GetLine() {
			line, _ := e.GetLine(i)
			for j := 0; j < len(line); j++ {
				buf.WriteByte('~')
			}
			return buf.String()
		}

		if i == start.GetLine() {
			line, _ := e.GetLine(i)
			for j := 0; j < len(line); j++ {
				if j < start.GetColumn() {
					buf.WriteByte(' ')
				} else if j == start.GetColumn() {
					buf.WriteByte('^')
				} else {
					buf.WriteByte('~')
				}
			}
			return buf.String()
		}

		if i == end.GetLine() {
			for j := 0; j < end.GetColumn()+1; j++ {
				if j == end.GetColumn() {
					buf.WriteByte('^')
				} else if j < end.GetColumn() {
					buf.WriteByte('~')
				} else {
					buf.WriteByte(' ')
				}
			}
			if strings.TrimSpace(endMessage) != "" {
				buf.WriteString(" -- " + endMessage)
			}
			return buf.String()
		}

		return ""
	})
	if err != nil {
		return ""
	}
	return raw
}
