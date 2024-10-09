package netx

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// dialPlainTCPConnWithRetry just handle plain tcp connection
// no tls here, but proxy here
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
	proxyHaveTimeoutError := false
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

	if len(config.Proxy) == 0 && config.EnableSystemProxyFromEnv && FixProxy(GetProxyFromEnv()) != "" {
		config.Proxy = append(config.Proxy, FixProxy(GetProxyFromEnv()))
	}

	if len(config.Proxy) <= 0 || config.ForceDisableProxy {
		if len(config.Proxy) == 0 && !config.ForceDisableProxy && config.ForceProxy {
			return nil, utils.Errorf("force proxy but no proxy available for target: %v", target)
		}

		if config.Debug {
			log.Infof("dial %s without proxy", target)
		}
		host, port, err := utils.ParseStringToHostPort(target)
		if err != nil {
			return nil, utils.Errorf("invalid target %#v, cannot find host:port", target)
		}
		ip := host
		if net.ParseIP(utils.FixForParseIP(host)) == nil {
			// not valid ip
			ip = LookupFirst(host, config.DNSOpts...)
		}
		if ip == "" {
			return nil, utils.Errorf("cannot resolve %v", target)
		}

		// handle ip address
		if config.DisallowAddress != nil {
			if config.DisallowAddress.Contains(ip) {
				return nil, utils.Errorf("disallow address %v by config(check your yakit system/network config)", ip)
			}
		}
		conn, err = net.DialTimeout("tcp", utils.HostPort(ip, port), config.Timeout)
		if err != nil {
			if config.Debug {
				log.Errorf("dial %s failed: %v", target, err)
			}
			lastError = err
			var opError *net.OpError
			switch {
			case errors.As(err, &opError):
				if opError.Timeout() {
					if config.Debug {
						log.Infof("dial %s timeout, retrying", target)
					}
					utils.JitterBackoff(minWait, maxWait, int(timeoutRetryCount+1))
					goto RETRY
				}
			}
			return nil, err
		}
		return conn, nil
	}

	DnsConfig := NewDefaultReliableDNSConfig()
	for _, o := range config.DNSOpts {
		o(DnsConfig)
	}

	if DnsConfig.DisabledDomain != nil {
		host, _, err := utils.ParseStringToHostPort(target)
		if err == nil && DnsConfig.DisabledDomain.Contains(host) {
			return nil, utils.Errorf("disallow domain %v by config(check your yakit system/network config)", target)
		}
	}

	var errs error
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
			errs = utils.JoinErrors(errs, err)
			continue
		}
		return conn, nil
	}
	if proxyHaveTimeoutError {
		proxyHaveTimeoutError = false
		goto RETRY
	}
	return nil, utils.Wrapf(errs, "connect: %v failed: no proxy available (in %v)", target, config.Proxy)
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
		DisallowAddress:     utils.NewHostsFilter(),
	}

	// default init
	defaultDialXOptionsMutex.Lock()
	for _, o := range defaultDialXOptions {
		o(config)
	}
	defaultDialXOptionsMutex.Unlock()

	// user init
	for _, o := range opt {
		o(config)
	}

	clientHelloSpec := config.ClientHelloSpec
	useTls := config.EnableTLS || config.GMTLSSupport

	if !useTls {
		if config.Debug {
			log.Infof("dial %s without tls", target)
		}
		return dialPlainTCPConnWithRetry(target, config)
	}

	// Enable TLS as default
	strategies := []TLSStrategy{TLS_Strategy_Ordinary}
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
		Renegotiation:      tls.RenegotiateFreelyAsClient,
	}
	if config.ShouldOverrideTLSConfig {
		tlsConfig = config.TLSConfig
	}
	tlsTimeout := 10 * time.Second
	if config.TLSTimeout > 0 {
		tlsTimeout = config.TLSTimeout
	}

	errs := make([]error, 0, len(strategies))
	for _, strategy := range strategies {
		if config.Debug {
			log.Infof("dial %v with tls strategy: %v", target, strategy)
		}
		conn, err := dialPlainTCPConnWithRetry(target, config)
		if err != nil {
			return nil, err
		}

		switch strategy {
		case TLS_Strategy_Ordinary:
			tlsConn, err := UpgradeToTLSConnectionWithTimeout(conn, sni, tlsConfig, tlsTimeout, clientHelloSpec, config.TLSNextProto...)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			return tlsConn, nil
		case TLS_Strategy_GMDail:
			gmtlsConfig := &gmtls.Config{
				GMSupport: &gmtls.GMSupport{
					WorkMode: gmtls.ModeGMSSLOnly,
				},
				ServerName:         sni,
				MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
				MaxVersion:         tls.VersionTLS13,
				InsecureSkipVerify: true,
				Renegotiation:      gmtls.RenegotiateFreelyAsClient,
			}
			if config.ShouldOverrideGMTLSConfig {
				gmtlsConfig = config.GMTLSConfig
			}

			tlsConn, err := UpgradeToTLSConnectionWithTimeout(conn, sni, gmtlsConfig, tlsTimeout, clientHelloSpec, config.TLSNextProto...)
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
				Renegotiation:      gmtls.RenegotiateFreelyAsClient,
			}
			tlsConn, err := UpgradeToTLSConnectionWithTimeout(conn, sni, gmtlsConfig, tlsTimeout, clientHelloSpec, config.TLSNextProto...)
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
