package lowhttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

func TestH2EarlyResponseClosesUploadHalf(t *testing.T) {
	headers, end := make(chan struct{}, 1), make(chan struct{}, 1)
	c := newH2ReadLoopTestConn(t, h2FrameCapture(func(f http2.Frame) {
		switch f := f.(type) {
		case *http2.HeadersFrame:
			headers <- struct{}{}
		case *http2.DataFrame:
			if f.StreamEnded() {
				end <- struct{}{}
			}
		}
	}))
	c.currentStreamID, c.initialWindowSize = 1, 0
	req, _ := http.NewRequest("POST", "http://h2.test/", nil)
	cs, err := c.newStream(req, []byte("POST / HTTP/2\r\nHost: h2.test\r\n\r\nupload"), nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cs.doRequest() }()
	select {
	case <-headers:
	case <-time.After(time.Second):
		t.Fatal("no request headers")
	}
	rl := &http2ClientConnReadLoop{h2Conn: c}
	var buf bytes.Buffer
	block := encodeHPACKBlockForCanceledStreamTest(t, hpack.NewEncoder(&buf), &buf, hpack.HeaderField{Name: ":status", Value: "413"})
	rl.processHeaders(readHeadersFrameForCanceledStreamTest(t, 1, block, true, true))
	select {
	case err := <-done:
		if !errors.Is(err, errH2UploadAborted) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("early response stranded upload")
	}
	select {
	case <-end:
	default:
		t.Fatal("local upload half left open")
	}
	resp, _, err := cs.waitResponse(context.Background(), time.Second)
	if err != nil || resp.StatusCode != 413 {
		t.Fatalf("lost early response: %d %v", resp.StatusCode, err)
	}
	if len(c.streams) != 0 || c.activeStreams != 0 {
		t.Fatal("stream leaked")
	}
}

func TestH2CompleteResponseWinsConnectionClose(t *testing.T) {
	for i := 0; i < 50; i++ {
		c := newH2ReadLoopTestConn(t, io.Discard)
		cs := newH2ReadLoopTestStream(t, c, 1)
		rl := &http2ClientConnReadLoop{h2Conn: c}
		rl.applyResponseHeaders(cs, []hpack.HeaderField{{Name: ":status", Value: "200"}}, true)
		c.setClose()
		resp, _, err := cs.waitResponse(context.Background(), time.Second)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("complete response lost on close: %v", err)
		}
	}
}

type h2BrokenWriter struct{}

