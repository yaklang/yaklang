package lowhttp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/pkg/errors"
)

type CompressionMode uint8

const (
	DisableCompression CompressionMode = 1 + iota
	EnableCompression
	CompressionContextTakeover
	NoCompressionContextTakeover
)

type WebsocketClientConfig struct {
	Proxy                  []string
	TotalTimeout           time.Duration
	TLS                    bool
	FromServerHandler      func([]byte)
	UpgradeResponseHandler func(*http.Response, []byte, *WebsocketExtensions, error) []byte
	FromServerHandlerEx    func(*WebsocketClient, []byte, []*Frame)
	AllFrameHandler        func(*WebsocketClient, *Frame, []byte, func())
	DisableReassembly      bool
	Context                context.Context
	cancel                 func()
	strictMode             bool
	serverMode             bool
	compressionMode        CompressionMode

	// Host Port
	Host string
	Port int
}

type WebsocketClientOpt func(config *WebsocketClientConfig)

func WithWebsocketProxy(t ...string) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.Proxy = t
	}
}

func WithWebsocketServerMode(b bool) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.serverMode = b
	}
}

func WithWebsocketDisableReassembly(b bool) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.DisableReassembly = b
	}
}

func WithWebsocketCompress(b bool) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		if b {
			config.compressionMode = EnableCompression
		} else {
			config.compressionMode = DisableCompression
		}
	}
}

func WithWebsocketCompressionContextTakeover(b bool) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		if b {
			config.compressionMode = CompressionContextTakeover
		} else {
			config.compressionMode = NoCompressionContextTakeover
		}
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

func WithWebsocketUpgradeResponseHandler(f func(*http.Response, []byte, *WebsocketExtensions, error) []byte) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.UpgradeResponseHandler = f
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

func WithWebsocketAllFrameHandler(f func(*WebsocketClient, *Frame, []byte, func())) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.AllFrameHandler = f
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
	ResponseInstance    *http.Response
	StartOnce           *sync.Once
	FromServerHandler   func([]byte)
	FromServerHandlerEx func(*WebsocketClient, []byte, []*Frame)
	AllFrameHandler     func(*WebsocketClient, *Frame, []byte, func())
	DisableReassembly   bool
	Extensions          *WebsocketExtensions
	Context             context.Context
	cancel              func()
	strictMode          bool
	serverMode          bool

	// websocket扩展
	// isDeflate bool
}

func (c *WebsocketClient) Wait() {
	if c.StartOnce == nil {
		panic("call Wait before Start")
	}
	if c.Context == nil {
		return
	}
	select {
	case <-c.Context.Done():
	}
}

