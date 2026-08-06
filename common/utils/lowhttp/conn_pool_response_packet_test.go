package lowhttp

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendHTTPResponseRest(t *testing.T) {
	t.Run("empty rest keeps captured packet", func(t *testing.T) {
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
		combined := appendHTTPResponseRest(packet, nil)

		require.Equal(t, packet, combined)
		require.Same(t, &packet[0], &combined[0])
	})

	t.Run("recovery result owns packet and rest", func(t *testing.T) {
		packet := []byte("HTTP/1.1 200 OK\r\n\r\n")
		rest := []byte("trailing")
		combined := appendHTTPResponseRest(packet, rest)

		require.Equal(t, "HTTP/1.1 200 OK\r\n\r\ntrailing", string(combined))
		packet[0] = 'X'
		rest[0] = 'X'
		require.Equal(t, "HTTP/1.1 200 OK\r\n\r\ntrailing", string(combined))
	})

	t.Run("rest without captured packet", func(t *testing.T) {
		rest := []byte("HTTP/1.1 200 OK\r\n\r\n")
		combined := appendHTTPResponseRest(nil, rest)

		require.Equal(t, rest, combined)
		rest[0] = 'X'
		require.Equal(t, byte('H'), combined[0])
	})
}

var connPoolResponsePacketSink []byte

func BenchmarkConnPoolResponsePacketSuccess256K(b *testing.B) {
	packet := bytes.Repeat([]byte{'r'}, 256<<10)

	b.Run("eager-recovery-buffer", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var recovery bytes.Buffer
			_, _ = recovery.Write(packet)
			connPoolResponsePacketSink = recovery.Bytes()
		}
	})

	b.Run("lazy-recovery-buffer", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			connPoolResponsePacketSink = appendHTTPResponseRest(packet, nil)
		}
	})
}
