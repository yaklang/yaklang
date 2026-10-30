package sharkcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func streamEvidenceTextForTest(u *tui) string {
	var text strings.Builder
	for _, row := range u.streamEvidenceRows() {
		text.WriteString(row.text)
		text.WriteByte('\n')
	}
	return text.String()
}

func TestStreamBeginningEvidenceDoesNotImposeHTTPOnOtherProtocols(t *testing.T) {
	req, _ := smb1ResponseDisplaySource()
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"SMB1", req}, {"TLSv1.3", []byte{0x17, 3, 3, 0, 4, 0x11, 0x22, 0x33, 0x44}},
		{"SSH", []byte("SSH-2.0-source-test\r\n")}, {"gRPC", []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := bytes.Clone(tc.data)
			v := &streamSnapshot{protocolStack: []string{tc.name}, ports: [2]uint16{50000, 80}, prefix: [2][]byte{tc.data}}
			view := streamEvidenceTextForTest(&tui{stream: v})
			require.Contains(t, view, "Observed connection: "+tc.name)
			require.NotContains(t, view, "No complete, valid HTTP header")
			require.NotContains(t, view, "Verified HTTP at captured offset 0")
			require.Contains(t, view, "actual TCP beginning unknown")
			require.Contains(t, view, "Raw first 32 bytes")
			require.Equal(t, original, v.prefix[0])
		})
	}
}

func TestStreamBeginningEvidenceKeepsIndependentHTTPProof(t *testing.T) {
	header := "POST /first HTTP/1.1\r\nHost: source.test\r\nContent-Length: 10000\r\n\r\n"
	data := append([]byte(header), 0xff, 0, 'S', 'M', 'B')
	for _, connection := range []string{"SMB1", "TLSv1.3", "HTTP", ""} {
		v := &streamSnapshot{protocolStack: []string{connection}, ports: [2]uint16{50000, 445}, prefix: [2][]byte{data}}
		view := streamEvidenceTextForTest(&tui{stream: v})
		require.Contains(t, view, "Verified HTTP at captured offset 0")
		require.Contains(t, view, "Host: source.test")
		require.Contains(t, view, "Content-Length: 10000")
		require.Contains(t, view, "body bytes are not proof of HTTP")
	}
	for _, payload := range []string{"GET /", "GET / HTTP/1.1\r\nHost: source.test\r\n", "GET / HTTP/9.9\r\n\r\n", "\xff\x00\xaa"} {
		v := &streamSnapshot{protocol: "HTTP", evidence: "port hint", ports: [2]uint16{50000, 80}, prefix: [2][]byte{[]byte(payload)}}
		view := streamEvidenceTextForTest(&tui{stream: v})
		require.NotContains(t, view, "Verified HTTP at captured offset 0")
		require.NotContains(t, view, "Prefix signature: HTTP")
		require.Contains(t, view, "Protocol at captured offset 0 is not identified.")
	}
}
