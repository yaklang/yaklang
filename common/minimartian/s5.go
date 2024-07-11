package minimartian

import (
	"context"
	"encoding/binary"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
)

type S5Config struct {
	HandshakeTimeout    time.Duration
	S5RequestTimeout    time.Duration
	DialDstTimeout      time.Duration
	Debug               bool
	DownstreamHTTPProxy string
	ProxyUsername       string
	ProxyPassword       string
}

func NewSocks5Config() *S5Config {
	return &S5Config{
		HandshakeTimeout: 30 * time.Second,
		S5RequestTimeout: 30 * time.Second,
		DialDstTimeout:   30 * time.Second,
	}
}

const (
	socks5Version = 0x05

	authNone                = 0x00
	authWithUsernameAndPass = 0x02
	authNoAcceptable        = 0xFF

	UsernameAndPassVersion     = 0x01
	UsernameAndPassAuthSuccess = 0x00
	UsernameAndPassAuthFail    = 0x01

	commandConnect = 0x01
	commandBind    = 0x02
	commandUDP     = 0x03

	addrTypeIPv4 = 0x01
	addrTypeFQDN = 0x03
	addrTypeIPv6 = 0x04

	replySuccess = 0x00
)

func IsSocks5HandleShake(conn net.Conn) (fConn net.Conn, _ bool, _ byte, _ error) {
	peekable := utils.NewPeekableNetConn(conn)

	defer func() {
		if err := recover(); err != nil {
			log.Infof("peekable failed: %s", err)
			fConn = peekable
		}
	}()

	raw, err := peekable.Peek(2)
	if err != nil {
		if err == io.EOF {
			return peekable, false, 0, nil
		}
		return nil, false, 0, utils.Errorf("peek failed: %s", err)
	}
	if len(raw) != 2 {
		return nil, false, 0, utils.Errorf("check s5 failed: %v", raw)
	}
	return peekable, raw[0] == socks5Version && raw[1] > 0, raw[0], nil
}

func (c *S5Config) Handshake(conn net.Conn) error {
	conn.SetReadDeadline(time.Now().Add(c.HandshakeTimeout))
	defer conn.SetReadDeadline(time.Time{})

	meta, err := utils.ReadN(conn, 2)
	if err != nil || len(meta) < 2 {
		return utils.Errorf("read negotiation header failed: %s", err)
	}

	version := meta[0]
	nMethods := meta[1]
	if version != socks5Version {
		return utils.Errorf("negotiation socks5 version failed: %v", strconv.Quote(string(meta)))
	}

	var methods = make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return utils.Errorf("negotiation shake auth methods failed: %s", err)
	}

	supportMethods := []byte{authWithUsernameAndPass}
	needAuth := c.ProxyPassword != "" || c.ProxyUsername != ""
	if !needAuth {
		supportMethods = append(supportMethods, authNone)
	}

	authFlag := byte(authNoAcceptable)
	for _, method := range methods {
		if utils.ContainsAny(supportMethods, method) {
			authFlag = method
		}
	}

	_, err = conn.Write([]byte{socks5Version, authFlag})
	if err != nil {
		return err
	}

	if needAuth {
		authVersion, err := utils.ReadN(conn, 1)
		if err != nil {
			return err
		}

		if len(authVersion) <= 0 {
			return utils.Errorf("read auth version get empty")
		}

		if authVersion[0] != byte(UsernameAndPassVersion) {
			return utils.Errorf("auth version failed: %v", strconv.Quote(string(authVersion[0])))
		}

		getDate := func() (string, error) {
			lengthByte, err := utils.ReadN(conn, 1)
			if err != nil {
				return "", err
			}
			if len(lengthByte) <= 0 {
				return "", utils.Errorf("read data length get empty")
			}
			data, err := utils.ReadN(conn, int(lengthByte[0]))
			if err != nil {
				return "", err
			}
			return string(data), nil
		}

		username, err := getDate()
		if err != nil {
			return err
		}
		password, err := getDate()
		if err != nil {
			return err
		}

		if !(c.ProxyUsername == username && c.ProxyPassword == password) {
			conn.Write([]byte{UsernameAndPassVersion, UsernameAndPassAuthFail})
			return utils.Errorf("auth failed")
		}
		conn.Write([]byte{UsernameAndPassVersion, UsernameAndPassAuthSuccess})
	}

	return nil
}

