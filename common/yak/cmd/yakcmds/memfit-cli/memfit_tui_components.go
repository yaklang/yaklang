package memfitcli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

const memfitMaxTranscriptBlocks = 200

type memfitCompletionKind int

const (
	memfitCompletionCommand memfitCompletionKind = iota
	memfitCompletionFile
	memfitCompletionDirectory
)

type memfitCompletion struct {
	kind        memfitCompletionKind
	value       string
	description string
}

type memfitEditorLayout struct {
	lines        []string
	cursorRow    int
	cursorColumn int
	firstRow     int
	totalRows    int
}

type memfitTranscriptBlock struct {
	title string
	body  []string
	color string
}

type memfitEditorPosition struct {
	row    int
	column int
}

var memfitCommandCompletions = []memfitCompletion{
	{kind: memfitCompletionCommand, value: "/help", description: "show commands and shortcuts"},
	{kind: memfitCompletionCommand, value: "/status", description: "show session and worker state"},
	{kind: memfitCompletionCommand, value: "/mode yolo", description: "allow actions automatically"},
	{kind: memfitCompletionCommand, value: "/mode ai", description: "let the model decide reviews"},
	{kind: memfitCompletionCommand, value: "/mode manual", description: "ask before reviewed actions"},
	{kind: memfitCompletionCommand, value: "/process", description: "toggle selected activity"},
	{kind: memfitCompletionCommand, value: "/process all", description: "show earlier activities"},
	{kind: memfitCompletionCommand, value: "/thinking", description: "toggle the latest thinking"},
	{kind: memfitCompletionCommand, value: "/history", description: "browse conversation history"},
	{kind: memfitCompletionCommand, value: "/history latest", description: "return to the latest message"},
	{kind: memfitCompletionCommand, value: "/queue", description: "show queued messages"},
	{kind: memfitCompletionCommand, value: "/queue clear", description: "discard queued messages"},
	{kind: memfitCompletionCommand, value: "/queue resume", description: "continue a paused queue"},
	{kind: memfitCompletionCommand, value: "/logs", description: "show worker diagnostics"},
	{kind: memfitCompletionCommand, value: "/clear", description: "clear the current view"},
	{kind: memfitCompletionCommand, value: "/exit", description: "end this session"},
}

func (ui *memfitTUI) completionInputSignature() uint64 {
	// Keep only a compact change detector: the editor should not retain another
	// copy of a prompt that may itself contain credentials or other secrets.
	const (
		offset64 = uint64(1469598103934665603)
		prime64  = uint64(1099511628211)
	)
	signature := offset64
	for _, value := range ui.buffer {
		signature ^= uint64(value)
		signature *= prime64
	}
	signature ^= uint64(ui.cursor + 1)
	signature *= prime64
	return signature
}

func (ui *memfitTUI) refreshCompletions() {
	signature := ui.completionInputSignature()
	if signature == ui.completionSignature {
		return
	}
	selected := ""
	if ui.completionIndex >= 0 && ui.completionIndex < len(ui.completions) {
		selected = ui.completions[ui.completionIndex].value
	}
	ui.completionSignature = signature
	ui.completionIndex = 0
	ui.completionStart = ui.cursor
	ui.completionEnd = ui.cursor
	ui.completions = nil

	kind, query, start, ok := memfitCompletionContext(ui.buffer, ui.cursor)
	if !ok {
		return
	}
	ui.completionStart, ui.completionEnd = start, ui.cursor
	switch kind {
	case memfitCompletionCommand:
		query = strings.ToLower(query)
		for _, completion := range memfitCommandCompletions {
			if strings.HasPrefix(strings.ToLower(completion.value), query) {
				ui.completions = append(ui.completions, completion)
			}
		}
	case memfitCompletionFile:
		ui.completions = memfitMentionCompletions(ui.config.Workdir, query)
	}
	for index := range ui.completions {
		if ui.completions[index].value == selected {
			ui.completionIndex = index
			break
		}
	}
}

