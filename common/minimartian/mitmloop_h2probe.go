package minimartian

import (
	"crypto/tls"
	"time"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"golang.org/x/net/http2"
)

// strongHostLocalAddrFromCtx extracts the strong-host local address from the
// connection context, when strong host mode is enabled.
func strongHostLocalAddrFromCtx(ctx *Context) string {
	if ctx != nil && ctx.GetSessionBoolValue("StrongHostMode") {
		return ctx.GetSessionStringValue("StrongHostLocalAddr")
	}
	return ""
}

// detectServerH2 synchronously probes whether the origin at cacheKey
// (host:port) actually speaks HTTP/2, using the proxy's downstream proxy
// settings.
//
// ALPN negotiation alone is NOT enough: fingerprinting WAF endpoints (e.g.
// browser-vendor APIs) negotiate h2 and then silently drop or kill the
// connection without ever sending their SETTINGS frame. So after ALPN says
// h2, we send our client preface and require the server's SETTINGS frame
// (its connection preface, RFC 7540 Section 3.5) within a short window.
//
// NEVER call this on a client-handshake critical path: a slow, bot-mitigated
// or otherwise unreachable origin stalls the dial up to the full timeout,
// during which the client cannot even finish its TLS handshake with the MITM
// and no traffic is captured (browser proxy checks fail with a timeout).
func (p *Proxy) detectServerH2(cacheKey, strongHostLocalAddr, proxyStr string) bool {
	if cacheKey == "" {
		return false
	}
	basicOptions := []netx.DialXOption{
		netx.DialX_WithTimeout(10 * time.Second),
		netx.DialX_WithProxy(proxyStr),
		netx.DialX_WithForceProxy(proxyStr != ""),
		netx.DialX_WithEnableSystemProxyFromEnv(!p.disableSystemProxy),
		netx.DialX_WithAppendTLSNextProto("h2"),
		netx.DialX_WithTLS(true),
		netx.DialX_WithDialer(p.dialer),
	}
	if strongHostLocalAddr != "" {
		basicOptions = append(basicOptions, netx.DialX_WithStrongHostMode(strongHostLocalAddr))
	}

	netConn, _ := netx.DialX(cacheKey, basicOptions...)
	if netConn == nil {
		return false
	}
	defer netConn.Close()

	negotiated := false
	switch ret := netConn.(type) {
	case *tls.Conn:
		negotiated = ret.ConnectionState().NegotiatedProtocol == "h2"
	case *gmtls.Conn:
		negotiated = ret.ConnectionState().NegotiatedProtocol == "h2"
	}
	if !negotiated {
		return false
	}

	// ALPN said h2; verify the server really speaks it by requiring its
	// SETTINGS frame. Bounded so WAF tarpits cannot stall the probe.
	_ = netConn.SetDeadline(time.Now().Add(3 * time.Second))
	fr := http2.NewFramer(netConn, netConn)
	if _, err := netConn.Write([]byte(http2.ClientPreface)); err != nil {
		return false
	}
	if err := fr.WriteSettings(); err != nil {
		return false
	}
	frame, err := fr.ReadFrame()
	if err != nil {
		return false
	}
	_, isSettings := frame.(*http2.SettingsFrame)
	return isSettings
}

// detectServerH2Async runs detectServerH2 in the background (at most once per
// origin) and caches the result, keeping origin detection off the request
// path. Until the probe lands, requests to the origin are served over
// HTTP/1.1 — the same downgrade Burp performs, which always works.
func (p *Proxy) detectServerH2Async(cacheKey, strongHostLocalAddr string) {
	if !p.http2 || cacheKey == "" {
		return
	}
	if _, ok := p.h2Cache.Load(cacheKey); ok {
		return
	}
	if _, loaded := p.h2ProbeInflight.LoadOrStore(cacheKey, struct{}{}); loaded {
		return
	}
	// capture downstream proxy settings synchronously at fire time
	var proxyStr string
	if p.proxyURL != nil {
		proxyStr = p.proxyURL.String()
	}
	go func() {
		defer p.h2ProbeInflight.Delete(cacheKey)
		serverUseH2 := p.detectServerH2(cacheKey, strongHostLocalAddr, proxyStr)
		log.Infof("async h2 detection for %v: %v", cacheKey, serverUseH2)
		p.h2Cache.Store(cacheKey, serverUseH2)
	}()
}

// offerH2ToClient decides whether the MITM advertises h2 in ALPN to the
// client during the TLS handshake. It never blocks on the origin: when the
// origin's capability is unknown, h2 is offered (the proxy translates to
// HTTP/1.1 upstream when the origin does not support h2) and the origin
// capability is probed in the background. Origins already known to be
// h1-only keep HTTP/1.1.
func (p *Proxy) offerH2ToClient(cacheKey, strongHostLocalAddr string) bool {
	if !p.http2 {
		return false
	}
	if cacheKey == "" {
		return true
	}
	if cached, ok := p.h2Cache.Load(cacheKey); ok {
		return cached.(bool)
	}
	p.detectServerH2Async(cacheKey, strongHostLocalAddr)
	return true
}
