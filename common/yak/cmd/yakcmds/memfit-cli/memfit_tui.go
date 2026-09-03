package memfitcli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
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
	memfitMaxProcessItems = 32
	memfitMaxProcessRunes = 16 << 10
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
	memfitKeyToggleProcess
	memfitKeyMouse
	memfitKeyCursorPosition
)

type memfitKey struct {
	kind    memfitKeyKind
	text    string
	row     int
	column  int
	button  int
	pressed bool
}

type memfitQueuedInput struct {
	text     string
	queuedAt time.Time
}

type memfitInfoRow struct {
	label string
	value string
}

type memfitProcessState int

const (
	memfitProcessInfo memfitProcessState = iota
	memfitProcessRunning
	memfitProcessDone
	memfitProcessWarning
	memfitProcessError
)

type memfitProcessItem struct {
	key       string
	kind      string
	detail    string
	state     memfitProcessState
	startedAt time.Time
	updatedAt time.Time
	expanded  bool
}

type memfitLiveLine struct {
	text       string
	color      string
	processKey string
	showMore   bool
}

type memfitProcessHit struct {
	offset     int
	cells      int
	processKey string
	showMore   bool
}

type memfitTUI struct {
	client memfitClient
	config memfitStartConfig
	color  bool
	width  int
	height int

	buffer       []rune
	cursor       int
	history      [][]rune
	historyIndex int
	historyDraft []rune

	busy                bool
	awaitingInput       bool
	interactiveID       string
	queued              []memfitQueuedInput
	queuePaused         bool
	liveRows            int
	liveRowsBelowCursor int
	activity            string
	streamKind          string
	streamWriter        string
	streamPreview       string
	answerSeen          bool
	answers             memfitAnswerStreams
	fallbackResult      string
	turnStarted         time.Time
	lastStatusTick      int64
	lastModel           string
	lastCtrlC           time.Time
	savedDraft          []rune
	savedDraftCursor    int

	processItems       []memfitProcessItem
	processShowAll     bool
	processSelectedKey string
	processHits        []memfitProcessHit
	pendingMouse       *memfitKey
	pendingProcessHits []memfitProcessHit
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
		fmt.Fprint(os.Stdout, "\x1b[?1006l\x1b[?1000l\x1b[?2004l")
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
		fmt.Fprintln(os.Stdout)
	}()

	width, height, _ := term.GetSize(int(os.Stdout.Fd()))
	ui = &memfitTUI{
		client:       client,
		config:       config,
		color:        os.Getenv("NO_COLOR") == "",
		width:        normalizeMemfitWidth(width),
		height:       normalizeMemfitHeight(height),
		historyIndex: -1,
	}
	ui.printHeader()
	// Button-event + SGR mouse tracking makes activity rows clickable without
	// taking over the alternate screen. Shift-click remains
	// available for native terminal text selection.
	fmt.Fprint(os.Stdout, "\x1b[?2004h\x1b[?1000h\x1b[?1006h")

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
			width, height, sizeErr := term.GetSize(int(os.Stdout.Fd()))
			resized := sizeErr == nil && (normalizeMemfitWidth(width) != ui.width || normalizeMemfitHeight(height) != ui.height)
			if resized {
				ui.width = normalizeMemfitWidth(width)
				ui.height = normalizeMemfitHeight(height)
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
		hint = "Enter send · Alt+Enter newline · click/^O details · Ctrl+C stop · /help"
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
	case memfitKeyToggleProcess:
		ui.toggleSelectedProcessItem()
		return false, nil
	case memfitKeyMouse:
		if key.pressed && key.button&64 == 0 && key.button&3 == 0 && len(ui.processHits) > 0 {
			pending := key
			ui.pendingMouse = &pending
			ui.pendingProcessHits = append([]memfitProcessHit(nil), ui.processHits...)
			// Ask the terminal for the composer's current row. This avoids
			// assuming that a normal-screen TUI is anchored to the viewport
			// bottom and keeps clicks correct before scrollback fills.
			fmt.Fprint(os.Stdout, "\x1b[6n")
		}
		return false, nil
	case memfitKeyCursorPosition:
		if ui.pendingMouse != nil {
			for _, hit := range ui.pendingProcessHits {
				clickedRow := key.row - hit.offset
				if ui.pendingMouse.row != clickedRow || ui.pendingMouse.column > maxInt(1, hit.cells) {
					continue
				}
				if hit.showMore {
					ui.processShowAll = !ui.processShowAll
				} else {
					ui.toggleProcessItem(hit.processKey)
				}
				break
			}
			ui.pendingMouse = nil
			ui.pendingProcessHits = nil
			ui.renderComposer()
		}
		return false, nil
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
			"Click an activity to open its details",
			"Ctrl+O        toggle the current activity",
			"/process      toggle current activity",
			"/process all  show earlier activities",
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
				"click / ^O  activity details",
				"/process all  earlier activity",
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
				"click / ^O details",
				"/status /queue /mode",
				"/logs /clear /exit",
				"Enter queues while busy.",
			}
		}
		ui.printTextSection("Commands", lines)
		return true, false, nil
	case "/process":
		if len(parts) == 2 {
			switch strings.ToLower(parts[1]) {
			case "open", "show", "on", "expand":
				ui.expandSelectedProcessItem()
			case "close", "hide", "off", "collapse":
				ui.collapseProcessItems()
			case "all":
				ui.processShowAll = true
			default:
				ui.printNotice("process", "Usage: /process [open|close|all]", memfitColorYellow)
				return true, false, nil
			}
		} else {
			ui.toggleSelectedProcessItem()
		}
		return true, false, nil
	case "/thinking", "/think":
		key := ui.latestProcessKindKey("Thinking")
		if key == "" {
			ui.printNotice("thinking", "No thinking stream is available yet", memfitColorDim)
			return true, false, nil
		}
		if len(parts) == 2 {
			switch strings.ToLower(parts[1]) {
			case "open", "show", "on", "expand":
				ui.processSelectedKey = key
				for index := range ui.processItems {
					ui.processItems[index].expanded = ui.processItems[index].key == key
				}
			case "close", "hide", "off", "collapse":
				if index := ui.processItemIndex(key); index >= 0 {
					ui.processItems[index].expanded = false
				}
			default:
				ui.printNotice("thinking", "Usage: /thinking [open|close]", memfitColorYellow)
				return true, false, nil
			}
		} else {
			ui.toggleProcessItem(key)
		}
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
		ui.processItems = nil
		ui.processShowAll = false
		ui.processSelectedKey = ""
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
	ui.processItems = nil
	ui.processShowAll = false
	ui.processSelectedKey = ""
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
		ui.finishProcessItems(memfitProcessError)
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
		ui.finishProcessItems(memfitProcessDone)
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
		ui.finishProcessKind("Thinking", memfitProcessDone)
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

	if isMemfitThoughtStream(event) && event.StreamDelta != "" {
		ui.activity = "Thinking"
		ui.appendProcessThought(envelopeID, event.StreamDelta)
		if ui.streamKind != "answer" {
			ui.streamKind = ""
			ui.streamPreview = ""
		}
		ui.renderComposer()
		return
	}

	processChanged := ui.recordProcessEvent(envelopeID, event)

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
			ui.activity = singleLineMemfitText("Review · " + question)
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

	if processChanged {
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

func (ui *memfitTUI) toggleSelectedProcessItem() {
	key := ui.processSelectedKey
	if key == "" || ui.processItemIndex(key) < 0 {
		key = ui.latestExpandableProcessKey()
	}
	if key != "" {
		ui.toggleProcessItem(key)
	}
	ui.renderComposer()
}

func (ui *memfitTUI) expandSelectedProcessItem() {
	key := ui.processSelectedKey
	if key == "" || ui.processItemIndex(key) < 0 {
		key = ui.latestExpandableProcessKey()
	}
	if index := ui.processItemIndex(key); index >= 0 {
		for other := range ui.processItems {
			ui.processItems[other].expanded = false
		}
		ui.processItems[index].expanded = true
		ui.processSelectedKey = key
	}
}

func (ui *memfitTUI) collapseProcessItems() {
	for index := range ui.processItems {
		ui.processItems[index].expanded = false
	}
	ui.processShowAll = false
}

func (ui *memfitTUI) toggleProcessItem(key string) {
	if index := ui.processItemIndex(key); index >= 0 {
		opening := !ui.processItems[index].expanded
		for other := range ui.processItems {
			ui.processItems[other].expanded = false
		}
		ui.processItems[index].expanded = opening
		ui.processSelectedKey = key
	}
}

func (ui *memfitTUI) processItemIndex(key string) int {
	for index := range ui.processItems {
		if ui.processItems[index].key == key {
			return index
		}
	}
	return -1
}

func (ui *memfitTUI) latestExpandableProcessKey() string {
	for index := len(ui.processItems) - 1; index >= 0; index-- {
		if hasMemfitProcessDetails(ui.processItems[index], ui.width) {
			return ui.processItems[index].key
		}
	}
	return ""
}

func (ui *memfitTUI) latestProcessKindKey(kind string) string {
	for index := len(ui.processItems) - 1; index >= 0; index-- {
		if ui.processItems[index].kind == kind {
			return ui.processItems[index].key
		}
	}
	return ""
}

func (ui *memfitTUI) appendProcessThought(writerID, delta string) {
	writerID = strings.TrimSpace(writerID)
	if writerID == "" {
		writerID = "default"
	}
	key := "thought:" + writerID
	clean := sanitizeMemfitTerminalText(delta)
	if clean == "" {
		return
	}
	for index := range ui.processItems {
		if ui.processItems[index].key != key {
			continue
		}
		ui.processItems[index].detail = appendMemfitProcessStream(ui.processItems[index].detail, clean)
		ui.processItems[index].state = memfitProcessRunning
		ui.processItems[index].updatedAt = time.Now()
		return
	}
	if len(ui.processItems) > 0 {
		last := &ui.processItems[len(ui.processItems)-1]
		if last.kind == "Thinking" && last.state == memfitProcessRunning {
			last.detail = appendMemfitProcessStream(last.detail, clean)
			last.updatedAt = time.Now()
			return
		}
	}
	ui.finishOtherProcessItems(key, "Thinking", memfitProcessDone)
	ui.upsertProcessItem(memfitProcessItem{
		key:       key,
		kind:      "Thinking",
		detail:    trimMemfitProcessText(clean),
		state:     memfitProcessRunning,
		startedAt: time.Now(),
		updatedAt: time.Now(),
	})
}

func (ui *memfitTUI) recordProcessEvent(envelopeID string, event memfitWorkerEvent) bool {
	typeName := schema.EventType(event.Type)
	key := memfitProcessEventKey(envelopeID, event)
	add := func(kind, detail, activity string, state memfitProcessState) bool {
		detail = trimMemfitProcessText(humanizeMemfitProcessDetail(detail))
		if kind != "Thinking" {
			ui.finishProcessKind("Thinking", memfitProcessDone)
		}
		ui.upsertProcessItem(memfitProcessItem{
			key:       key,
			kind:      kind,
			detail:    detail,
			state:     state,
			startedAt: time.Now(),
			updatedAt: time.Now(),
		})
		if activity != "" {
			ui.activity = singleLineMemfitText(activity)
		}
		return true
	}

	switch typeName {
	case schema.EVENT_TYPE_THOUGHT:
		detail := extractMemfitJSONField(event.Content, "thought", "content", "message")
		if detail == "" {
			detail = extractMemfitReadableContent(event.Content)
		}
		return add("Thinking", detail, "Thinking", memfitProcessDone)
	case schema.EVENT_TYPE_INTENT_RECOGNITION:
		key = "process:intent"
		intent := extractMemfitJSONField(event.Content, "intent", "summary", "message")
		matched := extractMemfitJSONList(event.Content, "matched_tool_names", "matched_forge_names", "matched_skill_names")
		detail := intent
		if matched != "" {
			detail = joinMemfitProcessDetail(detail, "Matched: "+matched)
		}
		return add("Intent", detail, "Intent recognized", memfitProcessDone)
	case schema.EVENT_TYPE_PERCEPTION:
		key = "process:understanding"
		detail := extractMemfitJSONField(event.Content, "summary", "intent", "message")
		shift := extractMemfitJSONField(event.Content, "intent_shift")
		if shift != "" && shift != "none" {
			detail = joinMemfitProcessDetail(detail, "Shift: "+shift)
		}
		return add("Understanding", detail, "Understanding context", memfitProcessDone)
	case schema.EVENT_TYPE_PERCEPTION_CAPABILITY:
		key = "process:capabilities"
		matched := extractMemfitJSONList(event.Content, "matched_tool_names", "matched_forge_names", "matched_skill_names", "matched_focus_mode_names")
		if matched == "" {
			matched = extractMemfitJSONField(event.Content, "query", "message")
		}
		return add("Capabilities", matched, "Selecting capabilities", memfitProcessDone)
	case schema.EVENT_TYPE_PERCEPTION_KNOWLEDGE:
		key = "process:knowledge-selection"
		bases := extractMemfitJSONList(event.Content, "knowledge_bases")
		detail := extractMemfitJSONField(event.Content, "query")
		if bases != "" {
			detail = joinMemfitProcessDetail(detail, "Sources: "+bases)
		}
		return add("Knowledge", detail, "Checking knowledge", memfitProcessDone)
	case schema.EVENT_TYPE_ACTION:
		detail := extractMemfitJSONField(event.Content, "action", "message")
		actionType := extractMemfitJSONField(event.Content, "action_type", "type")
		if actionType != "" {
			detail = joinMemfitProcessDetail(actionType, detail)
		}
		return add("Action", detail, "Acting", memfitProcessRunning)
	case schema.EVENT_TYPE_OBSERVATION:
		ui.finishProcessKind("Action", memfitProcessDone)
		detail := extractMemfitJSONField(event.Content, "observation", "summary", "message")
		source := extractMemfitJSONField(event.Content, "source")
		if source != "" {
			detail = joinMemfitProcessDetail(detail, "Source: "+source)
		}
		return add("Observation", detail, "Reviewing result", memfitProcessDone)
	case schema.EVENT_TYPE_ITERATION:
		current := extractMemfitJSONField(event.Content, "current")
		maximum := extractMemfitJSONField(event.Content, "max")
		detail := extractMemfitJSONField(event.Content, "description", "message")
		position := strings.Trim(strings.Join([]string{current, maximum}, "/"), "/")
		if position != "" {
			detail = joinMemfitProcessDetail("Iteration "+position, detail)
		}
		return add("Progress", detail, "Iterating", memfitProcessDone)
	case schema.EVENT_TYPE_PLAN, schema.EVENT_PLAN_TASK_ANALYSIS,
		schema.EVENT_TYPE_START_PLAN_AND_EXECUTION, schema.EVENT_TYPE_END_PLAN_AND_EXECUTION:
		key = "process:plan"
		detail := extractMemfitReadableContent(event.Content)
		if detail == "" {
			detail = "Execution plan updated"
		}
		return add("Plan", detail, "Planning", memfitProcessDone)
	case schema.EVENT_TOOL_CALL_START:
		name, detail := extractMemfitToolSummary(event.Content)
		display := name
		if detail != "" {
			display += " · " + detail
		}
		kind := memfitToolActivityKindFromContent(event.Content, name)
		return add(kind, display, memfitToolActivityVerb(kind)+" "+display, memfitProcessRunning)
	case schema.EVENT_TOOL_CALL_REASON:
		reason := extractMemfitJSONField(event.Content, "reason", "summary", "message")
		if reason != "" {
			ui.appendProcessItemDetail(key, "Why: "+reason)
		}
		return reason != ""
	case schema.EVENT_TOOL_CALL_PARAM:
		target := extractMemfitToolDetail(event.Content)
		if target == "" {
			return false
		}
		ui.appendProcessItemTarget(key, target)
		return true
	case schema.EVENT_TOOL_CALL_DECISION:
		decision := extractMemfitJSONField(event.Content, "summary", "action", "message")
		if decision != "" {
			ui.appendProcessItemDetail(key, "Decision: "+decision)
		}
		return decision != ""
	case schema.EVENT_TOOL_CALL_STATUS:
		status := extractMemfitJSONField(event.Content, "status")
		state := memfitProcessRunning
		if status == schema.TOOL_CALL_STATUS_DONE {
			state = memfitProcessDone
		}
		ui.updateProcessItemState(key, state)
		if status != "" {
			ui.activity = "Tool · " + strings.ReplaceAll(status, "_", " ")
		}
		return true
	case schema.EVENT_TOOL_CALL_PROGRESS_REVIEW:
		phase := extractMemfitJSONField(event.Content, "phase")
		decision := extractMemfitJSONField(event.Content, "decision", "error")
		detail := strings.TrimSpace(strings.Join([]string{phase, decision}, " · "))
		if detail != "" {
			ui.appendProcessItemDetail(key, "Progress check: "+detail)
		}
		return true
	case schema.EVENT_TOOL_CALL_SUMMARY:
		summary := extractMemfitJSONField(event.Content, "summary", "message")
		ui.appendProcessItemDetail(key, summary)
		return summary != ""
	case schema.EVENT_TOOL_CALL_RESULT, schema.EVENT_TOOL_CALL_DONE:
		if preview := extractMemfitToolResultPreview(event.Content); preview != "" {
			ui.appendProcessItemDetail(key, preview)
		}
		ui.updateProcessItemState(key, memfitProcessDone)
		ui.activity = "Tool completed"
		return true
	case schema.EVENT_TOOL_CALL_USER_CANCEL:
		ui.updateProcessItemState(key, memfitProcessWarning)
		ui.activity = "Tool cancelled"
		return true
	case schema.EVENT_TOOL_CALL_ERROR:
		message := extractMemfitJSONField(event.Content, "error", "message", "reason")
		ui.appendProcessItemDetail(key, message)
		ui.updateProcessItemState(key, memfitProcessError)
		return true
	case schema.EVENT_TYPE_KNOWLEDGE, schema.EVENT_TYPE_TASK_ABOUT_KNOWLEDGE:
		detail := extractMemfitNestedJSONField(event.Content, "Title", "title", "Source", "source")
		if detail == "" {
			detail = "Relevant context found"
		}
		return add("Knowledge", detail, "Using knowledge", memfitProcessDone)
	case schema.EVENT_TYPE_MEMORY_SEARCH_QUICKLY, schema.EVENT_TYPE_MEMORY_SEARCH_SPECIFIC:
		key = "process:memory"
		return add("Memory", "Searching previous context", "Searching memory", memfitProcessRunning)
	case schema.EVENT_TYPE_MEMORY_BUILD, schema.EVENT_TYPE_MEMORY_SAVE, schema.EVENT_TYPE_MEMORY_ADD_CONTEXT:
		key = "process:memory"
		return add("Memory", "Context updated", "Updating memory", memfitProcessDone)
	case schema.EVENT_TYPE_NOTIFY:
		detail := extractMemfitReadableContent(event.Content)
		if detail == "" {
			return false
		}
		ui.activity = singleLineMemfitText(detail)
		return true
	case schema.EVENT_TYPE_API_REQUEST_FAILED:
		return add("AI request", "A provider request failed; Memfit may retry.", "AI request failed", memfitProcessError)
	case schema.EVENT_TYPE_TODO_LIST_UPDATE, schema.EVENT_TYPE_CURRENT_TASK_TODO_LIST_UPDATE:
		key = "process:tasks"
		detail := formatMemfitTodoStats(event.Content)
		if detail == "" {
			detail = "Task list updated"
		}
		return add("Tasks", detail, "Updating tasks", memfitProcessDone)
	case schema.EVENT_TYPE_STRUCTURED:
		switch event.NodeID {
		case "timeline_item":
			kind, group, detail, state, ok := classifyMemfitTimelineItem(event.Content)
			if !ok {
				return false
			}
			key = "process:" + group
			return add(kind, detail, memfitTimelineActivity(kind, state), state)
		case "react_task_status_changed":
			status := extractMemfitJSONField(event.Content, "react_task_now_status", "status")
			if status == "" {
				return false
			}
			ui.activity = "Task " + strings.ReplaceAll(status, "_", " ")
			return true
		case "status", "task-review-decision", "plan-review-decision":
			detail := extractMemfitStructuredStatus(event.Content)
			if detail == "" {
				return false
			}
			ui.activity = singleLineMemfitText(detail)
			return true
		case "plan_exec_tasks":
			ui.activity = "Plan updated"
			return true
		}
	}
	return false
}

func (ui *memfitTUI) upsertProcessItem(item memfitProcessItem) {
	if item.key == "" {
		item.key = fmt.Sprintf("%s:%d", item.kind, time.Now().UnixNano())
	}
	item.detail = trimMemfitProcessText(item.detail)
	for index := range ui.processItems {
		if ui.processItems[index].key != item.key {
			continue
		}
		if item.kind != "" {
			ui.processItems[index].kind = item.kind
		}
		if item.detail != "" {
			ui.processItems[index].detail = item.detail
		}
		ui.processItems[index].state = item.state
		ui.processItems[index].updatedAt = item.updatedAt
		return
	}
	if len(ui.processItems) > 0 {
		last := &ui.processItems[len(ui.processItems)-1]
		if last.kind == item.kind && item.detail != "" && last.detail == item.detail {
			last.state = item.state
			last.updatedAt = item.updatedAt
			return
		}
	}
	if item.startedAt.IsZero() {
		item.startedAt = item.updatedAt
	}
	ui.processItems = append(ui.processItems, item)
	if len(ui.processItems) > memfitMaxProcessItems {
		ui.processItems = append([]memfitProcessItem(nil), ui.processItems[len(ui.processItems)-memfitMaxProcessItems:]...)
	}
}

func (ui *memfitTUI) appendProcessItemDetail(key, detail string) {
	detail = trimMemfitProcessText(humanizeMemfitProcessDetail(detail))
	if detail == "" {
		return
	}
	for index := range ui.processItems {
		if ui.processItems[index].key == key {
			ui.processItems[index].detail = appendMemfitProcessText(ui.processItems[index].detail, detail)
			ui.processItems[index].updatedAt = time.Now()
			return
		}
	}
	now := time.Now()
	ui.upsertProcessItem(memfitProcessItem{key: key, kind: "Tool", detail: detail, state: memfitProcessRunning, startedAt: now, updatedAt: now})
}

func (ui *memfitTUI) appendProcessItemTarget(key, target string) {
	target = trimMemfitProcessText(target)
	if target == "" {
		return
	}
	for index := range ui.processItems {
		if ui.processItems[index].key != key {
			continue
		}
		lines := strings.SplitN(ui.processItems[index].detail, "\n", 2)
		summary := strings.TrimSpace(lines[0])
		compact := truncateMemfitCells(singleLineMemfitText(target), 160)
		if compact != "" && !strings.Contains(summary, compact) {
			summary += " · " + compact
		}
		rest := ""
		if len(lines) == 2 {
			rest = lines[1]
		}
		rest = appendMemfitProcessText(rest, "Input:\n"+target)
		ui.processItems[index].detail = joinMemfitProcessDetail(summary, rest)
		ui.processItems[index].updatedAt = time.Now()
		return
	}
	ui.appendProcessItemDetail(key, "Input:\n"+target)
}

func (ui *memfitTUI) updateProcessItemState(key string, state memfitProcessState) {
	for index := range ui.processItems {
		if ui.processItems[index].key == key {
			ui.processItems[index].state = state
			ui.processItems[index].updatedAt = time.Now()
			return
		}
	}
	now := time.Now()
	ui.upsertProcessItem(memfitProcessItem{key: key, kind: "Tool", state: state, startedAt: now, updatedAt: now})
}

func (ui *memfitTUI) finishProcessItems(state memfitProcessState) {
	for index := range ui.processItems {
		if ui.processItems[index].state == memfitProcessRunning {
			ui.processItems[index].state = state
			ui.processItems[index].updatedAt = time.Now()
		}
	}
}

func (ui *memfitTUI) finishProcessKind(kind string, state memfitProcessState) {
	for index := range ui.processItems {
		if ui.processItems[index].kind == kind && ui.processItems[index].state == memfitProcessRunning {
			ui.processItems[index].state = state
			ui.processItems[index].updatedAt = time.Now()
		}
	}
}

func (ui *memfitTUI) finishOtherProcessItems(key, kind string, state memfitProcessState) {
	for index := range ui.processItems {
		if ui.processItems[index].key != key && ui.processItems[index].kind == kind && ui.processItems[index].state == memfitProcessRunning {
			ui.processItems[index].state = state
			ui.processItems[index].updatedAt = time.Now()
		}
	}
}

func (ui *memfitTUI) processPanelLines() []memfitLiveLine {
	if !ui.busy && len(ui.processItems) == 0 {
		return nil
	}
	if len(ui.processItems) == 0 {
		return nil
	}

	visible := 6
	if ui.width < 48 {
		visible = 4
	}
	if ui.width < 30 {
		visible = 3
	}
	start := 0
	if !ui.processShowAll && len(ui.processItems) > visible {
		start = len(ui.processItems) - visible
	}
	selectedKey := ui.processSelectedKey
	if selectedKey == "" || ui.processItemIndex(selectedKey) < 0 {
		selectedKey = ui.currentProcessKey()
	}
	groups := make([][]memfitLiveLine, 0, len(ui.processItems)-start)
	groupKeys := make([]string, 0, len(ui.processItems)-start)
	for index := start; index < len(ui.processItems); index++ {
		groups = append(groups, ui.processItemLines(ui.processItems[index], ui.processItems[index].key == selectedKey))
		groupKeys = append(groupKeys, ui.processItems[index].key)
	}

	budget := ui.height / 2
	if ui.processShowAll {
		budget = ui.height - 7
	}
	if budget < 4 {
		budget = 4
	}
	if budget > 18 {
		budget = 18
	}
	used := 0
	for _, group := range groups {
		used += len(group)
	}
	omitted := start
	for used > budget && len(groups) > 1 {
		if groupKeys[0] == selectedKey {
			last := len(groups) - 1
			used -= len(groups[last])
			groups = groups[:last]
			groupKeys = groupKeys[:last]
		} else {
			used -= len(groups[0])
			groups = groups[1:]
			groupKeys = groupKeys[1:]
		}
		omitted++
	}
	if len(groups) == 1 && len(groups[0]) > budget {
		group := groups[0]
		cropped := append([]memfitLiveLine(nil), group[:budget-1]...)
		cropped = append(cropped, memfitLiveLine{text: "      … more hidden", color: memfitColorDim})
		groups[0] = cropped
		used = len(cropped)
	}
	for omitted > 0 && used+1 > budget && len(groups) > 1 {
		if groupKeys[0] == selectedKey {
			last := len(groups) - 1
			used -= len(groups[last])
			groups = groups[:last]
			groupKeys = groupKeys[:last]
		} else {
			used -= len(groups[0])
			groups = groups[1:]
			groupKeys = groupKeys[1:]
		}
		omitted++
	}
	if omitted > 0 && len(groups) == 1 && len(groups[0])+1 > budget {
		limit := maxInt(1, budget-1)
		group := groups[0]
		cropped := append([]memfitLiveLine(nil), group[:limit]...)
		if limit > 1 {
			cropped[limit-1] = memfitLiveLine{text: "      … more hidden", color: memfitColorDim}
		}
		groups[0] = cropped
		used = len(cropped)
	}

	lines := make([]memfitLiveLine, 0, used+1)
	if omitted > 0 {
		label := fmt.Sprintf("  ◇ %d hidden · click to show", omitted)
		if ui.width < 38 {
			label = fmt.Sprintf("  ◇ %d more", omitted)
		}
		lines = append(lines, memfitLiveLine{text: label, color: memfitColorDim, showMore: true})
	} else if ui.processShowAll && len(ui.processItems) > visible {
		lines = append(lines, memfitLiveLine{text: "  ◇ Show less", color: memfitColorDim, showMore: true})
	}
	for _, group := range groups {
		lines = append(lines, group...)
	}
	return lines
}

func (ui *memfitTUI) currentProcessKey() string {
	for index := len(ui.processItems) - 1; index >= 0; index-- {
		if ui.processItems[index].state == memfitProcessRunning {
			return ui.processItems[index].key
		}
	}
	if len(ui.processItems) > 0 {
		return ui.processItems[len(ui.processItems)-1].key
	}
	return ""
}

func (ui *memfitTUI) processItemLines(item memfitProcessItem, selected bool) []memfitLiveLine {
	marker, color := memfitProcessStateStyle(item.state)
	detail := strings.TrimSpace(item.detail)
	disclosure := " "
	hasDetails := hasMemfitProcessDetails(item, ui.width)
	if hasDetails {
		disclosure = "▸"
		if item.expanded {
			disclosure = "▾"
		}
	}
	prefix := "  "
	if selected {
		prefix = "› "
		color = memfitColorBold + color
	}
	title := prefix + marker + " " + item.kind + " " + disclosure
	if item.kind != "Thinking" && detail != "" {
		if summary := firstMemfitProcessLine(detail); summary != "" {
			title += " · " + summary
		}
	}
	if elapsed := memfitProcessElapsed(item); elapsed != "" {
		if item.state == memfitProcessRunning {
			title += " for " + elapsed
		} else {
			title += " · " + elapsed
		}
	}
	lines := []memfitLiveLine{{text: title, color: color, processKey: item.key}}
	if !hasDetails || !item.expanded {
		return lines
	}
	expandedDetail := memfitExpandedProcessDetail(item)
	if expandedDetail == "" {
		return lines
	}
	wrapped := wrapMemfitCells(expandedDetail, maxInt(8, ui.width-5))
	limit := 5
	if item.kind == "Thinking" {
		limit = 10
	}
	if len(wrapped) > limit {
		wrapped = append(wrapped[:limit-1], "… more hidden")
	}
	for _, line := range wrapped {
		lines = append(lines, memfitLiveLine{text: "      " + line, color: memfitProcessDetailColor(line)})
	}
	return lines
}

func hasMemfitProcessDetails(item memfitProcessItem, width int) bool {
	detail := strings.TrimSpace(item.detail)
	if detail == "" {
		return false
	}
	if item.kind == "Thinking" || strings.Contains(detail, "\n") {
		return true
	}
	return runewidth.StringWidth(firstMemfitProcessLine(detail)) > maxInt(18, width/2)
}

func memfitExpandedProcessDetail(item memfitProcessItem) string {
	detail := strings.TrimSpace(item.detail)
	if item.kind == "Thinking" {
		return detail
	}
	parts := strings.SplitN(detail, "\n", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return detail
}

func firstMemfitProcessLine(detail string) string {
	for _, line := range strings.Split(detail, "\n") {
		if line = singleLineMemfitText(line); line != "" {
			return line
		}
	}
	return ""
}

func memfitProcessElapsed(item memfitProcessItem) string {
	if item.startedAt.IsZero() {
		return ""
	}
	end := item.updatedAt
	if item.state == memfitProcessRunning {
		end = time.Now()
	}
	elapsed := end.Sub(item.startedAt)
	if elapsed < time.Second {
		return ""
	}
	return elapsed.Round(100 * time.Millisecond).String()
}

func memfitProcessDetailColor(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "+") && !strings.HasPrefix(trimmed, "+++") {
		return memfitColorGreen
	}
	if strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "---") {
		return memfitColorRed
	}
	if strings.HasPrefix(trimmed, "@@") {
		return memfitColorCyan
	}
	return memfitColorDim
}