func memfitCompletionContext(buffer []rune, cursor int) (memfitCompletionKind, string, int, bool) {
	if cursor < 0 || cursor > len(buffer) {
		return 0, "", 0, false
	}
	before := string(buffer[:cursor])
	if !strings.Contains(before, "\n") && strings.HasPrefix(before, "/") {
		return memfitCompletionCommand, before, 0, true
	}
	start := cursor
	for start > 0 {
		if unicode.IsSpace(buffer[start-1]) && !memfitEscapedRune(buffer, start-1) {
			break
		}
		start--
	}
	if start >= cursor || buffer[start] != '@' {
		return 0, "", 0, false
	}
	query := strings.ReplaceAll(string(buffer[start+1:cursor]), `\ `, " ")
	return memfitCompletionFile, query, start, true
}

func memfitEscapedRune(buffer []rune, index int) bool {
	backslashes := 0
	for index > 0 && buffer[index-1] == '\\' {
		backslashes++
		index--
	}
	return backslashes%2 == 1
}

func memfitMentionCompletions(workdir, query string) []memfitCompletion {
	if workdir == "" {
		return nil
	}
	query = filepath.ToSlash(strings.TrimSpace(query))
	clean := filepath.Clean(filepath.FromSlash(query))
	if clean == "." {
		clean = ""
	}
	directory, prefix := filepath.Split(clean)
	directory = filepath.Clean(directory)
	if directory == "." {
		directory = ""
	}
	root, err := filepath.Abs(workdir)
	if err != nil {
		return nil
	}
	base := filepath.Join(root, directory)
	rel, err := filepath.Rel(root, base)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	if resolvedRoot, rootErr := filepath.EvalSymlinks(root); rootErr == nil {
		if resolvedBase, baseErr := filepath.EvalSymlinks(base); baseErr == nil {
			resolvedRel, resolvedErr := filepath.Rel(resolvedRoot, resolvedBase)
			if resolvedErr != nil || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) {
				return nil
			}
		}
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	prefixLower := strings.ToLower(prefix)
	result := make([]memfitCompletion, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == ".git" || name == "node_modules" || name == "vendor" {
			continue
		}
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(name), prefixLower) {
			continue
		}
		path := filepath.ToSlash(filepath.Join(directory, name))
		kind, description := memfitCompletionFile, "file"
		if entry.IsDir() {
			kind, description = memfitCompletionDirectory, "directory"
			path += "/"
		}
		result = append(result, memfitCompletion{
			kind:        kind,
			value:       "@" + strings.ReplaceAll(path, " ", `\ `),
			description: description,
		})
	}
	sort.SliceStable(result, func(left, right int) bool {
		if result[left].kind == result[right].kind {
			return strings.ToLower(result[left].value) < strings.ToLower(result[right].value)
		}
		return result[left].kind == memfitCompletionDirectory
	})
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

func (ui *memfitTUI) selectCompletion(delta int) {
	if len(ui.completions) == 0 {
		return
	}
	ui.completionIndex = (ui.completionIndex + delta + len(ui.completions)) % len(ui.completions)
}

func (ui *memfitTUI) acceptCompletion() (memfitCompletionKind, bool) {
	if ui.completionIndex < 0 || ui.completionIndex >= len(ui.completions) {
		return 0, false
	}
	completion := ui.completions[ui.completionIndex]
	replacement := []rune(completion.value)
	buffer := make([]rune, 0, len(ui.buffer)-(ui.completionEnd-ui.completionStart)+len(replacement)+1)
	buffer = append(buffer, ui.buffer[:ui.completionStart]...)
	buffer = append(buffer, replacement...)
	newCursor := len(buffer)
	buffer = append(buffer, ui.buffer[ui.completionEnd:]...)
	if completion.kind == memfitCompletionFile && (newCursor == len(buffer) || !unicode.IsSpace(buffer[newCursor])) {
		buffer = append(buffer[:newCursor], append([]rune{' '}, buffer[newCursor:]...)...)
		newCursor++
	}
	ui.buffer = buffer
	ui.cursor = newCursor
	ui.completions = nil
	ui.completionIndex = 0
	ui.completionSignature = ui.completionInputSignature()
	if completion.kind == memfitCompletionDirectory {
		ui.completionSignature = 0
		ui.refreshCompletions()
	}
	return completion.kind, true
}

