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

func MockTCPDNSServerDefault(domain string, h func(record string, domain string) string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	_ = cancel
	port := utils.GetRandomAvailableTCPPort()
	return MockDNSServerEx(ctx, false, true, domain, port, h)
}

func MockDNSServer(ctx context.Context, rootDomain string, port int, h func(record string, domain string) string) string {
	return MockDNSServerEx(ctx, false, false, rootDomain, port, h)
}

func MockDNSServerEx(ctx context.Context, noTcp, noUdp bool, rootDomain string, port int, h func(record string, domain string) string) string {
	if rootDomain == "" {
		rootDomain = "example.com"
	}

	if noTcp && noUdp {
		panic("NO NETWORK SET(noTcp && noUdp)")
	}

	server, err := NewDNSServer(rootDomain, "127.0.0.1", "127.0.0.1", port)
	if err != nil {
		panic(err)
	}
	if noUdp {
		server.udpCoreServer = nil
	}

	if noTcp {
		server.tcpCoreServer = nil
	}

	server.hijackCallback = h
	go func() {
		log.Infof("start to serve mock dns server: %s", utils.HostPort("127.0.0.1", port))
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
