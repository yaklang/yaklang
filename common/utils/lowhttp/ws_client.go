package lowhttp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/pkg/errors"
)

type WebsocketClientConfig struct {
	Proxy               string
	TotalTimeout        time.Duration
	TLS                 bool
	FromServerHandler   func([]byte)
	FromServerHandlerEx func(*WebsocketClient, []byte, []*Frame)
	Context             context.Context
	cancel              func()
	strictMode          bool

	// Host Port
	Host string
	Port int
}

type WebsocketClientOpt func(config *WebsocketClientConfig)

func WithWebsocketProxy(t string) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.Proxy = t
	}
}

func WithWebsocketHost(t string) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.Host = t
	}
}

func WithWebsocketPort(t int) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.Port = t
	}
}

func WithWebsocketFromServerHandler(f func([]byte)) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.FromServerHandler = f
	}
}

func WithWebsocketFromServerHandlerEx(f func(*WebsocketClient, []byte, []*Frame)) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.FromServerHandlerEx = f
	}
}

func WithWebsocketWithContext(ctx context.Context) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.Context, config.cancel = context.WithCancel(ctx)
	}
}

func WithWebsocketTotalTimeout(t float64) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.TotalTimeout = utils.FloatSecondDuration(t)
		if config.TotalTimeout <= 0 {
			config.TotalTimeout = time.Hour
		}
	}
}

func WithWebsocketTLS(t bool) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.TLS = t
	}
}

func WithWebsocketStrictMode(b bool) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.strictMode = b
	}
}

type WebsocketClient struct {
	conn                net.Conn
	fr                  *FrameReader
	fw                  *FrameWriter
	Request             []byte
	Response            []byte
	FromServerOnce      *sync.Once
	FromServerHandler   func([]byte)
	FromServerHandlerEx func(*WebsocketClient, []byte, []*Frame)
	Extensions          []string
	Context             context.Context
	cancel              func()
	strictMode          bool

	// websocket扩展
	// isDeflate bool
}

func (c *WebsocketClient) Wait() {
	if c.Context == nil {
		return
	}
	select {
	case <-c.Context.Done():
	}
}

func (c *WebsocketClient) Stop() {
	if c == nil {
		return
	}
	err := c.WriteClose()
	if err != nil {
		log.Errorf("[ws]write close failed: %s", err)
	}
	if c.cancel == nil {
		return
	}
	c.cancel()
}

