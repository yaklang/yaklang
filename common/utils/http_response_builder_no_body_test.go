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

func TestReadHTTPResponseStillDecodesOrdinaryChunkedBody(t *testing.T) {
	wire := []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n4\r\ntest\r\n0\r\n\r\n")
	rsp, err := ReadHTTPResponseFromBufioReader(bytes.NewReader(wire), &http.Request{Method: http.MethodGet})
	require.NoError(t, err)
	require.NotEqual(t, http.NoBody, rsp.Body)
	body, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "test")
}
