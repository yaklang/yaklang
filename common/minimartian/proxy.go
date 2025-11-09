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

package minimartian

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/mitm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

var (
	errClose = errors.New("closing connection")
	noop     = Noop("martian")
)

func isCloseable(err error) bool {
	if err == nil {
		return false
	}

	if utils.IsErrorNetOpTimeout(err) {
		return true
	}

	switch {
	case err == io.EOF, errors.Is(err, io.ErrClosedPipe), errors.Is(err, errClose), errors.Is(err, io.ErrUnexpectedEOF):
		return true
	default:
		log.Debugf("Unhandled CONNECTION ERROR: %v", err.Error())
		return true
	}
}

// Proxy is an HTTP proxy with support for TLS MITM and customizable behavior.
type Proxy struct {
	dialer          func(time.Duration, string) (net.Conn, error)
	timeout         time.Duration
	mitm            *mitm.Config
	proxyURL        *url.URL
	conns           sync.WaitGroup
	connsMu         sync.Mutex // protects conns.Add/Wait from concurrent access
	closing         chan bool
	http2           bool
	gmTLS           bool
	gmPrefer        bool
	gmTLSOnly       bool
	findProcessName bool
	reqmod          RequestModifier
	resmod          ResponseModifier

	// context cache
	ctxCacheLock     *sync.Mutex
	ctxCacheInitOnce *sync.Once
	ctxCache         *utils.Cache[*Context]

	// 限制用户名和密码
	proxyUsername string
	proxyPassword string

	// lowhttp config
	lowhttpConfig []lowhttp.LowhttpOpt

	maxContentLength int
	maxReadWaitTime  time.Duration

	h2Cache sync.Map

	forceDisableKeepAlive bool

	tunMode bool

	// connection pool for remote server connections
	connPool *lowhttp.LowHttpConnPool

	extraIncomingConnCh chan *WrapperedConn
}

func (p *Proxy) MergeExtraIncomingConectionChannel(ctx context.Context, ch chan *WrapperedConn) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case conn, ok := <-ch:
				if !ok {
					return
				}
				select {
				case p.extraIncomingConnCh <- conn:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
}

// MergeExtraIncomingConectionChannelLegacy 兼容旧版本的 net.Conn channel
// 自动将 net.Conn 包装为 wrapperedConn
func (p *Proxy) MergeExtraIncomingConectionChannelLegacy(ctx context.Context, ch chan net.Conn) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case conn, ok := <-ch:
				if !ok {
					return
				}
				wrapped := NewWrapperedConn(conn, false, nil)
				select {
				case p.extraIncomingConnCh <- wrapped:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
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
			p.ctxCache = utils.NewTTLCache[*Context](5 * time.Minute)
		})
	}
	p.ctxCache.Set(key, ctx)
}

func (p *Proxy) getCacheContext(r *http.Request) (*Context, bool) {
	if p == nil || p.ctxCache == nil {
		return nil, false
	}
	key := fmt.Sprintf("%p", r)
	ins, ok := p.ctxCache.Get(key)
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
		timeout:             5 * time.Minute,
		closing:             make(chan bool),
		reqmod:              noop,
		resmod:              noop,
		ctxCacheInitOnce:    new(sync.Once),
		ctxCacheLock:        new(sync.Mutex),
		ctxCache:            utils.NewTTLCache[*Context](5 * time.Minute),
		extraIncomingConnCh: make(chan *WrapperedConn),
	}
	return proxy
}

// SetDialer sets the proxy dialer
func (p *Proxy) SetDialer(dialer func(time.Duration, string) (net.Conn, error)) {
	p.dialer = dialer
}

// SetTimeout sets the request timeout of the proxy.
func (p *Proxy) SetTimeout(timeout time.Duration) {
	p.timeout = timeout
}

// SetMITM sets the config to use for MITMing of CONNECT requests.
func (p *Proxy) SetMITM(config *mitm.Config) {
	p.mitm = config
}

func (p *Proxy) SetMaxContentLength(i int) {
	p.maxContentLength = i
}

func (p *Proxy) SetMaxReadWaitTime(t time.Duration) {
	p.maxReadWaitTime = t
}

func (p *Proxy) GetMaxContentLength() int {
	if p == nil || p.maxContentLength <= 0 {
		return 0
	}
	return p.maxContentLength
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

// SetHTTPForceClose sets proxy no-keepalive
func (p *Proxy) SetHTTPForceClose(enable bool) {
	p.forceDisableKeepAlive = enable
}

// SetDial sets the dial func used to establish a connection.
//func (p *Proxy) SetDialContext(dial func(context.Context, string, string) (net.Conn, error)) {
//	p.dial = func(ctx context.Context, a, b string) (net.Conn, error) {
//		c, e := dial(ctx, a, b)
//		nosigpipe.IgnoreSIGPIPE(c)
//		return c, e
//	}
//
//	if tr, ok := p.roundTripper.(*http.Transport); ok {
//		tr.DialContext = p.dial
//	}
//}

// SetLowhttpConfig sets the lowhttp config
func (p *Proxy) SetLowhttpConfig(config []lowhttp.LowhttpOpt) {
	p.lowhttpConfig = config
}

func (p *Proxy) SetFindProcessName(b bool) {
	p.findProcessName = b
}

func (p *Proxy) SetTunMode(b bool) {
	p.tunMode = b
}

// SetConnPool sets the connection pool for remote server connections
func (p *Proxy) SetConnPool(pool *lowhttp.LowHttpConnPool) {
	p.connPool = pool
}

// Close sets the proxy to the closing state so it stops receiving new connections,
// finishes processing any inflight requests, and closes existing connections without
// reading anymore requests from them.
func (p *Proxy) Close() {
	log.Infof("mitm: closing down proxy")

	close(p.closing)

	log.Infof("mitm: waiting for connections to close")
	p.connsMu.Lock()
	p.conns.Wait()
	p.connsMu.Unlock()
	log.Infof("mitm: all connections closed")

	// Clean up connection pool if it exists
	if p.connPool != nil {
		log.Infof("mitm: clearing connection pool")
		p.connPool.Clear()
		log.Infof("mitm: connection pool cleared")
	}
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