func (c *WebsocketClient) StartFromServer() {
	if c.FromServerOnce == nil {
		c.FromServerOnce = new(sync.Once)
	}
	go func() {
		var (
			plainTextBuffer   bytes.Buffer
			remindBytesBuffer bytes.Buffer // e.g. fragmented text message, utf8 remind buffer
			fragmentFrames    = make([]*Frame, 0, 1)
			inFragment        = false
		)
		c.FromServerOnce.Do(func() {
			defer func() {
				c.cancel()
				c.conn.Close()
			}()

			for {
				select {
				case <-c.Context.Done():
					return
				default:
				}

				frame, err := c.fr.ReadFrame()
				if err != nil {
					if !errors.Is(err, io.EOF) {
						log.Errorf("[ws fr]read frame failed: %s", err)
					}
					return
				}
				frameType, isControl := frame.Type(), frame.IsControl()
				// frame.Show()

				// strict mode
				if c.strictMode {
					// rfc6455: 5.5
					// All control frames MUST have a payload length of 125 bytes or less and MUST NOT be fragmented.
					if isControl && (len(frame.payload) > 125 || !frame.FIN()) {
						c.WriteCloseEx(CloseProtocolError, "")
						return
					}

					// rfc6455: 5.2
					// RSV1, RSV2, RSV3:  1 bit each
					// MUST be 0 unless an extension is negotiated that defines meanings
					// for non-zero values.  If a nonzero value is received and none of
					// the negotiated extensions defines the meaning of such a nonzero
					// value, the receiving endpoint MUST _Fail the WebSocket
					// Connection_.
					if frame.HasRsv() && !c.HasExtensions() {
						c.WriteCloseEx(CloseProtocolError, "")
						return
					}

					// rfc6455: 5.2
					// %x3-7 are reserved for further non-control frames
					// %xB-F are reserved for further control frames
					if frame.IsReservedType() {
						c.WriteCloseEx(CloseProtocolError, "")
						return
					}

					// rfc6455: 7.4
					if frameType == CloseMessage && !frame.IsValidCloseCode() {
						c.WriteCloseEx(CloseProtocolError, "")
						return
					}

					// rfc6455: 5.5.1
					// Following the 2-byte integer, the body MAY contain UTF-8-encoded data with value /reason/, the interpretation of which is not defined by this specification.
					if frameType == CloseMessage && !utf8.Valid(frame.data) {
						c.WriteCloseEx(CloseInvalidFramePayloadData, "")
						return
					}

					// rfc6455: 5.6
					// The "Payload data" is text data encoded as UTF-8.  Note that a particular text frame might include a partial UTF-8 sequence; however, the whole message MUST contain valid UTF-8.  Invalid UTF-8 in reassembled messages is handled as described in Section 8.1.
					// continue frame should be the same type as the first frame
					firstFragmentFrame := frameType
					if len(fragmentFrames) > 0 && frameType == ContinueMessage {
						firstFragmentFrame = fragmentFrames[0].Type()
					}
					if firstFragmentFrame == TextMessage {
						// fin message so check the whole message payload
						if frame.FIN() {
							remindBytesBuffer.Reset()
							if !utf8.Valid(frame.data) {
								c.WriteCloseEx(CloseInvalidFramePayloadData, "")
								return
							}
						} else {
							// fragmented message so maybe the payload is not complete, compatibility check
							remindBytesBuffer.Write(frame.data)
							bytes := remindBytesBuffer.Bytes()
							valid, remindSize := IsValidUTF8WithRemind(bytes)
							if !valid {
								c.WriteCloseEx(CloseInvalidFramePayloadData, "")
								return
							} else if remindSize != -1 {
								remindBytesBuffer.Next(len(bytes) - remindSize)
							}
						}
					}
				}

				// rfc6455: 5.4
				// A fragmented message consists of a single frame
				// with the FIN bit clear and an opcode other than 0, followed by zero or more frames  with the FIN bit clear and the opcode set to 0, and terminated by a single frame with the FIN bit set and an opcode of 0.
				// 1.     FIN = 0 && opcode >  0
				// 2-n:   FIN = 0 && opcode == 0
				// final: FIN = 1 && opcode == 0
				validFragmentFrame := false
				if !inFragment {
					// first frame of a fragmented message
					validFragmentFrame = frameType != ContinueMessage
					inFragment = !frame.FIN() && frameType != ContinueMessage
				} else {
					// Control frames MAY be injected in the middle of a fragmented message.  Control frames themselves MUST NOT be fragmented.
					// So no control frame in the middle of a fragmented message is invalid
					validFragmentFrame = isControl || frameType == ContinueMessage
				}

				if !validFragmentFrame {
					c.WriteCloseEx(CloseProtocolError, "")
					return
				}

				// control frame handle first
				switch frameType {
				case CloseMessage:
					closeCode := CloseNormalClosure
					if frame.closeCode != nil {
						closeCode = int(*frame.closeCode)
					}
					log.Debugf("Websocket close status code: %d", closeCode)
					c.WriteClose()
					return
				case PingMessage:
					c.WritePong(frame.data)
					continue
				}

				plain := WebsocketFrameToData(frame)
				fragmentFrames = append(fragmentFrames, frame)
				if inFragment {
					plainTextBuffer.Write(plain)
					if !frame.FIN() { // continue to read next frame
						continue
					}
					plain = plainTextBuffer.Bytes()
					plainTextBuffer.Reset()
				}
				inFragment = false

				if frame.FIN() {
					handler := c.FromServerHandler
					handlerEx := c.FromServerHandlerEx
					if !isControl {
						if handler != nil {
							handler(plain)
						} else if handlerEx != nil {
							handlerEx(c, plain, fragmentFrames)
						} else {
							raw, data := frame.Bytes()
							fmt.Printf("websocket receive: %s\n verbose: %v", strconv.Quote(string(raw)), strconv.Quote(string(data)))
						}
					}

					fragmentFrames = fragmentFrames[:0]
				}

			}
		})
	}()
}

func (c *WebsocketClient) HasExtensions() bool {
	return len(c.Extensions) > 0
}

func (c *WebsocketClient) Write(r []byte) error {
	return c.WriteText(r)
}

func (c *WebsocketClient) WriteEx(r []byte, frameTyp int) error {
	if err := c.fw.write(r, frameTyp, true); err != nil {
		return errors.Wrap(err, "write text frame failed")
	}

	if err := c.fw.Flush(); err != nil {
		return errors.Wrap(err, "flush failed")
	}
	return nil
}

func (c *WebsocketClient) WriteBinary(r []byte) error {
	if err := c.fw.WriteBinary(r, true); err != nil {
		return errors.Wrap(err, "write binary frame failed")
	}
	if err := c.fw.Flush(); err != nil {
		return errors.Wrap(err, "flush failed")
	}
	return nil
}

func (c *WebsocketClient) WriteText(r []byte) error {
	if err := c.fw.WriteText(r, true); err != nil {
		return errors.Wrap(err, "write text frame failed")
	}
	if err := c.fw.Flush(); err != nil {
		return errors.Wrap(err, "flush failed")
	}
	return nil
}

func (c *WebsocketClient) WritePong(r []byte) error {
	if err := c.fw.WritePong(r, true); err != nil {
		return errors.Wrap(err, "write pong frame failed")
	}

	if err := c.fw.Flush(); err != nil {
		return errors.Wrap(err, "flush failed")
	}
	return nil
}