func memfitProcessStateStyle(state memfitProcessState) (string, string) {
	switch state {
	case memfitProcessRunning:
		return "◆", memfitColorCyan
	case memfitProcessDone:
		return "✓", memfitColorGreen
	case memfitProcessWarning:
		return "!", memfitColorYellow
	case memfitProcessError:
		return "×", memfitColorRed
	default:
		return "•", memfitColorDim
	}
}

func (ui *memfitTUI) renderComposer() {
	ui.clearLiveFrame()
	rows := 0
	type pendingHit struct {
		row int
		memfitProcessHit
	}
	pendingHits := make([]pendingHit, 0, 8)
	ui.processHits = nil
	for _, line := range ui.processPanelLines() {
		plain := truncateMemfitCells(line.text, ui.width-1)
		fmt.Fprintf(os.Stdout, "%s\r\n", ui.paint(line.color, plain))
		if line.processKey != "" || line.showMore {
			pendingHits = append(pendingHits, pendingHit{
				row: rows,
				memfitProcessHit: memfitProcessHit{
					cells:      runewidth.StringWidth(plain),
					processKey: line.processKey,
					showMore:   line.showMore,
				},
			})
		}
		rows++
	}
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
	fmt.Fprint(os.Stdout, "\r\n")
	composerRow := rows
	rows++

	dividerWidth := maxInt(1, ui.width-1)
	fmt.Fprintf(os.Stdout, "%s\r\n", ui.paint(memfitColorDim, strings.Repeat("─", dividerWidth)))
	rows++

	statusMarker, statusColor := "○", memfitColorDim
	if ui.busy {
		statusMarker, statusColor = "◆", memfitColorCyan
	}
	if ui.awaitingInput {
		statusMarker, statusColor = "?", memfitColorYellow
	}
	ui.renderFooter(statusMarker, statusColor)
	rows++

	// Keep the editing cursor in the composer while retaining two persistent
	// footer rows beneath it. clearLiveFrame knows how to descend to the footer
	// before erasing the complete live frame on the next update.
	fmt.Fprint(os.Stdout, "\r\x1b[2A")
	if column := runewidth.StringWidth(prefix) + cursorCells; column > 0 {
		fmt.Fprintf(os.Stdout, "\x1b[%dC", column)
	}
	ui.liveRows = rows
	ui.liveRowsBelowCursor = 2
	for _, hit := range pendingHits {
		hit.offset = composerRow - hit.row
		ui.processHits = append(ui.processHits, hit.memfitProcessHit)
	}
}

