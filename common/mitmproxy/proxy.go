package mitmproxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type MITMProxy struct {
	config *Config

	dialer           *net.Dialer
	activeTotalMutex *sync.Mutex
	activeTotal      int64
}

func (m *MITMProxy) AddConnectionCount() {
	m.activeTotalMutex.Lock()
	defer m.activeTotalMutex.Unlock()
	m.activeTotal++
}

func (m *MITMProxy) SubConnectionCount() {
	m.activeTotalMutex.Lock()
	defer m.activeTotalMutex.Unlock()
	m.activeTotal--
}

var initMITMOnce = new(sync.Once)

func GetMITMCACert() ([]byte, []byte, error) {
	initMITMOnce.Do(func() {
		crep.InitMITMCert()
	})
	ca, key, err := crep.GetDefaultCaAndKey()
	if err != nil {
		return nil, nil, err
	}
	return ca, key, nil
}

func NewMITMProxy(opt ...Option) (*MITMProxy, error) {
	config, err := NewConfig(opt...)
	if err != nil {
		return nil, utils.Errorf("generate config failed: %s", err)
	}
	proxy := &MITMProxy{config: config, activeTotalMutex: new(sync.Mutex)}
	if config.Timeout <= 0 {
		config.Timeout = 10 * time.Second
	}
	proxy.dialer = &net.Dialer{
		Timeout: config.Timeout,
	}
	proxy.config.mitmConfig.SkipTLSVerify(true)

	return proxy, nil
}

func (m *MITMProxy) Run(ctx context.Context) error {
	addr := utils.HostPort(m.config.Host, m.config.Port)
	log.Infof("start to serve on proxy http://%v", addr)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return utils.Errorf("create tcp listener [tcp://%v] failed: %v", addr, err)
	}
	currentCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()

		select {
		case <-currentCtx.Done():
			lis.Close()
		}
	}()
	for {
		select {
		case <-currentCtx.Done():
			return err
		default:
		}

		conn, err := lis.Accept()
		if err != nil {
			log.Errorf("accept mitm error: %s", err)
			continue
		}
		//log.Infof("accept from %v => %v total: %v", conn.RemoteAddr(), conn.LocalAddr(), m.activeTotal)
		m.AddConnectionCount()
		go m.serve(conn)
	}
}