func (h2BrokenWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }
func TestH2BrokenHeaderWriteCannotReplayPost(t *testing.T) {
	c := newH2ReadLoopTestConn(t, h2BrokenWriter{})
	c.currentStreamID = 1
	req, _ := http.NewRequest("POST", "http://h2.test/", nil)
	cs, err := c.newStream(req, []byte("POST / HTTP/2\r\nHost: h2.test\r\n\r\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = cs.doRequest()
	if err == nil || errors.Is(err, CreateStreamAfterGoAwayErr) || h2RequestCanRetry(req, err) || !c.isClosed() {
		t.Fatalf("unsafe write retry: %v", err)
	}
	cs.abort()
}

func TestH2PeerWindowOverflowScope(t *testing.T) {
	for _, id := range []uint32{0, 1} {
		c := newH2ReadLoopTestConn(t, io.Discard)
		cs := newH2ReadLoopTestStream(t, c, 1)
		c.sendWindow, cs.sendWindow = (1<<31)-1, (1<<31)-1
		rl := &http2ClientConnReadLoop{h2Conn: c}
		rl.processWindowUpdate(&http2.WindowUpdateFrame{FrameHeader: http2.FrameHeader{StreamID: id}, Increment: 1})
		if id == 0 && !c.isClosed() || id == 1 && (c.isClosed() || cs.streamErr == nil) {
			t.Fatalf("wrong overflow scope for %d", id)
		}
	}
}

func TestH2RejectsUnopenedStreams(t *testing.T) {
	for _, id := range []uint32{0, 2, 3, 1001} {
		c := newH2ReadLoopTestConn(t, io.Discard)
		c.currentStreamID = 3
		rl := &http2ClientConnReadLoop{h2Conn: c}
		rl.processHeaders(&http2.HeadersFrame{FrameHeader: http2.FrameHeader{StreamID: id}})
		if !c.isClosed() {
			t.Fatalf("accepted response on unopened stream %d", id)
		}
	}
}

func TestH2SensitiveRequestHeadersNeverIndexed(t *testing.T) {
	var block []byte
	c := newH2ReadLoopTestConn(t, h2FrameCapture(func(f http2.Frame) {
		if h, ok := f.(*http2.HeadersFrame); ok {
			block = append(block, h.HeaderBlockFragment()...)
		}
	}))
	c.currentStreamID = 1
	req, _ := http.NewRequest("GET", "http://h2.test/", nil)
	cs, err := c.newStream(req, []byte("GET / HTTP/2\r\nHost: h2.test\r\nAuthorization: secret\r\nCookie: private\r\n\r\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = cs.doRequest(); err != nil {
		t.Fatal(err)
	}
	fields, err := hpack.NewDecoder(4096, nil).DecodeFull(block)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, h := range fields {
		if h.Name == "authorization" || h.Name == "cookie" {
			found++
			if !h.Sensitive {
				t.Fatalf("indexed secret %s", h.Name)
			}
		}
	}
	if found != 2 {
		t.Fatal("missing credentials")
	}
	cs.abort()
}

func TestH2CancellationClosesSilentReadLoop(t *testing.T) {
	for i := 0; i < 25; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		p := h2PoolFor(ctx, time.Minute)
		client, peer := net.Pipe()
		pc := &persistConn{conn: client, p: p}
		pc.h2Conn()
		c := pc.alt
		c.pc = nil
		go c.readLoop()
		cancel()
		select {
		case <-c.readLoopExited:
		case <-time.After(time.Second):
			t.Fatal("silent read loop leaked after context cancellation")
		}
		peer.Close()
		p.Clear()
		if c.idleTimer.Reset(time.Hour) {
			t.Fatal("closed connection retained idle timer")
		}
		c.idleTimer.Stop()
	}
}

func TestH2StalledWriteObservesCancellation(t *testing.T) {
	client, peer := net.Pipe()
	defer client.Close()
	defer peer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	c := &http2ClientConn{conn: client, writeCtx: ctx}
	done := make(chan error, 1)
	go func() { _, err := (&h2DeadlineWriter{conn: c}).Write([]byte("blocked")); done <- err }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stalled write succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled writer leaked")
	}
	// The cancellation callback must be joined before a following write clears it.
	c.writeCtx = context.Background()
	go func() { b := make([]byte, 2); _, err := io.ReadFull(peer, b); done <- err }()
	if _, err := (&h2DeadlineWriter{conn: c}).Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestH2StreamingHandlerReturnReleasesStream(t *testing.T) {
	s, _ := h2ResourceServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		io.Copy(w, strings.NewReader(strings.Repeat("x", 4<<20)))
	}))
	p := h2PoolFor(context.Background(), time.Minute)
	defer p.Clear()
	_, err := h2ResourceRequest(s, p, WithNoBodyBuffer(true), WithBodyStreamReaderHandler(func([]byte, io.ReadCloser) {}))
	var streamErr http2.StreamError
	if !errors.As(err, &streamErr) || streamErr.Code != http2.ErrCodeCancel {
		t.Fatalf("handler return did not cancel stream: %v", err)
	}
	p.h2Mu.Lock()
	defer p.h2Mu.Unlock()
	for _, pc := range p.h2ConnMap {
		pc.alt.mu.Lock()
		active, retained := pc.alt.activeStreams, len(pc.alt.streams)
		pc.alt.mu.Unlock()
		if active != 0 || retained != 0 {
			t.Fatal("handler return retained stream")
		}
	}
}

