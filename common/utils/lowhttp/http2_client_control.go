package lowhttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/http/httpguts"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// takeSendQuota reserves both levels of flow-control credit atomically. Small
// remaining windows are usable; waiting never subtracts unsent bytes or prevents
// other streams from sending, and all waits observe cancellation and shutdown.
func (cs *http2ClientStream) takeSendQuota(ctx context.Context, remaining int) (int, error) {
	c := cs.h2Conn
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if c.closed {
			return 0, errH2ConnClosed
		}
		if cs.readEndStream.Load() {
			return 0, errH2UploadAborted
		}
		n := int64(remaining)
		if n > defaultMaxFrameSize {
			n = defaultMaxFrameSize
		}
		if n > int64(c.maxFrameSize) {
			n = int64(c.maxFrameSize)
		}
		if n > c.sendWindow {
			n = c.sendWindow
		}
		if n > cs.sendWindow {
			n = cs.sendWindow
		}
		if n > 0 {
			c.sendWindow -= n
			cs.sendWindow -= n
			return int(n), nil
		}
		c.streamsCond.Wait()
	}
}

var errH2UploadAborted = fmt.Errorf("http2: response ended before upload completed")

func (cs *http2ClientStream) finishUpload() {
	c := cs.h2Conn
	c.frWriteMutex.Lock()
	defer c.frWriteMutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	c.writeCtx = ctx
	// An empty DATA frame closes our half without invalidating a successful
	// response that the server has already sent (including early 4xx responses).
	err := c.fr.WriteData(cs.ID, true, nil)
	if err == nil {
		err = c.flushFrames()
	}
	c.writeCtx = nil
	cs.sentEndStream = err == nil
	if err != nil {
		c.setClose()
	}
}

// Called by the read loop without holding the connection state mutex.
func (rl *http2ClientConnReadLoop) failConnection(code http2.ErrCode, reason string) {
	c := rl.h2Conn
	err := fmt.Errorf("http2: %s: %w", reason, http2.ConnectionError(code))
	c.mu.Lock()
	if c.readErr == nil {
		c.readErr = err
	}
	c.mu.Unlock()
	c.setCloseReason(err.Error())
	c.setClose()
}

// Called with streamReadMu held. Peer errors remain stream-scoped and are
// returned to the caller instead of masquerading as successful empty responses.
func (rl *http2ClientConnReadLoop) failStream(cs *http2ClientStream, code http2.ErrCode, reason string) {
	cs.streamErr = http2.StreamError{StreamID: cs.ID, Code: code, Cause: fmt.Errorf("%s", reason)}
	cs.setEndStream()
	rl.h2Conn.frWriteMutex.Lock()
	err := rl.h2Conn.fr.WriteRSTStream(cs.ID, code)
	if err == nil {
		err = rl.h2Conn.flushFrames()
	}
	rl.h2Conn.frWriteMutex.Unlock()
	if err != nil {
		rl.h2Conn.setClose()
	}
}

func (cs *http2ClientStream) requestContextError() error {
	if cs.requestCtx != nil {
		return cs.requestCtx.Err()
	}
	return nil
}

// Framer calls Write with frWriteMutex held. Bound even control-frame writes so
// an unresponsive peer cannot strand the read loop and every request on it.
// Join cancellation before clearing the deadline or allowing another writer.
type h2DeadlineWriter struct{ conn *http2ClientConn }

func (w *h2DeadlineWriter) Write(p []byte) (int, error) {
	c := w.conn
	deadline := time.Now().Add(5 * time.Second)
	if c.writeCtx != nil {
		if err := c.writeCtx.Err(); err != nil {
			return 0, err
		}
		if d, ok := c.writeCtx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
	}
	if err := c.conn.SetWriteDeadline(deadline); err != nil {
		return 0, err
	}
	var stop func() bool
	var done chan struct{}
	if c.writeCtx != nil && c.writeCtx.Done() != nil {
		done = make(chan struct{})
		stop = context.AfterFunc(c.writeCtx, func() { _ = c.conn.SetWriteDeadline(time.Now()); close(done) })
	}
	n, err := c.conn.Write(p)
	if stop != nil && !stop() {
		<-done
	}
	_ = c.conn.SetWriteDeadline(time.Time{})
	return n, err
}