func (ui *memfitTUI) completionPanelLines() []memfitLiveLine {
	if len(ui.completions) == 0 {
		return nil
	}
	visible := 5
	if ui.height < 16 || ui.width < 40 {
		visible = 3
	}
	if visible > len(ui.completions) {
		visible = len(ui.completions)
	}
	start := ui.completionIndex - visible + 1
	if start < 0 {
		start = 0
	}
	if start+visible > len(ui.completions) {
		start = len(ui.completions) - visible
	}
	title := "Commands"
	if ui.completions[0].kind != memfitCompletionCommand {
		title = "Files"
	}
	header := "╭─ " + title + " · ↑↓ choose · Tab insert"
	if ui.width < 42 {
		header = "╭─ " + title + " · ↑↓ · Tab"
	}
	lines := []memfitLiveLine{{text: header, color: memfitColorDim}}
	for index := start; index < start+visible; index++ {
		completion := ui.completions[index]
		prefix, color := "│   ", memfitColorDim
		if index == ui.completionIndex {
			prefix, color = "│ › ", memfitColorBold+memfitColorCyan
		}
		available := maxInt(8, ui.width-runewidth.StringWidth(prefix)-1)
		value := completion.value
		if ui.width >= 52 && completion.description != "" {
			value += "  " + completion.description
		}
		lines = append(lines, memfitLiveLine{text: prefix + truncateMemfitCells(value, available), color: color})
	}
	footer := fmt.Sprintf("╰─ %d match", len(ui.completions))
	if len(ui.completions) != 1 {
		footer += "es"
	}
	lines = append(lines, memfitLiveLine{text: footer, color: memfitColorDim})
	return lines
}

func layoutMemfitEditor(buffer []rune, cursor, width, maxRows int) memfitEditorLayout {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(buffer) {
		cursor = len(buffer)
	}
	if maxRows < 1 {
		maxRows = 1
	}
	innerWidth := maxInt(4, width-3)
	rows := []strings.Builder{{}}
	row, column := 0, 0
	cursorPosition := memfitEditorPosition{}
	for index, value := range buffer {
		if index == cursor {
			cursorPosition = memfitEditorPosition{row: row, column: column}
		}
		if value == '\n' {
			row++
			rows = append(rows, strings.Builder{})
			column = 0
			continue
		}
		piece := string(value)
		if value == '\t' {
			piece = "    "
		} else if unicode.IsControl(value) {
			piece = "�"
		}
		pieceWidth := maxInt(1, runewidth.StringWidth(piece))
		if column > 0 && column+pieceWidth > innerWidth {
			row++
			rows = append(rows, strings.Builder{})
			column = 0
			if index == cursor {
				cursorPosition = memfitEditorPosition{row: row, column: column}
			}
		}
		rows[row].WriteString(piece)
		column += pieceWidth
	}
	if cursor == len(buffer) {
		cursorPosition = memfitEditorPosition{row: row, column: column}
	}
	totalRows := len(rows)
	firstRow := cursorPosition.row - maxRows + 1
	if firstRow < 0 {
		firstRow = 0
	}
	if firstRow+maxRows > totalRows {
		firstRow = maxInt(0, totalRows-maxRows)
	}
	lastRow := minMemfitInt(totalRows, firstRow+maxRows)
	visible := make([]string, 0, lastRow-firstRow)
	for index := firstRow; index < lastRow; index++ {
		visible = append(visible, rows[index].String())
	}
	return memfitEditorLayout{
		lines:        visible,
		cursorRow:    cursorPosition.row - firstRow,
		cursorColumn: cursorPosition.column,
		firstRow:     firstRow,
		totalRows:    totalRows,
	}
}

