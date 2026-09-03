package memfitcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	gopty "github.com/aymanbagabas/go-pty"
	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

const memfitTTYHelperEnvironment = "YAK_MEMFIT_TTY_TEST_HELPER"

// TestMemfitTTYHelper runs inside a child test process attached to a real PTY.
// The outer tests below drive this process exactly like a user drives Memfit.
func TestMemfitTTYHelper(t *testing.T) {
	if os.Getenv(memfitTTYHelperEnvironment) != "1" {
		return
	}
	client := newScriptedMemfitClient()
	defer client.Close()
	config := memfitStartConfig{
		Model:         "test-model",
		Workdir:       "/workspace/yaklang",
		Language:      "zh",
		ReviewPolicy:  "yolo",
		MaxIterations: 50,
	}
	require.NoError(t, runMemfitTUI(context.Background(), client, config, ""))
}

func TestMemfitTTYWidePasteAndAnswerSnapshot(t *testing.T) {
	h := newMemfitTTYHarness(t, 88, 22)
	defer h.Close()

	h.WaitFor("❯")
	h.Write("\x1b[200~你好，Memfit\n第二行 🌏\x1b[201~")
	h.WaitFor("你好，Memfit↵第二行 🌏")
	h.AssertSnapshot("wide-paste")

	h.Write("\r")
	h.WaitFor("echo: 你好，Memfit / 第二行 🌏")
	h.WaitFor("Completed")
	snapshot := h.WaitFor("────────────────\n○ Ready")
	require.Contains(t, snapshot, "❯\n────────────────")
	h.AssertSnapshot("wide-answer")
	raw := h.Raw()
	require.Contains(t, raw, "\x1b[?2004h", "bracketed paste must be enabled")
	require.Contains(t, raw, "\x1b[?1000h", "activity rows must accept mouse clicks")
	require.NotContains(t, raw, "\x1b[?1049", "Memfit must preserve native scrollback")
}

func TestMemfitTTYManualReviewAndCancellationSnapshot(t *testing.T) {
	h := newMemfitTTYHarness(t, 72, 20)
	defer h.Close()

	h.WaitFor("❯")
	h.Write("/mode manual\r")
	h.WaitFor("Review policy changed to MANUAL")
	h.Write("needs-review\r")
	h.WaitFor("Allow the deterministic test action?")
	h.WaitFor("reply ❯")
	h.AssertSnapshot("manual-review")

	h.Write("approve\r")
	h.WaitFor("approved: approve")
	h.WaitFor("Completed")
	h.WaitFor("────────────────\n○ Ready")
	h.Write("slow\r")
	h.WaitFor("queue ❯")
	h.Write("\x03")
	h.WaitFor("Cancelling")
	h.AssertSnapshot("cancelling")
	h.WaitFor("Cancelled")
}

func TestMemfitTTYCompactResizeAndHistorySnapshot(t *testing.T) {
	h := newMemfitTTYHarness(t, 44, 12)
	defer h.Close()

	h.WaitFor("memfit · YOLO")
	h.Write("first prompt\r")
	h.WaitFor("Completed")
	h.WaitFor("────────────────\n○ Ready")
	h.Write("/clear\r")
	time.Sleep(100 * time.Millisecond)
	h.Write("\x1b[A")
	h.WaitForScreen("❯ first prompt")
	h.Resize(24, 10)
	h.Write(" with a long 中文 suffix")
	h.WaitForScreen("suffix")
	h.AssertSnapshot("compact-resize-history")
}

func TestMemfitTTYVeryNarrowHelpSnapshot(t *testing.T) {
	h := newMemfitTTYHarness(t, 24, 12)
	defer h.Close()

	h.WaitFor("Enter send · ^C stop")
	h.Write("/help\r")
	h.WaitFor("/logs /clear /exit")
	h.AssertSnapshot("very-narrow-help")
}

func TestMemfitTTYVeryNarrowQueueStatusSnapshot(t *testing.T) {
	h := newMemfitTTYHarness(t, 24, 18)
	defer h.Close()

	h.WaitFor("❯")
	h.Write("queue-first\r")
	h.WaitFor("queue ❯")
	h.Write("next\r")
	h.WaitFor("Q1")
	h.Write("/status\r")
	h.WaitFor("PID 4242")
	h.AssertSnapshot("very-narrow-queue-status")
}

