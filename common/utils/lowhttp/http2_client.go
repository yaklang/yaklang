package lowhttp

import (
	"bytes"
	"context"
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

type http2ClientConn struct {
	conn net.Conn
	ctx  context.Context

	mu              *sync.Mutex
	streams         map[uint32]*http2ClientStream
	currentStreamID uint32

	idleTimeout time.Duration
	idleTimer   *time.Timer

	maxFrameSize      uint32
	initialWindowSize uint32
	maxStreamsCount   uint32
	headerListMaxSize uint32
	connWindowControl *windowSizeControl

	full         bool
	readGoAway   bool
	lastStreamID uint32

	closeCond       *sync.Cond
	closed          bool
	clientPrefaceOk *utils.AtomicBool

	hDec *hpack.Decoder

	http2StreamPool *sync.Pool

	fr           *http2.Framer
	frWriteMutex *sync.Mutex

	// 资源清理标记
	resourceCleaned *utils.AtomicBool
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

	readEndStream bool // peer send END_STREAM flag or RST_STREAM flag
	readHeaderEnd bool

	readEndStreamSignal chan struct{}

	callbackLock           *sync.Mutex
	readFirstFrameCallback func()
	firstFrameCallbackOnce sync.Once // only read first frame callback once
}

func (s *http2ClientStream) SetReadFirstFrameCallback(callback func()) {
	s.callbackLock.Lock()
	defer s.callbackLock.Unlock()
	s.readFirstFrameCallback = callback
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
	h2Conn.frWriteMutex.Lock()
	err = h2Conn.fr.WriteSettings([]http2.Setting{
		{ID: http2.SettingInitialWindowSize, Val: defaultStreamReceiveWindowSize},
		{ID: http2.SettingMaxFrameSize, Val: defaultMaxFrameSize},
		{ID: http2.SettingMaxConcurrentStreams, Val: defaultMaxConcurrentStreamSize},
		{ID: http2.SettingMaxHeaderListSize, Val: defaultMaxHeaderListSize},
	}...)
	h2Conn.frWriteMutex.Unlock()
	if err != nil {
		return utils.Wrapf(err, "write h2 setting failed")
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

func (h2Conn *http2ClientConn) setClose() {
	h2Conn.closeCond.L.Lock()
	if h2Conn.closed {
		h2Conn.closeCond.L.Unlock()
		return // 避免重复关闭
	}
	h2Conn.closed = true
	h2Conn.closeCond.L.Unlock()
	h2Conn.closeCond.Broadcast()

	// 清理资源
	h2Conn.cleanupResources()

	h2Conn.conn.Close()
}

// 清理HTTP/2连接相关资源
func (h2Conn *http2ClientConn) cleanupResources() {
	if h2Conn.resourceCleaned != nil && h2Conn.resourceCleaned.IsSet() {
		return // 已经清理过
	}

	// 停止空闲定时器
	if h2Conn.idleTimer != nil {
		h2Conn.idleTimer.Stop()
		h2Conn.idleTimer = nil
	}

	// 清理所有流
	h2Conn.mu.Lock()
	for streamID, stream := range h2Conn.streams {
		if stream != nil {
			stream.setEndStream()              // 结束流
			h2Conn.http2StreamPool.Put(stream) // 回收到池中
		}
		delete(h2Conn.streams, streamID)
	}
	h2Conn.mu.Unlock()

	// 重置hpack解码器以释放内部缓冲
	if h2Conn.hDec != nil {
		// hpack.Decoder没有公开的Reset方法，但可以重新创建
		h2Conn.hDec = hpack.NewDecoder(defaultHeaderTableSize, nil)
	}

	if h2Conn.resourceCleaned != nil {
		h2Conn.resourceCleaned.Set()
	}
}

func (h2Conn *http2ClientConn) setPreface() {
	h2Conn.clientPrefaceOk.Set()
}

var CreateStreamAfterGoAwayErr = utils.Errorf("h2 conn can not create new stream, because read go away flag")

// new stream
func (h2Conn *http2ClientConn) newStream(req *http.Request, packet []byte) (*http2ClientStream, error) {
	if h2Conn.readGoAway {
		return nil, CreateStreamAfterGoAwayErr
	}

	// 检查连接是否已关闭
	h2Conn.closeCond.L.Lock()
	if h2Conn.closed {
		h2Conn.closeCond.L.Unlock()
		return nil, errH2ConnClosed
	}
	h2Conn.closeCond.L.Unlock()

	newStreamID := h2Conn.getNewStreamID()
	cs := h2Conn.http2StreamPool.Get().(*http2ClientStream)
	// 重置流状态，防止污染
	*cs = http2ClientStream{}
	cs.h2Conn = h2Conn
	cs.ID = newStreamID
	cs.resp = new(http.Response)
	cs.resp.ProtoMajor = 2
	cs.streamWindowControl = newControl(int64(h2Conn.initialWindowSize))
	cs.bodyBuffer = new(bytes.Buffer)
	cs.hPackByte = new(bytes.Buffer)
	cs.sentHeaders = false
	cs.sentEndStream = false
	cs.readEndStream = false
	cs.readEndStreamSignal = make(chan struct{}, 1)
	cs.callbackLock = new(sync.Mutex)
	cs.firstFrameCallbackOnce = sync.Once{}
	cs.req = req
	cs.reqPacket = packet
	cs.resp.Header = make(http.Header) // init header

	h2Conn.mu.Lock()
	h2Conn.streams[newStreamID] = cs
	h2Conn.mu.Unlock()

	if (cs.ID/2)+1 >= h2Conn.maxStreamsCount {
		h2Conn.full = true
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
	defer func() {
		h2Conn.setClose()
	}()
	h2Conn.idleTimer.Reset(h2Conn.idleTimeout) // read new frame reset timer
	var rl = http2ClientConnReadLoop{h2Conn: h2Conn}
	//var gotSettings = false
	var readIdleTimeout = h2Conn.idleTimeout
	var t *time.Timer

	for !h2Conn.closed {
		select {
		case <-h2Conn.ctx.Done():
			return
		default:
		}

		frame, err := h2Conn.fr.ReadFrame()
		if t != nil {
			t.Reset(readIdleTimeout)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Debugf("http2: Transport readFrame error on conn %p: %v", rl.h2Conn.conn, err)
			}
			return
		}
		if !h2Conn.clientPrefaceOk.IsSet() { // check start read frame after preface
			if _, ok := frame.(*http2.SettingsFrame); !ok {
				log.Errorf("http2: Transport received non-SETTINGS frame before SETTINGS: %v", frame)
			}
			return
		}

		// log.Infof("h2 stream-id %v found frame: %v", frame.Header().StreamID, frame)

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

	// Check connection state before proceeding
	cs.h2Conn.closeCond.L.Lock()
	if cs.h2Conn.closed {
		cs.h2Conn.closeCond.L.Unlock()
		return utils.Error("h2 connection already closed")
	}
	cs.h2Conn.closeCond.L.Unlock()

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
				//endStream = endStream && endHeaders
				err := frame.WriteHeaders(http2.HeadersFrameParam{ // some server not accept endStream flag in headers frame
					StreamID:      streamID,
					BlockFragment: chunk,
					//EndStream:     endStream,
					EndHeaders: endHeaders,
				})
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
	if cs.h2Conn.closed {
		cs.h2Conn.frWriteMutex.Unlock()
		return utils.Error("h2 connection closed during write")
	}
	err := h2HeaderWriter(fr, cs.ID, false, cs.h2Conn.maxFrameSize, hPackBuf.Bytes())
	cs.h2Conn.frWriteMutex.Unlock()
	if err != nil {
		// Check if error is due to closed connection, which should trigger retry
		if strings.Contains(err.Error(), "use of closed connection") || strings.Contains(err.Error(), "broken pipe") {
			return CreateStreamAfterGoAwayErr // This will trigger retry logic
		}
		cs.h2Conn.setClose()
		return utils.Errorf("yak.h2 framer write headers failed: %s", err)
	}
	cs.sentHeaders = true
	if len(body) > 0 {
		chunks := funk.Chunk(body, defaultMaxFrameSize).([][]byte)
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
	} else {
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

func (cs *http2ClientStream) waitResponse(timeout time.Duration) (http.Response, []byte, error) {
	// Check if h2Conn is nil to prevent panic
	if cs.h2Conn == nil {
		return http.Response{}, nil, utils.Error("h2 connection is nil")
	}
	if cs.h2Conn.conn == nil {
		return http.Response{}, nil, utils.Error("h2 underlying connection is nil")
	}

	flow := fmt.Sprintf("%v->%v", cs.h2Conn.conn.LocalAddr(), cs.h2Conn.conn.RemoteAddr())
	closeFlag := make(chan struct{}, 10) // get read frame err
	go func() {
		cs.h2Conn.closeCond.L.Lock()
		for !cs.h2Conn.closed {
			cs.h2Conn.closeCond.Wait()
		}
		closeFlag <- struct{}{}
		cs.h2Conn.closeCond.L.Unlock()
	}()

	var err error
	select {
	case <-time.After(timeout):
		err = utils.Errorf("h2 stream-id %v wait response timeout : %s, maybe you can use HTTP/1.1 retry it", cs.ID, flow)
	case <-cs.readEndStreamSignal:
	case <-closeFlag:
		err = utils.Wrapf(errH2ConnClosed, "h2 stream-id %v wait response conn closed : %s", cs.ID, flow)
	}
	cs.resp.Body = io.NopCloser(cs.bodyBuffer)
	cs.respPacket, _ = utils.DumpHTTPResponse(cs.resp, len(cs.bodyBuffer.Bytes()) > 0)
	cs.h2Conn.http2StreamPool.Put(cs) // gc
	return *cs.resp, cs.respPacket, err
}

func (cs *http2ClientStream) setEndStream() {
	cs.readEndStream = true
	cs.readEndStreamSignal <- struct{}{}
}

func streamAliveCheck(cs *http2ClientStream, id uint32) error {
	if cs == nil {
		return utils.Errorf("unknown stream id: %v", id)
	}
	if cs.readEndStream {
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
		if cs.readFirstFrameCallback != nil {
			cs.callbackLock.Lock()
			defer cs.callbackLock.Unlock()
			cs.readFirstFrameCallback()
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
		}
		return
	}
	return
}

func (rl *http2ClientConnReadLoop) processData(f *http2.DataFrame) {
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if err := streamAliveCheck(cs, f.StreamID); err != nil {
		log.Errorf("h2 stream-id %v processData error: %v", f.StreamID, err)
		return
	}

	if !cs.readHeaderEnd {
		log.Errorf("http2: received DATA for has not HEADERS_ENDED stream %d", f.StreamID)
		return
	}

	fr := cs.h2Conn.fr

	if f.StreamEnded() { // end stream flag
		cs.setEndStream()
	}
	if dataLen := len(f.Data()); dataLen > 0 {
		if !cs.readEndStream {
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
		}
		cs.bodyBuffer.Write(f.Data())
	}

	return
}

func (rl *http2ClientConnReadLoop) processSettings(f *http2.SettingsFrame) {
	if f.IsAck() {
		return
	}

	f.ForeachSetting(func(setting http2.Setting) error {
		// log.Infof("h2 stream found server setting: %v", setting.String())
		switch setting.ID {
		case http2.SettingMaxHeaderListSize:
			rl.h2Conn.headerListMaxSize = setting.Val
		}
		return nil
	})
	// write settings ack
	rl.h2Conn.frWriteMutex.Lock()
	err := rl.h2Conn.fr.WriteSettingsAck()
	rl.h2Conn.frWriteMutex.Unlock()
	if err != nil {
		log.Errorf("h2 client write settings ack error: %v", err)
		return
	}
	return
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
	rl.h2Conn.frWriteMutex.Lock()
	err := rl.h2Conn.fr.WritePing(true, f.Data)
	rl.h2Conn.frWriteMutex.Unlock()
	if err != nil {
		log.Errorf("h2 server write ping ack error: %v", err)
	}
	return
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
	log.Infof("connection: %s is going away by %v", flow, f.ErrCode.String())
	log.Infof("flow: %v last stream id: %v", flow, f.LastStreamID)
	rl.h2Conn.readGoAway = true
	rl.h2Conn.lastStreamID = f.LastStreamID
	return
}
