package wsc

//
//import (
//	"bufio"
//	"context"
//	"fmt"
//	"github.com/tidwall/gjson"
//	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
//	"github.com/yaklang/yaklang/common/log"
//	"github.com/yaklang/yaklang/common/utils"
//	"github.com/yaklang/yaklang/common/utils/lowhttp"
//	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
//	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
//	"net"
//	"strings"
//)
//
//type WebsocketController struct {
//	Token     string
//	Port      int
//	Ctx       context.Context
//	clientsFw map[string]*lowhttp.FrameWriter
//	clientsFr map[string]*lowhttp.FrameReader
//	clients   map[string]net.Conn
//}
//
//func NewWebsocketController(token string, port int) *WebsocketController {
//	return &WebsocketController{Port: port, Token: token, clientsFw: make(map[string]*lowhttp.FrameWriter), clientsFr: make(map[string]*lowhttp.FrameReader), clients: make(map[string]net.Conn)}
//}
//
//// 在连接建立时将客户端连接存储起来
//func (w *WebsocketController) storeClient(clientAddr string, conn net.Conn) {
//	w.clients[clientAddr] = conn
//}
//
//// 在连接断开时将客户端连接从存储中移除
//func (w *WebsocketController) removeClient() {
//	delete(w.clientsFw, w.Token)
//	delete(w.clientsFr, w.Token)
//}
//
//func (w *WebsocketController) getClientFw() *lowhttp.FrameWriter {
//	return w.clientsFw[w.Token]
//}
//
//func (w *WebsocketController) getClientFr() *lowhttp.FrameReader {
//	return w.clientsFr[w.Token]
//}
//
//func (w *WebsocketController) Run() error {
//	log.Infof("start to listen websocket controller on : 0.0.0.0:%v", w.Port)
//	lis, err := net.Listen("tcp", utils.HostPort("0.0.0.0", w.Port))
//	if err != nil {
//		log.Errorf("Listen ws controller failed: %v", err)
//		return utils.Errorf("listen ws controller in ws://0.0.0.0:%v", w.Port)
//	}
//
//	rootCtx := w.Ctx
//	if rootCtx == nil {
//		rootCtx = context.Background()
//	}
//
//	rootCtx, cancel := context.WithCancel(rootCtx)
//	defer cancel()
//
//	for {
//		conn, err := lis.Accept()
//		if err != nil {
//			return err
//		}
//		log.Infof("accept %v ws connection", conn.RemoteAddr().String())
//		conn = ctxio.NewConn(rootCtx, conn)
//		serverReader := bufio.NewReader(conn)
//		serverWriter := bufio.NewWriter(conn)
//
//		go func() {
//			defer func() {
//				conn.Close()
//			}()
//			log.Infof("start to handle ws connection")
//			if err := w.handle(serverReader, serverWriter); err != nil {
//				log.Errorf("ws controller handle error: %v", err)
//			}
//		}()
//	}
//}
//
//func (w *WebsocketController) handle(br *bufio.Reader, bw *bufio.Writer) error {
//	req, err := utils.ReadHTTPRequestFromBufioReader(br)
//	if err != nil {
//		return err
//	}
//	raw := httpctx.GetBareRequestBytes(req)
//	if len(raw) <= 0 {
//		return utils.Error("BUG: request raw message is not fit in ws controller")
//	}
//
//	isDeflate := false
//	if utils.IContains(req.Header.Get(`Sec-WebSocket-Extensions`), "deflate") {
//		isDeflate = true
//	}
//
//	w.Token = lowhttp.GetHTTPRequestQueryParam(raw, "token")
//
//	fmt.Println(string(raw))
//
//	log.Infof("ws controller token is right handshake is ok!")
//
//	key := req.Header.Get("Sec-WebSocket-Key")
//	if key == "" {
//		key = req.Header.Get("sec-websocket-key")
//		if key == "" {
//			key = lowhttp.GetHTTPPacketHeader(raw, "Sec-WebSocket-Key")
//		}
//	}
//	rspKey := ""
//	var base []byte
//	if key != "" {
//		// sha1 with key + magic
//		log.Infof("fetch sec-websocket-key: %v", key)
//		base, err = codec.DecodeHex(codec.Sha1(key + `258EAFA5-E914-47DA-95CA-C5AB0DC85B11`))
//		if err != nil {
//			return utils.Errorf("calc ws controller response key failed: %v", err)
//		}
//		rspKey = codec.EncodeBase64(base)
//	}
//
//	// ws response
//	wsResponse := []byte("HTTP/1.1 101 Switching Protocols\r\n" +
//		"Upgrade: websocket\r\n" +
//		"Connection: Upgrade\r\n" +
//		"\r\n")
//	if rspKey != "" {
//		wsResponse = lowhttp.AppendHTTPPacketHeader(wsResponse, "Sec-WebSocket-Accept", rspKey)
//	}
//
//	fmt.Println(string(wsResponse))
//	if _, err := bw.Write(wsResponse); err != nil {
//		return utils.Errorf("write ws controller response failed: %v", err)
//	}
//	if err := bw.Flush(); err != nil {
//		return utils.Errorf("flush ws controller response failed: %v", err)
//	}
//
//	w.clientsFr[w.Token], w.clientsFw[w.Token] = lowhttp.NewFrameReader(br, isDeflate), lowhttp.NewFrameWriter(bw, isDeflate)
//	return w.frameHandler()
//}
//
//func (w *WebsocketController) frameHandler() error {
//	count := 0
//	_ = count
//	for {
//		frame, err := w.getClientFr().ReadFrame()
//		if err != nil {
//			log.Errorf("ws controller read frame failed: %v", err)
//			return err
//		}
//		frame.Show()
//		masked := frame.GetMask()
//		switch frame.Type() {
//		case lowhttp.PingMessage:
//			w.getClientFw().WritePong(frame.GetData(), masked)
//			continue
//		case lowhttp.TextMessage, lowhttp.BinaryMessage:
//			w.onMessage(frame.GetData())
//		}
//		//err = w.getClientFw().WriteText([]byte(fmt.Sprintf(`test %d`, count)), false)
//		//if err := w.getClientFw().Flush(); err != nil {
//		//	log.Errorf("ws controller flush failed: %v", err)
//		//	return err
//		//}
//		//count++
//	}
//}
//
//func (w *WebsocketController) onMessage(jsonText []byte) {
//	result := gjson.ParseBytes(jsonText)
//	msgType := result.Get("type").String()
//	switch strings.ToLower(strings.TrimSpace(msgType)) {
//	case "heartbeat":
//		log.Infof("heartbeat message from ws client")
//	case "chrome-extension":
//		w.getClientFw().WriteText(jsonText, false)
//		if err := w.getClientFw().Flush(); err != nil {
//			log.Errorf("ws controller flush failed: %v", err)
//		}
//	default:
//		if w.Token == "fuzzer" {
//			// 往 chrome client 发
//			fw := w.clientsFw["chrome"]
//			if fw != nil {
//				fw.WriteText(jsonText, false)
//				if err := fw.Flush(); err != nil {
//					log.Errorf("ws controller flush failed: %v", err)
//				}
//
//			}
//			log.Infof("from fuzzer recv client: %v", string(result.String()))
//
//		}
//		if w.Token == "chrome" {
//			// 往 fuzzer client 发
//			fw := w.clientsFw["fuzzer"]
//			if fw != nil {
//				fw.WriteText(jsonText, false)
//				if err := fw.Flush(); err != nil {
//					log.Errorf("ws controller flush failed: %v", err)
//				}
//			}
//			log.Infof("from chrome recv client: %v", string(result.String()))
//
//		}
//	}
//}
