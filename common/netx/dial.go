package netx

import (
	"context"
	"crypto/tls"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"time"
)

// NewDialContextFunc is a function that can be used to dial a connection.
func NewDialContextFunc(timeout time.Duration, opts ...DNSOption) func(ctx context.Context, network string, addr string) (net.Conn, error) {
	return func(ctx context.Context, network string, addr string) (net.Conn, error) {
		host, port, err := utils.ParseStringToHostPort(addr)
		if err != nil {
			return nil, utils.Errorf("cannot parse %v as host:port, reason: %v", addr, err)
		}

		ddl, ok := ctx.Deadline()
		if ok {
			if du := ddl.Sub(time.Now()); du.Seconds() > 0 {
				timeout = du
			}
		}

		if utils.IsIPv4(host) || utils.IsIPv6(host) {
			return net.DialTimeout(network, utils.HostPort(host, port), timeout)
		}

		newHost := LookupFirst(host, opts...)
		if newHost == "" {
			return nil, utils.Errorf("cannot resolve %v", addr)
		}
		return net.DialTimeout(network, utils.HostPort(newHost, port), timeout)
	}
}

var defaultDialContextFunc = NewDialContextFunc(30 * time.Second)
var defaultDialGMTLSContextFunc = NewDialGMTLSContextFunc(true, false, false, 30*time.Second)
var defaultDialForceGMTLSContextFunc = NewDialGMTLSContextFunc(true, false, true, 30*time.Second)
var defaultDialTLSContextFunc = NewDialGMTLSContextFunc(false, false, false, 30*time.Second)

// DialTimeoutWithoutProxy dials a connection with a timeout.
func DialTimeoutWithoutProxy(timeout time.Duration, network, addr string) (net.Conn, error) {
	return defaultDialContextFunc(utils.TimeoutContext(timeout), network, addr)
}

// DialContextWithoutProxy dials a connection with a context.
func DialContextWithoutProxy(ctx context.Context, network, addr string) (net.Conn, error) {
	return defaultDialContextFunc(ctx, network, addr)
}

// DialTLSContextWithoutProxy dials a TLS connection with a context.
func DialTLSContextWithoutProxy(ctx context.Context, network, addr string, tlsConfig *tls.Config) (net.Conn, error) {
	return defaultDialTLSContextFunc(ctx, network, addr)
}

// DialAutoGMTLSContextWithoutProxy dials a GMTLS connection with a context.
func DialAutoGMTLSContextWithoutProxy(ctx context.Context, network, addr string) (net.Conn, error) {
	return defaultDialGMTLSContextFunc(ctx, network, addr)
}

// DialForceGMTLSContextWithoutProxy dials a GMTLS connection with a context.
func DialForceGMTLSContextWithoutProxy(ctx context.Context, network, addr string) (net.Conn, error) {
	return defaultDialGMTLSContextFunc(ctx, network, addr)
}

func UpgradeToTLSConnection(conn net.Conn, sni string, i any) (net.Conn, error) {
	return UpgradeToTLSConnectionWithTimeout(conn, sni, i, 10*time.Second)
}

func UpgradeToTLSConnectionWithTimeout(conn net.Conn, sni string, i any, timeout time.Duration) (net.Conn, error) {
	if i == nil {
		i = &tls.Config{
			ServerName:         sni,
			MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
			MaxVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true,
		}
	}
	var gmtlsConfig *gmtls.Config
	var tlsConfig *tls.Config
	// i is a *tls.Config or *gmtls.Config
	switch ret := i.(type) {
	case *tls.Config:
		tlsConfig = ret
	case *gmtls.Config:
		gmtlsConfig = ret
	case *gmtls.GMSupport:
		gmtlsConfig = &gmtls.Config{
			GMSupport:          ret,
			ServerName:         sni,
			MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
			MaxVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true,
		}
	default:
		return nil, utils.Errorf("invalid tlsConfig type %T", i)
	}

	if tlsConfig != nil {
		var sConn = tls.Client(conn, tlsConfig)
		err := sConn.HandshakeContext(utils.TimeoutContext(timeout))
		if err != nil {
			return nil, err
		}
		return sConn, nil
	} else if gmtlsConfig != nil {
		var sConn = gmtls.Client(conn, gmtlsConfig)
		err := sConn.HandshakeContext(utils.TimeoutContext(timeout))
		if err != nil {
			return nil, err
		}
		return sConn, nil
	} else {
		return nil, utils.Errorf("invalid tlsConfig type %T", i)
	}
}

func NewDialGMTLSContextFunc(enableGM bool, preferGMTLS bool, onlyGMTLS bool, timeout time.Duration, opts ...DNSOption) func(ctx context.Context, network string, addr string) (net.Conn, error) {
	origin := NewDialContextFunc(timeout, opts...)
	return func(ctx context.Context, network string, addr string) (net.Conn, error) {
		plainConn, err := origin(ctx, network, addr)
		if err != nil {
			return nil, utils.Errorf("gmtls dialer with TCP dial: %v", err)
		}
		targetHost, _, err := utils.ParseStringToHostPort(addr)
		if err != nil {
			targetHost = addr
		}

		gmtlsConfig := &gmtls.GMSupport{
			WorkMode: "GMSSLOnly",
		}
		tlsConfig := &tls.Config{
			ServerName:         targetHost,
			MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
			MaxVersion:         tls.VersionTLS13,
			InsecureSkipVerify: true,
		}

		if !enableGM {
			sconn, err := UpgradeToTLSConnection(plainConn, targetHost, tlsConfig)
			if err != nil {
				plainConn.Close()
				return nil, err
			}
			return sconn, nil
		}

		if onlyGMTLS {
			conn, err := UpgradeToTLSConnection(plainConn, targetHost, gmtlsConfig)
			if err != nil {
				plainConn.Close()
				return nil, utils.Errorf("gmtls dialer with gmtls dial: %v", err)
			}
			return conn, nil
		}

		if preferGMTLS {
			conn, err := UpgradeToTLSConnection(plainConn, targetHost, gmtlsConfig)
			if err != nil {
				plainConn.Close()

				plainConn, err := origin(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				conn, err := UpgradeToTLSConnection(plainConn, targetHost, tlsConfig)
				if err != nil {
					plainConn.Close()
					return nil, utils.Errorf("dialer with tls dial: %v", err)
				}
				return conn, nil
			}
			return conn, nil
		} else {
			conn, err := UpgradeToTLSConnection(plainConn, targetHost, tlsConfig)
			if err != nil {
				plainConn.Close()

				plainConn, err := origin(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				conn, err := UpgradeToTLSConnection(plainConn, targetHost, gmtlsConfig)
				if err != nil {
					plainConn.Close()
					return nil, utils.Errorf("dialer with gmtls dial: %v", err)
				}
				return conn, nil
			}
			return conn, nil
		}
	}
}
