package subdomain

import (
	"context"
	"fmt"
	"github.com/miekg/dns"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dnsutil"
	"strings"
	"time"
)

var queryNs = dnsutil.QueryNSEx

func (s *SubdomainScanner) ZoneTransfer(ctx context.Context, target string) {
	queryNsCtx, _ := context.WithTimeout(ctx, 10*time.Second)

	ns, err := queryNs(s.dnsClient, queryNsCtx, target, s.config.DNSServers)
	if err != nil {
		log.Errorf("query nameserver failed: %s", err)
		return
	}

	ns = append(ns, s.config.DNSServers...)
	log.Infof("start to send axfr request to ns: %s", strings.Join(ns, " | "))
	for _, nameserver := range ns {
		log.Infof("start to checking axfr from %s", nameserver)
		req := &dns.Msg{}
		req.SetAxfr(dns.Fqdn(target))

		// 这里有点复杂 AXFR
		// https://github.com/OWASP/Amass/blob/9ccc0c034eafca74a621ac6850d130f1faad5fa7/resolvers/zone.go#L64
		conn, err := netx.DialTCPTimeout(10*time.Second, "tcp", utils.ToNsServer(nameserver))
		if err != nil {
			log.Infof("failed to setup TCP connection with the dns server: %s: %s", utils.ToNsServer(nameserver), err)
			continue
		}
		defer func() {
			_ = conn.Close()
		}()

		xfr := &dns.Transfer{Conn: &dns.Conn{Conn: conn}, ReadTimeout: 10 * time.Second}

		ens, err := xfr.In(req, "")
		if err != nil {
			log.Errorf("failed to xfr %s", err)
			continue
		}

		for en := range ens {
			for _, r := range en.RR {
				switch record := r.(type) {
				case *dns.A:
					s.onResult(&SubdomainResult{
						FromTarget:    target,
						FromDNSServer: nameserver,
						FromModeRaw:   ZONE_TRANSFER,
						IP:            record.A.String(),
						Domain:        record.Hdr.Name,
						Tags:          []string{fmt.Sprintf("axfr-from-%s", nameserver)},
					})
				}
			}
		}
	}
}
