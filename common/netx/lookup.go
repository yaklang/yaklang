package netx

import (
	"context"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	ipv6DNSCache = utils.NewTTLCache[string]()
	ipv4DNSCache = utils.NewTTLCache[string]()
)

func reliableLookupHost(host string, opt ...DNSOption) error {
	config := NewDefaultReliableDNSConfig()
	for _, o := range opt {
		o(config)
	}

	defer func() {
		if config != nil && config.OnFinished != nil {
			config.OnFinished()
		}
	}()

	if strings.Contains(host, ":") {
		host = utils.ExtractHost(host)
	}

	if config.DisabledDomain != nil {
		if config.DisabledDomain.Contains(host) {
			return utils.Errorf("domain %s is disabled(forbidden by yaklang config, check WithDNSDisabledDomain)", host)
		}
	}

	if config.Hosts != nil && len(config.Hosts) > 0 {
		result, ok := config.Hosts[host]
		if ok && result != "" {
			config.etcHostNoCache = true
			config.call("", host, result, "hosts", "hosts")
			return nil
		}
	}

	// handle hosts
	result, ok := GetHost(host)
	if ok && result != "" {
		config.call("", host, result, "hosts", "hosts")
		return nil
	}

	if !config.NoCache {
		// ttlcache v4 > v6
		result, ok := ipv4DNSCache.Get(host)
		if ok {
			config.call("", host, result, "cache", "cache")
			return nil
		}
		result, ok = ipv6DNSCache.Get(host)
		if ok {
			config.call("", host, result, "cache", "cache")
			return nil
		}
	}

	if utils.IsIPv4(host) || utils.IsIPv6(host) {
		config.call("", host, host, "system", "system")
		return nil
	}

	// handle system resolver
	if !config.DisableSystemResolver {
		nativeLookupHost(host, config)
		if config.count > 0 {
			return nil
		}
	}

	startDoH := func() {
		if len(config.SpecificDoH) > 0 {
			swg := utils.NewSizedWaitGroup(5)
			dohCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			for _, doh := range config.SpecificDoH {
				err := swg.AddWithContext(dohCtx)
				if err != nil {
					break
				}
				dohUrl := doh
				go func() {
					defer func() {
						if err := recover(); err != nil {
							log.Errorf("doh server %s panic: %v", dohUrl, err)
							utils.PrintCurrentGoroutineRuntimeStack()
						}
						swg.Done()
					}()
					err := dohRequest(host, dohUrl, config)
					if err != nil {
						log.Debugf("doh request failed: %s", err)
					}
				}()
			}
			swg.Wait()
		}
	}

	var dohExecuted bool
	if config.PreferDoH {
		log.Infof("start(prefer) to use doh to lookup %s", host)
		startDoH()
		dohExecuted = true
		if config.count > 0 {
			return nil
		}
	}

	// handle specific dns servers
	if len(config.SpecificDNSServers) > 0 {
		swg := utils.NewSizedWaitGroup(5)
		for _, customServer := range config.SpecificDNSServers {
			customServer := customServer
			swg.Add()
			go func() {
				defer swg.Done()
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("dns server %s panic: %v", customServer, err)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				err := _exec(customServer, host, config)
				if err != nil {
					log.Debugf("dns server %s lookup failed: %v", customServer, err)
				}
			}()
		}
		swg.Wait()
	} else {
		log.Infof("no user custom specific dns servers found for: %v", host)
	}

	if config.FallbackDoH && config.count <= 0 && !dohExecuted {
		log.Infof("start(fallback) to use doh to lookup %s", host)
		startDoH()
	}

	return nil
}

func LookupAll(host string, opt ...DNSOption) []string {
	var results []string
	opt = append(opt, WithDNSCallback(func(dnsType, domain, ip, fromServer, method string) {
		if ip == "" {
			return
		}
		results = append(results, ip)
	}))
	err := reliableLookupHost(host, opt...)
	if err != nil {
		log.Errorf("reliable lookup host %s failed: %v", host, err)
	}
	return results
}

func LookupCallback(host string, h func(dnsType, domain, ip, fromServer string, method string), opt ...DNSOption) error {
	opt = append(opt, WithDNSCallback(func(dnsType, domain, ip, fromServer, method string) {
		h(dnsType, domain, ip, fromServer, method)
	}))
	return reliableLookupHost(host, opt...)
}

func LookupFirst(host string, opt ...DNSOption) string {
	start := time.Now()
	defer func() {
		log.Debugf("lookup first %s cost %s", host, time.Since(start))
	}()

	var firstResult string
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	opt = append(opt, WithDNSCallback(func(dnsType, domain, ip, fromServer, method string) {
		if ip == "" {
			return
		}

		if firstResult == "" {
			firstResult = ip
			cancel()
		}
	}), WithDNSContext(ctx))
	go func() {
		defer cancel()
		err := reliableLookupHost(host, opt...)
		if err != nil {
			log.Errorf("reliable lookup host %s failed: %v", host, err)
		}
	}()
	select {
	case <-ctx.Done():
	}
	return firstResult
}
