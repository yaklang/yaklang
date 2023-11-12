package lowhttp

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type http2ConnectionConfig struct {
	handler func(header []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error)
	frame   *http2.Framer

	// writer
	hencBuf   *bytes.Buffer
	henc      *hpack.Encoder
	hencMutex *sync.Mutex

	wg *sync.WaitGroup
}

func (c *http2ConnectionConfig) writer(wrapper *h2RequestState, header []byte, body io.ReadCloser) error {
	if c.frame == nil {
		return utils.Error("h2 server frame config is nil")
	}
	streamId := wrapper.streamId
	frame := c.frame
	henc := c.henc
	buf := c.hencBuf

	c.hencMutex.Lock()
	buf.Reset()
	SplitHTTPPacket(header, nil, func(proto string, code int, codeMsg string) error {
		henc.WriteField(hpack.HeaderField{Name: ":status", Value: fmt.Sprint(code)})
		return nil
	}, func(line string) string {
		k, v := SplitHTTPHeader(line)
		henc.WriteField(hpack.HeaderField{Name: strings.ToLower(k), Value: v})
		return line
	})
	var hpackHeaderBytes = buf.Bytes()
	buf.Reset()

	//defer func() {
	//	log.Infof("handle h2 stream(%v) done", streamId)
	//}()
	//log.Infof("start to write h2 response header stream-id: %v", streamId)
	ret := funk.Chunk(hpackHeaderBytes, defaultMaxFrameSize).([][]byte)
	first := true
	for index, item := range ret {
		if first {
			first = false
			err := frame.WriteHeaders(http2.HeadersFrameParam{
				StreamID:      uint32(streamId),
				BlockFragment: item,
				EndStream:     false,
				EndHeaders:    index == len(ret)-1,
			})
			if err != nil {
				return utils.Wrapf(err, "h2framer write header(%v) for stream:%v failed", len(hpackHeaderBytes), streamId)
			}
		} else {
			err := frame.WriteContinuation(uint32(streamId), index == len(ret)-1, item)
			if err != nil {
				return utils.Wrapf(err, "h2framer write header(%v)-continuation for stream:%v failed", len(hpackHeaderBytes), streamId)
			}
		}
	}
	c.hencMutex.Unlock()

	results, err := io.ReadAll(body)
	//log.Infof("start to write data{%v} to stream-id: %v", len(results), streamId)
	if len(results) > 0 {
		chunks := funk.Chunk(results, defaultMaxFrameSize).([][]byte)
		first = true
		for index, dataFrameBytes := range chunks {
			dataFrameErr := frame.WriteData(uint32(streamId), index == len(chunks)-1, dataFrameBytes)
			if dataFrameErr != nil {
				return utils.Wrapf(dataFrameErr, "framer WriteData for stream{%v} failed", streamId)
			}
		}
	} else {
		dataFrameErr := frame.WriteData(uint32(streamId), true, nil)
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
		c.handler = h
	}
}

func (c *http2ConnectionConfig) handleRequest(wrapper *h2RequestState, header []byte, body io.ReadCloser) error {
	if c == nil || c.handler == nil {
		return utils.Error("h2 server handler config is nil")
	}
	header, rc, err := c.handler(header, body)
	if err != nil {
		return utils.Errorf("waiting for userspace handling for h2 stream(%v) failed: %v", wrapper.streamId, err)
	}
	return c.writer(wrapper, header, rc)
}

type h2RequestState struct {
	config *http2ConnectionConfig

	streamId       int
	headerHPackBuf *bytes.Buffer
	bodyReader     *utils.PipeReader
	bodyBuf        *utils.PipeWriter
	headerEnd      bool
}

func (w *h2RequestState) headerDone(req *http.Request, pairs []*ypb.KVPair) error {
	if w.headerEnd {
		return nil
	}
	w.headerEnd = true
	w.config.wg.Add(1)
	go func() {
		defer func() {
			time.Sleep(time.Second)
			w.config.wg.Done()
			if err := recover(); err != nil {
				log.Errorf("emitRequest panic, h2 stream(%v) header done failed: %s", w.streamId, err)
			}
		}()
		err := w.emitRequestHeader(req, pairs)
		if err != nil {
			log.Errorf("h2 stream(%v) header failed: %s", w.streamId, err)
			w.Close()
		}
	}()
	return nil
}

func (w *h2RequestState) Close() error {
	w.bodyReader.Close()
	w.bodyBuf.Close()
	return nil
	// return w.config.frame.WriteRSTStream(uint32(w.streamId), http2.ErrCodeStreamClosed)
}

