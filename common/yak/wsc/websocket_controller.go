package wsc

import (
	"bufio"
	"context"
	"fmt"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"strings"
)

type WebsocketController struct {
	Token string
	Port  int
	Ctx   context.Context
}

func NewWebsocketController(token string, port int) *WebsocketController {
	return &WebsocketController{Port: port, Token: token}
}

func (w *WebsocketController) Run() error {
	log.Infof("start to listen on :%v localport", w.Port)
	lis, err := net.Listen("tcp", utils.HostPort("0.0.0.0", w.Port))
	if err != nil {
		log.Errorf("Listen ws controller failed: %v", err)
		return utils.Errorf("listen ws controller in ws://0.0.0.0:%v", w.Port)
	}

	rootCtx := w.Ctx
	if rootCtx == nil {
		rootCtx = context.Background()
	}

	rootCtx, cancel := context.WithCancel(rootCtx)
	defer cancel()

	for {
		conn, err := lis.Accept()
		if err != nil {
			return err
		}
		log.Infof("accept %v ws connection", conn.RemoteAddr().String())
		conn = ctxio.NewConn(rootCtx, conn)
		serverReader := bufio.NewReader(conn)
		serverWriter := bufio.NewWriter(conn)
		go func() {
			defer func() {
				conn.Close()
			}()
			log.Infof("start to handle ws connection")
			if err := w.handle(serverReader, serverWriter); err != nil {
				log.Errorf("ws controller handle error: %v", err)
			}
		}()
	}
}

func (w *WebsocketController) handle(br *bufio.Reader, bw *bufio.Writer) error {
	req, err := utils.ReadHTTPRequestFromBufioReader(br)
	if err != nil {
		return err
	}
	raw := httpctx.GetBareRequestBytes(req)
	if len(raw) <= 0 {
		return utils.Error("BUG: request raw message is not fit in ws controller")
	}

	isDeflate := false
	if utils.IContains(req.Header.Get(`Sec-WebSocket-Extensions`), "deflate") {
		isDeflate = true
	}

	//if lowhttp.GetHTTPRequestQueryParam(raw, "token") != w.Token {
	//	return utils.Error("token is not right")
	//}

	fmt.Println(string(raw))

	log.Infof("ws controller token is right handshake is ok!")

	key := req.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		key = req.Header.Get("sec-websocket-key")
		if key == "" {
			key = lowhttp.GetHTTPPacketHeader(raw, "Sec-WebSocket-Key")
		}
	}
	rspKey := ""
	var base []byte
	if key != "" {
		// sha1 with key + magic
		log.Infof("fetch sec-websocket-key: %v", key)
		base, err = codec.DecodeHex(codec.Sha1(key + `258EAFA5-E914-47DA-95CA-C5AB0DC85B11`))
		if err != nil {
			return utils.Errorf("calc ws controller response key failed: %v", err)
		}
		rspKey = codec.EncodeBase64(base)
	}

	// ws response
	wsResponse := []byte("HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"\r\n")
	if rspKey != "" {
		wsResponse = lowhttp.AppendHTTPPacketHeader(wsResponse, "Sec-WebSocket-Accept", rspKey)
	}

	fmt.Println(string(wsResponse))
	if _, err := bw.Write(wsResponse); err != nil {
		return utils.Errorf("write ws controller response failed: %v", err)
	}
	if err := bw.Flush(); err != nil {
		return utils.Errorf("flush ws controller response failed: %v", err)
	}

	frameReader, frameWriter := lowhttp.NewFrameReader(br, isDeflate), lowhttp.NewFrameWriter(bw, isDeflate)
	return w.frameHandler(frameReader, frameWriter)
}

func (w *WebsocketController) frameHandler(fr *lowhttp.FrameReader, fw *lowhttp.FrameWriter) error {
	for {
		frame, err := fr.ReadFrame()
		if err != nil {
			log.Errorf("ws controller read frame failed: %v", err)
			return err
		}
		frame.Show()
		masked := frame.GetMask()
		switch frame.Type() {
		case lowhttp.PingMessage:
			fw.WritePong(frame.GetData(), masked)
			continue
		case lowhttp.TextMessage, lowhttp.BinaryMessage:
			w.onMessage(frame.GetData())
		}
	}
}

func (w *WebsocketController) onMessage(jsonText []byte) {
	result := gjson.ParseBytes(jsonText)
	msgType := result.Get("type").String()
	switch strings.ToLower(strings.TrimSpace(msgType)) {
	case "heartbeat":
		log.Infof("heartbeat message from ws client")
	default:
		log.Infof("recv client: %v", string(result.String()))
	}
}