func (ui *memfitTUI) renderFooter(marker, color string) {
	left, right := ui.footerSegments(marker)
	maxWidth := maxInt(1, ui.width-1)
	if right == "" {
		fmt.Fprint(os.Stdout, ui.paint(color, truncateMemfitCells(left, maxWidth)))
		return
	}
	gap := maxWidth - runewidth.StringWidth(left) - runewidth.StringWidth(right)
	if gap < 1 {
		fmt.Fprint(os.Stdout, ui.paint(color, truncateMemfitCells(left, maxWidth)))
		return
	}
	fmt.Fprint(os.Stdout, ui.paint(color, left))
	fmt.Fprint(os.Stdout, strings.Repeat(" ", gap))
	fmt.Fprint(os.Stdout, ui.paint(memfitColorDim, right))
}

func (ui *memfitTUI) footerSegments(marker string) (string, string) {
	maxWidth := maxInt(1, ui.width-1)
	left := truncateMemfitCells(marker+" "+ui.liveStatusSummary(), maxWidth)
	model := ui.config.Model
	if model == "" {
		model = ui.config.AIType
	}
	if model == "" {
		model = "configured model"
	}
	mode := strings.ToUpper(ui.config.ReviewPolicy)
	candidates := []string{
		model + " · " + mode + " · click/Ctrl+O details · /help",
		model + " · " + mode + " · ^O details",
		model + " · " + mode,
		mode + " · ^O details",
		mode,
	}
	for _, right := range candidates {
		if runewidth.StringWidth(left)+3+runewidth.StringWidth(right) <= maxWidth {
			return left, right
		}
	}
	return left, ""
}

