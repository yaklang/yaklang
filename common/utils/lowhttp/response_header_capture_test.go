package lowhttp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponseHeaderCaptureWriter(t *testing.T) {
	for _, header := range []string{
		"HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n",
		"HTTP/1.1 200 OK\nContent-Type: text/event-stream\n\n",
		"HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\n\r\n",
		"HTTP/1.1 200 OK\nContent-Type: text/event-stream\r\n\n",
	} {
		packet := header + strings.Repeat("data: ignored body\n\n", 1024)
		// Exercise every split, especially between CR/LF and header/body bytes.
		for split := 0; split <= len(header)+1; split++ {
			var raw bytes.Buffer
			capture := &responseHeaderCaptureWriter{dst: &raw}
			for _, part := range []string{packet[:split], packet[split:]} {
				n, err := capture.Write([]byte(part))
				require.NoError(t, err)
				require.Equal(t, len(part), n)
			}
			require.Equal(t, header, raw.String())
			require.Less(t, raw.Cap(), len(packet), "body bytes must not be buffered")
			capture.Reset()
			_, err := capture.Write([]byte("HTTP/1.1 204 No Content\r\n\r\nignored"))
			require.NoError(t, err)
			require.Equal(t, "HTTP/1.1 204 No Content\r\n\r\n", raw.String())
		}
	}
}
