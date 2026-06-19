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

type TcpServerConfigOpt func(c *tcpServerConfig)

// serverTls 是一个 TCP 服务器配置选项，用于让服务器以 TLS 方式提供服务
// 参数:
//   - crt: 服务器证书（PEM 格式内容或文件路径）
//   - key: 服务器私钥（PEM 格式内容或文件路径）
//   - cas: 可选的 CA 证书列表，用于校验客户端证书
//
// 返回值:
//   - 一个 TCP 服务器配置选项，作为可变参数传入 tcp.Serve
//
// Example:
// ```
// // 启动一个 TLS 的 TCP 服务器，此处仅作示意
// tcp.Serve("0.0.0.0", 8443, tcp.serverTls(cert, key))~
// ```
func _tcpServerTls(crt, key interface{}, cas ...interface{}) TcpServerConfigOpt {
	tlsConfig := BuildTlsConfig(crt, key, cas...)
	return func(c *tcpServerConfig) {
		c.tlsConfig = tlsConfig
	}
}

// serverContext 是一个 TCP 服务器配置选项，用于设置上下文以控制服务的生命周期
// 参数:
//   - ctx: 上下文对象，取消该上下文会停止服务器
//
// 返回值:
//   - 一个 TCP 服务器配置选项，作为可变参数传入 tcp.Serve
//
// Example:
// ```
// // 通过 context 控制 TCP 服务器的关闭，此处仅作示意
// ctx, cancel = context.WithCancel(context.Background())
// defer cancel()
// go tcp.Serve("0.0.0.0", 8080, tcp.serverContext(ctx))
// ```
func _tcpServeContext(ctx context.Context) TcpServerConfigOpt {
	return func(c *tcpServerConfig) {
		c.ctx = ctx
	}
}

// serverCallback 是一个 TCP 服务器配置选项，用于设置处理每个新连接的回调函数
// 参数:
//   - cb: 回调函数，接收一个 TCP 连接对象，可在其中收发数据
//
// 返回值:
//   - 一个 TCP 服务器配置选项，作为可变参数传入 tcp.Serve
//
// Example:
// ```
// // 设置 TCP 服务器处理每个连接的回调，此处仅作示意
//
//	tcp.Serve("0.0.0.0", 8080, tcp.serverCallback(func(conn) {
//	    data = conn.Recv()~
//	    conn.Send("echo: " + string(data))
//	}))~
//
// ```
func _tcpServeCallback(cb func(connection *tcpConnection)) TcpServerConfigOpt {
	return func(c *tcpServerConfig) {
		c.callback = cb
	}
}

// Serve 启动一个 TCP 服务器，监听指定地址并通过回调处理每个连接
// 参数:
//   - host: 监听的主机地址
//   - port: 监听的端口
//   - opts: 可选配置，例如 tcp.serverCallback、tcp.serverContext、tcp.serverTls
//
// 返回值:
//   - 错误信息，监听失败或服务结束时返回
//
// Example:
// ```
// // 启动 TCP 服务器处理连接，此处仅作示意
//
//	tcp.Serve("0.0.0.0", 8080, tcp.serverCallback(func(conn) {
//	    conn.Send("hello")
//	}))~
//
// ```
func tcpServe(host interface{}, port int, opts ...TcpServerConfigOpt) error {
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
