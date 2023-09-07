package netx

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"sync/atomic"
	"time"
)

type dialXConfig struct {
	Timeout time.Duration
	Proxy   []string

	// EnableTLS is true, force to use TLS, auto upgrade
	EnableTLS                 bool
	ShouldOverrideTLSConfig   bool
	TLSConfig                 *tls.Config
	ShouldOverrideGMTLSConfig bool
	GMTLSConfig               *gmtls.Config
	GMTLSSupport              bool
	GMTLSPrefer               bool
	GMTLSOnly                 bool
	TLSTimeout                time.Duration
	ShouldOverrideSNI         bool
	SNI                       string

	// Retry
	EnableTimeoutRetry  bool
	TimeoutRetryMax     int64
	TimeoutRetryMinWait time.Duration
	TimeoutRetryMaxWait time.Duration

	DNSOpts []DNSOption
}

type DialXOption func(c *dialXConfig)

func DialX_WithTimeoutRetry(max int) DialXOption {
	return func(c *dialXConfig) {
		c.EnableTimeoutRetry = true
		c.TimeoutRetryMax = int64(max)
	}
}

func DialX_WithDNSOptions(opt ...DNSOption) DialXOption {
	return func(c *dialXConfig) {
		c.DNSOpts = opt
	}
}

func DialX_WithTimeoutRetryWait(timeout time.Duration) DialXOption {
	return func(c *dialXConfig) {
		c.EnableTimeoutRetry = true
		c.TimeoutRetryMinWait = timeout
		c.TimeoutRetryMaxWait = timeout
	}
}

func DialX_WithTimeoutRetryWaitRange(min, max time.Duration) DialXOption {
	return func(c *dialXConfig) {
		c.EnableTimeoutRetry = true
		c.TimeoutRetryMinWait = min
		c.TimeoutRetryMaxWait = max
	}
}

func DialX_WithSNI(sni string) DialXOption {
	return func(c *dialXConfig) {
		c.ShouldOverrideSNI = true
		c.SNI = sni
	}
}

func DialX_WithTLSTimeout(t time.Duration) DialXOption {
	return func(c *dialXConfig) {
		c.TLSTimeout = t
	}
}

func DialX_WithTLS(b bool) DialXOption {
	return func(c *dialXConfig) {
		c.EnableTLS = b
	}
}

func DialX_WithGMTLSConfig(config *gmtls.Config) DialXOption {
	return func(c *dialXConfig) {
		c.EnableTLS = true
		c.ShouldOverrideGMTLSConfig = true
		c.GMTLSConfig = config
	}
}

func DialX_WithGMTLSPrefer(b bool) DialXOption {
	return func(c *dialXConfig) {
		c.GMTLSSupport = true
		c.GMTLSPrefer = b
	}
}

func DialX_WithGMTLSOnly(b bool) DialXOption {
	return func(c *dialXConfig) {
		c.GMTLSSupport = true
		c.GMTLSOnly = b
	}
}

func DialX_WithTimeout(timeout time.Duration) DialXOption {
	return func(c *dialXConfig) {
		c.Timeout = timeout
	}
}

func DialX_WithProxy(proxy ...string) DialXOption {
	return func(c *dialXConfig) {
		c.Proxy = utils.StringArrayFilterEmpty(proxy)
	}
}

func DialX_WithTLSConfig(tlsConfig *tls.Config) DialXOption {
	return func(c *dialXConfig) {
		c.EnableTLS = true
		c.ShouldOverrideTLSConfig = true
		c.TLSConfig = tlsConfig
	}
}

func DialX_WithGMTLSSupport(b bool) DialXOption {
	return func(c *dialXConfig) {
		if b {
			c.GMTLSSupport = true
			c.EnableTLS = true
		}
	}
}

type TLSStrategy string

const (
	TLS_Strategy_GMDail                   TLSStrategy = "gmtls"
	TLS_Strategy_GMDial_Without_GMSupport TLSStrategy = "gmtls-ns"
	TLS_Strategy_Ordinary                 TLSStrategy = "tls"
)

