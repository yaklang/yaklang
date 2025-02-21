package dns_lookup

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"sync"
	"time"
)

var defaultYakDNSMutex = new(sync.Mutex)
var defaultYakDNSOptions []DNSOption

var DefaultCustomDNSServers = []string{
	"223.5.5.5", "223.6.6.6",
	"120.53.53.53", "1.1.1.1",
	"8.8.8.8",
}
var DefaultCustomDoHServers = []string{
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
}

func SetDefaultDNSOptions(opt ...DNSOption) {
	defaultYakDNSMutex.Lock()
	defer defaultYakDNSMutex.Unlock()

	defaultYakDNSOptions = opt
}

func GetDefaultOptions() []DNSOption {
	defaultYakDNSMutex.Lock()
	defer defaultYakDNSMutex.Unlock()

	var result = make([]DNSOption, len(defaultYakDNSOptions))
	copy(result, defaultYakDNSOptions)
	return result
}

var defaultDoHHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: nil,
	},
	Timeout: 5 * time.Second,
}

type ReliableDNSConfig struct {
	Timeout time.Duration
	Hosts   map[string]string

	PreferTCP   bool
	FallbackTCP bool
	RetryTimes  int // default 3

	// DoH config
	PreferDoH   bool
	FallbackDoH bool // as backup
	SpecificDoH []string

	etcHostNoCache bool
	// NoCache
	NoCache bool

	// Disable System Resolver
	DisableSystemResolver bool

	// SpecificDNSServers 作为备选项
	FallbackSpecificDNS bool

	SpecificDNSServers []string

	// ctx
	BaseContext context.Context
	cancel      context.CancelFunc

	Callback func(dnsType string, domain string, ip, fromServer, method string)

	mutex *sync.Mutex
	count int64

	OnFinished func()

	// blacklist
	DisabledDomain *utils.GlobFilter
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

	if !r.etcHostNoCache {
		if isV6 {
			ipv6DNSCache.SetWithTTL(domain, ip, time.Second*time.Duration(ttlInt))
		} else {
			ipv4DNSCache.SetWithTTL(domain, ip, time.Second*time.Duration(ttlInt))
		}
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.count++
	log.Debugf("dns lookup[%s]-%s: %10s -> %15s %s", dnsType, fromServer, domain, ip, method)
	if r.Callback != nil {
		r.Callback(dnsType, domain, ip, fromServer, method)
	}
}

func WithDNSServers(s ...string) DNSOption {
	return func(c *ReliableDNSConfig) {
		c.SpecificDNSServers = s
	}
}

func WithDNSPreferTCP(b bool) DNSOption {
	return func(config *ReliableDNSConfig) {
		config.PreferTCP = b
	}
}

func NewBackupInitilizedReliableDNSConfig() *ReliableDNSConfig {
	ctx, cancel := context.WithCancel(context.Background())
	config := &ReliableDNSConfig{
		BaseContext:        ctx,
		cancel:             cancel,
		FallbackTCP:        false,
		RetryTimes:         3,
		Timeout:            5 * time.Second,
		mutex:              new(sync.Mutex),
		SpecificDoH:        DefaultCustomDoHServers,
		SpecificDNSServers: DefaultCustomDNSServers,
		DisabledDomain:     utils.NewGlobFilter('.'),
	}
	return config
}

func NewDefaultReliableDNSConfig() *ReliableDNSConfig {
	config := NewBackupInitilizedReliableDNSConfig()
	if ret := GetDefaultOptions(); len(ret) > 0 {
		for _, o := range ret {
			o(config)
		}
	}
	return config
}

func WithDNSContext(ctx context.Context) DNSOption {
	return func(c *ReliableDNSConfig) {
		c.BaseContext, c.cancel = context.WithCancel(ctx)
	}
}

func (r *ReliableDNSConfig) GetBaseContext() context.Context {
	if r.BaseContext != nil {
		return r.BaseContext
	} else {
		r.BaseContext, r.cancel = context.WithTimeout(context.Background(), 30*time.Second)
	}
	return r.BaseContext
}

func WithDNSFallbackDoH(b bool) DNSOption {
	return func(c *ReliableDNSConfig) {
		c.FallbackDoH = b
	}
}

func WithDNSNoCache(b bool) DNSOption {
	return func(c *ReliableDNSConfig) {
		c.NoCache = b
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
		origin := config.Callback
		config.Callback = func(dnsType string, domain string, ip, fromServer, method string) {
			if origin != nil {
				origin(dnsType, domain, ip, fromServer, method)
			}
			cb(dnsType, domain, ip, fromServer, method)
		}
	}
}

func WithTimeout(timeout time.Duration) DNSOption {
	return func(config *ReliableDNSConfig) {
		config.Timeout = timeout
	}
}

func WithTemporaryHosts(i map[string]string) DNSOption {
	return func(config *ReliableDNSConfig) {
		config.Hosts = i
	}
}

func WithDNSOnFinished(cb func()) DNSOption {
	return func(config *ReliableDNSConfig) {
		config.OnFinished = cb
	}
}

func WithDNSDisabledDomain(domain ...string) DNSOption {
	return func(config *ReliableDNSConfig) {
		domain = utils.StringArrayFilterEmpty(domain)
		if len(domain) <= 0 {
			return
		}
		if config.DisabledDomain == nil {
			config.DisabledDomain = utils.NewGlobFilter('.')
		}
		config.DisabledDomain.Add(domain...)
	}
}
