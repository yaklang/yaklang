package match

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPMessageLengthContentLength(t *testing.T) {
	completePayload := []byte("POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\nhello")
	length, complete := HTTPMessageLength(completePayload)
	require.True(t, complete)
	require.Equal(t, len(completePayload), length)

	partialPayload := []byte("POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\nhel")
	_, complete = HTTPMessageLength(partialPayload)
	require.False(t, complete)
}

func TestHTTPMessageLengthChunked(t *testing.T) {
	completePayload := []byte("POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n4\r\npayl\r\n4\r\noade\r\n0\r\n\r\n")
	length, complete := HTTPMessageLength(completePayload)
	require.True(t, complete)
	require.Equal(t, len(completePayload), length)

	partialPayload := []byte("POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n4\r\npayl\r\n")
	_, complete = HTTPMessageLength(partialPayload)
	require.False(t, complete)
}

func TestHTTPMessageLengthHeadersOnly(t *testing.T) {
	payload := []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	length, complete := HTTPMessageLength(payload)
	require.True(t, complete)
	require.Equal(t, len(payload), length)
}
