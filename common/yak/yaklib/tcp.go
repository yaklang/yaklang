package yaklib

import (
	"fmt"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"reflect"
	"time"

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
}

type dialerOpt func(d *_tcpDialer)

func _tcpConnect(host string, port interface{}, opts ...dialerOpt) (*tcpConnection, error) {
	tcpDialer := &_tcpDialer{}
	for _, opt := range opts {
		opt(tcpDialer)
	}

	var conn net.Conn
	var err error
	addr := utils.HostPort(fmt.Sprint(host), port)
	if tcpDialer.tlsConfig != nil {
		conn, err = netx.DialTLSTimeout(tcpDialer.timeout, addr, tcpDialer.tlsConfig, tcpDialer.proxy)
	} else {
		conn, err = netx.DialTCPTimeout(tcpDialer.timeout, addr, tcpDialer.proxy)
	}
	if err != nil {
		return nil, err
	}
	return &tcpConnection{Conn: conn}, nil
}

func _tcpTimeout(i float64) dialerOpt {
	return func(d *_tcpDialer) {
		d.timeout = _floatSeconds(i)
	}
}

func _tcpLocalAddr(i interface{}) dialerOpt {
	host, port, err := utils.ParseStringToHostPort(fmt.Sprint(i))
	if err != nil {
		log.Errorf("parse local addr failed: %s, ORIGIN: %v", err, i)
		return func(*_tcpDialer) {}
	}

	return func(d *_tcpDialer) {
		d.localAddr = &net.TCPAddr{
			IP:   net.ParseIP(host),
			Port: port,
		}
	}
}

func _tcpClientTls(crt, key interface{}, caCerts ...interface{}) dialerOpt {
	tlcConfig := BuildGmTlsConfig(crt, key, caCerts...)
	return func(d *_tcpDialer) {
		d.tlsConfig = tlcConfig
	}
}

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
