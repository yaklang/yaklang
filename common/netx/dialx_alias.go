package netx

import (
	"context"
	"github.com/yaklang/yaklang/common/netx/dns_lookup"
	"net"
	"time"
)

// NewDialContextFunc is a function that can be used to dial a connection.
func NewDialContextFunc(timeout time.Duration, opts ...dns_lookup.DNSOption) func(ctx context.Context, network string, addr string) (net.Conn, error) {
	return func(ctx context.Context, network string, addr string) (net.Conn, error) {
		ddl, ok := ctx.Deadline()
		if ok {
			if du := ddl.Sub(time.Now()); du.Seconds() > 0 && du < timeout {
				timeout = du
			}
		}

		return DialX(
			addr,
			DialX_WithDNSOptions(opts...), DialX_WithTimeout(timeout),
			//DialX_WithContext(ctx),
		)
	}
}

// DialTimeoutWithoutProxy dials a connection with a timeout.
func DialTimeoutWithoutProxy(timeout time.Duration, network, addr string) (net.Conn, error) {
	return DialX(addr, DialX_WithTimeout(timeout), DialX_WithDisableProxy(true))
}

// DialContextWithoutProxy dials a connection with a context.
func DialContextWithoutProxy(ctx context.Context, addr string) (net.Conn, error) {
	var timeout = 30 * time.Second
	ddl, ok := ctx.Deadline()
	if ok {
		if du := ddl.Sub(time.Now()); du.Seconds() > 0 && du < timeout {
			timeout = du
		}
	}
	return DialX(
		addr,
		DialX_WithTimeout(timeout),
		DialX_WithDisableProxy(true),
		//DialX_WithContext(ctx),
	)
}

func NewDialGMTLSContextFunc(enableGM bool, preferGMTLS bool, onlyGMTLS bool, timeout time.Duration, opts ...dns_lookup.DNSOption) func(ctx context.Context, network string, addr string) (net.Conn, error) {
	return func(ctx context.Context, network string, addr string) (net.Conn, error) {
		ddl, ok := ctx.Deadline()
		if ok {
			if du := ddl.Sub(time.Now()); du.Seconds() > 0 && du < timeout {
				timeout = du
			}
		}
		return DialX(
			addr,
			DialX_WithTimeout(timeout),
			DialX_WithTLS(true),
			DialX_WithGMTLSSupport(enableGM),
			DialX_WithGMTLSPrefer(preferGMTLS),
			DialX_WithGMTLSOnly(onlyGMTLS),
			DialX_WithDNSOptions(opts...),
		)
	}
}

// DialTimeout is a shortcut for DialX with timeout
func DialTimeout(connectTimeout time.Duration, target string, proxy ...string) (net.Conn, error) {
	return DialX(target, DialX_WithProxy(proxy...), DialX_WithTimeout(connectTimeout))
}

// DialTLSTimeout is a shortcut for DialX with timeout
func DialTLSTimeout(timeout time.Duration, target string, tlsConfig any, proxy ...string) (net.Conn, error) {
	return DialX(target, DialX_WithProxy(proxy...), DialX_WithTimeout(timeout), DialX_WithTLS(true), DialX_WithTLSConfig(tlsConfig))
}

// DialTCPTimeout is alias for DialTimeout
func DialTCPTimeout(timeout time.Duration, target string, proxies ...string) (net.Conn, error) {
	return DialTimeout(timeout, target, proxies...)
}
