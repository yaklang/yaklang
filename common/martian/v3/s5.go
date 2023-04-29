package martian

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"yaklang/common/cybertunnel/ctxio"
	"yaklang/common/log"
	"yaklang/common/utils"
	"strconv"
	"sync"
	"time"
)

type S5Config struct {
	HandshakeTimeout    time.Duration
	Debug               bool
	DownstreamHTTPProxy string
	ProxyUsername       string
	ProxyPassword       string
}

func NewSocks5Config() *S5Config {
	return &S5Config{
		HandshakeTimeout: 30 * time.Second,
	}
}

const (
	socks5Version = 0x05

	authNone                = 0x00
	authWithUsernameAndPass = 0x02
	authNoAcceptable        = 0xFF

	commandConnect = 0x01

	addrTypeIPv4 = 0x01
	addrTypeFQDN = 0x03
	addrTypeIPv6 = 0x04

	replySuccess = 0x00
)

func (c *S5Config) IsSocks5HandleShake(conn net.Conn) (fConn net.Conn, _ bool, _ error) {
	peekable := utils.NewPeekableNetConn(conn)

	defer func() {
		if err := recover(); err != nil {
			log.Infof("peekable failed: %s", err)
			fConn = peekable
		}
	}()

	raw, err := peekable.Peek(2)
	if err != nil {
		return nil, false, utils.Errorf("peek failed: %s", err)
	}
	if len(raw) != 2 {
		return nil, false, utils.Errorf("check s5 failed: %v", raw)
	}

	if raw[0] == socks5Version {
		if raw[1] > 0 {
			return peekable, true, nil
		}
	}
	return peekable, false, nil
}

func (c *S5Config) Handshake(conn net.Conn) (net.Conn, error) {
	peekable, isSocks5, err := c.IsSocks5HandleShake(conn)
	if err != nil {
		return nil, utils.Errorf("check s5 failed: %s", err)
	}
	if !isSocks5 {
		return nil, utils.Error("not socks5")
	}
	err = c.handshakeHandler(peekable)
	if err != nil {
		return nil, utils.Errorf("s5 handshake failed: %s", err)
	}
	return peekable, nil
}

func (c *S5Config) handshakeHandler(conn net.Conn) error {
	// handle shake
	var handshakeTimeout = 30 * time.Second

	conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
	var buf = make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return utils.Errorf("handle shake header failed: %s", err)
	}

	if buf[0] != socks5Version {
		return utils.Errorf("handle socks5 version failed: %v", strconv.Quote(string(buf)))
	}

	methods := make([]byte, buf[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return utils.Errorf("handle shake auth methods failed: %s", err)
	}

	finishedAuth := len(methods) > 0
	needpassword := false

	for _, method := range methods {
		switch method {
		case authNone:
			needpassword = false
			break
		case authWithUsernameAndPass:
			needpassword = true
		default:
			continue
		}
	}

	if !finishedAuth {
		conn.Write([]byte{socks5Version, authNoAcceptable})
		return utils.Errorf("handle shake auth methods failed: %v", strconv.Quote(string(methods)))
	}

	var authFlag byte = authNone
	if needpassword {
		authFlag = authWithUsernameAndPass
		conn.Write([]byte{socks5Version, authFlag})

		bytes, err := utils.ReadN(conn, 2)
		if err != nil {
			return err
		}
		if bytes[0] != 0x01 {
			return utils.Errorf("auth version failed: %v", strconv.Quote(string(bytes)))
		}
		var username string
		var password string
		ulen := int(bytes[1])
		if ulen == 0 {
			username = ""
		} else {
			usernameBytes, err := utils.ReadN(conn, ulen)
			if err != nil {
				return utils.Errorf("read username failed: %s, err")
			}
			username = string(usernameBytes)
		}

		bytes, err = utils.ReadN(conn, 1)
		if err != nil {
			return err
		}

		plen := int(bytes[0])
		if plen == 0 {
			password = ""
		} else {
			passwordBytes, err := utils.ReadN(conn, plen)
			if err != nil {
				return utils.Errorf("read password failed: %s, err")
			}
			password = string(passwordBytes)
		}

		if !(c.ProxyUsername == username && c.ProxyPassword == password) {
			return utils.Errorf("auth failed")
		}
		conn.Write([]byte{0x01, 0x00})
	} else {
		conn.Write([]byte{socks5Version, authFlag})
	}

	return nil
}

