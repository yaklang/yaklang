package lowhttp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

var errH2ConnClosed = utils.Error("http2 client conn closed")

// errH2ServerPrefaceTimeout marks a connection torn down because the server
// never sent its SETTINGS frame. Unlike errH2ConnClosed it must NOT be
// retried: reconnecting only rebuilds the same dead h2 conn against the same
// origin. Callers should fall back to HTTP/1.1 instead.
var errH2ServerPrefaceTimeout = utils.Error("http2 server preface timeout")

// h2ServerPrefaceTimeout bounds how long an h2 connection may go without the
// server's SETTINGS frame (its connection preface, RFC 7540 Section 3.5). A
// healthy h2 server sends it immediately upon connection establishment, so this
// only bites endpoints that negotiate h2 in ALPN but then stall (tarpit) or
// kill the connection (fingerprinting WAF). On expiry the connection is closed
// and the request falls back to HTTP/1.1.
const h2ServerPrefaceTimeout = 3 * time.Second

type http2ClientConn struct {
	conn net.Conn
	ctx  context.Context

	// streamReadMu pins streams while frames are applied, so cancellation cannot
	// recycle an object still in use by the read loop. Never hold it during ReadFrame.
	streamReadMu sync.Mutex
	readErr      error // protected by mu; fatal connection-level protocol error

	mu              *sync.Mutex
	streams         map[uint32]*http2ClientStream
	currentStreamID uint32

	// Idle-timeout management
	idleTimeout time.Duration
	idleTimer   *time.Timer

	pingInterval time.Duration
	pingTimeout  time.Duration
	pingConfigMu sync.RWMutex
	pingInFlight atomic.Bool
	pingSeq      int64 // atomic counter; generates unique PING data
	pingMu       sync.Mutex
	pendingPings map[[8]byte]chan struct{} // awaiting PING ACK responses

	// activeStreams counts in-flight streams (accessed under mu).
	// The idle timer only runs when activeStreams == 0.
	// streamsCond is signalled whenever a stream completes or the connection
	// closes, allowing goroutines blocked in newStream to re-check the limit.
	activeStreams int
	streamsCond   *sync.Cond // based on mu

	// http2Profile, when set, makes this connection write Chrome-like framing
	// (SETTINGS, connection WINDOW_UPDATE, pseudo-header order, HEADERS
	// priority and END_STREAM placement). nil means the default framing, which
	// stays compatible with non-conforming servers.
	http2Profile *http2Profile

	maxFrameSize           uint32
	initialWindowSize      uint32
	maxStreamsCount        uint32
	headerListMaxSize      uint32
	sendWindow             int64  // peer connection credit; protected by mu
	connRecvPending        uint32 // protected by streamReadMu
	receiveUpdateThreshold uint32

	full         bool
	readGoAway   bool
	lastStreamID uint32

	closed          bool
	clientPrefaceOk *utils.AtomicBool
	closeCh         chan struct{}
	closeOnce       sync.Once

	// serverPrefaceCh is signalled when the server's first SETTINGS frame
	// (its connection preface, RFC 7540 Section 3.5) arrives. Used to detect
	// endpoints that negotiate h2 in ALPN but then never speak h2 (silent
	// tarpit) or kill the connection (fingerprinting WAF): watchServerPreface
	// closes such a connection and the request falls back to HTTP/1.1.
	serverPrefaceCh chan struct{}

	// serverPrefaceTimedOut records that watchServerPreface, not a peer or
	// transport event, closed this connection. It turns the resulting stream
	// failure into a non-retryable error, so the caller downgrades to
	// HTTP/1.1 instead of rebuilding the same dead h2 conn.
	//
	// Plain int32 rather than *utils.AtomicBool: this struct is also built as
	// a literal outside the pool, and a zero value must be safe to read there.
	serverPrefaceTimedOut int32

	// readLoopRunning is 1 while the readLoop goroutine is active, 0 after it exits.
	// Accessed atomically; used by the debug printer to show goroutine liveness.
	readLoopRunning int32

	// readLoopExited is closed by readLoop's defer after readLoopRunning is set
	// to 0.  Anything that needs to observe "readLoop has fully exited" (e.g.
	// the tombstone recorder) waits on this channel instead of polling the
	// atomic flag, avoiding a busy-wait race.
	readLoopExited chan struct{}

	// closeReason records a human-readable explanation for why this connection
	// was closed.  Written once (by whichever path triggers the close first)
	// and read by removeConn when building the tombstone.  Protected by closeOnce
	// semantics — the first writer wins and subsequent writes are ignored.
	closeReasonOnce sync.Once
	closeReason     string // set before setClose(); read in removeConn()

	// pc is the owning persistConn; used by setClose to evict the connection
	// from h2ConnMap when it transitions to closed.
	pc *persistConn

	hDec     *hpack.Decoder
	hEnc     *hpack.Encoder // protected by frWriteMutex
	hEncBuf  bytes.Buffer
	writeCtx context.Context // protected by frWriteMutex

	http2StreamPool *sync.Pool

	bw           *bufio.Writer
	fr           *http2.Framer
	frWriteMutex *sync.Mutex
}

type http2ClientStream struct {
	ID     uint32
	h2Conn *http2ClientConn

	// stream control
	sendWindow    int64  // peer stream credit; protected by h2Conn.mu
	recvWindow    int64  // protected by streamReadMu
	recvPending   uint32 // protected by streamReadMu
	streamErr     error  // protected by streamReadMu
	requestCtx    context.Context
	cancelRequest context.CancelFunc

	req       *http.Request
	reqPacket []byte

	resp       *http.Response
	bodyBuffer *bytes.Buffer
	respPacket []byte

	sentHeaders   bool
	sentEndStream bool // send END_STREAM flag

	readEndStream    atomic.Bool // peer send END_STREAM flag or RST_STREAM flag
	readHeaderEnd    bool
	contentLength    int64
	bodyReceived     int64
	interimResponses int

	readEndStreamSignal chan struct{}

	callbackLock           *sync.Mutex
	readFirstFrameCallback func()
	firstFrameCallbackOnce sync.Once // only read first frame callback once

	option              *LowhttpExecConfig
	bodyStreamReader    io.ReadCloser
	bodyStreamWriter    io.WriteCloser
	bodyStreamOnce      sync.Once
	bodyStreamCloseOnce sync.Once
	bodyStreamDone      chan struct{}
	headersHandled      bool
	noBodyBuffer        bool
}

func (s *http2ClientStream) SetReadFirstFrameCallback(callback func()) {
	s.callbackLock.Lock()
	defer s.callbackLock.Unlock()
	s.readFirstFrameCallback = callback
}

func (h2Conn *http2ClientConn) setPingConfig(interval, timeout time.Duration) {
	h2Conn.pingConfigMu.Lock()
	h2Conn.pingInterval = interval
	h2Conn.pingTimeout = timeout
	h2Conn.pingConfigMu.Unlock()
}

func (h2Conn *http2ClientConn) pingConfig() (time.Duration, time.Duration) {
	h2Conn.pingConfigMu.RLock()
	interval, timeout := h2Conn.pingInterval, h2Conn.pingTimeout
	h2Conn.pingConfigMu.RUnlock()
	return interval, timeout
}

