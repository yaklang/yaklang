package yakdns

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/miekg/dns"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"net"
	"time"
)

func dnsRequestBytes(queryType uint16, domain string) ([]byte, error) {
	var msg = new(dns.Msg)
	fqdnDomain := dns.Fqdn(domain)
	msg.SetQuestion(fqdnDomain, queryType)
	bytes, err := msg.Pack()
	if err != nil {
		return nil, utils.Errorf("build dns packet failed: %s", err)
	}
	return bytes, nil
}

func extractIP(raw []byte) ([]string, error) {
	var msg = new(dns.Msg)
	err := msg.Unpack(raw)
	if err != nil {
		return nil, utils.Errorf("unpack dns packet failed: %s", err)
	}
	var ips []string
	for _, answer := range msg.Answer {
		if a, ok := answer.(*dns.A); ok {
			ips = append(ips, a.A.String())
		} else if a, ok := answer.(*dns.AAAA); ok {
			ips = append(ips, a.AAAA.String())
		} else if a, ok := answer.(*dns.NS); ok {
			log.Errorf("ns record found: %s", spew.Sdump(a))
		}
	}
	if len(ips) <= 0 {
		return nil, utils.Errorf("no ip found in dns packet")
	}
	return ips, nil
}

var dnsNetDialer = &net.Dialer{
	Timeout: 5,
}

func _exec(server string, domain string, config *ReliableDialConfig) error {
	req, err := dnsRequestBytes(dns.TypeA, domain)
	if err != nil {
		return err
	}

	rootCtx := context.Background()
	udpCtx, udpCancel := context.WithTimeout(rootCtx, 5*time.Second)
	defer udpCancel()

	server = utils.AppendDefaultPort(server, 53)
	conn, err := dnsNetDialer.DialContext(udpCtx, "udp", server)
	if err != nil {
		return err
	}
	conn.Write(req)
	var buf = make([]byte, 512)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		log.Warnf("read dns response failed: %s", err)
	}
	if n > 0 {
		ips, err := extractIP(buf[:n])
		if err != nil {
			log.Warnf("extract ip from dns response failed: %s", err)
		} else {
			for _, i := range ips {
				config.call("", domain, i, server, "yakdns.udp")
			}
			return nil
		}
	}

	log.Warnf("no ip found in udp dns response for %v, start to check tcp", domain)
	if config.NoFallbackTCP {
		return nil
	}

	tcpCtx, tcpCancel := context.WithTimeout(rootCtx, 5*time.Second)
	defer tcpCancel()
	conn, err = dnsNetDialer.DialContext(tcpCtx, "tcp", server)
	if err != nil {
		log.Errorf("fallback to dial tcp[%v] failed: %s", server, err)
	}
	if conn != nil {
		conn.Write(req)
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))

		buf = make([]byte, 512)
		n, err = conn.Read(buf)
		if err != nil && err != io.EOF {
			log.Warnf("read dns response failed: %s", err)
		}
		if n > 0 {
			ips, err := extractIP(buf[:n])
			if err != nil {
				log.Warnf("extract ip from dns response failed: %s", err)
			} else {
				for _, i := range ips {
					config.call("", domain, i, server, "yakdns.tcp")
				}
				return nil
			}
		}
	}
	return nil
}

func dohRequest(domain string) ([]string, error) {
	packet, err := dnsRequestBytes(dns.TypeA, domain)
	if err != nil {
		return nil, err
	}
	rsp, err := lowhttp.HTTP(
		lowhttp.WithPacketBytes(lowhttp.ReplaceHTTPPacketQueryParam([]byte(`GET /dns-query HTTP/1.1
Host: 1.1.1.1
User-Agent: go-http-client/1.1
Accept: application/dns-message

`), "dns", codec.EncodeBase64Url(packet))),
	)
	if err != nil {
		panic(err)
	}
	spew.Dump(rsp.RawPacket)
	return nil, utils.Errorf("not implemented")
}