func TestMemfitTTYCommandsAndErrorRecoverySnapshot(t *testing.T) {
	h := newMemfitTTYHarness(t, 68, 28)
	defer h.Close()

	h.WaitFor("❯")
	h.Write("/status\r")
	h.WaitFor("PID 4242")
	h.AssertSnapshot("status-panel")
	h.Write("/logs\r")
	h.WaitFor("erase-attempt")
	logsSnapshot := h.WaitFor("structured retry detail")
	require.NotContains(t, logsSnapshot, `{"message"`)
	h.AssertSnapshot("logs-panel")
	h.Write("error\r")
	h.WaitFor("deterministic provider failure")
	h.WaitFor("❯")
	h.Write("/help\r")
	h.WaitFor("/mode MODE")
	h.AssertSnapshot("commands-error-recovery")
}

func TestMemfitTTYMultilineStreamReturnsToColumnZero(t *testing.T) {
	h := newMemfitTTYHarness(t, 60, 14)
	defer h.Close()

	h.WaitFor("❯")
	h.Write("multiline\r")
	h.WaitFor("Completed")
	snapshot := h.WaitFor("第二行")
	require.Contains(t, snapshot, "Memfit\n  first line\n  第二行")
}

func TestMemfitTTYQueuesInputWhileBusySnapshot(t *testing.T) {
	h := newMemfitTTYHarness(t, 72, 22)
	defer h.Close()

	h.WaitFor("❯")
	h.Write("queue-first\r")
	h.WaitFor("queue ❯")
	h.Write("第二个问题 queued while busy\r")
	h.WaitFor("1 waiting")
	h.AssertSnapshot("queued-while-busy")
	h.Write("/status\r")
	h.WaitFor("PID 4242")
	h.AssertSnapshot("status-while-busy")

	h.WaitFor("echo: queue-first")
	h.WaitFor("echo: 第二个问题 queued while busy")
	h.Write("/queue\r")
	h.WaitFor("No messages waiting")
}

func TestMemfitTTYPauseAndResumeQueueAfterErrorSnapshot(t *testing.T) {
	h := newMemfitTTYHarness(t, 72, 22)
	defer h.Close()

	h.WaitFor("❯")
	h.Write("error-with-queue\r")
	h.WaitFor("queue ❯")
	h.Write("continue after error\r")
	h.WaitFor("Queued #1")
	h.WaitFor("Queue paused")
	h.AssertSnapshot("queue-paused-after-error")

	h.Write("/queue resume\r")
	h.WaitFor("echo: continue after error")
	h.WaitFor("Completed")
}

func TestMemfitTTYHidesToolJSONAndYOLOPromptsSnapshot(t *testing.T) {
	h := newMemfitTTYHarness(t, 76, 22)
	defer h.Close()

	h.WaitFor("❯")
	h.Write("tool-noise\r")
	h.WaitFor("Reading 文件读取 · README.md")
	h.WaitFor("Review · 读取 README.md")
	h.WaitFor("Tool issue")
	h.WaitFor("最终答案，只展示一次")
	h.WaitFor("Completed")
	snapshot := h.WaitFor("────────────────\n○ Ready")
	require.NotContains(t, snapshot, `"description"`)
	require.NotContains(t, snapshot, "Memfit update")
	require.NotContains(t, snapshot, "reply ❯")
	require.NotContains(t, snapshot, "中间答案")
	raw := h.Raw()
	require.NotContains(t, raw, `"description"`)
	require.NotContains(t, raw, "reply ❯")
	require.NotContains(t, raw, "Memfit update")
	h.AssertSnapshot("low-noise-tool-flow")
}

func TestMemfitTTYActivityTimelineExpandsIndividualStreams(t *testing.T) {
	h := newMemfitTTYHarness(t, 82, 26)
	defer h.Close()

	h.WaitFor("❯")
	h.Write("activity-flow\r")
	h.WaitFor("activity flow complete")
	h.WaitFor("Completed")
	collapsed := h.WaitFor("────────────────\n○ Ready")
	require.Contains(t, collapsed, "✓ Thinking ▸")
	require.NotContains(t, collapsed, "先识别用户意图，再选择最小范围的工具。")
	require.Contains(t, collapsed, "✓ Intent ▸ · inspect repository")
	require.Contains(t, collapsed, "✓ Read ▸ · 文件读取 · README.md")

	h.ClickText("Thinking")
	h.WaitFor("先识别用户意图，再选择最小范围的工具。")
	h.AssertSnapshot("activity-thinking-open")

	h.ClickText("Read")
	readOpen := h.WaitFor("Yaklang is a security-oriented language")
	require.NotContains(t, readOpen, `"description"`)
	require.NotContains(t, readOpen, `"call_tool_id"`)
	h.AssertSnapshot("activity-read-open")

	h.Resize(30, 14)
	narrow := h.WaitFor("Result:")
	for _, line := range strings.Split(narrow, "\n") {
		require.LessOrEqual(t, runewidth.StringWidth(line), 30)
	}
	require.Contains(t, narrow, "❯")
	h.AssertSnapshot("activity-narrow-open")
}