func (c *WebsocketClient) WaitChannel() <-chan struct{} {
	defaultChan := make(chan struct{})
	defer close(defaultChan)
	if c.StartOnce == nil || c.Context == nil {
		return defaultChan
	}
	return c.Context.Done()
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

func (c *WebsocketClient) Start() {
	if c.StartOnce == nil {
		c.StartOnce = new(sync.Once)
	}

	c.StartOnce.Do(func() {
		fromServerHandler := c.FromServerHandler
		fromServerHandlerEx := c.FromServerHandlerEx
		allFrameHandler := c.AllFrameHandler
		go func() {
			var (
				plainTextBuffer             bytes.Buffer
				remindBytesBuffer           bytes.Buffer // e.g. fragmented text message, utf8 remind buffer
				fragmentFrames              = make([]*Frame, 0, 1)
				inFragment, inCompressState = false, false
			)
			defer func() {
				c.cancel()
				c.conn.Close()
				c.fr.putFlateReader()
				c.fw.putFlateWriter()
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
				frameType, isControl, rsv1 := frame.Type(), frame.IsControl(), frame.RSV1()
				if rsv1 {
					inCompressState = true
				}
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
					// RSV1, RSV2, RSV3: 1 bit each
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

					// rfc7692 6.1
					// An endpoint MUST NOT set the "Per-Message Compressed" bit of control frames and non-first fragments of a data message. An endpoint receiving such a frame MUST _Fail the WebSocket Connection_.
					if rsv1 {
						if isControl || (frameType == ContinueMessage && len(fragmentFrames) > 0) {
							c.WriteCloseEx(CloseProtocolError, "")
							return
						}
					}

					// rfc6455: 5.6
					// The "Payload data" is text data encoded as UTF-8.  Note that a particular text frame might include a partial UTF-8 sequence; however, the whole message MUST contain valid UTF-8.  Invalid UTF-8 in reassembled messages is handled as described in Section 8.1.
					// continue frame should be the same type as the first frame
					firstFragmentFrame := frameType
					if len(fragmentFrames) > 0 && frameType == ContinueMessage {
						firstFragmentFrame = fragmentFrames[0].Type()
					}
					// rfc7692 6.1
					// The payload data portion in frames generated by a PMCE is not subject to the constraints for the original data type.  For example, the concatenation of the output data corresponding to the application data portion of frames of a compressed text message is not required to be valid UTF-8.  At the receiver, the payload data portion after decompression is subject to the constraints for the original data type again.
					if !inCompressState && !rsv1 && firstFragmentFrame == TextMessage {
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
					validFragmentFrame = frameType != ContinueMessage
					// first frame of a fragmented message
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

				// reassembly
				plain := frame.data
				fragmentFrames = append(fragmentFrames, frame)

				if !c.DisableReassembly && !isControl {
					// only for un-permessage-deflate frame
					if inFragment && !inCompressState {
						plainTextBuffer.Write(plain)
						if !frame.FIN() { // continue to read next frame
							continue
						}
						plain = plainTextBuffer.Bytes()
						plainTextBuffer.Reset()
					}
				}
				if allFrameHandler != nil {
					shouldReturn := false
					allFrameHandler(c, frame, plain, func() {
						shouldReturn = true
					})
					if shouldReturn {
						log.Debugf("[ws] call shutdown, return")
						return
					}
				} else {
					// control frame handle first
					switch frameType {
					case CloseMessage:
						c.WriteClose()
						return
					case PingMessage:
						c.WritePong(frame.data, !c.serverMode)
						continue
					}

					if !frame.FIN() {
						continue
					}
					inCompressState = false
					inFragment = false

					if !isControl {
						if fromServerHandler != nil {
							fromServerHandler(plain)
						} else if fromServerHandlerEx != nil {
							fromServerHandlerEx(c, plain, fragmentFrames)
						} else {
							raw, data := frame.Bytes()
							fmt.Printf("websocket receive: %s\n verbose: %v", strconv.Quote(string(raw)), strconv.Quote(string(data)))
						}
					}
				}

				fragmentFrames = fragmentFrames[:0]
			}
		}()
	})
}

func (c *WebsocketClient) HasExtensions() bool {
	return len(c.Extensions.Extensions) > 0
}

func (c *WebsocketClient) Write(r []byte) error {
	return c.WriteText(r)
}

func (c *WebsocketClient) WriteEx(r []byte, frameTyp int) error {
	if err := c.fw.WriteEx(r, frameTyp, true); err != nil {
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

func (c *WebsocketClient) WriteDirect(fin, flate bool, opcode int, mask bool, data []byte) error {
	_, err := c.fw.WriteDirect(fin, flate, opcode, mask, data)
	return err
}

func (c *WebsocketClient) WritePong(r []byte, masked bool) error {
	if err := c.fw.WritePong(r, masked); err != nil {
		return errors.Wrap(err, "write pong frame failed")
	}

	if err := c.fw.Flush(); err != nil {
		return errors.Wrap(err, "flush failed")
	}
	return nil
}

func (c *WebsocketClient) WriteClose() error {
	return c.WriteCloseEx(CloseNormalClosure, "")
}

func (c *WebsocketClient) WriteCloseEx(closeCode int, message string) error {
	if err := c.fw.WriteEx(GetClosePayloadFromCloseCode(closeCode), CloseMessage, true); err != nil {
		return errors.Wrap(err, "write close frame failed")
	}
	if err := c.fw.Flush(); err != nil {
		return errors.Wrap(err, "flush failed")
	}
	log.Debugf("[ws] write close frame: %d %s", closeCode, message)
	return nil
}

func (c *WebsocketClient) Close() error {
	c.cancel()
	return c.conn.Close()
}

func NewWebsocketClientByUpgradeRequest(req *http.Request, opt ...WebsocketClientOpt) (*WebsocketClient, error) {
	packet, err := utils.DumpHTTPRequest(req, true)
	if err != nil {
		return nil, utils.Wrap(err, "dump websocket first upgrade request error")
	}
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

	// 扩展压缩选项
	if config.compressionMode > 0 {
		value, valid := "", true
		switch config.compressionMode {
		case EnableCompression, NoCompressionContextTakeover:
			value = "permessage-deflate"
		case CompressionContextTakeover:
			value = "permessage-deflate; server_no_context_takeover; client_no_context_takeover"
		case DisableCompression:
		default:
			valid = false
		}
		// 修改websocket扩展选项请求头
		if valid {
			if value == "" {
				packet = DeleteHTTPPacketHeader(packet, "Sec-WebSocket-Extensions")
			} else {
				packet = ReplaceHTTPPacketHeader(packet, "Sec-WebSocket-Extensions", value)
			}
		}
	}
	// 过滤client_max_window_bits，因为这个选项不支持
	websocketExtensions := GetHTTPPacketHeader(packet, "Sec-WebSocket-Extensions")
	filtered := lo.FilterMap(strings.Split(websocketExtensions, ";"), func(s string, _ int) (string, bool) {
		trimed := strings.TrimSpace(s)
		if trimed == "" || strings.Contains(trimed, "client_max_window_bits") {
			return "", false
		}
		return trimed, true
	})
	packet = ReplaceHTTPPacketHeader(packet, "Sec-WebSocket-Extensions", strings.Join(filtered, "; "))

	// 获取连接
	addr := utils.HostPort(host, port)
	var conn net.Conn
	if config.TLS {
		conn, err = netx.DialTLSTimeout(30*time.Second, addr, nil, config.Proxy...)
		if err != nil {
			return nil, utils.Errorf("dial tls-conn failed: %s", err)
		}
	} else {
		conn, err = netx.DialTCPTimeout(30*time.Second, addr, config.Proxy...)
		if err != nil {
			return nil, utils.Errorf("dial conn failed: %s", err)
		}
	}

	// 判断websocket扩展
	requestRaw := FixHTTPRequest(packet)
	// 发送请求
	_, err = conn.Write(requestRaw)
	if err != nil {
		return nil, utils.Errorf("write conn[ws] failed: %s", err)
	}

	// 接收响应并判断
	var responseBuffer bytes.Buffer
	upgradeResponseHandler := config.UpgradeResponseHandler
	bufioReader := bufio.NewReaderSize(io.TeeReader(conn, &responseBuffer), 4096)
	rsp, err := utils.ReadHTTPResponseFromBufioReader(bufioReader, req)
	if err != nil {
		if upgradeResponseHandler != nil {
			upgradeResponseHandler(nil, nil, nil, err)
		}
		return nil, utils.Errorf("read response failed: %s", err)
	}
	extensions := GetWebsocketExtensions(rsp.Header)
	responseRaw := responseBuffer.Bytes()
	if upgradeResponseHandler != nil {
		newResponseRaw := upgradeResponseHandler(rsp, responseRaw, extensions, nil)
		if !bytes.Equal(newResponseRaw, responseRaw) {
			responseRaw = newResponseRaw
			rsp, err = ParseBytesToHTTPResponse(responseRaw)
			if err != nil {
				return nil, utils.Errorf("parse fixed response failed: %s", err)
			}
		}
	}

	remindBytes := make([]byte, bufioReader.Buffered())
	if len(remindBytes) > 0 {
		// write buffered data to remindBuffer
		bufioReader.Read(remindBytes)
	}

	if rsp.StatusCode != 101 {
		return nil, utils.Errorf("upgrade websocket failed(101 switch protocols failed): %s", rsp.Status)
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
	fr := NewFrameReader(io.MultiReader(bytes.NewBuffer(remindBytes), conn), extensions.IsDeflate)
	fw := NewFrameWriter(conn, extensions.IsDeflate)

	client := &WebsocketClient{
		conn:                conn,
		fr:                  fr,
		fw:                  fw,
		Request:             requestRaw,
		Response:            responseRaw,
		ResponseInstance:    rsp,
		FromServerHandler:   config.FromServerHandler,
		FromServerHandlerEx: config.FromServerHandlerEx,
		AllFrameHandler:     config.AllFrameHandler,
		DisableReassembly:   config.DisableReassembly,
		Extensions:          extensions,
		Context:             ctx,
		cancel:              cancel,
		strictMode:          config.strictMode,
		serverMode:          config.serverMode,
	}
	fr.SetWebsocketClient(client)
	fw.SetWebsocketClient(client)

	return client, nil
}

func NewWebsocketClient(packet []byte, opt ...WebsocketClientOpt) (*WebsocketClient, error) {
	req, err := ParseBytesToHttpRequest(packet)
	if err != nil {
		return nil, utils.Errorf("parse http request failed: %v", err)
	}
	return NewWebsocketClientByUpgradeRequest(req, opt...)
}

func NewWebsocketClientIns(conn net.Conn, fr *FrameReader, fw *FrameWriter, ext *WebsocketExtensions, opts ...WebsocketClientOpt) *WebsocketClient {
	config := &WebsocketClientConfig{TotalTimeout: time.Hour}

	for _, p := range opts {
		p(config)
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

	client := &WebsocketClient{
		conn:                conn,
		fr:                  fr,
		fw:                  fw,
		FromServerHandler:   config.FromServerHandler,
		FromServerHandlerEx: config.FromServerHandlerEx,
		AllFrameHandler:     config.AllFrameHandler,
		DisableReassembly:   config.DisableReassembly,
		Extensions:          ext,
		Context:             ctx,
		cancel:              cancel,
		strictMode:          config.strictMode,
		serverMode:          config.serverMode,
	}

	fr.SetWebsocketClient(client)
	fw.SetWebsocketClient(client)

	return client
}
