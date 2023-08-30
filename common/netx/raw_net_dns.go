package netx

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/miekg/dns"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

func extractIP(raw []byte, fromTCP bool) ([]string, error) {
	if fromTCP {
		if len(raw) > 2 {
			l := utils.NetworkByteOrderBytesToUint16(raw)
			if l > 0 {
				raw = raw[2:]
				if len(raw) >= int(l) {
					raw = raw[:l]
				} else {
					return nil, utils.Errorf("invalid tcp dns packet")
				}
			} else {
				return nil, utils.Errorf("invalid tcp dns packet")
			}
		}
	}

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
	Timeout: 30 * time.Second,
}

func _exec(server string, domain string, config *ReliableDNSConfig) error {
	if config.RetryTimes <= 0 {
		config.RetryTimes = 1
	}
	start := time.Now()
	defer func() {
		log.Debugf("exec dns request for %v cost: %v", domain, time.Now().Sub(start))
	}()

	for i := 0; i < config.RetryTimes; i++ {
		err := _execWithoutRetry(server, domain, config)
		if err != nil {
			log.Warnf("exec dns request failed: %s", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		return nil
	}
	return utils.Errorf("exec dns request failed")
}

func _execWithoutRetry(server string, domain string, config *ReliableDNSConfig) error {
	execStart := time.Now()
	req, err := dnsRequestBytes(dns.TypeA, domain)
	if err != nil {
		return err
	}
	server = utils.AppendDefaultPort(server, 53)

	rootCtx := config.GetBaseContext()
	udpCtx, udpCancel := context.WithTimeout(rootCtx, config.Timeout)
	defer udpCancel()

	var conn net.Conn
	var connStart time.Time

	tcpDNS := func() {
		connStart = time.Now()
		tcpCtx, tcpCancel := context.WithTimeout(rootCtx, config.Timeout)
		defer tcpCancel()
		conn, err = dnsNetDialer.DialContext(tcpCtx, "tcp", server)
		if err != nil {
			log.Errorf("fallback to dial tcp[%v] failed: %s", server, err)
		}
		if conn != nil {
			log.Debugf("execute dns request via tcp[%v], conn cost: %v", server, time.Now().Sub(connStart))
			start := time.Now()
			conn.Write(utils.NetworkByteOrderUint16ToBytes(uint16(len(req))))
			_, err := conn.Write(req)
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				tcpConn.CloseWrite()
			}
			if err != nil {
				log.Debugf("write tcp dns request failed: %s", err)
			}
			conn.SetReadDeadline(time.Now().Add(config.Timeout))

			buf := make([]byte, 512)
			var n int
			n, err = conn.Read(buf)
			log.Debugf("read tcp dns response[%v] cost:%v", n, time.Now().Sub(start))
			if err != nil && err != io.EOF {
				log.Warnf("read dns response failed: %s", err)
			}
			if n > 0 {
				ips, err := extractIP(buf[:n], true)
				if err != nil {
					log.Warnf("extract ip from dns response failed: %s", err)
					spew.Dump(buf[:n])
				} else {
					for _, i := range ips {
						config.call("", domain, i, server, "yakdns.tcp")
					}
				}
			}
		}
	}

	var tcpExecuted = false
	if config.PreferTCP {
		tcpExecuted = true
		tcpDNS()
		if config.count > 0 {
			return nil
		}
	}

	server = utils.AppendDefaultPort(server, 53)
	log.Debugf("start to dial udp dns server[%v] for %v cost: %v", server, domain, time.Now().Sub(execStart))
	connStart = time.Now()
	conn, err = dnsNetDialer.DialContext(udpCtx, "udp", server)
	if err != nil {
		log.Errorf("dial[%v] udp dns server failed: %s", time.Now().Sub(connStart), err)
	} else if conn != nil {
		start := time.Now()
		conn.Write(req)
		var buf = make([]byte, 512)
		conn.SetReadDeadline(time.Now().Add(config.Timeout))
		n, err := conn.Read(buf)
		log.Debugf("read udp dns response[%v] cost:%v conn-cost: %v", n, time.Now().Sub(start), time.Now().Sub(connStart))
		if err != nil && err != io.EOF {
			log.Warnf("read dns response failed: %s", err)
		}
		if n > 0 {
			ips, err := extractIP(buf[:n], false)
			if err != nil {
				log.Warnf("extract ip from dns response failed: %s", err)
			} else {
				for _, i := range ips {
					config.call("", domain, i, server, "yakdns.udp")
				}
				return nil
			}
		}
	}

	if !config.FallbackTCP {
		if config.count <= 0 {
			return utils.Errorf("not found ip for %#v", domain)
		}
		return nil
	}

	if !tcpExecuted {
		log.Warnf("no ip found in udp dns response for %v, start to check tcp", domain)
		tcpDNS()
	}

	if config.count <= 0 {
		return utils.Errorf("not found ip for %#v", domain)
	}
	return nil
}
