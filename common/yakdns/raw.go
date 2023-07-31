package yakdns

import (
	"context"
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/miekg/dns"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"net"
	"net/http"
	"net/url"
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

func _exec(server string, domain string, config *ReliableDNSConfig) error {
	if config.RetryTimes <= 0 {
		config.RetryTimes = 1
	}

	for i := 0; i < config.RetryTimes; i++ {
		err := _execWithoutRetry(server, domain, config)
		if err != nil {
			log.Warnf("exec dns request failed: %s", err)
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		return nil
	}
	return utils.Errorf("exec dns request failed")
}

func _execWithoutRetry(server string, domain string, config *ReliableDNSConfig) error {
	req, err := dnsRequestBytes(dns.TypeA, domain)
	if err != nil {
		return err
	}

	rootCtx := context.Background()
	udpCtx, udpCancel := context.WithTimeout(rootCtx, config.Timeout)
	defer udpCancel()

	server = utils.AppendDefaultPort(server, 53)
	conn, err := dnsNetDialer.DialContext(udpCtx, "udp", server)
	if err != nil {
		return err
	}
	conn.Write(req)
	var buf = make([]byte, 512)
	conn.SetReadDeadline(time.Now().Add(config.Timeout))
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
	if !config.FallbackTCP {
		return nil
	}

	tcpCtx, tcpCancel := context.WithTimeout(rootCtx, config.Timeout)
	defer tcpCancel()
	conn, err = dnsNetDialer.DialContext(tcpCtx, "tcp", server)
	if err != nil {
		log.Errorf("fallback to dial tcp[%v] failed: %s", server, err)
	}
	if conn != nil {
		conn.Write(req)
		conn.SetReadDeadline(time.Now().Add(config.Timeout))

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

type DoHDNSResponse struct {
	Status   int  `json:"Status"`
	TC       bool `json:"TC"`
	RD       bool `json:"RD"`
	RA       bool `json:"RA"`
	AD       bool `json:"AD"`
	CD       bool `json:"CD"`
	Question json.RawMessage
	Answer   []struct {
		Name string `json:"name"`
		Type int    `json:"type"`
		TTL  int    `json:"TTL"`
		Data string `json:"data"`
	} `json:"Answer"`
}

func dohRequest(domain string, dohUrl string, config *ReliableDNSConfig) error {
	log.Debugf("start to request doh: %v to %v", domain, dohUrl)
	var val = make(url.Values)
	val.Set("name", domain)
	val.Set("type", "1")
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()
	reqInstance, err := http.NewRequestWithContext(ctx, "GET", dohUrl, nil)
	if err != nil {
		return utils.Errorf("build doh request failed: %s", err)
	}
	reqInstance.URL.RawQuery = val.Encode()
	reqInstance.Header.Set("Accept", "application/dns-json")
	rspInstance, err := config.dohHTTPClient.Do(reqInstance)
	if err != nil {
		return utils.Errorf("doh request failed: %s", err)
	}
	body, _ := io.ReadAll(rspInstance.Body)
	var rspObj DoHDNSResponse
	err = json.Unmarshal(body, &rspObj)
	if err != nil {
		println(string(body))
		log.Errorf("unmarshal doh response failed: %s", err)
	}
	for _, a := range rspObj.Answer {
		config.call("A", domain, a.Data, dohUrl, "yakdns.doh", a.TTL)
	}
	return nil
}