func (s *http2ClientStream) handleHeadersDone() {
	if s.headersHandled {
		return
	}
	s.headersHandled = true

	headerRaw := s.buildResponseHeaderRaw()
	if s.option != nil && s.option.AutoDetectSSE {
		if IsSSEContentTypeHeader(headerRaw) {
			s.noBodyBuffer = true
			if s.req != nil {
				httpctx.SetNoBodyBuffer(s.req, true)
			}
		}
	}
	if reader, ok := s.bodyStreamReader.(*h2BodyReader); ok {
		conn, id := s.h2Conn, s.ID
		reader.p.mu.Lock()
		reader.p.onRead = func(n int) { conn.consumeStreamBytes(id, n) }
		reader.p.onClose = func() { conn.closeStreamReader(id) }
		reader.p.mu.Unlock()
	}
	s.startBodyStreamHandler(headerRaw)
}

func (s *http2ClientStream) buildResponseHeaderRaw() []byte {
	if s.resp == nil {
		return nil
	}
	proto := s.resp.Proto
	if proto == "" {
		proto = fmt.Sprintf("HTTP/%d.%d", s.resp.ProtoMajor, s.resp.ProtoMinor)
	}
	status := s.resp.Status
	if status == "" {
		code := s.resp.StatusCode
		if code <= 0 {
			code = http.StatusOK
		}
		status = fmt.Sprintf("%d %s", code, http.StatusText(code))
	}

	var buf bytes.Buffer
	buf.WriteString(proto)
	buf.WriteByte(' ')
	buf.WriteString(status)
	buf.WriteString("\r\n")
	for k, values := range s.resp.Header {
		for _, v := range values {
			buf.WriteString(k)
			buf.WriteString(": ")
			buf.WriteString(v)
			buf.WriteString("\r\n")
		}
	}
	buf.WriteString("\r\n")
	return buf.Bytes()
}

func (s *http2ClientStream) startBodyStreamHandler(headerRaw []byte) {
	if s.option == nil || s.option.BodyStreamReaderHandler == nil || s.bodyStreamReader == nil {
		return
	}
	headerCopy := append([]byte(nil), headerRaw...)
	reader := s.bodyStreamReader
	handler := s.option.BodyStreamReaderHandler
	s.bodyStreamOnce.Do(func() {
		if s.option.bodyStreamReaderHandled != nil {
			s.option.bodyStreamReaderHandled.Set()
		}
		// Capture the completion channel owned by this handler. The stream object
		// can be returned to sync.Pool and reused if a slow handler outlives the
		// bounded wait in waitBodyStreamHandler.
		done := s.bodyStreamDone
		go func(done chan struct{}) {
			defer reader.Close()
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("BodyStreamReaderHandler panic in http2: %v", r)
				}
				if done != nil {
					close(done)
				}
			}()
			handler(headerCopy, reader)
		}(done)
	})
}

func (s *http2ClientStream) closeBodyStreamWriter() {
	s.bodyStreamCloseOnce.Do(func() {
		if s.bodyStreamWriter != nil {
			_ = s.bodyStreamWriter.Close()
		}
	})
}

func (s *http2ClientStream) waitBodyStreamHandler() {
	if s.bodyStreamDone == nil || s.option == nil || s.option.bodyStreamReaderHandled == nil || !s.option.bodyStreamReaderHandled.IsSet() {
		return
	}
	select {
	case <-s.bodyStreamDone:
		return
	case <-time.After(2 * time.Second):
	}
	if s.bodyStreamReader != nil {
		_ = s.bodyStreamReader.Close()
	}
	select {
	case <-s.bodyStreamDone:
	case <-time.After(2 * time.Second):
		log.Warn("stream handler wait timeout: http2 stream handler")
	}
}

type http2ClientConnReadLoop struct {
	h2Conn *http2ClientConn

	// Header blocks belong to the connection, including when their stream is
	// canceled between HEADERS and CONTINUATION (RFC 9113 Sections 4.3 and 5.1).
	serverPreface      bool
	headerFields       []hpack.HeaderField
	headerBytes        uint64
	headerEncodedBytes int
	headerFrames       int
	headerTooLarge     bool
	headerStreamID     uint32
	headerEndStream    bool
}

// get stream by id
func (h2Conn *http2ClientConn) streamByID(id uint32) *http2ClientStream {
	h2Conn.mu.Lock()
	defer h2Conn.mu.Unlock()
	cs := h2Conn.streams[id]
	if cs != nil {
		return cs
	}
	return nil
}

func (h2Conn *http2ClientConn) preface() error {
	h2Conn.frWriteMutex.Lock()
	defer h2Conn.frWriteMutex.Unlock()
	var prefaceWriter io.Writer = &h2DeadlineWriter{conn: h2Conn}
	if h2Conn.bw != nil {
		prefaceWriter = h2Conn.bw
	}
	if _, err := prefaceWriter.Write([]byte(http2.ClientPreface)); err != nil {
		return err
	}
	settings := []http2.Setting{
		{ID: http2.SettingEnablePush, Val: 0},
		{ID: http2.SettingInitialWindowSize, Val: defaultStreamReceiveWindowSize},
		{ID: http2.SettingMaxFrameSize, Val: defaultMaxFrameSize},
		{ID: http2.SettingMaxConcurrentStreams, Val: defaultMaxConcurrentStreamSize},
		{ID: http2.SettingMaxHeaderListSize, Val: defaultMaxHeaderListSize},
	}
	// Increase connection-level flow control window from default 65535 to our desired size.
	// RFC 7540 Section 6.9.2: SETTINGS only affects stream-level windows.
	// Connection window must be increased via WINDOW_UPDATE.
	connWindowIncrease := int64(defaultStreamReceiveWindowSize) - 65535
	if profile := h2Conn.http2Profile; profile != nil {
		settings = profile.settings
		connWindowIncrease = int64(profile.connWindowUpdate)
	}
	err := h2Conn.fr.WriteSettings(settings...)
	if err != nil {
		return err
	}
	if connWindowIncrease > 0 {
		if err = h2Conn.fr.WriteWindowUpdate(0, uint32(connWindowIncrease)); err != nil {
			return err
		}
	}
	if err := h2Conn.flushFrames(); err != nil {
		return err
	}
	h2Conn.setPreface()
	return nil
}

// setCloseReason records the first (winning) reason this connection was closed.
// All subsequent callers are ignored so the tombstone always shows the root cause.
func (h2Conn *http2ClientConn) setCloseReason(reason string) {
	h2Conn.closeReasonOnce.Do(func() {
		h2Conn.closeReason = reason
	})
}

func (h2Conn *http2ClientConn) isClosed() bool {
	h2Conn.mu.Lock()
	closed := h2Conn.closed
	h2Conn.mu.Unlock()
	return closed
}

func (h2Conn *http2ClientConn) setClose() {
	// Mark closed while holding mu so newStream's wait loop sees it consistently.
	h2Conn.mu.Lock()
	h2Conn.closed = true
	if h2Conn.idleTimer != nil {
		h2Conn.idleTimer.Stop()
	}
	h2Conn.mu.Unlock()

	h2Conn.closeOnce.Do(func() {
		close(h2Conn.closeCh)
		// Evict this connection from the pool's h2ConnMap exactly once,
		// so the debug printer and getOrCreateH2Conn never see a CLOSED
		// entry lingering in the map.
		if h2Conn.pc != nil {
			h2Conn.pc.removeConn()
		}
	})
	// Wake all goroutines blocked in newStream waiting for a stream slot.
	h2Conn.streamsCond.Broadcast()
	h2Conn.conn.Close()
}

