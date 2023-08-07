// Copyright 2015 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package martian

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/utils/lowhttp/lowhttp2"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/martian/v3/mitm"
	"github.com/yaklang/yaklang/common/martian/v3/nosigpipe"
	"github.com/yaklang/yaklang/common/martian/v3/proxyutil"
)

var errClose = errors.New("closing connection")
var noop = Noop("martian")

func isCloseable(err error) bool {
	if err == nil {
		return false
	}

	if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
		return true
	}

	switch err {
	case io.EOF, io.ErrClosedPipe, errClose, io.ErrUnexpectedEOF:
		return true
	default:
		log.Debugf("Unhandled CONNECTION ERROR: %v", err.Error())
		return true
	}
}

// Proxy is an HTTP proxy with support for TLS MITM and customizable behavior.
type Proxy struct {
	roundTripper http.RoundTripper
	dial         func(context.Context, string, string) (net.Conn, error)
	timeout      time.Duration
	mitm         *mitm.Config
	proxyURL     *url.URL
	conns        sync.WaitGroup
	connsMu      sync.Mutex // protects conns.Add/Wait from concurrent access
	closing      chan bool
	http2        bool
	gmTLS        bool
	gmPrefer     bool
	gmTLSOnly    bool
	reqmod       RequestModifier
	resmod       ResponseModifier

	// context cache
	ctxCacheLock     *sync.Mutex
	ctxCacheInitOnce *sync.Once
	ctxCache         *ttlcache.Cache

	// 限制用户名和密码
	proxyUsername string
	proxyPassword string
}

func (p *Proxy) saveCache(r *http.Request, ctx *Context) {
	if p == nil {
		return
	}
	p.ctxCacheLock.Lock()
	defer p.ctxCacheLock.Unlock()
	key := fmt.Sprintf("%p", r)
	if p.ctxCache == nil {
		p.ctxCacheInitOnce.Do(func() {
			p.ctxCache = ttlcache.NewCache()
			p.ctxCache.SetTTL(5 * time.Minute)
		})
	}
	p.ctxCache.Set(key, ctx)
}

func (p *Proxy) getCacheContext(r *http.Request) (*Context, bool) {
	if p == nil || p.ctxCache == nil {
		return nil, false
	}
	key := fmt.Sprintf("%p", r)
	raw, ok := p.ctxCache.Get(key)
	if !ok {
		return nil, false
	}
	ins, ok := raw.(*Context)
	if !ok {
		return nil, false
	}
	return ins, true
}

func (p *Proxy) deleteCache(req *http.Request) {
	if p == nil || p.ctxCache == nil {
		return
	}
	key := fmt.Sprintf("%p", req)
	p.ctxCache.Remove(key)
}