func (m *MITMProxy) serve(conn net.Conn) {
	defer func() {
		m.SubConnectionCount()
		log.Infof("connection from: %s closed, now active conns: %v", conn.RemoteAddr(), m.activeTotal)
		conn.Close()
	}()

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("panic from conn: %v: %v", conn.RemoteAddr(), err)
			return
		}
	}()

	// 透明模式和非透明模式处理方式不一样
	// 透明模式先不管了
	if m.config.TransparentMode {
		panic("transparent mode failed: not implemented")
	}

	// 下面是代理模式的实现过程
	// 代理首先会有一个 "CONNECT" 过来，所以需要针对这个进行处理，一般回复一个 HTTP/1.1 200 Established\r\n\r\n
	var firstRequestMirrorBytes bytes.Buffer
	originReader := bufio.NewReader(io.TeeReader(conn, &firstRequestMirrorBytes))
	httpRequest, err := lowhttp.ReadHTTPRequest(originReader)
	if err != nil {
		log.Errorf("read request from[%v] failed: %s", conn.RemoteAddr(), err)
		return
	}

	if httpRequest.Method == "CONNECT" {
		conn.SetDeadline(time.Now().Add(m.config.Timeout))
		// 如果是 CONNECT 给人家回一个 Established，这种一般是开隧道用的
		// 开隧道的情况一般用在 HTTPS 或者其他隧道上，兼容 HTTP 隧道
		_, err = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		if err != nil {
			log.Errorf("write CONNECT response to %v failed: %s", conn.RemoteAddr(), err)
			return
		}
		m.connect(httpRequest, conn)
		return
	} else {
		host, port, err := utils.ParseStringToHostPort(httpRequest.RequestURI)
		if err != nil {
			log.Errorf("parse request uri failed: %s", httpRequest.RequestURI)
			// 如果不带这个头，我们暂且认为他是一个 webhook
			if m.config.webhookCallback != nil {
				raw := m.config.webhookCallback(httpRequest)
				if raw == nil {
					conn.Write([]byte("HTTP/1.1 200 Ok\r\nContent-Length: 0\r\n\r\n"))
				} else {
					conn.Write(raw)
				}
			}
			return
		}

		addr := utils.HostPort(host, port)
		//log.Infof("start to create new conn: %v tls:false", addr)
		newConn, err := m.newConnFor(addr, false, "")
		if err != nil {
			log.Errorf("create new conn to %v failed: %s", addr, err)
			return
		}
		defer newConn.Close()

		httpRequest.RequestURI = httpRequest.URL.RequestURI()
		if httpRequest.Body != nil {
			body, _ := ioutil.ReadAll(httpRequest.Body)
			httpRequest.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		}

		if m.config.mirrorRequestCallback != nil {
			m.config.mirrorRequestCallback(httpRequest)
		}

		rawRequest := firstRequestMirrorBytes.Bytes()
		var keepAlive = !httpRequest.Close

		header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rawRequest)
		header = strings.Join(funk.Filter(strings.Split(header, "\r\n"), func(i string) bool {
			lower := strings.ToLower(strings.TrimSpace(i))
			//if utils.MatchAllOfSubString(lower, "keep-alive") {
			//	keepAlive = true
			//}
			if strings.HasPrefix(lower, "proxy-") {
				return false
			}
			return true
		}).([]string), "\r\n")
		buffer := bytes.NewBufferString(header)
		buffer.Write(body)
		rawRequest = buffer.Bytes()

		// 劫持请求
		if m.config.hijackRequestCallback != nil {
			rawRequest = m.config.hijackRequestCallback(false, httpRequest, rawRequest)
		}
		if rawRequest == nil {
			return
		}
		newConn.Write(rawRequest)

		// 劫持响应
		err = mirrorResponse(httpRequest, newConn, func(i []byte) []byte {
			if m.config.hijackResponseCallback == nil {
				return i
			}
			return m.config.hijackResponseCallback(false, httpRequest, i, m.remoteAddrConvert(newConn.RemoteAddr().String()))
		}, conn, func(r *http.Response) {
			if m.config.mirrorResponseCallback != nil {
				m.config.mirrorResponseCallback(false, httpRequest, r, m.remoteAddrConvert(newConn.RemoteAddr().String()))
			}
		})
		if err != nil {
			log.Errorf("mirror response failed: %s", err)
			return
		}

		if !keepAlive {
			return
		}
		m.handleHTTP(conn, newConn, keepAlive, false)
	}
}

func (m *MITMProxy) connect(httpRequest *http.Request, conn net.Conn) {
	host := httpRequest.URL.Host
	//log.Infof("%v CONNECTed %v start to peek first byte to identify https/tls", conn.RemoteAddr(), host)
	var connKeepalive = !httpRequest.Close

	originConnPeekable := utils.NewPeekableNetConn(conn)
	raw, err := originConnPeekable.Peek(1)
	if err != nil {
		log.Errorf("peek [%v] failed: %s", conn.RemoteAddr(), err)
		return
	}
	isHttps := utils.NewAtomicBool()
	var originHttpConn net.Conn
	var sni string
	switch raw[0] {
	case 0x16:
		// HTTPS 升级，这是核心步骤
		//log.Infof("upgrade/hijacked (%v)%v to tls(https)", httpRequest.Host, conn.RemoteAddr())
		tconn := tls.Server(originConnPeekable, m.config.mitmConfig.TLSForHost(httpRequest.Host))
		err := tconn.Handshake()
		if err != nil {
			log.Errorf("upgrade tls error! handshake failed: %s", err)
			return
		}
		originHttpConn = tconn
		sni = tconn.ConnectionState().ServerName
		isHttps.Set()
	default:
		// HTTP
		log.Infof("recognized %v as http", conn.RemoteAddr())
		originHttpConn = originConnPeekable
		isHttps.UnSet()
	}

	newConn, err := m.newConnFor(host, isHttps.IsSet(), sni)
	if err != nil {
		log.Errorf("create new conn to(connect) %v failed: %s", host, err)
		return
	}
	defer newConn.Close()

	m.handleHTTP(originHttpConn, newConn, connKeepalive, isHttps.IsSet())
}

