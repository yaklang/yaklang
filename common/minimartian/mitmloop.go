package minimartian

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/process"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/nosigpipe"
	"github.com/yaklang/yaklang/common/minimartian/proxyutil"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

var IsDroppedError = utils.Error("dropped")

func (p *Proxy) startConnLog(statusContext context.Context) (func(string, net.Conn), func(string, net.Conn)) {
	var currentConnCount int64 = 0
	// 设置缓存并清除
	connsCached := new(sync.Map)
	cacheConns := func(uid string, c net.Conn) {
		connsCached.Store(uid, c)
		atomic.AddInt64(&currentConnCount, 1)
		log.Debugf("record connection from cache: %v=>%v, current count: %v", c.LocalAddr(), c.RemoteAddr(), currentConnCount)
	}
	removeConns := func(uid string, c net.Conn) {
		connsCached.Delete(uid)
		atomic.AddInt64(&currentConnCount, -1)
		if c == nil {
			log.Debugf("remove connection table from cache: %v=>%v, current coon: %v", c.LocalAddr(), c.RemoteAddr(), currentConnCount)
		}
	}

	go func() {
		var lastCurrentConnCount int64
		for {
			select {
			case <-statusContext.Done():
				return
			default:
				if currentConnCount > 0 && lastCurrentConnCount != currentConnCount {
					log.Infof("mitm frontend active connections count: %v", currentConnCount)
					lastCurrentConnCount = currentConnCount
				}
				time.Sleep(3 * time.Second)
			}
		}
	}()

	go func() {
		defer func() {
			count := 0
			connsCached.Range(func(key, value interface{}) bool {
				count++
				connIns, ok := value.(net.Conn)
				if ok && connIns != nil {
					log.Infof("closing remote addr: %s", connIns.RemoteAddr())
					connIns.Close()
				}
				return true
			})
			if count > 0 {
				log.Debugf("CONNECTION UNBALANCED: %v", count)
				log.Debugf("CONNECTION UNBALANCED: %v", count)
				log.Debugf("CONNECTION UNBALANCED: %v", count)
				log.Debugf("CONNECTION UNBALANCED: %v", count)
			}
		}()
		for {
			select {
			case <-statusContext.Done():
				log.Info("closing martian proxying...")
				return
			default:
				if p.Closing() {
					return
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()
	return cacheConns, removeConns
}

// Serve accepts connections from the listener and handles the requests.
func (p *Proxy) Serve(l net.Listener, baseCtx context.Context) error {
	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	defer l.Close()
	s5config := NewSocks5Config()

	host, port, err := utils.ParseStringToHostPort(l.Addr().String())
	if err != nil {
		return err
	}
	if host == "0.0.0.0" || host == `[::]` {
		host = "127.0.0.1"
	}
	s5config.DownstreamHTTPProxy = "http://" + utils.HostPort(host, port)
	s5config.ProxyPassword = p.proxyPassword
	s5config.ProxyUsername = p.proxyUsername
	if s5config.ProxyPassword != "" || s5config.ProxyUsername != "" {
		urlIns, err := url.Parse(s5config.DownstreamHTTPProxy)
		if err != nil {
			return utils.Errorf("parse s5 downstream url failed, err: %v", err)
		}
		urlIns.User = url.UserPassword(s5config.ProxyUsername, s5config.ProxyPassword)
		s5config.DownstreamHTTPProxy = urlIns.String()
	}

	statusContext, statusCancel := context.WithCancel(ctx)
	defer statusCancel()
	cacheConns, removeConns := p.startConnLog(statusContext)

	var delay time.Duration

	log.Infof("(mitm) ready for recv connection from: %v", l.Addr().String())

	incomming := make(chan *WrapperedConn)
	wg := new(sync.WaitGroup)
	wg.Add(2)
	// start listener to start accept connection
	go func() {
		defer func() {
			wg.Done()
			cancel()
		}()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			conn, err := l.Accept()
			if err != nil {
				if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
					if delay == 0 {
						delay = 5 * time.Millisecond
					} else {
						delay *= 2
					}
					if max := time.Second; delay > max {
						delay = max
					}

					log.Debugf("mitm: temporary error on accept: %v", err)
					time.Sleep(delay)
					continue
				}
				log.Errorf("mitm: failed to accept: %v", err)
				return
			}
			// Wrap the accepted connection
			wrapped := NewWrapperedConn(conn, false, nil)
			select {
			case incomming <- wrapped:
			case <-ctx.Done():
				conn.Close()
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case wrappedConn, ok := <-p.extraIncomingConnCh:
				if !ok {
					return
				}
				select {
				case incomming <- wrappedConn:
				case <-ctx.Done():
					wrappedConn.Close()
					return
				}
			}
		}
	}()
	defer wg.Wait()

	for {
		if p.Closing() {
			l.Close()
			return nil
		}

		var wrappedConn *WrapperedConn
		select {
		case <-ctx.Done():
			log.Info("closing martian proxying...")
			l.Close()
			return nil
		case rawConn, ok := <-incomming:
			if !ok {
				return utils.Errorf("incomming channel closed")
			}
			wrappedConn = rawConn
		}

		if wrappedConn == nil || wrappedConn.Conn == nil {
			continue
		}

		conn := wrappedConn.Conn

		// generate ksuid
		uid := ksuid.New().String()

		nosigpipe.IgnoreSIGPIPE(conn)
		select {
		case <-ctx.Done():
			conn.Close()
			log.Info("closing martian proxying...")
			l.Close()
			return nil
		default:
			cacheConns(uid, conn)
		}
		delay = 0

		log.Debugf("mitm: accepted connection from %s", conn.RemoteAddr())
		go func(uidStr string, originConn net.Conn, wrapped *WrapperedConn) {
			subCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("handle mitm proxy loop failed: %s", err)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
				if originConn != nil {
					originConn.Close()
				}
			}()
			defer removeConns(uidStr, originConn)

			handledConnection, isS5, firstByte, err := IsSocks5HandleShake(originConn)
			if err != nil {
				log.Errorf("check socks5 handle shake failed: %s", err)
				return
			}
			isTls := firstByte == 0x16
			proxyContext, err := CreateProxyHandleContext(subCtx, handledConnection)
			if err != nil {
				log.Error(err)
				return
			}

			// Apply metaInfo and strongHostMode from wrapperedConn to session
			if wrapped != nil {
				session := proxyContext.Session()
				if wrapped.IsStrongHostMode() {
					session.Set("StrongHostMode", true)
					// Get localAddr from WrapperedConn (required for strong host mode)
					localAddr := wrapped.GetStrongHostLocalAddr()
					if localAddr != "" {
						session.Set("StrongHostLocalAddr", localAddr)
					}
				}
				metaInfo := wrapped.GetMetaInfo()
				if len(metaInfo) > 0 {
					session.Set("ConnMetaInfo", metaInfo)
					// Also set individual meta info keys for easy access
					for k, v := range metaInfo {
						session.Set("ConnMetaInfo_"+k, v)
					}
				}
			}

			if isS5 {
				dstHost, dstPort, err := s5config.ServerConnect(handledConnection)
				if err != nil {
					log.Errorf("server s5 connect failed: %s", err)
					return
				}
				handledConnection, isTls, err = IsTlsHandleShake(handledConnection)
				if err != nil {
					log.Errorf("check tls handle shake failed: %s", err)
					return
				}
				proxyContext, err = CreateProxyHandleContext(subCtx, handledConnection)
				if err != nil {
					log.Error(err)
					return
				}
				// Re-apply metaInfo after recreating context
				if wrapped != nil {
					session := proxyContext.Session()
					if wrapped.IsStrongHostMode() {
						session.Set("StrongHostMode", true)
						// Get localAddr from WrapperedConn (required for strong host mode)
						localAddr := wrapped.GetStrongHostLocalAddr()
						if localAddr != "" {
							session.Set("StrongHostLocalAddr", localAddr)
						}
					}
					metaInfo := wrapped.GetMetaInfo()
					if len(metaInfo) > 0 {
						session.Set("ConnMetaInfo", metaInfo)
						for k, v := range metaInfo {
							session.Set("ConnMetaInfo_"+k, v)
						}
					}
				}
				sessionBindConnectTo(proxyContext.Session(), PROTO_S5, dstHost, dstPort)
			}
			p.handleLoop(isTls, handledConnection, proxyContext)
		}(uid, conn, wrappedConn)
	}
}

var cachedTLSConfig *tls.Config

func (p *Proxy) defaultTLSConfig() *tls.Config {
	if cachedTLSConfig != nil {
		cachedTLSConfig = &tls.Config{
			InsecureSkipVerify: true,
			GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
				var hostname string
				if info.ServerName != "" {
					hostname = info.ServerName
				} else {
					hostname = "127.0.0.1"
				}
				return p.mitm.GetCertificateByHostname(hostname)
			},
			NextProtos: make([]string, 0),
		}
	}
	return cachedTLSConfig
}