// NewProxy returns a new HTTP proxy.
func NewProxy() *Proxy {
	proxy := &Proxy{
		roundTripper: &http.Transport{
			// TODO(adamtanner): This forces the http.Transport to not upgrade requests
			// to HTTP/2 in Go 1.6+. Remove this once Martian can support HTTP/2.
			TLSNextProto:          make(map[string]func(string, *tls.Conn) http.RoundTripper),
			Proxy:                 http.ProxyFromEnvironment,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
		timeout:          5 * time.Minute,
		closing:          make(chan bool),
		reqmod:           noop,
		resmod:           noop,
		ctxCacheInitOnce: new(sync.Once),
		ctxCacheLock:     new(sync.Mutex),
		ctxCache:         ttlcache.NewCache(),
	}
	proxy.ctxCache.SetTTL(5 * time.Minute)
	proxy.SetDialContext(netx.NewDialContextFunc(30 * time.Second))
	return proxy
}

// SetRoundTripper sets the http.RoundTripper of the proxy.
func (p *Proxy) SetRoundTripper(rt http.RoundTripper) {
	p.roundTripper = rt

	if tr, ok := p.roundTripper.(*http.Transport); ok {
		tr.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
		tr.Proxy = http.ProxyURL(p.proxyURL)
		tr.DialContext = p.dial
	}
}

// SetDownstreamProxy sets the proxy that receives requests from the upstream
// proxy.
func (p *Proxy) SetDownstreamProxy(proxyURL *url.URL) {
	p.proxyURL = proxyURL

	if tr, ok := p.roundTripper.(*http.Transport); ok {
		tr.Proxy = http.ProxyURL(p.proxyURL)
	}
}

// SetTimeout sets the request timeout of the proxy.
func (p *Proxy) SetTimeout(timeout time.Duration) {
	p.timeout = timeout
}

// SetMITM sets the config to use for MITMing of CONNECT requests.
func (p *Proxy) SetMITM(config *mitm.Config) {
	p.mitm = config
}

// SetH2 sets the switch to turn on HTTP2 support
func (p *Proxy) SetH2(enable bool) {
	p.http2 = enable
}

// SetAuth sets the username and password for proxy authentication.
func (p *Proxy) SetAuth(user, pass string) {
	p.proxyUsername = user
	p.proxyPassword = pass
}

// SetGMTLS sets the switch to turn on GM support
func (p *Proxy) SetGMTLS(enable bool) {
	p.gmTLS = enable
}

// SetGMPrefer sets the switch to prefer using GM style TLS
func (p *Proxy) SetGMPrefer(enable bool) {
	p.gmPrefer = enable
}

// SetGMOnly sets the switch to use ONLY GM TLS
func (p *Proxy) SetGMOnly(enable bool) {
	p.gmTLSOnly = enable
}

// SetDial sets the dial func used to establish a connection.
func (p *Proxy) SetDialContext(dial func(context.Context, string, string) (net.Conn, error)) {
	p.dial = func(ctx context.Context, a, b string) (net.Conn, error) {
		c, e := dial(ctx, a, b)
		nosigpipe.IgnoreSIGPIPE(c)
		return c, e
	}

	if tr, ok := p.roundTripper.(*http.Transport); ok {
		tr.DialContext = p.dial
	}
}

// Close sets the proxy to the closing state so it stops receiving new connections,
// finishes processing any inflight requests, and closes existing connections without
// reading anymore requests from them.
func (p *Proxy) Close() {
	log.Infof("martian: closing down proxy")

	close(p.closing)

	log.Infof("martian: waiting for connections to close")
	p.connsMu.Lock()
	p.conns.Wait()
	p.connsMu.Unlock()
	log.Infof("martian: all connections closed")
}

// Closing returns whether the proxy is in the closing state.
func (p *Proxy) Closing() bool {
	select {
	case <-p.closing:
		return true
	default:
		return false
	}
}

// SetRequestModifier sets the request modifier.
func (p *Proxy) SetRequestModifier(reqmod RequestModifier) {
	if reqmod == nil {
		reqmod = noop
	}

	p.reqmod = reqmod
}

// SetResponseModifier sets the response modifier.
func (p *Proxy) SetResponseModifier(resmod ResponseModifier) {
	if resmod == nil {
		resmod = noop
	}

	p.resmod = resmod
}

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
	var connsCached = new(sync.Map)
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
					log.Infof("active connections count: %v", currentConnCount)
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

				log.Debugf("martian: temporary error on accept: %v", err)
				time.Sleep(delay)
				continue
			}
			log.Errorf("martian: failed to accept: %v", err)
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

		log.Debugf("martian: accepted connection from %s", conn.RemoteAddr())
		if tconn, ok := conn.(*net.TCPConn); ok {
			_ = tconn
			//tconn.SetKeepAlive(true)
			//tconn.SetKeepAlivePeriod(3 * time.Minute)
		}

		go func(uidStr string, originConn net.Conn) {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("handle mitm proxy loop failed: %s", err)
					if originConn != nil {
						originConn.Close()
					}
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			var ok bool
			var handledConnection net.Conn
			//utils.Debug(func() {
			//	// time.Sleep(50 * time.Millisecond)
			//})
			// TODO: s5config.IsSocks5HandleShake May be blocked... why? need more tests
			handledConnection, ok, err = s5config.IsSocks5HandleShake(originConn)
			if err != nil {
				removeConns(uidStr, originConn)
				log.Errorf("check socks5 handle shake failed: %s", err)
				return
			}
			defer func() {
				removeConns(uidStr, handledConnection)
			}()
			if ok {
				err := s5config.ServeConn(handledConnection)
				if err != nil {
					log.Errorf("socks5 handle failed: %s", err)
					return
				}
				return
			} else {
				p.handleLoop(handledConnection, ctx)
			}
		}(uid, conn)
	}
}

