package minimartian

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/process"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/nosigpipe"
	"github.com/yaklang/yaklang/common/minimartian/proxyutil"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

var IsDroppedError = utils.Error("dropped")

const (
	S5_CONNECT_HOST = "S5ConnectHost"
	S5_CONNECT_PORT = "S5ConnectPort"
	S5_CONNECT_ADDR = "S5ConnectAddr"
	AUTH_FINISH     = "authFinish"
)

// Serve accepts connections from the listener and handles the requests.
func (p *Proxy) Serve(l net.Listener, ctx context.Context) error {
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

	statusContext, cancel := context.WithCancel(ctx)
	defer cancel()

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
			case <-ctx.Done():
				log.Info("closing martian proxying...")
				l.Close()
				return
			default:
				if p.Closing() {
					l.Close()
					return
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()

	var delay time.Duration

	log.Infof("(mitm) ready for recv connection from: %v", l.Addr().String())
	for {
		if p.Closing() {
			return nil
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
			return err
		}
		if conn == nil {
			continue
		}

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
		go func(uidStr string, originConn net.Conn) {
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
			var isS5 bool
			var handledConnection net.Conn
			var firstByte byte
			handledConnection, isS5, firstByte, err = IsSocks5HandleShake(originConn)
			if err != nil {
				removeConns(uidStr, originConn)
				log.Errorf("check socks5 handle shake failed: %s", err)
				return
			}
			isTls := firstByte == 0x16

			defer func() {
				removeConns(uidStr, handledConnection)
			}()

			if isS5 {
				err := s5config.Handshake(handledConnection)
				if err != nil {
					log.Errorf("socks5 Handshake failed: %s", err)
					return
				}
				req, err := s5config.HandleS5RequestHeader(handledConnection)
				if err != nil {
					log.Errorf("socks5 handle request failed: %s", err)
					return
				}
				if req.Cmd != commandConnect {
					log.Errorf("mitm socks5 proxy error : mitm not support command %s", req.Cmd)
					return
				}
				host, port, err := utils.ParseStringToHostPort(handledConnection.LocalAddr().String())
				if err != nil {
					log.Errorf("socks5 server parse host port failed: %v", err)
					return
				}
				_, err = handledConnection.Write(NewReply(net.ParseIP(host), port))
				if err != nil {
					log.Errorf("socks5 server reply failed: %v", err)
					return
				}
				dstPort := req.GetDstPort()
				dstHost := req.GetDstHost()
				subCtx = context.WithValue(subCtx, S5_CONNECT_ADDR, utils.HostPort(dstHost, dstPort))
				subCtx = context.WithValue(subCtx, S5_CONNECT_HOST, dstHost)
				subCtx = context.WithValue(subCtx, S5_CONNECT_PORT, strconv.Itoa(dstPort))
				handledConnection, isTls, err = IsTlsHandleShake(handledConnection)
				if err != nil {
					log.Errorf("check tls handle shake failed: %s", err)
					return
				}
			}
			p.handleLoop(isTls, handledConnection, subCtx)
		}(uid, conn)
	}
}

func IsTlsHandleShake(conn net.Conn) (fConn net.Conn, _ bool, _ error) {
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
			return peekable, false, nil
		}
		return nil, false, utils.Errorf("peek failed: %s", err)
	}
	if len(raw) != 2 {
		return nil, false, utils.Errorf("check s5 failed: %v", raw)
	}
	return peekable, raw[0] == 0x16, nil
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

