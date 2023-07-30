package yakdns

import (
	"github.com/ReneKroon/ttlcache"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"sync"
)

var v6Cache = ttlcache.NewCache()
var cache = ttlcache.NewCache()

func LookupFirstIP()

func reliableLookupHost(host string, opt ...func(*ReliableDialConfig)) error {
	var config = NewDefaultReliableDialConfig()
	for _, o := range opt {
		o(config)
	}

	// handle hosts
	result, ok := GetHost(host)
	if ok && result == "" {
		config.call("", host, result, "hosts", "hosts")
		return nil
	}

	// ttlcache v4 > v6
	cachedResult, ok := cache.Get(host)
	if ok {
		result, ok := cachedResult.(string)
		if ok {
			config.call("", host, result, "cache", "cache")
			return nil
		}
	}
	cachedResult, ok = v6Cache.Get(host)
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

	// handle specific dns servers
	if len(config.SpecificDNSServers) <= 0 {
		return utils.Error("no dns server specified, disable system")
	}

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
	return nil
}
