package crep

import (
	"bufio"
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"net"
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

	if lowhttp.GetHTTPRequestQueryParam(raw, "token") != w.Token {
		return utils.Error("token is not right")
	}

	log.Infof("ws controller token is right handshake is ok!")
	// ws response
	if _, err := bw.WriteString("HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n\r\n"); err != nil {
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
			w.onMessage(utils.InterfaceToMapInterface(frame.GetData()))
		}
	}
}

func (w *WebsocketController) onMessage(i map[string]any) {
	spew.Dump(i)
}
