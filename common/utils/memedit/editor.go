package memedit

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	regexp2 "github.com/VillanCh/go-pcre2-lite/regexp2"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var (
	ErrorStop = errors.New("stop")
)

// lineMappings is an immutable line-number mapping snapshot (startOffsets and lens are published as a pair).
// The same MemEditor may be shared by multiple scan goroutines (ssadb.irSourceCache reuses
// editors by hash). Writing the two fields directly would let readers observe fields from
// different goroutines during concurrent lazy builds, causing an out-of-range panic in
// GetEndOffsetByLine. Build a local snapshot first, then publish it atomically via atomic.Pointer.
type lineMappings struct {
	startOffsets []int
	lens         []int
}

type MemEditor struct {
	sourceCodeCtxStack []string

	// hash
	sourceCodeMd5    string
	sourceCodeSha1   string
	sourceCodeSha256 string
	irSourceHash     string

	// fileUrl and source
	fileUrl string

	programName string
	folderPath  string
	fileName    string

	safeSourceCode *SafeString

	// editor
	lineMappings atomic.Pointer[lineMappings]
	cursor       int // simulated cursor position (pointer feature)

	// runeOffsetMap is the memoized rune→byte-offset map for the current
	// safeSourceCode. FileFilter (sf_prog.go) used to rebuild this per call
	// (NewRuneOffsetMap over the full source each time = ~71GB/20% of alloc on
	// javacms-core). Built lazily by GetRuneOffsetMap; nil'd by
	// invalidateSourceCodeState so a pushed/edited source rebuilds it.
	runeOffsetMap atomic.Pointer[RuneOffsetMap]
}

// NewMemEditorByBytes reuses the provided byte slice without copying it.
// Callers must treat bs as immutable after this call.
func NewMemEditorByBytes(bs []byte) *MemEditor {
	editor := &MemEditor{
		safeSourceCode: NewSafeString(bs),
	}
	return editor
}

// NewMemEditor creates an in-memory editor from a source string (exported as memeditor.New).
// It provides line/offset reads, search, and range location for source analysis and slicing.
//
// Args:
//   - sourceCode: source text to edit/analyze
//
// Returns:
//   - the in-memory editor, supporting GetLineCount / GetLine / GetSourceCode etc.
//
// Example:
// ```
// editor = memeditor.New("line1\nline2\nline3")
// println(editor.GetLineCount())   // OUT: 3
// assert editor.GetLineCount() == 3, "editor should report 3 lines"
// line2 = editor.GetLine(2)~
// assert line2 == "line2", "GetLine(2) should return the second line"
// ```
func NewMemEditor(sourceCode string) *MemEditor {
	editor := &MemEditor{
		safeSourceCode: NewSafeString(sourceCode),
	}
	return editor
}

func NewMemEditorWithFileUrl(sourceCode string, fileUrl string) *MemEditor {
	editor := NewMemEditor(sourceCode)
	editor.SetUrl(fileUrl)
	return editor
}

func (ve *MemEditor) ensureLineMappings() {
	ve.getLineMappings()
}

// getLineMappings returns the current line-mapping snapshot, lazily building it if needed.
// Callers must use only the returned snapshot and not mix snapshots across calls.
func (ve *MemEditor) getLineMappings() *lineMappings {
	if ve == nil {
		return nil
	}
	if lm := ve.lineMappings.Load(); lm != nil {
		return lm
	}
	ve.recalculateLineMappings()
	return ve.lineMappings.Load()
}

func (ve *MemEditor) invalidateLineMappings() {
	if ve == nil {
		return
	}
	ve.lineMappings.Store(nil)
}

func (ve *MemEditor) invalidateSourceCodeState() {
	if ve == nil {
		return
	}
	ve.ResetSourceCodeHash()
	ve.invalidateLineMappings()
	ve.runeOffsetMap.Store(nil)
}

// GetRuneOffsetMap returns the memoized rune→byte-offset map for the current
// source, building it lazily on first call. Use this instead of
// NewRuneOffsetMap(me.GetSourceCode()) in hot paths (e.g. FileFilter) so a
// file scanned N times pays the O(sourceLen) build once, not N times. The map
// is invalidated (nil'd) by invalidateSourceCodeState (which fires on actual
// source edits); PushSourceCodeContext only changes the hash salt, not the
// source bytes, so rune offsets stay valid across a push.
func (ve *MemEditor) GetRuneOffsetMap() *RuneOffsetMap {
	if ve == nil {
		return nil
	}
	if m := ve.runeOffsetMap.Load(); m != nil {
		return m
	}
	// GetSourceCodeUnsafe aliases safeSourceCode.bytes (zero-copy); the map
	// only reads it during build and stores offsets + length, not the string,
	// so we don't retain the alias beyond this call.
	m := NewRuneOffsetMap(ve.GetSourceCodeUnsafe())
	ve.runeOffsetMap.Store(m)
	return m
}