func (p *Proxy) handleLoop(isTLSConn bool, conn net.Conn, ctx *Context) {
	if conn == nil {
		return
	}

	p.connsMu.Lock()
	if p.Closing() { // protect closing,avoid add after close
		return
	}
	p.conns.Add(1)
	p.connsMu.Unlock()

	defer p.conns.Done()
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("handle proxy loop failed: %s", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	s := ctx.Session()
	brw := s.brw

	/* TLS */
	if isTLSConn {
		s.MarkSecure()
		s.Set(httpctx.REQUEST_CONTEXT_ConnectToHTTPS, true)
		var serverUseH2 bool
		if p.http2 {
			// does remote server use h2?
			var proxyStr string
			if p.proxyURL != nil {
				proxyStr = p.proxyURL.String()
			}

			// Check the cache first.
			cacheKey := utils.HostPort(ctx.GetSessionStringValue(httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost), ctx.GetSessionIntValue(httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort))
			if cached, ok := p.h2Cache.Load(cacheKey); ok {
				log.Infof("use cached h2 %v", cacheKey)
				serverUseH2 = cached.(bool)
			} else {
				// TODO: should connect every connection?
				netConn, _ := netx.DialX(
					cacheKey,
					netx.DialX_WithTimeout(10*time.Second),
					netx.DialX_WithProxy(proxyStr),
					netx.DialX_WithForceProxy(proxyStr != ""),
					netx.DialX_WithTLSNextProto("h2"),
					netx.DialX_WithTLS(true),
					netx.DialX_WithDialer(p.dialer),
				)
				if netConn != nil {
					switch ret := netConn.(type) {
					case *tls.Conn:
						if ret.ConnectionState().NegotiatedProtocol == "h2" {
							serverUseH2 = true
						}
					case *gmtls.Conn:
						if ret.ConnectionState().NegotiatedProtocol == "h2" {
							serverUseH2 = true
						}
					}
					netConn.Close()
				}

				// Store the result in the cache.
				p.h2Cache.Store(cacheKey, serverUseH2)
			}
		}
		tlsConn, useH2, err := p.TLSHandshake(utils.TimeoutContextSeconds(5), conn, serverUseH2)
		if err != nil {
			log.Errorf("tls handshake faile:%v", err)
			return
		}
		if useH2 {
			err := p.proxyH2(p.closing, tlsConn, nil, ctx)
			if err != nil {
				log.Errorf("mitm proxy h2 failed: %v", err)
			}
			return
		}
		brw.Writer.Reset(tlsConn)
		brw.Reader.Reset(tlsConn)
		conn = tlsConn
	}

	/* handle cleaning proxy! */

	timerInterval := time.Second * 10
	var timer *time.Timer
	for {
		conn.SetDeadline(time.Time{})
		log.Debugf("waiting conn: %v", conn.RemoteAddr())
		err := p.handle(ctx, timer, conn, brw)
		if timer == nil {
			timer = time.AfterFunc(timerInterval, func() {
				conn.Close()
			})
		} else {
			timer.Reset(timerInterval)
		}

		if err != nil {
			if isCloseable(err) {
				log.Debugf("closing conn(%v): %v", err, conn.RemoteAddr())
				return
			} else {
				log.Infof("continue read conn: %v with err: %v", conn.RemoteAddr(), err)
			}
		}
	}
}

// handleConnectionTunnel handles a CONNECT request.
func (p *Proxy) handleConnectionTunnel(req *http.Request, timer *time.Timer, conn net.Conn, ctx *Context, session *Session, brw *bufio.ReadWriter, connectedTo string) error {
	var err error

	httpctx.SetRequestViaCONNECT(req, true)
	// set session ctx, session > httpctx
	parsedConnectedToPort := httpctx.GetContextIntInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort)
	parsedConnectedToHost := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost)
	sessionBindConnectTo(session, PROTO_HTTP, parsedConnectedToHost, parsedConnectedToPort)
	session.Set(httpctx.REQUEST_CONTEXT_KEY_ViaConnect, true)

	if err := p.reqmod.ModifyRequest(req); err != nil {
		if !strings.Contains(err.Error(), "ignore connect") {
			log.Errorf("mitm: error modifying CONNECT request: %v", err)
			proxyutil.Warning(req.Header, err)
		}
	}
	if session.Hijacked() {
		log.Debugf("mitm: connection hijacked by request modifier")
		return nil
	}

	if p.mitm == nil {
		conn.Close()
		return utils.Errorf("mitm: no MITM config set for CONNECT request: %s", req.Host)
	}

	log.Debugf("mitm: attempting MITM for connection: %s", req.Host)
	/*
		return a const response connection established...
	*/
	res := p.connectResponse(req)

	if err := p.resmod.ModifyResponse(res); err != nil {
		log.Errorf("mitm: error modifying CONNECT response: %v", err)
		proxyutil.Warning(res.Header, err)
	}
	if session.Hijacked() {
		log.Debugf("mitm: connection hijacked by response modifier")
		return nil
	}
	var responseBytes []byte
	responseBytes, err = utils.DumpHTTPResponse(res, true, brw)
	_ = responseBytes
	if err != nil {
		log.Errorf("CONNECT Request: got error while writing response back to client: %v", err)
	}
	if err := brw.Flush(); err != nil {
		log.Errorf("CONNECT Request: got error while flushing response back to client: %v", err)
	}

	log.Debugf("mitm: completed MITM for connection: %s", req.Host)

	var isTLS bool
	conn, isTLS, err = IsTlsHandleShake(conn)
	if err != nil {
		return err
	}
	// 22 is the TLS handshake.
	isTLS = isTLS || httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_ConnectToHTTPS)
	session.Set(httpctx.REQUEST_CONTEXT_ConnectToHTTPS, isTLS)
	if parsedConnectedToPort == 0 {
		if isTLS {
			parsedConnectedToPort = 443
		} else {
			parsedConnectedToPort = 80
		}
		session.Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort, parsedConnectedToPort)
	}

	// https://tools.ietf.org/html/rfc5246#section-6.2.1
	if isTLS {
		session.MarkSecure()

		var serverUseH2 bool
		if p.http2 {
			// does remote server use h2?
			var proxyStr string
			if p.proxyURL != nil {
				proxyStr = p.proxyURL.String()
			}

			// Check the cache first.
			cacheKey := utils.HostPort(parsedConnectedToHost, parsedConnectedToPort)
			if cached, ok := p.h2Cache.Load(cacheKey); ok {
				log.Infof("use cached h2 %v", cacheKey)
				serverUseH2 = cached.(bool)
			} else {
				// TODO: should connect every connection?
				netConn, _ := netx.DialX(
					cacheKey,
					netx.DialX_WithTimeout(10*time.Second),
					netx.DialX_WithProxy(proxyStr),
					netx.DialX_WithForceProxy(proxyStr != ""),
					netx.DialX_WithTLSNextProto("h2"),
					netx.DialX_WithTLS(true),
					netx.DialX_WithDialer(p.dialer),
				)
				if netConn != nil {
					switch ret := netConn.(type) {
					case *tls.Conn:
						if ret.ConnectionState().NegotiatedProtocol == "h2" {
							serverUseH2 = true
						}
					case *gmtls.Conn:
						if ret.ConnectionState().NegotiatedProtocol == "h2" {
							serverUseH2 = true
						}
					}
					netConn.Close()
				}

				// Store the result in the cache.
				p.h2Cache.Store(cacheKey, serverUseH2)
			}
		}

		// fallback: 最普通的情况，没有任何 http2 支持
		// do as ordinary https server and use *tls.Conn
		tlsConn, useH2, err := p.TLSHandshake(utils.TimeoutContextSeconds(5), conn, serverUseH2)
		if err != nil {
			p.mitm.HandshakeErrorCallback(req, err)
			return utils.Errorf("tls handshake faile:%v", err)
		}
		if useH2 {
			return p.proxyH2(p.closing, tlsConn, nil, ctx)
		}
		brw.Writer.Reset(tlsConn)
		brw.Reader.Reset(tlsConn)
		return p.handle(ctx, timer, tlsConn, brw)
	}
	// -> Client Connection <- is plain HTTP connection
	// Prepend the previously read data to be read again.
	brw.Reader.Reset(conn)
	return p.handle(ctx, timer, conn, brw) // should read next request from client
}