func (c *S5Config) HandleS5RequestHeader(conn net.Conn) (version byte, cmd byte, AType byte, target string, err error) {
	headerBuf, err := utils.ReadN(conn, 4)
	if err != nil || len(headerBuf) < 4 {
		return
	}
	version, cmd, reserved, AType := headerBuf[0], headerBuf[1], headerBuf[2], headerBuf[3]
	_ = reserved

	if version != socks5Version {
		err = utils.Errorf("invalid s5 version: %v", strconv.Quote(string(version)))
		return
	}

	var targetHost string
	switch AType {
	case addrTypeIPv4:
		v4 := make(net.IP, net.IPv4len)
		if _, err = io.ReadFull(conn, v4); err != nil {
			return
		}
		targetHost = v4.String()
	case addrTypeIPv6:
		v6 := make(net.IP, net.IPv6len)
		if _, err = io.ReadFull(conn, v6); err != nil {
			return
		}
		targetHost = v6.String()
	case addrTypeFQDN:
		var buf []byte
		buf, err = utils.ReadN(conn, 1)
		if err != nil || len(buf) < 1 {
			return
		}
		fqdnLen := buf[0]
		fqdn := make([]byte, fqdnLen)
		if _, err = io.ReadFull(conn, fqdn); err != nil {
			return
		}
		targetHost = string(fqdn)
	default:
		err = utils.Errorf("read s5 req addr type failed: %v", strconv.Quote(string(AType)))
		return
	}

	var targetPort uint16 // 0-65535 (0xffff f=(0b1111))
	if err = binary.Read(conn, binary.BigEndian, &targetPort); err != nil {
		return
	}

	return version, cmd, AType, utils.HostPort(targetHost, targetPort), nil
}

func BuildReply(host net.IP, port int) []byte {
	var portsBytes = make([]byte, 2)
	binary.BigEndian.PutUint16(portsBytes, uint16(port))
	reply := []byte{socks5Version, replySuccess, 0x00}
	if ipIns := host.To4(); ipIns != nil {
		reply = append(reply, addrTypeIPv4)
		reply = append(reply, ipIns...)
		reply = append(reply, portsBytes...)
	} else {
		reply = append(reply, addrTypeIPv4)
		reply = append(reply, 0x00, 0x00, 0x00, 0x00)
		reply = append(reply, portsBytes...)
	}
	return reply
}

func (c *S5Config) HandleConnect(conn net.Conn, target string) error {
	dstConn, err := netx.DialTCPTimeout(c.DialDstTimeout, target)
	if err != nil {
		return utils.Errorf("dial target failed: %s", err)
	}
	if dstConn == nil {
		return utils.Error("BUG: act conn is nil")
	}

	// 告诉客户端已成功连接到目标服务器
	host, port, _ := utils.ParseStringToHostPort(dstConn.LocalAddr().String())
	conn.Write(BuildReply(net.ParseIP(host), port))
	err = c.ConnectionFallback(dstConn, conn)
	if err != nil {
		return err
	}
	return nil
}

