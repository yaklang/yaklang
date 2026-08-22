package lowhttp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/go-funk"
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

	mu              *sync.Mutex
	streams         map[uint32]*http2ClientStream
	currentStreamID uint32

	// Idle-timeout management
	idleTimeout time.Duration
	idleTimer   *time.Timer

	pingInterval time.Duration
	pingTimeout  time.Duration
	pingConfigMu sync.RWMutex
	pingSeq      int64 // atomic counter; generates unique PING data
	pingMu       sync.Mutex
	pendingPings map[[8]byte]chan struct{} // awaiting PING ACK responses

	// activeStreams counts in-flight streams (accessed under mu).
	// The idle timer only runs when activeStreams == 0.
	// streamsCond is signalled whenever a stream completes or the connection
	// closes, allowing goroutines blocked in newStream to re-check the limit.
	activeStreams int
	streamsCond   *sync.Cond // based on mu

	maxFrameSize      uint32
	initialWindowSize uint32
	maxStreamsCount   uint32
	headerListMaxSize uint32
	connWindowControl *windowSizeControl

	// chromeFingerprint 开启后，SETTINGS、WINDOW_UPDATE、伪头顺序与
	// HEADERS 帧标志均按 Chrome 的行为发送
	chromeFingerprint bool

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

	hDec *hpack.Decoder

	http2StreamPool *sync.Pool

	fr           *http2.Framer
	frWriteMutex *sync.Mutex
}

type http2ClientStream struct {
	ID     uint32
	h2Conn *http2ClientConn

	// stream control
	streamWindowControl *windowSizeControl

	req       *http.Request
	reqPacket []byte

	resp       *http.Response
	bodyBuffer *bytes.Buffer
	respPacket []byte

	// read hPack
	hPackByte *bytes.Buffer

	sentHeaders   bool
	sentEndStream bool // send END_STREAM flag

	readEndStream atomic.Bool // peer send END_STREAM flag or RST_STREAM flag
	readHeaderEnd bool

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
	_, err := h2Conn.conn.Write([]byte(http2.ClientPreface))
	if err != nil {
		return utils.Wrapf(err, "write h2 preface failed")
	}
	settings := []http2.Setting{
		{ID: http2.SettingInitialWindowSize, Val: defaultStreamReceiveWindowSize},
		{ID: http2.SettingMaxFrameSize, Val: defaultMaxFrameSize},
		{ID: http2.SettingMaxConcurrentStreams, Val: defaultMaxConcurrentStreamSize},
		{ID: http2.SettingMaxHeaderListSize, Val: defaultMaxHeaderListSize},
	}
	connWindowIncrease := defaultStreamReceiveWindowSize - 65535
	if h2Conn.chromeFingerprint {
		settings = []http2.Setting{
			{ID: http2.SettingHeaderTableSize, Val: defaultHeaderTableSize},
			{ID: http2.SettingEnablePush, Val: 0},
			{ID: http2.SettingInitialWindowSize, Val: chromeH2InitialWindowSize},
			{ID: http2.SettingMaxHeaderListSize, Val: chromeH2MaxHeaderListSize},
		}
		connWindowIncrease = chromeH2ConnWindowIncrement
	}

	h2Conn.frWriteMutex.Lock()
	err = h2Conn.fr.WriteSettings(settings...)
	if err != nil {
		h2Conn.frWriteMutex.Unlock()
		return utils.Wrapf(err, "write h2 setting failed")
	}
	// Increase connection-level flow control window from default 65535 to our desired size.
	// RFC 7540 Section 6.9.2: SETTINGS only affects stream-level windows.
	// Connection window must be increased via WINDOW_UPDATE.
	if connWindowIncrease > 0 {
		err = h2Conn.fr.WriteWindowUpdate(0, uint32(connWindowIncrease))
	}
	h2Conn.frWriteMutex.Unlock()
	if err != nil {
		return utils.Wrapf(err, "write h2 connection window update failed")
	}
	h2Conn.setPreface()
	return nil

	//prefaceFlag := make(chan struct{}, 1) // get preface ok
	//go func() {
	//	h2Conn.preFaceCond.L.Lock()
	//	for !h2Conn.prefaceOk {
	//		h2Conn.preFaceCond.Wait()
	//	}
	//	prefaceFlag <- struct{}{}
	//	h2Conn.preFaceCond.L.Unlock()
	//}()
	//
	//closeFlag := make(chan struct{}, 1) // get read frame err
	//go func() {
	//	h2Conn.closeCond.L.Lock()
	//	for !h2Conn.closed {
	//		h2Conn.closeCond.Wait()
	//	}
	//	closeFlag <- struct{}{}
	//	h2Conn.closeCond.L.Unlock()
	//}()
	//
	//select {
	//case <-closeFlag:
	//	return utils.Errorf("h2 preface read err")
	//case <-prefaceFlag:
	//	return nil
	//}
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
	if req != nil && req.Context() != nil {
		requestCtx = req.Context()
	}
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
		if h2Conn.closed || h2Conn.readGoAway {
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
	if h2Conn.closed || h2Conn.readGoAway {
		h2Conn.mu.Unlock()
		return nil, CreateStreamAfterGoAwayErr
	}
	// Reserve the slot before releasing the lock to prevent TOCTOU races.
	h2Conn.activeStreams++
	firstStream := h2Conn.activeStreams == 1
	h2Conn.mu.Unlock()

	if firstStream {
		h2Conn.idleTimer.Stop()
	}

	cs := h2Conn.http2StreamPool.Get().(*http2ClientStream)
	cs.h2Conn = h2Conn
	cs.ID = 0 // assigned later in doRequest under frWriteMutex to guarantee wire order
	cs.resp = new(http.Response)
	cs.resp.ProtoMajor = 2
	h2Conn.mu.Lock()
	initialWindowSize := h2Conn.initialWindowSize
	h2Conn.mu.Unlock()
	cs.streamWindowControl = newControl(int64(initialWindowSize))
	cs.bodyBuffer = new(bytes.Buffer)
	cs.hPackByte = new(bytes.Buffer)
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
			reader, writer := utils.NewBufPipe(nil)
			cs.bodyStreamReader = reader
			cs.bodyStreamWriter = writer
			cs.bodyStreamDone = make(chan struct{})
		}
	}

	return cs, nil
}