func (h2Conn *http2ClientConn) setPreface() {
	h2Conn.clientPrefaceOk.Set()
}

// watchServerPreface tears the connection down when the server never sends its
// SETTINGS frame (its connection preface, RFC 7540 Section 3.5). Endpoints that
// negotiate h2 in ALPN and then go silent — fingerprinting WAF tarpits — would
// otherwise hold requests until the caller's full timeout.
//
// The check deliberately runs in the background instead of gating connection
// setup: RFC 7540 Section 3.5 lets a client send requests immediately after its
// own preface, and middleboxes exist that withhold the server preface until the
// client's HEADERS arrive. Blocking setup on it would deadlock against those.
// Closing the conn makes in-flight requests fail fast, and the caller falls back
// to HTTP/1.1.
func (h2Conn *http2ClientConn) watchServerPreface() {
	go func() {
		select {
		case <-h2Conn.serverPrefaceCh:
		case <-h2Conn.closeCh:
		case <-time.After(h2ServerPrefaceTimeout):
			atomic.StoreInt32(&h2Conn.serverPrefaceTimedOut, 1)
			h2Conn.setCloseReason("server preface timeout")
			h2Conn.setClose()
		}
	}()
}

var CreateStreamAfterGoAwayErr = utils.Errorf("h2 conn can not create new stream, because read go away flag")

// newStream obtains an http2ClientStream from the pool and initialises it for
// the given request.  If the connection is already at SETTINGS_MAX_CONCURRENT_STREAMS,
// the call blocks until a slot becomes available or the connection is closed —
// the same behaviour as Go's net/http H2 transport.
func (h2Conn *http2ClientConn) newStream(req *http.Request, packet []byte, option *LowhttpExecConfig) (*http2ClientStream, error) {
	requestCtx := context.Background()
	if req != nil {
		requestCtx = req.Context()
	}
	cancelRequest := func() {}
	if option != nil && option.Timeout > 0 {
		requestCtx, cancelRequest = context.WithTimeout(requestCtx, option.Timeout)
	}
	reserved := false
	defer func() {
		if !reserved {
			cancelRequest()
		}
	}()
	stopCancelWake := context.AfterFunc(requestCtx, func() {
		h2Conn.mu.Lock()
		h2Conn.streamsCond.Broadcast()
		h2Conn.mu.Unlock()
	})
	defer stopCancelWake()

	// Wait for a concurrent-stream slot.  Access activeStreams and the
	// connection-state flags under mu so that streamsCond.Wait() is race-free.
	h2Conn.mu.Lock()
	for h2Conn.activeStreams >= int(h2Conn.maxStreamsCount) {
		if err := requestCtx.Err(); err != nil {
			h2Conn.mu.Unlock()
			return nil, err
		}
		if h2Conn.closed || h2Conn.readGoAway || h2Conn.full {
			h2Conn.mu.Unlock()
			return nil, CreateStreamAfterGoAwayErr
		}
		// Atomically releases mu and suspends goroutine.
		// Woken by streamsCond.Broadcast() in waitResponse / setClose / processGoAway.
		h2Conn.streamsCond.Wait()
	}
	if err := requestCtx.Err(); err != nil {
		h2Conn.mu.Unlock()
		return nil, err
	}
	if h2Conn.closed || h2Conn.readGoAway || h2Conn.full {
		h2Conn.mu.Unlock()
		return nil, CreateStreamAfterGoAwayErr
	}
	// Reserve the slot before releasing the lock to prevent TOCTOU races.
	h2Conn.activeStreams++
	reserved = true
	if h2Conn.idleTimer != nil {
		h2Conn.idleTimer.Stop()
	}
	h2Conn.mu.Unlock()

	if h2Conn.pc != nil {
		h2Conn.pc.p.markH2Active(h2Conn.pc)
	}
	cs := h2Conn.http2StreamPool.Get().(*http2ClientStream)
	// A stream returned by sync.Pool may contain state from its previous
	// request. Zero it before initialization so protocol flags and callbacks
	// can never bleed into the next stream.
	*cs = http2ClientStream{}
	cs.h2Conn = h2Conn
	cs.ID = 0 // assigned later in doRequest under frWriteMutex to guarantee wire order
	cs.resp = new(http.Response)
	cs.resp.ProtoMajor = 2
	cs.contentLength = -1
	cs.recvWindow = defaultStreamReceiveWindowSize
	h2Conn.mu.Lock()
	initialWindowSize := h2Conn.initialWindowSize
	h2Conn.mu.Unlock()
	cs.sendWindow = int64(initialWindowSize)
	cs.requestCtx, cs.cancelRequest = requestCtx, cancelRequest
	cs.bodyBuffer = new(bytes.Buffer)
	cs.sentHeaders = false
	cs.sentEndStream = false
	cs.readEndStream.Store(false)
	cs.readEndStreamSignal = make(chan struct{}, 1)
	cs.callbackLock = new(sync.Mutex)
	cs.firstFrameCallbackOnce = sync.Once{}
	cs.req = req
	cs.reqPacket = packet
	cs.resp.Header = make(http.Header) // init header
	cs.option = option
	cs.headersHandled = false
	cs.bodyStreamOnce = sync.Once{}
	cs.bodyStreamCloseOnce = sync.Once{}
	cs.bodyStreamDone = nil
	cs.bodyStreamReader = nil
	cs.bodyStreamWriter = nil
	cs.noBodyBuffer = false
	if option != nil {
		cs.noBodyBuffer = option.NoBodyBuffer
		if option.BodyStreamReaderHandler != nil {
			reader, writer := newH2BodyPipe()
			cs.bodyStreamReader = reader
			cs.bodyStreamWriter = writer
			cs.bodyStreamDone = make(chan struct{})
		}
	}

	return cs, nil
}

// get new stream id
func (h2Conn *http2ClientConn) getNewStreamID() uint32 {
	return atomic.AddUint32(&h2Conn.currentStreamID, 2) - 2
}

