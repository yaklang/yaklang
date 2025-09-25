package memedit

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var (
	ErrorStop = errors.New("stop")
)

type MemEditor struct {
	sourceCodeCtxStack []string

	// hash
	sourceCodeMd5    string
	sourceCodeSha1   string
	sourceCodeSha256 string

	// fileUrl and source
	fileUrl string

	programName string
	folderPath  string
	fileName    string

	safeSourceCode *SafeString

	// editor
	lineLensMap        map[int]int
	lineStartOffsetMap map[int]int
	cursor             int // 模拟光标位置（指针功能）
}

func NewMemEditorByBytes(bs []byte) *MemEditor {
	str := utils.UnsafeBytesToString(bs)
	return NewMemEditor(str)
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
	editor := NewMemEditor(sourceCode)
	editor.SetUrl(fileUrl)
	return editor
}

func (ve *MemEditor) CodeLength() int {
	return ve.safeSourceCode.Len()
}

func (ve *MemEditor) GetLineCount() int {
	return len(ve.lineLensMap)
}

func (ve *MemEditor) SetUrl(url string) {
	ve.fileUrl = url
}

func (ve *MemEditor) GetUrl() string {
	return path.Join("/", ve.GetProgramName(), ve.GetFolderPath(), ve.GetFilename())
}

func (ve *MemEditor) SetProgramName(programName string) {
	ve.programName = programName
}

func (ve *MemEditor) GetProgramName() string {
	return ve.programName
}

func (ve *MemEditor) SetFolderPath(folderPath string) {
	ve.folderPath = folderPath
}

func (ve *MemEditor) GetFolderPath() string {
	if ve.folderPath == "" && ve.fileUrl != "" {
		// split from ve.GetUrl
		ve.folderPath, ve.fileName = path.Split(ve.fileUrl)
	}
	return ve.folderPath
}

func (ve *MemEditor) SetFileName(fileName string) {
	ve.fileName = fileName
}

// GetIrSourceHash 使用程序名称、路径和源代码计算哈希值
func (ve *MemEditor) GetIrSourceHash() string {
	data := ve.GetProgramName() + ve.GetFolderPath() + ve.GetFilename() + ve.GetSourceCode()
	hash := codec.Md5(data)
	return hash
}

func (ve *MemEditor) GetFilename() string {
	if ve.fileName == "" {
		// split from ve.GetUrl
		ve.folderPath, ve.fileName = path.Split(ve.fileUrl)
	}
	return ve.fileName
}