func (ui *memfitTUI) clearLiveFrame() {
	if ui.liveRows == 0 {
		return
	}
	fmt.Fprint(os.Stdout, "\r")
	if ui.liveRowsBelowCursor > 0 {
		fmt.Fprintf(os.Stdout, "\x1b[%dB", ui.liveRowsBelowCursor)
	}
	fmt.Fprint(os.Stdout, "\x1b[2K")
	for row := 1; row < ui.liveRows; row++ {
		fmt.Fprint(os.Stdout, "\x1b[1A\r\x1b[2K")
	}
	ui.liveRows = 0
	ui.liveRowsBelowCursor = 0
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
	if elapsed := memfitVisibleElapsed(ui.turnStarted); elapsed != "" {
		return activity + " · " + elapsed
	}
	return activity
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
	elapsed := memfitVisibleElapsed(ui.turnStarted)
	if len(ui.queued) > 0 {
		if ui.width < 40 {
			return fmt.Sprintf("%s · Q%d", activity, len(ui.queued))
		}
		if elapsed != "" {
			return fmt.Sprintf("%s · %s · %s", activity, ui.queueSummary(), elapsed)
		}
		return fmt.Sprintf("%s · %s", activity, ui.queueSummary())
	}
	if elapsed != "" {
		return fmt.Sprintf("%s · %s", activity, elapsed)
	}
	return activity
}

