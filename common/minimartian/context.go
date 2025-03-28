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
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// Context provides information and storage for a single request/response pair.
// Contexts are linked to shared session that is used for multiple requests on
// a single connection.
type Context struct {
	session *Session
	id      string

	mu            sync.RWMutex
	vals          map[string]interface{}
	skipRoundTrip bool
	skipLogging   bool
	apiRequest    bool
}

func (c *Context) GetSessionValue(i string) any {
	if c == nil {
		return nil
	}
	if s := c.Session(); s != nil {
		i, _ := s.Get(i)
		return i
	}
	return nil
}

func (c *Context) GetSessionBoolValue(i string) bool {
	if c == nil {
		return false
	}
	if c.Session() != nil {
		i, _ := c.Session().Get(i)
		return utils.InterfaceToBoolean(i)
	}
	return false
}

func (c *Context) GetSessionStringValue(i string) string {
	if c == nil {
		return ""
	}
	if c.Session() != nil {
		i, _ := c.Session().Get(i)
		return utils.InterfaceToString(i)
	}
	return ""
}

func (c *Context) GetSessionIntValue(i string) int {
	if c == nil {
		return 0
	}
	if c.Session() != nil {
		i, _ := c.Session().Get(i)
		return utils.InterfaceToInt(i)
	}
	return 0
}

// Session provides information and storage about a connection.
type Session struct {
	mu       sync.RWMutex
	id       string
	secure   bool
	hijacked bool
	conn     net.Conn
	brw      *bufio.ReadWriter
	vals     map[string]interface{}
}

var (
	ctxmu sync.RWMutex
	//ctxs  = make(map[*http.Request]*Context)
)

// NewContext returns a context for the in-flight HTTP request.
func NewContext(req *http.Request, proxy *Proxy) *Context {
	ctxmu.RLock()
	defer ctxmu.RUnlock()

	ctx, _ := proxy.getCacheContext(req) // ctxs[req]
	return ctx
}

// TestContext builds a new session and associated context and returns the
// context and a function to remove the associated context. If it fails to
// generate either a new session or a new context it will return an error.
// Intended for tests only.
func TestContext(req *http.Request, conn net.Conn, bw *bufio.ReadWriter, p *Proxy) (ctx *Context, remove func(), err error) {
	ctxmu.Lock()
	defer ctxmu.Unlock()

	ctx, ok := p.getCacheContext(req)
	if ok {
		return ctx, func() { unlink(req, p) }, nil
	}

	s, err := newSession(conn, bw)
	if err != nil {
		return nil, nil, err
	}

	ctx, err = withSession(s)
	if err != nil {
		return nil, nil, err
	}

	p.saveCache(req, ctx)

	return ctx, func() { unlink(req, p) }, nil
}

// ID returns the session ID.
func (s *Session) ID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.id
}

// IsSecure returns whether the current session is from a secure connection,
// such as when receiving requests from a TLS connection that has been MITM'd.
func (s *Session) IsSecure() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.secure
}

// MarkSecure marks the session as secure.
func (s *Session) MarkSecure() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.secure = true
}

// MarkInsecure marks the session as insecure.
func (s *Session) MarkInsecure() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.secure = false
}

// Hijack takes control of the connection from the proxy. No further action
// will be taken by the proxy and the connection will be closed following the
// return of the hijacker.
func (s *Session) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.hijacked {
		return nil, nil, fmt.Errorf("mitm: session has already been hijacked")
	}
	s.hijacked = true

	return s.conn, s.brw, nil
}

// Hijacked returns whether the connection has been hijacked.
func (s *Session) Hijacked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.hijacked
}

// setConn resets the underlying connection and bufio.ReadWriter of the
// session. Used by the proxy when the connection is upgraded to TLS.
func (s *Session) setConn(conn net.Conn, brw *bufio.ReadWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.conn = conn
	s.brw = brw
}

// Get takes key and returns the associated value from the session.
func (s *Session) Get(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.vals[key]

	return val, ok
}

// Set takes a key and associates it with val in the session. The value is
// persisted for the entire session across multiple requests and responses.
func (s *Session) Set(key string, val interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.vals[key] = val
}

// Session returns the session for the context.
func (ctx *Context) Session() *Session {
	return ctx.session
}

// ID returns the context ID.
func (ctx *Context) ID() string {
	return ctx.id
}

// Get takes key and returns the associated value from the context.
func (ctx *Context) Get(key string) (interface{}, bool) {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	val, ok := ctx.vals[key]

	return val, ok
}

// Set takes a key and associates it with val in the context. The value is
// persisted for the duration of the request and is removed on the following
// request.
func (ctx *Context) Set(key string, val interface{}) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.vals[key] = val
}

// SkipRoundTrip skips the round trip for the current request.
func (ctx *Context) SkipRoundTrip() {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.skipRoundTrip = true
}

// SkippingRoundTrip returns whether the current round trip will be skipped.
func (ctx *Context) SkippingRoundTrip() bool {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	return ctx.skipRoundTrip
}

// SkipLogging skips logging by Martian loggers for the current request.
func (ctx *Context) SkipLogging() {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.skipLogging = true
}

// SkippingLogging returns whether the current request / response pair will be logged.
func (ctx *Context) SkippingLogging() bool {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	return ctx.skipLogging
}

// APIRequest marks the requests as a request to the proxy API.
func (ctx *Context) APIRequest() {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.apiRequest = true
}

// IsAPIRequest returns true when the request patterns matches a pattern in the proxy
// mux. The mux is usually defined as a parameter to the api.Forwarder, which uses
// http.DefaultServeMux by default.
func (ctx *Context) IsAPIRequest() bool {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	return ctx.apiRequest
}

// newID creates a new 16 character random hex ID; note these are not UUIDs.
func newID() (string, error) {
	src := make([]byte, 8)
	if _, err := rand.Read(src); err != nil {
		return "", err
	}

	return hex.EncodeToString(src), nil
}

// link associates the context with request.
func link(req *http.Request, ctx *Context, p *Proxy) {
	ctxmu.Lock()
	defer ctxmu.Unlock()

	p.saveCache(req, ctx)
}

// unlink removes the context for request.
func unlink(req *http.Request, proxy *Proxy) {
	ctxmu.Lock()
	defer ctxmu.Unlock()

	proxy.deleteCache(req)
}

// newSession builds a new session.
func newSession(conn net.Conn, brw *bufio.ReadWriter) (*Session, error) {
	sid, err := newID()
	if err != nil {
		return nil, err
	}

	return &Session{
		id:   sid,
		conn: conn,
		brw:  brw,
		vals: make(map[string]interface{}),
	}, nil
}

// withSession builds a new context from an existing session. Session must be
// non-nil.
func withSession(s *Session) (*Context, error) {
	cid, err := newID()
	if err != nil {
		return nil, err
	}

	return &Context{
		session: s,
		id:      cid,
		vals:    make(map[string]interface{}),
	}, nil
}