func (ui *memfitTUI) editorMaxRows() int {
	switch {
	case ui.height < 10:
		return 1
	case ui.height < 16:
		return 2
	case ui.height >= 32:
		return 6
	default:
		return 4
	}
}

func (ui *memfitTUI) editorTitle(layout memfitEditorLayout) string {
	title := "Message"
	if ui.awaitingInput {
		title = "Reply required"
	} else if ui.busy {
		title = "Queued message"
	}
	if layout.totalRows > len(layout.lines) {
		title += fmt.Sprintf(" · lines %d–%d/%d", layout.firstRow+1, layout.firstRow+len(layout.lines), layout.totalRows)
	}
	return title
}

func (ui *memfitTUI) editorLines() ([]memfitLiveLine, int, int) {
	prompt := "❯ "
	if ui.awaitingInput {
		prompt = "reply ❯ "
	} else if ui.busy {
		prompt = "queue ❯ "
	}
	promptWidth := runewidth.StringWidth(prompt)
	layout := layoutMemfitEditor(ui.buffer, ui.cursor, ui.width-promptWidth, ui.editorMaxRows())
	title := ui.editorTitle(layout)
	top := "╭─ " + title + " "
	top += strings.Repeat("─", maxInt(0, ui.width-1-runewidth.StringWidth(top)))
	lines := []memfitLiveLine{{text: top, color: memfitColorDim}}
	for index, line := range layout.lines {
		color := ""
		linePrompt := strings.Repeat(" ", promptWidth)
		if layout.firstRow+index == 0 {
			linePrompt = prompt
		}
		if len(ui.buffer) == 0 && index == 0 {
			line = "Type a message · @ files · / commands"
			if ui.width < 36 {
				line = "Type a message · @ · /"
			}
			color = memfitColorDim
		}
		lines = append(lines, memfitLiveLine{
			text:  "│ " + linePrompt + truncateMemfitCells(line, maxInt(4, ui.width-3-promptWidth)),
			color: color,
		})
	}
	bottom := "╰" + strings.Repeat("─", maxInt(1, ui.width-2))
	lines = append(lines, memfitLiveLine{text: bottom, color: memfitColorDim})
	return lines, 1 + layout.cursorRow, 2 + promptWidth + layout.cursorColumn
}

func memfitMoveCursorLine(buffer []rune, cursor, direction int) (int, bool) {
	if cursor < 0 || cursor > len(buffer) || direction == 0 {
		return cursor, false
	}
	lineStart := cursor
	for lineStart > 0 && buffer[lineStart-1] != '\n' {
		lineStart--
	}
	column := cursor - lineStart
	if direction < 0 {
		if lineStart == 0 {
			return cursor, false
		}
		previousEnd := lineStart - 1
		previousStart := previousEnd
		for previousStart > 0 && buffer[previousStart-1] != '\n' {
			previousStart--
		}
		return previousStart + minMemfitInt(column, previousEnd-previousStart), true
	}
	lineEnd := cursor
	for lineEnd < len(buffer) && buffer[lineEnd] != '\n' {
		lineEnd++
	}
	if lineEnd == len(buffer) {
		return cursor, false
	}
	nextStart := lineEnd + 1
	nextEnd := nextStart
	for nextEnd < len(buffer) && buffer[nextEnd] != '\n' {
		nextEnd++
	}
	return nextStart + minMemfitInt(column, nextEnd-nextStart), true
}

func minMemfitInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func memfitLineBoundary(buffer []rune, cursor int, end bool) int {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(buffer) {
		cursor = len(buffer)
	}
	if end {
		for cursor < len(buffer) && buffer[cursor] != '\n' {
			cursor++
		}
		return cursor
	}
	for cursor > 0 && buffer[cursor-1] != '\n' {
		cursor--
	}
	return cursor
}

