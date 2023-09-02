package lowhttp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"
)

type WebsocketClientConfig struct {
	Proxy             string
	TotalTimeout      time.Duration
	TLS               bool
	FromServerHandler func([]byte)
	Context           context.Context
	cancel            func()

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

type WebsocketClient struct {
	conn              net.Conn
	fr                *FrameReader
	fw                *FrameWriter
	Request           []byte
	Response          []byte
	FromServerOnce    *sync.Once
	FromServerHandler func([]byte)
	Context           context.Context
	cancel            func()

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
			frame *Frame
			err   error
		)
		c.FromServerOnce.Do(func() {
			defer func() {
				c.cancel()
			}()

			for {
				select {
				case <-c.Context.Done():
					_ = c.conn.Close()
					return
				default:
				}

				frame, err = c.fr.ReadFrame()
				if err != nil {
					log.Errorf("[ws fr]read frame failed: %s", err)
					return
				}

				if frame.Type() == PingMessage {
					c.WritePong(frame.rawPayloadData)
					continue
				}

				plain := WebsocketFrameToData(frame)
				f := c.FromServerHandler
				if f != nil {
					f(plain)
				} else {
					raw, data := frame.Bytes()
					fmt.Printf("websocket receive: %s\n verbose: %v", strconv.Quote(string(raw)), strconv.Quote(string(data)))
				}
			}
		})
	}()
}

func (c *WebsocketClient) Write(r []byte) error {
	return c.WriteText(r)
}

func (c *WebsocketClient) WriteBinary(r []byte) error {
	if len(r) <= 0 {
		return nil
	}

	if err := c.fw.WriteBinary(r, true); err != nil {
		return errors.Wrap(err, "write pong frame failed")
	}
	if err := c.fw.Flush(); err != nil {
		return errors.Wrap(err, "flush failed")
	}
	return nil
}

func (c *WebsocketClient) WriteText(r []byte) error {
	if len(r) <= 0 {
		return nil
	}

	if err := c.fw.WriteText(r, true); err != nil {
		return errors.Wrap(err, "write pong frame failed")
	}
	if err := c.fw.Flush(); err != nil {
		return errors.Wrap(err, "flush failed")
	}
	return nil
}

func (c *WebsocketClient) WritePong(r []byte) error {
	if len(r) <= 0 {
		return nil
	}

	if err := c.fw.WritePong(r, true); err != nil {
		return errors.Wrap(err, "write pong frame failed")
	}

	if err := c.fw.Flush(); err != nil {
		return errors.Wrap(err, "flush failed")
	}
	return nil
}

func (c *WebsocketClient) WriteClose() error {
	if err := c.fw.write(nil, CloseMessage, true); err != nil {
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
	if config.Proxy == "" && netx.GetProxyFromEnv() != "" {
		config.Proxy = netx.GetProxyFromEnv()
	}

	var port = config.Port
	var host = config.Host

	// 修正端口
	var noFixPort = false
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
	var addr = utils.HostPort(host, port)
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
	var responseRaw bytes.Buffer
	rsp, err := http.ReadResponse(bufio.NewReader(io.TeeReader(conn, &responseRaw)), nil)
	if err != nil {
		return nil, utils.Errorf("read response failed: %s", err)
	}
	//if rsp.StatusCode != 101 {
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
	return &WebsocketClient{
		conn:              conn,
		fr:                NewFrameReader(conn, isDeflate),
		fw:                NewFrameWriter(conn, isDeflate),
		Request:           requestRaw,
		Response:          responseRaw.Bytes(),
		FromServerHandler: config.FromServerHandler,
		Context:           ctx,
		cancel:            cancel,
	}, nil
}