func (p *Proxy) handleLoop(conn net.Conn, rootCtx context.Context) {
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
		log.Errorf("martian: failed to create session: %v", err)
		return
	}

	ctx, err := withSession(s)
	if err != nil {
		log.Errorf("martian: failed to create context: %v", err)
		return
	}

	for {
		deadline := time.Now().Add(p.timeout)
		conn.SetDeadline(deadline)

		log.Debugf("waiting conn: %v", conn.RemoteAddr())
		err := p.handle(ctx, conn, brw)
		if err != nil && isCloseable(err) {
			log.Debugf("closing conn: %v", conn.RemoteAddr())
			return
		}
	}
}

func (p *Proxy) handle(ctx *Context, conn net.Conn, brw *bufio.ReadWriter) error {
	log.Debugf("martian: waiting for request: %v", conn.RemoteAddr())
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
		r, err := lowhttp.ReadHTTPRequest(brw.Reader)
		if err != nil {
			errc <- err
			return
		}
		reqc <- r
	}()
	select {
	case err := <-errc:
		if isCloseable(err) {
			log.Debugf("martian: connection closed prematurely: %v", err)
			conn.Close()
		} else {
			log.Errorf("martian: failed to read request: %v", err)
		}
		// TODO: TCPConn.WriteClose() to avoid sending an RST to the client.
		return errClose
	case req = <-reqc:
	case <-p.closing:
		return errClose
	}
	defer req.Body.Close()

	session := ctx.Session()
	ctx, err := withSession(session)
	if err != nil {
		log.Errorf("martian: failed to build new context: %v", err)
		return err
	}

	link(req, ctx, p)
	defer unlink(req, p)

	if tconn, ok := conn.(*tls.Conn); ok {
		session.MarkSecure()

		cs := tconn.ConnectionState()
		req.TLS = &cs
	}

	req.URL.Scheme = "http"
	if session.IsSecure() {
		log.Debugf("martian: forcing HTTPS inside secure session")
		req.URL.Scheme = "https"
	}

	req.RemoteAddr = conn.RemoteAddr().String()
	if req.URL.Host == "" {
		req.URL.Host = req.Host
	}

	isProxy := req.Method == "CONNECT" || req.Header.Get("Proxy-Connection") != "" || req.Header.Get("Proxy-Authorization") != ""
	if isProxy {
		if p.proxyUsername != "" || p.proxyPassword != "" {
			// 开启认证
			failed := func(reason string) error {
				res := proxyutil.NewResponse(407, http.NoBody, req)
				res.Status = "407 Authentication Required"
				res.Header.Set("Proxy-Authenticate", "Basic realm=\"yakit proxy\", charset=\"UTF-8\"")
				e := fmt.Errorf("reason: %v", reason)
				proxyutil.Warning(res.Header, e)
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
			} else {
				return failed("empty Proxy-Authorization Header")
			}
		}
	}

	if req.Method == "CONNECT" {
		// req auth enable
		var connectedTo = req.Host
		if host, port, err := utils.ParseStringToHostPort(req.URL.String()); err == nil {
			connectedTo = utils.HostPort(host, port)
		}
		if req.URL.Scheme == "https" {
			connectedTo = strings.TrimSuffix(connectedTo, ":443")
		}

		var parsedConnectedToHost, parsedConnectedToPort, _ = utils.ParseStringToHostPort(connectedTo)
		if parsedConnectedToPort <= 0 {
			parsedConnectedToHost = connectedTo
		}
		ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost, parsedConnectedToHost)
		ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedTo, connectedTo)

		if err := p.reqmod.ModifyRequest(req); err != nil {
			if !strings.Contains(err.Error(), "ignore connect") {
				log.Errorf("martian: error modifying CONNECT request: %v", err)
				proxyutil.Warning(req.Header, err)
			}
		}
		if session.Hijacked() {
			log.Debugf("martian: connection hijacked by request modifier")
			return nil
		}

		if p.mitm != nil {
			log.Debugf("martian: attempting MITM for connection: %s", req.Host)
			res := p.connectResponse(req)

			if err := p.resmod.ModifyResponse(res); err != nil {
				log.Errorf("martian: error modifying CONNECT response: %v", err)
				proxyutil.Warning(res.Header, err)
			}
			if session.Hijacked() {
				log.Debugf("martian: connection hijacked by response modifier")
				return nil
			}

			if err := res.Write(brw); err != nil {
				log.Errorf("martian: got error while writing response back to client: %v", err)
			}
			if err := brw.Flush(); err != nil {
				log.Errorf("martian: got error while flushing response back to client: %v", err)
			}

			log.Debugf("martian: completed MITM for connection: %s", req.Host)

			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			b := make([]byte, 1)
			if _, err := brw.Read(b); err != nil {
				log.Errorf("martian: error peeking message through CONNECT tunnel to determine type: %v", err)
				return err
			}
			conn.SetReadDeadline(time.Time{})

			// Drain all of the rest of the buffered data.
			buf := make([]byte, brw.Reader.Buffered())
			brw.Read(buf)

			isHttps := b[0] == 0x16
			ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_IsHttps, isHttps)
			if parsedConnectedToPort == 0 {
				if isHttps {
					parsedConnectedToPort = 443
				} else {
					parsedConnectedToPort = 80
				}
			}
			ctx.Session().Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort, parsedConnectedToPort)

			// 22 is the TLS handshake.
			// https://tools.ietf.org/html/rfc5246#section-6.2.1
			if isHttps {
				var serverUseH2 bool
				if p.http2 {
					// does remote server use h2?
					defaultTLSConfig := utils.NewDefaultTLSConfig()
					defaultTLSConfig.NextProtos = []string{"h2"}
					var proxyStr string
					if p.proxyURL != nil {
						proxyStr = p.proxyURL.String()
					}
					netConn, _ := netx.DialTLSTimeout(5*time.Second, utils.HostPort(parsedConnectedToHost, parsedConnectedToPort), defaultTLSConfig, proxyStr)
					if netConn != nil {
						switch ret := netConn.(type) {
						case *tls.Conn:
							if ret.ConnectionState().NegotiatedProtocol == "h2" {
								serverUseH2 = true
							}
						}
						netConn.Close()
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
				var nconn net.Conn = tlsconn

				if nextProto == "h2" {
					return p.proxyH2(p.closing, tlsconn, req.URL)
				}

				brw.Writer.Reset(nconn)
				brw.Reader.Reset(nconn)
				// -> Client Connection <- is none HTTP2 HTTPS connection
				return p.handle(ctx, nconn, brw)

				///* update lib from official martian to add support for intercepting and analysing *h2* request */
				///* also fix bug from *martian* since they did not respect server side ALPN */
				//var rawConn net.Conn
				//var err error
				//
				////check if target server support h2 if not we will degrade to use http/1.1
				//rawConn, err = p.handshakeWithTarget(req)
				//if err != nil {
				//	return fmt.Errorf("fail to connect to %v: %w", req.URL, err)
				//}
				//_, ok := rawConn.(*tls.Conn)
				//
				//if !ok {
				//	// target server is GM TLS
				//	// omit HTTP/2 with GM for now
				//	tlsconn := tls.Server(&peekedConn{conn, io.MultiReader(bytes.NewReader(b), bytes.NewReader(buf), conn)}, p.mitm.TLSForHost(req.Host, false))
				//	if err := tlsconn.Handshake(); err != nil {
				//		p.mitm.HandshakeErrorCallback(req, err)
				//		return err
				//	}
				//	var nconn net.Conn
				//	nconn = tlsconn
				//	brw.Writer.Reset(nconn)
				//	brw.Reader.Reset(nconn)
				//	// -> Client Connection <- is none HTTP2 HTTPS connection
				//	return p.handle(ctx, nconn, brw)
				//}
				//
				//sc := rawConn.(*tls.Conn)
				//
				//if sc.ConnectionState().NegotiatedProtocol == "h2" && p.http2 { //server support h2
				//	cc := tls.Server(&peekedConn{conn, io.MultiReader(bytes.NewReader(b), bytes.NewReader(buf), conn)}, p.mitm.TLSForHost(req.Host, true))
				//	if err := cc.Handshake(); err != nil {
				//		p.mitm.HandshakeErrorCallback(req, err)
				//		return err
				//	}
				//
				//	if cc.ConnectionState().NegotiatedProtocol == "h2" { //browser also want h2 then proxy with h2
				//		// -> Client Connection <- is HTTP2 HTTPS connection (P.S no support for h2c all http2 is https)
				//		return p.proxyH2(p.closing, cc, req.URL)
				//	}
				//} else { //server not support h2 so we completely disable h2 support to handle using previous version of martian
				//	tlsconn := tls.Server(&peekedConn{conn, io.MultiReader(bytes.NewReader(b), bytes.NewReader(buf), conn)}, p.mitm.TLSForHost(req.Host, false))
				//	if err := tlsconn.Handshake(); err != nil {
				//		p.mitm.HandshakeErrorCallback(req, err)
				//		return err
				//	}
				//	var nconn net.Conn
				//	nconn = tlsconn
				//	brw.Writer.Reset(nconn)
				//	brw.Reader.Reset(nconn)
				//	// -> Client Connection <- is none HTTP2 HTTPS connection
				//	return p.handle(ctx, nconn, brw)
				//}
			}
			// -> Client Connection <- is plain HTTP connection
			// Prepend the previously read data to be read again by http.ReadRequest.
			brw.Reader.Reset(io.MultiReader(bytes.NewReader(b), bytes.NewReader(buf), conn))
			return p.handle(ctx, conn, brw)
		}

		log.Infof("martian: attempting to establish CONNECT tunnel: %s", req.URL.Host)
		res, cconn, cerr := p.connect(req)
		if cerr != nil {
			log.Errorf("martian: failed to CONNECT: %v", err)
			res = proxyutil.NewResponse(502, nil, req)
			proxyutil.Warning(res.Header, cerr)

			if err := p.resmod.ModifyResponse(res); err != nil {
				log.Errorf("martian: error modifying CONNECT response: %v", err)
				proxyutil.Warning(res.Header, err)
			}
			if session.Hijacked() {
				log.Debugf("martian: connection hijacked by response modifier")
				return nil
			}

			if err := res.Write(brw); err != nil {
				log.Errorf("martian: got error while writing response back to client: %v", err)
			}
			err := brw.Flush()
			if err != nil {
				log.Errorf("martian: got error while flushing response back to client: %v", err)
			}
			return err
		}
		defer res.Body.Close()
		defer cconn.Close()

		if err := p.resmod.ModifyResponse(res); err != nil {
			log.Errorf("martian: error modifying CONNECT response: %v", err)
			proxyutil.Warning(res.Header, err)
		}
		if session.Hijacked() {
			log.Debugf("martian: connection hijacked by response modifier")
			return nil
		}
		res.ContentLength = -1
		if err := res.Write(brw); err != nil {
			log.Errorf("martian: got error while writing response back to client: %v", err)
		}
		if err := brw.Flush(); err != nil {
			log.Errorf("martian: got error while flushing response back to client: %v", err)
		}

		cbw := bufio.NewWriter(cconn)
		cbr := bufio.NewReader(cconn)
		defer cbw.Flush()

		copySync := func(w io.Writer, r io.Reader, donec chan<- bool) {
			if _, err := io.Copy(w, r); err != nil && err != io.EOF {
				log.Errorf("martian: failed to copy CONNECT tunnel: %v", err)
			}

			log.Debugf("martian: CONNECT tunnel finished copying")
			donec <- true
		}

		donec := make(chan bool, 2)
		go copySync(cbw, brw, donec)
		go copySync(brw, cbr, donec)

		log.Debugf("martian: established CONNECT tunnel, proxying traffic")
		<-donec
		<-donec
		log.Debugf("martian: closed CONNECT tunnel")

		return errClose
	}

	if err := p.reqmod.ModifyRequest(req); err != nil {
		log.Errorf("martian: error modifying request: %v", err)
		proxyutil.Warning(req.Header, err)
	}
	if session.Hijacked() {
		log.Debugf("martian: connection hijacked by request modifier")
		return nil
	}

	res, err := p.roundTrip(ctx, req)
	if (err != nil && err != io.EOF) || res == nil {
		if strings.Contains(err.Error(), "no such host") {
			httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_NOLOG, true)
			res = proxyutil.NewResponse(200, strings.NewReader(proxyutil.GetErrorRspBody(fmt.Sprintf("Unknown host: %s", req.Host))), req)
		} else {
			log.Debugf("martian: failed to round trip: %v", err)
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
		if err := p.resmod.ModifyResponse(res); err != nil {
			log.Errorf("martian: error modifying response: %v", err)
			proxyutil.Warning(res.Header, err)
		}
	}

	if session.Hijacked() {
		log.Debugf("martian: connection hijacked by response modifier")
		return nil
	}

	var closing error
	if req.Close || res.Close || p.Closing() {
		log.Debugf("martian: received close request: %v", req.RemoteAddr)
		res.Close = true
		closing = errClose
	}

	err = res.Write(brw)
	if err != nil {
		log.Errorf("martian: got error while writing response back to client: %v", err)
	}
	//Handle proxy getting stuck when upstream stops responding midway
	//see https://github.com/google/martian/pull/349
	if err == io.ErrUnexpectedEOF {
		closing = errClose
	}

	err = brw.Flush()
	if err != nil {
		log.Errorf("martian: got error while flushing response back to client: %v", err)
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

func (p *Proxy) roundTrip(ctx *Context, req *http.Request) (*http.Response, error) {
	if ctx.SkippingRoundTrip() {
		log.Debugf("martian: skipping round trip")
		return proxyutil.NewResponse(200, nil, req), nil
	}
	if httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_IsDropped) {
		log.Debugf("martian: skipping round trip due to user manually drop")
		return proxyutil.NewResponse(200, strings.NewReader(proxyutil.GetErrorRspBody("请求被用户丢弃")), req), nil
	}

	https := ctx.GetSessionBoolValue(httpctx.REQUEST_CONTEXT_KEY_IsHttps)
	httpctx.SetContextValueInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_IsHttps, https)

	inherit := func(i string) {
		httpctx.SetContextValueInfoFromRequest(req, i, ctx.GetSessionStringValue(i))
	}
	inherit(httpctx.REQUEST_CONTEXT_KEY_ConnectedTo)
	inherit(httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort)
	inherit(httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost)
	return p.roundTripper.RoundTrip(req)
}

