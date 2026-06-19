package yaklib

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"reflect"
	"time"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"

	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/regen"
)

func _floatSeconds(f float64) time.Duration {
	return time.Duration(float64(time.Second) * f)
}

type tcpConnection struct {
	net.Conn

	// 默认读的超时
	timeoutSeconds time.Duration
}

func (t *tcpConnection) Send(i interface{}) error {
	var err error
	switch ret := i.(type) {
	case []byte:
		_, err = t.Write(ret)
	case string:
		_, err = t.Write([]byte(ret))
	default:
		return utils.Errorf("error param type:[%v] value:[%#v], need string/[]byte", reflect.TypeOf(i), i)
	}
	return err
}

func (t *tcpConnection) SetTimeout(seconds float64) {
	t.timeoutSeconds = time.Duration(float64(time.Second) * seconds)
}

func (t *tcpConnection) GetTimeout() time.Duration {
	if t.timeoutSeconds <= 0 {
		return 10 * time.Second
	}
	return t.timeoutSeconds
}

func (t *tcpConnection) Recv() ([]byte, error) {
	results, err := utils.ReadConnWithTimeout(t, t.GetTimeout())
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (t *tcpConnection) RecvLen(i int64) ([]byte, error) {
	return ioutil.ReadAll(io.LimitReader(t, i))
}

func (t *tcpConnection) ReadFast(f ...float64) ([]byte, error) {
	timeout := t.GetTimeout()
	if len(f) > 0 && f[0] > 0 {
		timeout = _floatSeconds(f[0])
	}
	data, err := utils.ReadUntilStableEx(t, false, t.Conn, timeout, 300*time.Millisecond)
	if err != nil && err != io.EOF {
		return data, err
	}
	return data, nil
}

func (t *tcpConnection) ReadFastUntilByte(f byte) ([]byte, error) {
	timeout := t.GetTimeout()
	data, err := utils.ReadUntilStableEx(t, false, t.Conn, timeout, 300*time.Millisecond, f)
	if err != nil && err != io.EOF {
		return data, err
	}
	return data, nil
}

func (t *tcpConnection) RecvString() (string, error) {
	raw, err := t.Recv()
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (t *tcpConnection) RecvTimeout(seconds float64) ([]byte, error) {
	results, err := utils.ReadConnWithTimeout(t, time.Duration(float64(time.Second)*seconds))
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (t *tcpConnection) RecvStringTimeout(seconds float64) (string, error) {
	raw, err := t.RecvTimeout(seconds)
	if err != nil {
		return "", err
	}
	return string(raw), err
}

type _tcpDialer struct {
	tlsConfig *gmtls.Config
	proxy     string
	timeout   time.Duration
	localAddr *net.TCPAddr
	err       error // Error from invalid localAddr
}

type dialerOpt func(d *_tcpDialer)

// Connect 建立一个 TCP 连接，返回一个可收发数据的 TCP 连接对象
// 参数:
//   - host: 目标主机地址
//   - port: 目标端口
//   - opts: 可选配置，例如 tcp.clientTimeout、tcp.clientProxy、tcp.clientTls、tcp.clientLocal
//
// 返回值:
//   - TCP 连接对象，可调用 Send/Recv 等方法
//   - 错误信息，连接失败时返回非空
//
// Example:
// ```
// // 建立 TCP 连接并收发数据，依赖网络，此处仅作示意
// conn = tcp.Connect("www.example.com", 80, tcp.clientTimeout(5))~
// conn.Send("GET / HTTP/1.1\r\nHost: www.example.com\r\n\r\n")~
// rsp = conn.Recv()~
// println(string(rsp))
// ```
func _tcpConnect(host string, port interface{}, opts ...dialerOpt) (*tcpConnection, error) {
	tcpDialer := &_tcpDialer{}
	for _, opt := range opts {
		opt(tcpDialer)
	}

	// Check for errors from options (e.g., invalid localAddr)
	if tcpDialer.err != nil {
		return nil, tcpDialer.err
	}

	var conn net.Conn
	var err error
	addr := utils.HostPort(fmt.Sprint(host), port)

	// Build DialX options
	dialOpts := []netx.DialXOption{
		netx.DialX_WithTimeout(tcpDialer.timeout),
	}

	// Add local address if specified
	if tcpDialer.localAddr != nil {
		dialOpts = append(dialOpts, netx.DialX_WithTCPLocalAddr(tcpDialer.localAddr))
	}

	// Add proxy if specified
	if tcpDialer.proxy != "" {
		dialOpts = append(dialOpts, netx.DialX_WithProxy(tcpDialer.proxy))
	}

	// Add TLS config if specified
	if tcpDialer.tlsConfig != nil {
		dialOpts = append(dialOpts, netx.DialX_WithTLS(true), netx.DialX_WithGMTLSConfig(tcpDialer.tlsConfig))
	}

	conn, err = netx.DialX(addr, dialOpts...)
	if err != nil {
		return nil, err
	}
	return &tcpConnection{Conn: conn}, nil
}

// clientTimeout 是一个 TCP 客户端配置选项，用于设置连接与读写的超时时间（单位：秒）
// 参数:
//   - i: 超时时间，单位为秒，支持小数
//
// 返回值:
//   - 一个 TCP 客户端配置选项，作为可变参数传入 tcp.Connect
//
// Example:
// ```
// // 设置 5 秒超时建立 TCP 连接，此处仅作示意
// conn = tcp.Connect("www.example.com", 80, tcp.clientTimeout(5))~
// println(conn)
// ```
func _tcpTimeout(i float64) dialerOpt {
	return func(d *_tcpDialer) {
		d.timeout = _floatSeconds(i)
	}
}

// clientLocal 是一个 TCP 客户端配置选项，用于指定本地绑定的 IP 地址（不允许使用域名）
// 参数:
//   - i: 本地 IP 地址，可为 "192.168.0.1" 或 "192.168.0.1:0" 形式
//
// 返回值:
//   - 一个 TCP 客户端配置选项，作为可变参数传入 tcp.Connect
//
// Example:
// ```
// // 指定本地出口 IP 建立 TCP 连接，此处仅作示意
// conn = tcp.Connect("www.example.com", 80, tcp.clientLocal("0.0.0.0:0"), tcp.clientTimeout(5))~
// println(conn)
// ```
func _tcpLocalAddr(i interface{}) dialerOpt {
	addrStr := fmt.Sprint(i)

	// Try to parse as IP address first (e.g., "192.168.0.1")
	ip := net.ParseIP(utils.FixForParseIP(addrStr))
	if ip != nil {
		return func(d *_tcpDialer) {
			d.localAddr = &net.TCPAddr{
				IP:   ip,
				Port: 0, // Let system choose port
			}
		}
	}

	// Try to parse as host:port format (e.g., "192.168.0.1:0")
	host, port, err := utils.ParseStringToHostPort(addrStr)
	if err == nil {
		ip = net.ParseIP(utils.FixForParseIP(host))
		if ip != nil {
			return func(d *_tcpDialer) {
				d.localAddr = &net.TCPAddr{
					IP:   ip,
					Port: port,
				}
			}
		}
	}

	// If not a valid IP, return error - DNS resolution is not allowed for localAddr
	return func(d *_tcpDialer) {
		d.err = fmt.Errorf("localAddr '%s' is not a valid IP address, DNS resolution is not allowed", addrStr)
	}
}

// clientTls 是一个 TCP 客户端配置选项，用于以 TLS（含国密 GMTLS）方式建立连接
// 参数:
//   - crt: 客户端证书（PEM 格式内容或文件路径）
//   - key: 客户端私钥（PEM 格式内容或文件路径）
//   - caCerts: 可选的 CA 证书列表，用于校验服务端证书
//
// 返回值:
//   - 一个 TCP 客户端配置选项，作为可变参数传入 tcp.Connect
//
// Example:
// ```
// // 以双向 TLS 建立 TCP 连接，此处仅作示意
// conn = tcp.Connect("www.example.com", 443, tcp.clientTls(cert, key), tcp.clientTimeout(5))~
// println(conn)
// ```
func _tcpClientTls(crt, key interface{}, caCerts ...interface{}) dialerOpt {
	tlcConfig := BuildGmTlsConfig(crt, key, caCerts...)
	return func(d *_tcpDialer) {
		d.tlsConfig = tlcConfig
	}
}

// clientProxy 是一个 TCP 客户端配置选项，用于通过代理建立连接
// 参数:
//   - proxy: 代理地址，支持 http、https、socks4、socks5 协议
//
// 返回值:
//   - 一个 TCP 客户端配置选项，作为可变参数传入 tcp.Connect
//
// Example:
// ```
// // 通过本地 socks5 代理建立 TCP 连接，此处仅作示意
// conn = tcp.Connect("www.example.com", 80, tcp.clientProxy("socks5://127.0.0.1:1080"), tcp.clientTimeout(5))~
// println(conn)
// ```
func _tcpClientProxy(proxy string) dialerOpt {
	return func(d *_tcpDialer) {
		d.proxy = proxy
	}
}

var TcpExports = map[string]interface{}{
	"MockServe":       utils.DebugMockHTTP,
	"MockTCPProtocol": DebugMockTCPProtocol,

	"Connect": _tcpConnect,

	// 设置超时和 local
	"clientTimeout": _tcpTimeout,
	"clientLocal":   _tcpLocalAddr,
	"clientTls":     _tcpClientTls,
	"clientProxy":   _tcpClientProxy,

	// 设置 tcp 服务器
	"Serve":          tcpServe,
	"serverCallback": _tcpServeCallback,
	"serverContext":  _tcpServeContext,
	"serverTls":      _tcpServerTls,

	// tcp 端口转发
	"Forward": _tcpPortForward,
}

var (
	Tcp_Server_Callback = _tcpServeCallback
	Tcp_Server_Context  = _tcpServeContext
	Tcp_Server_Tls      = _tcpServerTls
)

// MockTCPProtocol 启动一个模拟指定协议指纹的 TCP 服务，用于测试，返回监听的主机与端口
// 参数:
//   - name: 要模拟的服务名称（指纹规则名）
//
// 返回值:
//   - 模拟服务监听的主机地址
//   - 模拟服务监听的端口
//
// Example:
// ```
// // 启动一个模拟 TCP 协议的本地服务用于测试，此处仅作示意
// host, port = tcp.MockTCPProtocol("http")
// println(host, port)
// ```
func DebugMockTCPProtocol(name string) (string, int) {
	cfg := fp.NewConfig(fp.WithTransportProtos(fp.ParseStringToProto([]interface{}{"tcp"}...)...))
	blocks := fp.GetRuleBlockByServiceName(name, cfg)
	var generate string
	var err error
	responses := make(map[string][][]byte)
	for _, block := range blocks {
		payload := block.Probe.Payload
		log.Infof("payload: %#v", payload)
		for _, match := range block.Matched {
			r := match.MatchRule.String()
			log.Infof("ServiceName: [%s] , ProductVerbose: [%s]", match.ServiceName, match.ProductVerbose)
			generate, err = regen.GenerateOne(r)
			if err != nil {
				continue
			}
			responses[payload] = append(responses[payload], convertToBytes(generate))
		}
	}
	return DebugMockTCPFromScan(30*time.Minute, responses)
}

func DebugMockTCPFromScan(du time.Duration, responses map[string][][]byte) (string, int) {
	var (
		listener net.Listener
		err      error
	)
	addr := utils.GetRandomLocalAddr()
	time.Sleep(time.Millisecond * 300)
	host, port, _ := utils.ParseStringToHostPort(addr)

	go func() {
		listener, err = net.Listen("tcp", addr)
		if err != nil {
			panic(err)
		}
		defer listener.Close()

		go func() {
			time.Sleep(du)
			listener.Close()
		}()

		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go func(conn net.Conn) {
				defer conn.Close()

				buffer := make([]byte, 1024)
				n, err := conn.Read(buffer)
				if err != nil {
					return
				}

				requestPayload := string(buffer[:n])

				log.Infof("requestPayload: %#v from: %v", requestPayload, conn.RemoteAddr().String())

				if responses, ok := responses[requestPayload]; ok {
					rand.NewSource(time.Now().UnixNano())
					response := responses[rand.Intn(len(responses))]
					log.Infof("send: %#v to: %v", string(response), conn.RemoteAddr().String())
					conn.Write(response)
					time.Sleep(50 * time.Millisecond)
				}
			}(conn)
		}
	}()

	time.Sleep(time.Millisecond * 100)
	return host, port
}