func newH2RequestState(
	streamId int, config *http2ConnectionConfig,
) *h2RequestState {
	r, w := utils.NewBufPipe(nil)
	return &h2RequestState{
		config:         config,
		streamId:       int(streamId),
		headerHPackBuf: new(bytes.Buffer),
		bodyReader:     r,
		bodyBuf:        w,
	}
}

func (w *h2RequestState) emitRequestHeader(req *http.Request, pairs []*ypb.KVPair) error {
	var buf = new(bytes.Buffer)
	if req == nil {
		return utils.Error("h2 server request is nil")
	}
	buf.WriteString(req.Method)
	buf.WriteByte(' ')
	buf.WriteString(req.RequestURI)
	buf.WriteString(" HTTP/2\r\n")
	buf.WriteString("Host: ")
	buf.WriteString(req.Host)
	for _, p := range pairs {
		buf.WriteString("\r\n")
		buf.WriteString(p.Key)
		buf.WriteString(": ")
		buf.WriteString(p.Value)
	}
	buf.WriteString("\r\n\r\n")
	err := w.config.handleRequest(w, buf.Bytes(), w.bodyReader)
	if err != nil && err != io.EOF {
		log.Errorf("emitRequestHeader failed: %v", err)
	}
	return nil
}

func serveH2(r io.Reader, conn net.Conn, opt ...h2Option) error {
	var config = &http2ConnectionConfig{
		handler: func(header []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error) {
			return nil, nil, utils.Errorf("h2 config is nil")
		},
		wg: new(sync.WaitGroup),
	}
	for _, o := range opt {
		o(config)
	}

	// handshake
	// 1. read preface
	var preface = make([]byte, len(http2.ClientPreface))
	n, err := io.ReadAtLeast(r, preface, len(preface))
	if err != nil {
		return utils.Errorf("h2 server read preface error: %v", err)
	}
	if n != len(preface) {
		return utils.Errorf("h2 server read preface error: read %d bytes, expected %d bytes", n, len(preface))
	}

	frame := http2.NewFramer(conn, r)
	config.frame = frame
	// send settings
	/*
		{SettingMaxFrameSize, sc.srv.maxReadFrameSize()},
		{SettingMaxConcurrentStreams, sc.advMaxStreams},
		{SettingMaxHeaderListSize, sc.maxHeaderListSize()},
		{SettingInitialWindowSize, uint32(sc.srv.initialStreamRecvWindowSize())},
	*/
	err = frame.WriteSettings(
		http2.Setting{ID: http2.SettingInitialWindowSize, Val: defaultStreamReceiveWindowSize},
		http2.Setting{ID: http2.SettingMaxFrameSize, Val: defaultMaxFrameSize},
		http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: defaultMaxConcurrentStreamSize},
		http2.Setting{ID: http2.SettingMaxHeaderListSize, Val: defaultMaxHeaderListSize},
	)
	if err != nil {
		return utils.Errorf("h2 server write settings error: %v", err)
	}

	var (
		hdec = hpack.NewDecoder(defaultHeaderTableSize, nil)

		// hpack encoder
		hencMutex = new(sync.Mutex)
		hencBuf   = new(bytes.Buffer)
		henc      = hpack.NewEncoder(hencBuf)
	)

	config.henc = henc
	config.hencBuf = hencBuf
	config.hencMutex = hencMutex

	// read settings
	streamToBuf := new(sync.Map)
	getReq := func(streamIdU21 uint32) *h2RequestState {
		streamId := int(streamIdU21)
		var req *h2RequestState
		raw, ok := streamToBuf.Load(streamId)
		if !ok {
			req = newH2RequestState(streamId, config)
			streamToBuf.Store(streamId, req)
			return req
		} else {
			return raw.(*h2RequestState)
		}
	}

	handleRequestHeader := func(req *h2RequestState) (*http.Request, []*ypb.KVPair, error) {
		var reqInstance = new(http.Request)
		var pairs []*ypb.KVPair
		hdec.SetEmitFunc(func(hf hpack.HeaderField) {
			// :authority -> host
			// :method -> method
			// :path -> requestUri
			// :schema -> schema
			switch ret := strings.ToLower(hf.Name); ret {
			case ":method":
				reqInstance.Method = strings.ToUpper(hf.Value)
			case ":path":
				reqInstance.RequestURI = hf.Value
			case ":authority":
				reqInstance.Host = hf.Value
			case ":scheme":
			default:
				if hf.IsPseudo() {
					log.Warnf("unhandled pseudo header: %s", hf.Name)
				} else {
					pairs = append(pairs, &ypb.KVPair{
						Key:   hf.Name,
						Value: hf.Value,
					})
				}
			}
		})
		_, err := hdec.Write(req.headerHPackBuf.Bytes())
		if err != nil {
			return nil, nil, err
		}
		return reqInstance, pairs, nil
	}

	for {
		rawFrame, err := frame.ReadFrame()
		if err != nil {
			return utils.Errorf("h2 server read frame error: %v", err)
		}
		//log.Infof("h2 server read frame: %v", rawFrame)

		switch ret := rawFrame.(type) {
		case *http2.SettingsFrame:
			if ret.IsAck() {
				continue
			}

			ret.ForeachSetting(func(setting http2.Setting) error {
				log.Infof("h2 stream found client setting: %v", setting.String())
				switch setting.ID {
				case http2.SettingMaxFrameSize:
				case http2.SettingMaxConcurrentStreams:
				}
				return nil
			})
			// write settings ack
			err := frame.WriteSettingsAck()
			if err != nil {
				return utils.Errorf("h2 server write settings ack error: %v", err)
			}
		case *http2.WindowUpdateFrame:
			// update window
		case *http2.HeadersFrame:
			// build request
			// log.Infof("h2 stream-id fetch header: %v", ret.StreamID)
			streamId := ret.StreamID
			req := getReq(streamId)
			if b := ret.HeaderBlockFragment(); len(b) > 0 {
				req.headerHPackBuf.Write(b)
			}
			if ret.StreamEnded() {
				req.bodyBuf.Close()
			}
			if ret.HeadersEnded() {
				reqInstance, pairs, err := handleRequestHeader(req)
				if err != nil {
					log.Errorf("hpack decode header failed: %s, close connection", err)
					conn.Close()
					return err
				}
				// log.Infof("h2 stream-id done header: %v", ret.StreamID)
				err = req.headerDone(reqInstance, pairs)
				if err != nil {
					return err
				}
			}

		case *http2.ContinuationFrame:
			req := getReq(ret.StreamID)
			if b := ret.HeaderBlockFragment(); len(b) > 0 {
				req.headerHPackBuf.Write(b)
			}
			if ret.HeadersEnded() {
				reqInstance, pairs, err := handleRequestHeader(req)
				if err != nil {
					log.Errorf("hpack decode header(continuation-frame) failed: %s, close connection", err)
					conn.Close()
					return err
				}
				// log.Infof("h2 stream-id done header: %v", ret.StreamID)
				err = req.headerDone(reqInstance, pairs)
				if err != nil {
					return err
				}
			}
		case *http2.DataFrame:
			// update window
			err := frame.WriteWindowUpdate(0, transportDefaultConnFlow)
			if err != nil {
				return utils.Errorf("h2 server write window update error: %v", err)
			}
			err = frame.WriteWindowUpdate(ret.StreamID, transportDefaultConnFlow)
			if err != nil {
				return utils.Errorf("h2 server write window update error: %v", err)
			}

			req := getReq(ret.StreamID)
			if len(ret.Data()) > 0 {
				req.bodyBuf.Write(ret.Data())
			}
			if ret.StreamEnded() {
				req.bodyBuf.Close()
			}
		case *http2.PingFrame:
			err := frame.WritePing(true, ret.Data)
			if err != nil {
				return utils.Errorf("h2 server write ping error: %v", err)
			}
		case *http2.RSTStreamFrame:
			// close stream
			log.Infof("h2 stream-id closed: %v reason: %v", ret.StreamID, ret.ErrCode.String())
			req := getReq(ret.StreamID)
			req.Close()
			streamToBuf.Delete(int(ret.StreamID))
			streamToBuf.Delete(ret.StreamID)
		case *http2.GoAwayFrame:
			flow := fmt.Sprintf("%v->%v", conn.LocalAddr(), conn.RemoteAddr())
			log.Infof("connection: %s is going away, start to waitgroup and return", flow)
			config.wg.Wait()
			log.Infof("connection: %s is start to closing", flow)
			time.AfterFunc(time.Millisecond*800, func() {
				conn.Close()
			})
			return nil
		default:
			log.Errorf("h2 server unknown frame type: %T", ret)
			log.Infof("unhandled frame: %v", ret)
		}
	}
}

func ServeHTTP2Connection(conn net.Conn, handler func(header []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error)) error {
	return serveH2(conn, conn, withH2Handler(handler))
}
