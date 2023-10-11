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

// NewDialContextFuncEx 扩展方法支持更多的Dial配置
func NewDialContextFuncEx(config *dialXConfig) func(ctx context.Context, network string, addr string) (net.Conn, error) {
	return func(ctx context.Context, network string, addr string) (net.Conn, error) {
		d := net.Dialer{
			Timeout:   config.Timeout,
			KeepAlive: config.KeepAlive,
		}
		host, port, err := utils.ParseStringToHostPort(addr)
		if err != nil {
			return nil, utils.Errorf("cannot parse %v as host:port, reason: %v", addr, err)
		}

		ddl, ok := ctx.Deadline()
		if ok {
			if du := ddl.Sub(time.Now()); du.Seconds() > 0 {
				d.Timeout = du
			}
		}

		if utils.IsIPv4(host) || utils.IsIPv6(host) {
			return d.Dial(network, utils.HostPort(host, port))
		}

		newHost := LookupFirst(host, config.DNSOpts...)
		if newHost == "" {
			return nil, utils.Errorf("cannot resolve %v", addr)
		}
		return d.Dial(network, utils.HostPort(newHost, port))
	}
}

var defaultDialContextFunc = NewDialContextFunc(30 * time.Second)

// DialTimeoutWithoutProxy dials a connection with a timeout.
func DialTimeoutWithoutProxy(timeout time.Duration, network, addr string) (net.Conn, error) {
	return defaultDialContextFunc(utils.TimeoutContext(timeout), network, addr)
}

// DialContextWithoutProxy dials a connection with a context.
func DialContextWithoutProxy(ctx context.Context, network, addr string) (net.Conn, error) {
	return defaultDialContextFunc(ctx, network, addr)
}

func NewDialGMTLSContextFunc(enableGM bool, preferGMTLS bool, onlyGMTLS bool, timeout time.Duration, opts ...DNSOption) func(ctx context.Context, network string, addr string) (net.Conn, error) {
	origin := NewDialContextFunc(timeout, opts...)
	return func(ctx context.Context, network string, addr string) (net.Conn, error) {
		targetHost, _, err := utils.ParseStringToHostPort(addr)
		if err != nil {
			targetHost = addr
		}

		var strategies = []TLSStrategy{TLS_Strategy_Ordinary}
		if enableGM {
			if onlyGMTLS {
				strategies = []TLSStrategy{TLS_Strategy_GMDail, TLS_Strategy_GMDial_Without_GMSupport}
			} else if preferGMTLS {
				strategies = []TLSStrategy{TLS_Strategy_GMDail, TLS_Strategy_Ordinary, TLS_Strategy_GMDial_Without_GMSupport}
			} else {
				strategies = []TLSStrategy{TLS_Strategy_Ordinary, TLS_Strategy_GMDail, TLS_Strategy_GMDial_Without_GMSupport}
			}
		}

		var errs = make([]error, 0, len(strategies))
		for _, strategy := range strategies {
			plainConn, err := origin(ctx, network, addr)
			if err != nil {
				return nil, utils.Errorf("dialer with TCP dial: %v", err)
			}

			switch strategy {
			case TLS_Strategy_Ordinary:
				tlsConfig := &tls.Config{
					ServerName:         targetHost,
					MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
					MaxVersion:         tls.VersionTLS13,
					InsecureSkipVerify: true,
				}
				conn, err := UpgradeToTLSConnection(plainConn, targetHost, tlsConfig)
				if err != nil {
					plainConn.Close()
					errs = append(errs, err)
					continue
				}
				return conn, nil
			case TLS_Strategy_GMDail:
				gmtlsConfig := &gmtls.Config{
					GMSupport: &gmtls.GMSupport{
						WorkMode: gmtls.ModeGMSSLOnly,
					},
					ServerName:         targetHost,
					MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
					MaxVersion:         tls.VersionTLS13,
					InsecureSkipVerify: true,
				}
				conn, err := UpgradeToTLSConnection(plainConn, targetHost, gmtlsConfig)
				if err != nil {
					plainConn.Close()
					errs = append(errs, err)
					continue
				}
				return conn, nil
			case TLS_Strategy_GMDial_Without_GMSupport:
				gmtlsConfig := &gmtls.Config{
					ServerName:         targetHost,
					MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
					MaxVersion:         tls.VersionTLS13,
					InsecureSkipVerify: true,
				}
				conn, err := UpgradeToTLSConnection(plainConn, targetHost, gmtlsConfig)
				if err != nil {
					plainConn.Close()
					errs = append(errs, err)
					continue
				}
				return conn, nil
			default:
				return nil, utils.Errorf("unknown tls strategy %v", strategy)
			}
		}

		if len(errs) > 0 {
			return nil, utils.Errorf("all tls strategy failed: %v", errs)
		}
		return nil, utils.Error("unknown tls strategy error, BUG here!")
	}
}