func (p *Proxy) handleLoop(isTLSConn bool, conn net.Conn, rootCtx context.Context) {
	if conn == nil {
		return
	}

	p.connsMu.Lock()
	p.conns.Add(1)
	p.connsMu.Unlock()
	defer p.conns.Done()
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()
	if p.Closing() {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("handle proxy loop failed: %s", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	brw := bufio.NewReadWriter(bufio.NewReader(ctxio.NewReader(rootCtx, conn)), bufio.NewWriter(ctxio.NewWriter(rootCtx, conn)))
	s, err := newSession(conn, brw)
	if err != nil {
		log.Errorf("mitm: failed to create session: %v", err)
		return
	}

	ctx, err := withSession(s)
	if err != nil {
		log.Errorf("mitm: failed to create context: %v", err)
		return
	}

	ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_RequestProxyProtocol, "http")
	// s5 proxy needs to have higher priority than http proxy
	if s5ProxyAddr, ok := rootCtx.Value(S5_CONNECT_ADDR).(string); ok && s5ProxyAddr != "" {
		ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedTo, s5ProxyAddr)
		if s5ProxyHost, ok := rootCtx.Value(S5_CONNECT_HOST).(string); ok && s5ProxyHost != "" {
			ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost, rootCtx.Value(S5_CONNECT_HOST).(string))
		}
		if s5ProxyPort, ok := rootCtx.Value(S5_CONNECT_PORT).(string); ok && s5ProxyPort != "" {
			ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort, rootCtx.Value(S5_CONNECT_PORT).(string))
		}
		ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_RequestProxyProtocol, "socks5")
		ctx.Session().Set(AUTH_FINISH, true)
	}

	/* TLS */
	if isTLSConn {
		ctx.Session().MarkSecure()
		ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_IsHttps, true)
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
				)
				if netConn != nil {
					switch ret := netConn.(type) {
					case *tls.Conn:
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
		conn = tls.Server(conn, p.mitm.TLSForHost("127.0.0.1", p.http2 && serverUseH2))
		if tlsConn, ok := conn.(*tls.Conn); ok && tlsConn != nil {
			err := tlsConn.HandshakeContext(utils.TimeoutContextSeconds(5))
			if err != nil {
				log.Errorf("mitm recv tls conn from client, but handshake error: %v", err)
				return
			}
			if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
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

func (p *Proxy) setHTTPCtxConnectTo(req *http.Request) (string, error) {
	connectedTo, err := utils.GetConnectedToHostPortFromHTTPRequest(req)
	if err != nil {
		return "", utils.Wrap(err, "mitm: invalid host")
	}

	return connectedTo, nil
}

// handleConnectionTunnel handles a CONNECT request.
func (p *Proxy) handleConnectionTunnel(req *http.Request, timer *time.Timer, conn net.Conn, ctx *Context, session *Session, brw *bufio.ReadWriter, connectedTo string) error {
	var err error

	httpctx.SetRequestViaCONNECT(req, true)
	// set session ctx, session > httpctx
	parsedConnectedToPort := httpctx.GetContextIntInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort)
	parsedConnectedToHost := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost)
	ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost, parsedConnectedToHost)
	ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort, parsedConnectedToPort)
	ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedTo, connectedTo)
	ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ViaConnect, true)

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

	// peek the first byte to determine if this is an HTTPS connection.
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	b := make([]byte, 1)
	if _, err := io.ReadFull(brw, b); err != nil {
		log.Errorf("mitm: error peeking message through CONNECT tunnel to determine type: %v", err)
		return err
	}
	conn.SetReadDeadline(time.Time{})

	// Drain all of the rest of the buffered data.
	buf := make([]byte, brw.Reader.Buffered())
	brw.Read(buf)

	// 22 is the TLS handshake.
	isHttps := b[0] == 0x16
	ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_IsHttps, isHttps)
	if parsedConnectedToPort == 0 {
		if isHttps {
			parsedConnectedToPort = 443
		} else {
			parsedConnectedToPort = 80
		}
		ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort, parsedConnectedToPort)
	}

	// https://tools.ietf.org/html/rfc5246#section-6.2.1
	if isHttps {
		ctx.Session().MarkSecure()

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
		tlsConfig := p.mitm.TLSForHost(req.Host, p.http2 && serverUseH2)
		tlsconn := tls.Server(&peekedConn{
			Conn: conn,
			r:    io.MultiReader(bytes.NewReader(b), bytes.NewReader(buf), conn),
		}, tlsConfig)
		if err := tlsconn.Handshake(); err != nil {
			p.mitm.HandshakeErrorCallback(req, err)
			return err
		}
		nextProto := tlsconn.ConnectionState().NegotiatedProtocol
		log.Debugf("connect from browser: %v use: %v", tlsconn.RemoteAddr().String(), nextProto)
		if nextProto == "h2" {
			return p.proxyH2(p.closing, tlsconn, req.URL, ctx)
		}

		brw.Writer.Reset(tlsconn)
		brw.Reader.Reset(tlsconn)
		// -> Client Connection <- is none HTTP2 HTTPS connection
		return p.handle(ctx, timer, tlsconn, brw)
	}
	// -> Client Connection <- is plain HTTP connection
	// Prepend the previously read data to be read again.
	brw.Reader.Reset(io.MultiReader(bytes.NewReader(b), bytes.NewReader(buf), conn))
	return p.handle(ctx, timer, conn, brw)
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
	httpctx.SetMITMFrontendReadWriter(req, brw)
	httpctx.SetPluginContext(req, consts.NewPluginContext())

	session := ctx.Session()
	ctx, err := withSession(session)
	if err != nil {
		log.Errorf("mitm: failed to build new context: %v", err)
		return err
	}

	link(req, ctx, p)
	defer unlink(req, p)

	proxyProtocol := ctx.GetSessionStringValue(httpctx.REQUEST_CONTEXT_KEY_RequestProxyProtocol)
	authFinish := ctx.GetSessionBoolValue(AUTH_FINISH)
	needAuth := p.proxyUsername != "" || p.proxyPassword != ""
	httpctx.SetRequestProxyProtocol(req, proxyProtocol)

	var isHttps bool
	if tconn, ok := conn.(*tls.Conn); ok {
		session.MarkSecure()

		cs := tconn.ConnectionState()
		req.TLS = &cs
		req.URL.Scheme = "https"
		isHttps = true
		httpctx.SetRequestHTTPS(req, true)
	}

	if session.IsSecure() {
		log.Debugf("mitm: forcing HTTPS inside secure session")
		req.URL.Scheme = "https"
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

	if proxyProtocol == "http" { // if proxy is http ,need handle proxy request connect(1.1) or 1.0
		if needAuth && !authFinish {
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
				ctx.Session().Set(AUTH_FINISH, true)
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
	}

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

// A peekedConn subverts the net.Conn.Read implementation, primarily so that
// sniffed bytes can be transparently prepended.
type peekedConn struct {
	net.Conn
	r io.Reader
}

// Read allows control over the embedded net.Conn's read data. By using an
// io.MultiReader one can read from a conn, and then replace what they read, to
// be read again.
func (c *peekedConn) Read(buf []byte) (int, error) { return c.r.Read(buf) }

// connectResponse fix previous 200 CONNECT response with content-length issue
func (p *Proxy) connectResponse(req *http.Request) *http.Response {
	// "Connection Established" is the standard status for connect request. ref-link https://github.com/google/martian/issues/306
	// Content-Length  should not be set, otherwise awvs will not work ref-link https://github.com/chaitin/xray/issues/627
	resp := proxyutil.NewResponse(200, nil, req)
	resp.Header.Del("Content-Type")
	resp.Close = false
	resp.Status = fmt.Sprintf("%d %s", 200, "Connection established")
	resp.Proto = "HTTP/1.0"
	resp.ProtoMajor = 1
	resp.ProtoMinor = 0
	resp.ContentLength = -1
	return resp
}

func (p *Proxy) handshakeWithTarget(req *http.Request) (net.Conn, error) {
	var rawConn net.Conn
	var err error
	var proxyUrl string
	gmConfig := &gmtls.Config{
		InsecureSkipVerify: true,
		GMSupport:          &gmtls.GMSupport{},
		ServerName:         utils.ExtractHost(req.URL.Host),
	}

	if p.proxyURL != nil {
		proxyUrl = p.proxyURL.String()
	}
	vanillaTLS := func() {
		rawConn, err = netx.DialTLSTimeout(time.Second*10, req.URL.Host, &gmtls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2", "http/1.1"},
			ServerName:         utils.ExtractHost(req.URL.Host),
		}, proxyUrl)
	}
	gmTLS := func() {
		rawConn, err = netx.DialTLSTimeout(time.Second*10, req.URL.Host, gmConfig, proxyUrl)
	}
	var taskGroup []func()

	// when not enable gmTLS
	if !p.gmTLS {
		taskGroup = append(taskGroup, vanillaTLS)
	} else {
		// when enable gmTLS add another func
		if !p.gmTLSOnly {
			taskGroup = append(taskGroup, vanillaTLS)
		}
		taskGroup = append(taskGroup, gmTLS)
	}

	// handle gmPrefer option
	// we get at least one option in taskGroup
	if p.gmTLS && p.gmPrefer && !p.gmTLSOnly {
		taskGroup[0], taskGroup[1] = taskGroup[1], taskGroup[0] // vanilla TLS always be the first
	}

	for _, task := range taskGroup {
		task()
		if len(taskGroup) > 1 && err != nil {
			continue
		} else {
			break
		}
	}
	return rawConn, err
}
