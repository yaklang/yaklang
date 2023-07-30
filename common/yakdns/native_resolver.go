package yakdns

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"sync"
)

/*
Resolver is a DNS resolver.

It is used to resolve a domain name to an IP address.
*/

var (
	defaultResolver = &net.Resolver{}
	pureGoResolver  = &net.Resolver{PreferGo: true}
)

// nativeLookupHost looks up the given host using the local resolver.
// It returns a slice of that host's addresses.
func nativeLookupHost(ctx context.Context, host string) []string {
	wg := new(sync.WaitGroup)
	wg.Add(2)
	ctx, cancel := context.WithCancel(ctx)
	var results = make(chan []string, 3)
	defer close(results)
	go func() {
		defer wg.Done()
		defer func() {
			cancel()
			if err := recover(); err != nil {
				log.Debugf("net.Resolver panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		result, err := defaultResolver.LookupHost(ctx, host)
		if err != nil {
			log.Debugf("default dns resolver lookup failed: %v", err)
			return
		}
		results <- result
	}()
	go func() {
		defer wg.Done()
		defer func() {
			cancel()
			if err := recover(); err != nil {
				log.Errorf("net.Resolver panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		result, err := pureGoResolver.LookupHost(ctx, host)
		if err != nil {
			log.Debugf("pure go dns resolver lookup failed: %v", err)
			return
		}
		results <- result
	}()
	wg.Wait()
	results <- nil
	select {
	case ret := <-results:
		return ret
	}
}
