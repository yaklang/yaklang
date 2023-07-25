package lowhttp

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net/url"
	"strings"
	"time"
)

func ConnectVulinboxAgentEx(addr string, handler func(request []byte), onPing func(), onClose func()) (func(), error) {
	return ConnectVulinboxAgentRaw(addr, func(bytes []byte) {
		t := strings.ToLower(utils.ExtractMapValueString(bytes, "action"))
		log.Debugf(`vulinbox ws agent fetch message: %v`, t)
		switch t {
		case "ping":
			if onPing != nil {
				onPing()
			}
		case "databack":
			if utils.ExtractMapValueString(bytes, "type") == "http-request" {
				handler([]byte(utils.ExtractMapValueString(bytes, "data")))
			}
		}
	}, func() {
		if onClose != nil {
			onClose()
		}
		log.Infof("vulinbox agent: %v is closed", addr)
	})
}

func ConnectVulinboxAgent(addr string, handler func(request []byte), onPing ...func()) (func(), error) {
	return ConnectVulinboxAgentEx(addr, handler, func() {
		for _, i := range onPing {
			i()
		}
	}, nil)
}

func ConnectVulinboxAgentRaw(addr string, handler func([]byte), onClose func()) (func(), error) {

	addr = utils.AppendDefaultPort(addr, 8787)

	if addr == "" {
		addr = "ws://127.0.0.1:8787"
	}
	addr = strings.ReplaceAll(addr, "0.0.0.0", "127.0.0.1")
	addr = strings.ReplaceAll(addr, "[::]", "127.0.0.1")

	uri, err := url.ParseRequestURI(addr)
	if err != nil {
		return nil, err
	}

	log.Info("start to create ws client to connect vulinbox/_/ws/agent")
	wsPacket := ReplaceHTTPPacketHeader([]byte(`GET /_/ws/agent HTTP/1.1
Host: vuliobox:8787
Connection: Upgrade
Sec-WebSocket-Key: kpFli2X1YeW53YainWGFzA==
Sec-WebSocket-Version: 13
Upgrade: websocket
User-Agent: FeedbackStreamer/1.0

`), "Host", uri.Host)
	fmt.Println(string(wsPacket))
	var start = false
	client, err := NewWebsocketClient(wsPacket, WithWebsocketFromServerHandler(func(bytes []byte) {
		if !start {
			if utils.ExtractMapValueString(bytes, "action") == "ping" {
				start = true
			}
		}
		handler(bytes)
	}), WithWebsocketTLS(strings.HasPrefix(addr, "wss://") || strings.HasPrefix(addr, "https://")))
	if err != nil {
		return nil, err
	}
	client.StartFromServer()
	log.Info("start to wait for vulinbox ws agent connected")
	if utils.Spinlock(5, func() bool {
		return start
	}) != nil {
		client.Stop()
		return nil, utils.Errorf("vulinbox ws agent connect timeout")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		go func() {
			for {
				select {
				case <-ctx.Done():
					client.Stop()
					return
				default:
					client.WriteText([]byte(`{"action":"ping"}`))
					time.Sleep(time.Second)
				}
			}
		}()
		client.Wait()
		client.Stop()
		if onClose != nil {
			onClose()
		}
	}()
	log.Info("vulinbox ws agent connected")
	return cancel, nil
}
