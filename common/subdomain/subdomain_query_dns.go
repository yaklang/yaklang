package subdomain

import (
	"context"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
	"strings"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func queryDNS(domain string, servers []string, ctx context.Context, timeout time.Duration, client *dns.Client, qType uint16) (_ *dns.Msg, _ time.Duration, server string, _ error) {
	if !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}

	if client == nil {
		ctx, _ = context.WithTimeout(ctx, timeout)
		client = &dns.Client{}
	}

	msg := &dns.Msg{}
	msg.SetQuestion(domain, qType)

	for _, server := range servers {
		server = utils.ToNsServer(server)

		rsp, ttl, err := client.ExchangeContext(ctx, msg, server)
		if err != nil {
			log.Debugf("query [%s] failed from [%s]: %s", domain, server, err)
			continue
		}
		return rsp, ttl, server, nil
	}
	return nil, 0, "", errors.Errorf("no record found for %s from [%s]", domain, strings.Join(servers, "|"))
}

func (s *SubdomainScanner) QueryA(ctx context.Context, domain string) (ip string, server string, _ error) {
	msg, _, server, err := queryDNS(domain, s.config.DNSServers, ctx, 3*time.Second, s.dnsClient, dns.TypeA)
	if err != nil {
		return "", "", errors.Errorf("query dns A failed: %s", msg)
	}

	for _, r := range msg.Answer {
		switch record := r.(type) {
		case *dns.A:
			if record.A.String() != "" {
				return record.A.String(), server, nil
			}
		}
	}
	return "", "", errors.Errorf("no A record for %s", domain)
}

func (s *SubdomainScanner) QueryAAAA(ctx context.Context, domain string) (ip6 string, server string, _ error) {
	msg, _, server, err := queryDNS(domain, s.config.DNSServers, ctx, 3*time.Second, s.dnsClient, dns.TypeAAAA)
	if err != nil {
		return "", "", errors.Errorf("query dns A failed: %s", msg)
	}

	for _, r := range msg.Answer {
		switch record := r.(type) {
		case *dns.AAAA:
			if record.AAAA.String() != "" {
				return record.AAAA.String(), server, nil
			}
		}
	}
	return "", "", errors.Errorf("no A record for %s", domain)
}