func (ui *memfitTUI) recordTranscript(title string, body []string, color string) {
	if strings.TrimSpace(title) == "" && len(body) == 0 {
		return
	}
	copyBody := append([]string(nil), body...)
	ui.transcript = append(ui.transcript, memfitTranscriptBlock{title: title, body: copyBody, color: color})
	if len(ui.transcript) > memfitMaxTranscriptBlocks {
		ui.transcript = append([]memfitTranscriptBlock(nil), ui.transcript[len(ui.transcript)-memfitMaxTranscriptBlocks:]...)
	}
}

func (ui *memfitTUI) recordProcessTranscript() {
	for _, item := range ui.processItems {
		marker, color := memfitProcessStateStyle(item.state)
		title := marker + " " + item.kind
		detail := strings.TrimSpace(item.detail)
		var body []string
		if detail != "" {
			if item.kind == "Thinking" {
				body = strings.Split(detail, "\n")
			} else {
				summary := firstMemfitProcessLine(detail)
				if summary != "" {
					title += " · " + summary
				}
				if strings.Contains(detail, "\n") {
					body = strings.Split(memfitExpandedProcessDetail(item), "\n")
				}
			}
		}
		ui.recordTranscript(title, body, color)
	}
}

func (ui *memfitTUI) flattenedTranscriptLines() []memfitLiveLine {
	lines := make([]memfitLiveLine, 0, len(ui.transcript)*3)
	width := maxInt(8, ui.width-5)
	for _, block := range ui.transcript {
		lines = append(lines, memfitLiveLine{text: block.title, color: memfitColorBold + block.color})
		for _, paragraph := range block.body {
			wrapped := wrapMemfitCells(sanitizeMemfitTerminalText(paragraph), width)
			if len(wrapped) == 0 {
				wrapped = []string{""}
			}
			for _, line := range wrapped {
				lines = append(lines, memfitLiveLine{text: "  " + line})
			}
		}
		lines = append(lines, memfitLiveLine{})
	}
	return lines
}

func (ui *memfitTUI) scrollTranscript(delta int) {
	lines := ui.flattenedTranscriptLines()
	if len(lines) == 0 {
		ui.transcriptScroll = 0
		return
	}
	ui.transcriptScroll += delta
	if ui.transcriptScroll < 0 {
		ui.transcriptScroll = 0
	}
	editorLines, _, _ := ui.editorLines()
	bodyRows := maxInt(1, ui.height-len(editorLines)-3)
	maximum := maxInt(1, len(lines)-bodyRows+1)
	if ui.transcriptScroll > maximum {
		ui.transcriptScroll = maximum
	}
}

func (ui *memfitTUI) transcriptPanelLines(maxRows int) []memfitLiveLine {
	if ui.transcriptScroll <= 0 || maxRows < 3 {
		return nil
	}
	all := ui.flattenedTranscriptLines()
	if len(all) == 0 {
		return nil
	}
	bodyRows := maxRows - 2
	end := len(all) - ui.transcriptScroll + 1
	if end > len(all) {
		end = len(all)
	}
	if end < 1 {
		end = 1
	}
	start := maxInt(0, end-bodyRows)
	view := append([]memfitLiveLine(nil), all[start:end]...)
	header := "╭─ History · " + memfitHistoryOffsetLabel(ui.transcriptScroll) + " · PgDn latest"
	if ui.width < 46 {
		header = fmt.Sprintf("╭─ History · %d back", ui.transcriptScroll)
	}
	result := []memfitLiveLine{{text: header, color: memfitColorBold + memfitColorCyan}}
	for _, line := range view {
		line.text = "│ " + line.text
		result = append(result, line)
	}
	result = append(result, memfitLiveLine{
		text:  fmt.Sprintf("╰─ lines %d–%d/%d", start+1, end, len(all)),
		color: memfitColorDim,
	})
	return result
}

func memfitHistoryOffsetLabel(offset int) string {
	if offset == 1 {
		return "1 line back"
	}
	return fmt.Sprintf("%d lines back", offset)
}
