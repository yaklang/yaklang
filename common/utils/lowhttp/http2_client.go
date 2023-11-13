package lowhttp

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"io"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type http2ClientConn struct {
	conn net.Conn

	mu              *sync.Mutex
	streams         map[uint32]*http2ClientStream
	currentStreamID uint32

	idleTimeout time.Duration
	idleTimer   *time.Timer

	maxFrameSize      uint32
	initialWindowSize uint32
	maxStreamsCount   uint32
	connWindowControl *windowSizeControl

	readGoAway bool
	closed     bool
	full       bool

	br *bufio.Reader
	fr *http2.Framer
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

	//read hPack
	hPackDec  *hpack.Decoder
	hPackByte *bytes.Buffer

	sentHeaders   bool
	sentEndStream bool //send END_STREAM flag

	readEndStream bool // peer send END_STREAM flag or RST_STREAM flag
	readHeaderEnd bool
}

type http2ClientConnReadLoop struct {
	h2Conn http2ClientConn
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
		return utils.Wrapf(err, "h2 preface failed")
	}
	return nil
}

func (h2Conn *http2ClientConn) setting() error {
	err := h2Conn.fr.WriteSettings([]http2.Setting{
		{ID: http2.SettingInitialWindowSize, Val: defaultStreamReceiveWindowSize},
		{ID: http2.SettingMaxFrameSize, Val: defaultMaxFrameSize},
		{ID: http2.SettingMaxConcurrentStreams, Val: defaultMaxConcurrentStreamSize},
		{ID: http2.SettingMaxHeaderListSize, Val: defaultMaxHeaderListSize},
	}...)
	if err != nil {
		return utils.Wrapf(err, "yak.h2 write setting failed")
	}
	return nil
}

// new stream
func (h2Conn *http2ClientConn) newStream(req *http.Request, packet []byte) *http2ClientStream {
	if h2Conn.readGoAway {
		log.Error("h2 conn can not create new stream, because read go away flag")
		return nil
	}
	newStreamID := h2Conn.getNewStreamID()
	cs := &http2ClientStream{
		h2Conn:              h2Conn,
		ID:                  newStreamID,
		resp:                new(http.Response),
		streamWindowControl: newControl(int64(h2Conn.initialWindowSize)),
		bodyBuffer:          new(bytes.Buffer),
		hPackDec:            hpack.NewDecoder(defaultHeaderTableSize, nil),
		hPackByte:           new(bytes.Buffer),
		sentHeaders:         false,
		sentEndStream:       false,
		readEndStream:       false,
		req:                 req,
		reqPacket:           packet,
	}

	cs.resp.Header = make(http.Header) // init header

	h2Conn.mu.Lock()
	h2Conn.streams[newStreamID] = cs
	h2Conn.mu.Unlock()

	if (cs.ID/2)+1 >= h2Conn.maxStreamsCount {
		h2Conn.full = true
	}
	return cs
}

// get new stream id
func (h2Conn *http2ClientConn) getNewStreamID() uint32 {
	newStreamID := atomic.LoadUint32(&h2Conn.currentStreamID)
	atomic.AddUint32(&h2Conn.currentStreamID, 2)
	return newStreamID
}