func (rl *http2ClientConnReadLoop) applyResponseHeaders(cs *http2ClientStream, fields []hpack.HeaderField, endStream bool) {
	header := make(http.Header)
	status := ""
	sawRegular := false
	trailer := cs.readHeaderEnd
	for _, h := range fields {
		if h.IsPseudo() {
			if sawRegular || trailer || h.Name != ":status" || status != "" {
				rl.failStream(cs, http2.ErrCodeProtocol, "invalid response pseudo headers")
				return
			}
			status = h.Value
			continue
		}
		sawRegular = true
		if h.Name != strings.ToLower(h.Name) || !httpguts.ValidHeaderFieldName(h.Name) || !httpguts.ValidHeaderFieldValue(h.Value) || strings.Trim(h.Value, " \t") != h.Value {
			rl.failStream(cs, http2.ErrCodeProtocol, "invalid response header field")
			return
		}
		switch h.Name {
		case "connection", "proxy-connection", "keep-alive", "upgrade", "transfer-encoding":
			rl.failStream(cs, http2.ErrCodeProtocol, "connection-specific response header")
			return
		}
		if trailer && (h.Name == "content-length" || h.Name == "trailer" || h.Name == "host") {
			rl.failStream(cs, http2.ErrCodeProtocol, "invalid response trailer")
			return
		}
		header.Add(h.Name, h.Value)
	}
	if trailer {
		if !endStream {
			rl.failStream(cs, http2.ErrCodeProtocol, "trailers without END_STREAM")
			return
		}
		cs.resp.Trailer = header
		rl.endResponse(cs)
		return
	}
	code, err := strconv.Atoi(status)
	if err != nil || len(status) != 3 || code < 100 || code > 999 || code == 101 {
		rl.failStream(cs, http2.ErrCodeProtocol, "missing or invalid :status")
		return
	}
	if code < 200 {
		cs.interimResponses++
		if endStream || cs.interimResponses > 16 {
			rl.failStream(cs, http2.ErrCodeProtocol, "invalid or excessive informational responses")
		}
		return
	}
	cs.contentLength = -1
	for _, v := range header.Values("Content-Length") {
		if v == "" || strings.IndexFunc(v, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			rl.failStream(cs, http2.ErrCodeProtocol, "invalid content-length")
			return
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 || (cs.contentLength >= 0 && cs.contentLength != n) {
			rl.failStream(cs, http2.ErrCodeProtocol, "invalid content-length")
			return
		}
		cs.contentLength = n
	}
	cs.resp.StatusCode = code
	cs.resp.Header = header
	cs.resp.ContentLength = cs.contentLength
	cs.readHeaderEnd = true
	cs.handleHeadersDone()
	if endStream {
		rl.endResponse(cs)
	}
}

func (rl *http2ClientConnReadLoop) endResponse(cs *http2ClientStream) {
	noBody := cs.req != nil && cs.req.Method == http.MethodHead || cs.resp.StatusCode == 304 || cs.resp.StatusCode == 204
	if cs.contentLength >= 0 && !noBody && cs.bodyReceived != cs.contentLength {
		cs.streamErr = io.ErrUnexpectedEOF
	}
	cs.setEndStream()
}

// retire stops cache reuse while allowing already accepted streams to drain.
func (c *http2ClientConn) retire() {
	c.mu.Lock()
	c.readGoAway = true
	idle := c.activeStreams == 0
	c.streamsCond.Broadcast()
	c.mu.Unlock()
	if c.pc != nil {
		c.pc.removeConn()
	}
	if idle {
		c.setClose()
	}
}

func h2RequestCanRetry(req *http.Request, err error) bool {
	var streamErr http2.StreamError
	if errors.As(err, &streamErr) {
		return streamErr.Code == http2.ErrCodeRefusedStream
	}
	if !errors.Is(err, errH2ConnClosed) || req == nil {
		return false
	}
	switch req.Method {
	case "GET", "HEAD", "OPTIONS", "TRACE", "PUT", "DELETE":
		return true
	}
	return false
}

var errH2BodyWindowExceeded = errors.New("http2: response stream receive window exceeded")

func (c *http2ClientConn) consumeStreamBytes(id uint32, n int) {
	c.streamReadMu.Lock()
	defer c.streamReadMu.Unlock()
	cs := c.streamByID(id)
	if cs == nil || cs.readEndStream.Load() {
		return
	}
	cs.recvPending += uint32(n)
	if cs.recvPending < c.receiveUpdateThreshold {
		return
	}
	c.frWriteMutex.Lock()
	err := c.fr.WriteWindowUpdate(id, cs.recvPending)
	cs.recvWindow += int64(cs.recvPending)
	cs.recvPending = 0
	if err == nil {
		err = c.flushFrames()
	}
	c.frWriteMutex.Unlock()
	if err != nil {
		c.setClose()
	}
}
func (c *http2ClientConn) closeStreamReader(id uint32) {
	c.streamReadMu.Lock()
	defer c.streamReadMu.Unlock()
	cs := c.streamByID(id)
	if cs == nil || cs.readEndStream.Load() {
		return
	}
	rl := http2ClientConnReadLoop{h2Conn: c}
	rl.failStream(cs, http2.ErrCodeCancel, "response body reader closed")
}

func (c *http2ClientConn) flushFrames() error {
	if c.bw != nil {
		return c.bw.Flush()
	}
	return nil
}

func (rl *http2ClientConnReadLoop) isIdleStream(id uint32) bool {
	next := atomic.LoadUint32(&rl.h2Conn.currentStreamID)
	return id == 0 || id%2 == 0 || (next != 0 && id >= next)
}