// read frame loop
func (h2Conn *http2ClientConn) readLoop() {
	atomic.StoreInt32(&h2Conn.readLoopRunning, 1)
	defer func() {
		h2Conn.hDec.SetEmitFunc(func(hpack.HeaderField) {})
		// Order matters:
		//  1. setClose() evicts the conn from h2ConnMap and triggers tombstone
		//     recording (async, waiting on readLoopExited).
		//  2. Clear readLoopRunning so the tombstone goroutine sees 0.
		//  3. Close readLoopExited to unblock the tombstone goroutine.
		h2Conn.setClose()
		atomic.StoreInt32(&h2Conn.readLoopRunning, 0)
		close(h2Conn.readLoopExited)
	}()
	stopContext := context.AfterFunc(h2Conn.ctx, h2Conn.setClose)
	defer stopContext()
	var rl = http2ClientConnReadLoop{h2Conn: h2Conn}

	// Ping-based health check: if no frame is received for pingInterval,
	// probe the server with a PING frame.  A missing ACK within pingTimeout
	// means the connection is dead and it is closed immediately.
	var pingTimer *time.Timer
	pingInterval, _ := h2Conn.pingConfig()
	if pingInterval > 0 {
		pingTimer = time.AfterFunc(pingInterval, h2Conn.healthCheck)
		defer pingTimer.Stop()
	}

	for !h2Conn.isClosed() {
		select {
		case <-h2Conn.ctx.Done():
			h2Conn.setCloseReason("ctx-cancelled")
			return
		default:
		}

		frame, err := h2Conn.fr.ReadFrame()
		// Any received frame proves the connection is still alive;
		// reset the ping timer so we only probe truly silent connections.
		if pingTimer != nil {
			interval, _ := h2Conn.pingConfig()
			if interval > 0 {
				pingTimer.Reset(interval)
			} else {
				pingTimer.Stop()
			}
		}
		if err != nil {
			var streamErr http2.StreamError
			if errors.As(err, &streamErr) {
				h2Conn.streamReadMu.Lock()
				if cs := h2Conn.streamByID(streamErr.StreamID); cs != nil && !cs.readEndStream.Load() {
					rl.failStream(cs, streamErr.Code, "invalid stream frame")
				}
				h2Conn.streamReadMu.Unlock()
				continue
			}
			var connErr http2.ConnectionError
			if errors.As(err, &connErr) {
				rl.failConnection(http2.ErrCode(connErr), "invalid connection frame")
				return
			}
			if errors.Is(err, http2.ErrFrameTooLarge) {
				rl.failConnection(http2.ErrCodeFrameSize, "frame exceeds advertised size")
				return
			}
			if errors.Is(err, io.EOF) {
				h2Conn.setCloseReason("remote-EOF")
				log.Infof("http2: conn %v readLoop: server closed connection (EOF)", h2Conn.conn.RemoteAddr())
			} else {
				reason := fmt.Sprintf("readFrame-err: %v", err)
				h2Conn.setCloseReason(reason)
				log.Infof("http2: conn %v readLoop: readFrame error: %v", h2Conn.conn.RemoteAddr(), err)
			}
			return
		}
		if !rl.serverPreface {
			sf, ok := frame.(*http2.SettingsFrame)
			if !ok || sf.IsAck() {
				rl.failConnection(http2.ErrCodeProtocol, "server preface must be non-ACK SETTINGS")
				return
			}
			rl.serverPreface = true
		}

		switch f := frame.(type) {
		case *http2.HeadersFrame:
			rl.processHeaders(f)
		case *http2.ContinuationFrame:
			rl.processContinuation(f)
		case *http2.DataFrame:
			rl.processData(f)
		case *http2.GoAwayFrame:
			rl.processGoAway(f)
		case *http2.RSTStreamFrame:
			rl.processResetStream(f)
		case *http2.SettingsFrame:
			rl.processSettings(f)
		case *http2.WindowUpdateFrame:
			rl.processWindowUpdate(f)
		case *http2.PingFrame:
			rl.processPing(f)
		case *http2.PushPromiseFrame:
			rl.failConnection(http2.ErrCodeProtocol, "server push is disabled")
		case *http2.PriorityFrame:
			// PRIORITY is advisory and does not change stream lifecycle.
		default:
			log.Warnf("Transport: unhandled response frame type %T", f)
		}
	}
}

