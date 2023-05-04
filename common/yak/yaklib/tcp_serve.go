package yaklib

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"strconv"
)

type tcpServerConfig struct {
	ctx       context.Context
	callback  func(conn *tcpConnection)
	tlsConfig *tls.Config
}

type tcpServerConfigOpt func(c *tcpServerConfig)

func _tcpServerTls(crt, key interface{}, cas ...interface{}) tcpServerConfigOpt {
	tlsConfig := BuildTlsConfig(crt, key, cas...)
	return func(c *tcpServerConfig) {
		c.tlsConfig = tlsConfig
	}
}

func _tcpServeContext(ctx context.Context) tcpServerConfigOpt {
	return func(c *tcpServerConfig) {
		c.ctx = ctx
	}
}

func _tcpServeCallback(cb func(connection *tcpConnection)) tcpServerConfigOpt {
	return func(c *tcpServerConfig) {
		c.callback = cb
	}
}

func tcpServe(host interface{}, port int, opts ...tcpServerConfigOpt) error {
	config := &tcpServerConfig{ctx: context.Background()}

	for _, opt := range opts {
		opt(config)
	}

	if config.ctx == nil {
		config.ctx = context.Background()
	}

	addr := utils.HostPort(fmt.Sprint(host), port)
	var lis net.Listener
	var err error
	if config.tlsConfig == nil {
		lis, err = net.Listen("tcp", addr)
	} else {
		lis, err = tls.Listen("tcp", addr, config.tlsConfig)
	}
	if err != nil {
		return utils.Errorf("listen %v failed: %s", addr, err)
	}

	go func() {
		select {
		case <-config.ctx.Done():
			lis.Close()
		}
	}()

	for {
		con, err := lis.Accept()
		if err != nil {
			return utils.Errorf("tcp listener [%v] cannot accept: %v", addr, err)
		}

		log.Infof("recv tcp connection from %v to %v", con.RemoteAddr().String(), con.LocalAddr().String())
		go func(peerConn *tcpConnection) {
			if config.callback != nil {
				config.callback(peerConn)
			} else {
				scanner := bufio.NewScanner(peerConn)
				scanner.Split(bufio.ScanBytes)

				for scanner.Scan() {
					raw := scanner.Text()
					raw = strconv.QuoteToGraphic(raw)
					if len(raw) > 2 {
						raw = raw[1 : len(raw)-1]
					}
					fmt.Printf("%v", raw)
				}
			}
		}(&tcpConnection{Conn: con})
	}
}
