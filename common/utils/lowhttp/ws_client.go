package lowhttp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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
	Proxy                         []string
	TotalTimeout                  time.Duration
	TLS                           bool
	FromServerHandler             func([]byte)
	UpgradeResponseHandler        func(*http.Response, []byte, *WebsocketExtensions, error) []byte
	FromServerHandlerEx           func(*WebsocketClient, []byte, []*Frame)
	AllFrameHandler               func(*WebsocketClient, *Frame, []byte, func())
	DisableReassembly             bool
	Context                       context.Context
	cancel                        func()
	strictMode                    bool
	serverMode                    bool
	compressionMode               CompressionMode
	compressionConfigured         bool
	compressionOffer              string
	writeCompressionNoTakeover    bool
	writeCompressionNoTakeoverSet bool
	writeCompressionWindowBits    int

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
		config.compressionConfigured = true
		if b {
			config.compressionMode = EnableCompression
			config.compressionOffer = websocketPermessageDeflate
			config.writeCompressionNoTakeover = false
			config.writeCompressionNoTakeoverSet = false
		} else {
			config.compressionMode = DisableCompression
			config.compressionOffer = ""
			config.writeCompressionNoTakeover = false
			config.writeCompressionNoTakeoverSet = false
			config.writeCompressionWindowBits = 0
		}
	}
}

func WithWebsocketCompressionContextTakeover(b bool) WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.compressionConfigured = true
		if b {
			config.compressionMode = CompressionContextTakeover
			config.compressionOffer = websocketPermessageDeflate
			config.writeCompressionNoTakeover = false
			config.writeCompressionNoTakeoverSet = false
		} else {
			config.compressionMode = NoCompressionContextTakeover
			config.writeCompressionNoTakeover = true
			config.writeCompressionNoTakeoverSet = true
			config.compressionOffer = formatPermessageDeflateOffer(PermessageDeflateParameters{
				ClientNoContextTakeover: true,
				ServerNoContextTakeover: true,
			})
		}
	}
}