// do request
func (cs *http2ClientStream) doRequest() error {
	// Check if h2Conn is nil to prevent panic
	if cs.h2Conn == nil {
		return utils.Error("h2 connection is nil")
	}

	// Check connection state before proceeding (use mu for consistency with newStream).
	cs.h2Conn.mu.Lock()
	closed := cs.h2Conn.closed
	cs.h2Conn.mu.Unlock()
	if closed {
		return CreateStreamAfterGoAwayErr // no request bytes have been written
	}

	fr := cs.h2Conn.fr
	if fr == nil {
		return utils.Error("http2 conn framer is nil")
	}

	var requestHeaders []hpack.HeaderField
	addH2Header := func(k, v string) {
		requestHeaders = append(requestHeaders, hpack.HeaderField{Name: k, Value: v,
			Sensitive: k == "authorization" || k == "proxy-authorization" || k == "cookie"})
	}

	isHttps := httpctx.GetRequestHTTPS(cs.req)
	schema := "https"
	if !isHttps {
		schema = "http"
	}

	addH2Header(":authority", "") // 占位

	methodReq := http.MethodGet
	_, body := SplitHTTPHeadersAndBodyFromPacketEx(cs.reqPacket, func(method string, requestUri string, proto string) error {
		if method != "" {
			methodReq = method
		}
		addH2Header(":method", methodReq)
		if !utils.AsciiEqualFold(method, "CONNECT") {
			addH2Header(":path", requestUri)
			addH2Header(":scheme", schema)
		}
		return nil
	}, func(line string) {
		result := strings.SplitN(line, ":", 2)
		if len(result) == 1 {
			addH2Header(strings.ToLower(result[0]), "")
		} else if len(result) == 2 {
			key := strings.ToLower(result[0])
			value := strings.TrimLeft(result[1], " ")
			switch key {
			case "host": // :authority
				for index, h := range requestHeaders {
					if h.Name == ":authority" {
						requestHeaders[index].Value = value
						break
					}
				}

			case "content-length", "connection", "proxy-connection", // todo cl问题是否处理
				"transfer-encoding", "upgrade",
				"keep-alive": // H2不应该存在的头
			default:
				addH2Header(key, value)
			}
		}
	})
	profile := cs.h2Conn.http2Profile
	if profile != nil {
		requestHeaders = profile.reorderPseudoHeaders(requestHeaders)
	}

	h2HeaderWriter := func(frame *http2.Framer, streamID uint32, endStream bool, maxFrameSize uint32, hdrs []byte) error {
		first := true // first frame written (HEADERS is first, then CONTINUATION)
		for len(hdrs) > 0 {
			chunk := hdrs
			if len(chunk) > int(maxFrameSize) {
				chunk = chunk[:maxFrameSize]
			}
			hdrs = hdrs[len(chunk):]
			endHeaders := len(hdrs) == 0
			if first {
				// Default framing omits END_STREAM here: some servers do not
				// accept it on HEADERS. A profile opts back into it, and RFC
				// 7540 6.2 keeps the flag on HEADERS even when CONTINUATION
				// frames follow.
				param := http2.HeadersFrameParam{
					StreamID:      streamID,
					BlockFragment: chunk,
					EndStream:     endStream,
					EndHeaders:    endHeaders,
				}
				if profile != nil && !profile.headersPriority.IsZero() {
					param.Priority = profile.headersPriority
				}
				err := frame.WriteHeaders(param)
				first = false
				if err != nil {
					return err
				}
			} else {
				err := frame.WriteContinuation(streamID, endHeaders, chunk)
				if err != nil {
					return err
				}
			}
		}
		cs.sentEndStream = endStream
		return nil
	}

	cs.h2Conn.frWriteMutex.Lock()
	// Double check connection state while holding write mutex
	cs.h2Conn.mu.Lock()
	closed = cs.h2Conn.closed
	readGoAway := cs.h2Conn.readGoAway
	maxFrameSize := cs.h2Conn.maxFrameSize
	if maxFrameSize > defaultMaxFrameSize {
		maxFrameSize = defaultMaxFrameSize
	}
	cs.h2Conn.mu.Unlock()
	if closed {
		cs.h2Conn.frWriteMutex.Unlock()
		return CreateStreamAfterGoAwayErr // still before stream registration or HEADERS
	}
	if readGoAway || atomic.LoadUint32(&cs.h2Conn.currentStreamID) > (1<<31)-1 {
		cs.h2Conn.frWriteMutex.Unlock()
		return CreateStreamAfterGoAwayErr
	}
	if err := cs.requestContextError(); err != nil {
		cs.h2Conn.frWriteMutex.Unlock()
		return err
	}
	c := cs.h2Conn
	if c.hEnc == nil {
		c.hEnc = hpack.NewEncoder(&c.hEncBuf)
	}
	c.mu.Lock()
	limit := c.headerListMaxSize
	c.mu.Unlock()
	var size uint64
	for _, h := range requestHeaders {
		size += uint64(len(h.Name)) + uint64(len(h.Value)) + 32
	}
	if size > uint64(limit) {
		c.frWriteMutex.Unlock()
		return fmt.Errorf("http2: request header list exceeds peer limit %d", limit)
	}
	c.hEncBuf.Reset()
	for _, h := range requestHeaders {
		if err := c.hEnc.WriteField(h); err != nil {
			c.frWriteMutex.Unlock()
			return err
		}
	}
	// Assign stream ID under frWriteMutex to guarantee wire-order matches ID order.
	// RFC 7540 Section 5.1.1: stream IDs must be strictly increasing on the wire.
	cs.ID = cs.h2Conn.getNewStreamID()
	cs.h2Conn.mu.Lock()
	// SETTINGS may have changed between newStream and registration.
	cs.sendWindow = int64(cs.h2Conn.initialWindowSize)
	cs.h2Conn.streams[cs.ID] = cs
	cs.h2Conn.full = cs.ID == (1<<31)-1
	cs.h2Conn.mu.Unlock()
	// activeStreams was already incremented in newStream when the slot was reserved.
	endStreamOnHeaders := profile != nil && profile.endStreamOnHeaders && len(body) == 0
	c.writeCtx = cs.requestCtx
	err := h2HeaderWriter(fr, cs.ID, endStreamOnHeaders, maxFrameSize, c.hEncBuf.Bytes())
	if err == nil && len(body) == 0 && !endStreamOnHeaders {
		err = fr.WriteData(cs.ID, true, nil)
		cs.sentEndStream = err == nil
	}
	if err == nil {
		err = c.flushFrames()
	}
	c.writeCtx = nil
	// Large one-off request fields should not pin an oversized encode buffer.
	if c.hEncBuf.Cap() > 64<<10 {
		c.hEncBuf = bytes.Buffer{}
	}
	cs.h2Conn.frWriteMutex.Unlock()
	if err != nil {
		cs.h2Conn.setCloseReason(fmt.Sprintf("write-headers-err: %v", err))
		cs.h2Conn.setClose()
		return fmt.Errorf("yak.h2 framer write headers failed: %w", err)
	}
	cs.sentHeaders = true
	ctx := cs.requestCtx
	if ctx == nil {
		ctx = context.Background()
	}
	conn := cs.h2Conn
	stopWake := context.AfterFunc(ctx, func() {
		conn.mu.Lock()
		conn.streamsCond.Broadcast()
		conn.mu.Unlock()
	})
	defer stopWake()
	for len(body) > 0 {
		n, err := cs.takeSendQuota(ctx, len(body))
		if err != nil {
			if errors.Is(err, errH2UploadAborted) {
				// The response can finish while our upload is flow-control blocked.
				// Close the local half too, so the peer can release the stream.
				cs.finishUpload()
			}
			return err
		}
		cs.h2Conn.frWriteMutex.Lock()
		cs.h2Conn.writeCtx = ctx
		err = fr.WriteData(cs.ID, n == len(body), body[:n])
		if err == nil {
			err = cs.h2Conn.flushFrames()
		}
		cs.h2Conn.writeCtx = nil
		cs.h2Conn.frWriteMutex.Unlock()
		if err != nil {
			cs.h2Conn.setClose()
			return err
		}
		body = body[n:]
		if len(body) == 0 {
			cs.sentEndStream = true
		}
	}
	if !cs.sentEndStream {
		// Preserve the established empty-DATA request framing for compatibility.
		cs.h2Conn.frWriteMutex.Lock()
		cs.h2Conn.writeCtx = ctx
		err = fr.WriteData(cs.ID, true, nil)
		if err == nil {
			err = cs.h2Conn.flushFrames()
		}
		cs.h2Conn.writeCtx = nil
		cs.h2Conn.frWriteMutex.Unlock()
		if err != nil {
			cs.h2Conn.setClose()
			return err
		}
	}
	cs.sentEndStream = true
	return nil
}

func (cs *http2ClientStream) waitResponse(ctx context.Context, timeout time.Duration) (http.Response, []byte, error) {
	// Check if h2Conn is nil to prevent panic
	if cs.h2Conn == nil {
		return http.Response{}, nil, utils.Error("h2 connection is nil")
	}
	if cs.h2Conn.conn == nil {
		return http.Response{}, nil, utils.Error("h2 underlying connection is nil")
	}

	flow := fmt.Sprintf("%v->%v", cs.h2Conn.conn.LocalAddr(), cs.h2Conn.conn.RemoteAddr())
	if cs.requestCtx != nil {
		ctx = cs.requestCtx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var err error
	connectionClosed := false
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		err = utils.Errorf("h2 stream-id %v wait response timeout : %s, maybe you can use HTTP/1.1 retry it", cs.ID, flow)
		cs.resetStream(http2.ErrCodeCancel)
	case <-ctx.Done():
		err = ctx.Err()
		cs.resetStream(http2.ErrCodeCancel)
	case <-cs.readEndStreamSignal:
	case <-cs.h2Conn.closeCh:
		connectionClosed = true
		cs.h2Conn.mu.Lock()
		readErr := cs.h2Conn.readErr
		cs.h2Conn.mu.Unlock()
		if readErr != nil {
			err = readErr
		} else if atomic.LoadInt32(&cs.h2Conn.serverPrefaceTimedOut) == 1 {
			// Not a transport hiccup: this origin negotiated h2 and then never
			// spoke it. Retrying rebuilds the same dead conn, so surface a
			// non-retryable error and let the caller fall back to HTTP/1.1.
			err = utils.Wrapf(errH2ServerPrefaceTimeout, "h2 stream-id %v never saw the server preface : %s", cs.ID, flow)
		} else {
			err = utils.Wrapf(errH2ConnClosed, "h2 stream-id %v wait response conn closed : %s", cs.ID, flow)
		}
	}
	// Wait for any frame handler using this stream before inspecting its response.
	// Mark it ended even when the connection closed before END_STREAM arrived.
	cs.h2Conn.streamReadMu.Lock()
	if connectionClosed && cs.readEndStream.Load() && cs.readHeaderEnd {
		// END_STREAM may race transport shutdown. A complete response wins.
		err = cs.streamErr
	}
	cs.setEndStream()
	if err == nil {
		err = cs.streamErr
	}
	cs.h2Conn.streamReadMu.Unlock()
	cs.waitBodyStreamHandler()

	cs.releaseSlot()

	cs.resp.Body = io.NopCloser(cs.bodyBuffer)
	cs.respPacket, _ = utils.DumpHTTPResponse(cs.resp, len(cs.bodyBuffer.Bytes()) > 0)
	resp := *cs.resp
	responsePacket := cs.respPacket
	cs.recycle()
	return resp, responsePacket, err
}