func TestMemfitTTYPreservesDraftWhileResponseUpdatesSnapshot(t *testing.T) {
	h := newMemfitTTYHarness(t, 72, 22)
	defer h.Close()

	h.WaitFor("❯")
	h.Write("stream-while-type\r")
	h.WaitFor("Memfit › answer starts")
	h.Write("draft 中文 stays")
	h.WaitFor("Memfit › answer starts and continues")
	h.WaitFor("Completed")
	snapshot := h.WaitFor("❯ draft 中文 stays")
	require.Contains(t, snapshot, "answer starts and continues while typing")
	h.AssertSnapshot("typing-during-response")
}

type scriptedMemfitClient struct {
	events    chan memfitEnvelope
	logs      chan string
	done      chan struct{}
	closeOnce sync.Once
	logTail   []string
}

func newScriptedMemfitClient() *scriptedMemfitClient {
	return &scriptedMemfitClient{
		events: make(chan memfitEnvelope, 64),
		logs:   make(chan string, 8),
		done:   make(chan struct{}),
		logTail: []string{
			"[WARN] 2026-09-02 15:08:01 [provider:42] worker retry is available",
			`{"message":"structured retry detail","attempt":2}`,
			"\x1b[2Jerase-attempt stayed plain text",
		},
	}
}

func (c *scriptedMemfitClient) send(typ, id string, payload any) error {
	switch typ {
	case "input":
		input, ok := payload.(memfitInput)
		if !ok {
			return fmt.Errorf("unexpected input payload %T", payload)
		}
		c.emit("accepted", id, memfitStatus{Message: "input accepted"})
		switch input.Text {
		case "needs-review":
			go func() {
				time.Sleep(25 * time.Millisecond)
				c.emit("event", "approval-1", memfitWorkerEvent{
					Type:    string(schema.EVENT_TYPE_PERMISSION_REQUIRE),
					Content: `{"interactive_id":"approval-1","question":"Allow the deterministic test action?"}`,
				})
			}()
		case "slow":
			// The task remains active until the test sends Ctrl+C.
		case "queue-first":
			go func() {
				time.Sleep(900 * time.Millisecond)
				c.completeEcho(input.Text)
			}()
		case "tool-noise":
			go c.completeNoisyToolFlow()
		case "activity-flow":
			go c.completeActivityFlow()
		case "stream-while-type":
			go c.completeSlowStream()
		case "error":
			go func() {
				time.Sleep(25 * time.Millisecond)
				c.emit("error", id, memfitStatus{Message: "deterministic provider failure"})
			}()
		case "error-with-queue":
			go func() {
				time.Sleep(350 * time.Millisecond)
				c.emit("error", id, memfitStatus{Message: "deterministic provider failure"})
			}()
		case "multiline":
			go c.completeAnswer("first line\n第二行")
		default:
			go c.completeEcho(input.Text)
		}
	case "interactive":
		input, ok := payload.(memfitInput)
		if !ok {
			return fmt.Errorf("unexpected interactive payload %T", payload)
		}
		go c.completeAnswer("approved: " + input.Text)
	case "cancel":
		go func() {
			time.Sleep(180 * time.Millisecond)
			c.emit("turn_done", id, memfitStatus{TaskID: "test-task", Status: "cancelled"})
		}()
	case "review":
		c.emit("status", id, memfitStatus{Message: "review policy updated"})
	case "shutdown":
		c.Close()
	default:
		return fmt.Errorf("unexpected frame type %q", typ)
	}
	return nil
}

func (c *scriptedMemfitClient) completeEcho(input string) {
	c.completeAnswer("echo: " + strings.ReplaceAll(input, "\n", " / "))
}

