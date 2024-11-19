package netx

import (
	"crypto/tls"
	"github.com/yaklang/yaklang/common/consts"
	"net"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/utils"
)

type dialXConfig struct {
	Timeout           time.Duration
	ForceDisableProxy bool
	// when empty proxy and EnableSystemProxyFromEnv(true),
	// fetch via getProxyFromEnv()
	EnableSystemProxyFromEnv bool
	ForceProxy               bool
	Proxy                    []string
	KeepAlive                time.Duration

	// EnableTLS is true, force to use TLS, auto upgrade
	EnableTLS               bool
	ShouldOverrideTLSConfig bool
	TLSConfig               *gmtls.Config
	//ShouldOverrideGMTLSConfig bool
	//GMTLSConfig               *gmtls.Config
	GMTLSSupport      bool
	GMTLSPrefer       bool
	GMTLSOnly         bool
	TLSTimeout        time.Duration
	ShouldOverrideSNI bool // High priority (will overwrite TlsConfig)
	SNI               string
	TLSNextProto      []string

	// Retry
	EnableTimeoutRetry  bool
	TimeoutRetryMax     int64
	TimeoutRetryMinWait time.Duration
	TimeoutRetryMaxWait time.Duration

	DNSOpts []DNSOption

	Debug bool

	// DisallowAddress
	DisallowAddress *utils.HostsFilter

	// ClientHelloSpec
	ClientHelloSpec *utls.ClientHelloSpec

	LocalAddr *net.UDPAddr

	JustListen bool // just listen udp , not connect .
}

type DialXOption func(c *dialXConfig)

var (
	defaultDialXOptions      []DialXOption
	defaultDialXOptionsMutex = new(sync.Mutex)
)

func SetDefaultDialXConfig(opt ...DialXOption) {
	defaultDialXOptionsMutex.Lock()
	defer defaultDialXOptionsMutex.Unlock()

	defaultDialXOptions = opt
}

func DialX_WithDisableProxy(b bool) DialXOption {
	return func(c *dialXConfig) {
		c.ForceDisableProxy = b
	}
}

func DialX_WithForceProxy(b bool) DialXOption {
	return func(c *dialXConfig) {
		c.ForceProxy = b
	}
}

func DialX_WithDisallowAddress(a ...string) DialXOption {
	return func(c *dialXConfig) {
		a = utils.StringArrayFilterEmpty(a)
		if len(a) == 0 {
			return
		}
		if c.DisallowAddress == nil {
			c.DisallowAddress = utils.NewHostsFilter()
		}
		c.DisallowAddress.Add(a...)
	}
}

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

func DialX_WithKeepAlive(aliveTime time.Duration) DialXOption {
	return func(c *dialXConfig) {
		c.KeepAlive = aliveTime
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

func DialX_Debug(b bool) DialXOption {
	return func(c *dialXConfig) {
		c.Debug = true
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
		c.ShouldOverrideTLSConfig = true
		c.TLSConfig = initTlsConfigVersion(config)
	}
}

func DialX_WithGMTLSPrefer(b bool) DialXOption {
	return func(c *dialXConfig) {
		c.GMTLSPrefer = b
	}
}

func DialX_WithGMTLSOnly(b bool) DialXOption {
	return func(c *dialXConfig) {
		c.GMTLSOnly = b
	}
}

func DialX_WithTimeout(timeout time.Duration) DialXOption {
	return func(c *dialXConfig) {
		c.Timeout = timeout
	}
}

func DialX_WithProxy(proxy ...string) DialXOption {
	proxy = utils.StringArrayFilterEmpty(proxy)
	if len(proxy) == 0 {
		return func(c *dialXConfig) {}
	}

	return func(c *dialXConfig) {
		c.Proxy = proxy
	}
}

func DialX_WithTLSNextProto(nextProtos ...string) DialXOption {
	return func(c *dialXConfig) {
		c.TLSNextProto = nextProtos
	}
}

func DialX_WithTLSConfig(tlsConfig any) DialXOption {
	return func(c *dialXConfig) {
		c.EnableTLS = true
		switch ret := tlsConfig.(type) {
		case *tls.Config:
			if gmtlsConfig, err := gmtls.SimpleTlsConfigToGmTlsConfig(ret); err == nil {
				c.TLSConfig = initTlsConfigVersion(gmtlsConfig)
			}
		case *gmtls.Config:
			c.ShouldOverrideTLSConfig = true
			c.TLSConfig = initTlsConfigVersion(ret)
		case *gmtls.GMSupport:
			c.ShouldOverrideTLSConfig = true
			c.TLSConfig = initTlsConfigVersion(&gmtls.Config{
				GMSupport: ret,
			})
		}
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

func DialX_WithEnableSystemProxyFromEnv(b bool) DialXOption {
	return func(c *dialXConfig) {
		c.EnableSystemProxyFromEnv = b
	}
}

func DialX_WithClientHelloSpec(spec *utls.ClientHelloSpec) DialXOption {
	return func(c *dialXConfig) {
		c.ClientHelloSpec = spec
	}
}

func DialX_WithLocalAddr(addr *net.UDPAddr) DialXOption {
	return func(c *dialXConfig) {
		c.LocalAddr = addr
	}
}

func DialX_WithUdpJustListen(b bool) DialXOption {
	return func(c *dialXConfig) {
		c.JustListen = b
	}
}

type TLSStrategy string

const (
	TLS_Strategy_GMDail                   TLSStrategy = "gmtls"
	TLS_Strategy_GMDial_Without_GMSupport TLSStrategy = "gmtls-ns"
	TLS_Strategy_Ordinary                 TLSStrategy = "tls"
)

func initTlsConfigVersion(config *gmtls.Config) *gmtls.Config {
	minVer, maxVer := consts.GetGlobalTLSVersion()
	if config.MinVersion == 0 {
		config.MinVersion = minVer
	}
	if config.MaxVersion == 0 {
		config.MaxVersion = maxVer
	}
	return config
}
