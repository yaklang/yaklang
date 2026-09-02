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
		{"\x17", memfitKeyDeleteWord},
	}
	for _, test := range tests {
		key, err := readMemfitKey(bufio.NewReader(strings.NewReader(test.input)))
		require.NoError(t, err)
		require.Equal(t, test.kind, key.kind, "input %q", test.input)
	}
}

func TestMemfitReadableContentExtraction(t *testing.T) {
	require.Equal(t, "Need approval", extractMemfitReadableContent(`{"question":"Need approval"}`))
	require.Equal(t, "plain text", extractMemfitReadableContent("plain text"))
	require.Equal(t, "id-1", extractMemfitInteractiveID("fallback", `{"id":"id-1"}`))
}

func TestMemfitStreamClassification(t *testing.T) {
	answer := memfitWorkerEvent{Type: string(schema.EVENT_TYPE_STREAM), NodeID: "re-act-loop-answer-payload"}
	require.True(t, isMemfitAnswerStream(answer))
	require.False(t, isMemfitThoughtStream(answer))

	thought := memfitWorkerEvent{Type: string(schema.EVENT_TYPE_STREAM), NodeID: "re-act-loop-thought", VizSource: "human_readable_thought"}
	require.False(t, isMemfitAnswerStream(thought))
	require.True(t, isMemfitThoughtStream(thought))
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