func WithWebsocketRFC7692FullCompression() WebsocketClientOpt {
	return func(config *WebsocketClientConfig) {
		config.compressionConfigured = true
		config.compressionMode = EnableCompression
		config.writeCompressionNoTakeover = true
		config.writeCompressionNoTakeoverSet = true
		config.compressionOffer = formatPermessageDeflateOffer(PermessageDeflateParameters{
			ClientNoContextTakeover: true,
			ClientMaxWindowBitsSet:  true,
			ClientMaxWindowBitsBare: true,
		})
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
	conn                          net.Conn
	fr                            *FrameReader
	fw                            *FrameWriter
	Request                       []byte
	Response                      []byte
	ResponseInstance              *http.Response
	StartOnce                     *sync.Once
	FromServerHandler             func([]byte)
	FromServerHandlerEx           func(*WebsocketClient, []byte, []*Frame)
	AllFrameHandler               func(*WebsocketClient, *Frame, []byte, func())
	DisableReassembly             bool
	Extensions                    *WebsocketExtensions
	Context                       context.Context
	cancel                        func()
	strictMode                    bool
	serverMode                    bool
	writeCompressionNoTakeover    bool
	writeCompressionNoTakeoverSet bool
	writeCompressionWindowBits    int

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
				if c.fr.dict != nil {
					c.fr.dict.close()
					c.fr.dict = nil
				}
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
					if isExpectedWebsocketReadError(err) {
						log.Debugf("[ws fr] connection closed: %s", err)
					} else {
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
					// RFC 6455 section 5.1: clients mask frames sent to servers,
					// while servers must not mask frames sent to clients.
					if frame.GetMask() != c.serverMode {
						c.WriteCloseEx(CloseProtocolError, "")
						return
					}

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
					hasDeflate := c.Extensions != nil && c.Extensions.IsDeflate
					if frame.RSV2() || frame.RSV3() || (rsv1 && !hasDeflate) {
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
						if isControl || frameType == ContinueMessage {
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
					if firstFragmentFrame == TextMessage {
						isFragmentedText := len(fragmentFrames) > 0 || !frame.FIN()
						if inCompressState {
							if frame.FIN() && !utf8.Valid(frame.data) {
								c.WriteCloseEx(CloseInvalidFramePayloadData, "")
								return
							}
						} else if isFragmentedText {
							remindBytesBuffer.Write(frame.data)
							if frame.FIN() {
								if !utf8.Valid(remindBytesBuffer.Bytes()) {
									c.WriteCloseEx(CloseInvalidFramePayloadData, "")
									return
								}
								remindBytesBuffer.Reset()
							} else if valid, _ := IsValidUTF8WithRemind(remindBytesBuffer.Bytes()); !valid {
								c.WriteCloseEx(CloseInvalidFramePayloadData, "")
								return
							}
						} else if !utf8.Valid(frame.data) {
							c.WriteCloseEx(CloseInvalidFramePayloadData, "")
							return
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
				if !isControl {
					fragmentFrames = append(fragmentFrames, frame)
				}

				reassembleMessage := !c.DisableReassembly || inCompressState
				if reassembleMessage && !isControl {
					if inFragment {
						if !inCompressState {
							plainTextBuffer.Write(plain)
							if frame.FIN() {
								plain = plainTextBuffer.Bytes()
								plainTextBuffer.Reset()
							}
						}
						// A compressed fragmented message is decompressed by FrameReader
						// only when its final continuation arrives. Do not expose the
						// intermediate compressed octets as application messages.
						if !frame.FIN() {
							continue
						}
					}
				}
				if allFrameHandler != nil {
					callbackFrame := frame
					if reassembleMessage && !isControl && len(fragmentFrames) > 1 {
						// Reassembly produces one complete message. Preserve the data
						// opcode from its first fragment instead of forwarding the final
						// continuation opcode.
						reassembledFrame := *frame
						reassembledFrame.SetOpcode(fragmentFrames[0].Type())
						callbackFrame = &reassembledFrame
					}
					shouldReturn := false
					allFrameHandler(c, callbackFrame, plain, func() {
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

				if !isControl && frame.FIN() {
					inCompressState = false
					inFragment = false
					fragmentFrames = fragmentFrames[:0]
				}
			}
		}()
	})
}

func (c *WebsocketClient) HasExtensions() bool {
	return c.Extensions != nil && len(c.Extensions.Extensions) > 0
}

func (c *WebsocketClient) writeFlateContextTakeover() bool {
	if c == nil || c.Extensions == nil || !c.Extensions.writeFlateContextTakeover(c.serverMode) {
		return false
	}
	return !c.writeCompressionNoTakeoverSet || !c.writeCompressionNoTakeover
}

func (c *WebsocketClient) writeFlateWindowBits() int {
	if c == nil {
		return websocketDefaultWindowBits
	}
	if c.Extensions == nil {
		return websocketDefaultWindowBits
	}
	negotiated := c.Extensions.writeFlateWindowBits(c.serverMode)
	if c.serverMode || c.Extensions.PermessageDeflate == nil || c.Extensions.PermessageDeflate.ClientMaxWindowBitsSet {
		return negotiated
	}
	if c.writeCompressionWindowBits >= 8 && c.writeCompressionWindowBits <= websocketDefaultWindowBits {
		return c.writeCompressionWindowBits
	}
	return negotiated
}

func isExpectedWebsocketReadError(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) || errors.Is(err, errInvalidWebsocketFrame)
}

func websocketHeaderContainsToken(headers http.Header, name, token string) bool {
	for _, value := range websocketHeaderValues(headers, name) {
		for _, current := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(current), token) {
				return true
			}
		}
	}
	return false
}

func validateWebsocketUpgradeResponse(requestRaw []byte, response *http.Response) error {
	if response == nil || response.StatusCode != http.StatusSwitchingProtocols {
		return utils.Error("websocket: expected HTTP 101 Switching Protocols")
	}
	if !websocketHeaderContainsToken(response.Header, "Upgrade", "websocket") {
		return utils.Error("websocket: response is missing Upgrade: websocket")
	}
	if !websocketHeaderContainsToken(response.Header, "Connection", "upgrade") {
		return utils.Error("websocket: response is missing Connection: Upgrade")
	}
	key := strings.TrimSpace(GetHTTPPacketHeader(requestRaw, "Sec-WebSocket-Key"))
	decodedKey, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decodedKey) != 16 {
		return utils.Error("websocket: request contains an invalid Sec-WebSocket-Key")
	}
	acceptValues := websocketHeaderValues(response.Header, "Sec-WebSocket-Accept")
	if len(acceptValues) != 1 || strings.TrimSpace(acceptValues[0]) != ComputeWebsocketAcceptKey(key) {
		return utils.Error("websocket: invalid Sec-WebSocket-Accept response")
	}

	selectedProtocols := websocketHeaderValues(response.Header, "Sec-WebSocket-Protocol")
	if len(selectedProtocols) == 0 {
		return nil
	}
	if len(selectedProtocols) != 1 || strings.Contains(selectedProtocols[0], ",") {
		return utils.Error("websocket: server selected multiple subprotocols")
	}
	selected := strings.TrimSpace(selectedProtocols[0])
	for _, offered := range strings.Split(GetHTTPPacketHeader(requestRaw, "Sec-WebSocket-Protocol"), ",") {
		if selected != "" && strings.TrimSpace(offered) == selected {
			return nil
		}
	}
	return utils.Errorf("websocket: server selected unoffered subprotocol %q", selected)
}

// ValidateWebsocketUpgradeResponse validates the server side of an RFC 6455
// opening handshake against the request bytes sent on the wire.
func ValidateWebsocketUpgradeResponse(requestRaw []byte, response *http.Response) error {
	return validateWebsocketUpgradeResponse(requestRaw, response)
}

func (c *WebsocketClient) Write(r []byte) error {
	return c.WriteText(r)
}

func (c *WebsocketClient) WriteEx(r []byte, frameTyp int) error {
	if err := c.fw.WriteEx(r, frameTyp, !c.serverMode); err != nil {
		return errors.Wrap(err, "write text frame failed")
	}

	if err := c.fw.Flush(); err != nil {
		return errors.Wrap(err, "flush failed")
	}
	return nil
}

func (c *WebsocketClient) WriteBinary(r []byte) error {
	if err := c.fw.WriteBinary(r, !c.serverMode); err != nil {
		return errors.Wrap(err, "write binary frame failed")
	}
	if err := c.fw.Flush(); err != nil {
		return errors.Wrap(err, "flush failed")
	}
	return nil
}

func (c *WebsocketClient) WriteText(r []byte) error {
	if err := c.fw.WriteText(r, !c.serverMode); err != nil {
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
	if !utf8.ValidString(message) {
		return utils.Error("websocket: close reason must be valid utf8")
	}
	if closeCode == 0 && message != "" {
		return utils.Error("websocket: close reason requires a close code")
	}
	payload := []byte(nil)
	if closeCode != 0 {
		payload = GetClosePayloadFromCloseCode(closeCode)
	}
	payload = append(payload, message...)
	if err := c.fw.WriteEx(payload, CloseMessage, !c.serverMode); err != nil {
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

	if config.compressionConfigured {
		if config.compressionOffer == "" {
			packet = DeleteHTTPPacketHeader(packet, "Sec-WebSocket-Extensions")
		} else {
			packet = ReplaceHTTPPacketHeader(packet, "Sec-WebSocket-Extensions", config.compressionOffer)
		}
	}

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
	requestExtensions := make(http.Header)
	if value := GetHTTPPacketHeader(requestRaw, "Sec-WebSocket-Extensions"); value != "" {
		requestExtensions.Set("Sec-WebSocket-Extensions", value)
	}
	if _, offers, offerErr := parsedWebsocketExtensions(requestExtensions, false); offerErr == nil {
		for _, offer := range offers {
			if !config.writeCompressionNoTakeoverSet && offer.ClientNoContextTakeover {
				config.writeCompressionNoTakeover = true
				config.writeCompressionNoTakeoverSet = true
			}
			if config.writeCompressionWindowBits == 0 && offer.ClientMaxWindowBitsSet && !offer.ClientMaxWindowBitsBare {
				config.writeCompressionWindowBits = offer.ClientMaxWindowBits
			}
		}
	}
	extensions, extensionErr := ValidateWebsocketExtensions(requestExtensions, rsp.Header)
	if extensionErr != nil {
		_ = conn.Close()
		return nil, extensionErr
	}
	responseRaw := responseBuffer.Bytes()
	if upgradeResponseHandler != nil {
		newResponseRaw := upgradeResponseHandler(rsp, responseRaw, extensions, nil)
		if !bytes.Equal(newResponseRaw, responseRaw) {
			responseRaw = newResponseRaw
			rsp, err = ParseBytesToHTTPResponse(responseRaw)
			if err != nil {
				return nil, utils.Errorf("parse fixed response failed: %s", err)
			}
			extensions, extensionErr = ValidateWebsocketExtensions(requestExtensions, rsp.Header)
			if extensionErr != nil {
				_ = conn.Close()
				return nil, extensionErr
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
	if config.strictMode {
		if err := validateWebsocketUpgradeResponse(requestRaw, rsp); err != nil {
			_ = conn.Close()
			return nil, err
		}
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
		conn:                          conn,
		fr:                            fr,
		fw:                            fw,
		Request:                       requestRaw,
		Response:                      responseRaw,
		ResponseInstance:              rsp,
		FromServerHandler:             config.FromServerHandler,
		FromServerHandlerEx:           config.FromServerHandlerEx,
		AllFrameHandler:               config.AllFrameHandler,
		DisableReassembly:             config.DisableReassembly,
		Extensions:                    extensions,
		Context:                       ctx,
		cancel:                        cancel,
		strictMode:                    config.strictMode,
		serverMode:                    config.serverMode,
		writeCompressionNoTakeover:    config.writeCompressionNoTakeover,
		writeCompressionNoTakeoverSet: config.writeCompressionNoTakeoverSet,
		writeCompressionWindowBits:    config.writeCompressionWindowBits,
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
		conn:                          conn,
		fr:                            fr,
		fw:                            fw,
		FromServerHandler:             config.FromServerHandler,
		FromServerHandlerEx:           config.FromServerHandlerEx,
		AllFrameHandler:               config.AllFrameHandler,
		DisableReassembly:             config.DisableReassembly,
		Extensions:                    ext,
		Context:                       ctx,
		cancel:                        cancel,
		strictMode:                    config.strictMode,
		serverMode:                    config.serverMode,
		writeCompressionNoTakeover:    config.writeCompressionNoTakeover,
		writeCompressionNoTakeoverSet: config.writeCompressionNoTakeoverSet,
		writeCompressionWindowBits:    config.writeCompressionWindowBits,
	}

	fr.SetWebsocketClient(client)
	fw.SetWebsocketClient(client)

	return client
}
