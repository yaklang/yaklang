package utils

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadSingleHTTPResponseNoBodyStatusPreservesFollowingBytes(t *testing.T) {
	tail := []byte{0x81, 0x04, 'p', 'i', 'n', 'g'}
	tests := []struct {
		name     string
		response string
		status   int
	}{
		{
			name:     "switching protocols ignores chunked",
			response: "HTTP/1.1 101 Switching Protocols\r\nTransfer-Encoding: chunked\r\n\r\n",
			status:   http.StatusSwitchingProtocols,
		},
		{
			name:     "switching protocols ignores content length",
			response: "HTTP/1.1 101 Switching Protocols\r\nContent-Length: 4096\r\n\r\n",
			status:   http.StatusSwitchingProtocols,
		},
		{
			name:     "switching protocols ignores conflicting framing",
			response: "HTTP/1.1 101 Switching Protocols\r\nContent-Length: 4096\r\nTransfer-Encoding: chunked\r\n\r\n",
			status:   http.StatusSwitchingProtocols,
		},
		{
			name:     "processing ignores chunked",
			response: "HTTP/1.1 102 Processing\r\nTransfer-Encoding: chunked\r\n\r\n",
			status:   http.StatusProcessing,
		},
		{
			name:     "no content ignores chunked",
			response: "HTTP/1.1 204 No Content\r\nTransfer-Encoding: chunked\r\n\r\n",
			status:   http.StatusNoContent,
		},
		{
			name:     "not modified ignores content length and chunked",
			response: "HTTP/1.1 304 Not Modified\r\nContent-Length: 4096\r\nTransfer-Encoding: chunked\r\n\r\n",
			status:   http.StatusNotModified,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wire := append([]byte(test.response), tail...)
			reader := bufio.NewReaderSize(bytes.NewReader(wire), 4096)
			rsp, err := ReadSingleHTTPResponseFromBufioReader(reader, &http.Request{Method: http.MethodGet})
			require.NoError(t, err)
			require.Equal(t, test.status, rsp.StatusCode)
			require.Equal(t, http.NoBody, rsp.Body)
			require.Zero(t, rsp.ContentLength)

			remaining, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, tail, remaining)
		})
	}
}

func TestReadHTTPResponseSkipsInformationalChainAndPreservesFollowingBytes(t *testing.T) {
	tail := []byte{0x81, 0x02, 'o', 'k'}
	wire := []byte("HTTP/1.1 100 Continue\r\nX-Interim: one\r\n\r\n" +
		"HTTP/1.1 103 Early Hints\r\nLink: </style.css>; rel=preload\r\n\r\n" +
		"HTTP/1.1 102 Processing\r\n\r\n" +
		"HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nTransfer-Encoding: chunked\r\n\r\n")
	wire = append(wire, tail...)

	reader := bufio.NewReaderSize(bytes.NewReader(wire), 4096)
	rsp, err := ReadHTTPResponseFromBufioReader(reader, &http.Request{Method: http.MethodGet})
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, rsp.StatusCode)
	require.Equal(t, http.NoBody, rsp.Body)

	remaining, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, tail, remaining)
}

func TestReadHTTPResponseFromBytesKeepsStandalone1xxWithBody(t *testing.T) {
	// Regression for d8bda01fc: skipping all 1xx broke standalone generated packets
	// such as chaosmaker CrossVerify traffic (status 104/107/108 + body).
	raw := []byte("HTTP/1.1 107 Mop\r\nContent-Length: 32\r\n\r\nActive Internet connectionstcp!!")
	rsp, err := ReadHTTPResponseFromBytes(raw, nil)
	require.NoError(t, err)
	require.Equal(t, 107, rsp.StatusCode)

	// Suricata http_server_body matching reads from raw packet; parsing must succeed
	// even when the status is informational.
	providerOK := rsp != nil
	require.True(t, providerOK)

	raw100 := []byte("HTTP/1.1 100 Continue\r\nX-Test: 1\r\n\r\nActive Internet connectionstcp!!")
	rsp100, err := ReadHTTPResponseFromBytes(raw100, nil)
	require.NoError(t, err)
	require.Equal(t, 100, rsp100.StatusCode)
}
