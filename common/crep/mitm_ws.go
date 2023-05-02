package crep

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
	"yaklang.io/yaklang/common/log"
	logger "yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/martian/v3"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/lowhttp"

	"github.com/pkg/errors"
)

type WebSocketModifier struct {
	ProxyStr                       string
	websocketHijackMode            *utils.AtomicBool
	forceTextFrame                 *utils.AtomicBool
	websocketRequestHijackHandler  func(req []byte, r *http.Request, rspIns *http.Response, startTs int64) []byte
	websocketResponseHijackHandler func(rsp []byte, r *http.Request, rspIns *http.Response, startTs int64) []byte
	websocketRequestMirror         func(req []byte)
	websocketResponseMirror        func(rsp []byte)
	TR                             *http.Transport
	writeExcludeHeader             map[string]bool
	wsCanonicalHeader              []string
	ProxyGetter                    func() *martian.Proxy
	RequestHijackCallback          func(req *http.Request) error
}

func (w *WebSocketModifier) ModifyRequest(req *http.Request) error {
	var (
		err error
		got bool
		//webSocketKey string
	)

	// check if it is a websocket upgrade request
	if req.Method != http.MethodGet {
		return nil
	}
	for _, vs := range req.Header["Connection"] {
		for _, v := range strings.Split(vs, ",") {
			if strings.TrimSpace(strings.ToLower(v)) == "upgrade" {
				got = true
			}
		}
	}
	if !got {
		return nil
	}
	if req.Header.Get("Upgrade") != "websocket" {
		return nil
	}

	// 如果请求存在permessage-deflate扩展则设置isDeflate
	isDeflate := lowhttp.IsPermessageDeflate(req.Header)

	// hijack request
	if err := w.RequestHijackCallback(req); err != nil {
		return err
	}

	ctx := martian.NewContext(req, w.ProxyGetter())
	if ctx == nil {
		return nil
	}
	ctx.SkipRoundTrip()

	conn, brw, err := ctx.Session().Hijack()
	if err != nil {
		logger.Error(err)
		return err
	}
	if err = brw.Flush(); err != nil {
		logger.Error(err)
		return err
	}
	defer conn.Close()

	log.Infof("start to exec websocket hijack %v", conn.RemoteAddr())
	var addr string
	hostname := req.URL.Hostname()
	port := req.URL.Port()
	scheme := req.URL.Scheme
	if port == "" {
		switch scheme {
		case "http", "HTTP":
			port = "80"
			break
		case "https", "HTTPS":
			port = "443"
			break
		default:
			return utils.Errorf("unknown schema: %v", scheme)
		}
	}
	if strings.Contains(hostname, ":") {
		addr = fmt.Sprintf("[%s]:%s", hostname, port)
	} else {
		addr = fmt.Sprintf("%s:%s", hostname, port)
	}

	// dial remote
	var remoteConn net.Conn
	switch strings.ToLower(scheme) {
	case "https", "wss":
		logger.Infof("building websocket tls tunnel to %s", addr)
		//remoteConn, err = tls.DialWithDialer(dialer, "tcp", addr, w.TR.TLSClientConfig)
		remoteConn, err = utils.GetAutoProxyConnWithTLS(addr, w.ProxyStr, 30*time.Second, w.TR.TLSClientConfig)
		break
	default:
		logger.Infof("building websocket tunnel to %s", addr)
		remoteConn, err = utils.GetAutoProxyConn(addr, w.ProxyStr, 30*time.Second)
	}
	if err != nil {
		logger.Error(err)
		return err
	}
	defer remoteConn.Close()

	// client upgrade request to remote
	remoteConnReader := bufio.NewReader(remoteConn)
	remoteConnWriter := bufio.NewWriter(remoteConn)
	if _, err = w.writeWSReq(req, remoteConnWriter); err != nil {
		return err
	}
	remoteConnWriter.Flush()

	rspBytes, err := lowhttp.ReadHTTPPacketSafe(remoteConnReader)
	if err != nil {
		return errors.Wrap(err, "lowhttp.ReadHTTPPacketSafe")
	}
	// 这里不校验，也没关系，反正本来就是为了更好兼容 "劫持部分"
	//websocketAccept := rsp.Header.Get("Sec-WebSocket-Accept")
	//checkSum := lowhttp.ComputeWebsocketAcceptKey(webSocketKey)
	//if webSocketKey != "" && websocketAccept != checkSum {
	//	return utils.Errorf("Sec-WebSocket-Accept header value invalid: %s != %s", websocketAccept, checkSum)
	//}
	rsp, err := lowhttp.ParseBytesToHTTPResponse(rspBytes)
	if err != nil {
		return utils.Errorf("parse 101 response bytes to http response failed; %s", err)
	}
	if rsp.StatusCode != 101 {
		return utils.Errorf("101 switch protocol failed: now %v", rsp.StatusCode)
	}

	// 如果响应中不存在permessage-deflate扩展则要反设置isDeflate
	serverSupportDeflate := lowhttp.IsPermessageDeflate(rsp.Header)

	// 当服务端不支持permessage-deflate扩展时，客户端也不应该使用
	if !serverSupportDeflate && isDeflate {
		isDeflate = false
	}

	if _, err = brw.Writer.Write(rspBytes); err != nil {
		return err
	}
	if err = brw.Writer.Flush(); err != nil {
		return err
	}

	hijackCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := time.Now().UnixNano()

	log.Infof("start to build websocket hijack tunnel: %v", conn.RemoteAddr())
	// 透明模式
	if w.websocketHijackMode == nil || !w.websocketHijackMode.IsSet() {
		go w.copySync(lowhttp.NewFrameWriter(remoteConn, isDeflate), lowhttp.NewFrameReader(brw.Reader, isDeflate), true, req, rsp, cancel, ts)
		go w.copySync(lowhttp.NewFrameWriter(brw.Writer, isDeflate), lowhttp.NewFrameReader(remoteConn, isDeflate), false, req, rsp, cancel, ts)
	} else {
		//go w.copyHijack(lowhttp.NewFrameWriter(remoteConn), lowhttp.NewFrameReader(brw.Reader), true, req, rsp, cancel, ts)
		//go w.copyHijack(lowhttp.NewFrameWriter(brw.Writer), lowhttp.NewFrameReader(remoteConn), false, req, rsp, cancel, ts)
		//go io.Copy(remoteConn, io.TeeReader(brw.Reader, os.Stdout))
		//go io.Copy(brw.Writer, io.TeeReader(remoteConn, os.Stdout))
		go w.copyHijack(remoteConnWriter, brw.Reader, true, req, rsp, cancel, ts, isDeflate)
		go w.copyHijack(brw.Writer, remoteConnReader, false, req, rsp, cancel, ts, isDeflate)
	}

	select {
	case <-hijackCtx.Done():
		break
	}
	logger.Infof("websocket tunnel for %s closed", addr)
	return nil
}

