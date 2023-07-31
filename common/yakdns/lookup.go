package yakdns

import (
	"context"
	"github.com/ReneKroon/ttlcache"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"sync"
)

var ipv6DNSCache = ttlcache.NewCache()
var ipv4DNSCache = ttlcache.NewCache()

func reliableLookupHost(host string, opt ...DNSOption) error {
	var config = NewDefaultReliableDNSConfig()
	for _, o := range opt {
		o(config)
	}

	// handle hosts
	result, ok := GetHost(host)
	if ok && result != "" {
		config.call("", host, result, "hosts", "hosts")
		return nil
	}

	// ttlcache v4 > v6
	cachedResult, ok := ipv4DNSCache.Get(host)
	if ok {
		result, ok := cachedResult.(string)
		if ok {
			config.call("", host, result, "cache", "cache")
			return nil
		}
	}
	cachedResult, ok = ipv6DNSCache.Get(host)
	if ok {
		result, ok := cachedResult.(string)
		if ok {
			config.call("", host, result, "cache", "cache")
			return nil
		}
	}

	// handle system resolver
	if !config.DisableSystemResolver {
		var results = nativeLookupHost(utils.TimeoutContextSeconds(5), host)
		if len(results) > 0 {
			var noContinueExec bool
			for _, result := range results {
				result := strings.TrimSpace(result)
				if result == "" {
					continue
				}
				noContinueExec = true
				config.call("", host, result, "system", "system")
			}
			if noContinueExec {
				return nil
			}
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
		wg := new(sync.WaitGroup)
		for _, customServer := range config.SpecificDNSServers {
			customServer := customServer
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("dns server %s panic: %v", customServer, err)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				err := _exec(customServer, host, config)
				if err != nil {
					log.Errorf("dns server %s lookup failed: %v", customServer, err)
				}
			}()
		}
		wg.Wait()
	} else {
		log.Warn("no user custom specific dns servers found")
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

func LookupFirst(host string, opt ...DNSOption) string {
	result := make(chan string)
	opt = append(opt, WithDNSCallback(func(dnsType, domain, ip, fromServer, method string) {
		if ip == "" {
			return
		}
		select {
		case result <- ip:
		default:
		}
	}))
	go func() {
		err := reliableLookupHost(host, opt...)
		if err != nil {
			log.Errorf("reliable lookup host %s failed: %v", host, err)
		}
		select {
		case result <- "":
		}
	}()

	select {
	case resultStr := <-result:
		defer close(result)
		return resultStr
	}
}
