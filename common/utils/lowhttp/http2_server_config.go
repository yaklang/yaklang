package lowhttp

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"io"
	"net"
	"strings"
	"sync"
)

type http2ConnectionConfig struct {
	handler      func(context.Context, []byte, io.ReadCloser) ([]byte, io.ReadCloser, error)
	frame        *http2.Framer
	conn         net.Conn
	frWriteMutex *sync.Mutex

	// writer
	hencBuf   *bytes.Buffer
	henc      *hpack.Encoder
	hencMutex *sync.Mutex

	wg *sync.WaitGroup

	connWindowControl     *windowSizeControl
	peerInitialWindowSize uint32
	streamWindows         sync.Map // streamID (uint32) -> *windowSizeControl
}

func (c *http2ConnectionConfig) close() error {
	return c.conn.Close()
}

func (c *http2ConnectionConfig) getStreamWindow(streamID uint32) *windowSizeControl {
	if v, ok := c.streamWindows.Load(streamID); ok {
		return v.(*windowSizeControl)
	}
	w := newControl(int64(c.peerInitialWindowSize))
	actual, _ := c.streamWindows.LoadOrStore(streamID, w)
	return actual.(*windowSizeControl)
}

func (c *http2ConnectionConfig) removeStreamWindow(streamID uint32) {
	c.streamWindows.Delete(streamID)
}

// h2IllegalResponseHeaderFields are HTTP/1.x connection-specific header
// fields that must never be forwarded into an HTTP/2 response (RFC 7540
// Section 8.1.2.2). Responses translated from an HTTP/1.x upstream (the
// client negotiated h2 while the origin speaks h1) routinely carry these;
// strict h2 clients (browsers, x/net) reset the stream as malformed.
//
// content-length is also dropped: h2 DATA frames are self-delimiting, and a
// stale length from the h1 representation (dechunking, decompression, or a
// MITM body rewrite) triggers CONTENT_LENGTH_MISMATCH on strict clients.
var h2IllegalResponseHeaderFields = map[string]struct{}{
	"connection":          {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailer":             {},
	"transfer-encoding":   {},
	"upgrade":             {},
	"content-length":      {},
}

func (c *http2ConnectionConfig) writer(wrapper *h2RequestState, header []byte, body io.ReadCloser) error {
	if c.frame == nil {
		return utils.Error("h2 server frame config is nil")
	}
	streamId := wrapper.streamId
	frame := c.frame
	henc := c.henc
	buf := c.hencBuf
	frWriteMutex := c.frWriteMutex

	// Encode and write the response headers while holding hencMutex, so the
	// shared hpack encoder's dynamic table stays consistent with the wire
	// order across concurrent streams.
	headerErr := func() error {
		c.hencMutex.Lock()
		defer c.hencMutex.Unlock()
		buf.Reset()
		SplitHTTPPacket(header, nil, func(proto string, code int, codeMsg string) error {
			henc.WriteField(hpack.HeaderField{Name: ":status", Value: fmt.Sprint(code)})
			return nil
		}, func(line string) string {
			k, v := SplitHTTPHeader(line)
			lk := strings.ToLower(k)
			if _, illegal := h2IllegalResponseHeaderFields[lk]; illegal {
				return line // do not encode connection-specific h1 headers into h2
			}
			henc.WriteField(hpack.HeaderField{Name: lk, Value: v})
			return line
		})
		hpackHeaderBytes := append([]byte(nil), buf.Bytes()...)
		buf.Reset()

		ret := funk.Chunk(hpackHeaderBytes, defaultMaxFrameSize).([][]byte)
		for index, item := range ret {
			if index == 0 {
				frWriteMutex.Lock()
				err := frame.WriteHeaders(http2.HeadersFrameParam{
					StreamID:      uint32(streamId),
					BlockFragment: item,
					EndStream:     false,
					EndHeaders:    index == len(ret)-1,
				})
				frWriteMutex.Unlock()
				if err != nil {
					return utils.Wrapf(err, "h2framer write header(%v) for stream:%v failed", len(hpackHeaderBytes), streamId)
				}
			} else {
				frWriteMutex.Lock()
				err := frame.WriteContinuation(uint32(streamId), index == len(ret)-1, item)
				frWriteMutex.Unlock()
				if err != nil {
					return utils.Wrapf(err, "h2framer write header(%v)-continuation for stream:%v failed", len(hpackHeaderBytes), streamId)
				}
			}
		}
		return nil
	}()
	if headerErr != nil {
		return headerErr
	}

	results, err := io.ReadAll(body)
	streamWindow := c.getStreamWindow(uint32(streamId))
	defer c.removeStreamWindow(uint32(streamId))

	if len(results) > 0 {
		chunks := funk.Chunk(results, defaultMaxFrameSize).([][]byte)
		for index, dataFrameBytes := range chunks {
			dataLen := int64(len(dataFrameBytes))

			streamWindow.decreaseWindowSize(dataLen)
			c.connWindowControl.decreaseWindowSize(dataLen)

			frWriteMutex.Lock()
			dataFrameErr := frame.WriteData(uint32(streamId), index == len(chunks)-1, dataFrameBytes)
			frWriteMutex.Unlock()
			if dataFrameErr != nil {
				return utils.Wrapf(dataFrameErr, "framer WriteData for stream{%v} failed", streamId)
			}
		}
	} else {
		frWriteMutex.Lock()
		dataFrameErr := frame.WriteData(uint32(streamId), true, nil)
		frWriteMutex.Unlock()
		if dataFrameErr != nil {
			return utils.Wrapf(dataFrameErr, "framer WriteData for stream{%v} failed", streamId)
		}
	}
	if err != nil {
		return utils.Wrapf(err, "read body for stream{%v} failed", streamId)
	}
	return nil
}

type h2Option func(*http2ConnectionConfig)

func withH2Handler(h func(header []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error)) h2Option {
	return func(c *http2ConnectionConfig) {
		c.handler = func(_ context.Context, header []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error) {
			return h(header, body)
		}
	}
}

func withH2ContextHandler(h func(context.Context, []byte, io.ReadCloser) ([]byte, io.ReadCloser, error)) h2Option {
	return func(c *http2ConnectionConfig) {
		c.handler = h
	}
}

func (c *http2ConnectionConfig) handleRequest(wrapper *h2RequestState, header []byte, body io.ReadCloser) error {
	if c == nil || c.handler == nil {
		return utils.Error("h2 server handler config is nil")
	}
	header, rc, err := c.handler(wrapper.ctx, header, body)
	if err != nil {
		return utils.Errorf("waiting for userspace handling for h2 stream(%v) failed: %v", wrapper.streamId, err)
	}
	return c.writer(wrapper, header, rc)
}