func memfitVisibleElapsed(start time.Time) string {
	if start.IsZero() {
		return ""
	}
	elapsed := time.Since(start)
	if elapsed < time.Second {
		return ""
	}
	return elapsed.Round(100 * time.Millisecond).String()
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
	case 0x0f:
		return memfitKey{kind: memfitKeyToggleProcess}, nil
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
	if key, ok := parseMemfitTerminalReport(code); ok {
		return key, nil
	}
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

func parseMemfitTerminalReport(code string) (memfitKey, bool) {
	if strings.HasSuffix(code, "R") {
		parts := strings.Split(strings.TrimSuffix(code, "R"), ";")
		if len(parts) == 2 {
			row, rowErr := strconv.Atoi(parts[0])
			column, columnErr := strconv.Atoi(parts[1])
			if rowErr == nil && columnErr == nil && row > 0 && column > 0 {
				return memfitKey{kind: memfitKeyCursorPosition, row: row, column: column}, true
			}
		}
	}
	if len(code) >= 3 && code[0] == '<' && (code[len(code)-1] == 'M' || code[len(code)-1] == 'm') {
		parts := strings.Split(code[1:len(code)-1], ";")
		if len(parts) == 3 {
			button, buttonErr := strconv.Atoi(parts[0])
			column, columnErr := strconv.Atoi(parts[1])
			row, rowErr := strconv.Atoi(parts[2])
			if buttonErr == nil && columnErr == nil && rowErr == nil && column > 0 && row > 0 {
				return memfitKey{
					kind:    memfitKeyMouse,
					row:     row,
					column:  column,
					button:  button,
					pressed: code[len(code)-1] == 'M',
				}, true
			}
		}
	}
	return memfitKey{}, false
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

func humanizeMemfitProcessDetail(input string) string {
	input = strings.TrimSpace(sanitizeMemfitTerminalText(input))
	if input == "" {
		return ""
	}
	if json.Valid([]byte(input)) {
		return humanizeMemfitMessage(input)
	}
	return input
}

func trimMemfitProcessText(input string) string {
	input = strings.TrimSpace(sanitizeMemfitTerminalText(input))
	runes := []rune(input)
	if len(runes) <= memfitMaxProcessRunes {
		return input
	}
	return "…\n" + strings.TrimSpace(string(runes[len(runes)-memfitMaxProcessRunes:]))
}

func appendMemfitProcessStream(current, delta string) string {
	return trimMemfitProcessText(current + sanitizeMemfitTerminalText(delta))
}

func appendMemfitProcessText(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" {
		return trimMemfitProcessText(next)
	}
	if next == "" || strings.Contains(current, next) {
		return trimMemfitProcessText(current)
	}
	return trimMemfitProcessText(current + "\n" + next)
}

func joinMemfitProcessDetail(parts ...string) string {
	result := ""
	for _, part := range parts {
		result = appendMemfitProcessText(result, part)
	}
	return result
}

func memfitProcessEventKey(envelopeID string, event memfitWorkerEvent) string {
	id := strings.TrimSpace(event.CallToolID)
	if id == "" {
		id = extractMemfitJSONField(event.Content, "call_tool_id")
	}
	if id == "" {
		id = strings.TrimSpace(envelopeID)
	}
	if id == "" {
		id = strings.TrimSpace(event.NodeID)
	}
	if id == "" {
		id = fmt.Sprintf("%d", event.Timestamp)
	}
	if isMemfitToolProcessEvent(event.Type) {
		return "tool:" + id
	}
	return event.Type + ":" + id
}

func isMemfitToolProcessEvent(eventType string) bool {
	switch eventType {
	case string(schema.EVENT_TOOL_CALL_START), string(schema.EVENT_TOOL_CALL_STATUS),
		string(schema.EVENT_TOOL_CALL_DONE), string(schema.EVENT_TOOL_CALL_ERROR),
		string(schema.EVENT_TOOL_CALL_SUMMARY), string(schema.EVENT_TOOL_CALL_DECISION),
		string(schema.EVENT_TOOL_CALL_RESULT), string(schema.EVENT_TOOL_CALL_PARAM),
		string(schema.EVENT_TOOL_CALL_REASON), string(schema.EVENT_TOOL_CALL_PROGRESS_REVIEW),
		string(schema.EVENT_TOOL_CALL_USER_CANCEL):
		return true
	default:
		return false
	}
}

func extractMemfitJSONList(content string, keys ...string) string {
	var root map[string]any
	if json.Unmarshal([]byte(content), &root) != nil {
		return ""
	}
	seen := make(map[string]struct{})
	values := make([]string, 0, 8)
	for _, key := range keys {
		appendMemfitJSONListValue(root[key], seen, &values)
	}
	if len(values) > 6 {
		values = append(values[:6], fmt.Sprintf("+%d", len(values)-6))
	}
	return strings.Join(values, ", ")
}

func appendMemfitJSONListValue(value any, seen map[string]struct{}, output *[]string) {
	switch typed := value.(type) {
	case string:
		for _, item := range strings.FieldsFunc(typed, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			*output = append(*output, item)
		}
	case []any:
		for _, child := range typed {
			appendMemfitJSONListValue(child, seen, output)
		}
	}
}

func extractMemfitNestedJSONField(content string, keys ...string) string {
	var root any
	if json.Unmarshal([]byte(content), &root) != nil {
		return ""
	}
	return findMemfitNestedScalar(root, keys)
}

func findMemfitNestedScalar(value any, keys []string) string {
	switch typed := value.(type) {
	case map[string]any:
		if result := scalarMemfitMapString(typed, keys...); result != "" {
			return result
		}
		for _, child := range typed {
			if result := findMemfitNestedScalar(child, keys); result != "" {
				return result
			}
		}
	case []any:
		for _, child := range typed {
			if result := findMemfitNestedScalar(child, keys); result != "" {
				return result
			}
		}
	}
	return ""
}

func formatMemfitTodoStats(content string) string {
	var root map[string]any
	if json.Unmarshal([]byte(content), &root) != nil {
		return ""
	}
	stats, _ := root["stats"].(map[string]any)
	if len(stats) == 0 {
		return ""
	}
	parts := make([]string, 0, 5)
	for _, item := range []struct {
		key   string
		label string
	}{{"doing", "active"}, {"pending", "waiting"}, {"done", "done"}, {"skipped", "skipped"}, {"deleted", "removed"}} {
		value := scalarMemfitMapString(stats, item.key)
		if value != "" && value != "0" {
			parts = append(parts, value+" "+item.label)
		}
	}
	return strings.Join(parts, " · ")
}

func extractMemfitStructuredStatus(content string) string {
	var root map[string]any
	if json.Unmarshal([]byte(content), &root) != nil {
		return ""
	}
	if status := scalarMemfitMapString(root, "react_task_now_status", "status", "state"); status != "" {
		return "Task " + strings.ReplaceAll(status, "_", " ")
	}
	for _, key := range []string{"message", "summary", "title", "content", "reason", "human_readable"} {
		if result := findMemfitReadableValue(root[key]); result != "" {
			return compactMemfitMessage(result, 480)
		}
	}
	return ""
}

func classifyMemfitTimelineItem(content string) (kind, group, detail string, state memfitProcessState, ok bool) {
	var item struct {
		Deleted   bool   `json:"deleted"`
		Type      string `json:"type"`
		EntryType string `json:"entry_type"`
		Content   string `json:"content"`
		RawText   string `json:"raw_text"`
	}
	if json.Unmarshal([]byte(content), &item) != nil || item.Deleted || item.Type == "tool_result" {
		return "", "", "", memfitProcessInfo, false
	}
	entry := strings.TrimSpace(item.EntryType)
	if entry == "" {
		entry = extractMemfitTimelineEntry(item.RawText)
	}
	entry = normalizeMemfitTimelineEntry(entry)
	detail = trimMemfitProcessText(item.Content)
	if entry == "" || detail == "" {
		return "", "", "", memfitProcessInfo, false
	}

	state = memfitProcessDone
	if containsAnyMemfit(entry, "start", "init", "searching", "processing", "running") {
		state = memfitProcessRunning
	}
	if containsAnyMemfit(entry, "warning", "blocked", "skip", "insufficient") {
		state = memfitProcessWarning
	}
	if containsAnyMemfit(entry, "error", "failed", "panic", "timeout", "missing") {
		state = memfitProcessError
	}

	switch {
	case strings.HasPrefix(entry, "intent"):
		if containsAnyMemfit(entry, "recommended", "context_enrichment") {
			return "Capabilities", "capabilities", detail, state, true
		}
		return "Intent", "intent", detail, state, true
	case strings.HasPrefix(entry, "perception"):
		if strings.Contains(entry, "capabilit") {
			return "Capabilities", "capabilities", detail, state, true
		}
		return "Understanding", "understanding", detail, state, true
	case containsAnyMemfit(entry, "search_capabilit", "skill_loaded", "skills_batch_loaded", "recent_tool_preloaded", "load_capability"):
		return "Capabilities", "capabilities", detail, state, true
	case containsAnyMemfit(entry, "plan_mode", "direct_plan", "plan_execute", "plan_only", "detached_plan"):
		return "Plan", "plan", detail, state, true
	case containsAnyMemfit(entry, "phase1", "phase2", "phase3", "audit_start", "audit_done"):
		return "Progress", "progress", detail, state, true
	case containsAnyMemfit(entry, "error", "failed", "panic", "timeout", "warning", "blocked", "missing"):
		return "Issue", "issue", detail, state, true
	case containsAnyMemfit(entry, "finding", "result", "run_passed", "run_skipped", "verify", "evidence", "report_finish", "saved"):
		return "Observation", "observation", detail, state, true
	default:
		// Timeline breadcrumbs such as user input, assistant output, finish,
		// and task initialization are useful to the model but duplicate the
		// transcript for a human. Keep them out of the activity list.
		return "", "", "", memfitProcessInfo, false
	}
}

func extractMemfitTimelineEntry(raw string) string {
	raw = strings.TrimSpace(raw)
	start := strings.IndexByte(raw, '[')
	if start < 0 {
		return ""
	}
	end := strings.IndexByte(raw[start+1:], ']')
	if end < 0 {
		return ""
	}
	return strings.Trim(raw[start+1:start+1+end], "[] ")
}

func normalizeMemfitTimelineEntry(entry string) string {
	entry = strings.ToLower(strings.Trim(entry, "[] \t\r\n"))
	entry = strings.NewReplacer("-", "_", " ", "_").Replace(entry)
	return entry
}

func memfitTimelineActivity(kind string, state memfitProcessState) string {
	if state == memfitProcessError {
		return kind + " failed"
	}
	if state == memfitProcessWarning {
		return kind + " needs attention"
	}
	switch kind {
	case "Intent":
		return "Understanding intent"
	case "Understanding":
		return "Understanding context"
	case "Capabilities":
		return "Selecting capabilities"
	case "Plan":
		return "Planning"
	case "Observation":
		return "Reviewing result"
	case "Progress":
		return "Working"
	default:
		return kind
	}
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
		if _, ok := typed["verbose_name_i18n"].(map[string]any); ok {
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

func extractMemfitToolDetail(content string) string {
	var value any
	if json.Unmarshal([]byte(content), &value) != nil {
		return ""
	}
	return findMemfitToolDetail(value)
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

func memfitToolActivityKind(name string) string {
	lower := strings.ToLower(name)
	switch {
	case containsAnyMemfit(lower, "edit", "modify", "write", "patch", "create file", "编辑", "修改", "写入"):
		return "Edit"
	case containsAnyMemfit(lower, "read", "search", "find", "query", "list", "读取", "阅读", "查看", "浏览", "搜索", "查找", "检索"):
		return "Read"
	case containsAnyMemfit(lower, "bash", "shell", "terminal", "execute", "command", "运行", "执行", "命令"):
		return "Run"
	default:
		return "Tool"
	}
}

func memfitToolActivityKindFromContent(content, displayName string) string {
	var value any
	if json.Unmarshal([]byte(content), &value) == nil {
		if tool := findMemfitToolObject(value); tool != nil {
			machineName := scalarMemfitMapString(tool, "name", "tool_name")
			if machineName != "" {
				return memfitToolActivityKind(machineName + " " + displayName)
			}
		}
	}
	return memfitToolActivityKind(displayName)
}

func memfitToolActivityVerb(kind string) string {
	switch kind {
	case "Read":
		return "Reading"
	case "Edit":
		return "Editing"
	case "Run":
		return "Running"
	default:
		return "Using"
	}
}

func containsAnyMemfit(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func extractMemfitToolResultPreview(content string) string {
	var root any
	if json.Unmarshal([]byte(content), &root) != nil {
		return ""
	}
	if value, ok := root.(map[string]any); ok {
		if result, exists := value["result"]; exists {
			root = result
		} else {
			// Completion-only events carry timing and protocol metadata, not a
			// result users need to inspect.
			return ""
		}
	}
	preview := readableMemfitToolResult(root, 0)
	preview = trimMemfitProcessText(preview)
	if preview == "" {
		return ""
	}
	return "Result:\n" + preview
}

func readableMemfitToolResult(value any, depth int) string {
	if depth > 3 {
		return ""
	}
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(sanitizeMemfitTerminalText(typed))
		if text == "" {
			return ""
		}
		if json.Valid([]byte(text)) {
			var nested any
			if json.Unmarshal([]byte(text), &nested) == nil {
				if readable := readableMemfitToolResult(nested, depth+1); readable != "" {
					return readable
				}
				return "Structured result available"
			}
		}
		return text
	case map[string]any:
		for _, key := range []string{"output", "stdout", "content", "message", "summary", "text", "result", "error"} {
			if child, ok := typed[key]; ok {
				if readable := readableMemfitToolResult(child, depth+1); readable != "" {
					return readable
				}
			}
		}
		return ""
	case []any:
		parts := make([]string, 0, 4)
		for _, child := range typed {
			if readable := readableMemfitToolResult(child, depth+1); readable != "" {
				parts = append(parts, readable)
			}
			if len(parts) == 4 {
				break
			}
		}
		return strings.Join(parts, "\n")
	case json.Number:
		return typed.String()
	case float64, bool:
		return fmt.Sprint(typed)
	default:
		return ""
	}
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

func normalizeMemfitHeight(height int) int {
	if height <= 0 {
		return 24
	}
	if height < 6 {
		return 6
	}
	return height
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