func (c *WebsocketClient) WriteClose() error {
	data := FormatCloseMessage(CloseNormalClosure, "")
	if err := c.fw.write(data, CloseMessage, true); err != nil {
		return errors.Wrap(err, "write close frame failed")
	}
	if err := c.fw.Flush(); err != nil {
		return errors.Wrap(err, "flush failed")
	}
	return nil
}

func (c *WebsocketClient) WriteCloseEx(closeCode int, message string) error {
	if c.strictMode && len(message) > 125 {
		message = message[:125]
	}

	data := FormatCloseMessage(closeCode, message)
	if err := c.fw.write(data, CloseMessage, true); err != nil {
		return errors.Wrap(err, "write close frame failed")
	}
	if err := c.fw.Flush(); err != nil {
		return errors.Wrap(err, "flush failed")
	}
	return nil
}

func NewWebsocketClient(packet []byte, opt ...WebsocketClientOpt) (*WebsocketClient, error) {
	config := &WebsocketClientConfig{TotalTimeout: time.Hour}
	for _, p := range opt {
		p(config)
	}

	port := config.Port
	host := config.Host

	// 修正端口
	noFixPort := false
	if config.Port <= 0 {
		if config.TLS {
			config.Port = 443
		} else {
			config.Port = 80
		}
	} else {
		noFixPort = true
	}

	urlIns, err := ExtractURLFromHTTPRequestRaw(packet, config.TLS)
	if err != nil {
		return nil, utils.Errorf("extract url from request failed: %s", err)
	}

	// 修正host
	if config.Host == "" {
		var newPort int
		host, newPort, _ = utils.ParseStringToHostPort(urlIns.String())
		if !noFixPort && newPort > 0 {
			port = newPort
		}
		if host == "" {
			host = urlIns.String()
		}
	}

	// 获取连接
	addr := utils.HostPort(host, port)
	var conn net.Conn
	if config.TLS {
		conn, err = netx.DialTLSTimeout(10*time.Second, addr, nil, config.Proxy)
		if err != nil {
			return nil, utils.Errorf("dial tls-conn failed: %s", err)
		}
	} else {
		conn, err = netx.DialTCPTimeout(10*time.Second, addr, config.Proxy)
		if err != nil {
			return nil, utils.Errorf("dial conn failed: %s", err)
		}
	}

	// 判断websocket扩展
	requestRaw := FixHTTPRequest(packet)
	req, err := ParseBytesToHttpRequest(requestRaw)
	if err != nil {
		return nil, utils.Errorf("parse request failed: %s", err)
	}

	// 如果请求存在permessage-deflate扩展则设置isDeflate
	isDeflate := IsPermessageDeflate(req.Header)

	// 发送请求
	_, err = conn.Write(requestRaw)
	if err != nil {
		return nil, utils.Errorf("write conn[ws] failed: %s", err)
	}

	// 接收响应并判断
	var responseBuffer bytes.Buffer
	bufioReader := bufio.NewReaderSize(io.TeeReader(conn, &responseBuffer), 4096)
	rsp, err := utils.ReadHTTPResponseFromBufioReader(bufioReader, req)
	if err != nil {
		return nil, utils.Errorf("read response failed: %s", err)
	}

	responseRaw := responseBuffer.Bytes()

	remindBytes := make([]byte, bufioReader.Buffered())
	// write buffered data to remindBuffer
	bufioReader.Read(remindBytes)

	if rsp.StatusCode != 101 && rsp.StatusCode != 200 {
		return nil, utils.Errorf("upgrade websocket failed(101 switch protocols failed): %s", rsp.Status)
	}
	// 如果响应中不存在permessage-deflate扩展则要反设置isDeflate
	serverSupportDeflate := IsPermessageDeflate(rsp.Header)

	// 当服务端不支持permessage-deflate扩展时，客户端也不应该使用
	if !serverSupportDeflate && isDeflate {
		isDeflate = false
	}

	var ctx context.Context
	var cancel func()
	if config.Context == nil {
		ctx, cancel = context.WithTimeout(context.Background(), config.TotalTimeout)
	} else {
		ctx, cancel = config.Context, config.cancel
	}

	if cancel == nil {
		ctx, cancel = context.WithCancel(ctx)
	}
	fr := NewFrameReader(io.MultiReader(bytes.NewBuffer(remindBytes), conn), isDeflate)
	client := &WebsocketClient{
		conn:                conn,
		fr:                  fr,
		fw:                  NewFrameWriter(conn, isDeflate),
		Request:             requestRaw,
		Response:            responseRaw,
		FromServerHandler:   config.FromServerHandler,
		FromServerHandlerEx: config.FromServerHandlerEx,
		Extensions:          GetWebsocketExtensions(rsp.Header),
		Context:             ctx,
		cancel:              cancel,
		strictMode:          config.strictMode,
	}
	fr.SetWebsocketClient(client)

	return client, nil
}