func (m *MITMProxy) handleHTTP(in net.Conn, out net.Conn, keepalive bool, isHttps bool) {
	for {
		//in.SetDeadline(time.Now().Add(m.config.Timeout))
		//out.SetDeadline(time.Now().Add(m.config.Timeout))

		var req *http.Request
		var rsp *http.Response
		_ = rsp
		err := mirrorRequest(in, func(req *http.Request, i []byte) []byte {
			if m.config.hijackRequestCallback == nil {
				return i
			}
			return m.config.hijackRequestCallback(isHttps, req, i)
		}, out, func(r *http.Request) {
			req = r
		}, func(r *http.Request) {
			if m.config.mirrorRequestCallback != nil {
				m.config.mirrorRequestCallback(r)
			}
		})
		if err != nil {
			log.Debugf("mirror request failed: %s", err)
			return
		}

		err = mirrorResponse(req, out, func(i []byte) []byte {
			if m.config.hijackResponseCallback == nil {
				return i
			}
			return m.config.hijackResponseCallback(isHttps, req, i, m.remoteAddrConvert(out.RemoteAddr().String()))
		}, in, func(r *http.Response) {
			rsp = r
		}, func(r *http.Response) {
			if m.config.mirrorResponseCallback != nil {
				m.config.mirrorResponseCallback(false, req, r, m.remoteAddrConvert(out.RemoteAddr().String()))
			}
		})
		if err != nil {
			log.Debugf("mirror response failed: %s", err)
			return
		}

		if !keepalive {
			log.Errorf("close by connection for %v <=> %v", in.RemoteAddr(), in.RemoteAddr())
			return
		}
	}
}

func (m *MITMProxy) remoteAddrConvert(remoteAddr string) string {
	if len(m.config.DownstreamProxy) <= 0 {
		return remoteAddr
	}
	value, ok := ttlCacheRemoteAddr.Get(remoteAddr)
	if ok {
		return fmt.Sprint(value)
	}
	return remoteAddr
}

var ttlCacheRemoteAddr = ttlcache.NewCache()

func init() {
	ttlCacheRemoteAddr.SetTTL(30 * time.Second)
}

func (m *MITMProxy) newConnFor(target string, isTls bool, sni string) (net.Conn, error) {
	host, port, err := utils.ParseStringToHostPort(target)
	if err != nil {
		host = target
		port = 80
	}
	originTarget := host
	if sni != "" {
		originTarget = sni
	}
	if !(utils.IsIPv4(host) || utils.IsIPv6(host)) {
		host = utils.GetFirstIPByDnsWithCache(host, 5*time.Second)
		if host == "" {
			return nil, utils.Errorf("dns error for %v", originTarget)
		}
		log.Debugf("dns: %v => %v", originTarget, host)
	}

	addr := utils.HostPort(host, port)
	if len(m.config.DownstreamProxy) <= 0 {
		if isTls {
			conn, err := tls.DialWithDialer(m.dialer, "tcp", addr, &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
				MaxVersion:         tls.VersionTLS13,
				ServerName:         originTarget,
			})
			if err != nil {
				return nil, utils.Errorf("dial tls to conn %v failed: %s", utils.HostPort(host, port), err)
			}
			return conn, nil
		} else {
			conn, err := m.dialer.Dial("tcp", addr)
			if err != nil {
				return nil, err
			}
			return conn, nil
		}
	}

	conn, err := utils.TCPConnect(addr, m.config.Timeout, m.config.DownstreamProxy...)
	if err != nil {
		return nil, utils.Errorf("dial remote[%v] failed: %s", addr, err)
	}
	ttlCacheRemoteAddr.Set(conn.RemoteAddr().String(), addr)

	if isTls {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: originTarget,
			MinVersion: tls.VersionSSL30,
			MaxVersion: tls.VersionTLS13,
		})
		err := tlsConn.Handshake()
		if err != nil {
			log.Errorf("tls [=>%v] handshake failed: %s", addr, err)
		}
		//ctx, cancel := context.WithTimeout(context.Background(), m.config.Timeout)
		//defer cancel()
		//err := tlsConn.HandshakeContext(ctx)
		//if err != nil {
		//	return nil, utils.Errorf("tls client[->%v] handshake error: %s", addr, err)
		//}
		return tlsConn, nil
		//conn, err := tls.DialWithDialer(m.dialer, "tcp", utils.HostPort(host, port), )
		//if err != nil {
		//	return nil, utils.Errorf("dial tls to conn %v failed: %s", utils.HostPort(host, port), err)
		//}
		//return conn, nil
	}
	return conn, nil
}
