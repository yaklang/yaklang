package crep

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	logger "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian"
	"github.com/yaklang/yaklang/common/minimartian/proxyutil"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

type (
	websocketHijackHandler = func(req []byte, r *http.Request, rspIns *http.Response, startTs int64) []byte
	websocketMirrorHandler = func(rsp []byte)
	websocketUpgradeMirror = func(isHttps bool, req *http.Request, rsp *http.Response, startTs int64)
)

type WebSocketModifier struct {
	ProxyStr                       []string
	websocketHijackMode            *utils.AtomicBool
	forceTextFrame                 *utils.AtomicBool
	enableCompression              *utils.AtomicBool
	websocketRequestHijackHandler  websocketHijackHandler
	websocketResponseHijackHandler websocketHijackHandler
	websocketRequestMirror         websocketMirrorHandler
	websocketResponseMirror        websocketMirrorHandler
	websocketUpgradeRequestMirror  websocketUpgradeMirror
	writeExcludeHeader             map[string]bool
	wsCanonicalHeader              []string
	ProxyGetter                    func() *minimartian.Proxy
	RequestHijackCallback          func(req *http.Request) error
	ResponseHijackCallback         func(req *http.Request, rsp *http.Response, rspRaw []byte) []byte
}

func (w *WebSocketModifier) ModifyRequest(req *http.Request) error {
	var err error

	isHijack := w.websocketHijackMode != nil && w.websocketHijackMode.IsSet()

	// hijack request
	if err := w.RequestHijackCallback(req); err != nil {
		return err
	}

	ctx := minimartian.NewContext(req, w.ProxyGetter())
	if ctx == nil {
		return nil
	}
	ctx.SkipRoundTrip()

	localConn, brw, err := ctx.Session().Hijack()
	if err != nil {
		logger.Error(err)
		return err
	}
	if err = brw.Flush(); err != nil {
		logger.Error(err)
		return err
	}
	defer localConn.Close()

	log.Infof("start to exec websocket hijack %v", localConn.RemoteAddr())

	addr := ""
	isTLS := httpctx.GetRequestHTTPS(req)
	hostname := req.URL.Hostname()
	portStr := req.URL.Port()
	scheme := req.URL.Scheme
	if portStr == "" {
		switch scheme {
		case "http", "HTTP":
			portStr = "80"
			break
		case "https", "HTTPS":
			portStr = "443"
			break
		default:
			return utils.Errorf("unknown schema: %v", scheme)
		}
	}

	if strings.Contains(hostname, ":") {
		addr = fmt.Sprintf("[%s]:%s", hostname, portStr)
	} else {
		addr = fmt.Sprintf("%s:%s", hostname, portStr)
	}
	logger.Infof("building websocket tunnel to %s", addr)

	// parse port to int
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return utils.Wrap(err, "parse port to int error")
	}

	var (
		toClient, toServer             *lowhttp.WebsocketClient
		isClientClosed, isServerClosed bool
		upgradeRspIns                  *http.Response
	)
	_, _ = isClientClosed, isServerClosed

	serverAllFrameCallback := func(c *lowhttp.WebsocketClient, f *lowhttp.Frame, data []byte, shutdown func()) {
		opcode := f.Type()
		switch opcode {
		case lowhttp.PingMessage:
			c.WritePong(data, true)
		case lowhttp.TextMessage, lowhttp.BinaryMessage:
			if isHijack {
				b := w.websocketResponseHijackHandler(data, upgradeRspIns.Request, upgradeRspIns, time.Now().UnixNano())
				toClient.WriteDirect(f.FIN(), f.RSV1(), opcode, f.GetMask(), b)
			} else {
				if w.websocketResponseMirror != nil {
					go w.websocketResponseMirror(data)
				}
				toClient.WriteDirect(f.FIN(), f.RSV1(), opcode, f.GetMask(), data)
			}
		case lowhttp.CloseMessage:
			toClient.WriteCloseEx(f.GetCloseCode(), "")
			isServerClosed = true
			log.Debugf("[grpc-ws] [>server] write close message: %d %s", f.GetCloseCode(), f.GetData())
		default:
			toClient.WriteDirect(f.FIN(), f.RSV1(), opcode, f.GetMask(), data)
			log.Debugf("[grpc-ws] [>server] write unknown message: %d", f.GetData())
		}
		if isClientClosed && isServerClosed {
			c.Close()
			toClient.Close()
			log.Debugf("[grpc-ws] [>server] close client and server")
		}
	}

	clientAllFrameCallback := func(c *lowhttp.WebsocketClient, f *lowhttp.Frame, data []byte, shutdown func()) {
		opcode := f.Type()
		switch opcode {
		case lowhttp.PingMessage:
			c.WritePong(data, true)
		case lowhttp.TextMessage, lowhttp.BinaryMessage:
			if isHijack {
				b := w.websocketRequestHijackHandler(data, upgradeRspIns.Request, upgradeRspIns, time.Now().UnixNano())
				toServer.WriteDirect(f.FIN(), f.RSV1(), opcode, f.GetMask(), b)
			} else {
				if w.websocketRequestMirror != nil {
					go w.websocketRequestMirror(data)
				}
				toServer.WriteDirect(f.FIN(), f.RSV1(), opcode, f.GetMask(), data)
			}
		case lowhttp.CloseMessage:
			toServer.WriteCloseEx(f.GetCloseCode(), "")
			isClientClosed = true
			log.Debugf("[grpc-ws] [>client] write close message: %d %s", f.GetCloseCode(), f.GetData())
		default:
			toServer.WriteDirect(f.FIN(), f.RSV1(), opcode, f.GetMask(), data)
			log.Debugf("[grpc-ws] [>client] write unknown message: %d", f.GetData())
		}
		if isClientClosed && isServerClosed {
			c.Close()
			toServer.Close()
			log.Debugf("[grpc-ws] [>client] close client and server")
		}
	}

	toServer, err = lowhttp.NewWebsocketClientByUpgradeRequest(req,
		lowhttp.WithWebsocketCompressionContextTakeover(true),
		lowhttp.WithWebsocketHost(hostname),
		lowhttp.WithWebsocketPort(port),
		lowhttp.WithWebsocketProxy(w.ProxyStr...),
		lowhttp.WithWebsocketTLS(isTLS),
		lowhttp.WithWebsocketCompress(w.enableCompression.IsSet()),
		lowhttp.WithWebsocketDisableReassembly(!isHijack), // if transparent mode, disable reassembly
		lowhttp.WithWebsocketUpgradeResponseHandler(func(rsp *http.Response, rspRaw []byte, ext *lowhttp.WebsocketExtensions, err error) []byte {
			if err != nil {
				rsp = proxyutil.NewResponse(502, nil, req)
				rspRaw, _ = utils.DumpHTTPResponse(rsp, true)
			}
			httpctx.SetBareResponseBytes(req, rspRaw)
			fixRspRaw := w.ResponseHijackCallback(req, rsp, rspRaw)

			// write back to client
			brw.Write(fixRspRaw)
			brw.Flush()
			upgradeRspIns = rsp

			// Save HTTPFlow
			if w.websocketUpgradeRequestMirror != nil {
				w.websocketUpgradeRequestMirror(isTLS, req, rsp, time.Now().Unix())
			}
			return fixRspRaw
		}),
		lowhttp.WithWebsocketAllFrameHandler(serverAllFrameCallback),
	)
	if err != nil {
		return err
	}

	// init toClient
	isDeflate := toServer.Extensions.IsDeflate
	clientFrameReader := lowhttp.NewFrameReaderFromBufio(brw.Reader, isDeflate)
	clientFrameWriter := lowhttp.NewFrameWriterFromBufio(brw.Writer, isDeflate)
	toClient = lowhttp.NewWebsocketClientIns(
		localConn,
		clientFrameReader,
		clientFrameWriter,
		toServer.Extensions,
		lowhttp.WithWebsocketDisableReassembly(!isHijack),
		lowhttp.WithWebsocketAllFrameHandler(clientAllFrameCallback),
		lowhttp.WithWebsocketCompress(w.enableCompression.IsSet()),
	)

	toServer.Start()
	toClient.Start()

	select { //  server or client closed , another side should be closed too
	case <-toServer.WaitChannel():
	case <-toClient.WaitChannel():
	}
	toServer.Close()
	toClient.Close()

	logger.Infof("websocket tunnel for %s closed", addr)
	return nil
}

