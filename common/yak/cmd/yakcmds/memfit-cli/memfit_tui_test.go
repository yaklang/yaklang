package memfitcli

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestMemfitSanitizeTerminalText(t *testing.T) {
	input := "safe\x1b[31m red\x1b[0m\rOVER\x1b]0;title\a\n下一行\x00"
	result := sanitizeMemfitTerminalText(input)
	require.Equal(t, "safe redOVER\n下一行", result)
	require.NotContains(t, result, "\x1b")
	require.Equal(t, "one\r\ntwo", sanitizeMemfitTTYText("one\ntwo"))
}

func TestMemfitInputViewportPreservesWideAndMultilineInput(t *testing.T) {
	buffer := []rune("你好\nworld-very-long-tail")
	text, cursor := memfitInputViewport(buffer, len(buffer), 12)
	require.LessOrEqual(t, runewidth.StringWidth(text), 12)
	require.Contains(t, text, "tail")
	require.GreaterOrEqual(t, runewidth.StringWidth(text), 10)
	require.GreaterOrEqual(t, cursor, 0)
	require.LessOrEqual(t, cursor, runewidth.StringWidth(text))

	text, _ = memfitInputViewport([]rune("a\nb"), 2, 20)
	require.Equal(t, "a↵b", text)
}

func TestMemfitBracketedPastePreservesNewlinesWithoutSubmitting(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\x1b[200~line 1\r\n第二行\x1b[201~"))
	key, err := readMemfitKey(reader)
	require.NoError(t, err)
	require.Equal(t, memfitKeyInsert, key.kind)
	require.Equal(t, "line 1\n第二行", key.text)
}

func TestMemfitKeyBindings(t *testing.T) {
	tests := []struct {
		input string
		kind  memfitKeyKind
	}{
		{"\x1b[A", memfitKeyUp},
		{"\x1b[B", memfitKeyDown},
		{"\x1b[3~", memfitKeyDelete},
		{"\x1b\r", memfitKeyNewline},
		{"\x03", memfitKeyInterrupt},
		{"\x0f", memfitKeyToggleProcess},
		{"\x17", memfitKeyDeleteWord},
		{"\x1b[12;7R", memfitKeyCursorPosition},
		{"\x1b[<0;4;9M", memfitKeyMouse},
	}
	for _, test := range tests {
		key, err := readMemfitKey(bufio.NewReader(strings.NewReader(test.input)))
		require.NoError(t, err)
		require.Equal(t, test.kind, key.kind, "input %q", test.input)
	}
	cursor, err := readMemfitKey(bufio.NewReader(strings.NewReader("\x1b[12;7R")))
	require.NoError(t, err)
	require.Equal(t, 12, cursor.row)
	require.Equal(t, 7, cursor.column)

	mouse, err := readMemfitKey(bufio.NewReader(strings.NewReader("\x1b[<0;4;9M")))
	require.NoError(t, err)
	require.True(t, mouse.pressed)
	require.Equal(t, 0, mouse.button)
	require.Equal(t, 4, mouse.column)
	require.Equal(t, 9, mouse.row)
}

func TestMemfitReadableContentExtraction(t *testing.T) {
	require.Equal(t, "Need approval", extractMemfitReadableContent(`{"question":"Need approval"}`))
	require.Equal(t, "plain text", extractMemfitReadableContent("plain text"))
	require.Equal(t, "id-1", extractMemfitInteractiveID("fallback", `{"id":"id-1"}`))
	require.Equal(t, "nested message", humanizeMemfitMessage(`"{\"message\":\"nested message\"}"`))
	require.Contains(t, humanizeMemfitMessage(`{"opaque":{"value":1}}`), "Structured details hidden")
}

func TestMemfitToolSummaryAvoidsSchemaJSON(t *testing.T) {
	content := `{"tool":{"description":"long internal schema","name":"read_file",` +
		`"verbose_name":"File Reader","verbose_name_i18n":{"Zh":"文件读取"}},` +
		`"params":{"file_path":"README.md"}}`
	name, detail := extractMemfitToolSummary(content)
	require.Equal(t, "文件读取", name)
	require.Equal(t, "README.md", detail)
	require.Equal(t, "Using 文件读取 · README.md", describeMemfitEvent(memfitWorkerEvent{
		Type:    string(schema.EVENT_TOOL_CALL_START),
		Content: content,
	}))
}

func TestMemfitWrapCellsPreservesWideRunesAndLineBreaks(t *testing.T) {
	lines := wrapMemfitCells("abcdef\n你好世界", 5)
	require.Equal(t, []string{"abcde", "f", "你好", "世界"}, lines)
	for _, line := range lines {
		require.LessOrEqual(t, runewidth.StringWidth(line), 5)
	}
	require.Equal(t,
		[]string{"Use a tool", "to read", "README.md"},
		wrapMemfitCells("Use a tool to read README.md", 12),
	)
}

func TestMemfitStreamClassification(t *testing.T) {
	answer := memfitWorkerEvent{Type: string(schema.EVENT_TYPE_STREAM), NodeID: "re-act-loop-answer-payload"}
	require.True(t, isMemfitAnswerStream(answer))
	require.False(t, isMemfitThoughtStream(answer))

	thought := memfitWorkerEvent{Type: string(schema.EVENT_TYPE_STREAM), NodeID: "re-act-loop-thought", VizSource: "human_readable_thought"}
	require.False(t, isMemfitAnswerStream(thought))
	require.True(t, isMemfitThoughtStream(thought))
}

func TestMemfitProcessEventHumanization(t *testing.T) {
	ui := &memfitTUI{width: 72, height: 24}
	require.True(t, ui.recordProcessEvent("intent-1", memfitWorkerEvent{
		Type: string(schema.EVENT_TYPE_INTENT_RECOGNITION),
		Content: `{"intent":"inspect repository","matched_tool_names":["read_file","bash"],` +
			`"recommended_tools":{"description":"must stay hidden"}}`,
	}))
	require.Len(t, ui.processItems, 1)
	require.Equal(t, "Intent", ui.processItems[0].kind)
	require.Contains(t, ui.processItems[0].detail, "inspect repository")
	require.Contains(t, ui.processItems[0].detail, "read_file, bash")
	require.NotContains(t, ui.processItems[0].detail, "description")

	require.True(t, ui.recordProcessEvent("tool-event", memfitWorkerEvent{
		Type:       string(schema.EVENT_TOOL_CALL_START),
		CallToolID: "tool-1",
		Content:    `{"tool":{"verbose_name_i18n":{"Zh":"文件读取"}},"params":{"file_path":"README.md"}}`,
	}))
	require.True(t, ui.recordProcessEvent("tool-done", memfitWorkerEvent{
		Type:       string(schema.EVENT_TOOL_CALL_PARAM),
		CallToolID: "tool-1",
		Content:    `{"call_tool_id":"tool-1","params":{"file_path":"README.md"}}`,
	}))
	require.True(t, ui.recordProcessEvent("tool-done", memfitWorkerEvent{
		Type:       string(schema.EVENT_TOOL_CALL_RESULT),
		CallToolID: "tool-1",
		Content:    `{"call_tool_id":"tool-1","result":"first line\n+added line\n-removed line"}`,
	}))
	require.Len(t, ui.processItems, 2, "tool lifecycle events must update one row")
	require.Equal(t, "Read", ui.processItems[1].kind)
	require.Equal(t, memfitProcessDone, ui.processItems[1].state)
	require.Contains(t, ui.processItems[1].detail, "Result:\nfirst line")
	require.Contains(t, ui.processItems[1].detail, "Input:\nREADME.md")
	require.NotContains(t, ui.processItems[1].detail, "call_tool_id")
}

func TestMemfitProcessTimelineClassifiesCoreStreamsAndHidesDetailsByDefault(t *testing.T) {
	ui := &memfitTUI{width: 72, height: 24, busy: true}
	ui.appendProcessThought("thought-1", "先判断意图，再读取必要信息。")
	collapsed := joinMemfitLiveLines(ui.processPanelLines())
	require.Contains(t, collapsed, "Thinking ▸")
	require.NotContains(t, collapsed, "先判断意图")
	ui.toggleProcessItem("thought:thought-1")
	require.Contains(t, joinMemfitLiveLines(ui.processPanelLines()), "先判断意图")

	streams := []struct {
		typeName schema.EventType
		content  string
		kind     string
	}{
		{schema.EVENT_TYPE_INTENT_RECOGNITION, `{"intent":"inspect repository"}`, "Intent"},
		{schema.EVENT_TYPE_PERCEPTION, `{"summary":"repository context understood"}`, "Understanding"},
		{schema.EVENT_TYPE_ACTION, `{"action":"read the README","action_type":"tool"}`, "Action"},
		{schema.EVENT_TYPE_OBSERVATION, `{"observation":"project purpose found","source":"README.md"}`, "Observation"},
		{schema.EVENT_TYPE_START_PLAN_AND_EXECUTION, `{"message":"prepare a minimal plan"}`, "Plan"},
		{schema.EVENT_TYPE_MEMORY_SEARCH_QUICKLY, `{}`, "Memory"},
	}
	for index, stream := range streams {
		require.True(t, ui.recordProcessEvent(string(rune('a'+index)), memfitWorkerEvent{
			Type:    string(stream.typeName),
			Content: stream.content,
		}))
		require.Equal(t, stream.kind, ui.processItems[len(ui.processItems)-1].kind)
	}
}

func TestMemfitFooterSegmentsAdaptToTerminalWidth(t *testing.T) {
	ui := &memfitTUI{
		config: memfitStartConfig{Model: "test-model", ReviewPolicy: "yolo"},
		width:  88,
	}
	left, right := ui.footerSegments("○")
	require.Equal(t, "○ Ready", left)
	require.Equal(t, "test-model · YOLO · click/Ctrl+O details · /help", right)

	ui.width = 30
	left, right = ui.footerSegments("○")
	require.Equal(t, "○ Ready", left)
	require.Equal(t, "test-model · YOLO", right)

	ui.width = 24
	left, right = ui.footerSegments("○")
	require.Equal(t, "○ Ready", left)
	require.Equal(t, "YOLO", right)

	ui.busy = true
	ui.activity = "Thinking"
	ui.queued = []memfitQueuedInput{{text: "next"}}
	left, right = ui.footerSegments("◆")
	require.Equal(t, "◆ Thinking · Q1", left)
	require.Equal(t, "YOLO", right)
}

func TestMemfitStructuredTimelineKeepsSemanticsAndDropsBreadcrumbs(t *testing.T) {
	kind, group, detail, state, ok := classifyMemfitTimelineItem(`{
		"type":"text",
		"raw_text":"[intent_analysis] [task:main]:\n意图识别：检查仓库结构",
		"content":"意图识别：检查仓库结构"
	}`)
	require.True(t, ok)
	require.Equal(t, "Intent", kind)
	require.Equal(t, "intent", group)
	require.Equal(t, "意图识别：检查仓库结构", detail)
	require.Equal(t, memfitProcessDone, state)

	_, _, _, _, ok = classifyMemfitTimelineItem(`{
		"type":"text",
		"raw_text":"[current task user input]:\nread README",
		"content":"read README"
	}`)
	require.False(t, ok, "the visible user transcript must not be duplicated as an activity")

	_, _, _, _, ok = classifyMemfitTimelineItem(`{
		"type":"tool_result",
		"content":"large duplicate tool output"
	}`)
	require.False(t, ok, "native tool lifecycle events own tool result rendering")
}

func joinMemfitLiveLines(lines []memfitLiveLine) string {
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		values = append(values, line.text)
	}
	return strings.Join(values, "\n")
}

func TestMemfitAnswerStreamsSelectLatestWriter(t *testing.T) {
	var streams memfitAnswerStreams
	streams.Append("writer-1", "first")
	streams.Append("writer-2", "final ")
	streams.Append("writer-2", "answer")
	require.Equal(t, "writer-1", streams.FirstID())
	require.Equal(t, "first", streams.First())
	require.Equal(t, "final answer", streams.Last())
	streams.Reset()
	require.Empty(t, streams.Last())
}

func TestMemfitPlainEmitsReplayedAnswerOnce(t *testing.T) {
	client := newScriptedMemfitClient()
	defer client.Close()
	readSide, writeSide, err := os.Pipe()
	require.NoError(t, err)
	defer readSide.Close()
	defer writeSide.Close()
	oldStdout := os.Stdout
	os.Stdout = writeSide
	defer func() { os.Stdout = oldStdout }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, runMemfitPlain(ctx, client, memfitStartConfig{}, "once"))
	require.NoError(t, writeSide.Close())
	os.Stdout = oldStdout
	output, err := io.ReadAll(readSide)
	require.NoError(t, err)
	require.NoError(t, readSide.Close())
	require.Equal(t, "echo: once\n", string(output))
}