// recycle drops every request-owned reference before returning the stream to
// sync.Pool. Without the reset, an idle HTTP/2 connection can retain the most
// recent request, response, packet buffers, callbacks, and configuration until
// the pool entry is reused or a GC cycle discards it.
func (cs *http2ClientStream) recycle() {
	if cs == nil || cs.h2Conn == nil || cs.h2Conn.http2StreamPool == nil {
		return
	}
	pool := cs.h2Conn.http2StreamPool
	if cs.cancelRequest != nil {
		cs.cancelRequest()
	}
	*cs = http2ClientStream{}
	pool.Put(cs)
}

// releaseSlot removes a stream from the connection without affecting other
// streams sharing the same HTTP/2 connection.
func (cs *http2ClientStream) releaseSlot() {
	cs.h2Conn.streamReadMu.Lock()
	defer cs.h2Conn.streamReadMu.Unlock()
	cs.h2Conn.mu.Lock()
	if cs.ID > 0 {
		delete(cs.h2Conn.streams, cs.ID)
	}
	if cs.h2Conn.activeStreams > 0 {
		cs.h2Conn.activeStreams--
	}
	idleNow := cs.h2Conn.activeStreams == 0
	closeNow := idleNow && (cs.h2Conn.readGoAway || cs.h2Conn.full)
	if idleNow && !cs.h2Conn.closed && !closeNow && cs.h2Conn.idleTimer != nil {
		cs.h2Conn.idleTimer.Reset(cs.h2Conn.idleTimeout)
	}
	cs.h2Conn.streamsCond.Broadcast()
	cs.h2Conn.mu.Unlock()
	if closeNow {
		cs.h2Conn.setClose()
	} else if idleNow && cs.h2Conn.pc != nil {
		cs.h2Conn.pc.p.markH2Idle(cs.h2Conn.pc)
	}
}

func (cs *http2ClientStream) abort() {
	if cs == nil || cs.h2Conn == nil {
		return
	}
	cs.resetStream(http2.ErrCodeCancel)
	cs.releaseSlot()
	cs.recycle()
}

func (cs *http2ClientStream) resetStream(code http2.ErrCode) {
	if cs == nil || cs.h2Conn == nil {
		return
	}
	cs.h2Conn.streamReadMu.Lock()
	defer cs.h2Conn.streamReadMu.Unlock()
	if cs.ID > 0 && !cs.readEndStream.Load() {
		cs.h2Conn.frWriteMutex.Lock()
		resetCtx, cancelReset := context.WithTimeout(context.Background(), 100*time.Millisecond)
		cs.h2Conn.writeCtx = resetCtx
		writeErr := cs.h2Conn.fr.WriteRSTStream(cs.ID, code)
		if writeErr == nil {
			writeErr = cs.h2Conn.flushFrames()
		}
		cs.h2Conn.writeCtx = nil
		cancelReset()
		cs.h2Conn.frWriteMutex.Unlock()
		if writeErr != nil {
			cs.h2Conn.setClose()
		}
	}
	cs.setEndStream()
}

func (cs *http2ClientStream) setEndStream() {
	cs.readEndStream.Store(true)
	cs.h2Conn.mu.Lock()
	if cs.h2Conn.streamsCond != nil {
		cs.h2Conn.streamsCond.Broadcast()
	}
	cs.h2Conn.mu.Unlock()
	select {
	case cs.readEndStreamSignal <- struct{}{}:
	default:
	}
	cs.closeBodyStreamWriter()
}

func streamAliveCheck(cs *http2ClientStream, id uint32) error {
	if cs == nil {
		return utils.Errorf("unknown stream id: %v", id)
	}
	if cs.readEndStream.Load() {
		return utils.Errorf("http2: received DATA for END_STREAM stream %d", cs.ID)
	}
	return nil
}

func (rl *http2ClientConnReadLoop) processHeaders(f *http2.HeadersFrame) {
	rl.h2Conn.streamReadMu.Lock()
	defer rl.h2Conn.streamReadMu.Unlock()

	if rl.isIdleStream(f.StreamID) {
		rl.failConnection(http2.ErrCodeProtocol, "HEADERS on idle stream")
		return
	}
	rl.headerStreamID = f.StreamID
	rl.headerEndStream = f.StreamEnded()
	rl.headerFields = nil
	rl.headerBytes, rl.headerEncodedBytes, rl.headerFrames = 0, 0, 0
	rl.headerTooLarge = false
	dec := rl.h2Conn.hDec
	dec.SetMaxStringLength(defaultMaxHeaderListSize)
	dec.SetEmitEnabled(true)
	dec.SetEmitFunc(func(h hpack.HeaderField) {
		rl.headerBytes += uint64(len(h.Name)) + uint64(len(h.Value)) + 32
		if rl.headerBytes > defaultMaxHeaderListSize {
			rl.headerTooLarge = true
			rl.headerFields = nil
			dec.SetEmitEnabled(false)
			return
		}
		rl.headerFields = append(rl.headerFields, h)
	})
	if cs := rl.h2Conn.streamByID(f.StreamID); cs != nil && !cs.readEndStream.Load() {
		cs.firstFrameCallbackOnce.Do(func() {
			cs.callbackLock.Lock()
			callback := cs.readFirstFrameCallback
			cs.callbackLock.Unlock()
			if callback != nil {
				callback()
			}
		})
	}
	rl.processHeaderFragment(f.HeaderBlockFragment(), f.HeadersEnded())
}

func (rl *http2ClientConnReadLoop) processContinuation(f *http2.ContinuationFrame) {
	rl.h2Conn.streamReadMu.Lock()
	defer rl.h2Conn.streamReadMu.Unlock()
	// Framer validates that CONTINUATION follows HEADERS on the same stream.
	rl.processHeaderFragment(f.HeaderBlockFragment(), f.HeadersEnded())
}

// processHeaderFragment runs under streamReadMu, but keeps no stream pointer
// between frames. Decode even discarded blocks to advance the shared HPACK table.
func (rl *http2ClientConnReadLoop) processHeaderFragment(fragment []byte, endHeaders bool) {
	rl.headerFrames++
	rl.headerEncodedBytes += len(fragment)
	if rl.headerFrames > 1024 || rl.headerEncodedBytes > 2*defaultMaxHeaderListSize {
		rl.h2Conn.hDec.SetEmitFunc(func(hpack.HeaderField) {})
		rl.headerFields = nil
		rl.failConnection(http2.ErrCodeEnhanceYourCalm, "response header block too large")
		return
	}
	_, err := rl.h2Conn.hDec.Write(fragment)
	if err == nil && endHeaders {
		err = rl.h2Conn.hDec.Close()
	}
	if err != nil {
		rl.h2Conn.hDec.SetEmitFunc(func(hpack.HeaderField) {})
		rl.headerFields = nil
		rl.failConnection(http2.ErrCodeCompression, "HPACK decode failed: "+err.Error())
		return
	}
	if !endHeaders {
		return
	}
	fields := rl.headerFields
	rl.headerFields = nil
	rl.h2Conn.hDec.SetEmitFunc(func(hpack.HeaderField) {})
	cs := rl.h2Conn.streamByID(rl.headerStreamID)
	if cs == nil || cs.readEndStream.Load() {
		return
	}
	if rl.headerTooLarge {
		rl.failStream(cs, http2.ErrCodeEnhanceYourCalm, "response header list too large")
		return
	}
	rl.applyResponseHeaders(cs, fields, rl.headerEndStream)
}