func (p *Proxy) connect(req *http.Request) (*http.Response, net.Conn, error) {
	if p.proxyURL != nil {
		log.Debugf("martian: CONNECT with downstream proxy: %s", p.proxyURL.Host)

		conn, err := p.dial(context.Background(), "tcp", p.proxyURL.Host)
		if err != nil {
			return nil, nil, err
		}
		pbw := bufio.NewWriter(conn)
		pbr := bufio.NewReader(conn)

		req.Write(pbw)
		pbw.Flush()

		res, err := http.ReadResponse(pbr, req)
		if err != nil {
			return nil, nil, err
		}

		return res, conn, nil
	}

	log.Debugf("martian: CONNECT to host directly: %s", req.URL.Host)

	conn, err := p.dial(req.Context(), "tcp", req.URL.Host)
	if err != nil {
		return nil, nil, err
	}

	return p.connectResponse(req), conn, nil
}

// connectResponse fix previous 200 CONNECT response with content-length issue
func (p *Proxy) connectResponse(req *http.Request) *http.Response {
	// "Connection Established" is the standard status for connect request. ref-link https://github.com/google/martian/issues/306
	// Content-Length  should not be set, otherwise awvs will not work ref-link https://github.com/chaitin/xray/issues/627
	resp := proxyutil.NewResponse(200, nil, req)
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
	var gmConfig = &gmtls.Config{
		InsecureSkipVerify: true,
		GMSupport:          &gmtls.GMSupport{},
		ServerName:         utils.ExtractHost(req.URL.Host),
	}

	if p.proxyURL != nil {
		proxyUrl = p.proxyURL.String()
	}
	vanillaTLS := func() {
		rawConn, err = netx.DialTLSTimeout(time.Second*10, req.URL.Host, &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
			MaxVersion:         tls.VersionTLS13,
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

// proxyH2 proxies HTTP/2 traffic between a client connection, `cc`, and the HTTP/2 `url` assuming
// h2 is being used. Since no browsers use h2c, it's safe to assume all traffic uses TLS.
// Revision this func from martian h2 package since it was not compatible with martian modifier style
func (p *Proxy) proxyH2(closing chan bool, cc *tls.Conn, url *url.URL) error {
	if p.mitm.H2Config().EnableDebugLogs {
		log.Infof("\u001b[1;35mProxying %v with HTTP/2\u001b[0m", url)
	}

	go func() {
		select {
		case <-closing:
		}
		cc.Close()
	}()

	var tr http.RoundTripper

	tr = p.roundTripper
	proxyClient := lowhttp2.Server{
		PermitProhibitedCipherSuites: true,
	}
	proxyToServer := http.Client{
		Transport: tr,
	}
	handler := makeNewH2Handler(p.reqmod, p.resmod, &proxyToServer, url.Host)
	proxyClientConfig := &lowhttp2.ServeConnOpts{Handler: handler}
	proxyClient.ServeConn(cc, proxyClientConfig)
	return nil

}

type H2Handler struct {
	reqmod        RequestModifier
	resmod        ResponseModifier
	proxyToServer *http.Client
	serverHost    string
	//flowMux       sync.Mutex
}

func makeNewH2Handler(reqmod RequestModifier, resmod ResponseModifier, proxyToServer *http.Client, serverHost string) *H2Handler {
	return &H2Handler{reqmod: reqmod, resmod: resmod, proxyToServer: proxyToServer, serverHost: serverHost}
}

func (h *H2Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	//if strings.Contains(req.URL.String(), "/xxx") {
	//	log.Infof("Hit")
	//	reqRaw, _ := utils.HttpDumpWithBody(req, true)
	//	println(string(reqRaw))
	//}
	if err := h.reqmod.ModifyRequest(req); err != nil {
		log.Errorf("martian: error modifying request: %v", err)
		proxyutil.Warning(req.Header, err)
		return
	}
	if httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_IsDropped) {
		w.WriteHeader(200)
		w.Write([]byte(proxyutil.GetErrorRspBody("请求被用户丢弃")))
	} else {
		rsp, err := h.proxyToServer.Transport.RoundTrip(req)
		if err != nil {
			log.Errorf("martian: error requesting to remote server: %v", err)
			return
		}
		defer rsp.Body.Close()

		if err := h.resmod.ModifyResponse(rsp); err != nil {
			log.Errorf("martian: error modifying response: %v", err)
			proxyutil.Warning(req.Header, err)
			return
		}

		for k, v := range rsp.Header {
			w.Header().Set(k, v[0])
		}
		w.WriteHeader(rsp.StatusCode)
		rspBody, _ := io.ReadAll(rsp.Body)

		w.Write(rspBody)
	}

}