func (c *scriptedMemfitClient) completeAnswer(answer string) {
	time.Sleep(25 * time.Millisecond)
	c.emit("event", "thought", memfitWorkerEvent{
		Type:        string(schema.EVENT_TYPE_STREAM),
		NodeID:      "re-act-loop-thought",
		IsStream:    true,
		IsReason:    true,
		StreamDelta: "checking…",
	})
	time.Sleep(20 * time.Millisecond)
	answerEvent := memfitWorkerEvent{
		Type:        string(schema.EVENT_TYPE_STREAM),
		NodeID:      "re-act-loop-answer-payload",
		IsStream:    true,
		StreamDelta: answer,
		AIModel:     "test-model",
	}
	c.emit("event", "answer-1", answerEvent)
	// Reproduce the engine replay observed while it confirms task completion.
	time.Sleep(20 * time.Millisecond)
	c.emit("event", "answer-2", answerEvent)
	time.Sleep(20 * time.Millisecond)
	c.emit("turn_done", "", memfitStatus{TaskID: "test-task", Status: "completed"})
}

func (c *scriptedMemfitClient) completeNoisyToolFlow() {
	time.Sleep(25 * time.Millisecond)
	c.emit("event", "thought", memfitWorkerEvent{
		Type:        string(schema.EVENT_TYPE_STREAM),
		NodeID:      "re-act-loop-thought",
		IsStream:    true,
		IsReason:    true,
		StreamDelta: "读取 README 了解项目定位",
	})
	time.Sleep(20 * time.Millisecond)
	c.emit("event", "tool-1", memfitWorkerEvent{
		Type:       string(schema.EVENT_TOOL_CALL_START),
		CallToolID: "tool-1",
		Content: `{"tool":{"description":"A very long internal schema that users should never see",` +
			`"name":"read_file","verbose_name":"File Reader","verbose_name_i18n":{"Zh":"文件读取"}},` +
			`"params":{"file_path":"README.md"}}`,
	})
	time.Sleep(20 * time.Millisecond)
	c.emit("event", "plan-1", memfitWorkerEvent{
		Type:    string(schema.EVENT_TYPE_PERMISSION_REQUIRE),
		Content: `{"question":"读取 README.md 获取项目介绍"}`,
	})
	time.Sleep(20 * time.Millisecond)
	c.emit("event", "tool-error", memfitWorkerEvent{
		Type:       string(schema.EVENT_TOOL_CALL_ERROR),
		CallToolID: "tool-1",
		Content:    `{"error":"missing required command; Memfit corrected the tool input and retried"}`,
	})
	time.Sleep(20 * time.Millisecond)
	c.emit("event", "answer-early", memfitWorkerEvent{
		Type:        string(schema.EVENT_TYPE_STREAM),
		NodeID:      "re-act-loop-answer-payload",
		IsStream:    true,
		StreamDelta: "中间答案",
	})
	time.Sleep(20 * time.Millisecond)
	c.emit("event", "answer-final", memfitWorkerEvent{
		Type:        string(schema.EVENT_TYPE_STREAM),
		NodeID:      "re-act-loop-answer-payload",
		IsStream:    true,
		StreamDelta: "最终答案，只展示一次。",
		AIModel:     "test-model",
	})
	time.Sleep(20 * time.Millisecond)
	c.emit("turn_done", "", memfitStatus{TaskID: "test-task", Status: "completed"})
}

