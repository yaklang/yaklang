package s5

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/martian/v3/mitm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type S5Config struct {
	HandshakeTimeout    time.Duration
	HijackMode          bool
	SkipHttp2           bool
	MITMTLSConfig       *mitm.Config
	Debug               bool
	DownstreamHTTPProxy string
}

func NewConfig() (*S5Config, error) {
	cert, priv, err := crep.GetDefaultCAAndPriv()
	if err != nil {
		return nil, utils.Errorf("get default ca and priv failed: %s", err)
	}

	tlsMITMConfig, err := mitm.NewConfig(cert, priv)
	if err != nil {
		return nil, utils.Errorf("new mitm config failed: %s", err)
	}
	return &S5Config{
		HandshakeTimeout: 30 * time.Second,
		HijackMode:       true,
		MITMTLSConfig:    tlsMITMConfig,
		SkipHttp2:        true,
	}, nil
}

func (h *S5Config) TLSConfigFromSNI(i string) *tls.Config {
	return h.MITMTLSConfig.TLSForHost(i, h.SkipHttp2)
}

func (h *S5Config) IsHijackMode() bool {
	if h == nil {
		return false
	}
	return h.HijackMode
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

func (c *S5Config) IsSocks5HandleShake(conn net.Conn) (net.Conn, bool, error) {
	peekable := utils.NewPeekableNetConn(conn)
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
	var handshakeTimeout = 3 * time.Second

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

	if needpassword {
		return utils.Error("not implement auth pass")
	}

	conn.Write([]byte{socks5Version, authNone})
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
	if host, _, _ := utils.ParseStringToHostPort(actConn.RemoteAddr().String()); utils.IsIPv4(host) {
		reply = append(reply, addrTypeIPv4)
		reply = append(reply, net.ParseIP(host).To4()...)
		reply = append(reply, portsBytes...)
	} else {
		reply = append(reply, addrBytesRaw...)
	}

	conn.Write(reply)
	return actConn, nil
}

func (c *S5Config) ServeConn(conn net.Conn) error {
	defer func() {
		conn.Close()
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
	var sni string
	var (
		/* client hello 是否被解密 */
		clientHelloHacked bool
		alpnContainsHttp  bool
		wrapHttp          bool
	)

	if c.IsHijackMode() {
		peekableSrc := utils.NewPeekableNetConn(src)
		src = peekableSrc

		// 尝试劫持 TLS 握手包
		firstByte, err := peekableSrc.Peek(1)
		if err != nil {
			return nil, nil, false, err
		}

		if len(firstByte) == 0 {
			return nil, nil, false, utils.Error("peek first byte failed")
		}
		switch firstByte[0] {
		case 0x16:
			// TLS 握手包
			results, err := peekableSrc.Peek(6)
			if err != nil {
				log.Errorf("peek tls handshake failed: %s", err)
				break
			}
			var length = binary.BigEndian.Uint16([]byte{results[3], results[4]})
			totalLen := 5 + int(length)
			log.Infof("start to fetch tls handshake, total len: %d", totalLen)

			fullClientHello, err := peekableSrc.Peek(totalLen)
			if err != nil {
				log.Errorf("peek tls handshake failed: %s", err)
				break
			}

			client, err := tlsutils.ParseClientHello(fullClientHello)
			if err != nil {
				log.Errorf("parse client hello failed: %s", err)
				break
			}

			if client != nil {
				sni = client.SNI()
				clientHelloHacked = true
				alpnContainsHttp = client.MaybeHttp()
			}

			// TLS 连接成功，直接握手
			var tlsProxiedConn = tls.Client(proxiedConn, utils.NewDefaultTLSConfig())
			err = tlsProxiedConn.Handshake()
			if err != nil {
				return nil, nil, false, utils.Errorf("tls handshake to %v failed: %s", proxiedConn.RemoteAddr().String(), err)
			}
			proxiedConn = tlsProxiedConn

			if clientHelloHacked {
				if sni != "" {
					log.Infof("checking sni: %v", sni)
				} else {
					log.Infof("tls connection without sni. maybe not browser...")
				}

				if sni == "" {
					if ipAddr, _, _ := utils.ParseStringToHostPort(proxiedConn.RemoteAddr().String()); utils.IsIPv4(ipAddr) {
						sni = ipAddr
					} else if utils.IsIPv6(ipAddr) {
						sni = ipAddr
					}
					log.Infof("sni is empty... fullback to ip: %v", sni)
				}

				serverConfig := c.TLSConfigFromSNI(sni)
				if serverConfig != nil {
					log.Infof("hijack tls connection, sni: %v", sni)
					tlsSrc := tls.Server(src, serverConfig)
					if err := tlsSrc.Handshake(); err != nil {
						return nil, nil, false, utils.Errorf("hijack tls handshake failed: %s", err)
					}
					src = tlsSrc
					log.Infof("tls source conn: %s is off", src.RemoteAddr().String())
				}
			}
		default:
			raw, _ := peekableSrc.Peek(3)
			switch strings.ToUpper(string(raw)) {
			case "GET", "HEA", "POS", "DEL", "PUT", "OPT", "PAT", "CON", "TRA":
				wrapHttp = true
			default:
			}
		}
	}

	if (clientHelloHacked && alpnContainsHttp) || wrapHttp {
		// 这个是劫持 http 的标志
	}
	return src, proxiedConn, true, nil
}
