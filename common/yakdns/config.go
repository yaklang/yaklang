package yakdns

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
	"time"
)

type ReliableDialConfig struct {
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
}

func (r *ReliableDialConfig) call(dnsType, domain, ip, fromServer, method string, ttl ...int) {
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
		v6Cache.SetWithTTL(domain, ip, time.Second*time.Duration(ttlInt))
	} else {
		cache.SetWithTTL(domain, ip, time.Second*time.Duration(ttlInt))
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.count++
	log.Infof("dns lookup[%s]-%s: %10s -> %15s %s", dnsType, fromServer, domain, ip, method)
	if r.Callback != nil {
		r.Callback(dnsType, domain, ip, fromServer, method)
	}
}

func NewDefaultReliableDialConfig() *ReliableDialConfig {
	return &ReliableDialConfig{
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
	}
}

func WithFallbackDoH(b bool) func(*ReliableDialConfig) {
	return func(c *ReliableDialConfig) {
		c.FallbackDoH = b
	}
}

func WithSpecificDoH(s ...string) func(*ReliableDialConfig) {
	return func(c *ReliableDialConfig) {
		c.SpecificDoH = s
	}
}

func WithNoFallbackTCP(b bool) func(*ReliableDialConfig) {
	return func(c *ReliableDialConfig) {
		c.FallbackTCP = b
	}
}

func WithRetryTimes(i int) func(*ReliableDialConfig) {
	return func(c *ReliableDialConfig) {
		c.RetryTimes = i
	}
}

func WithDisableSystemResolver(b bool) func(*ReliableDialConfig) {
	return func(c *ReliableDialConfig) {
		c.DisableSystemResolver = b
	}
}

func WithFallbackSpecificDNS(b bool) func(*ReliableDialConfig) {
	return func(c *ReliableDialConfig) {
		c.FallbackSpecificDNS = b
	}
}