func (w *WebSocketModifier) copyHijack(writer *bufio.Writer, reader *bufio.Reader, isRequest bool, req *http.Request, rsp *http.Response, cancel context.CancelFunc, ts int64, isDeflate bool) {
	defer cancel()

	var (
		b               []byte
		frame           *lowhttp.Frame
		err             error
		callbackHandler func([]byte, *http.Request, *http.Response, int64) []byte
		//forceTextFrame  bool = !(w.forceTextFrame == nil || !w.forceTextFrame.IsSet())
	)

	// hijack
	if isRequest {
		callbackHandler = w.websocketRequestHijackHandler
	} else {
		callbackHandler = w.websocketResponseHijackHandler
	}

	if callbackHandler == nil {
		callbackHandler = func(bytes []byte, request *http.Request, response *http.Response, i int64) []byte {
			return bytes
		}
	}

	frameReader := lowhttp.NewFrameReaderFromBufio(reader, isDeflate)
	frameWriter := lowhttp.NewFrameWriterFromBufio(writer, isDeflate)

	for {
		frame, err = frameReader.ReadFrame()
		if err != nil {
			if frame != nil {
				rawData, _ := frame.Bytes()
				switch frame.Type() {
				case lowhttp.BinaryMessage, lowhttp.TextMessage:
					callbackHandler(rawData, req, rsp, ts)
				}
				frameWriter.WriteRaw(rawData)
				frameWriter.Flush()
			}
			break
		}

		raw, clearData := frame.Bytes()
		if len(raw) < 2 {
			break
		}
		_ = clearData
		//frame.Show()

		masked := raw[1]&0b10000000 != 0

		switch frame.Type() {
		case lowhttp.TextMessage:
			b = callbackHandler(clearData, req, rsp, ts)
			newFrame, err := lowhttp.DataToWebsocketFrame(b, raw[0], masked)
			if err != nil {
				frameWriter.WriteRaw(raw)
				frameWriter.Flush()
				continue
			}
			newFrame.SetMaskingKey(frame.GetMaskingKey())
			newRaw, _ := newFrame.Bytes()
			frameWriter.WriteRaw(newRaw)
		case lowhttp.BinaryMessage:
			b = callbackHandler(clearData, req, rsp, ts)
			newFrame, err := lowhttp.DataToWebsocketFrame(b, raw[0], masked)
			if err != nil {
				frameWriter.WriteRaw(raw)
				frameWriter.Flush()
				continue
			}
			newFrame.SetMaskingKey(frame.GetMaskingKey())
			newRaw, _ := newFrame.Bytes()
			frameWriter.WriteRaw(newRaw)
		case lowhttp.PingMessage:
			frameWriter.WritePong(frame.RawPayloadData(), masked)
		default:
			if err = frameWriter.WriteFrame(frame); err != nil {
				log.Errorf("write frame failed: %s", err)
			}
		}

		if err = frameWriter.Flush(); err != nil {
			break
		}

	}

	if err != nil {
		log.Errorf("websocket dial with hijack mode error: %v", err)
	}
}