func (p *Proxy) handle(ctx *Context, timer *time.Timer, conn net.Conn, brw *bufio.ReadWriter) error {
	log.Debugf("mitm: waiting for request: %v", conn.RemoteAddr())
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("handle proxy request panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	var req *http.Request
	reqc := make(chan *http.Request, 1)
	errc := make(chan error, 1)
	go func() {
		r, err := utils.ReadHTTPRequestFromBufioReaderOnFirstLine(brw.Reader, func(s string) {
			if timer != nil {
				timer.Stop()
			}
		})
		if err != nil {
			errc <- err
			return
		}

		if p.forceDisableKeepAlive && r != nil {
			r.Close = true
		}
		if timer != nil {
			timer.Stop()
		}
		reqc <- r
	}()
	select {
	case err := <-errc:
		if isCloseable(err) {
			log.Debugf("mitm: connection closed prematurely: %v", err)
			conn.Close()
		} else {
			log.Errorf("mitm: failed to read request: %v", err)
		}
		// TODO: TCPConn.WriteClose() to avoid sending an RST to the client.
		return errClose
	case req = <-reqc:
	case <-p.closing:
		return errClose
	}
	defer req.Body.Close()

	// set process name
	if p.findProcessName {
		_, name, err := process.FindProcessNameByConn(conn)
		if err != nil {
			log.Errorf("mitm: conn[%s->%s] failed to get process name: %v", conn.LocalAddr(), conn.RemoteAddr(), err)
		} else {
			httpctx.SetProcessName(req, name)
		}
	}

	session := ctx.Session()
	ctx, err := withSession(session)
	if err != nil {
		log.Errorf("mitm: failed to build new context: %v", err)
		return err
	}

	httpctx.SetMITMFrontendReadWriter(req, brw)

	link(req, ctx, p)
	defer unlink(req, p)

	// set plugin context
	httpctx.SetPluginContext(req, consts.NewPluginContext())

	// Set IsStrongHostMode in httpctx from session if enabled
	// This is critical for transparent hijacking of tun-generated data
	if ctx.GetSessionBoolValue("StrongHostMode") {
		httpctx.SetIsStrongHostMode(req, true)
		// Also set tag for backward compatibility and visibility
		existingTags := httpctx.GetFlowTags(req)
		hasTag := false
		for _, tag := range existingTags {
			if tag == "IsStrongHostMode" {
				hasTag = true
				break
			}
		}
		if !hasTag {
			existingTags = append(existingTags, "IsStrongHostMode")
			httpctx.SetFlowTags(req, existingTags)
		}

		// Get local IP address from session for strong host mode binding
		// This is set from WrapperedConn's GetStrongHostLocalAddr() method
		localAddrIP := ctx.GetSessionStringValue("StrongHostLocalAddr")

		// Set local IP address to httpctx if found
		if localAddrIP != "" {
			httpctx.SetContextValueInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_StrongHostLocalAddr, localAddrIP)
			log.Debugf("mitm: set StrongHostLocalAddr in httpctx: %s for request from extraIncomingConn: %v", localAddrIP, req.URL)
		} else {
			log.Debugf("mitm: strong host mode enabled but no StrongHostLocalAddr found in session for request: %v", req.URL)
		}

		log.Debugf("mitm: set IsStrongHostMode in httpctx for request from extraIncomingConn: %v", req.URL)
	}

	return p.handleProxyAuth(conn, req, timer, ctx) // mitm mode should process proxy proto
}