func (c *S5Config) HandleBind(conn net.Conn, target string) error {
	bindPort := utils.GetRandomAvailableTCPPort()
	targetHost, targetPort, _ := utils.ParseStringToHostPort(target)
	if !utils.IsIPv4(target) && !utils.IsIPv6(target) {
		targetHost = netx.LookupFirst(targetHost, netx.WithTimeout(3*time.Second))
		if targetHost == "" {
			return errors.Errorf("cannot found domain[%s]'s ip address", target)
		}
	}
	_, _, bindHost, err := netutil.Route(time.Second*5, targetHost)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", utils.HostPort(bindHost.String(), bindPort))
	if err != nil {
		return err
	}

	// 告诉客户端已开始监听
	_, err = conn.Write(BuildReply(bindHost, bindPort))
	if err != nil {
		return err
	}

	defer listener.Close()
	for {
		dstConn, err := listener.Accept()
		if err != nil {
			return utils.Errorf("socks5 server bind mode accept err %v", err)
		}
		if dstConn.RemoteAddr().String() == utils.HostPort(targetHost, targetPort) {
			// 第二次 reply
			_, err = conn.Write(BuildReply(net.ParseIP(targetHost), targetPort))
			if err != nil {
				return err
			}
			err := c.ConnectionFallback(dstConn, conn)
			if err != nil {
				return utils.Errorf("socks5 server bind mode connectFallback err %v", err)
			}
			break
		}
	}

	return nil
}

func (c *S5Config) HandleS5Request(conn net.Conn) (net.Conn, error) {
	conn.SetDeadline(time.Now().Add(c.S5RequestTimeout))
	defer conn.SetDeadline(time.Time{})

	_, cmd, _, target, err := c.HandleS5RequestHeader(conn)
	if err != nil {
		return conn, err
	}

	switch cmd {
	case commandConnect:
		err := c.HandleConnect(conn, target)
		if err != nil {
			return nil, err
		}
	case commandBind:
		err := c.HandleBind(conn, target)
		if err != nil {
			return nil, err
		}
	case commandUDP:
		//TODO
	}

	return conn, nil
}

func (c *S5Config) ServeConn(conn net.Conn) error {
	defer func() {
		if conn != nil {
			conn.Close()
		}
		if err := recover(); err != nil {
			log.Errorf("serve socks5 connection panic: %v", err)
		}
	}()

	err := c.Handshake(conn)
	if err != nil {
		return err
	}

	dstConn, err := c.HandleS5Request(conn)
	if err != nil {
		return err
	}
	var isHttp bool
	conn, dstConn, isHttp, err = c.HijackSource(conn, dstConn)
	if err != nil {
		return err
	}
	_ = isHttp
	return c.ConnectionFallback(conn, dstConn)
	//if !isHttp {
	//	return c.ConnectionFallback(conn, dstConn)
	//}
	//return nil
}

func (c *S5Config) ConnectionFallback(src, proxiedConn net.Conn) error {
	// fullback
	wg := new(sync.WaitGroup)
	wg.Add(2)
	dst := proxiedConn
	wCtx, cancel := context.WithCancel(context.Background())
	ctxSrc := ctxio.NewReaderWriter(wCtx, src)
	ctxDst := ctxio.NewReaderWriter(wCtx, dst)

	// 从 src 读取数据，写入 dst
	go func() {
		defer func() {
			cancel()
			wg.Done()
		}()
		var err error
		if c.Debug {
			_, err = io.Copy(ctxDst, io.TeeReader(ctxSrc, os.Stdout))
		} else {
			_, err = io.Copy(ctxDst, ctxSrc)
		}
		if err != nil && c.Debug && err != io.EOF {
			log.Warnf("bridge %v -> %v failed: %s", src.RemoteAddr().String(), dst.RemoteAddr().String(), err)
		}
	}()

	// 从 dst 读取数据，写入 src
	go func() {
		defer func() {
			cancel()
			wg.Done()
		}()

		var err error
		if c.Debug {
			_, err = io.Copy(ctxSrc, io.TeeReader(ctxDst, os.Stdout))
		} else {
			_, err = io.Copy(ctxSrc, ctxDst)
		}
		if err != nil && c.Debug && err != io.EOF {
			log.Warnf("bridge %v -> %v failed: %s", src.RemoteAddr().String(), dst.RemoteAddr().String(), err)
		}
	}()
	wg.Wait()
	return nil
}

func (c *S5Config) HijackSource(src, proxiedConn net.Conn) (net.Conn, net.Conn, bool, error) {
	// martian s5 is no need to hijack
	return src, proxiedConn, true, nil
}
