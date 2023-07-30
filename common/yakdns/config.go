package yakdns

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

type ReliableDialConfig struct {
	NoFallbackTCP bool
	RetryTimes    int // default 3

	// Disable System Resolver
	DisableSystemResolver bool

	// SpecificDNSServers 作为备选项
	FallbackSpecificDNS bool

	SpecificDNSServers []string

	Callback func(dnsType string, domain string, ip, fromServer, method string)
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

	log.Infof("dns lookup[%s]-%s: %10s -> %15s %s", dnsType, fromServer, domain, ip, method)
	if r.Callback != nil {
		r.Callback(dnsType, domain, ip, fromServer, method)
	}
}

func NewDefaultReliableDialConfig() *ReliableDialConfig {
	return &ReliableDialConfig{
		NoFallbackTCP: false,
		RetryTimes:    3,
	}
}

func WithNoFallbackTCP(b bool) func(*ReliableDialConfig) {
	return func(c *ReliableDialConfig) {
		c.NoFallbackTCP = b
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