// handleProxyAuth handles proxy authentication.
func (p *Proxy) handleProxyAuth(conn net.Conn, req *http.Request, timer *time.Timer, ctx *Context) error {
	session := ctx.Session()
	brw := session.brw
	needAuth := (p.proxyUsername != "" || p.proxyPassword != "") && !ctx.GetSessionBoolValue(AUTH_FINISH)

	var isHttps bool
	if tconn, ok := conn.(*tls.Conn); ok { // check req self https or not
		session.MarkSecure()

		cs := tconn.ConnectionState()
		req.TLS = &cs
		isHttps = true
		httpctx.SetRequestHTTPS(req, true)
	} else if gmConn, ok := conn.(*gmtls.Conn); ok {
		session.MarkSecure()

		cs := gmConn.ConnectionState()
		req.TLS = &tls.ConnectionState{ // set simple message
			Version:            cs.Version,
			CipherSuite:        cs.CipherSuite,
			NegotiatedProtocol: cs.NegotiatedProtocol,
			ServerName:         cs.ServerName,
		}
		isHttps = true
		httpctx.SetRequestHTTPS(req, true)
	}

	if session.IsSecure() {
		log.Debugf("mitm: forcing HTTPS inside secure session")
	}

	req.RemoteAddr = conn.RemoteAddr().String()
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	if host == "" {
		host = ctx.GetSessionStringValue(httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost)
		port := ctx.GetSessionIntValue(httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort)
		if (isHttps && port != 443) || (!isHttps && port != 80) {
			host = utils.HostPort(host, port)
		}

		if host == "" {
			conn.Close()
			return utils.Errorf("mitm: no host (and not connect to) in request: \n%v\n\n", string(httpctx.GetBareRequestBytes(req)))
		}
	}

	if req.URL.Host == "" {
		req.URL.Host = host
	}
	if req.Host == "" {
		req.Host = host
	}

	if ctx.GetSessionBoolValue(httpctx.REQUEST_CONTEXT_KEY_ViaConnect) {
		httpctx.SetRequestViaCONNECT(req, true)
	}

	if needAuth {
		// 开启认证
		failed := func(reason string) error {
			res := proxyutil.NewResponse(407, http.NoBody, req)
			res.Status = "407 Authentication Required"
			res.Header.Set("Proxy-Authenticate", "Basic realm=\"yakit proxy\", charset=\"UTF-8\"")
			e := fmt.Errorf("reason: %v", reason)
			proxyutil.Warning(res.Header, e)
			_, err := utils.DumpHTTPResponse(res, true, brw)
			if err != nil {
				// never happen
				err = errors.Join(err, e)
				log.Errorf("got error while writing failed response back to client: %v", err)
			}
			brw.Flush()
			conn.Close()
			return e
		}
		if req.Header.Get("Proxy-Authorization") == "" {
			return failed("empty Proxy-Authorization Header")
		}

		proxyAuth := req.Header.Get("Proxy-Authorization")
		originProxyAuth := proxyAuth
		if proxyAuth != "" {
			proxyAuth = strings.Replace(proxyAuth, "Basic ", "", -1)
			proxyAuth, err := base64.StdEncoding.DecodeString(proxyAuth)
			if err != nil {
				return failed("decode Proxy-Authorization[" + originProxyAuth + "] Header failed")
			}
			user, pass := lowhttp.SplitHTTPHeader(string(proxyAuth))
			if !(user == p.proxyUsername && pass == p.proxyPassword) {
				// 认证失败
				return failed("username/password is not valid!")
			}
			session.Set(AUTH_FINISH, true)
		} else {
			return failed("empty Proxy-Authorization Header")
		}
	}
	connectedTo, err := p.setHTTPCtxConnectTo(req)
	if err != nil {
		conn.Close()
		return err
	}
	if req.Method == "CONNECT" { // handle connect request
		return p.handleConnectionTunnel(req, timer, conn, ctx, session, brw, connectedTo)
	}
	return p.handleRequest(conn, req, ctx)
}

