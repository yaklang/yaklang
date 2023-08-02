package yaklib

import (
	"context"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/regen"
	"github.com/yaklang/yaklang/common/yakdns"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"reflect"
	"strconv"
	"time"
)

type udpConn struct {
	*net.UDPConn

	timeoutSeconds time.Duration
}

type udpClientConfig struct {
	localAddr      *net.UDPAddr
	timeoutSeconds time.Duration
}

type udpClientOption func(i *udpClientConfig)

func clientLocalAddr(target string) udpClientOption {
	return func(i *udpClientConfig) {
		addr, err := net.ResolveUDPAddr("udp", target)
		if err != nil {
			log.Errorf("resove udp addr failed: %s origin: %v", target, addr.String())
			return
		}
		i.localAddr = addr
	}
}

func clientTimeout(target float64) udpClientOption {
	return func(i *udpClientConfig) {
		i.timeoutSeconds = utils.FloatSecondDuration(target)
	}
}

func connectUdp(target string, opts ...udpClientOption) (*udpConn, error) {
	config := &udpClientConfig{timeoutSeconds: 10 * time.Second}
	for _, opt := range opts {
		opt(config)
	}

	var conn net.Conn
	remoteAddr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		return nil, err
	}

	if config.localAddr != nil {
		conn, err = net.DialUDP("udp", config.localAddr, remoteAddr)
		if err != nil {
			return nil, utils.Errorf("dial udp[%s] failed: %s", target, err)
		}
	} else {
		conn, err = net.Dial("udp", remoteAddr.String())
		if err != nil {
			return nil, utils.Errorf("dial udp[%s] failed: %s", target, err)
		}
	}

	uc, ok := conn.(*net.UDPConn)
	if !ok {
		return nil, utils.Errorf("BUG: not a net.UDPConn instead of %v", reflect.TypeOf(conn))
	}

	return &udpConn{UDPConn: uc, timeoutSeconds: config.timeoutSeconds}, nil
}

func (t *udpConn) SetTimeout(seconds float64) {
	t.timeoutSeconds = utils.FloatSecondDuration(seconds)
}

func (t *udpConn) GetTimeout() time.Duration {
	if t.timeoutSeconds <= 0 {
		t.timeoutSeconds = 10 * time.Second
		return 10 * time.Second
	}
	return t.timeoutSeconds
}