func TestH2InvalidServerPrefaceClosesReadLoop(t *testing.T) {
	for _, ack := range []bool{false, true} {
		client, peer := net.Pipe()
		p := h2PoolFor(context.Background(), time.Minute)
		pc := &persistConn{conn: client, p: p}
		pc.h2Conn()
		c := pc.alt
		c.pc = nil
		go c.readLoop()
		fr := http2.NewFramer(peer, nil)
		var err error
		if ack {
			err = fr.WriteSettingsAck()
		} else {
			err = fr.WritePing(false, [8]byte{})
		}
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-c.readLoopExited:
		case <-time.After(time.Second):
			c.setClose()
			t.Fatal("invalid server preface did not close connection")
		}
		c.mu.Lock()
		readErr := c.readErr
		c.mu.Unlock()
		var connErr http2.ConnectionError
		if !errors.As(readErr, &connErr) || http2.ErrCode(connErr) != http2.ErrCodeProtocol {
			t.Fatalf("wrong preface error: %v", readErr)
		}
		peer.Close()
		p.Clear()
	}
}

func TestH2LastStreamIDDrainsBeforeClosing(t *testing.T) {
	c := newH2ReadLoopTestConn(t, io.Discard)
	c.currentStreamID = (1 << 31) - 1
	req, _ := http.NewRequest("GET", "http://h2.test/", nil)
	cs, err := c.newStream(req, []byte("GET / HTTP/2\r\nHost: h2.test\r\n\r\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = cs.doRequest(); err != nil {
		t.Fatal(err)
	}
	if _, err = c.newStream(req, nil, nil); !errors.Is(err, CreateStreamAfterGoAwayErr) {
		t.Fatalf("stream ID exhaustion ignored: %v", err)
	}
	if c.isClosed() {
		t.Fatal("last active stream closed early")
	}
	rl := &http2ClientConnReadLoop{h2Conn: c}
	rl.applyResponseHeaders(cs, []hpack.HeaderField{{Name: ":status", Value: "200"}}, true)
	if _, _, err = cs.waitResponse(context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
	if !c.isClosed() {
		t.Fatal("exhausted connection retained after drain")
	}
}

func TestH2StreamingWindowCreditFollowsConsumption(t *testing.T) {
	var updates []http2.WindowUpdateFrame
	c := newH2ReadLoopTestConn(t, h2FrameCapture(func(f http2.Frame) {
		if w, ok := f.(*http2.WindowUpdateFrame); ok {
			updates = append(updates, *w)
		}
	}))
	c.receiveUpdateThreshold = 32 << 10
	cs := newH2ReadLoopTestStream(t, c, 1)
	cs.readHeaderEnd = true
	cs.resp.StatusCode = 200
	cs.noBodyBuffer = true
	reader, writer := newH2BodyPipe()
	cs.bodyStreamReader, cs.bodyStreamWriter = reader, writer
	rl := &http2ClientConnReadLoop{h2Conn: c}
	var wire bytes.Buffer
	fw := http2.NewFramer(&wire, nil)
	for i := 0; i < 2; i++ {
		if err := fw.WriteData(1, false, make([]byte, 16<<10)); err != nil {
			t.Fatal(err)
		}
	}
	fr := http2.NewFramer(io.Discard, &wire)
	for i := 0; i < 2; i++ {
		f, err := fr.ReadFrame()
		if err != nil {
			t.Fatal(err)
		}
		rl.processData(f.(*http2.DataFrame))
	}
	if len(updates) != 1 || updates[0].StreamID != 0 || updates[0].Increment != 32<<10 {
		t.Fatalf("unconsumed data returned stream credit: %+v", updates)
	}
	c.consumeStreamBytes(1, 16<<10)
	if len(updates) != 1 {
		t.Fatal("stream credit was not batched")
	}
	c.consumeStreamBytes(1, 16<<10)
	if len(updates) != 2 || updates[1].StreamID != 1 || updates[1].Increment != 32<<10 || cs.recvWindow != defaultStreamReceiveWindowSize {
		t.Fatalf("consumption lost credit: %+v", updates)
	}
	cs.abort()
	reader.Close()
}
