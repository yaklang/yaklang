package yaklib

import (
	"crypto/tls"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"io/ioutil"
	"net"
	"reflect"
	"time"
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
	net.Dialer

	tlsConfig *tls.Config
	proxy     string
	timeout   time.Duration
}

type dialerOpt func(d *_tcpDialer)

func _tcpConnect(host string, port interface{}, opts ...dialerOpt) (*tcpConnection, error) {

	dialer := net.Dialer{
		Timeout:   10 * time.Second,
		LocalAddr: nil,
	}
	tcpDialer := &_tcpDialer{Dialer: dialer}
	for _, opt := range opts {
		opt(tcpDialer)
	}

	var conn net.Conn
	var err error
	addr := utils.HostPort(fmt.Sprint(host), port)
	if tcpDialer.tlsConfig == nil {
		conn, err = utils.GetAutoProxyConn(addr, tcpDialer.proxy, tcpDialer.Timeout)
	} else {
		conn, err = utils.GetAutoProxyConnWithTLS(addr, tcpDialer.proxy, tcpDialer.Timeout, tcpDialer.tlsConfig)
	}
	if err != nil {
		return nil, err
	}
	return &tcpConnection{Conn: conn}, nil
}

func _tcpTimeout(i float64) dialerOpt {
	return func(d *_tcpDialer) {
		d.Timeout = _floatSeconds(i)
	}
}

func _tcpLocalAddr(i interface{}) dialerOpt {
	host, port, err := utils.ParseStringToHostPort(fmt.Sprint(i))
	if err != nil {
		log.Errorf("parse local addr failed: %s, ORIGIN: %v", err, i)
		return func(*_tcpDialer) {}
	}

	return func(d *_tcpDialer) {
		d.LocalAddr = &net.TCPAddr{
			IP:   net.ParseIP(host),
			Port: port,
		}
	}
}

func _tcpClientTls(crt, key interface{}, caCerts ...interface{}) dialerOpt {
	tlcConfig := BuildTlsConfig(crt, key, caCerts...)
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
	"Connect": _tcpConnect,

	// 设置超时和 local
	"clientTimeout": _tcpTimeout,
	"clientLocal":   _tcpLocalAddr,
	"clientTls":     _tcpClientTls,
	"cliengProxy":   _tcpClientProxy,

	// 设置 tcp 服务器
	"Serve":          tcpServe,
	"serverCallback": _tcpServeCallback,
	"serverContext":  _tcpServeContext,
	"serverTls":      _tcpServerTls,

	// tcp 端口转发
	"Forward": _tcpPortForward,
}