func (ve *MemEditor) CodeLength() int {
	return ve.safeSourceCode.Len()
}

func (ve *MemEditor) GetLineCount() int {
	lm := ve.getLineMappings()
	if lm == nil {
		return 0
	}
	return len(lm.lens)
}

func (ve *MemEditor) SetUrl(url string) {
	ve.ResetSourceCodeHash()
	ve.fileUrl = url
}

// GetUrl returns the file's full URL path: /programName/folderPath/fileName.
func (ve *MemEditor) GetUrl() string {
	// Ensure folderPath and fileName are initialized
	if ve.folderPath == "" && ve.fileUrl != "" {
		dir, name := path.Split(ve.fileUrl)
		ve.fileName = name
		ve.folderPath = ve.normalizeFolderPath(dir)
	}
	// Use the internal folderPath (without programName) instead of GetFolderPath() (which includes programName)
	// This avoids duplicating programName in the path
	parts := []string{"/", ve.GetProgramName()}
	if ve.folderPath != "" {
		parts = append(parts, ve.folderPath)
	}
	parts = append(parts, ve.GetFilename())
	return path.Join(parts...)
}

// GetFilePath returns the file path: /folderPath/fileName.
func (v *MemEditor) GetFilePath() string {
	return path.Join("/", v.GetFolderPath(), v.GetFilename())
}

func (ve *MemEditor) SetProgramName(programName string) {
	ve.ResetSourceCodeHash()
	ve.programName = programName
	// Re-normalize folderPath because a programName change may affect normalization
	if ve.folderPath != "" {
		ve.folderPath = ve.normalizeFolderPath(ve.folderPath)
	}
}

// GetProgramName returns the program name.
func (ve *MemEditor) GetProgramName() string {
	return ve.programName
}

// GetGlobalFolderPath returns the full path: programName/folderPath/.
// Used mainly for DB queries/storage: the path includes programName and ends with a slash.
func (ve *MemEditor) GetGlobalFolderPath() string {
	folderPath := ve.GetFolderPath()
	programName := ve.GetProgramName()

	if programName == "" {
		if folderPath == "" {
			return ""
		}
		// Ensure it ends with /
		if !strings.HasSuffix(folderPath, "/") {
			return folderPath + "/"
		}
		return folderPath
	}

	// programName is not empty
	ret := path.Join(programName, folderPath)
	// Ensure it ends with /
	if !strings.HasSuffix(ret, "/") {
		ret = ret + "/"
	}
	return ret
}

// JoinGlobalPath joins a path with the global directory.
// Used for resolving import paths and other cases needing a globally unique path.
func (ve *MemEditor) JoinGlobalPath(subPath string) string {
	// If subPath is already absolute or contains programName (assumed), special handling may be needed
	// but usually subPath is relative or a plain file name

	// Get the global directory path (with programName)
	// Note: no trailing / is needed here because path.Join handles it
	globalDir := ve.GetGlobalFolderPath()

	// If globalDir is empty, return subPath directly
	if globalDir == "" {
		return subPath
	}

	return path.Join(globalDir, subPath)
}

// JoinProgramPath joins a path with programName.
// Used to convert a root-relative path into a globally unique path (programName prefix).
func (ve *MemEditor) JoinProgramPath(subPath string) string {
	programName := ve.GetProgramName()
	if programName == "" {
		return subPath
	}
	return path.Join(programName, subPath)
}

// normalizeFolderPath normalizes folderPath: strip leading/trailing slashes and any programName prefix.
func (ve *MemEditor) normalizeFolderPath(folderPath string) string {
	// Strip leading slash
	folderPath = strings.TrimPrefix(folderPath, "/")

	// Strip trailing slash
	folderPath = strings.TrimSuffix(folderPath, "/")

	// If folderPath starts with "{programName}/", strip that prefix
	if ve.programName != "" {
		prefix := ve.programName + "/"
		if strings.HasPrefix(folderPath, prefix) {
			folderPath = strings.TrimPrefix(folderPath, prefix)
		} else if folderPath == ve.programName {
			// If folderPath equals programName exactly, it is an empty path
			folderPath = ""
		}
	}

	return folderPath
}

