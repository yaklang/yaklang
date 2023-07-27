package vulinboxAgentClient

import (
	"context"
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/vulinbox"
	"strconv"
	"strings"
)

type Client struct {
	wsClient *lowhttp.WebsocketClient

	// todo: use ttl map
	ackWaitMap *ttlcache.Cache
	sendBuf    chan []byte

	ctx     context.Context
	onClose func()
	cancel  func()
}

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
		sendBuf:    make(chan []byte, 1024),
		ackWaitMap: ttlcache.NewCache(),
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
		isTls = utils.IsTLSService(addr)
	}
	log.Info("start to create ws client to connect vulinbox/_/ws/agent")

	// prepare handshake packet
	wsPacket := lowhttp.ReplaceHTTPPacketHeader(handshakePacket, "Host", addr)
	fmt.Println(string(wsPacket))

	// connnect
	c.wsClient, err = lowhttp.NewWebsocketClient(wsPacket,
		lowhttp.WithWebsocketFromServerHandler(c.MessageMux),
		lowhttp.WithWebsocketTLS(isTls),
		lowhttp.WithWebsocketHost(host),
		lowhttp.WithWebsocketPort(port),
	)
	if err != nil {
		return nil, err
	}

	// serve
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.wsClient.StartFromServer()
	go c.sendLoop()
	log.Info("start to wait for vulinbox ws agent connected")

	// test ping
	ping := vulinbox.NewPingAction()
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
		case msg := <-c.sendBuf:
			if err := c.wsClient.WriteText(msg); err != nil {
				log.Errorf("cannot write text: %v", err)
				c.cancel()
			}
		}
	}
}

func (c *Client) MessageMux(bytes []byte) {
	ap := utils.MustUnmarshalJson[vulinbox.AgentProtocol](bytes)
	if ap == nil {
		log.Errorf("cannot unmarshal agent protocol: %v", string(bytes))
		return
	}

	log.Debugf(`vulinbox ws agent fetch message: %v`, ap.Action)

	switch ap.Action {
	case vulinbox.ActionAck:
		f, ok := c.ackWaitMap.Get(strconv.Itoa(int(ap.ActionId)))
		if !ok {
			log.Errorf("unkown ack:: %v", ap)
			return
		}

		err := f.(func([]byte) error)(bytes)
		if err != nil {
			log.Errorf("cannot handle ack: %v", err)
			return
		}

	case vulinbox.ActionDataback:
		databack := utils.MustUnmarshalJson[vulinbox.DatabackAction](bytes)
		if databack == nil {
			log.Errorf("cannot unmarshal databack action: %v", string(bytes))
			return
		}
		if databack.Type == "suricata" {
			if databack.Data != nil {
				log.Debugf("vulinbox ws agent fetch suricata databack: %v", databack.Data)
			}
		} else {
			log.Debugf("vulinbox ws agent fetch databack: %v", string(bytes))
		}
	}
}