// get new stream id
func (h2Conn *http2ClientConn) getNewStreamID() uint32 {
	newStreamID := atomic.LoadUint32(&h2Conn.currentStreamID)
	atomic.AddUint32(&h2Conn.currentStreamID, 2)
	return newStreamID
}

// read frame loop
func (h2Conn *http2ClientConn) readLoop() {
	atomic.StoreInt32(&h2Conn.readLoopRunning, 1)
	defer func() {
		// Order matters:
		//  1. setClose() evicts the conn from h2ConnMap and triggers tombstone
		//     recording (async, waiting on readLoopExited).
		//  2. Clear readLoopRunning so the tombstone goroutine sees 0.
		//  3. Close readLoopExited to unblock the tombstone goroutine.
		h2Conn.setClose()
		atomic.StoreInt32(&h2Conn.readLoopRunning, 0)
		close(h2Conn.readLoopExited)
	}()
	h2Conn.idleTimer.Reset(h2Conn.idleTimeout)
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
			pingTimer.Reset(interval)
		}
		if err != nil {
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
		if !h2Conn.clientPrefaceOk.IsSet() {
			// readLoop may start before preface() sets clientPrefaceOk.
			// The first frame from server must be SETTINGS; process it and continue.
			if sf, ok := frame.(*http2.SettingsFrame); ok {
				rl.processSettings(sf)
				continue
			}
			reason := fmt.Sprintf("unexpected-frame-before-settings: %T", frame)
			h2Conn.setCloseReason(reason)
			log.Errorf("http2: Transport received non-SETTINGS frame before SETTINGS: %v", frame)
			return
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
		return utils.Error("h2 connection already closed")
	}

	cs.h2Conn.idleTimer.Reset(cs.h2Conn.idleTimeout) // new request reset timer
	fr := cs.h2Conn.fr
	if fr == nil {
		return utils.Error("http2 conn framer is nil")
	}

	var requestHeaders []hpack.HeaderField
	addH2Header := func(k, v string) {
		requestHeaders = append(requestHeaders, hpack.HeaderField{Name: k, Value: v})
	}

	isHttps := httpctx.GetRequestHTTPS(cs.req)
	schema := "https"
	if !isHttps {
		schema = "http"
	}

	addH2Header(":authority", "") // 占位

	var hPackBuf bytes.Buffer
	hPackEnc := hpack.NewEncoder(&hPackBuf)

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
	if cs.h2Conn.chromeFingerprint {
		requestHeaders = reorderChromePseudoHeaders(requestHeaders)
	}
	for _, h := range requestHeaders {
		hPackEnc.WriteField(h)
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
				param := http2.HeadersFrameParam{
					StreamID:      streamID,
					BlockFragment: chunk,
					EndStream:     endStream,
					EndHeaders:    endHeaders,
				}
				if cs.h2Conn.chromeFingerprint {
					param.Priority = http2.PriorityParam{
						StreamDep: 0,
						Exclusive: true,
						Weight:    chromeH2HeadersWeight - 1,
					}
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
	maxStreamsCount := cs.h2Conn.maxStreamsCount
	maxFrameSize := cs.h2Conn.maxFrameSize
	cs.h2Conn.mu.Unlock()
	if closed {
		cs.h2Conn.frWriteMutex.Unlock()
		return utils.Error("h2 connection closed during write")
	}
	if readGoAway {
		cs.h2Conn.frWriteMutex.Unlock()
		return CreateStreamAfterGoAwayErr
	}
	// Assign stream ID under frWriteMutex to guarantee wire-order matches ID order.
	// RFC 7540 Section 5.1.1: stream IDs must be strictly increasing on the wire.
	cs.ID = cs.h2Conn.getNewStreamID()
	cs.h2Conn.mu.Lock()
	cs.h2Conn.streams[cs.ID] = cs
	cs.h2Conn.mu.Unlock()
	if (cs.ID/2)+1 >= maxStreamsCount {
		cs.h2Conn.full = true
	}
	// activeStreams was already incremented in newStream when the slot was reserved.
	endStreamOnHeaders := cs.h2Conn.chromeFingerprint && len(body) == 0
	err := h2HeaderWriter(fr, cs.ID, endStreamOnHeaders, maxFrameSize, hPackBuf.Bytes())
	cs.h2Conn.frWriteMutex.Unlock()
	if err != nil {
		// Check if error is due to closed connection, which should trigger retry
		if strings.Contains(err.Error(), "use of closed connection") || strings.Contains(err.Error(), "broken pipe") {
			return CreateStreamAfterGoAwayErr // This will trigger retry logic
		}
		cs.h2Conn.setCloseReason(fmt.Sprintf("write-headers-err: %v", err))
		cs.h2Conn.setClose()
		return utils.Errorf("yak.h2 framer write headers failed: %s", err)
	}
	cs.sentHeaders = true
	if len(body) > 0 {
		maxFrame := int(maxFrameSize)
		if maxFrame <= 0 {
			maxFrame = defaultMaxFrameSize
		}
		chunks := funk.Chunk(body, maxFrame).([][]byte)
		for index, dataFrameBytes := range chunks {
			dataLen := len(dataFrameBytes)

			// control by window size
			cs.streamWindowControl.decreaseWindowSize(int64(dataLen))
			cs.h2Conn.connWindowControl.decreaseWindowSize(int64(dataLen))
			cs.h2Conn.frWriteMutex.Lock()
			dataFrameErr := fr.WriteData(cs.ID, index == len(chunks)-1, dataFrameBytes)
			cs.h2Conn.frWriteMutex.Unlock()
			if dataFrameErr != nil {
				return utils.Wrapf(dataFrameErr, "framer WriteData for stream{%v} failed", cs.ID)
			}
		}
	} else if !endStreamOnHeaders {
		//if !cs.sentEndStream {
		cs.h2Conn.frWriteMutex.Lock()
		dataFrameErr := fr.WriteData(cs.ID, true, []byte{})
		cs.h2Conn.frWriteMutex.Unlock()
		if dataFrameErr != nil {
			return utils.Wrapf(dataFrameErr, "framer WriteData for stream{%v} failed", cs.ID)
		}
		//}
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
	if ctx == nil {
		ctx = context.Background()
	}
	var err error
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
		if atomic.LoadInt32(&cs.h2Conn.serverPrefaceTimedOut) == 1 {
			// Not a transport hiccup: this origin negotiated h2 and then never
			// spoke it. Retrying rebuilds the same dead conn, so surface a
			// non-retryable error and let the caller fall back to HTTP/1.1.
			err = utils.Wrapf(errH2ServerPrefaceTimeout, "h2 stream-id %v never saw the server preface : %s", cs.ID, flow)
		} else {
			err = utils.Wrapf(errH2ConnClosed, "h2 stream-id %v wait response conn closed : %s", cs.ID, flow)
		}
	}
	cs.closeBodyStreamWriter()
	cs.waitBodyStreamHandler()

	cs.releaseSlot()

	cs.resp.Body = io.NopCloser(cs.bodyBuffer)
	cs.respPacket, _ = utils.DumpHTTPResponse(cs.resp, len(cs.bodyBuffer.Bytes()) > 0)
	resp := *cs.resp
	responsePacket := cs.respPacket
	cs.h2Conn.http2StreamPool.Put(cs) // gc
	return resp, responsePacket, err
}

// releaseSlot removes a stream from the connection without affecting other
// streams sharing the same HTTP/2 connection.
func (cs *http2ClientStream) releaseSlot() {
	cs.h2Conn.mu.Lock()
	if cs.ID > 0 {
		delete(cs.h2Conn.streams, cs.ID)
	}
	if cs.h2Conn.activeStreams > 0 {
		cs.h2Conn.activeStreams--
	}
	idleNow := cs.h2Conn.activeStreams <= 0
	cs.h2Conn.mu.Unlock()
	// Broadcast wakes any goroutines blocked in newStream waiting for a free slot.
	cs.h2Conn.streamsCond.Broadcast()
	if idleNow {
		cs.h2Conn.idleTimer.Reset(cs.h2Conn.idleTimeout)
	}
}

func (cs *http2ClientStream) abort() {
	if cs == nil || cs.h2Conn == nil {
		return
	}
	cs.resetStream(http2.ErrCodeCancel)
	cs.releaseSlot()
	cs.h2Conn.http2StreamPool.Put(cs)
}

func (cs *http2ClientStream) resetStream(code http2.ErrCode) {
	if cs == nil || cs.h2Conn == nil {
		return
	}
	if cs.ID > 0 && !cs.readEndStream.Load() {
		cs.h2Conn.frWriteMutex.Lock()
		writeErr := cs.h2Conn.fr.WriteRSTStream(cs.ID, code)
		cs.h2Conn.frWriteMutex.Unlock()
		if writeErr != nil && !errors.Is(writeErr, net.ErrClosed) {
			log.Debugf("h2 stream-id %v reset failed: %v", cs.ID, writeErr)
		}
	}
	cs.setEndStream()
}

func (cs *http2ClientStream) setEndStream() {
	cs.readEndStream.Store(true)
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
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if err := streamAliveCheck(cs, f.StreamID); err != nil {
		log.Errorf("h2 stream-id %v processHeaders error: %v", f.StreamID, err)
		return
	}

	cs.firstFrameCallbackOnce.Do(func() {
		cs.callbackLock.Lock()
		callback := cs.readFirstFrameCallback
		cs.callbackLock.Unlock()
		if callback != nil {
			callback()
		}
	})

	cs.hPackByte.Write(f.HeaderBlockFragment()) // 存入 hPack缓冲区

	if f.HeadersEnded() { // 当头部结束时才开始解析
		respInstance := cs.resp
		parsedHeaders, err := cs.h2Conn.hDec.DecodeFull(cs.hPackByte.Bytes())
		cs.hPackByte.Reset()
		if err != nil {
			log.Errorf("h2 stream-id %v hpack decode header frame failed: %v", f.StreamID, err)
			return
		}
		for _, h := range parsedHeaders {
			if h.IsPseudo() {
				if utils.AsciiEqualFold(h.Name, ":status") {
					respInstance.StatusCode, _ = strconv.Atoi(h.Value)
				}
				continue
			}
			respInstance.Header.Add(h.Name, h.Value)
		}

		if f.HeadersEnded() {
			cs.readHeaderEnd = true
			cs.handleHeadersDone()
		}

		if f.StreamEnded() {
			cs.setEndStream()
		}
		return
	}
	return
}

func (rl *http2ClientConnReadLoop) processContinuation(f *http2.ContinuationFrame) {
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if err := streamAliveCheck(cs, f.StreamID); err != nil {
		log.Errorf("h2 stream-id %v processContinuation error: %v", f.StreamID, err)
		return
	}

	if cs.readHeaderEnd {
		log.Errorf("http2: received HEADERS for HEADERS_ENDED stream %d", f.StreamID)
		return
	}

	cs.hPackByte.Write(f.HeaderBlockFragment()) // 存入 hPack缓冲区

	if f.HeadersEnded() { // 当头部结束时才开始解析
		respInstance := cs.resp
		parsedHeaders, err := cs.h2Conn.hDec.DecodeFull(cs.hPackByte.Bytes())
		cs.hPackByte.Reset()
		if err != nil {
			log.Errorf("h2 stream-id %v hpack decode header frame failed: %v", f.StreamID, err)
			return
		}
		for _, h := range parsedHeaders {
			if h.IsPseudo() {
				if utils.AsciiEqualFold(h.Name, ":status") {
					respInstance.Status = h.Value
				}
				continue
			}
			respInstance.Header.Add(h.Name, h.Value)
		}
		if f.HeadersEnded() {
			cs.readHeaderEnd = true
			cs.handleHeadersDone()
		}
		return
	}
	return
}

func (rl *http2ClientConnReadLoop) processData(f *http2.DataFrame) {
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if err := streamAliveCheck(cs, f.StreamID); err != nil {
		// Server sent DATA after END_STREAM — protocol violation, silently ignore.
		log.Warnf("h2 stream-id %v processData ignored: %v", f.StreamID, err)
		return
	}

	if !cs.readHeaderEnd {
		log.Errorf("http2: received DATA for has not HEADERS_ENDED stream %d", f.StreamID)
		return
	}

	fr := cs.h2Conn.fr

	// Process data before marking END_STREAM so the body bytes are captured
	// and WINDOW_UPDATE is sent even when END_STREAM is set on the same frame.
	if dataLen := len(f.Data()); dataLen > 0 {
		cs.h2Conn.frWriteMutex.Lock()
		err := fr.WriteWindowUpdate(0, uint32(dataLen))
		cs.h2Conn.frWriteMutex.Unlock()
		if err != nil {
			log.Errorf("h2 stream-id %v write window update(connect level) error: %v", f.StreamID, err)
			return
		}
		cs.h2Conn.frWriteMutex.Lock()
		err = fr.WriteWindowUpdate(f.StreamID, uint32(dataLen))
		cs.h2Conn.frWriteMutex.Unlock()
		if err != nil {
			log.Errorf("h2 server write window update(stream level) error: %v", err)
			return
		}
		if cs.bodyStreamWriter != nil {
			_, _ = cs.bodyStreamWriter.Write(f.Data())
		}
		if !cs.noBodyBuffer {
			cs.bodyBuffer.Write(f.Data())
		}
	}

	if f.StreamEnded() { // end stream flag, must come after body is written
		cs.setEndStream()
	}

	return
}

func (rl *http2ClientConnReadLoop) processSettings(f *http2.SettingsFrame) {
	if f.IsAck() {
		return
	}

	// The server's first SETTINGS frame is its connection preface: signal
	// conn setup that this h2 connection is actually alive.
	select {
	case rl.h2Conn.serverPrefaceCh <- struct{}{}:
	default:
	}

	f.ForeachSetting(func(setting http2.Setting) error {
		switch setting.ID {
		case http2.SettingMaxHeaderListSize:
			rl.h2Conn.mu.Lock()
			rl.h2Conn.headerListMaxSize = setting.Val
			rl.h2Conn.mu.Unlock()
		case http2.SettingMaxConcurrentStreams:
			rl.h2Conn.mu.Lock()
			rl.h2Conn.maxStreamsCount = setting.Val
			rl.h2Conn.mu.Unlock()
		case http2.SettingMaxFrameSize:
			if setting.Val >= 1<<14 && setting.Val <= 1<<24-1 {
				rl.h2Conn.mu.Lock()
				rl.h2Conn.maxFrameSize = setting.Val
				rl.h2Conn.mu.Unlock()
			}
		case http2.SettingInitialWindowSize:
			if setting.Val > 1<<31-1 {
				return nil
			}
			rl.h2Conn.mu.Lock()
			delta := int64(setting.Val) - int64(rl.h2Conn.initialWindowSize)
			rl.h2Conn.initialWindowSize = setting.Val
			for _, cs := range rl.h2Conn.streams {
				if cs.streamWindowControl != nil {
					cs.streamWindowControl.adjustWindowSize(delta)
				}
			}
			rl.h2Conn.mu.Unlock()
		case http2.SettingHeaderTableSize:
			rl.h2Conn.hDec.SetMaxDynamicTableSize(setting.Val)
		}
		return nil
	})

	rl.h2Conn.frWriteMutex.Lock()
	err := rl.h2Conn.fr.WriteSettingsAck()
	rl.h2Conn.frWriteMutex.Unlock()
	if err != nil {
		log.Errorf("h2 client write settings ack error: %v", err)
		return
	}
}

func (rl *http2ClientConnReadLoop) processWindowUpdate(f *http2.WindowUpdateFrame) {
	if f.StreamID == 0 {
		log.Debugf("h2(WINDOW_UPDATE<connect level>) server allow client to (inc) %v bytes", f.Increment)
		rl.h2Conn.connWindowControl.increaseWindowSize(int64(f.Increment))
		return
	}
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if err := streamAliveCheck(cs, f.StreamID); err != nil {
		log.Errorf("h2 stream-id %v processWindowUpdate error: %v", f.StreamID, err)
		return
	}
	cs.streamWindowControl.increaseWindowSize(int64(f.Increment))
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
	rl.h2Conn.frWriteMutex.Unlock()
	if err != nil {
		log.Errorf("h2 client write ping ack error: %v", err)
	}
}

func (rl *http2ClientConnReadLoop) processResetStream(f *http2.RSTStreamFrame) {
	log.Infof("h2 stream-id  %v closed: %v", f.StreamID, f.ErrCode.String())
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if cs == nil {
		log.Errorf("unknown stream id: %v", f.StreamID)
		return
	}
	cs.setEndStream()
	return
}

func (rl *http2ClientConnReadLoop) processGoAway(f *http2.GoAwayFrame) {
	flow := fmt.Sprintf("%v->%v", rl.h2Conn.conn.LocalAddr(), rl.h2Conn.conn.RemoteAddr())
	log.Infof("connection: %s is going away by %v, lastStreamID=%v", flow, f.ErrCode.String(), f.LastStreamID)

	reason := fmt.Sprintf("goaway: errCode=%s lastStreamID=%d", f.ErrCode.String(), f.LastStreamID)
	rl.h2Conn.setCloseReason(reason)

	// Set readGoAway under mu so newStream's wait loop and canUse checks are consistent.
	rl.h2Conn.mu.Lock()
	rl.h2Conn.readGoAway = true
	rl.h2Conn.lastStreamID = f.LastStreamID
	for id, cs := range rl.h2Conn.streams {
		if id > f.LastStreamID {
			cs.setEndStream()
		}
	}
	rl.h2Conn.mu.Unlock()
	// Wake any goroutines blocked in newStream so they see readGoAway == true.
	rl.h2Conn.streamsCond.Broadcast()
}

// healthCheck sends a PING frame to verify the connection is still alive.
// It is called by the ping timer in readLoop after pingInterval of silence.
// If the server does not ACK within pingTimeout, the connection is closed.
func (h2Conn *http2ClientConn) healthCheck() {
	if h2Conn.isClosed() {
		return
	}
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
