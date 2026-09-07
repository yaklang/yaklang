package lowhttp

import (
	"bytes"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTP2StreamRecycleDropsRequestOwnedReferences(t *testing.T) {
	streamPool := &sync.Pool{New: func() any { return new(http2ClientStream) }}
	h2Conn := &http2ClientConn{http2StreamPool: streamPool}
	stream := &http2ClientStream{
		h2Conn:                 h2Conn,
		ID:                     9,
		req:                    &http.Request{},
		reqPacket:              bytes.Repeat([]byte("q"), 1<<20),
		resp:                   &http.Response{Header: make(http.Header)},
		bodyBuffer:             bytes.NewBuffer(bytes.Repeat([]byte("b"), 1<<20)),
		respPacket:             bytes.Repeat([]byte("r"), 1<<20),
		hPackByte:              bytes.NewBuffer(bytes.Repeat([]byte("h"), 4096)),
		readHeaderEnd:          true,
		callbackLock:           new(sync.Mutex),
		readFirstFrameCallback: func() {},
		option:                 &LowhttpExecConfig{Payloads: []string{"retained"}},
		bodyStreamDone:         make(chan struct{}),
	}

	stream.recycle()
	recycled := streamPool.Get().(*http2ClientStream)
	require.Nil(t, recycled.h2Conn)
	require.Zero(t, recycled.ID)
	require.Nil(t, recycled.req)
	require.Nil(t, recycled.reqPacket)
	require.Nil(t, recycled.resp)
	require.Nil(t, recycled.bodyBuffer)
	require.Nil(t, recycled.respPacket)
	require.Nil(t, recycled.hPackByte)
	require.False(t, recycled.readHeaderEnd)
	require.Nil(t, recycled.readFirstFrameCallback)
	require.Nil(t, recycled.option)
	require.Nil(t, recycled.bodyStreamDone)
}

func TestHTTP2NewStreamClearsReusedProtocolState(t *testing.T) {
	streamPool := &sync.Pool{New: func() any { return new(http2ClientStream) }}
	streamPool.Put(&http2ClientStream{
		readHeaderEnd:          true,
		headersHandled:         true,
		respPacket:             []byte("old response"),
		readFirstFrameCallback: func() {},
	})

	idleTimer := time.NewTimer(time.Hour)
	defer idleTimer.Stop()
	h2Conn := &http2ClientConn{
		mu:              new(sync.Mutex),
		maxStreamsCount: 1,
		idleTimer:       idleTimer,
		http2StreamPool: streamPool,
	}
	h2Conn.streamsCond = sync.NewCond(h2Conn.mu)

	stream, err := h2Conn.newStream(&http.Request{}, []byte("GET / HTTP/2\r\n\r\n"), nil)
	require.NoError(t, err)
	require.False(t, stream.readHeaderEnd)
	require.False(t, stream.headersHandled)
	require.Nil(t, stream.respPacket)
	require.Nil(t, stream.readFirstFrameCallback)

	stream.releaseSlot()
	stream.recycle()
}
