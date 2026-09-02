package yakcmds

import (
	"bufio"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestMemfitSanitizeTerminalText(t *testing.T) {
	input := "safe\x1b[31m red\x1b[0m\rOVER\x1b]0;title\a\n下一行\x00"
	result := sanitizeMemfitTerminalText(input)
	require.Equal(t, "safe redOVER\n下一行", result)
	require.NotContains(t, result, "\x1b")
}

func TestMemfitInputViewportPreservesWideAndMultilineInput(t *testing.T) {
	buffer := []rune("你好\nworld-very-long-tail")
	text, cursor := memfitInputViewport(buffer, len(buffer), 12)
	require.LessOrEqual(t, runewidth.StringWidth(text), 12)
	require.Contains(t, text, "tail")
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
