package sharkcli

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTerminalDecoderFragmentedSGRAndUTF8(t *testing.T) {
	var d inputDecoder
	var events []terminalEvent
	now := time.Now()
	stream := "\x1b[<64;301;42M\x1b[<0;310;44M\x1b[<32;310;49M\x1b[<0;310;49m\x1b[1;5A\x1bO1;2B\x1b[Z中"
	for i := range []byte(stream) {
		events = append(events, d.feed([]byte{stream[i]}, now.Add(time.Duration(i)*time.Millisecond))...)
	}
	require.Len(t, events, 8)
	require.Equal(t, mouseWheel, events[0].mouse.action)
	require.Equal(t, 300, events[0].mouse.x)
	require.Equal(t, 41, events[0].mouse.y)
	require.Equal(t, -3, events[0].mouse.delta)
	require.Equal(t, mousePress, events[1].mouse.action)
	require.Equal(t, mouseMotion, events[2].mouse.action)
	require.Equal(t, mouseRelease, events[3].mouse.action)
	require.Equal(t, "\x1b[A", events[4].key)
	require.Equal(t, "\x1b[B", events[5].key)
	require.Equal(t, "\x1b[Z", events[6].key)
	require.Equal(t, "中", events[7].key)
	require.Empty(t, d.pending)
}
func TestTerminalDecoderMouseFallbacksAndFocus(t *testing.T) {
	var d inputDecoder
	events := d.feed(append([]byte("\x1b[M"), []byte{32 + 65, 33 + 90, 33 + 20}...), time.Now())
	require.Len(t, events, 1)
	require.Equal(t, mouseWheel, events[0].mouse.action)
	require.Equal(t, 3, events[0].mouse.delta)
	require.Equal(t, 90, events[0].mouse.x)
	events = d.feed([]byte("\x1b[96;250;21M\x1b[I\x1b[O\x1b[<66;20;21M"), time.Now())
	require.Len(t, events, 4)
	require.Equal(t, mouseWheel, events[0].mouse.action)
	require.Equal(t, -3, events[0].mouse.delta)
	require.Equal(t, 1, events[1].focus)
	require.Equal(t, -1, events[2].focus)
	require.True(t, events[3].mouse.horizontal)
}
func TestTerminalDecoderPasteAndEscapeNeverBecomeShortcuts(t *testing.T) {
	var d inputDecoder
	var events []terminalEvent
	now := time.Now()
	stream := "\x1b[200~q\r\x1b[A你好\x1b[201~"
	for _, b := range []byte(stream) {
		events = append(events, d.feed([]byte{b}, now)...)
	}
	require.Len(t, events, 1)
	require.Equal(t, "q\r\x1b[A你好", events[0].paste)
	require.Empty(t, events[0].key)
	u := &tui{editor: "BPF", notice: "Enter applies · Esc cancels · Ctrl+U clears"}
	require.False(t, u.event(events[0], &captureSource{}))
	require.Equal(t, "q  [A你好", u.input)
	require.Empty(t, d.feed([]byte{27}, now))
	require.Empty(t, d.expire(now.Add(50*time.Millisecond)))
	expired := d.expire(now.Add(100 * time.Millisecond))
	require.Equal(t, []terminalEvent{{key: "\x1b"}}, expired)
	u.event(expired[0], &captureSource{})
	require.Empty(t, u.editor)
	require.Empty(t, u.input)
	require.Empty(t, u.notice)
	events = d.feed([]byte("\x1bq"), now)
	require.Equal(t, []terminalEvent{{key: "\x1b"}, {key: "q"}}, events)
}
func TestMalformedReportsAreConsumedAndPasteIsBounded(t *testing.T) {
	var d inputDecoder
	now := time.Now()
	require.Empty(t, d.feed([]byte("\x1b[<64;0;3M\x1b[<64;999999;3M\x1b[<128;20;30M\x1b[6;6R"), now))
	require.Empty(t, d.feed([]byte("\x1b[<"+strings.Repeat("9", 256)), now))
	events := d.feed([]byte("Mq"), now)
	require.Equal(t, []terminalEvent{{key: "q"}}, events)
	events = d.feed([]byte("\x1b[200~"+strings.Repeat("x", 128<<10)+"\x1b[201~"), now)
	require.Len(t, events, 1)
	require.Len(t, events[0].paste, 64<<10)
	u := &tui{editor: "BPF"}
	u.event(events[0], &captureSource{})
	require.Len(t, u.input, 4096)
}
func TestTerminalModesHaveMatchingCleanup(t *testing.T) {
	for _, mode := range []string{"1000", "1002", "1006", "1004", "2004"} {
		require.Contains(t, terminalEnter, "\x1b[?"+mode+"h")
		require.Contains(t, terminalLeave, "\x1b[?"+mode+"l")
	}
	require.Contains(t, terminalLeave, "\x1b[?25h")
	require.Contains(t, terminalLeave, "\x1b[?1049l")
}
