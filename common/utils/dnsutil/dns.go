package dnsutil

import (
	"context"
	"fmt"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakdns"
	"strings"
	"time"
)

func qualifyDomain(domain string) string {
	return fmt.Sprintf("%s.", formatDomain(domain))
}

func formatDomain(target string) string {
	for strings.HasPrefix(target, ".") {
		target = target[1:]
	}
	return target
}

func QueryNS(target string, timeout time.Duration, nameServers []string) []string {
	if nameServers == nil {
		nameServers = utils.DefaultDNSServer
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	servers, err := QueryNSEx(&utils.DefaultDNSClient, ctx, target, nameServers)
	if err != nil {
		log.Error(err.Error())
		return nil
	}
	return servers
}

func QueryNSEx(client *dns.Client, ctx context.Context, target string, servers []string) ([]string, error) {
	queryNS := &dns.Msg{}

	target = qualifyDomain(target)

	ctx, _ = context.WithTimeout(ctx, 10*time.Second)
	queryNS.SetQuestion(target, dns.TypeNS)

	var results []string
	for _, server := range servers {
		server = utils.ToNsServer(server)

		rsp, _, err := client.ExchangeContext(ctx, queryNS, server)
		if err != nil {
			continue
		}

		for _, r := range rsp.Answer {
			switch record := r.(type) {
			case *dns.NS:
				results = append(results, record.Ns)
			}
		}
		return results, nil
	}
	return nil, errors.Errorf("cannot query ns record for %s", target)
}

func QueryIPAll(target string, timeout time.Duration, dnsServers []string) []string {
	return yakdns.LookupAll(target, yakdns.WithTimeout(timeout), yakdns.WithDNSServers(dnsServers...))
}

func QueryIP(target string, timeout time.Duration, dnsServers []string) string {
	return yakdns.LookupFirst(target, yakdns.WithTimeout(timeout), yakdns.WithDNSServers(dnsServers...))
}

func QueryTxt(target string, timeout time.Duration, nameServers []string) []string {
	if nameServers == nil {
		nameServers = utils.DefaultDNSServer
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	servers, err := QueryTxTEx(&utils.DefaultDNSClient, ctx, target, nameServers)
	if err != nil {
		log.Error(err.Error())
		return nil
	}
	return servers
}

func QueryTxTEx(client *dns.Client, ctx context.Context, target string, servers []string) ([]string, error) {
	queryTxt := &dns.Msg{}

	target = qualifyDomain(target)
	ctx, _ = context.WithTimeout(ctx, 10*time.Second)
	queryTxt.SetQuestion(target, dns.TypeTXT)
	var results []string
	for _, server := range servers {
		server = utils.ToNsServer(server)

		rsp, _, err := client.ExchangeContext(ctx, queryTxt, server)
		if err != nil {
			continue
		}

		for _, r := range rsp.Answer {
			switch record := r.(type) {
			case *dns.TXT:
				results = append(results, record.Txt...)
			}
		}
		return results, nil
	}
	return nil, errors.Errorf("cannot query TXT record for %s", target)
}

func QueryAXFR(target string, timeout time.Duration, nameServers []string) []string {
	if nameServers == nil {
		nameServers = utils.DefaultDNSServer
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	servers, err := QueryAXFREx(ctx, target, nameServers)
	if err != nil {
		log.Error(err.Error())
		return nil
	}
	return servers
}

func QueryAXFREx(ctx context.Context, target string, servers []string) ([]string, error) {
	queryAxfr := &dns.Msg{}

	target = qualifyDomain(target)
	ctx, _ = context.WithTimeout(ctx, 10*time.Second)
	queryAxfr.SetAxfr(target)
	var results []string
	for _, server := range servers {
		server = utils.ToNsServer(server)
		cn, err := utils.DefaultDNSConn("tcp", server)
		if err != nil {
			continue
		}
		//rsp, _, err := client.ExchangeContext(ctx, queryAxfr, server)
		err = cn.WriteMsg(queryAxfr)
		//r, err := cn.ReadMsg()
		if err != nil {
			continue
		}
		data, err := cn.ReadMsg()
		for _, r := range data.Answer {
			results = append(results, r.String())

		}
		return results, nil
	}
	return nil, errors.Errorf("cannot query Axfr record for %s", target)
}
