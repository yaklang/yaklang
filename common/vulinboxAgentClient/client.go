package vulinboxAgentClient

import (
	"context"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/vulinbox"
	"strings"
	"sync"
	"time"
)

var AckWaitMap sync.Map

func WaitNow(id uint32) ([]byte, error) {
	var msg chan []byte
	AckWaitMap.Store(id, func(data []byte) error {
		msg <- data
		return nil
	})
	t := time.NewTimer(time.Second * 10)
	select {
	case <-t.C:
		return nil, errors.New("timeout")
	case m := <-msg:
		return m, nil
	}
}

func ConnectEx(addr string, handler func(request []byte), onPing func(), onClose func()) (func(), error) {
	return ConnectRaw(addr, func(bytes []byte) {
		ap := utils.MustUnmarshalJson[vulinbox.AgentProtocol](bytes)
		if ap == nil {
			log.Errorf("cannot unmarshal agent protocol: %v", string(bytes))
			return
		}
		log.Debugf(`vulinbox ws agent fetch message: %v`, ap.Action)
		switch ap.Action {
		case vulinbox.ActionAck:
			if f, ok := AckWaitMap.Load(ap.ActionId); ok {
				err := f.(func([]byte) error)(bytes)
				if err != nil {
					log.Errorf("cannot handle ack: %v", err)
					return
				}
			}
			log.Error("unkown ack id: %v", ap.ActionId)
		case vulinbox.ActionDataback:
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

func Connect(addr string, handler func(request []byte), onPing ...func()) (func(), error) {
	return ConnectEx(addr, handler, func() {
		for _, i := range onPing {
			i()
		}
	}, nil)
}

func ConnectRaw(addr string, handler func([]byte), onClose func()) (func(), error) {
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
	wsPacket := lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /_/ws/agent HTTP/1.1
Host: vulinbox:8787
Connection: Upgrade
Sec-WebSocket-Key: kpFli2X1YeW53YainWGFzA==
Sec-WebSocket-Version: 13
Upgrade: websocket
User-Agent: FeedbackStreamer/1.0

`), "Host", addr)
	fmt.Println(string(wsPacket))
	var start = false
	client, err := lowhttp.NewWebsocketClient(wsPacket, lowhttp.WithWebsocketFromServerHandler(func(bytes []byte) {
		if !start {
			if utils.ExtractMapValueString(bytes, "action") == "ping" {
				start = true
			}
		}
		handler(bytes)
	}), lowhttp.WithWebsocketTLS(isTls), lowhttp.WithWebsocketHost(host), lowhttp.WithWebsocketPort(port))
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
					ping := vulinbox.NewPingAction()
					err := client.WriteText(utils.Jsonify(ping))
					if err != nil {
						log.Errorf("cannot write ping: %v", err)
						client.Stop()
					}
					_, err = WaitNow(ping.ActionId)
					if err != nil {
						log.Errorf("cannot wait ping: %v", err)
						client.Stop()
					}
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