// handleRequest handles an ordinary HTTP request.
func (p *Proxy) handleRequest(conn net.Conn, req *http.Request, ctx *Context) error {
	// Set IsStrongHostMode in httpctx from session if enabled (for tunnel mode and other paths)
	// This is critical for transparent hijacking of tun-generated data
	if ctx.GetSessionBoolValue("StrongHostMode") {
		httpctx.SetIsStrongHostMode(req, true)
		// Also set tag for backward compatibility and visibility
		existingTags := httpctx.GetFlowTags(req)
		hasTag := false
		for _, tag := range existingTags {
			if tag == "IsStrongHostMode" {
				hasTag = true
				break
			}
		}
		if !hasTag {
			existingTags = append(existingTags, "IsStrongHostMode")
			httpctx.SetFlowTags(req, existingTags)
		}
		log.Debugf("mitm: set IsStrongHostMode in httpctx for request in handleRequest: %v", req.URL)
	}

	if httpctx.GetRequestHTTPS(req) || ctx.GetSessionBoolValue(httpctx.REQUEST_CONTEXT_ConnectToHTTPS) {
		req.URL.Scheme = "https"
		httpctx.SetRequestHTTPS(req, true)
	}
	session := ctx.Session()
	brw := session.brw
	if err := p.reqmod.ModifyRequest(req); err != nil {
		log.Errorf("mitm: error modifying request: %v", err)
		proxyutil.Warning(req.Header, err)
	}
	if session.Hijacked() {
		log.Debugf("mitm: connection hijacked by request modifier")
		return nil
	}

	res, err := p.doHTTPRequest(ctx, req)
	if (err != nil && err != io.EOF) || res == nil {
		if strings.Contains(err.Error(), "no such host") {
			httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_NOLOG, true)
			res = proxyutil.NewResponse(200, strings.NewReader(proxyutil.GetPrettyErrorRsp(fmt.Sprintf("Unknown host: %s", req.Host))), req)
		} else {
			log.Debugf("mitm: failed to round trip: %v", err)
			res = proxyutil.NewResponse(502, nil, req)
			proxyutil.Warning(res.Header, err)
		}
	}
	defer func() {
		if res == nil {
			return
		}
		if res.Body == nil {
			return
		}
		res.Body.Close()
	}()

	if !httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_IsDropped) {
		err := p.resmod.ModifyResponse(res)
		if err != nil {
			if errors.Is(err, IsDroppedError) {
				res = proxyutil.NewResponseFromOldResponse(200, strings.NewReader(proxyutil.GetPrettyErrorRsp("响应被用户丢弃")), req, res)
			} else {
				log.Errorf("mitm: error modifying response: %v", err)
				proxyutil.Warning(res.Header, err)
			}
		}
	}

	if session.Hijacked() {
		log.Debugf("mitm: connection hijacked by response modifier")
		return nil
	}

	var closing error
	if req.Close || res.Close || p.Closing() || p.forceDisableKeepAlive {
		log.Debugf("mitm: received close request: %v", req.RemoteAddr)
		res.Close = true
		closing = errClose
	}

	if httpctx.GetMITMSkipFrontendFeedback(req) {
		// skip frontend feedback
		// if met this case, means that "response" is handled.
		err = brw.Flush()
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				closing = errClose
			} else if strings.Contains(err.Error(), `write: broken pipe`) {
				closing = errClose
			}
			return closing
		}
		return nil
	}

	var responseBytes []byte
	responseBytes, err = utils.DumpHTTPResponse(res, true, brw)
	_ = responseBytes
	if err != nil {
		log.Errorf("handle ordinary request: got error while writing response back to client: %v", err)
	}

	// Handle proxy getting stuck when upstream stops responding midway
	// see https://github.com/google/martian/pull/349
	if errors.Is(err, io.ErrUnexpectedEOF) {
		closing = errClose
	}

	err = brw.Flush()
	if err != nil {
		log.Errorf("handle ordinary request: got error while flushing response back to client: %v", err)
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		closing = errClose
	}
	if p.forceDisableKeepAlive { // if http force close ,  just use only once
		conn.Close()
	}

	return closing
}