func (t *udpConn) Recv() ([]byte, error) {
	results, err := utils.ReadConnWithTimeout(t, t.GetTimeout())
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (r *udpConn) RecvLen(i int64) ([]byte, error) {
	return ioutil.ReadAll(io.LimitReader(r, i))
}

func (t *udpConn) RecvString() (string, error) {
	raw, err := t.Recv()
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (t *udpConn) RecvTimeout(seconds float64) ([]byte, error) {
	results, err := utils.ReadConnWithTimeout(t, time.Duration(float64(time.Second)*seconds))
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (t *udpConn) RecvStringTimeout(seconds float64) (string, error) {
	raw, err := t.RecvTimeout(seconds)
	if err != nil {
		return "", err
	}
	return string(raw), err
}

func (t *udpConn) SendTo(i interface{}, target string) error {
	host, port, err := utils.ParseStringToHostPort(target)
	if err != nil {
		return err
	}

	if !utils.IsIPv4(host) {
		host = yakdns.LookupFirst(host, yakdns.WithTimeout(t.GetTimeout()))
		if host == "" {
			return utils.Errorf("cannot found ip by %v", host)
		}
	}
	addr := &net.UDPAddr{
		IP:   net.ParseIP(host),
		Port: port,
	}

	switch ret := i.(type) {
	case []byte:
		_, err = t.WriteToUDP(ret, addr)
	case string:
		_, err = t.WriteToUDP([]byte(ret), addr)
	default:
		return utils.Errorf("error param type:[%v] value:[%#v], need string/[]byte", reflect.TypeOf(i), i)
	}
	return err
}

func (t *udpConn) Send(i interface{}) error {
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

func (t *udpConn) ReadFromAddr() ([]byte, net.Addr, error) {
	var raw []byte
	var buf = make([]byte, 4096)
	for {
		t.UDPConn.SetDeadline(time.Now().Add(t.timeoutSeconds))
		n, addr, err := t.UDPConn.ReadFromUDP(buf)
		raw = append(raw, buf[:n]...)
		if n < len(buf) {
			return raw, addr, err
		}
	}
}

func (t *udpConn) ReadStringFromAddr() (string, net.Addr, error) {
	raw, addr, err := t.ReadFromAddr()
	return string(raw), addr, err
}

type udpServerConfig struct {
	callback func(conn *udpConn, msg []byte, addr net.Addr)
	ctx      context.Context
	timeout  time.Duration
}

type udpServerOpt func(config *udpServerConfig)

func udpServe(host string, port interface{}, opts ...udpServerOpt) error {
	config := &udpServerConfig{timeout: 10 * time.Second}
	for _, opt := range opts {
		opt(config)
	}

	if config.ctx == nil {
		config.ctx = context.Background()
	}

	udpAddr, err := net.ResolveUDPAddr("udp", utils.HostPort(host, port))
	if err != nil {
		return utils.Errorf("resolve udp addr: %v", err)
	}

	log.Infof("start to listen udp://%v", udpAddr)
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Infof("finished listening on udp://%v", udpAddr)

	wConn := &udpConn{
		UDPConn:        conn,
		timeoutSeconds: config.timeout,
	}
	for {
		//select {
		//case <-config.ctx.Done():
		//	return config.ctx.Err()
		//default:
		//}

		raw, addr, err := wConn.ReadFromAddr()
		if err != nil && raw == nil {
			continue
		}
		log.Infof("recv: %#v from: %v", raw, addr.String())
		go func() {
			if config.callback == nil {
				config.callback = func(conn *udpConn, msg []byte, addr net.Addr) {
					log.Infof("udp://%v send %v local: %v", addr.String(), strconv.Quote(string(msg)), utils.HostPort(host, port))
				}
			}
			config.callback(wConn, raw, addr)
		}()
	}
}

var UDPExport = map[string]interface{}{
	"MockUDPProtocol": DebugMockUDPProtocol,
	"Connect":         connectUdp,
	"clientTimeout":   clientTimeout,
	"clientLocalAddr": clientLocalAddr,

	"Serve": udpServe,
	"serverTimeout": func(f float64) udpServerOpt {
		return func(config *udpServerConfig) {
			config.timeout = utils.FloatSecondDuration(f)
		}
	},
	"serverContext": func(ctx context.Context) udpServerOpt {
		return func(config *udpServerConfig) {
			config.ctx = ctx
		}
	},
	"serverCallback": func(cb func(*udpConn, []byte, net.Addr)) udpServerOpt {
		return func(config *udpServerConfig) {
			config.callback = cb
		}
	},
}

func DebugMockUDP(rsp []byte) (string, int) {
	return DebugMockUDPWithTimeout(1*time.Minute, rsp)
}

func DebugMockUDPProtocol(name string) (string, int) {
	cfg := fp.NewConfig(fp.WithTransportProtos(fp.ParseStringToProto([]interface{}{"udp"}...)...))
	blocks := fp.GetRuleBlockByServiceName(name, cfg)
	var generates []string
	var err error
	responses := make(map[string][][]byte)
	for _, block := range blocks {
		payload := block.Probe.Payload
		for _, match := range block.Matched {
			r := match.MatchRule.String()
			log.Infof("ServiceName: [%s] , ProductVerbose: [%s]", match.ServiceName, match.ProductVerbose)
			log.Infof("MatchRule: [%s]", r)
			generates, err = regen.GenerateOne(r)
			if err != nil {
				continue
			}
			responses[payload] = append(responses[payload], convertToBytes(generates[0]))
		}
	}
	return DebugMockUDPFromScan(3*time.Minute, responses)
}

func DebugMockUDPFromScan(du time.Duration, responses map[string][][]byte) (string, int) {
	addr := utils.GetRandomLocalAddr()
	time.Sleep(time.Millisecond * 300)
	host, port, _ := utils.ParseStringToHostPort(addr)
	go func() {
		conn, err := net.ListenPacket("udp", addr)
		if err != nil {
			panic(err)
		}
		defer conn.Close()
		go func() {
			time.Sleep(du)
			conn.Close()
		}()

		buffer := make([]byte, 1024)
		for {
			n, addr, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}

			requestPayload := string(buffer[:n])
			log.Infof("recv: %#v from: %v", requestPayload, addr.String())
			if responses, ok := responses[requestPayload]; ok {
				rand.NewSource(time.Now().UnixNano())
				response := responses[rand.Intn(len(responses))]
				log.Infof("send: %#v to: %v", string(response), addr.String())
				conn.WriteTo(response, addr)
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()
	time.Sleep(time.Millisecond * 100)
	return host, port
}

func convertToBytes(s string) []byte {
	var result []byte
	for _, r := range s {
		if r > 127 || r < 32 || (r >= 0x7F && r <= 0xA0) { // ASCII 范围之外的字符
			result = append(result, byte(r))
		} else {
			result = append(result, byte(r))
		}
	}
	return result
}

func DebugMockUDPWithTimeout(du time.Duration, rsp []byte) (string, int) {
	addr := utils.GetRandomLocalAddr()
	time.Sleep(time.Millisecond * 300)
	host, port, _ := utils.ParseStringToHostPort(addr)
	go func() {
		conn, err := net.ListenPacket("udp", addr)
		if err != nil {
			panic(err)
		}
		defer conn.Close()
		go func() {
			time.Sleep(du)
			conn.Close()
		}()

		buffer := make([]byte, 1024)
		for {
			_, addr, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}

			conn.WriteTo(rsp, addr)
			time.Sleep(50 * time.Millisecond)
		}
	}()
	time.Sleep(time.Millisecond * 100)
	return host, port
}
