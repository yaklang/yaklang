package yakdns

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
)

/*
Resolver is a DNS resolver.

It is used to resolve a domain name to an IP address.
*/

var (
	defaultResolver = &net.Resolver{}
	pureGoResolver  = &net.Resolver{PreferGo: true}
)

func _execDefault(host string, config *ReliableDNSConfig) []string {
	defer func() {
		if err := recover(); err != nil {
			log.Debugf("net.Resolver panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	ctx, cancel := context.WithTimeout(config.GetBaseContext(), config.Timeout)
	defer cancel()
	result, err := defaultResolver.LookupHost(ctx, host)
	if err != nil {
		log.Debugf("default dns resolver lookup failed: %v", err)
	}
	if len(result) > 0 {
		return result
	}
	return nil
}

func _execPureGoDefault(host string, config *ReliableDNSConfig) []string {
	defer func() {
		if err := recover(); err != nil {
			log.Debugf("net.Resolver panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	ctx, cancel := context.WithTimeout(config.GetBaseContext(), config.Timeout)
	defer cancel()
	result, err := pureGoResolver.LookupHost(ctx, host)
	if err != nil {
		log.Debugf("default dns resolver lookup failed: %v", err)
	}
	if len(result) > 0 {
		return result
	}
	return nil
}

// nativeLookupHost looks up the given host using the local resolver.
// It returns a slice of that host's addresses.
func nativeLookupHost(host string, config *ReliableDNSConfig) {
	result := _execDefault(host, config)
	if len(result) > 0 {
		for _, ip := range result {
			config.call("", host, ip, "system-default", "system-default")
		}
		return
	}
	for _, ip := range _execPureGoDefault(host, config) {
		config.call("", host, ip, "system-prefergo", "system-prefergo")
	}
}