func (ve *MemEditor) GetLength() int {
	return ve.safeSourceCode.Len()
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

func (ve *MemEditor) GetPositionByOffset(offset int) *Position {
	result, _ := ve.GetPositionByOffsetWithError(offset)
	return result
}

func (ve *MemEditor) GetPositionByLine(line, column int) *Position {
	return NewPosition(line, column)
}

func (ve *MemEditor) GetPositionByOffsetWithError(offset int) (*Position, error) {
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

func (ve *MemEditor) GetRangeOffset(start, end int) *Range {
	ret := NewRange(ve.GetPositionByOffset(start), ve.GetPositionByOffset(end))
	ret.SetEditor(ve)
	return ret
}

func (ve *MemEditor) GetRangeByPosition(start, end *Position) *Range {
	ret := NewRange(start, end)
	ret.SetEditor(ve)
	return ret
}

func (ve *MemEditor) GetFullRange() *Range {
	return ve.GetRangeOffset(0, ve.safeSourceCode.Len())
}

// GetTextFromRangeWithError 根据Range获取文本，优先使用Offset，其次使用Line和Column
func (ve *MemEditor) GetTextFromRangeWithError(r *Range) (string, error) {
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
func (ve *MemEditor) UpdateTextByRange(r *Range, newText string) error {
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

	lines := strings.Split(ve.safeSourceCode.String(), "\n")
	lineNums := len(lines)

	ve.lineLensMap = make(map[int]int, lineNums)
	ve.lineStartOffsetMap = make(map[int]int, lineNums)
	currentOffset := 0
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

func (ve *MemEditor) GetOffsetByPosition(p *Position) int {
	return ve.GetOffsetByPositionRaw(p.GetLine(), p.GetColumn())
}

func (ve *MemEditor) GetTextFromPosition(p1, p2 *Position) string {
	return ve.GetTextFromOffset(ve.GetOffsetByPositionRaw(p1.GetLine(), p1.GetColumn()), ve.GetOffsetByPositionRaw(p2.GetLine(), p2.GetColumn()))
}

func (ve *MemEditor) GetTextFromPositionInt(startLine, startCol, endLine, endCol int) string {
	return ve.GetTextFromOffset(ve.GetOffsetByPositionRaw(startLine, startCol), ve.GetOffsetByPositionRaw(endLine, endCol))
}

func (ve *MemEditor) GetTextFromRange(i *Range) string {
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

func (ve *MemEditor) ExpandWordTextRange(i *Range) *Range {
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

func (ve *MemEditor) GetWordTextFromRange(i *Range) string {
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

func (ve *MemEditor) FindStringRange(feature string, callback func(*Range) error) error {
	startIndex := 0
	for {
		featureRunes := []rune(feature)
		featureLen := len(featureRunes)
		index := ve.safeSourceCode.SafeSliceToEnd(startIndex).Index(featureRunes)
		if index == -1 {
			break // No more matches found
		}

		absoluteIndex := startIndex + index
		startPos, _ := ve.GetPositionByOffsetWithError(absoluteIndex)
		endPos, _ := ve.GetPositionByOffsetWithError(absoluteIndex + featureLen)
		err := callback(ve.GetRangeByPosition(startPos, endPos))
		if err != nil {
			return err // Return error if callback fails
		}

		startIndex = absoluteIndex + featureLen // Move past this feature occurrence
	}
	return nil
}

func (ve *MemEditor) FindStringRangeIndexFirst(startIndex int, feature string, callback func(*Range)) (end int, ok bool) {
	var r *Range
	ve.FindStringRange(feature, func(ri *Range) error {
		r = ri
		ok = true
		callback(ri)
		return ErrorStop
	})
	if !ok {
		return -1, false
	}
	return r.GetEndOffset(), true
}

func (ve *MemEditor) FindRegexpRange(patternStr string, callback func(*Range) error) error {
	pattern, err := regexp2.Compile(patternStr, regexp2.None)
	if err != nil {
		return err // 处理正则表达式编译错误
	}
	match, err := pattern.FindRunesMatch(ve.safeSourceCode.Runes())
	if err != nil {
		return err
	}

	for {
		if match == nil {
			break
		}
		matchStart := match.Index
		matchEnd := matchStart + match.Length

		startPos, _ := ve.GetPositionByOffsetWithError(matchStart)
		endPos, _ := ve.GetPositionByOffsetWithError(matchEnd)
		err = callback(ve.GetRangeByPosition(startPos, endPos))
		if err != nil {
			return err // 如果回调函数出错，提前退出
		}
		match, err = pattern.FindNextMatch(match)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ve *MemEditor) GetMinAndMaxOffset(pos ...*Position) (int, int) {
	minOffset := ve.safeSourceCode.Len()
	maxOffset := 0
	for _, p := range pos {
		offset := ve.GetOffsetByPosition(p)
		minOffset = utils.Min(minOffset, offset)
		maxOffset = utils.Max(maxOffset, offset)
	}
	return minOffset, maxOffset
}

func (ve *MemEditor) GetContextAroundRange(startPos, endPos *Position, n int, prefix ...func(i int) string) (string, error) {
	var prefixFunc func(i int) string
	if len(prefix) > 0 && prefix[0] != nil {
		prefixFunc = prefix[0]
	}
	return ve.GetContextAroundRangeEx(startPos, endPos, n, prefixFunc, nil)
}

func (ve *MemEditor) GetContextAroundRangeEx(startPos, endPos *Position, n int, prefix func(i int) string, suffix func(i int) string) (string, error) {
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

func (ve *MemEditor) GetTextFromRangeContext(i *Range, lineNum int) string {
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

func (ve *MemEditor) GetSourceCode(index ...int) string {
	if len(index) == 0 {
		return ve.safeSourceCode.String()
	} else if len(index) == 1 {
		return ve.safeSourceCode.SliceBeforeStart(index[0])
	} else if len(index) >= 2 {
		return ve.safeSourceCode.Slice2(index[0], index[1])
	} else {
		log.Warnf("GetSourceCode: invalid index: %v", index)
		return ""
	}
}

func (e *MemEditor) GetTextContextWithPrompt(p *Range, n int, msg ...string) string {
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

// =============================================================================
// 编辑功能 - Edit Functions
// =============================================================================

// InsertAtPosition 在指定位置插入文本
func (ve *MemEditor) InsertAtPosition(pos *Position, text string) error {
	if pos == nil {
		return errors.New("position cannot be nil")
	}

	offset, err := ve.GetOffsetByPositionWithError(pos.GetLine(), pos.GetColumn())
	if err != nil {
		return err
	}

	return ve.InsertAtOffset(offset, text)
}

// InsertAtOffset 在指定偏移量处插入文本
func (ve *MemEditor) InsertAtOffset(offset int, text string) error {
	if offset < 0 || offset > ve.safeSourceCode.Len() {
		return errors.New("offset out of bounds")
	}

	before := ve.safeSourceCode.SliceBeforeStart(offset)
	after := ""
	if offset < ve.safeSourceCode.Len() {
		after = ve.safeSourceCode.Slice2(offset, ve.safeSourceCode.Len())
	}

	ve.safeSourceCode = NewSafeString(before + text + after)
	ve.recalculateLineMappings()

	return nil
}

// InsertAtLine 在指定行号的开头插入文本（行号从1开始）
func (ve *MemEditor) InsertAtLine(lineNumber int, text string) error {
	if lineNumber < 1 {
		return errors.New("line number must be positive")
	}

	// 如果行号超出范围，在最后添加新行
	if lineNumber > len(ve.lineStartOffsetMap) {
		// 添加到文件末尾，确保以换行符结尾
		sourceCode := ve.safeSourceCode.String()
		if !strings.HasSuffix(sourceCode, "\n") {
			sourceCode += "\n"
		}
		// 添加空行直到目标行号
		for i := len(ve.lineStartOffsetMap); i < lineNumber-1; i++ {
			sourceCode += "\n"
		}
		sourceCode += text
		ve.safeSourceCode = NewSafeString(sourceCode)
		ve.recalculateLineMappings()
		return nil
	}

	offset, err := ve.GetStartOffsetByLine(lineNumber)
	if err != nil {
		return err
	}

	return ve.InsertAtOffset(offset, text)
}

// ReplaceLine 替换指定行的内容（行号从1开始）
func (ve *MemEditor) ReplaceLine(lineNumber int, text string) error {
	if lineNumber < 1 {
		return errors.New("line number must be positive")
	}

	if lineNumber > len(ve.lineStartOffsetMap) {
		return errors.New("line number out of range")
	}

	startOffset, err := ve.GetStartOffsetByLine(lineNumber)
	if err != nil {
		return err
	}

	endOffset, err := ve.GetEndOffsetByLine(lineNumber)
	if err != nil {
		return err
	}

	before := ve.safeSourceCode.SliceBeforeStart(startOffset)
	after := ""
	if endOffset < ve.safeSourceCode.Len() {
		after = ve.safeSourceCode.Slice2(endOffset, ve.safeSourceCode.Len())
	}

	ve.safeSourceCode = NewSafeString(before + text + after)
	ve.recalculateLineMappings()

	return nil
}

// ReplaceLineRange 替换指定行范围的内容（行号从1开始，包含起始和结束行）
func (ve *MemEditor) ReplaceLineRange(startLine, endLine int, text string) error {
	if startLine < 1 || endLine < 1 {
		return errors.New("line numbers must be positive")
	}

	if startLine > endLine {
		return errors.New("start line must be less than or equal to end line")
	}

	if startLine > len(ve.lineStartOffsetMap) || endLine > len(ve.lineStartOffsetMap) {
		return errors.New("line number out of range")
	}

	startOffset, err := ve.GetStartOffsetByLine(startLine)
	if err != nil {
		return err
	}

	endOffset, err := ve.GetEndOffsetByLine(endLine)
	if err != nil {
		return err
	}

	before := ve.safeSourceCode.SliceBeforeStart(startOffset)
	after := ""
	if endOffset < ve.safeSourceCode.Len() {
		after = ve.safeSourceCode.Slice2(endOffset, ve.safeSourceCode.Len())
	}

	ve.safeSourceCode = NewSafeString(before + text + after)
	ve.recalculateLineMappings()

	return nil
}

// DeleteLine 删除指定行（行号从1开始）
func (ve *MemEditor) DeleteLine(lineNumber int) error {
	if lineNumber < 1 {
		return errors.New("line number must be positive")
	}

	if lineNumber > len(ve.lineStartOffsetMap) {
		return errors.New("line number out of range")
	}

	startOffset, err := ve.GetStartOffsetByLine(lineNumber)
	if err != nil {
		return err
	}

	// 对于最后一行，需要特殊处理
	if lineNumber == len(ve.lineStartOffsetMap) {
		// 如果是最后一行，删除到文件末尾
		before := ve.safeSourceCode.SliceBeforeStart(startOffset)
		// 如果前面有内容且不以换行符结尾，移除前面的换行符
		if len(before) > 0 && strings.HasSuffix(before, "\n") {
			before = before[:len(before)-1]
		}
		ve.safeSourceCode = NewSafeString(before)
	} else {
		// 不是最后一行，删除包括换行符
		endOffset, err := ve.GetEndOffsetByLine(lineNumber)
		if err != nil {
			return err
		}
		// 包括行末的换行符
		if endOffset < ve.safeSourceCode.Len() {
			endOffset++
		}

		before := ve.safeSourceCode.SliceBeforeStart(startOffset)
		after := ""
		if endOffset < ve.safeSourceCode.Len() {
			after = ve.safeSourceCode.Slice2(endOffset, ve.safeSourceCode.Len())
		}

		ve.safeSourceCode = NewSafeString(before + after)
	}

	ve.recalculateLineMappings()
	return nil
}

// DeleteLineRange 删除指定行范围（行号从1开始，包含起始和结束行）
func (ve *MemEditor) DeleteLineRange(startLine, endLine int) error {
	if startLine < 1 || endLine < 1 {
		return errors.New("line numbers must be positive")
	}

	if startLine > endLine {
		return errors.New("start line must be less than or equal to end line")
	}

	if startLine > len(ve.lineStartOffsetMap) || endLine > len(ve.lineStartOffsetMap) {
		return errors.New("line number out of range")
	}

	startOffset, err := ve.GetStartOffsetByLine(startLine)
	if err != nil {
		return err
	}

	var endOffset int
	// 对于最后一行，需要特殊处理
	if endLine == len(ve.lineStartOffsetMap) {
		endOffset = ve.safeSourceCode.Len()
		// 如果删除包含最后一行，需要删除前面的换行符
		if startLine > 1 && startOffset > 0 {
			startOffset--
		}
	} else {
		endOffset, err = ve.GetEndOffsetByLine(endLine)
		if err != nil {
			return err
		}
		// 包括行末的换行符
		endOffset++
	}

	before := ve.safeSourceCode.SliceBeforeStart(startOffset)
	after := ""
	if endOffset < ve.safeSourceCode.Len() {
		after = ve.safeSourceCode.Slice2(endOffset, ve.safeSourceCode.Len())
	}

	ve.safeSourceCode = NewSafeString(before + after)
	ve.recalculateLineMappings()

	return nil
}

// AppendLine 在文件末尾添加新行
func (ve *MemEditor) AppendLine(text string) error {
	sourceCode := ve.safeSourceCode.String()
	if !strings.HasSuffix(sourceCode, "\n") && sourceCode != "" {
		sourceCode += "\n"
	}
	sourceCode += text

	ve.safeSourceCode = NewSafeString(sourceCode)
	ve.recalculateLineMappings()

	return nil
}

// PrependLine 在文件开头添加新行
func (ve *MemEditor) PrependLine(text string) error {
	sourceCode := text + "\n" + ve.safeSourceCode.String()
	ve.safeSourceCode = NewSafeString(sourceCode)
	ve.recalculateLineMappings()

	return nil
}
