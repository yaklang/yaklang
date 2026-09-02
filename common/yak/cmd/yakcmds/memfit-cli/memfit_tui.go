package memfitcli

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
	memfitColorReset      = "\x1b[0m"
	memfitColorBold       = "\x1b[1m"
	memfitColorDim        = "\x1b[2m"
	memfitColorCyan       = "\x1b[36m"
	memfitColorGreen      = "\x1b[32m"
	memfitColorYellow     = "\x1b[33m"
	memfitColorRed        = "\x1b[31m"
	memfitMaxQueuedInputs = 100
)

func memfitCanUseTUI() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) &&
		term.IsTerminal(int(os.Stdout.Fd())) &&
		strings.ToLower(os.Getenv("TERM")) != "dumb"
}

func runMemfitPlain(ctx context.Context, client memfitClient, config memfitStartConfig, query string) error {
	id := fmt.Sprintf("input-%d", memfitNowMillis())
	if err := client.send("input", id, memfitInput{Text: query}); err != nil {
		return err
	}
	var answers memfitAnswerStreams
	var fallbackResult string
	for {
		select {
		case envelope := <-client.Events():
			switch envelope.Type {
			case "event":
				event, err := decodeMemfitPayload[memfitWorkerEvent](envelope)
				if err != nil {
					return err
				}
				if isMemfitAnswerStream(event) && event.StreamDelta != "" {
					answers.Append(envelope.ID, event.StreamDelta)
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
				result := answers.Last()
				if result == "" {
					result = fallbackResult
				}
				if result != "" {
					fmt.Fprint(os.Stdout, sanitizeMemfitTerminalText(result))
					fmt.Fprintln(os.Stdout)
				}
				status, _ := decodeMemfitPayload[memfitStatus](envelope)
				if status.Status == "failed" || status.Status == "aborted" {
					return utils.Errorf("memfit task %s", status.Status)
				}
				return nil
			}
		case line := <-client.Logs():
			if config.Debug {
				fmt.Fprintf(os.Stderr, "[worker] %s\n", sanitizeMemfitTerminalText(line))
			}
		case <-client.Done():
			if err := client.WaitError(); err != nil {
				return utils.Errorf("memfit worker exited: %v%s", err, client.formattedLogTail())
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

type memfitAnswerStreams struct {
	order []string
	text  map[string]*strings.Builder
}

func (s *memfitAnswerStreams) Append(writerID, delta string) {
	if writerID == "" {
		writerID = "default"
	}
	if s.text == nil {
		s.text = make(map[string]*strings.Builder)
	}
	stream, ok := s.text[writerID]
	if !ok {
		stream = &strings.Builder{}
		s.text[writerID] = stream
		s.order = append(s.order, writerID)
	}
	stream.WriteString(delta)
}

func (s *memfitAnswerStreams) FirstID() string {
	if len(s.order) == 0 {
		return ""
	}
	return s.order[0]
}

func (s *memfitAnswerStreams) First() string {
	if len(s.order) == 0 {
		return ""
	}
	return s.text[s.order[0]].String()
}

func (s *memfitAnswerStreams) Last() string {
	if len(s.order) == 0 {
		return ""
	}
	return s.text[s.order[len(s.order)-1]].String()
}

func (s *memfitAnswerStreams) Reset() {
	s.order = nil
	s.text = nil
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

type memfitQueuedInput struct {
	text     string
	queuedAt time.Time
}

type memfitInfoRow struct {
	label string
	value string
}

type memfitTUI struct {
	client memfitClient
	config memfitStartConfig
	color  bool
	width  int

	buffer       []rune
	cursor       int
	history      [][]rune
	historyIndex int
	historyDraft []rune

	busy             bool
	awaitingInput    bool
	interactiveID    string
	queued           []memfitQueuedInput
	queuePaused      bool
	liveRows         int
	activity         string
	streamKind       string
	streamWriter     string
	streamPreview    string
	answerSeen       bool
	answers          memfitAnswerStreams
	fallbackResult   string
	turnStarted      time.Time
	lastStatusTick   int64
	lastModel        string
	lastCtrlC        time.Time
	savedDraft       []rune
	savedDraftCursor int
}

func runMemfitTUI(ctx context.Context, client memfitClient, config memfitStartConfig, initialQuery string) error {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return utils.Wrap(err, "enable memfit terminal input")
	}
	var ui *memfitTUI
	defer func() {
		if ui != nil {
			ui.clearLiveFrame()
		}
		fmt.Fprint(os.Stdout, "\x1b[?2004l")
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
		fmt.Fprintln(os.Stdout)
	}()

	width, _, _ := term.GetSize(int(os.Stdout.Fd()))
	ui = &memfitTUI{
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
		case envelope := <-client.Events():
			if err := ui.handleEnvelope(envelope); err != nil {
				return err
			}
		case line := <-client.Logs():
			if config.Debug {
				ui.printNotice("worker", line, memfitColorDim)
			}
		case <-resizeTicker.C:
			width, _, sizeErr := term.GetSize(int(os.Stdout.Fd()))
			resized := sizeErr == nil && normalizeMemfitWidth(width) != ui.width
			if resized {
				ui.width = normalizeMemfitWidth(width)
			}
			statusTick := time.Now().Unix()
			if resized || (ui.busy && statusTick != ui.lastStatusTick) {
				ui.lastStatusTick = statusTick
				ui.renderComposer()
			}
		case <-client.Done():
			ui.clearLiveFrame()
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
		hint := "Enter send · Ctrl+C stop · /help"
		if ui.width < 34 {
			hint = "Enter send · ^C stop"
		}
		if ui.width < 24 {
			hint = "Enter send · ^C"
		}
		fmt.Fprintf(os.Stdout, "%s\r\n", ui.paint(memfitColorDim, hint))
		fmt.Fprint(os.Stdout, "\r\n")
		return
	}
	fmt.Fprintf(os.Stdout, "%s  %s\r\n", ui.paint(memfitColorBold+memfitColorCyan, "◆ memfit"), ui.paint(memfitColorDim, "local isolated agent"))
	detail := fmt.Sprintf("%s · %s · %s", model, strings.ToUpper(ui.config.ReviewPolicy), compactMemfitPath(ui.config.Workdir, maxInt(18, ui.width/3)))
	fmt.Fprintf(os.Stdout, "%s\r\n", ui.paint(memfitColorDim, truncateMemfitCells(detail, ui.width-1)))
	hint := "Enter send · Ctrl+C stop · /help"
	if ui.width >= 64 {
		hint = "Enter send · Alt+Enter newline · Ctrl+C stop · /help"
	}
	if ui.width >= 82 {
		hint = "Enter send · Alt/Shift+Enter newline · ↑↓ history · Ctrl+C stop · /help"
	}
	fmt.Fprintf(os.Stdout, "%s\r\n\r\n", ui.paint(memfitColorDim, hint))
}

func (ui *memfitTUI) handleKey(key memfitKey) (bool, error) {
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
		ui.clearLiveFrame()
		fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
		ui.printHeader()
	case memfitKeyInterrupt:
		if len(ui.buffer) > 0 {
			ui.buffer = nil
			ui.cursor = 0
			ui.historyIndex = -1
		} else if ui.busy {
			if time.Since(ui.lastCtrlC) < time.Second {
				return true, nil
			}
			ui.lastCtrlC = time.Now()
			ui.activity = "Cancelling"
			ui.streamKind = ""
			ui.streamWriter = ""
			ui.streamPreview = ""
			ui.printNotice("cancel", "Cancellation requested; press Ctrl+C again to exit", memfitColorYellow)
			return false, ui.client.send("cancel", fmt.Sprintf("cancel-%d", memfitNowMillis()), nil)
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
		if ui.busy || len(ui.queued) > 0 {
			ui.enqueue(text)
			if !ui.busy && !ui.queuePaused {
				return false, ui.startNextQueued()
			}
			ui.renderComposer()
			return false, nil
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
	ui.clearLiveFrame()
	switch command {
	case "/exit", "/quit", "/q":
		return true, true, nil
	case "/help", "/?":
		lines := []string{
			"/status       session and worker status",
			"/queue        show queued messages",
			"/queue clear  discard queued messages",
			"/queue resume continue a paused queue",
			"/mode MODE    set yolo, ai, or manual",
			"/logs         recent worker diagnostics",
			"/clear        clear the view",
			"/exit         end the session",
			"Enter queues your message while Memfit is working.",
		}
		if ui.width < 48 {
			lines = []string{
				"/status  session info",
				"/queue   queued messages",
				"/mode    review mode",
				"/logs    worker diagnostics",
				"/clear   clear view",
				"/exit    quit",
				"Enter queues while busy.",
			}
		}
		if ui.width < 32 {
			lines = []string{
				"/status /queue /mode",
				"/logs /clear /exit",
				"Enter queues while busy.",
			}
		}
		ui.printTextSection("Commands", lines)
		return true, false, nil
	case "/status", "/config":
		model := "configured Yaklang tiers"
		if ui.config.AIType != "" {
			model = ui.config.AIType
		}
		if ui.config.Model != "" {
			model = ui.config.Model
			if ui.config.AIType != "" {
				model = ui.config.AIType + "/" + model
			}
		}
		rows := []memfitInfoRow{
			{label: "State", value: ui.statusSummary()},
			{label: "Worker", value: fmt.Sprintf("PID %d", ui.client.PID())},
			{label: "Model", value: model},
			{label: "Review", value: strings.ToUpper(ui.config.ReviewPolicy)},
			{label: "Queue", value: ui.queueSummary()},
			{label: "Project", value: ui.config.Workdir},
		}
		if ui.width < 48 {
			ui.printTextSection("Session", []string{
				ui.statusSummary() + " · " + strings.ToUpper(ui.config.ReviewPolicy),
				fmt.Sprintf("PID %d · %s", ui.client.PID(), ui.compactQueueSummary()),
				model,
				compactMemfitPath(ui.config.Workdir, maxInt(8, ui.width-3)),
			})
		} else {
			ui.printInfoSection("Session", rows)
		}
		return true, false, nil
	case "/logs":
		ui.printWorkerLogs(ui.client.LogTail())
		return true, false, nil
	case "/queue":
		if len(parts) == 2 && strings.EqualFold(parts[1], "clear") {
			count := len(ui.queued)
			ui.queued = nil
			ui.queuePaused = false
			ui.printNotice("queue", fmt.Sprintf("Cleared %d queued message(s)", count), memfitColorYellow)
			return true, false, nil
		}
		if len(parts) == 2 && strings.EqualFold(parts[1], "resume") {
			ui.queuePaused = false
			if !ui.busy {
				return true, false, ui.startNextQueued()
			}
			return true, false, nil
		}
		ui.printQueue()
		return true, false, nil
	case "/clear":
		fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
		ui.printHeader()
		return true, false, nil
	case "/mode", "/review":
		if len(parts) != 2 {
			ui.printNotice("mode", "Usage: /mode yolo|ai|manual", memfitColorYellow)
			return true, false, nil
		}
		policy := strings.ToLower(parts[1])
		if err := validateMemfitReviewPolicy(policy); err != nil {
			ui.printNotice("mode", err.Error(), memfitColorYellow)
			return true, false, nil
		}
		ui.config.ReviewPolicy = policy
		err = ui.client.send("review", fmt.Sprintf("review-%d", memfitNowMillis()), memfitReviewUpdate{Policy: policy})
		if err == nil {
			ui.printNotice("mode", "Review policy changed to "+strings.ToUpper(policy), memfitColorGreen)
		}
		return true, false, err
	default:
		ui.printNotice("command", "Unknown command "+command+"; use /help", memfitColorYellow)
		return true, false, nil
	}
}

func (ui *memfitTUI) submit(text string) error {
	ui.rememberHistory(text)
	return ui.startTurn(text)
}

func (ui *memfitTUI) startTurn(text string) error {
	ui.clearLiveFrame()
	ui.printInputBlock("You", text, memfitColorGreen)
	ui.busy = true
	ui.queuePaused = false
	ui.awaitingInput = false
	ui.interactiveID = ""
	ui.turnStarted = time.Now()
	ui.lastStatusTick = time.Now().Unix()
	ui.activity = "Thinking"
	ui.streamKind = ""
	ui.streamWriter = ""
	ui.streamPreview = ""
	ui.answerSeen = false
	ui.answers.Reset()
	ui.fallbackResult = ""
	if err := ui.client.send("input", fmt.Sprintf("input-%d", memfitNowMillis()), memfitInput{Text: text}); err != nil {
		return err
	}
	ui.renderComposer()
	return nil
}

func (ui *memfitTUI) enqueue(text string) {
	ui.rememberHistory(text)
	if len(ui.queued) >= memfitMaxQueuedInputs {
		ui.printNotice("queue", fmt.Sprintf("Queue is full (%d messages); use /queue clear first", memfitMaxQueuedInputs), memfitColorYellow)
		return
	}
	ui.queued = append(ui.queued, memfitQueuedInput{text: text, queuedAt: time.Now()})
	ui.clearLiveFrame()
	ui.printInputBlock(fmt.Sprintf("Queued #%d", len(ui.queued)), text, memfitColorYellow)
}

func (ui *memfitTUI) startNextQueued() error {
	if ui.busy || len(ui.queued) == 0 {
		ui.renderComposer()
		return nil
	}
	next := ui.queued[0]
	ui.queued = append([]memfitQueuedInput(nil), ui.queued[1:]...)
	return ui.startTurn(next.text)
}

func (ui *memfitTUI) submitInteractive(text string) error {
	ui.clearLiveFrame()
	ui.rememberHistory(text)
	ui.printInputBlock("Reply", text, memfitColorYellow)
	id := ui.interactiveID
	ui.awaitingInput = false
	ui.interactiveID = ""
	ui.activity = "Continuing"
	ui.streamKind = ""
	ui.streamWriter = ""
	ui.streamPreview = ""
	if err := ui.client.send("interactive", id, memfitInput{Text: text}); err != nil {
		return err
	}
	ui.restoreSavedDraft()
	ui.renderComposer()
	return nil
}

func (ui *memfitTUI) handleEnvelope(envelope memfitEnvelope) error {
	switch envelope.Type {
	case "accepted", "status":
		return nil
	case "error":
		status, _ := decodeMemfitPayload[memfitStatus](envelope)
		ui.clearLiveFrame()
		ui.printTextSection("Error", []string{humanizeMemfitMessage(status.Message)}, memfitColorRed)
		ui.busy = false
		ui.awaitingInput = false
		ui.activity = "Ready"
		ui.streamKind = ""
		ui.streamWriter = ""
		ui.streamPreview = ""
		ui.queuePaused = len(ui.queued) > 0
		ui.restoreSavedDraft()
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
		result := ui.answers.Last()
		if result == "" {
			result = ui.fallbackResult
		}
		ui.clearLiveFrame()
		if result != "" {
			ui.printAnswerBlock(result)
		}
		elapsed := time.Since(ui.turnStarted).Round(100 * time.Millisecond)
		marker, color := "✓", memfitColorGreen
		if status.Status != "completed" && status.Status != "skipped" {
			marker, color = "■", memfitColorYellow
		}
		detail := fmt.Sprintf("%s · %s", titleMemfitStatus(status.Status), elapsed)
		if ui.lastModel != "" && ui.width >= 58 {
			detail += " · " + ui.lastModel
		}
		fmt.Fprintf(os.Stdout, "%s %s\r\n\r\n", ui.paint(color, marker), ui.paint(memfitColorDim, detail))
		ui.busy = false
		ui.awaitingInput = false
		ui.interactiveID = ""
		ui.activity = "Ready"
		ui.streamKind = ""
		ui.streamWriter = ""
		ui.streamPreview = ""
		ui.answers.Reset()
		ui.fallbackResult = ""
		ui.restoreSavedDraft()
		if len(ui.queued) > 0 {
			return ui.startNextQueued()
		}
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
		writerID := envelopeID
		if writerID == "" {
			writerID = "default"
		}
		ui.answers.Append(writerID, event.StreamDelta)
		ui.activity = "Responding"
		if ui.streamWriter != writerID {
			ui.streamPreview = ""
			ui.streamWriter = writerID
		}
		ui.streamKind = "answer"
		ui.streamPreview = lastMemfitPreviewLine(ui.streamPreview + event.StreamDelta)
		ui.answerSeen = true
		ui.renderComposer()
		return
	}

	if isMemfitThoughtStream(event) && event.StreamDelta != "" && !ui.answerSeen {
		ui.activity = "Thinking"
		if ui.streamKind != "thought" {
			ui.streamPreview = ""
		}
		ui.streamKind = "thought"
		ui.streamPreview = lastMemfitPreviewLine(ui.streamPreview + event.StreamDelta)
		ui.renderComposer()
		return
	}

	if event.Type == string(schema.EVENT_TYPE_RESULT) && event.Content != "" {
		ui.fallbackResult = extractMemfitReadableContent(event.Content)
	}

	if event.Type == string(schema.EVENT_TOOL_CALL_ERROR) {
		message := extractMemfitJSONField(event.Content, "error", "message", "reason")
		if message == "" {
			message = "The tool could not run; Memfit may retry with corrected input."
		}
		ui.activity = "Recovering from tool issue"
		ui.streamKind = ""
		ui.streamWriter = ""
		ui.streamPreview = ""
		ui.clearLiveFrame()
		ui.printTextSection("Tool issue", []string{compactMemfitMessage(message, ui.width*3)}, memfitColorYellow)
		ui.renderComposer()
		return
	}

	if event.Type == string(schema.EVENT_TYPE_API_REQUEST_FAILED) {
		message := extractMemfitReadableContent(event.Content)
		if message == "" {
			message = "The AI request failed."
		}
		ui.activity = "AI request failed"
		ui.streamKind = ""
		ui.streamWriter = ""
		ui.streamPreview = ""
		ui.clearLiveFrame()
		ui.printTextSection("Request issue", []string{compactMemfitMessage(message, ui.width*3)}, memfitColorRed)
		ui.renderComposer()
		return
	}

	if isMemfitInteractiveEvent(event.Type) {
		question := humanizeMemfitMessage(extractMemfitReadableContent(event.Content))
		if question == "" {
			question = "Memfit needs your input"
		}
		if ui.config.ReviewPolicy == "yolo" {
			ui.activity = singleLineMemfitText("Plan · " + question)
			ui.streamKind = ""
			ui.streamWriter = ""
			ui.streamPreview = ""
			ui.renderComposer()
			return
		}
		ui.clearLiveFrame()
		if len(ui.buffer) > 0 {
			ui.savedDraft = append([]rune(nil), ui.buffer...)
			ui.savedDraftCursor = ui.cursor
			ui.buffer = nil
			ui.cursor = 0
		}
		ui.printTextSection("Action required", []string{question}, memfitColorYellow)
		ui.awaitingInput = true
		ui.interactiveID = extractMemfitInteractiveID(envelopeID, event.Content)
		ui.activity = "Waiting for your reply"
		ui.renderComposer()
		return
	}

	if description := describeMemfitEvent(event); description != "" {
		ui.activity = singleLineMemfitText(description)
		ui.streamKind = ""
		ui.streamWriter = ""
		ui.streamPreview = ""
		ui.renderComposer()
	}
}

func (ui *memfitTUI) renderComposer() {
	ui.clearLiveFrame()
	rows := 0
	if ui.streamPreview != "" {
		label, color := "Thinking › ", memfitColorDim
		if ui.streamKind == "answer" {
			label, color = "Memfit › ", memfitColorCyan
		}
		preview := singleLineMemfitText(ui.streamPreview)
		line := label + preview
		fmt.Fprintf(os.Stdout, "%s\r\n", ui.paint(color, truncateMemfitCells(line, ui.width-1)))
		rows++
	}
	statusMarker, statusColor := "○", memfitColorDim
	if ui.busy {
		statusMarker, statusColor = "◆", memfitColorCyan
	}
	if ui.awaitingInput {
		statusMarker, statusColor = "?", memfitColorYellow
	}
	status := statusMarker + " " + ui.liveStatusSummary()
	fmt.Fprintf(os.Stdout, "%s\r\n", ui.paint(statusColor, truncateMemfitCells(status, ui.width-1)))
	rows++
	prefix := "❯ "
	if ui.awaitingInput {
		prefix = "reply ❯ "
	} else if ui.busy {
		prefix = "queue ❯ "
	}
	available := maxInt(8, ui.width-runewidth.StringWidth(prefix)-1)
	text, cursorCells := memfitInputViewport(ui.buffer, ui.cursor, available)
	fmt.Fprint(os.Stdout, ui.paint(memfitColorBold+memfitColorCyan, prefix))
	fmt.Fprint(os.Stdout, text)
	rows++
	endCells := runewidth.StringWidth(text)
	if move := endCells - cursorCells; move > 0 {
		fmt.Fprintf(os.Stdout, "\x1b[%dD", move)
	}
	ui.liveRows = rows
}

func (ui *memfitTUI) clearLiveFrame() {
	if ui.liveRows == 0 {
		return
	}
	fmt.Fprint(os.Stdout, "\r\x1b[2K")
	for row := 1; row < ui.liveRows; row++ {
		fmt.Fprint(os.Stdout, "\x1b[1A\r\x1b[2K")
	}
	ui.liveRows = 0
}

func (ui *memfitTUI) printNotice(label, message, color string) {
	ui.clearLiveFrame()
	ui.printTextSection(strings.ToUpper(label[:1])+label[1:], []string{humanizeMemfitMessage(message)}, color)
	ui.renderComposer()
}

func (ui *memfitTUI) printInputBlock(label, text, color string) {
	ui.clearLiveFrame()
	fmt.Fprintf(os.Stdout, "%s\r\n", ui.paint(memfitColorBold+color, label))
	ui.printWrappedText(text, "  ")
	fmt.Fprint(os.Stdout, "\r\n")
}

func (ui *memfitTUI) printAnswerBlock(text string) {
	fmt.Fprintf(os.Stdout, "%s\r\n", ui.paint(memfitColorBold+memfitColorCyan, "Memfit"))
	ui.printWrappedText(text, "  ")
	fmt.Fprint(os.Stdout, "\r\n")
}

func (ui *memfitTUI) printTextSection(title string, lines []string, colors ...string) {
	color := memfitColorBold
	if len(colors) > 0 {
		color = memfitColorBold + colors[0]
	}
	fmt.Fprintf(os.Stdout, "%s\r\n", ui.paint(color, title))
	for _, line := range lines {
		ui.printWrappedText(line, "  ")
	}
	fmt.Fprint(os.Stdout, "\r\n")
}

func (ui *memfitTUI) printInfoSection(title string, rows []memfitInfoRow) {
	fmt.Fprintf(os.Stdout, "%s\r\n", ui.paint(memfitColorBold, title))
	for _, row := range rows {
		label := fmt.Sprintf("  %-8s", row.label)
		ui.printWrappedWithPrefix(label, "            ", row.value)
	}
	fmt.Fprint(os.Stdout, "\r\n")
}

func (ui *memfitTUI) printQueue() {
	if len(ui.queued) == 0 {
		ui.printTextSection("Queue", []string{"No messages waiting."})
		return
	}
	lines := make([]string, 0, len(ui.queued)*2)
	for index, queued := range ui.queued {
		age := time.Since(queued.queuedAt).Round(time.Second)
		lines = append(lines, fmt.Sprintf("#%d · waiting %s", index+1, age), "  "+singleLineMemfitText(queued.text))
	}
	ui.printTextSection(fmt.Sprintf("Queue · %d waiting", len(ui.queued)), lines)
}

func (ui *memfitTUI) printWorkerLogs(logs []string) {
	if len(logs) == 0 {
		ui.printTextSection("Worker diagnostics", []string{"No recent diagnostics."})
		return
	}
	if len(logs) > 8 {
		logs = logs[len(logs)-8:]
	}
	rows := make([]memfitInfoRow, 0, len(logs))
	for _, line := range logs {
		label, message := formatMemfitLogLine(line)
		if message == "" {
			continue
		}
		rows = append(rows, memfitInfoRow{label: label, value: message})
	}
	if len(rows) == 0 {
		ui.printTextSection("Worker diagnostics", []string{"No user-relevant diagnostics."})
		return
	}
	ui.printInfoSection(fmt.Sprintf("Worker diagnostics · %d", len(rows)), rows)
}

func (ui *memfitTUI) printWrappedText(text, indent string) {
	ui.printWrappedWithPrefix(indent, indent, text)
}

func (ui *memfitTUI) printWrappedWithPrefix(firstPrefix, nextPrefix, text string) {
	width := maxInt(8, ui.width-runewidth.StringWidth(firstPrefix)-1)
	lines := wrapMemfitCells(sanitizeMemfitTerminalText(text), width)
	if len(lines) == 0 {
		lines = []string{""}
	}
	for index, line := range lines {
		prefix := nextPrefix
		if index == 0 {
			prefix = firstPrefix
		}
		fmt.Fprintf(os.Stdout, "%s%s\r\n", prefix, line)
	}

}

func (ui *memfitTUI) statusSummary() string {
	if ui.awaitingInput {
		return "Waiting for your reply"
	}
	if !ui.busy {
		if ui.queuePaused && len(ui.queued) > 0 {
			return "Ready · queue paused"
		}
		return "Ready"
	}
	activity := ui.activity
	if activity == "" {
		activity = "Working"
	}
	elapsed := time.Since(ui.turnStarted).Round(100 * time.Millisecond)
	return fmt.Sprintf("%s · %s", activity, elapsed)
}

func (ui *memfitTUI) liveStatusSummary() string {
	if ui.awaitingInput {
		if len(ui.queued) > 0 {
			if ui.width < 40 {
				return fmt.Sprintf("Reply needed · Q%d", len(ui.queued))
			}
			return "Waiting for your reply · " + ui.queueSummary()
		}
		return "Waiting for your reply"
	}
	if !ui.busy {
		if ui.queuePaused && len(ui.queued) > 0 {
			if ui.width < 40 {
				return fmt.Sprintf("Queue paused · Q%d", len(ui.queued))
			}
			return fmt.Sprintf("Queue paused · %d waiting", len(ui.queued))
		}
		return "Ready"
	}
	activity := ui.activity
	if activity == "" {
		activity = "Working"
	}
	elapsed := time.Since(ui.turnStarted).Round(100 * time.Millisecond)
	if len(ui.queued) > 0 {
		if ui.width < 40 {
			return fmt.Sprintf("%s · Q%d", activity, len(ui.queued))
		}
		return fmt.Sprintf("%s · %s · %s", activity, ui.queueSummary(), elapsed)
	}
	return fmt.Sprintf("%s · %s", activity, elapsed)
}

func (ui *memfitTUI) queueSummary() string {
	if len(ui.queued) == 0 {
		return "empty"
	}
	label := fmt.Sprintf("%d waiting", len(ui.queued))
	if ui.queuePaused {
		label += " (paused)"
	}
	return label
}

func (ui *memfitTUI) compactQueueSummary() string {
	if len(ui.queued) == 0 {
		return "queue empty"
	}
	label := fmt.Sprintf("Q%d", len(ui.queued))
	if ui.queuePaused {
		label += " paused"
	}
	return label
}

func (ui *memfitTUI) restoreSavedDraft() {
	if ui.savedDraft == nil {
		return
	}
	ui.buffer = append([]rune(nil), ui.savedDraft...)
	ui.cursor = ui.savedDraftCursor
	if ui.cursor > len(ui.buffer) {
		ui.cursor = len(ui.buffer)
	}
	ui.savedDraft = nil
	ui.savedDraftCursor = 0
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
		// Keep the cursor visible while using the full available width. The old
		// half-width budget wasted space on compact terminals at the end of input.
		budget := maxCells - 1 // reserve the leading ellipsis
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

func sanitizeMemfitTTYText(input string) string {
	return strings.ReplaceAll(sanitizeMemfitTerminalText(input), "\n", "\r\n")
}

func singleLineMemfitText(input string) string {
	return strings.Join(strings.Fields(sanitizeMemfitTerminalText(input)), " ")
}

func lastMemfitPreviewLine(input string) string {
	lines := strings.Split(sanitizeMemfitTerminalText(input), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if line := singleLineMemfitText(lines[index]); line != "" {
			return line
		}
	}
	return ""
}

func titleMemfitStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "Finished"
	}
	runes := []rune(status)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func humanizeMemfitMessage(input string) string {
	input = strings.TrimSpace(sanitizeMemfitTerminalText(input))
	for depth := 0; depth < 3 && input != ""; depth++ {
		var value any
		if json.Unmarshal([]byte(input), &value) != nil {
			break
		}
		readable := findMemfitReadableValue(value)
		if readable == "" || readable == input {
			return "Structured details hidden; use --debug if you need the raw event."
		}
		input = strings.TrimSpace(readable)
	}
	return input
}

func compactMemfitMessage(input string, maxCells int) string {
	message := singleLineMemfitText(humanizeMemfitMessage(input))
	if message == "" {
		return "No readable details were provided."
	}
	return truncateMemfitCells(message, maxInt(16, maxCells))
}

func wrapMemfitCells(input string, maxCells int) []string {
	if maxCells < 1 {
		maxCells = 1
	}
	input = strings.ReplaceAll(input, "\t", "    ")
	paragraphs := strings.Split(input, "\n")
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		remaining := []rune(paragraph)
		preferWords := !strings.HasPrefix(strings.TrimSpace(paragraph), "|") &&
			!strings.HasPrefix(paragraph, "    ")
		for len(remaining) > 0 {
			used, cut, lastSpace := 0, 0, -1
			for index, r := range remaining {
				width := maxInt(1, runewidth.RuneWidth(r))
				if used > 0 && used+width > maxCells {
					break
				}
				used += width
				cut = index + 1
				if preferWords && unicode.IsSpace(r) {
					lastSpace = cut
				}
			}
			if cut >= len(remaining) {
				lines = append(lines, string(remaining))
				break
			}
			if lastSpace > 0 {
				line := strings.TrimRightFunc(string(remaining[:lastSpace]), unicode.IsSpace)
				if line != "" {
					lines = append(lines, line)
				}
				remaining = remaining[lastSpace:]
				for len(remaining) > 0 && unicode.IsSpace(remaining[0]) {
					remaining = remaining[1:]
				}
				continue
			}
			lines = append(lines, string(remaining[:cut]))
			remaining = remaining[cut:]
		}
	}
	return lines
}

func formatMemfitLogLine(input string) (string, string) {
	line := singleLineMemfitText(input)
	if line == "" {
		return "", ""
	}
	if json.Valid([]byte(line)) {
		message := humanizeMemfitMessage(line)
		if strings.HasPrefix(message, "Structured details hidden") {
			return "EVENT", message
		}
		return "EVENT", singleLineMemfitText(message)
	}
	level := "WORKER"
	if strings.HasPrefix(line, "[") {
		if end := strings.IndexByte(line, ']'); end > 1 {
			level = strings.ToUpper(strings.TrimSpace(line[1:end]))
			line = strings.TrimSpace(line[end+1:])
		}
	}
	fields := strings.Fields(line)
	if len(fields) >= 2 && looksLikeMemfitDate(fields[0]) && strings.Contains(fields[1], ":") {
		line = strings.TrimSpace(strings.TrimPrefix(line, fields[0]+" "+fields[1]))
	}
	source := ""
	if strings.HasPrefix(line, "[") {
		if end := strings.IndexByte(line, ']'); end > 1 {
			source = strings.TrimSpace(line[1:end])
			line = strings.TrimSpace(line[end+1:])
		}
	}
	line = humanizeMemfitMessage(line)
	if source != "" {
		line = source + " · " + line
	}
	return truncateMemfitCells(level, 8), singleLineMemfitText(line)
}

func looksLikeMemfitDate(value string) bool {
	return len(value) >= 10 && value[4] == '-' && value[7] == '-'
}

func extractMemfitToolSummary(content string) (string, string) {
	var value any
	if json.Unmarshal([]byte(content), &value) != nil {
		return "Tool", ""
	}
	tool := findMemfitToolObject(value)
	if tool == nil {
		return "Tool", ""
	}
	name := scalarMemfitMapString(tool, "verbose_name", "tool_name", "name")
	if localized, ok := tool["verbose_name_i18n"].(map[string]any); ok {
		if zh := scalarMemfitMapString(localized, "Zh", "zh", "ZH"); zh != "" {
			name = zh
		}
	}
	if name == "" {
		name = "Tool"
	}
	detail := findMemfitToolDetail(value)
	return humanizeMemfitToolName(name), detail
}

func findMemfitToolObject(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		if scalarMemfitMapString(typed, "verbose_name", "tool_name", "name") != "" {
			return typed
		}
		for _, key := range []string{"tool", "function", "metadata", "data"} {
			if child, ok := typed[key]; ok {
				if result := findMemfitToolObject(child); result != nil {
					return result
				}
			}
		}
		for _, child := range typed {
			if result := findMemfitToolObject(child); result != nil {
				return result
			}
		}
	case []any:
		for _, child := range typed {
			if result := findMemfitToolObject(child); result != nil {
				return result
			}
		}
	}
	return nil
}

func scalarMemfitMapString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		switch typed := value[key].(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case json.Number:
			return typed.String()
		case float64:
			return fmt.Sprintf("%v", typed)
		}
	}
	return ""
}

func findMemfitToolDetail(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"path", "file_path", "filename", "command", "query", "url", "target"} {
			if detail := scalarMemfitMapString(typed, key); detail != "" {
				return singleLineMemfitText(detail)
			}
		}
		for _, key := range []string{"params", "arguments", "input", "data"} {
			if child, ok := typed[key]; ok {
				if result := findMemfitToolDetail(child); result != "" {
					return result
				}
			}
		}
	case []any:
		for _, child := range typed {
			if result := findMemfitToolDetail(child); result != "" {
				return result
			}
		}
	}
	return ""
}

func humanizeMemfitToolName(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(name, "_", " "), "-", " "))
	if name == "" {
		return "Tool"
	}
	words := strings.Fields(name)
	for index, word := range words {
		if word == "bash" || word == "HTTP" || word == "URL" {
			continue
		}
		words[index] = titleMemfitStatus(word)
	}
	return strings.Join(words, " ")
}

func describeMemfitEvent(event memfitWorkerEvent) string {
	switch event.Type {
	case string(schema.EVENT_TOOL_CALL_START):
		name, detail := extractMemfitToolSummary(event.Content)
		if detail != "" {
			return "Using " + name + " · " + detail
		}
		return "Using " + name
	case string(schema.EVENT_TOOL_CALL_ERROR):
		message := extractMemfitJSONField(event.Content, "error", "message", "reason")
		if message == "" {
			message = "tool failed"
		}
		return "Tool failed · " + humanizeMemfitMessage(message)
	case string(schema.EVENT_TYPE_API_REQUEST_FAILED):
		message := extractMemfitReadableContent(event.Content)
		if message == "" {
			message = "AI request failed"
		}
		return "AI request failed · " + humanizeMemfitMessage(message)
	case string(schema.EVENT_TYPE_NOTIFY):
		return humanizeMemfitMessage(extractMemfitReadableContent(event.Content))
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
	return scalarMemfitMapString(value, keys...)
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