func (ve *MemEditor) SetFolderPath(folderPath string) {
	ve.ResetSourceCodeHash()
	ve.folderPath = ve.normalizeFolderPath(folderPath)
}

// GetFolderPath returns the clean folder path (without programName).
func (ve *MemEditor) GetFolderPath() string {
	if ve.folderPath == "" && ve.fileUrl != "" {
		// split from ve.GetUrl
		dir, name := path.Split(ve.fileUrl)
		ve.fileName = name
		ve.folderPath = ve.normalizeFolderPath(dir)
	}
	return ve.folderPath
}

func (ve *MemEditor) SetFileName(fileName string) {
	ve.ResetSourceCodeHash()
	ve.fileName = fileName
}

// GetFilename returns the file name.
func (ve *MemEditor) GetFilename() string {
	if ve.fileName == "" && ve.fileUrl != "" {
		// split from ve.GetUrl
		dir, name := path.Split(ve.fileUrl)
		ve.folderPath = ve.normalizeFolderPath(dir)
		ve.fileName = name
	}
	return ve.fileName
}

// GetIrSourceHash hashes the program name, path, and source code.
// Format: programName + "/" + folderPath + "/" + fileName + "|" + sourceCode
// Note: folderPath is already normalized (no programName prefix, no leading/trailing slashes).
func (ve *MemEditor) GetIrSourceHash() string {
	if ve == nil {
		return ""
	}
	if ve.irSourceHash != "" {
		return ve.irSourceHash
	}
	programName := ve.GetProgramName()
	// Ensure initialization
	if ve.folderPath == "" && ve.fileUrl != "" {
		ve.GetFolderPath()
	}
	folderPath := ve.folderPath // use the internal normalized path
	fileName := ve.GetFilename()

	var sourceBytes []byte
	if ve.safeSourceCode != nil {
		sourceBytes = ve.safeSourceCode.Bytes()
	}
	// Empty source returns an empty string
	if programName == "" && folderPath == "" && fileName == "" && len(sourceBytes) == 0 {
		return ""
	}

	// Compute md5(programName + "/" + folderPath + "/" + fileName + "|" + sourceCode)
	// without allocating the full concatenated string.
	h := md5.New()
	_, _ = h.Write(utils.UnsafeStringToBytes(programName))
	_, _ = h.Write([]byte{'/'})
	if folderPath != "" {
		_, _ = h.Write(utils.UnsafeStringToBytes(folderPath))
		_, _ = h.Write([]byte{'/'})
	}
	_, _ = h.Write(utils.UnsafeStringToBytes(fileName))
	_, _ = h.Write([]byte{'|'})
	if len(sourceBytes) > 0 {
		_, _ = h.Write(sourceBytes)
	}
	ve.irSourceHash = hex.EncodeToString(h.Sum(nil))
	return ve.irSourceHash
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
	lm := ve.getLineMappings()
	if line < 1 || col < 1 {
		return 0, errors.New("line number and column number must be positive")
	}

	// Adjust line to an internal 0-based index
	adjustedLine := line - 1
	adjustedCol := col - 1

	// Check whether the line number is out of range
	if lm == nil || adjustedLine >= len(lm.startOffsets) || adjustedLine >= len(lm.lens) {
		return ve.safeSourceCode.Len(), errors.New("line number out of range")
	}

	// Check whether the column is beyond the current line length
	if adjustedCol > lm.lens[adjustedLine] {
		adjustedCol = lm.lens[adjustedLine] // Clamp the column to the maximum length of the line
	}

	lineStartOffset := lm.startOffsets[adjustedLine]
	if adjustedLine < len(lm.lens)-1 {
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
	lm := ve.getLineMappings()
	if x < 1 {
		return 0, errors.New("line number should be positive")
	}

	x = x - 1
	if lm == nil || x >= len(lm.startOffsets) {
		return 0, errors.New("line number out of range")
	}

	return lm.startOffsets[x], nil
}

func (ve *MemEditor) GetEndOffsetByLine(x int) (int, error) {
	lm := ve.getLineMappings()
	if x < 1 {
		return 0, errors.New("line number should be positive")
	}

	x = x - 1
	if lm == nil || x >= len(lm.startOffsets) || x >= len(lm.lens) {
		return 0, errors.New("line number out of range")
	}

	return lm.startOffsets[x] + lm.lens[x], nil
}

// GetLine returns the content of the given line.
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

// Select returns the text of the given range.
func (ve *MemEditor) Select(start, end int) (string, error) {
	if start < 0 || end > ve.safeSourceCode.Len() || start > end {
		return "", errors.New("invalid range for select")
	}
	return ve.safeSourceCode.Slice2(start, end), nil
}

// CompareText compares the range text with the given string.
func (ve *MemEditor) CompareRangeWithString(start, end int, compareTo string) (bool, error) {
	selectedText, err := ve.Select(start, end)
	if err != nil {
		return false, err
	}
	return selectedText == compareTo, nil
}

// MoveCursor moves the simulated cursor position.
func (ve *MemEditor) MoveCursor(position int) error {
	if position < 0 || position > ve.safeSourceCode.Len() {
		return errors.New("position out of bounds")
	}
	ve.cursor = position
	return nil
}

// GetCurrentLine returns the content of the line at the cursor.
func (ve *MemEditor) GetCurrentLine() (string, error) {
	lm := ve.getLineMappings()
	if lm == nil {
		return "", errors.New("current position is out of the source code range")
	}
	for lineNumber, startOffset := range lm.startOffsets {
		if lineNumber < len(lm.lens) && ve.cursor >= startOffset && ve.cursor <= (startOffset+lm.lens[lineNumber]) {
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
	lm := ve.getLineMappings()
	if lm == nil || len(lm.startOffsets) == 0 || len(lm.lens) == 0 {
		return NewPosition(1, 1), errors.New("empty source editor")
	}
	if offset < 0 {
		// Negative offset returns the initial position
		return NewPosition(1, 1), errors.New("offset is negative")
	}
	if offset >= ve.safeSourceCode.Len() {
		// Offset beyond the maximum returns the last position
		lastLine := len(lm.startOffsets) - 1 // index of the last line (0-based)
		if lastLine < 0 || lastLine >= len(lm.lens) {
			return NewPosition(1, 1), utils.Errorf("offset %d is out of range", offset)
		}
		lastLineStart := lm.startOffsets[lastLine]
		lastLineLen := lm.lens[lastLine]
		outOfRange := utils.Errorf("offset %d is out of range", offset)
		if offset == ve.safeSourceCode.Len() && lastLineLen == 0 {
			// Special case: the last line has no content
			return NewPosition(lastLine+1, 1), outOfRange
		}
		return NewPosition(lastLine+1, utils.Min(offset-lastLineStart, lastLineLen)+1), outOfRange
	}

	// Locate the line with binary search
	low, high := 0, len(lm.startOffsets)-1
	for low <= high {
		mid := low + (high-low)/2
		startOffset := lm.startOffsets[mid]

		if startOffset == offset {
			return NewPosition(mid+1, 1), nil
		} else if startOffset < offset {
			if mid == high || mid+1 >= len(lm.startOffsets) || lm.startOffsets[mid+1] > offset {
				column := offset - startOffset
				return NewPosition(mid+1, column+1), nil
			}
			low = mid + 1
		} else {
			high = mid - 1
		}
	}

	// Should never be reached
	return NewPosition(1, 1), errors.New("position not found")
}

func (ve *MemEditor) GetRangeOffset(start, end int) *Range {
	start, end = ClampOffsetPair(ve, start, end)
	return NewRangeFromOffsets(ve, start, end)
}

func (ve *MemEditor) GetRangeByPosition(start, end *Position) *Range {
	if ve == nil || start == nil || end == nil {
		return NewRange(start, end)
	}
	// Use best-effort offsets even when line/column is slightly out of range.
	// Clients (e.g. Monaco/LSP) may send stale end positions; clamp instead of dropping offsets.
	startOff := ve.GetOffsetByPositionRaw(start.GetLine(), start.GetColumn())
	endOff := ve.GetOffsetByPositionRaw(end.GetLine(), end.GetColumn())
	return NewRangeFromOffsets(ve, startOff, endOff)
}

func (ve *MemEditor) GetFullRange() *Range {
	return ve.GetRangeOffset(0, ve.safeSourceCode.Len())
}

// GetTextFromRangeWithError gets range text, preferring Offset and falling back to Line/Column.
func (ve *MemEditor) GetTextFromRangeWithError(r *Range) (string, error) {
	start := r.GetStart()
	end := r.GetEnd()

	var startOffset, endOffset int
	// Compute Offset from Line and Column
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

// UpdateTextByRange updates text by range, preferring Offset and falling back to Line/Column.
func (ve *MemEditor) UpdateTextByRange(r *Range, newText string) error {
	start := r.GetStart()
	end := r.GetEnd()

	var startOffset, endOffset int
	var err error
	// Compute Offset from Line and Column
	startOffset, err = ve.GetOffsetByPositionWithError(start.GetLine(), start.GetColumn())
	if err != nil {
		return err // offset computation failed
	}
	endOffset, err = ve.GetOffsetByPositionWithError(end.GetLine(), end.GetColumn())
	if err != nil {
		return err // offset computation failed
	}

	// Check whether the offset range is valid
	if startOffset > endOffset {
		return errors.New("start position is after end position")
	}
	if endOffset > ve.safeSourceCode.Len() {
		return errors.New("end offset is out of bounds")
	}

	// Use safe string splitting to prevent out-of-range access
	before := ve.safeSourceCode.SliceBeforeStart(startOffset) // text before the start offset
	after := ""                                               // empty tail by default
	if endOffset < ve.safeSourceCode.Len() {
		after = ve.safeSourceCode.Slice2(endOffset, ve.safeSourceCode.Len()) // text after the end offset
	}

	ve.safeSourceCode = NewSafeString(before + newText + after) // build the new source

	// Update the line mapping
	ve.invalidateSourceCodeState()
	return nil
}

func (ve *MemEditor) ResetSourceCodeHash() {
	if ve == nil {
		return
	}
	ve.sourceCodeMd5 = ""
	ve.sourceCodeSha1 = ""
	ve.sourceCodeSha256 = ""
	ve.irSourceHash = ""
}

// recalculateLineMappings recomputes the line mappings.
func (ve *MemEditor) recalculateLineMappings() {
	data := ve.safeSourceCode.Bytes()
	lineNums := bytes.Count(data, []byte{'\n'}) + 1

	lm := &lineMappings{
		lens:         make([]int, 0, lineNums),
		startOffsets: make([]int, 1, lineNums),
	}
	currentOffset := 0
	currentLineLen := 0

	if len(data) == 0 {
		lm.lens = append(lm.lens, 0)
		ve.lineMappings.Store(lm)
		return
	}

	if ve.safeSourceCode.isASCII() || !ve.safeSourceCode.utf8Valid {
		for _, b := range data {
			if b == '\n' {
				lm.lens = append(lm.lens, currentLineLen)
				currentOffset++
				lm.startOffsets = append(lm.startOffsets, currentOffset)
				currentLineLen = 0
				continue
			}
			currentLineLen++
			currentOffset++
		}
	} else {
		for len(data) > 0 {
			r, size := utf8.DecodeRune(data)
			data = data[size:]
			if r == '\n' {
				lm.lens = append(lm.lens, currentLineLen)
				currentOffset++
				lm.startOffsets = append(lm.startOffsets, currentOffset)
				currentLineLen = 0
				continue
			}
			currentLineLen++
			currentOffset++
		}
	}
	lm.lens = append(lm.lens, currentLineLen)
	ve.lineMappings.Store(lm)
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
	// Extend the start offset back to the previous word boundary
	startWordOffset := startOffset
	for startWordOffset > 0 && !ve.boundary(rune(ve.safeSourceCode.Slice1(startWordOffset-1))) {
		startWordOffset--
	}

	// Extend the end offset forward to the next word boundary
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

// GetWordWithPointAtPosition gets the word at the cursor; if preceded by '.', it also tries to include the previous word.
// e.g. at 'd' in a.b.c.d it returns (c.d, startPos, endPos).
// Mirrors getWordWithPointAtPosition in the yakit Monaco Editor.
func (ve *MemEditor) GetWordWithPointAtPosition(position *Position) (word string, start *Position, end *Position) {
	if position == nil {
		return "", nil, nil
	}

	// Get the word range at the current position
	wordRange := ve.GetRangeByPosition(position, position)
	wordRange = ve.ExpandWordTextRange(wordRange)

	word = wordRange.GetText()
	start = wordRange.GetStart()
	end = wordRange.GetEnd()

	// If start column > 0, check whether the previous char is '.'
	if start.GetColumn() > 0 {
		prevCharOffset, err := ve.GetOffsetByPositionWithError(start.GetLine(), start.GetColumn()-1)
		if err == nil && prevCharOffset >= 0 && prevCharOffset < ve.safeSourceCode.Len() {
			prevChar := ve.safeSourceCode.Slice1(prevCharOffset)

			if prevChar == '.' {
				// Previous char is '.', try to find the previous word
				if start.GetColumn() >= 2 {
					prevWordPos := ve.GetPositionByLine(start.GetLine(), start.GetColumn()-2)
					prevWordRange := ve.GetRangeByPosition(prevWordPos, prevWordPos)
					prevWordRange = ve.ExpandWordTextRange(prevWordRange)
					prevWord := prevWordRange.GetText()

					if prevWord != "" {
						// Join previous word + "." + current word
						word = prevWord + "." + word
						start = prevWordRange.GetStart()
					} else {
						// Only "." + current word
						word = "." + word
						start = ve.GetPositionByLine(start.GetLine(), start.GetColumn()-1)
					}
				} else {
					// Only "." + current word
					word = "." + word
					start = ve.GetPositionByLine(start.GetLine(), start.GetColumn()-1)
				}
			}
		}
	}

	return word, start, end
}

func (ve *MemEditor) IsOffsetValid(offset int) bool {
	return offset >= 0 && offset <= ve.safeSourceCode.Len()
}

func (ve *MemEditor) IsValidPosition(line, col int) bool {
	lm := ve.getLineMappings()
	if line < 1 || col < 0 {
		return false
	}
	adjustedLine := line - 1
	if lm == nil || adjustedLine >= len(lm.startOffsets) || adjustedLine >= len(lm.lens) {
		return false
	}
	return col <= lm.lens[adjustedLine]
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
		return err // regexp compilation failed
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
			return err // abort early if the callback fails
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
	lm := ve.getLineMappings()
	if lm == nil {
		return "", errors.New("empty source editor")
	}
	start, end := ve.GetMinAndMaxOffset(startPos, endPos)
	if start < 0 || end > ve.safeSourceCode.Len() || start > end {
		return "", errors.New("invalid range")
	}

	startLine, _ := ve.GetPositionByOffsetWithError(start)
	endLine, _ := ve.GetPositionByOffsetWithError(end)

	startContextLine := utils.Max(startLine.GetLine()-n, 1)
	endContextLine := utils.Min(endLine.GetLine()+n, len(lm.startOffsets))

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

// GetSourceCodeUnsafe returns the full source code without allocating a new string.
//
// WARNING: The returned string aliases internal buffers. It is intended for
// read-only, performance-sensitive paths (e.g. scanning/matching). Do not use it
// if the underlying editor may be mutated concurrently.
func (ve *MemEditor) GetSourceCodeUnsafe() string {
	if ve == nil || ve.safeSourceCode == nil {
		return ""
	}
	return utils.UnsafeBytesToString(ve.safeSourceCode.Bytes())
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
// Edit functions
// =============================================================================

// InsertAtPosition inserts text at the given position.
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

// InsertAtOffset inserts text at the given offset.
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
	ve.invalidateSourceCodeState()

	return nil
}

// InsertAtLine inserts text at the start of the given line (1-based).
func (ve *MemEditor) InsertAtLine(lineNumber int, text string) error {
	lm := ve.getLineMappings()
	if lineNumber < 1 {
		return errors.New("line number must be positive")
	}
	if lm == nil {
		return errors.New("empty source editor")
	}

	// If the line is out of range, append a new line at the end
	if lineNumber > len(lm.startOffsets) {
		// Append at the end of the file, ensuring a trailing newline
		sourceCode := ve.safeSourceCode.String()
		if !strings.HasSuffix(sourceCode, "\n") {
			sourceCode += "\n"
		}
		// Add empty lines until the target line number
		for i := len(lm.startOffsets); i < lineNumber-1; i++ {
			sourceCode += "\n"
		}
		sourceCode += text
		ve.safeSourceCode = NewSafeString(sourceCode)
		ve.invalidateSourceCodeState()
		return nil
	}

	offset, err := ve.GetStartOffsetByLine(lineNumber)
	if err != nil {
		return err
	}

	return ve.InsertAtOffset(offset, text)
}

// ReplaceLine replaces the content of the given line (1-based).
func (ve *MemEditor) ReplaceLine(lineNumber int, text string) error {
	lm := ve.getLineMappings()
	if lineNumber < 1 {
		return errors.New("line number must be positive")
	}
	if lm == nil {
		return errors.New("empty source editor")
	}

	if lineNumber > len(lm.startOffsets) {
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
	ve.invalidateSourceCodeState()

	return nil
}

// ReplaceLineRange replaces the content of a line range (1-based, inclusive).
func (ve *MemEditor) ReplaceLineRange(startLine, endLine int, text string) error {
	lm := ve.getLineMappings()
	if startLine < 1 || endLine < 1 {
		return errors.New("line numbers must be positive")
	}
	if lm == nil {
		return errors.New("empty source editor")
	}

	if startLine > endLine {
		return errors.New("start line must be less than or equal to end line")
	}

	if startLine > len(lm.startOffsets) || endLine > len(lm.startOffsets) {
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
	ve.invalidateSourceCodeState()

	return nil
}

// DeleteLine deletes the given line (1-based).
func (ve *MemEditor) DeleteLine(lineNumber int) error {
	lm := ve.getLineMappings()
	if lineNumber < 1 {
		return errors.New("line number must be positive")
	}
	if lm == nil {
		return errors.New("empty source editor")
	}

	if lineNumber > len(lm.startOffsets) {
		return errors.New("line number out of range")
	}

	startOffset, err := ve.GetStartOffsetByLine(lineNumber)
	if err != nil {
		return err
	}

	// The last line needs special handling
	if lineNumber == len(lm.startOffsets) {
		// If it is the last line, delete to the end of the file
		before := ve.safeSourceCode.SliceBeforeStart(startOffset)
		// If there is preceding content without a trailing newline, remove the preceding newline
		if len(before) > 0 && strings.HasSuffix(before, "\n") {
			before = before[:len(before)-1]
		}
		ve.safeSourceCode = NewSafeString(before)
	} else {
		// Not the last line: delete including the newline
		endOffset, err := ve.GetEndOffsetByLine(lineNumber)
		if err != nil {
			return err
		}
		// Include the trailing newline
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

	ve.invalidateSourceCodeState()
	return nil
}

// DeleteLineRange deletes a line range (1-based, inclusive).
func (ve *MemEditor) DeleteLineRange(startLine, endLine int) error {
	lm := ve.getLineMappings()
	if startLine < 1 || endLine < 1 {
		return errors.New("line numbers must be positive")
	}
	if lm == nil {
		return errors.New("empty source editor")
	}

	if startLine > endLine {
		return errors.New("start line must be less than or equal to end line")
	}

	if startLine > len(lm.startOffsets) || endLine > len(lm.startOffsets) {
		return errors.New("line number out of range")
	}

	startOffset, err := ve.GetStartOffsetByLine(startLine)
	if err != nil {
		return err
	}

	var endOffset int
	// The last line needs special handling
	if endLine == len(lm.startOffsets) {
		endOffset = ve.safeSourceCode.Len()
		// If the deletion includes the last line, also remove the preceding newline
		if startLine > 1 && startOffset > 0 {
			startOffset--
		}
	} else {
		endOffset, err = ve.GetEndOffsetByLine(endLine)
		if err != nil {
			return err
		}
		// Include the trailing newline
		endOffset++
	}

	before := ve.safeSourceCode.SliceBeforeStart(startOffset)
	after := ""
	if endOffset < ve.safeSourceCode.Len() {
		after = ve.safeSourceCode.Slice2(endOffset, ve.safeSourceCode.Len())
	}

	ve.safeSourceCode = NewSafeString(before + after)
	ve.invalidateSourceCodeState()

	return nil
}

// AppendLine appends a new line at the end of the file.
func (ve *MemEditor) AppendLine(text string) error {
	sourceCode := ve.safeSourceCode.String()
	if !strings.HasSuffix(sourceCode, "\n") && sourceCode != "" {
		sourceCode += "\n"
	}
	sourceCode += text

	ve.safeSourceCode = NewSafeString(sourceCode)
	ve.invalidateSourceCodeState()

	return nil
}

// PrependLine prepends a new line at the start of the file.
func (ve *MemEditor) PrependLine(text string) error {
	sourceCode := text + "\n" + ve.safeSourceCode.String()
	ve.safeSourceCode = NewSafeString(sourceCode)
	ve.invalidateSourceCodeState()

	return nil
}
