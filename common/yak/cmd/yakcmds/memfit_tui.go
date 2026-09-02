package yakcmds

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/term"
)

const (
	memfitColorReset  = "\x1b[0m"
	memfitColorBold   = "\x1b[1m"
	memfitColorDim    = "\x1b[2m"
	memfitColorCyan   = "\x1b[36m"
	memfitColorGreen  = "\x1b[32m"
	memfitColorYellow = "\x1b[33m"
	memfitColorRed    = "\x1b[31m"
)

func memfitCanUseTUI() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) &&
		term.IsTerminal(int(os.Stdout.Fd())) &&
		strings.ToLower(os.Getenv("TERM")) != "dumb"
}

func runMemfitPlain(ctx context.Context, client *memfitProcessClient, config memfitStartConfig, query string) error {
	id := fmt.Sprintf("input-%d", memfitNowMillis())
	if err := client.send("input", id, memfitInput{Text: query}); err != nil {
		return err
	}
	printed := false
	var fallbackResult string
	for {
		select {
		case envelope := <-client.events:
			switch envelope.Type {
			case "event":
				event, err := decodeMemfitPayload[memfitWorkerEvent](envelope)
				if err != nil {
					return err
				}
				if isMemfitAnswerStream(event) && event.StreamDelta != "" {
					_, _ = io.WriteString(os.Stdout, sanitizeMemfitTerminalText(event.StreamDelta))
					printed = true
				}
				if event.Type == string(schema.EVENT_TYPE_RESULT) && event.Content != "" {
					fallbackResult = extractMemfitReadableContent(event.Content)
				}
				if config.Debug {
					if description := describeMemfitEvent(event); description != "" {
						fmt.Fprintf(os.Stderr, "\n[memfit] %s\n", description)
					}
				}
			case "error":
				status, _ := decodeMemfitPayload[memfitStatus](envelope)
				return utils.Error(status.Message)
			case "turn_done":
				if !printed && fallbackResult != "" {
					fmt.Fprint(os.Stdout, sanitizeMemfitTerminalText(fallbackResult))
				}
				if printed || fallbackResult != "" {
					fmt.Fprintln(os.Stdout)
				}
				status, _ := decodeMemfitPayload[memfitStatus](envelope)
				if status.Status == "failed" || status.Status == "aborted" {
					return utils.Errorf("memfit task %s", status.Status)
				}
				return nil
			}
		case line := <-client.logs:
			if config.Debug {
				fmt.Fprintf(os.Stderr, "[worker] %s\n", sanitizeMemfitTerminalText(line))
			}
		case <-client.done:
			if err := client.WaitError(); err != nil {
				return utils.Errorf("memfit worker exited: %v%s", err, client.formattedLogTail())
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

type memfitKeyKind int

const (
	memfitKeyInsert memfitKeyKind = iota
	memfitKeySubmit
	memfitKeyNewline
	memfitKeyLeft
	memfitKeyRight
	memfitKeyHome
	memfitKeyEnd
	memfitKeyUp
	memfitKeyDown
	memfitKeyBackspace
	memfitKeyDelete
	memfitKeyDeleteWord
	memfitKeyClearLeft
	memfitKeyClearRight
	memfitKeyInterrupt
	memfitKeyEOF
	memfitKeyRedraw
)

type memfitKey struct {
	kind memfitKeyKind
	text string
}

type memfitTUI struct {
	client *memfitProcessClient
	config memfitStartConfig
	color  bool
	width  int

	buffer       []rune
	cursor       int
	history      [][]rune
	historyIndex int
	historyDraft []rune

	busy          bool
	awaitingInput bool
	interactiveID string
	statusLine    bool
	streamOpen    bool
	streamID      string
	streamKind    string
	answerSeen    bool
	turnStarted   time.Time
	lastModel     string
	lastCtrlC     time.Time
}

func runMemfitTUI(ctx context.Context, client *memfitProcessClient, config memfitStartConfig, initialQuery string) error {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return utils.Wrap(err, "enable memfit terminal input")
	}
	defer func() {
		fmt.Fprint(os.Stdout, "\r\x1b[2K\x1b[?2004l")
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
		fmt.Fprintln(os.Stdout)
	}()

	width, _, _ := term.GetSize(int(os.Stdout.Fd()))
	ui := &memfitTUI{
		client:       client,
		config:       config,
		color:        os.Getenv("NO_COLOR") == "",
		width:        normalizeMemfitWidth(width),
		historyIndex: -1,
	}
	ui.printHeader()
	fmt.Fprint(os.Stdout, "\x1b[?2004h")

	keys := make(chan memfitKey, 128)
	go readMemfitKeys(os.Stdin, keys)
	resizeTicker := time.NewTicker(250 * time.Millisecond)
	defer resizeTicker.Stop()

	if strings.TrimSpace(initialQuery) != "" {
		if err := ui.submit(initialQuery); err != nil {
			return err
		}
	} else {
		ui.renderComposer()
	}

	for {
		select {
		case key := <-keys:
			exit, err := ui.handleKey(key)
			if err != nil {
				return err
			}
			if exit {
				return nil
			}
		case envelope := <-client.events:
			if err := ui.handleEnvelope(envelope); err != nil {
				return err
			}
		case line := <-client.logs:
			if config.Debug {
				ui.printNotice("worker", line, memfitColorDim)
			}
		case <-resizeTicker.C:
			width, _, sizeErr := term.GetSize(int(os.Stdout.Fd()))
			if sizeErr == nil && normalizeMemfitWidth(width) != ui.width {
				ui.width = normalizeMemfitWidth(width)
				if !ui.busy || ui.awaitingInput {
					ui.renderComposer()
				}
			}
		case <-client.done:
			ui.finishOutput()
			if err := client.WaitError(); err != nil {
				return utils.Errorf("memfit worker exited: %v%s", err, client.formattedLogTail())
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (ui *memfitTUI) printHeader() {
	model := "Yaklang configured model"
	if ui.config.Model != "" {
		model = ui.config.Model
	} else if ui.config.AIType != "" {
		model = ui.config.AIType
	}
	if ui.width < 48 {
		fmt.Fprintf(os.Stdout, "%s memfit · %s\r\n", ui.paint(memfitColorBold+memfitColorCyan, "◆"), strings.ToUpper(ui.config.ReviewPolicy))
		fmt.Fprintf(os.Stdout, "%s\r\n", ui.paint(memfitColorDim, "Enter send · Ctrl+C stop · /help"))
		return
	}
	fmt.Fprintf(os.Stdout, "%s  %s\r\n", ui.paint(memfitColorBold+memfitColorCyan, "◆ memfit"), ui.paint(memfitColorDim, "local isolated agent"))
	detail := fmt.Sprintf("%s · %s · %s", model, strings.ToUpper(ui.config.ReviewPolicy), compactMemfitPath(ui.config.Workdir, maxInt(18, ui.width/3)))
	fmt.Fprintf(os.Stdout, "%s\r\n", ui.paint(memfitColorDim, truncateMemfitCells(detail, ui.width)))
	fmt.Fprintf(os.Stdout, "%s\r\n", ui.paint(memfitColorDim, "Enter send · Alt/Shift+Enter newline · ↑↓ history · Ctrl+C stop · /help"))
}

func (ui *memfitTUI) handleKey(key memfitKey) (bool, error) {
	if ui.busy && !ui.awaitingInput {
		switch key.kind {
		case memfitKeyInterrupt:
			if time.Since(ui.lastCtrlC) < time.Second {
				return true, nil
			}
			ui.lastCtrlC = time.Now()
			ui.clearStatusLine()
			ui.finishOutput()
			fmt.Fprintf(os.Stdout, "%s cancellation requested; press Ctrl+C again to exit\r\n", ui.paint(memfitColorYellow, "■"))
			ui.showBusyStatus("cancelling")
			return false, ui.client.send("cancel", fmt.Sprintf("cancel-%d", memfitNowMillis()), nil)
		case memfitKeyRedraw:
			ui.clearStatusLine()
			fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
			ui.printHeader()
			ui.showBusyStatus("working")
		case memfitKeyEOF:
			return true, nil
		}
		return false, nil
	}

	switch key.kind {
	case memfitKeyInsert:
		ui.insert(key.text)
	case memfitKeyNewline:
		ui.insert("\n")
	case memfitKeyLeft:
		if ui.cursor > 0 {
			ui.cursor--
		}
	case memfitKeyRight:
		if ui.cursor < len(ui.buffer) {
			ui.cursor++
		}
	case memfitKeyHome:
		ui.cursor = 0
	case memfitKeyEnd:
		ui.cursor = len(ui.buffer)
	case memfitKeyUp:
		ui.moveHistory(-1)
	case memfitKeyDown:
		ui.moveHistory(1)
	case memfitKeyBackspace:
		if ui.cursor > 0 {
			ui.buffer = append(ui.buffer[:ui.cursor-1], ui.buffer[ui.cursor:]...)
			ui.cursor--
		}
	case memfitKeyDelete:
		if ui.cursor < len(ui.buffer) {
			ui.buffer = append(ui.buffer[:ui.cursor], ui.buffer[ui.cursor+1:]...)
		}
	case memfitKeyDeleteWord:
		ui.deleteWord()
	case memfitKeyClearLeft:
		ui.buffer = append([]rune(nil), ui.buffer[ui.cursor:]...)
		ui.cursor = 0
	case memfitKeyClearRight:
		ui.buffer = append([]rune(nil), ui.buffer[:ui.cursor]...)
	case memfitKeyRedraw:
		ui.clearComposer()
		fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
		ui.printHeader()
	case memfitKeyInterrupt:
		if len(ui.buffer) > 0 {
			ui.buffer = nil
			ui.cursor = 0
			ui.historyIndex = -1
		} else {
			return true, nil
		}
	case memfitKeyEOF:
		if len(ui.buffer) == 0 {
			return true, nil
		}
	case memfitKeySubmit:
		text := strings.TrimSpace(string(ui.buffer))
		if text == "" {
			ui.renderComposer()
			return false, nil
		}
		ui.buffer = nil
		ui.cursor = 0
		ui.historyIndex = -1
		if ui.awaitingInput {
			return false, ui.submitInteractive(text)
		}
		if handled, exit, err := ui.handleLocalCommand(text); handled {
			if !exit {
				ui.renderComposer()
			}
			return exit, err
		}
		return false, ui.submit(text)
	}
	ui.renderComposer()
	return false, nil
}

func (ui *memfitTUI) handleLocalCommand(input string) (handled, exit bool, err error) {
	if !strings.HasPrefix(input, "/") {
		return false, false, nil
	}
	parts := strings.Fields(input)
	command := strings.ToLower(parts[0])
	ui.clearComposer()
	switch command {
	case "/exit", "/quit", "/q":
		return true, true, nil
	case "/help", "/?":
		fmt.Fprintln(os.Stdout, ui.paint(memfitColorBold, "Memfit commands"))
		fmt.Fprintln(os.Stdout, "  /status          show process, model source, mode, and workdir")
		fmt.Fprintln(os.Stdout, "  /mode MODE       switch yolo, ai, or manual review")
		fmt.Fprintln(os.Stdout, "  /logs            show the filtered worker log tail")
		fmt.Fprintln(os.Stdout, "  /clear           clear the visible viewport (scrollback is preserved)")
		fmt.Fprintln(os.Stdout, "  /exit            end the session")
		return true, false, nil
	case "/status", "/config":
		model := "configured Yaklang tiers"
		if ui.config.AIType != "" {
			model = ui.config.AIType
		}
		if ui.config.Model != "" {
			model += "/" + ui.config.Model
		}
		fmt.Fprintf(os.Stdout, "worker pid: %d\r\nmodel: %s\r\nreview: %s\r\nworkdir: %s\r\n", ui.client.cmd.Process.Pid, model, ui.config.ReviewPolicy, ui.config.Workdir)
		return true, false, nil
	case "/logs":
		logs := ui.client.LogTail()
		if len(logs) == 0 {
			fmt.Fprintln(os.Stdout, ui.paint(memfitColorDim, "worker log is empty"))
			return true, false, nil
		}
		if len(logs) > 20 {
			logs = logs[len(logs)-20:]
		}
		for _, line := range logs {
			fmt.Fprintf(os.Stdout, "%s %s\r\n", ui.paint(memfitColorDim, "│"), sanitizeMemfitTerminalText(line))
		}
		return true, false, nil
	case "/clear":
		fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
		ui.printHeader()
		return true, false, nil
	case "/mode", "/review":
		if len(parts) != 2 {
			fmt.Fprintln(os.Stdout, ui.paint(memfitColorYellow, "usage: /mode yolo|ai|manual"))
			return true, false, nil
		}
		policy := strings.ToLower(parts[1])
		if err := validateMemfitReviewPolicy(policy); err != nil {
			fmt.Fprintln(os.Stdout, ui.paint(memfitColorYellow, err.Error()))
			return true, false, nil
		}
		ui.config.ReviewPolicy = policy
		err = ui.client.send("review", fmt.Sprintf("review-%d", memfitNowMillis()), memfitReviewUpdate{Policy: policy})
		if err == nil {
			fmt.Fprintf(os.Stdout, "%s review policy: %s\r\n", ui.paint(memfitColorGreen, "✓"), strings.ToUpper(policy))
		}
		return true, false, err
	default:
		fmt.Fprintf(os.Stdout, "%s unknown command %s; use /help\r\n", ui.paint(memfitColorYellow, "!"), command)
		return true, false, nil
	}
}

func (ui *memfitTUI) submit(text string) error {
	ui.clearComposer()
	ui.rememberHistory(text)
	ui.printUserInput(text)
	ui.busy = true
	ui.awaitingInput = false
	ui.turnStarted = time.Now()
	ui.streamOpen = false
	ui.streamID = ""
	ui.streamKind = ""
	ui.answerSeen = false
	ui.showBusyStatus("thinking")
	return ui.client.send("input", fmt.Sprintf("input-%d", memfitNowMillis()), memfitInput{Text: text})
}

func (ui *memfitTUI) submitInteractive(text string) error {
	ui.clearComposer()
	ui.rememberHistory(text)
	ui.printUserInput(text)
	id := ui.interactiveID
	ui.awaitingInput = false
	ui.interactiveID = ""
	ui.showBusyStatus("continuing")
	return ui.client.send("interactive", id, memfitInput{Text: text})
}

func (ui *memfitTUI) handleEnvelope(envelope memfitEnvelope) error {
	switch envelope.Type {
	case "accepted", "status":
		return nil
	case "error":
		status, _ := decodeMemfitPayload[memfitStatus](envelope)
		ui.finishOutput()
		ui.clearStatusLine()
		fmt.Fprintf(os.Stdout, "%s %s\r\n", ui.paint(memfitColorRed, "error:"), sanitizeMemfitTerminalText(status.Message))
		ui.busy = false
		ui.awaitingInput = false
		ui.renderComposer()
		return nil
	case "event":
		event, err := decodeMemfitPayload[memfitWorkerEvent](envelope)
		if err != nil {
			return err
		}
		ui.handleWorkerEvent(envelope.ID, event)
	case "turn_done":
		status, _ := decodeMemfitPayload[memfitStatus](envelope)
		ui.finishOutput()
		ui.clearStatusLine()
		elapsed := time.Since(ui.turnStarted).Round(100 * time.Millisecond)
		marker, color := "✓", memfitColorGreen
		if status.Status != "completed" && status.Status != "skipped" {
			marker, color = "■", memfitColorYellow
		}
		detail := fmt.Sprintf("%s · %s", status.Status, elapsed)
		if ui.lastModel != "" && ui.width >= 58 {
			detail += " · " + ui.lastModel
		}
		fmt.Fprintf(os.Stdout, "%s %s\r\n", ui.paint(color, marker), ui.paint(memfitColorDim, detail))
		ui.busy = false
		ui.awaitingInput = false
		ui.interactiveID = ""
		ui.renderComposer()
	}
	return nil
}

func (ui *memfitTUI) handleWorkerEvent(envelopeID string, event memfitWorkerEvent) {
	if event.ModelVerbose != "" {
		ui.lastModel = event.ModelVerbose
	} else if event.AIModel != "" {
		ui.lastModel = event.AIModel
	}
	if isMemfitAnswerStream(event) && event.StreamDelta != "" {
		ui.clearStatusLine()
		if !ui.streamOpen || ui.streamKind != "answer" || (event.NodeID != "" && ui.streamID != "" && event.NodeID != ui.streamID) {
			ui.finishOutput()
			fmt.Fprint(os.Stdout, ui.paint(memfitColorBold+memfitColorCyan, "Memfit › "))
			ui.streamOpen = true
			ui.streamID = event.NodeID
			ui.streamKind = "answer"
		}
		fmt.Fprint(os.Stdout, sanitizeMemfitTerminalText(event.StreamDelta))
		ui.answerSeen = true
		return
	}

	if isMemfitThoughtStream(event) && event.StreamDelta != "" && !ui.answerSeen {
		ui.clearStatusLine()
		if !ui.streamOpen || ui.streamKind != "thought" || (event.NodeID != "" && ui.streamID != "" && event.NodeID != ui.streamID) {
			ui.finishOutput()
			fmt.Fprint(os.Stdout, ui.paint(memfitColorDim, "Thinking › "))
			ui.streamOpen = true
			ui.streamID = event.NodeID
			ui.streamKind = "thought"
		}
		fmt.Fprint(os.Stdout, ui.paint(memfitColorDim, sanitizeMemfitTerminalText(event.StreamDelta)))
		return
	}

	if isMemfitInteractiveEvent(event.Type) {
		ui.finishOutput()
		ui.clearStatusLine()
		question := extractMemfitReadableContent(event.Content)
		if question == "" {
			question = "Memfit needs your input"
		}
		fmt.Fprintf(os.Stdout, "%s %s\r\n", ui.paint(memfitColorYellow+memfitColorBold, "?"), sanitizeMemfitTerminalText(question))
		ui.awaitingInput = true
		ui.interactiveID = extractMemfitInteractiveID(envelopeID, event.Content)
		ui.renderComposer()
		return
	}

	if description := describeMemfitEvent(event); description != "" {
		ui.finishOutput()
		ui.clearStatusLine()
		fmt.Fprintf(os.Stdout, "%s %s\r\n", ui.paint(memfitColorDim, "↳"), sanitizeMemfitTerminalText(description))
		ui.showBusyStatus("working")
	}
}

func (ui *memfitTUI) renderComposer() {
	if ui.busy && !ui.awaitingInput {
		return
	}
	prefix := "❯ "
	if ui.awaitingInput {
		prefix = "reply ❯ "
	}
	available := maxInt(8, ui.width-runewidth.StringWidth(prefix)-1)
	text, cursorCells := memfitInputViewport(ui.buffer, ui.cursor, available)
	fmt.Fprint(os.Stdout, "\r\x1b[2K")
	fmt.Fprint(os.Stdout, ui.paint(memfitColorBold+memfitColorCyan, prefix))
	fmt.Fprint(os.Stdout, text)
	endCells := runewidth.StringWidth(text)
	if move := endCells - cursorCells; move > 0 {
		fmt.Fprintf(os.Stdout, "\x1b[%dD", move)
	}
}

func (ui *memfitTUI) clearComposer() {
	fmt.Fprint(os.Stdout, "\r\x1b[2K")
}

func (ui *memfitTUI) showBusyStatus(status string) {
	ui.clearStatusLine()
	fmt.Fprintf(os.Stdout, "%s %s…", ui.paint(memfitColorCyan, "◆"), ui.paint(memfitColorDim, status))
	ui.statusLine = true
}

func (ui *memfitTUI) clearStatusLine() {
	if ui.statusLine {
		fmt.Fprint(os.Stdout, "\r\x1b[2K")
		ui.statusLine = false
	}
}

func (ui *memfitTUI) finishOutput() {
	if ui.streamOpen {
		fmt.Fprint(os.Stdout, "\r\n")
		ui.streamOpen = false
		ui.streamID = ""
		ui.streamKind = ""
	}
}

func (ui *memfitTUI) printUserInput(text string) {
	lines := strings.Split(sanitizeMemfitTerminalText(text), "\n")
	for index, line := range lines {
		prefix := "  "
		if index == 0 {
			prefix = ui.paint(memfitColorBold+memfitColorGreen, "You › ")
		}
		fmt.Fprintf(os.Stdout, "%s%s\r\n", prefix, line)
	}
}

func (ui *memfitTUI) printNotice(label, message, color string) {
	ui.finishOutput()
	ui.clearStatusLine()
	fmt.Fprintf(os.Stdout, "%s %s\r\n", ui.paint(color, "["+label+"]"), sanitizeMemfitTerminalText(message))
	if ui.busy && !ui.awaitingInput {
		ui.showBusyStatus("working")
	} else {
		ui.renderComposer()
	}
}

func (ui *memfitTUI) paint(code, text string) string {
	if !ui.color {
		return text
	}
	return code + text + memfitColorReset
}

func (ui *memfitTUI) insert(text string) {
	if text == "" {
		return
	}
	runes := []rune(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"))
	ui.buffer = append(ui.buffer, make([]rune, len(runes))...)
	copy(ui.buffer[ui.cursor+len(runes):], ui.buffer[ui.cursor:len(ui.buffer)-len(runes)])
	copy(ui.buffer[ui.cursor:], runes)
	ui.cursor += len(runes)
}

func (ui *memfitTUI) deleteWord() {
	end := ui.cursor
	for ui.cursor > 0 && unicode.IsSpace(ui.buffer[ui.cursor-1]) {
		ui.cursor--
	}
	for ui.cursor > 0 && !unicode.IsSpace(ui.buffer[ui.cursor-1]) {
		ui.cursor--
	}
	ui.buffer = append(ui.buffer[:ui.cursor], ui.buffer[end:]...)
}

func (ui *memfitTUI) rememberHistory(text string) {
	entry := []rune(text)
	if len(ui.history) == 0 || string(ui.history[len(ui.history)-1]) != text {
		ui.history = append(ui.history, entry)
		if len(ui.history) > 100 {
			ui.history = append([][]rune(nil), ui.history[len(ui.history)-100:]...)
		}
	}
	ui.historyIndex = -1
	ui.historyDraft = nil
}

func (ui *memfitTUI) moveHistory(direction int) {
	if len(ui.history) == 0 {
		return
	}
	if direction < 0 {
		if ui.historyIndex == -1 {
			ui.historyDraft = append([]rune(nil), ui.buffer...)
			ui.historyIndex = len(ui.history) - 1
		} else if ui.historyIndex > 0 {
			ui.historyIndex--
		}
	} else {
		if ui.historyIndex == -1 {
			return
		}
		if ui.historyIndex < len(ui.history)-1 {
			ui.historyIndex++
		} else {
			ui.historyIndex = -1
			ui.buffer = append([]rune(nil), ui.historyDraft...)
			ui.cursor = len(ui.buffer)
			return
		}
	}
	ui.buffer = append([]rune(nil), ui.history[ui.historyIndex]...)
	ui.cursor = len(ui.buffer)
}

func readMemfitKeys(reader io.Reader, output chan<- memfitKey) {
	r := bufio.NewReader(reader)
	for {
		key, err := readMemfitKey(r)
		if err != nil {
			output <- memfitKey{kind: memfitKeyEOF}
			return
		}
		output <- key
	}
}

func readMemfitKey(r *bufio.Reader) (memfitKey, error) {
	b, err := r.ReadByte()
	if err != nil {
		return memfitKey{}, err
	}
	switch b {
	case 0x01:
		return memfitKey{kind: memfitKeyHome}, nil
	case 0x03:
		return memfitKey{kind: memfitKeyInterrupt}, nil
	case 0x04:
		return memfitKey{kind: memfitKeyEOF}, nil
	case 0x05:
		return memfitKey{kind: memfitKeyEnd}, nil
	case 0x0b:
		return memfitKey{kind: memfitKeyClearRight}, nil
	case 0x0c:
		return memfitKey{kind: memfitKeyRedraw}, nil
	case 0x15:
		return memfitKey{kind: memfitKeyClearLeft}, nil
	case 0x17:
		return memfitKey{kind: memfitKeyDeleteWord}, nil
	case '\r', '\n':
		return memfitKey{kind: memfitKeySubmit}, nil
	case 0x7f, 0x08:
		return memfitKey{kind: memfitKeyBackspace}, nil
	case '\t':
		return memfitKey{kind: memfitKeyInsert, text: "\t"}, nil
	case 0x1b:
		return readMemfitEscapeKey(r)
	}
	if b < 0x20 {
		return memfitKey{kind: memfitKeyInsert}, nil
	}
	if b < utf8.RuneSelf {
		return memfitKey{kind: memfitKeyInsert, text: string([]byte{b})}, nil
	}
	sequence := []byte{b}
	for len(sequence) < utf8.UTFMax && !utf8.FullRune(sequence) {
		next, readErr := r.ReadByte()
		if readErr != nil {
			return memfitKey{}, readErr
		}
		sequence = append(sequence, next)
	}
	runeValue, _ := utf8.DecodeRune(sequence)
	return memfitKey{kind: memfitKeyInsert, text: string(runeValue)}, nil
}

func readMemfitEscapeKey(r *bufio.Reader) (memfitKey, error) {
	next, err := r.ReadByte()
	if err != nil {
		return memfitKey{kind: memfitKeyInsert}, nil
	}
	if next == '\r' || next == '\n' {
		return memfitKey{kind: memfitKeyNewline}, nil
	}
	if next == 'O' {
		final, readErr := r.ReadByte()
		if readErr != nil {
			return memfitKey{}, readErr
		}
		switch final {
		case 'H':
			return memfitKey{kind: memfitKeyHome}, nil
		case 'F':
			return memfitKey{kind: memfitKeyEnd}, nil
		}
	}
	if next != '[' {
		return memfitKey{kind: memfitKeyInsert, text: string(next)}, nil
	}
	var sequence []byte
	for len(sequence) < 32 {
		part, readErr := r.ReadByte()
		if readErr != nil {
			return memfitKey{}, readErr
		}
		sequence = append(sequence, part)
		if part >= '@' && part <= '~' {
			break
		}
	}
	code := string(sequence)
	switch code {
	case "A":
		return memfitKey{kind: memfitKeyUp}, nil
	case "B":
		return memfitKey{kind: memfitKeyDown}, nil
	case "C":
		return memfitKey{kind: memfitKeyRight}, nil
	case "D":
		return memfitKey{kind: memfitKeyLeft}, nil
	case "H", "1~", "7~":
		return memfitKey{kind: memfitKeyHome}, nil
	case "F", "4~", "8~":
		return memfitKey{kind: memfitKeyEnd}, nil
	case "3~":
		return memfitKey{kind: memfitKeyDelete}, nil
	case "13;2u", "13;3u", "27;2;13~":
		return memfitKey{kind: memfitKeyNewline}, nil
	case "200~":
		paste, readErr := readMemfitBracketedPaste(r)
		return memfitKey{kind: memfitKeyInsert, text: paste}, readErr
	default:
		return memfitKey{kind: memfitKeyInsert}, nil
	}
}

func readMemfitBracketedPaste(r *bufio.Reader) (string, error) {
	terminator := []byte("\x1b[201~")
	buffer := make([]byte, 0, 4096)
	for len(buffer) < 16<<20 {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		buffer = append(buffer, b)
		if bytes.HasSuffix(buffer, terminator) {
			buffer = buffer[:len(buffer)-len(terminator)]
			return strings.ReplaceAll(strings.ReplaceAll(string(buffer), "\r\n", "\n"), "\r", "\n"), nil
		}
	}
	return "", utils.Error("memfit paste exceeds 16 MiB")
}

func memfitInputViewport(buffer []rune, cursor, maxCells int) (string, int) {
	if maxCells < 4 {
		maxCells = 4
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(buffer) {
		cursor = len(buffer)
	}
	pieces := make([]string, len(buffer))
	widths := make([]int, len(buffer))
	for i, r := range buffer {
		switch r {
		case '\n':
			pieces[i] = "↵"
		case '\t':
			pieces[i] = "⇥"
		default:
			if unicode.IsControl(r) {
				pieces[i] = "�"
			} else {
				pieces[i] = string(r)
			}
		}
		widths[i] = maxInt(1, runewidth.StringWidth(pieces[i]))
	}

	start := 0
	widthBeforeCursor := 0
	for i := 0; i < cursor; i++ {
		widthBeforeCursor += widths[i]
	}
	if widthBeforeCursor > maxCells-3 {
		budget := maxCells / 2
		used := 0
		start = cursor
		for start > 0 && used+widths[start-1] <= budget {
			start--
			used += widths[start]
		}
	}

	prefix := ""
	used := 0
	if start > 0 {
		prefix = "…"
		used = 1
	}
	end := start
	for end < len(buffer) {
		reserveSuffix := 0
		if end+1 < len(buffer) {
			reserveSuffix = 1
		}
		if used+widths[end]+reserveSuffix > maxCells {
			break
		}
		used += widths[end]
		end++
	}
	suffix := ""
	if end < len(buffer) {
		suffix = "…"
	}
	var text strings.Builder
	text.WriteString(prefix)
	cursorCells := runewidth.StringWidth(prefix)
	for i := start; i < end; i++ {
		if i < cursor {
			cursorCells += widths[i]
		}
		text.WriteString(pieces[i])
	}
	text.WriteString(suffix)
	return text.String(), cursorCells
}

func sanitizeMemfitTerminalText(input string) string {
	var output strings.Builder
	for index := 0; index < len(input); {
		if input[index] == 0x1b {
			index++
			if index >= len(input) {
				break
			}
			switch input[index] {
			case '[':
				index++
				for index < len(input) {
					b := input[index]
					index++
					if b >= '@' && b <= '~' {
						break
					}
				}
			case ']':
				index++
				for index < len(input) {
					if input[index] == '\a' {
						index++
						break
					}
					if input[index] == 0x1b && index+1 < len(input) && input[index+1] == '\\' {
						index += 2
						break
					}
					index++
				}
			default:
				index++
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(input[index:])
		if size == 0 {
			break
		}
		index += size
		switch r {
		case '\n', '\t':
			output.WriteRune(r)
		case '\r':
			// Never allow worker output to move the cursor backwards.
		default:
			if !unicode.IsControl(r) {
				output.WriteRune(r)
			}
		}
	}
	return output.String()
}

func describeMemfitEvent(event memfitWorkerEvent) string {
	switch event.Type {
	case string(schema.EVENT_TOOL_CALL_START):
		name := extractMemfitJSONField(event.Content, "tool_name", "name", "tool")
		if name == "" {
			name = "tool"
		}
		return "running " + name
	case string(schema.EVENT_TOOL_CALL_ERROR):
		message := extractMemfitJSONField(event.Content, "error", "message", "reason")
		if message == "" {
			message = "tool failed"
		}
		return message
	case string(schema.EVENT_TYPE_API_REQUEST_FAILED):
		message := extractMemfitReadableContent(event.Content)
		if message == "" {
			message = "AI request failed"
		}
		return message
	case string(schema.EVENT_TYPE_NOTIFY):
		return extractMemfitReadableContent(event.Content)
	}
	return ""
}

func isMemfitAnswerStream(event memfitWorkerEvent) bool {
	return event.Type == string(schema.EVENT_TYPE_STREAM) &&
		!event.IsSystem &&
		!event.IsReason &&
		event.NodeID == "re-act-loop-answer-payload"
}

func isMemfitThoughtStream(event memfitWorkerEvent) bool {
	return event.Type == string(schema.EVENT_TYPE_STREAM) &&
		!event.IsSystem &&
		(event.IsReason || event.NodeID == "re-act-loop-thought" || event.VizSource == "human_readable_thought")
}

func extractMemfitReadableContent(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	var value any
	if json.Unmarshal([]byte(content), &value) != nil {
		return content
	}
	return findMemfitReadableValue(value)
}

func findMemfitReadableValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		for _, key := range []string{"message", "question", "prompt", "content", "result", "reason", "error", "text"} {
			if child, ok := typed[key]; ok {
				if result := findMemfitReadableValue(child); result != "" {
					return result
				}
			}
		}
	case []any:
		var values []string
		for _, child := range typed {
			if result := findMemfitReadableValue(child); result != "" {
				values = append(values, result)
			}
		}
		return strings.Join(values, " · ")
	}
	return ""
}

func extractMemfitJSONField(content string, keys ...string) string {
	var value map[string]any
	if json.Unmarshal([]byte(content), &value) != nil {
		return ""
	}
	for _, key := range keys {
		if result := utils.InterfaceToString(value[key]); result != "" {
			return result
		}
	}
	return ""
}

func isMemfitInteractiveEvent(eventType string) bool {
	switch schema.EventType(eventType) {
	case schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE,
		schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE,
		schema.EVENT_TYPE_TASK_REVIEW_REQUIRE,
		schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE,
		schema.EVENT_TYPE_PERMISSION_REQUIRE:
		return true
	default:
		return false
	}
}

func extractMemfitInteractiveID(fallback, content string) string {
	if id := extractMemfitJSONField(content, "id", "interactive_id"); id != "" {
		return id
	}
	return fallback
}

func normalizeMemfitWidth(width int) int {
	if width < 20 {
		return 20
	}
	return width
}

func compactMemfitPath(path string, maxCells int) string {
	if runewidth.StringWidth(path) <= maxCells {
		return path
	}
	parts := strings.Split(strings.TrimRight(path, string(os.PathSeparator)), string(os.PathSeparator))
	if len(parts) > 1 {
		path = "…" + string(os.PathSeparator) + parts[len(parts)-1]
	}
	return truncateMemfitCells(path, maxCells)
}

func truncateMemfitCells(value string, maxCells int) string {
	if runewidth.StringWidth(value) <= maxCells {
		return value
	}
	var output strings.Builder
	used := 0
	for _, r := range value {
		width := maxInt(1, runewidth.RuneWidth(r))
		if used+width+1 > maxCells {
			break
		}
		output.WriteRune(r)
		used += width
	}
	output.WriteRune('…')
	return output.String()
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