// read frame loop
func (h2Conn *http2ClientConn) readLoop() {
	h2Conn.idleTimer.Reset(h2Conn.idleTimeout) // read new frame reset timer
	rl := http2ClientConnReadLoop{h2Conn: *h2Conn}
	gotSettings := false
	readIdleTimeout := h2Conn.idleTimeout
	var t *time.Timer

	for {
		f, err := h2Conn.fr.ReadFrame()
		if t != nil {
			t.Reset(readIdleTimeout)
		}
		if err != nil {
			log.Errorf("http2: Transport readFrame error on conn %p: (%T) %v", rl.h2Conn.conn, err, err)
			return
		}
		if !gotSettings {
			if _, ok := f.(*http2.SettingsFrame); !ok {
				log.Errorf("protocol error: received %T before a SETTINGS frame", f)
				return
			}
			gotSettings = true
		}

		log.Infof("h2 stream-id %v found frame: %v", f.Header().StreamID, reflect.TypeOf(f).String())

		switch f := f.(type) {
		case *http2.HeadersFrame:
			err = rl.processHeaders(f)
		case *http2.ContinuationFrame:
			err = rl.processContinuation(f)
		case *http2.DataFrame:
			err = rl.processData(f)
		case *http2.GoAwayFrame:
			err = rl.processGoAway(f)
		case *http2.RSTStreamFrame:
			err = rl.processResetStream(f)
		case *http2.SettingsFrame:
			err = rl.processSettings(f)
		case *http2.WindowUpdateFrame:
			err = rl.processWindowUpdate(f)
		case *http2.PingFrame:
			err = rl.processPing(f)
		default:
			log.Warnf("Transport: unhandled response frame type %T", f)
		}

	}
}

// do request
func (cs *http2ClientStream) doRequest() error {
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
		schema = "https"
	}

	if connectedPort := httpctx.GetContextIntInfoFromRequest(cs.req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort); connectedPort > 0 {
		portValid := (connectedPort == 443 && isHttps) || (connectedPort == 80 && !isHttps)
		if !portValid {
			if host := httpctx.GetContextStringInfoFromRequest(cs.req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost); host != "" {
				addH2Header(":authority", utils.HostPort(host, portValid))
			}
		}
	}

	var hPackBuf bytes.Buffer
	hPackEnc := hpack.NewEncoder(&hPackBuf)

	var methodReq = http.MethodGet
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
				addH2Header(":authority", value)

			case "content-length", "connection", "proxy-connection", //todo cl问题是否处理
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

	endRequestStream := len(body) <= 0
	err := h2FramerWriteHeaders(cs.h2Conn.fr, cs.ID, endRequestStream, cs.h2Conn.maxFrameSize, hPackBuf.Bytes())
	if err != nil {
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

			dataFrameErr := fr.WriteData(cs.ID, index == len(chunks)-1, dataFrameBytes)
			if dataFrameErr != nil {
				return utils.Wrapf(dataFrameErr, "framer WriteData for stream{%v} failed", cs.ID)
			}
		}
	}
	cs.sentEndStream = true
	return nil
}