func dialPlainTCPConnWithRetry(target string, config *dialXConfig) (net.Conn, error) {
	var timeoutRetryMax int64 = 1
	if config.EnableTimeoutRetry {
		timeoutRetryMax = config.TimeoutRetryMax
	} else {
		timeoutRetryMax = 0
	}

	// do first as zero
	var timeoutRetryCount int64 = -1
	addRetry := func() int64 {
		return atomic.AddInt64(&timeoutRetryCount, 1)
	}

	minWait, maxWait := config.TimeoutRetryMinWait, config.TimeoutRetryMaxWait
	if minWait > maxWait {
		minWait, maxWait = maxWait, minWait
	}

	var lastError error
	var proxyHaveTimeoutError = false
RETRY:
	if ret := addRetry(); ret > timeoutRetryMax {
		if timeoutRetryMax > 0 {
			return nil, fmt.Errorf("timeout retry(%v) > max(%v)", ret, timeoutRetryMax)
		}
		if lastError != nil {
			return nil, lastError
		}
		return nil, fmt.Errorf("i/o timeout for %v", target)
	}

	// handle plain
	// not need to upgrade
	var conn net.Conn
	var err error
	dialerFunc := NewDialContextFunc(config.Timeout, config.DNSOpts...)
	if len(config.Proxy) <= 0 {
		conn, err = dialerFunc(utils.TimeoutContext(config.Timeout), "tcp", target)
		//conn, err := DialTCPTimeout(config.Timeout, target, config.Proxy...)
		if err != nil {
			lastError = err
			var opError *net.OpError
			switch {
			case errors.As(err, &opError):
				if opError.Timeout() {
					jitterBackoff(minWait, maxWait, int(timeoutRetryCount+1))
					goto RETRY
				}
			}
			return nil, err
		}
		return conn, nil
	}

	for _, proxy := range config.Proxy {
		conn, err := getConnForceProxy(target, proxy, config.Timeout)
		if err != nil {
			log.Errorf("proxy conn failed: %s", err)
			if !proxyHaveTimeoutError {
				var opError *net.OpError
				if errors.As(err, &opError) {
					proxyHaveTimeoutError = true
				}
			}
			continue
		}
		return conn, nil
	}
	if proxyHaveTimeoutError {
		proxyHaveTimeoutError = false
		goto RETRY
	}
	return nil, utils.Errorf("connect: %v failed: no proxy available (in %v)", target, config.Proxy)
}

/*
DialX is netx dial with more options

Proxy is a list of proxy servers, if empty, no proxy will be used, otherwise retry with each proxy until success (no redirect)
*/
func DialX(target string, opt ...DialXOption) (net.Conn, error) {
	config := &dialXConfig{
		Timeout:             10 * time.Second,
		TLSTimeout:          5 * time.Second,
		EnableTimeoutRetry:  false,
		TimeoutRetryMax:     3,
		TimeoutRetryMinWait: 100 * time.Millisecond,
		TimeoutRetryMaxWait: 500 * time.Millisecond,
	}

	for _, o := range opt {
		o(config)
	}

	useTls := config.EnableTLS || config.GMTLSSupport

	if !useTls {
		return dialPlainTCPConnWithRetry(target, config)
	}

	// Enable TLS as default
	var strategies = []TLSStrategy{TLS_Strategy_Ordinary}
	if config.GMTLSSupport {
		if config.GMTLSOnly {
			strategies = []TLSStrategy{TLS_Strategy_GMDail, TLS_Strategy_GMDial_Without_GMSupport}
		} else if config.GMTLSPrefer {
			strategies = []TLSStrategy{TLS_Strategy_GMDail, TLS_Strategy_Ordinary, TLS_Strategy_GMDial_Without_GMSupport}
		} else {
			strategies = []TLSStrategy{TLS_Strategy_Ordinary, TLS_Strategy_GMDail, TLS_Strategy_GMDial_Without_GMSupport}
		}
	}

	sni := utils.ExtractHost(target)
	if config.ShouldOverrideSNI {
		sni = config.SNI
	}

	var tlsConfig any = &tls.Config{
		ServerName:         sni,
		MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
	}
	if config.ShouldOverrideTLSConfig {
		tlsConfig = config.TLSConfig
	}
	var tlsTimeout = 10 * time.Second
	if config.TLSTimeout > 0 {
		tlsTimeout = config.TLSTimeout
	}

	var errs = make([]error, 0, len(strategies))
	for _, strategy := range strategies {
		conn, err := dialPlainTCPConnWithRetry(target, config)
		if err != nil {
			return nil, err
		}

		switch strategy {
		case TLS_Strategy_Ordinary:
			tlsConn, err := UpgradeToTLSConnectionWithTimeout(conn, sni, tlsConfig, tlsTimeout)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			return tlsConn, nil
		case TLS_Strategy_GMDail:
			if config.ShouldOverrideGMTLSConfig {
				tlsConfig = config.GMTLSConfig
			} else {
				tlsConfig = &gmtls.Config{
					GMSupport: &gmtls.GMSupport{
						WorkMode: gmtls.ModeGMSSLOnly,
					},
					ServerName:         sni,
					MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
					MaxVersion:         tls.VersionTLS13,
					InsecureSkipVerify: true,
				}
			}
			tlsConn, err := UpgradeToTLSConnectionWithTimeout(conn, sni, tlsConfig, tlsTimeout)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			return tlsConn, nil
		case TLS_Strategy_GMDial_Without_GMSupport:
			gmtlsConfig := &gmtls.Config{
				ServerName:         sni,
				MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
				MaxVersion:         tls.VersionTLS13,
				InsecureSkipVerify: true,
			}
			tlsConn, err := UpgradeToTLSConnectionWithTimeout(conn, sni, gmtlsConfig, tlsTimeout)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			return tlsConn, nil
		default:
			return nil, utils.Errorf("unknown tls strategy %v", strategy)
		}
	}

	if len(errs) > 0 {
		return nil, utils.Errorf("all tls strategy failed: %v", errs)
	}
	return nil, utils.Error("unknown tls strategy error, BUG here!")
}