// deprecated
func (w *WebSocketModifier) legacyModifyRequest(req *http.Request) error {
	var (
		err error
		got bool
		// webSocketKey string
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

	isDeflate := false

	// hijack request
	if err := w.RequestHijackCallback(req); err != nil {
		return err
	}

	ctx := minimartian.NewContext(req, w.ProxyGetter())
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
		remoteConn, err = netx.DialX(
			addr,
			netx.DialX_WithProxy(w.ProxyStr...),
			netx.DialX_WithTimeout(30*time.Second),
			netx.DialX_WithTLS(true),
		)
		break
	default:
		logger.Infof("building websocket tunnel to %s", addr)
		remoteConn, err = netx.DialX(
			addr,
			netx.DialX_WithProxy(w.ProxyStr...),
			netx.DialX_WithTimeout(30*time.Second),
			netx.DialX_WithTLS(false),
		)
	}
	if err != nil {
		logger.Error(err)
		return err
	}
	defer remoteConn.Close()
	req.RemoteAddr = remoteConn.RemoteAddr().String()
	httpctx.SetRemoteAddr(req, remoteConn.RemoteAddr().String())

	// client upgrade request to remote
	remoteConnReader := bufio.NewReader(remoteConn)
	remoteConnWriter := bufio.NewWriter(remoteConn)
	if _, err = w.writeWSReq(req, remoteConnWriter); err != nil {
		return err
	}
	remoteConnWriter.Flush()

	rspIns, err := utils.ReadHTTPResponseFromBufioReader(remoteConnReader, req)
	if err != nil {
		rspIns = proxyutil.NewResponse(502, nil, req)
		rspBytes, _ := utils.DumpHTTPResponse(rspIns, true)
		w.ResponseHijackCallback(req, rspIns, rspBytes)
		return errors.Wrap(err, "lowhttp.ReadHTTPResponseFromBufioReader")
	}

	rspBytes, err := utils.DumpHTTPResponse(rspIns, true)
	if err != nil {
		w.ResponseHijackCallback(req, rspIns, rspBytes)
		return errors.Wrap(err, "lowhttp.DumpHTTPResponse")
	}
	fixRsp := w.ResponseHijackCallback(req, rspIns, rspBytes)
	// 这里不校验，也没关系，反正本来就是为了更好兼容 "劫持部分"
	//websocketAccept := rsp.Header.Get("Sec-WebSocket-Accept")
	//checkSum := lowhttp.ComputeWebsocketAcceptKey(webSocketKey)
	//if webSocketKey != "" && websocketAccept != checkSum {
	//	return utils.Errorf("Sec-WebSocket-Accept header value invalid: %s != %s", websocketAccept, checkSum)
	//}
	rsp, err := lowhttp.ParseBytesToHTTPResponse(fixRsp)
	if err != nil {
		return utils.Errorf("parse 101 response bytes to http response failed; %s", err)
	}
	if rsp.StatusCode != 101 {
		return utils.Errorf("101 switch protocol failed: now %v", rsp.StatusCode)
	}

	extensions := lowhttp.GetWebsocketExtensions(rsp.Header)
	// 当服务端支持permessage-deflate扩展时，证明协商支持压缩
	if extensions.IsDeflate {
		isDeflate = true
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
		// forceTextFrame  bool = !(w.forceTextFrame == nil || !w.forceTextFrame.IsSet())
	)
	_ = b
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
	// isDeflate = false
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

		masked := frame.GetMask()

		showData := frame.GetData()
		switch frame.Type() {
		case lowhttp.TextMessage, lowhttp.BinaryMessage:
			b = callbackHandler(showData, req, rsp, ts)
			frameWriter.WriteDirect(frame.FIN(), frame.RSV1(), frame.Type(), masked, b)
		case lowhttp.PingMessage:
			frameWriter.WritePong(showData, masked)
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
			writer.WritePong(frame.GetData(), true)
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
			go callbackHandler(frame.GetData())
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

	_, err = bw.Write([]byte("\r\n"))
	if err != nil {
		return
	}

	return
}
