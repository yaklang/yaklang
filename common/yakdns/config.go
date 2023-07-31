package yakdns

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"sync"
	"time"
)

type ReliableDNSConfig struct {
	Timeout time.Duration

	FallbackTCP bool
	RetryTimes  int // default 3

	// DoH config
	PreferDoH   bool
	FallbackDoH bool // as backup
	SpecificDoH []string

	// Disable System Resolver
	DisableSystemResolver bool

	// SpecificDNSServers 作为备选项
	FallbackSpecificDNS bool

	SpecificDNSServers []string

	Callback func(dnsType string, domain string, ip, fromServer, method string)

	mutex *sync.Mutex
	count int64

	dohHTTPClient *http.Client
}

type DNSOption func(*ReliableDNSConfig)

func (r *ReliableDNSConfig) call(dnsType, domain, ip, fromServer, method string, ttl ...int) {
	var isV6 = utils.IsIPv6(ip)
	if dnsType == "" {
		if utils.IsIPv4(ip) {
			dnsType = "A"
		} else if isV6 {
			dnsType = "AAAA"
		}
	}

	var ttlInt int
	if len(ttl) > 0 {
		ttlInt = ttl[0]
	}
	if ttlInt <= 0 {
		ttlInt = 300
	}

	if isV6 {
		ipv6DNSCache.SetWithTTL(domain, ip, time.Second*time.Duration(ttlInt))
	} else {
		ipv4DNSCache.SetWithTTL(domain, ip, time.Second*time.Duration(ttlInt))
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.count++
	log.Infof("dns lookup[%s]-%s: %10s -> %15s %s", dnsType, fromServer, domain, ip, method)
	if r.Callback != nil {
		r.Callback(dnsType, domain, ip, fromServer, method)
	}
}

func WithDNSServers(s ...string) DNSOption {
	return func(c *ReliableDNSConfig) {
		c.SpecificDNSServers = s
	}
}

func NewDefaultReliableDNSConfig() *ReliableDNSConfig {
	return &ReliableDNSConfig{
		FallbackTCP: false,
		RetryTimes:  3,
		Timeout:     5 * time.Second,
		mutex:       new(sync.Mutex),
		SpecificDoH: []string{
			// aliyun
			"https://223.5.5.5/resolve",
			"https://223.6.6.6/resolve",

			// tencent
			"https://1.12.12.12/dns-query",
			"https://120.53.53.53/dns-query",

			// public
			"https://1.1.1.1/dns-query",
			"https://8.8.8.8/resolve",
			"https://8.8.4.4/resolve",
		},
		dohHTTPClient: &http.Client{
			Transport: &http.Transport{
				Proxy: nil,
			},
			Timeout: 5 * time.Second,
		},
	}
}

func WithDNSFallbackDoH(b bool) DNSOption {
	return func(c *ReliableDNSConfig) {
		c.FallbackDoH = b
	}
}

func WithDNSPreferDoH(b bool) DNSOption {
	return func(c *ReliableDNSConfig) {
		c.PreferDoH = b
	}
}

func WithDNSSpecificDoH(s ...string) DNSOption {
	return func(c *ReliableDNSConfig) {
		c.SpecificDoH = s
	}
}

func WithDNSFallbackTCP(b bool) DNSOption {
	return func(c *ReliableDNSConfig) {
		c.FallbackTCP = b
	}
}

func WithDNSRetryTimes(i int) DNSOption {
	return func(c *ReliableDNSConfig) {
		c.RetryTimes = i
	}
}

func WithDNSDisableSystemResolver(b bool) DNSOption {
	return func(c *ReliableDNSConfig) {
		c.DisableSystemResolver = b
	}
}

func WithDNSFallbackSpecificDNS(b bool) DNSOption {
	return func(c *ReliableDNSConfig) {
		c.FallbackSpecificDNS = b
	}
}

func WithDNSCallback(cb func(dnsType, domain, ip, fromServer, method string)) DNSOption {
	return func(config *ReliableDNSConfig) {
		config.Callback = cb
	}
}