func (c *scriptedMemfitClient) completeActivityFlow() {
	time.Sleep(25 * time.Millisecond)
	c.emit("event", "thought-activity", memfitWorkerEvent{
		Type:        string(schema.EVENT_TYPE_STREAM),
		NodeID:      "re-act-loop-thought",
		IsStream:    true,
		IsReason:    true,
		StreamDelta: "先识别用户意图，再选择最小范围的工具。",
	})
	time.Sleep(15 * time.Millisecond)
	c.emit("event", "intent-activity", memfitWorkerEvent{
		Type:    string(schema.EVENT_TYPE_INTENT_RECOGNITION),
		Content: `{"intent":"inspect repository","matched_tool_names":["read_file"],"description":"hidden schema"}`,
	})
	time.Sleep(15 * time.Millisecond)
	c.emit("event", "perception-activity", memfitWorkerEvent{
		Type:    string(schema.EVENT_TYPE_PERCEPTION),
		Content: `{"summary":"README contains project context","confidence":0.94}`,
	})
	time.Sleep(15 * time.Millisecond)
	c.emit("event", "tool-start-activity", memfitWorkerEvent{
		Type:       string(schema.EVENT_TOOL_CALL_START),
		CallToolID: "activity-tool-1",
		Content:    `{"tool":{"name":"read_file","verbose_name_i18n":{"Zh":"文件读取"},"description":"hidden schema"},"params":{"file_path":"README.md"}}`,
	})
	time.Sleep(15 * time.Millisecond)
	c.emit("event", "tool-reason-activity", memfitWorkerEvent{
		Type:       string(schema.EVENT_TOOL_CALL_REASON),
		CallToolID: "activity-tool-1",
		Content:    `{"reason":"Read the project overview before answering"}`,
	})
	time.Sleep(15 * time.Millisecond)
	c.emit("event", "tool-result-activity", memfitWorkerEvent{
		Type:       string(schema.EVENT_TOOL_CALL_RESULT),
		CallToolID: "activity-tool-1",
		Content:    `{"call_tool_id":"activity-tool-1","result":"# Yaklang\nYaklang is a security-oriented language."}`,
	})
	time.Sleep(15 * time.Millisecond)
	c.emit("event", "observation-activity", memfitWorkerEvent{
		Type:    string(schema.EVENT_TYPE_OBSERVATION),
		Content: `{"observation":"Project purpose confirmed","source":"README.md"}`,
	})
	time.Sleep(15 * time.Millisecond)
	c.emit("event", "activity-answer", memfitWorkerEvent{
		Type:        string(schema.EVENT_TYPE_STREAM),
		NodeID:      "re-act-loop-answer-payload",
		IsStream:    true,
		StreamDelta: "activity flow complete",
		AIModel:     "test-model",
	})
	time.Sleep(15 * time.Millisecond)
	c.emit("turn_done", "", memfitStatus{TaskID: "test-task", Status: "completed"})
}

func (c *scriptedMemfitClient) completeSlowStream() {
	time.Sleep(30 * time.Millisecond)
	parts := []string{"answer starts", " and continues", " while typing"}
	for _, part := range parts {
		c.emit("event", "answer-stream", memfitWorkerEvent{
			Type:        string(schema.EVENT_TYPE_STREAM),
			NodeID:      "re-act-loop-answer-payload",
			IsStream:    true,
			StreamDelta: part,
			AIModel:     "test-model",
		})
		time.Sleep(140 * time.Millisecond)
	}
	c.emit("turn_done", "", memfitStatus{TaskID: "test-task", Status: "completed"})
}

func (c *scriptedMemfitClient) emit(typ, id string, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	select {
	case c.events <- memfitEnvelope{Version: memfitProtocolVersion, Type: typ, ID: id, Data: raw}:
	case <-c.done:
	}
}

func (c *scriptedMemfitClient) Events() <-chan memfitEnvelope { return c.events }
func (c *scriptedMemfitClient) Logs() <-chan string           { return c.logs }
func (c *scriptedMemfitClient) Done() <-chan struct{}         { return c.done }
func (c *scriptedMemfitClient) WaitError() error              { return nil }
func (c *scriptedMemfitClient) LogTail() []string {
	return append([]string(nil), c.logTail...)
}
func (c *scriptedMemfitClient) formattedLogTail() string { return "" }
func (c *scriptedMemfitClient) PID() int                 { return 4242 }
func (c *scriptedMemfitClient) Close() {
	c.closeOnce.Do(func() { close(c.done) })
}

type memfitTTYHarness struct {
	t      *testing.T
	pty    gopty.Pty
	cmd    *gopty.Cmd
	screen *memfitTestScreen
	done   chan error
	mu     sync.Mutex
	raw    bytes.Buffer
}

func newMemfitTTYHarness(t *testing.T, width, height int) *memfitTTYHarness {
	t.Helper()
	terminal, err := gopty.New()
	require.NoError(t, err)
	require.NoError(t, terminal.Resize(width, height))

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	t.Cleanup(cancel)
	cmd := terminal.CommandContext(ctx, os.Args[0], "-test.run=^TestMemfitTTYHelper$", "-test.count=1")
	cmd.Env = append(filterMemfitTestEnvironment(os.Environ()),
		memfitTTYHelperEnvironment+"=1",
		"TERM=xterm-256color",
		"NO_COLOR=1",
		"LOG_LEVEL=warn",
	)
	require.NoError(t, cmd.Start())

	h := &memfitTTYHarness{
		t:      t,
		pty:    terminal,
		cmd:    cmd,
		screen: newMemfitTestScreen(width, height),
		done:   make(chan error, 1),
	}
	go h.read()
	go func() { h.done <- cmd.Wait() }()
	return h
}