func (rl *http2ClientConnReadLoop) processData(f *http2.DataFrame) {
	rl.h2Conn.streamReadMu.Lock()
	defer rl.h2Conn.streamReadMu.Unlock()

	// Every DATA payload consumes connection credit, including padding and
	// in-flight frames on canceled streams (RFC 9113 Sections 5.1 and 6.1).
	// Return it before looking up the stream so unrelated requests can progress.
	if rl.isIdleStream(f.StreamID) {
		rl.failConnection(http2.ErrCodeProtocol, "DATA on idle stream")
		return
	}
	rl.h2Conn.connRecvPending += f.Length
	if rl.h2Conn.connRecvPending > 0 && rl.h2Conn.connRecvPending >= rl.h2Conn.receiveUpdateThreshold {
		rl.h2Conn.frWriteMutex.Lock()
		err := rl.h2Conn.fr.WriteWindowUpdate(0, rl.h2Conn.connRecvPending)
		rl.h2Conn.connRecvPending = 0
		if err == nil {
			err = rl.h2Conn.flushFrames()
		}
		rl.h2Conn.frWriteMutex.Unlock()
		if err != nil {
			rl.h2Conn.setClose()
			log.Errorf("h2 stream-id %v write window update(connect level) error: %v", f.StreamID, err)
			return
		}
	}
	cs := rl.h2Conn.streamByID(f.StreamID)
	if cs == nil || cs.readEndStream.Load() {
		return
	}
	if !cs.readHeaderEnd {
		rl.failStream(cs, http2.ErrCodeProtocol, "DATA before final response headers")
		return
	}
	cs.recvWindow -= int64(f.Length)
	if cs.recvWindow < 0 {
		rl.failStream(cs, http2.ErrCodeFlowControl, "stream receive window exceeded")
		return
	}
	streamCredit := f.Length
	if cs.bodyStreamWriter != nil {
		streamCredit -= uint32(len(f.Data()))
	}
	cs.recvPending += streamCredit
	if !f.StreamEnded() && cs.recvPending > 0 && cs.recvPending >= rl.h2Conn.receiveUpdateThreshold {
		rl.h2Conn.frWriteMutex.Lock()
		err := rl.h2Conn.fr.WriteWindowUpdate(f.StreamID, cs.recvPending)
		cs.recvWindow += int64(cs.recvPending)
		cs.recvPending = 0
		if err == nil {
			err = rl.h2Conn.flushFrames()
		}
		rl.h2Conn.frWriteMutex.Unlock()
		if err != nil {
			rl.h2Conn.setClose()
			log.Errorf("h2 server write window update(stream level) error: %v", err)
			return
		}
	}
	if data := f.Data(); len(data) > 0 {
		cs.bodyReceived += int64(len(data))
		if cs.contentLength >= 0 && cs.bodyReceived > cs.contentLength || cs.resp.StatusCode == 204 || cs.resp.StatusCode == 304 || cs.req != nil && cs.req.Method == http.MethodHead {
			rl.failStream(cs, http2.ErrCodeProtocol, "response body exceeds declared length")
			return
		}
		if cs.option != nil && cs.option.EnableMaxContentLength && cs.option.MaxContentLength > 0 && cs.bodyReceived > int64(cs.option.MaxContentLength) {
			keep := len(data) - int(cs.bodyReceived-int64(cs.option.MaxContentLength))
			if keep > 0 {
				if !cs.noBodyBuffer {
					cs.bodyBuffer.Write(data[:keep])
				}
				if cs.bodyStreamWriter != nil {
					_, _ = cs.bodyStreamWriter.Write(data[:keep])
				}
			}
			if cs.req != nil {
				httpctx.SetResponseTooLarge(cs.req, true)
			}
			cs.setEndStream()
			rl.h2Conn.frWriteMutex.Lock()
			err := rl.h2Conn.fr.WriteRSTStream(cs.ID, http2.ErrCodeCancel)
			if err == nil {
				err = rl.h2Conn.flushFrames()
			}
			rl.h2Conn.frWriteMutex.Unlock()
			if err != nil {
				rl.h2Conn.setClose()
			}
			return
		}
		if cs.bodyStreamWriter != nil {
			if _, err := cs.bodyStreamWriter.Write(data); err != nil {
				rl.failStream(cs, http2.ErrCodeCancel, "response body consumer closed or exceeded receive window")
				return
			}
		}
		if !cs.noBodyBuffer {
			cs.bodyBuffer.Write(data)
		}
	}
	if f.StreamEnded() {
		rl.endResponse(cs)
	}
}

func (rl *http2ClientConnReadLoop) processSettings(f *http2.SettingsFrame) {
	if f.IsAck() {
		return
	}
	if err := f.ForeachSetting(func(s http2.Setting) error {
		if s.ID == http2.SettingEnablePush {
			return http2.ConnectionError(http2.ErrCodeProtocol)
		}
		return s.Valid()
	}); err != nil {
		code := http2.ErrCodeProtocol
		if e, ok := err.(http2.ConnectionError); ok {
			code = http2.ErrCode(e)
		}
		rl.failConnection(code, "invalid SETTINGS")
		return
	}
	c := rl.h2Conn
	c.frWriteMutex.Lock()
	c.mu.Lock()
	var overflow bool
	f.ForeachSetting(func(setting http2.Setting) error {
		switch setting.ID {
		case http2.SettingMaxHeaderListSize:
			c.headerListMaxSize = setting.Val
		case http2.SettingMaxConcurrentStreams:
			c.maxStreamsCount = setting.Val
		case http2.SettingMaxFrameSize:
			c.maxFrameSize = setting.Val
		case http2.SettingInitialWindowSize:
			delta := int64(setting.Val) - int64(c.initialWindowSize)
			c.initialWindowSize = setting.Val
			for _, cs := range c.streams {
				cs.sendWindow += delta
				overflow = overflow || cs.sendWindow > (1<<31)-1
			}
		case http2.SettingHeaderTableSize:
			// Peer settings limit our encoder, never our response decoder.
			if c.hEnc == nil {
				c.hEnc = hpack.NewEncoder(&c.hEncBuf)
			}
			size := setting.Val
			if size > 4096 {
				size = 4096
			} // bound local encoder memory
			c.hEnc.SetMaxDynamicTableSizeLimit(size)
			c.hEnc.SetMaxDynamicTableSize(size)
		}
		return nil
	})
	c.streamsCond.Broadcast()
	c.mu.Unlock()
	var err error
	if !overflow {
		err = c.fr.WriteSettingsAck()
		if err == nil {
			err = c.flushFrames()
		}
	}
	c.frWriteMutex.Unlock()
	if overflow {
		rl.failConnection(http2.ErrCodeFlowControl, "SETTINGS stream window overflow")
		return
	}
	if err != nil {
		c.setClose()
		return
	}
	select {
	case c.serverPrefaceCh <- struct{}{}:
	default:
	}
}