func (cs *http2ClientStream) waitResponse() (http.Response, []byte) {
	for !cs.readEndStream {
		time.Sleep(time.Millisecond * 200)
	}
	cs.resp.Body = io.NopCloser(cs.bodyBuffer)
	cs.respPacket, _ = utils.DumpHTTPResponse(cs.resp, len(cs.bodyBuffer.Bytes()) > 0)
	return *cs.resp, cs.respPacket
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

func (rl *http2ClientConnReadLoop) processHeaders(f *http2.HeadersFrame) error {
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if err := streamAliveCheck(cs, f.StreamID); err != nil {
		return err
	}

	if cs.readHeaderEnd {
		return utils.Errorf("http2: received HEADERS for HEADERS_ENDED stream %d", f.StreamID)
	}

	cs.hPackByte.Write(f.HeaderBlockFragment()) //存入 hPack缓冲区

	if f.HeadersEnded() { //当头部结束时才开始解析
		var respInstance = cs.resp
		parsedHeaders, err := cs.hPackDec.DecodeFull(cs.hPackByte.Bytes())
		if err != nil {
			return utils.Wrapf(err, "hpack decode header frame failed")
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
			cs.readEndStream = true
		}
		return nil
	}
	return nil
}

func (rl *http2ClientConnReadLoop) processContinuation(f *http2.ContinuationFrame) error {
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if err := streamAliveCheck(cs, f.StreamID); err != nil {
		return err
	}

	if cs.readHeaderEnd {
		return utils.Errorf("http2: received HEADERS for HEADERS_ENDED stream %d", f.StreamID)
	}

	cs.hPackByte.Write(f.HeaderBlockFragment()) //存入 hPack缓冲区

	if f.HeadersEnded() { //当头部结束时才开始解析
		var respInstance = cs.resp
		parsedHeaders, err := cs.hPackDec.DecodeFull(cs.hPackByte.Bytes())
		if err != nil {
			return utils.Wrapf(err, "hpack decode header frame failed")
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
		return nil
	}
	return nil
}

func (rl *http2ClientConnReadLoop) processData(f *http2.DataFrame) error {
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if err := streamAliveCheck(cs, f.StreamID); err != nil {
		return err
	}

	if !cs.readHeaderEnd {
		return utils.Errorf("http2: received DATA for has not HEADERS_ENDED stream %d", f.StreamID)
	}

	fr := cs.h2Conn.fr

	if dataLen := len(f.Data()); dataLen > 0 {
		err := fr.WriteWindowUpdate(0, uint32(dataLen))
		if err != nil {
			return utils.Errorf("h2 server write window update(connect level) error: %v", err)
		}
		err = fr.WriteWindowUpdate(f.StreamID, uint32(dataLen))
		if err != nil {
			return utils.Errorf("h2 server write window update(stream level) error: %v", err)
		}
		cs.bodyBuffer.Write(f.Data())
	}

	if f.StreamEnded() { // end stream flag
		cs.readEndStream = true
	}
	return nil
}

func (rl *http2ClientConnReadLoop) processSettings(f *http2.SettingsFrame) error {
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if err := streamAliveCheck(cs, f.StreamID); err != nil {
		return err
	}
	if f.IsAck() {
		return nil
	}

	f.ForeachSetting(func(setting http2.Setting) error {
		log.Infof("h2 stream found server setting: %v", setting.String())
		switch setting.ID {
		case http2.SettingMaxFrameSize:
			rl.h2Conn.maxFrameSize = setting.Val
		case http2.SettingInitialWindowSize:
			rl.h2Conn.initialWindowSize = setting.Val
		case http2.SettingMaxConcurrentStreams:
			rl.h2Conn.maxStreamsCount = setting.Val
		}
		return nil
	})
	// write settings ack
	err := cs.h2Conn.fr.WriteSettingsAck()
	if err != nil {
		return utils.Wrapf(err, "h2 client write settings ack error")
	}
	return nil
}

func (rl *http2ClientConnReadLoop) processWindowUpdate(f *http2.WindowUpdateFrame) error {
	if f.StreamID == 0 {
		log.Infof("h2(WINDOW_UPDATE<connect level>) server allow client to (inc) %v bytes", f.Increment)
		rl.h2Conn.connWindowControl.increaseWindowSize(int64(f.Increment))
		return nil
	}
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if err := streamAliveCheck(cs, f.StreamID); err != nil {
		return err
	}
	cs.streamWindowControl.increaseWindowSize(int64(f.Increment))
	return nil
}

func (rl *http2ClientConnReadLoop) processPing(f *http2.PingFrame) error {
	err := rl.h2Conn.fr.WritePing(true, f.Data)
	if err != nil {
		return utils.Errorf("h2 server write ping error: %v", err)
	}
	return nil
}

func (rl *http2ClientConnReadLoop) processResetStream(f *http2.RSTStreamFrame) error {
	log.Infof("h2 stream-id  %v closed: %v", f.StreamID, f.ErrCode.String())
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if cs == nil {
		return utils.Errorf("unknown stream id: %v", f.StreamID)
	}
	cs.readEndStream = true
	return nil
}

func (rl *http2ClientConnReadLoop) processGoAway(f *http2.GoAwayFrame) error {
	cs := rl.h2Conn.streamByID(f.StreamID) // get stream by id
	if err := streamAliveCheck(cs, f.StreamID); err != nil {
		return err
	}
	flow := fmt.Sprintf("%v->%v", cs.h2Conn.conn.LocalAddr(), cs.h2Conn.conn.RemoteAddr())
	log.Infof("connection: %s is going away", flow)
	rl.h2Conn.readGoAway = true
	return nil
}
