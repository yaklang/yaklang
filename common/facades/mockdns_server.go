package facades

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

func MockDNSServerDefault(domain string, h func(record string, domain string) string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	_ = cancel
	port := utils.GetRandomAvailableTCPPort()
	return MockDNSServer(ctx, domain, port, h)
}

func MockDNSServer(ctx context.Context, rootDomain string, port int, h func(record string, domain string) string) string {
	if rootDomain == "" {
		rootDomain = "example.com"
	}
	server, err := NewDNSServer(rootDomain, "127.0.0.1", "127.0.0.1", port)
	if err != nil {
		panic(err)
	}
	server.hijackCallback = h
	go func() {
		err := server.Serve(ctx)
		if err != nil {
			log.Errorf("DNS Server Error: %s", err.Error())
		}
	}()
	err = utils.WaitConnect(utils.HostPort("127.0.0.1", port), 10)
	if err != nil {
		log.Errorf("connect dns server failed: %s", err)
		return ""
	}
	if port == 53 {
		return "127.0.0.1"
	}
	return utils.HostPort("127.0.0.1", port)
}