func (rl *http2ClientConnReadLoop) processWindowUpdate(f *http2.WindowUpdateFrame) {
	rl.h2Conn.streamReadMu.Lock()
	defer rl.h2Conn.streamReadMu.Unlock()
	if f.StreamID != 0 && rl.isIdleStream(f.StreamID) {
		rl.failConnection(http2.ErrCodeProtocol, "WINDOW_UPDATE on idle stream")
		return
	}
	if f.StreamID == 0 {
		log.Debugf("h2(WINDOW_UPDATE<connect level>) server allow client to (inc) %v bytes", f.Increment)
		rl.h2Conn.mu.Lock()
		if rl.h2Conn.sendWindow+int64(f.Increment) > (1<<31)-1 {
			rl.h2Conn.mu.Unlock()
			rl.failConnection(http2.ErrCodeFlowControl, "connection send window overflow")
			return
		}
		rl.h2Conn.sendWindow += int64(f.Increment)
		rl.h2Conn.streamsCond.Broadcast()
		rl.h2Conn.mu.Unlock()
		return
	}
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if err := streamAliveCheck(cs, f.StreamID); err != nil {
		log.Debugf("h2 stream-id %v processWindowUpdate ignored: %v", f.StreamID, err)
		return
	}
	rl.h2Conn.mu.Lock()
	if cs.sendWindow+int64(f.Increment) > (1<<31)-1 {
		rl.h2Conn.mu.Unlock()
		rl.failStream(cs, http2.ErrCodeFlowControl, "stream send window overflow")
		return
	}
	cs.sendWindow += int64(f.Increment)
	rl.h2Conn.streamsCond.Broadcast()
	rl.h2Conn.mu.Unlock()
	return
}

func (rl *http2ClientConnReadLoop) processPing(f *http2.PingFrame) {
	if f.IsAck() {
		// Server is acknowledging our PING; unblock the waiting sendPing call.
		rl.h2Conn.pingMu.Lock()
		if ch, ok := rl.h2Conn.pendingPings[f.Data]; ok {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
		rl.h2Conn.pingMu.Unlock()
		return
	}
	// Server-initiated PING — respond with ACK (RFC 7540 Section 6.7).
	rl.h2Conn.frWriteMutex.Lock()
	err := rl.h2Conn.fr.WritePing(true, f.Data)
	if err == nil {
		err = rl.h2Conn.flushFrames()
	}
	rl.h2Conn.frWriteMutex.Unlock()
	if err != nil {
		rl.h2Conn.setClose()
		log.Errorf("h2 client write ping ack error: %v", err)
	}
}

func (rl *http2ClientConnReadLoop) processResetStream(f *http2.RSTStreamFrame) {
	rl.h2Conn.streamReadMu.Lock()
	defer rl.h2Conn.streamReadMu.Unlock()
	if rl.isIdleStream(f.StreamID) {
		rl.failConnection(http2.ErrCodeProtocol, "RST_STREAM on idle stream")
		return
	}
	log.Debugf("h2 stream-id %v closed: %v", f.StreamID, f.ErrCode.String())
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if cs == nil || cs.readEndStream.Load() {
		log.Debugf("h2 RST_STREAM ignored for closed stream: %v", f.StreamID)
		return
	}
	cs.streamErr = http2.StreamError{StreamID: cs.ID, Code: f.ErrCode}
	cs.setEndStream()
	return
}

func (rl *http2ClientConnReadLoop) processGoAway(f *http2.GoAwayFrame) {
	rl.h2Conn.streamReadMu.Lock()
	defer rl.h2Conn.streamReadMu.Unlock()
	flow := fmt.Sprintf("%v->%v", rl.h2Conn.conn.LocalAddr(), rl.h2Conn.conn.RemoteAddr())
	log.Infof("connection: %s is going away by %v, lastStreamID=%v", flow, f.ErrCode.String(), f.LastStreamID)

	reason := fmt.Sprintf("goaway: errCode=%s lastStreamID=%d", f.ErrCode.String(), f.LastStreamID)
	rl.h2Conn.setCloseReason(reason)

	// Set readGoAway under mu so newStream's wait loop and canUse checks are consistent.
	rl.h2Conn.mu.Lock()
	if rl.h2Conn.readGoAway && f.LastStreamID > rl.h2Conn.lastStreamID {
		rl.h2Conn.mu.Unlock()
		rl.failConnection(http2.ErrCodeProtocol, "GOAWAY last stream ID increased")
		return
	}
	rl.h2Conn.readGoAway = true
	rl.h2Conn.lastStreamID = f.LastStreamID
	for id, cs := range rl.h2Conn.streams {
		if id > f.LastStreamID {
			cs.streamErr = http2.StreamError{StreamID: id, Code: http2.ErrCodeRefusedStream}
			cs.readEndStream.Store(true)
			select {
			case cs.readEndStreamSignal <- struct{}{}:
			default:
			}
			cs.closeBodyStreamWriter()
		}
	}
	rl.h2Conn.mu.Unlock()
	rl.h2Conn.retire()
}

// healthCheck sends a PING frame to verify the connection is still alive.
// It is called by the ping timer in readLoop after pingInterval of silence.
// If the server does not ACK within pingTimeout, the connection is closed.
func (h2Conn *http2ClientConn) healthCheck() {
	if h2Conn.isClosed() || !h2Conn.pingInFlight.CompareAndSwap(false, true) {
		return
	}
	defer h2Conn.pingInFlight.Store(false)
	log.Debugf("h2 conn %p: sending PING health-check to %v", h2Conn, h2Conn.conn.RemoteAddr())
	if err := h2Conn.sendPing(); err != nil {
		reason := fmt.Sprintf("ping-failed: %v", err)
		h2Conn.setCloseReason(reason)
		log.Infof("h2 conn %v: PING failed (%v), closing connection", h2Conn.conn.RemoteAddr(), err)
		h2Conn.setClose()
	}
}

// sendPing writes a PING frame and waits for the server's ACK.
// It returns nil on success, or an error if the ACK does not arrive within
// pingTimeout or the connection is closed in the meantime.
func (h2Conn *http2ClientConn) sendPing() error {
	if h2Conn.pendingPings == nil {
		return utils.Error("h2 conn: pendingPings not initialised")
	}

	// Build a unique 8-byte PING payload from a per-connection counter.
	seq := atomic.AddInt64(&h2Conn.pingSeq, 1)
	var data [8]byte
	binary.BigEndian.PutUint64(data[:], uint64(seq))

	ackCh := make(chan struct{}, 1)
	h2Conn.pingMu.Lock()
	h2Conn.pendingPings[data] = ackCh
	h2Conn.pingMu.Unlock()

	defer func() {
		h2Conn.pingMu.Lock()
		delete(h2Conn.pendingPings, data)
		h2Conn.pingMu.Unlock()
	}()

	h2Conn.frWriteMutex.Lock()
	err := h2Conn.fr.WritePing(false, data)
	if err == nil {
		err = h2Conn.flushFrames()
	}
	h2Conn.frWriteMutex.Unlock()
	if err != nil {
		return utils.Wrapf(err, "h2 conn: write PING failed")
	}

	_, timeout := h2Conn.pingConfig()
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ackCh:
		return nil
	case <-timer.C:
		return utils.Errorf("h2 conn: PING ACK timeout after %v", timeout)
	case <-h2Conn.closeCh:
		return utils.Error("h2 conn: connection closed while waiting for PING ACK")
	}
}
