package lowhttp

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"time"
)

func ConnectVulinboxAgentEx(addr string, handler func(request []byte), onPing func(), onClose func()) (func(), error) {
	return ConnectVulinboxAgentRaw(addr, func(bytes []byte) {
		t := strings.ToLower(utils.ExtractMapValueString(bytes, "type"))
		log.Debugf(`vulinbox ws agent fetch message: %v`, t)
		switch t {
		case "ping":
			if onPing != nil {
				onPing()
			}
		case "request":
			handler([]byte(utils.ExtractMapValueString(bytes, "request")))
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
	var cancel = func() {}

	if addr == "" {
		addr = "127.0.0.1:8787"
	}

	host, port, _ := utils.ParseStringToHostPort(addr)
	if port <= 0 {
		host = "127.0.0.1"
		port = 8787
	} else {
		addr = utils.HostPort(host, port)
		addr = strings.ReplaceAll(addr, "0.0.0.0", "127.0.0.1")
		addr = strings.ReplaceAll(addr, "[::]", "127.0.0.1")
	}

	log.Info("start to create ws client to connect vulinbox/_/ws/agent")
	wsPacket := ReplaceHTTPPacketHeader([]byte(`GET /_/ws/agent HTTP/1.1
Host: vuliobox:8787
Connection: Upgrade
Sec-WebSocket-Key: kpFli2X1YeW53YainWGFzA==
Sec-WebSocket-Version: 13
Upgrade: websocket
User-Agent: FeedbackStreamer/1.0

`), "Host", addr)
	fmt.Println(string(wsPacket))
	var start = false
	client, err := NewWebsocketClient(wsPacket, WithWebsocketFromServerHandler(func(bytes []byte) {
		if !start {
			if utils.ExtractMapValueString(bytes, "type") == "ping" {
				start = true
			}
		}
		handler(bytes)
	}))
	if err != nil {
		cancel()
		return cancel, err
	}
	client.StartFromServer()
	cancel = func() {
		client.Stop()
	}
	log.Info("start to wait for vulinbox ws agent connected")
	if utils.Spinlock(5, func() bool {
		return start
	}) != nil {
		cancel()
		return nil, utils.Errorf("vulinbox ws agent connect timeout")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					client.WriteText([]byte(`{"type":"ping"}`))
					time.Sleep(time.Second)
				}
			}
		}()
		client.Wait()
		cancel()
		if onClose != nil {
			onClose()
		}
	}()
	log.Info("vulinbox ws agent connected")
	return cancel, nil
}
