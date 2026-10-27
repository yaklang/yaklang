package yakscript

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
)

func TestReadSSE(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n", "\r"} {
		raw := "\ufeff: heartbeat\nevent: progress\ndata: 中文\ndata: second line\n\nevent: done\ndata: [DONE]\n\n"
		var events []string
		err := ReadSSE(iotest.OneByteReader(strings.NewReader(strings.ReplaceAll(raw, "\n", newline))), func(event, data string) error {
			events = append(events, event+":"+data)
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, []string{"progress:中文\nsecond line", "done:[DONE]"}, events)
	}
	t.Run("incomplete event", func(t *testing.T) {
		err := ReadSSE(strings.NewReader("event: message\ndata: partial\n"), func(string, string) error { t.Fatal("incomplete event dispatched"); return nil })
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	})
	t.Run("stop at terminal event", func(t *testing.T) {
		require.NoError(t, ReadSSE(strings.NewReader("data: [DONE]\n\ndata: ignored\n\n"), func(_, data string) error { require.Equal(t, "[DONE]", data); return io.EOF }))
	})
}