func (h *memfitTTYHarness) read() {
	buffer := make([]byte, 8192)
	for {
		n, err := h.pty.Read(buffer)
		if n > 0 {
			h.mu.Lock()
			h.raw.Write(buffer[:n])
			h.screen.Write(buffer[:n])
			h.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (h *memfitTTYHarness) Write(input string) {
	h.t.Helper()
	_, err := io.WriteString(h.pty, input)
	require.NoError(h.t, err)
}

func (h *memfitTTYHarness) ClickText(text string) {
	h.t.Helper()
	h.mu.Lock()
	targetRow, targetColumn := -1, -1
	for row, cells := range h.screen.cells {
		var line strings.Builder
		for _, cell := range cells {
			if cell != "\x00" {
				line.WriteString(cell)
			}
		}
		if byteIndex := strings.Index(line.String(), text); byteIndex >= 0 {
			targetRow = row + 1
			targetColumn = runewidth.StringWidth(line.String()[:byteIndex]) + 1
			break
		}
	}
	cursorRow, cursorColumn := h.screen.row+1, h.screen.col+1
	h.mu.Unlock()
	require.Positive(h.t, targetRow, "could not find clickable text %q", text)
	require.Positive(h.t, targetColumn)
	h.Write(fmt.Sprintf("\x1b[<0;%d;%dM", targetColumn, targetRow))
	time.Sleep(20 * time.Millisecond)
	h.Write(fmt.Sprintf("\x1b[%d;%dR", cursorRow, cursorColumn))
}

func (h *memfitTTYHarness) Resize(width, height int) {
	h.t.Helper()
	// Resize the screen model first so bytes emitted immediately after the PTY
	// resize are interpreted with the same geometry as the real terminal.
	h.mu.Lock()
	h.screen.Resize(width, height)
	h.mu.Unlock()
	require.NoError(h.t, h.pty.Resize(width, height))
	// Memfit polls terminal size rather than owning SIGWINCH handlers.
	time.Sleep(350 * time.Millisecond)
}

func (h *memfitTTYHarness) WaitFor(want string) string {
	h.t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		snapshot := h.screen.Snapshot()
		raw := h.raw.String()
		h.mu.Unlock()
		if strings.Contains(snapshot, want) || strings.Contains(raw, want) {
			return snapshot
		}
		select {
		case err := <-h.done:
			h.t.Fatalf("TTY helper exited while waiting for %q: %v\nraw output:\n%s", want, err, visibleMemfitControlBytes(raw))
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.mu.Lock()
	snapshot := h.screen.Snapshot()
	raw := h.raw.String()
	h.mu.Unlock()
	h.t.Fatalf("timed out waiting for %q\nscreen:\n%s\nraw output:\n%s", want, snapshot, visibleMemfitControlBytes(raw))
	return ""
}

func (h *memfitTTYHarness) WaitForScreen(want string) string {
	h.t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		snapshot := h.screen.Snapshot()
		raw := h.raw.String()
		h.mu.Unlock()
		if strings.Contains(snapshot, want) {
			return snapshot
		}
		select {
		case err := <-h.done:
			h.t.Fatalf("TTY helper exited while waiting for %q on screen: %v\nraw output:\n%s", want, err, visibleMemfitControlBytes(raw))
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.mu.Lock()
	snapshot := h.screen.Snapshot()
	raw := h.raw.String()
	h.mu.Unlock()
	h.t.Fatalf("timed out waiting for %q on screen\nscreen:\n%s\nraw output:\n%s", want, snapshot, visibleMemfitControlBytes(raw))
	return ""
}

func (h *memfitTTYHarness) AssertSnapshot(name string) {
	h.t.Helper()
	h.mu.Lock()
	got := h.screen.Snapshot()
	h.mu.Unlock()
	assertMemfitTTYGolden(h.t, name, got)
}

func (h *memfitTTYHarness) Raw() string {
	h.t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.raw.String()
}

func (h *memfitTTYHarness) Close() {
	h.t.Helper()
	_, _ = io.WriteString(h.pty, "\x03\x03")
	select {
	case <-h.done:
	case <-time.After(2 * time.Second):
		if h.cmd.Process != nil {
			_ = h.cmd.Process.Kill()
		}
		select {
		case <-h.done:
		case <-time.After(time.Second):
		}
	}
	_ = h.pty.Close()
}

func assertMemfitTTYGolden(t *testing.T, name, got string) {
	t.Helper()
	got = regexp.MustCompile(`\b\d+(?:\.\d+)?(?:ms|s)\b`).ReplaceAllString(got, "<elapsed>")
	path := filepath.Join("testdata", "tty", name+".golden")
	if os.Getenv("YAK_MEMFIT_UPDATE_TTY_GOLDEN") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got+"\n"), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run with YAK_MEMFIT_UPDATE_TTY_GOLDEN=1 to create terminal snapshots")
	require.Equal(t, strings.TrimSuffix(string(want), "\n"), got)
	if artifactDir := os.Getenv("YAK_MEMFIT_TTY_ARTIFACT_DIR"); artifactDir != "" {
		require.NoError(t, os.MkdirAll(artifactDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(artifactDir, name+".txt"), []byte(got+"\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(artifactDir, name+".svg"), renderMemfitTTYSVG(name, got), 0o644))
	}
}

func renderMemfitTTYSVG(title, snapshot string) []byte {
	lines := strings.Split(snapshot, "\n")
	maxCells := 1
	for _, line := range lines {
		maxCells = maxInt(maxCells, runewidth.StringWidth(line))
	}
	width := maxCells*9 + 40
	height := len(lines)*20 + 54
	var output strings.Builder
	fmt.Fprintf(&output, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	output.WriteString(`<rect width="100%" height="100%" rx="10" fill="#101418"/>`)
	output.WriteString(`<circle cx="18" cy="17" r="5" fill="#ff5f57"/><circle cx="34" cy="17" r="5" fill="#febc2e"/><circle cx="50" cy="17" r="5" fill="#28c840"/>`)
	fmt.Fprintf(&output, `<text x="%d" y="21" text-anchor="middle" fill="#8b949e" font-family="Arial Unicode MS, Menlo, monospace" font-size="11">%s</text>`, width/2, html.EscapeString(title))
	for index, line := range lines {
		color := "#d6deeb"
		if strings.HasPrefix(line, "◆") || strings.HasPrefix(line, "Memfit ›") {
			color = "#56d4dd"
		} else if strings.HasPrefix(line, "You ›") || strings.HasPrefix(line, "✓") {
			color = "#7ee787"
		} else if strings.HasPrefix(line, "?") || strings.HasPrefix(line, "■") {
			color = "#f2cc60"
		}
		fmt.Fprintf(&output, `<text xml:space="preserve" x="18" y="%d" fill="%s" font-family="Arial Unicode MS, Menlo, monospace" font-size="14">%s</text>`, 47+index*20, color, html.EscapeString(line))
	}
	output.WriteString(`</svg>`)
	return []byte(output.String())
}

func filterMemfitTestEnvironment(parent []string) []string {
	result := make([]string, 0, len(parent))
	for _, entry := range parent {
		if strings.HasPrefix(entry, memfitTTYHelperEnvironment+"=") ||
			strings.HasPrefix(entry, "TERM=") ||
			strings.HasPrefix(entry, "NO_COLOR=") ||
			strings.HasPrefix(entry, "LOG_LEVEL=") {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func visibleMemfitControlBytes(raw string) string {
	raw = strings.ReplaceAll(raw, "\x1b", "<ESC>")
	raw = strings.ReplaceAll(raw, "\r", "<CR>")
	return raw
}

// memfitTestScreen is a deliberately small VT renderer for the exact control
// sequences emitted by Memfit. It turns PTY output into stable text screenshots.
type memfitTestScreen struct {
	width, height int
	row, col      int
	cells         [][]string
	pending       []byte
}

func newMemfitTestScreen(width, height int) *memfitTestScreen {
	s := &memfitTestScreen{}
	s.Resize(width, height)
	return s
}

func (s *memfitTestScreen) Resize(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	next := make([][]string, height)
	for row := range next {
		next[row] = make([]string, width)
	}
	if s.cells != nil {
		rows := minInt(height, s.height)
		cols := minInt(width, s.width)
		for row := 0; row < rows; row++ {
			copy(next[row], s.cells[row][:cols])
		}
	}
	s.width, s.height, s.cells = width, height, next
	if s.row >= height {
		s.row = height - 1
	}
	if s.col >= width {
		s.col = width - 1
	}
}

func (s *memfitTestScreen) Write(input []byte) {
	s.pending = append(s.pending, input...)
	for len(s.pending) > 0 {
		if s.pending[0] == 0x1b {
			consumed, complete := s.consumeEscape(s.pending)
			if !complete {
				return
			}
			s.pending = s.pending[consumed:]
			continue
		}
		switch s.pending[0] {
		case '\r':
			s.col = 0
			s.pending = s.pending[1:]
			continue
		case '\n':
			s.newline()
			s.pending = s.pending[1:]
			continue
		case '\b':
			if s.col > 0 {
				s.col--
			}
			s.pending = s.pending[1:]
			continue
		}
		if s.pending[0] < 0x20 || s.pending[0] == 0x7f {
			s.pending = s.pending[1:]
			continue
		}
		if !utf8.FullRune(s.pending) {
			return
		}
		r, size := utf8.DecodeRune(s.pending)
		s.pending = s.pending[size:]
		s.putRune(r)
	}
}

func (s *memfitTestScreen) consumeEscape(input []byte) (int, bool) {
	if len(input) < 2 {
		return 0, false
	}
	if input[1] != '[' {
		return 2, true
	}
	for index := 2; index < len(input); index++ {
		if input[index] < '@' || input[index] > '~' {
			continue
		}
		params := string(input[2:index])
		s.applyCSI(params, input[index])
		return index + 1, true
	}
	return 0, false
}

func (s *memfitTestScreen) applyCSI(params string, final byte) {
	switch final {
	case 'A':
		amount := parseMemfitCSIAmount(params, 1)
		s.row -= amount
		if s.row < 0 {
			s.row = 0
		}
	case 'B':
		amount := parseMemfitCSIAmount(params, 1)
		s.row += amount
		if s.row >= s.height {
			s.row = s.height - 1
		}
	case 'C':
		amount := parseMemfitCSIAmount(params, 1)
		s.col += amount
		if s.col >= s.width {
			s.col = s.width - 1
		}
	case 'D':
		amount := parseMemfitCSIAmount(params, 1)
		s.col -= amount
		if s.col < 0 {
			s.col = 0
		}
	case 'K':
		if params == "2" || params == "" {
			for col := range s.cells[s.row] {
				s.cells[s.row][col] = ""
			}
		}
	case 'J':
		if params == "2" {
			for row := range s.cells {
				for col := range s.cells[row] {
					s.cells[row][col] = ""
				}
			}
		}
	case 'H', 'f':
		if params == "" {
			s.row, s.col = 0, 0
		}
	}
}

func parseMemfitCSIAmount(value string, fallback int) int {
	if strings.HasPrefix(value, "?") || strings.Contains(value, ";") {
		return fallback
	}
	amount, err := strconv.Atoi(value)
	if err != nil || amount < 1 {
		return fallback
	}
	return amount
}

func (s *memfitTestScreen) putRune(r rune) {
	width := runewidth.RuneWidth(r)
	if width <= 0 {
		if s.col > 0 {
			s.cells[s.row][s.col-1] += string(r)
		}
		return
	}
	if s.col+width > s.width {
		s.col = 0
		s.newline()
	}
	s.cells[s.row][s.col] = string(r)
	for offset := 1; offset < width && s.col+offset < s.width; offset++ {
		s.cells[s.row][s.col+offset] = "\x00"
	}
	s.col += width
	if s.col >= s.width {
		s.col = 0
		s.newline()
	}
}

func (s *memfitTestScreen) newline() {
	s.row++
	if s.row < s.height {
		return
	}
	copy(s.cells, s.cells[1:])
	s.cells[s.height-1] = make([]string, s.width)
	s.row = s.height - 1
}

func (s *memfitTestScreen) Snapshot() string {
	lines := make([]string, len(s.cells))
	for row := range s.cells {
		var line strings.Builder
		for _, cell := range s.cells[row] {
			switch cell {
			case "":
				line.WriteByte(' ')
			case "\x00":
				// Continuation cell of a wide rune.
			default:
				line.WriteString(cell)
			}
		}
		lines[row] = strings.TrimRight(line.String(), " ")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

var _ memfitClient = (*scriptedMemfitClient)(nil)
