package vulinboxagentclient

import (
	"context"
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/vulinboxagentproto"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	wsClient *lowhttp.WebsocketClient

	ackWaitMap *ttlcache.Cache
	sendBuf    chan []byte

	databackHandler     map[string]func(any)
	databackHandlerLock sync.RWMutex

	ctx     context.Context
	onClose func()
	cancel  func()
}

const Expire = 10 * time.Second

var handshakePacket = []byte(`GET /_/ws/agent HTTP/1.1
Host: vulinbox:8787
Connection: Upgrade
Sec-WebSocket-Key: kpFli2X1YeW53YainWGFzA==
Sec-WebSocket-Version: 13
Upgrade: websocket
User-Agent: FeedbackStreamer/1.0

`)

type Option func(c *Client)

func Connect(addr string, options ...Option) (*Client, error) {
	// new client
	var c = &Client{
		sendBuf:         make(chan []byte, 1024),
		ackWaitMap:      ttlcache.NewCache(),
		databackHandler: map[string]func(any){},
	}
	c.ackWaitMap.SetTTL(Expire)
	c.ackWaitMap.SkipTtlExtensionOnHit(true)
	for _, option := range options {
		option(c)
	}

	// prepare addr
	addr = utils.AppendDefaultPort(addr, 8787)
	addr = strings.ReplaceAll(addr, "0.0.0.0", "127.0.0.1")
	addr = strings.ReplaceAll(addr, "[::]", "127.0.0.1")
	host, port, err := utils.ParseStringToHostPort(addr)
	if err != nil {
		return nil, utils.Errorf("cannot fetch host and port from addr: %s", err)
	}
	var isTls = port == 443
	if !isTls {
		isTls = netx.IsTLSService(addr)
	}
	log.Info("start to create ws client to connect vulinbox/_/ws/agent")

	// prepare handshake packet
	wsPacket := lowhttp.ReplaceHTTPPacketHeader(handshakePacket, "Host", addr)
	fmt.Println(string(wsPacket))

	// connnect
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.wsClient, err = lowhttp.NewWebsocketClient(wsPacket,
		lowhttp.WithWebsocketFromServerHandler(c.MessageMux),
		lowhttp.WithWebsocketTLS(isTls),
		lowhttp.WithWebsocketHost(host),
		lowhttp.WithWebsocketPort(port),
		lowhttp.WithWebsocketWithContext(c.ctx),
	)
	if err != nil {
		c.cancel()
		return nil, err
	}

	// serve
	c.wsClient.StartFromServer()
	go c.sendLoop()
	log.Info("start to wait for vulinbox ws agent connected")

	// test ping
	ping := vulinboxagentproto.NewPingAction()
	start := false
	c.Msg().Callback(func(_ []byte) error {
		start = true
		return nil
	}).Send(ping)

	// wait for ack
	if err := utils.Spinlock(10, func() bool {
		return start
	}); err != nil {
		c.cancel()
		return nil, fmt.Errorf("wait ack failed: %v", err)
	}

	log.Info("vulinbox ws agent connected")
	return c, nil
}

func (c *Client) Disconnect() {
	c.cancel()
}

func (c *Client) sendLoop() {
	defer c.onClose()
	for {
		select {
		case <-c.ctx.Done():
			c.wsClient.Stop()
			return
		case <-c.wsClient.Context.Done():
			c.cancel()
			return
		case msg := <-c.sendBuf:
			if err := c.wsClient.WriteText(msg); err != nil {
				log.Errorf("cannot write text: %v", err)
				c.cancel()
			}
		}
	}
}

func (c *Client) MessageMux(bytes []byte) {
	ap := utils.MustUnmarshalJson[vulinboxagentproto.AgentProtocol](bytes)
	if ap == nil {
		log.Errorf("cannot unmarshal agent protocol: %v", string(bytes))
		return
	}

	log.Debugf(`vulinbox ws agent fetch message: %v`, ap.Action)

	switch ap.Action {
	case vulinboxagentproto.ActionAck:
		f, ok := c.ackWaitMap.Get(strconv.Itoa(int(ap.ActionId)))
		if !ok {
			return
		}

		err := f.(func([]byte) error)(bytes)
		if err != nil {
			log.Errorf("cannot handle ack: %v", err)
			return
		}

	case vulinboxagentproto.ActionDataback:
		databack := utils.MustUnmarshalJson[vulinboxagentproto.DatabackAction](bytes)
		if databack == nil {
			log.Errorf("cannot unmarshal databack action: %v", string(bytes))
			return
		}
		log.Debugf("vulinbox ws agent fetch %s databack: %v", databack.Type, databack.Data)
		c.databackHandlerLock.RLock()
		defer c.databackHandlerLock.RUnlock()
		if f, ok := c.databackHandler[databack.Type]; ok {
			f(databack.Data)
		}
	}
}

// RegisterDataback register databack handler
// attention that no racing protect here, so you should register databack handler before connect
func (c *Client) RegisterDataback(kind string, f func(any)) {
	c.databackHandlerLock.Lock()
	defer c.databackHandlerLock.Unlock()
	c.databackHandler[kind] = f
}
