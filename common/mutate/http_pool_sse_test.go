package mutate

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPPoolIsSSERequest(t *testing.T) {
	tests := []struct {
		name   string
		accept string
		want   bool
	}{
		{name: "exact", accept: "text/event-stream", want: true},
		{name: "mixed", accept: "application/json, text/event-stream", want: true},
		{name: "parameters", accept: "text/event-stream; q=0.8", want: true},
		{name: "explicitly_rejected", accept: "application/json, text/event-stream; q=0"},
		{name: "json_profile", accept: `application/json; profile="text/event-stream"`},
		{name: "missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := []byte("GET / HTTP/1.1\r\nHost: example.test\r\nAccept: " + tt.accept + "\r\n\r\n")
			require.Equal(t, tt.want, httpPoolIsSSERequest(raw))
		})
	}
}

func TestHTTPPoolIsSSEResponseHeader(t *testing.T) {
	tests := []struct {
		name        string
		headers     string
		wantSSE     bool
		wantChunked bool
	}{
		{name: "sse_content_length", headers: "Content-Type: text/event-stream; charset=utf-8", wantSSE: true},
		{name: "sse_chunked", headers: "Content-Type: text/event-stream\r\nTransfer-Encoding: gzip, chunked", wantSSE: true, wantChunked: true},
		{name: "json_profile", headers: `Content-Type: application/json; profile="text/event-stream"`},
		{name: "json_hint", headers: "Content-Type: application/json\r\nX-Stream-Hint: text/event-stream"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := []byte("HTTP/1.1 200 OK\r\n" + tt.headers + "\r\n\r\n")
			gotSSE, gotChunked := httpPoolIsSSEResponseHeader(raw)
			require.Equal(t, tt.wantSSE, gotSSE)
			require.Equal(t, tt.wantChunked, gotChunked)
		})
	}
}

func TestHTTPPoolChunkedStreamDecodeLossless(t *testing.T) {
	largeChunk := strings.Repeat("a", 96*1024+17)
	smallChunk := "event: done\ndata: ok\n\n"
	raw := fmt.Sprintf("%x\r\n%s\r\n%x; extension=value\r\n%s\r\n0\r\nTrailer: value\r\n\r\n", len(largeChunk), largeChunk, len(smallChunk), smallChunk)

	var decoded bytes.Buffer
	err := httpPoolChunkedStreamDecode(context.Background(), strings.NewReader(raw), func(data []byte) {
		decoded.Write(data)
	})
	require.NoError(t, err)
	require.Equal(t, largeChunk+smallChunk, decoded.String())

	err = httpPoolChunkedStreamDecode(context.Background(), strings.NewReader("1\r\naXX0\r\n\r\n"), func([]byte) {})
	require.ErrorContains(t, err, "invalid chunk terminator")
}