func (c *S5Config) HandleS5Request(conn net.Conn) (net.Conn, error) {
	buf := make([]byte, 4)

	s5RequestTimeout := 30 * time.Second

	conn.SetDeadline(time.Now().Add(s5RequestTimeout))
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, utils.Errorf("cannot read s5 req header: %s", err)
	}

	version, cmd, reserved := buf[0], buf[1], buf[2]

	if version != socks5Version {
		return nil, utils.Errorf("invalid s5 version: %v", strconv.Quote(string(buf)))
	}

	if cmd != commandConnect {
		return nil, utils.Errorf("invalid s5 command: %v", strconv.Quote(string(buf)))
	}
	_ = reserved

	// 要连接的具体目标
	var targetHost string

	var addrBytesRaw []byte
	addrType := buf[3]
	switch addrType {
	case addrTypeIPv4:
		v4 := make(net.IP, net.IPv4len)
		if _, err := io.ReadFull(conn, v4); err != nil {
			return nil, err
		}
		addrBytesRaw = append(addrBytesRaw, addrTypeIPv4)
		addrBytesRaw = append(addrBytesRaw, v4...)
		targetHost = v4.String()
	case addrTypeIPv6:
		v6 := make(net.IP, net.IPv6len)
		if _, err := io.ReadFull(conn, v6); err != nil {
			return nil, err
		}
		addrBytesRaw = append(addrBytesRaw, addrTypeIPv6)
		addrBytesRaw = append(addrBytesRaw, v6...)
		targetHost = v6.String()
	case addrTypeFQDN:
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return nil, err
		}
		fqdnLen := buf[0]
		fqdn := make([]byte, fqdnLen)
		if _, err := io.ReadFull(conn, fqdn); err != nil {
			return nil, err
		}
		addrBytesRaw = append(addrBytesRaw, addrTypeFQDN, buf[0])
		addrBytesRaw = append(addrBytesRaw, fqdn...)
		targetHost = string(fqdn)
	default:
		return nil, utils.Errorf("read s5 req addr type failed: %v", strconv.Quote(string(buf)))
	}

	var targetPort uint16 // 0-65535 (0xffff f=(0b1111))
	if err := binary.Read(conn, binary.BigEndian, &targetPort); err != nil {
		return nil, utils.Errorf("read s5 req port failed: %s", err)
	}
	var portsBytes = make([]byte, 2)
	binary.BigEndian.PutUint16(portsBytes, targetPort)
	addrBytesRaw = append(addrBytesRaw, portsBytes...)

	targetAddr := utils.HostPort(targetHost, targetPort)
	log.Infof("socks5 recv target cmd: %s", targetAddr)

	downstreamProxy := c.DownstreamHTTPProxy
	var proxyConnectionTimeout = 30 * time.Second
	var actConn net.Conn
	if downstreamProxy != "" {
		var err error
		actConn, err = utils.GetProxyConn(targetAddr, downstreamProxy, proxyConnectionTimeout)
		if err != nil {
			return nil, utils.Errorf("downstream fetch conn failed: %s", err)
		}
	} else {
		var err error
		actConn, err = net.DialTimeout("tcp", targetAddr, proxyConnectionTimeout)
		if err != nil {
			return nil, utils.Errorf("dial target failed: %s", err)
		}
	}
	if actConn == nil {
		return nil, utils.Error("BUG: act conn is nil")
	}

	// 告诉客户端已成功连接到目标服务器
	reply := []byte{socks5Version, replySuccess, 0x00}
	host, _, _ := utils.ParseStringToHostPort(actConn.RemoteAddr().String())
	if utils.IsIPv4(host) {
		reply = append(reply, addrTypeIPv4)
		reply = append(reply, net.ParseIP(host).To4()...)
		reply = append(reply, portsBytes...)
	} else {
		reply = append(reply, addrTypeIPv4)
		reply = append(reply, 0x00, 0x00, 0x00, 0x00)
		reply = append(reply, portsBytes...)
	}

	conn.Write(reply)
	return actConn, nil
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
	var err error
	conn, err = c.Handshake(conn)
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