func (p *Proxy) TLSHandshake(ctx context.Context, conn net.Conn, serverUseH2 bool) (net.Conn, bool, error) {
	peekConn, version, err := peekTLSVersion(conn) // !!!!!! should use peekdConn for handshake!!!
	if err != nil {
		return nil, false, utils.Errorf("peek tls conn from client falied: %v", err)
	}
	var newConn net.Conn
	var useH2 bool
	if version < tls.VersionTLS12 {
		config := p.mitm.ObsoleteTLS("127.0.0.1", p.http2 && serverUseH2)
		config.GMSupport = &gmtls.GMSupport{WorkMode: gmtls.ModeAutoSwitch}
		tlsConn := gmtls.Server(peekConn, config)
		if tlsConn != nil {
			err := tlsConn.HandshakeContext(ctx)
			if err != nil {
				return nil, false, utils.Errorf("mitm recv tls conn from client, but handshake error: %v", err)
			}
			useH2 = tlsConn.ConnectionState().NegotiatedProtocol == "h2"
			newConn = tlsConn
		}
	} else {
		tlsConn := tls.Server(peekConn, p.mitm.TLSForHost("127.0.0.1", p.http2 && serverUseH2))
		if tlsConn != nil {
			err = tlsConn.HandshakeContext(ctx)
			if err != nil {
				return nil, false, utils.Errorf("mitm recv tls conn from client, but handshake error: %v", err)
			}
			useH2 = tlsConn.ConnectionState().NegotiatedProtocol == "h2"
			newConn = tlsConn
		}
	}
	return newConn, useH2, nil
}