func (w *WebSocketModifier) copySync(writer *lowhttp.FrameWriter, reader *lowhttp.FrameReader, isRequest bool, req *http.Request, rsp *http.Response, cancel context.CancelFunc, ts int64) {
	defer cancel()

	var (
		frame           *lowhttp.Frame
		err             error
		callbackHandler func([]byte)
	)

	if isRequest {
		callbackHandler = w.websocketRequestMirror
	} else {
		callbackHandler = w.websocketResponseMirror
	}

	for {
		frame, err = reader.ReadFrame()
		switch frame.Type() {
		case lowhttp.BinaryMessage, lowhttp.TextMessage:

		case lowhttp.PingMessage:
			writer.WritePong(frame.RawPayloadData(), true)
		default:
			continue
		}

		if err != nil {
			if frame != nil {
				rawData, _ := frame.Bytes()
				if len(rawData) <= 0 {
					break
				}
				callbackHandler(rawData)
				writer.WriteRaw(rawData)
				writer.Flush()
				continue
			}
			break
		}

		// mirror

		if callbackHandler != nil {
			go callbackHandler(lowhttp.WebsocketFrameToData(frame))
		}

		if err = writer.WriteFrame(frame); err != nil {
			break
		}

		if err = writer.Flush(); err != nil {
			break
		}
	}

	if err != nil {
		log.Errorf("websocket dial with hijack mode error: %v", err)
		cancel()
	}
}

// todo: trailer
func (w *WebSocketModifier) writeWSReq(req *http.Request, bw io.Writer) (webSocketKey string, err error) {
	raw, err := utils.HttpDumpWithBody(req, true)
	if err != nil {
		log.Warnf("dump websocket first upgrade req failed: %s", err)
	}
	if len(raw) > 0 {
		bw.Write(raw)
		return "", nil
	}

	// can't use req.Header.Get
	if keys, ok := req.Header["Sec-WebSocket-Key"]; ok {
		webSocketKey = keys[0]
	}

	_, err = fmt.Fprintf(bw, "GET %s HTTP/1.1\r\n", req.URL.RequestURI())
	if err != nil {
		return
	}

	var host string
	if req.Host != "" {
		host = req.Host
	} else if req.URL.Host != "" {
		host = req.Host
	} else {
		err = utils.Errorf("missing host")
		return
	}
	_, err = fmt.Fprintf(bw, "Host: %s\r\n", host)
	if err != nil {
		return
	}

	if w.writeExcludeHeader == nil {
		w.writeExcludeHeader = map[string]bool{
			"Host":                     true,
			"Sec-Websocket-Extensions": true,
			"Sec-Websocket-Key":        true,
			"Sec-Websocket-Protocol":   true,
			"Sec-Websocket-Version":    true,
		}
		w.wsCanonicalHeader = []string{
			"Sec-WebSocket-Extensions",
			"Sec-WebSocket-Key",
			"Sec-WebSocket-Protocol",
			"Sec-WebSocket-Version",
		}
	}

	err = req.Header.WriteSubset(bw, w.writeExcludeHeader)
	if err != nil {
		return
	}

	// write WebSocket special headers
	headers := req.Header
	for _, k := range w.wsCanonicalHeader {
		if values, ok := headers[k]; ok {
			for _, v := range values {
				if v == "" {
					continue
				}
				_, err = fmt.Fprintf(bw, "%s: %s\r\n", k, v)
				if err != nil {
					return
				}
			}
		}
	}

	_, err = io.WriteString(bw, "\r\n")
	if err != nil {
		return
	}
	//err = bw.Flush()
	if err != nil {
		return
	}

	return
}
