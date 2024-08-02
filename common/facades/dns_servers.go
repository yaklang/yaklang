package facades

import (
	"context"
	"github.com/miekg/dns"
	"github.com/yaklang/yaklang/common/domainextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"strings"
	"sync"
	"time"
)

func fqdn(r string) string {
	return dns.Fqdn(r)
}

type DNSServer struct {
	// ns1.[domain]
	//ns1Domain string
	//ns2Domain string

	// smtp mail.
	//mxDomain string

	// .domain for A/AAAA
	// dotDomain string
	ipAddr net.IP

	// txt
	txtRecordHandler func() []string

	// time to line
	ttl uint64

	// coreServer for conn
	udpCoreServer *dns.Server
	tcpCoreServer *dns.Server

	hijackCallback func(t string, domain string) string
	callback       FacadeCallback
	addrConvertor  func(i string) string
}

func (d *DNSServer) SetCallback(f FacadeCallback) {
	d.callback = f
}

func (d *DNSServer) SetAddrConvertor(i func(string) string) {
	d.addrConvertor = i
}

func NewDNSServer(domain, dnsLogIP, serveIPRaw string, port int) (*DNSServer, error) {
	ipAddr := net.ParseIP(utils.FixForParseIP(dnsLogIP))
	if ipAddr == nil {
		return nil, utils.Errorf("parsed ip[%v] failed", dnsLogIP)
	}

	serveIP := net.ParseIP(utils.FixForParseIP(serveIPRaw))
	if serveIP == nil {
		return nil, utils.Errorf("parsed listen/served ip[%v] failed", dnsLogIP)
	}

	domain = dns.Fqdn(domain)
	ins := &DNSServer{
		// mxDomain:  fmt.Sprintf("mail.%v", domain),
		// dotDomain: fmt.Sprintf(".%v", domain),
		ipAddr: ipAddr,
		ttl:    3600,
	}

	addr := utils.HostPort(serveIP.String(), port)
	ins.udpCoreServer = &dns.Server{
		Addr:    addr,
		Net:     "udp",
		Handler: ins,
	}
	ins.tcpCoreServer = &dns.Server{
		Addr:    addr,
		Net:     "tcp",
		Handler: ins,
	}
	return ins, nil
}

func (d *DNSServer) handleQuestion(question dns.Question, w dns.ResponseWriter, r *dns.Msg) {

	rootDomain := domainextractor.ExtractRootDomain(question.Name)
	dotDomain := fqdn(rootDomain)
	if !strings.HasPrefix(dotDomain, ".") {
		dotDomain = "." + dotDomain
	}

	visitorLog := NewVisitorLog("dns")
	visitorLog.Set("remote-addr", w.RemoteAddr())
	visitorLog.SetTimestampNow()
	visitorLog.Set("external-ip", d.ipAddr.String())
	visitorLog.Set("root-domain", dotDomain)
	if d.addrConvertor != nil {
		visitorLog.SetRemoteIP(d.addrConvertor(w.RemoteAddr().String()))
	} else {
		visitorLog.SetRemoteIP(w.RemoteAddr().String())
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	requestMsg := r.String()
	//log.Infof("NEW DNS Req: %v", requestMsg)
	visitorLog.Set("raw", requestMsg)
	domain := m.Question[0].Name
	visitorLog.SetDomain(domain)
	log.Infof("dns req for: %s [%v]", domain, dotDomain)
	if strings.HasSuffix(strings.ToLower(domain), strings.ToLower(dotDomain)) {
		payload := domain[:len(domain)-len(dotDomain)]
		visitorLog.Set("payload", payload)
		if index := strings.LastIndex(payload, "."); index > 0 {
			token := payload[index:]
			token = strings.Trim(token, ".")
			visitorLog.Set("token", strings.ToLower(token))
			log.Infof("dnslog set token: %v", token)
		} else {
			visitorLog.Set("token", strings.ToLower(payload))
			log.Infof("dnslog set(payload) token: %v", payload)
		}
	} else {
		log.Warnf("no target domain: %v", domain)
	}

	var uniqueID, fullID string
	ttl := uint32(d.ttl)
	_ = uniqueID
	_ = fullID

	switch r.Question[0].Qtype {
	case dns.TypeTXT:
		visitorLog.SetDNSType("TXT")
		var txts []string
		if d.hijackCallback != nil {
			txts = []string{d.hijackCallback("TXT", domain)}
		}
		if d.txtRecordHandler != nil {
			txts = d.txtRecordHandler()
		}
		m.Answer = append(m.Answer, &dns.TXT{Hdr: dns.RR_Header{Name: dns.Fqdn(domain), Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: ttl}, Txt: txts})
	case dns.TypeANY:
		visitorLog.SetDNSType("ANY")
		fallthrough
	case dns.TypeA:
		log.Infof("recv A record from %v", w.RemoteAddr())
		visitorLog.SetDNSType("A")
		if d.hijackCallback != nil {
			addr := net.ParseIP(utils.FixForParseIP(d.hijackCallback("A", domain)))
			if addr != nil {
				m.Answer = append(m.Answer, &dns.A{Hdr: dns.RR_Header{Name: dns.Fqdn(domain), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: addr})
			}
		} else {
			//nsHeader := dns.RR_Header{Name: dns.Fqdn(domain), Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: ttl}
			handleCloud := func(ipAddress net.IP) {
				m.Answer = append(m.Answer, &dns.A{Hdr: dns.RR_Header{Name: dns.Fqdn(domain), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: ipAddress})

				//m.Ns = append(m.Ns, &dns.NS{Hdr: nsHeader, Ns: d.ns1Domain})
				//m.Ns = append(m.Ns, &dns.NS{Hdr: nsHeader, Ns: d.ns2Domain})
				//m.Extra = append(m.Extra, &dns.A{Hdr: dns.RR_Header{Name: d.ns1Domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: d.ipAddr})
				//m.Extra = append(m.Extra, &dns.A{Hdr: dns.RR_Header{Name: d.ns2Domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: d.ipAddr})
			}
			handleAppWithCname := func(cname string, ips ...net.IP) {
				fqdnCname := dns.Fqdn(cname)
				m.Answer = append(m.Answer, &dns.CNAME{Hdr: dns.RR_Header{Name: dns.Fqdn(domain), Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: ttl}, Target: fqdnCname})
				for _, ip := range ips {
					m.Answer = append(m.Answer, &dns.A{Hdr: dns.RR_Header{Name: fqdnCname, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: ip})
				}

				//m.Ns = append(m.Ns, &dns.NS{Hdr: nsHeader, Ns: d.ns1Domain})
				//m.Ns = append(m.Ns, &dns.NS{Hdr: nsHeader, Ns: d.ns2Domain})
				//m.Extra = append(m.Extra, &dns.A{Hdr: dns.RR_Header{Name: d.ns1Domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: d.ipAddr})
				//m.Extra = append(m.Extra, &dns.A{Hdr: dns.RR_Header{Name: d.ns2Domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: d.ipAddr})
			}
			_ = handleAppWithCname

			switch {
			case strings.EqualFold(domain, "aws"+dotDomain):
				handleCloud(net.ParseIP("169.254.169.254"))
			case strings.EqualFold(domain, "alibaba"+dotDomain):
				handleCloud(net.ParseIP("100.100.100.200"))
			//case strings.EqualFold(domain, "app"+h.dotDomain):
			//	handleAppWithCname("projectdiscovery.github.io", net.ParseIP("185.199.108.153"), net.ParseIP("185.199.110.153"), net.ParseIP("185.199.111.153"), net.ParseIP("185.199.108.153"))
			default:
				handleCloud(d.ipAddr)
			}
		}
	//case dns.TypeSOA:
	//	visitorLog.SetDNSType("SOA")
	//	hostmaster := "admin" + d.dotDomain
	//	m.Answer = append(m.Answer, &dns.SOA{Hdr: dns.RR_Header{Name: dns.Fqdn(domain), Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: ttl}, Ns: d.ns1Domain, Mbox: hostmaster})
	//case dns.TypeMX:
	//	if d.hijackCallback != nil {
	//		mx := d.hijackCallback("MX", domain)
	//		if mx != "" {
	//			m.Answer = append(m.Answer, &dns.MX{Hdr: dns.RR_Header{Name: fqdn(domain), Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: ttl}, Mx: mx, Preference: 1})
	//		}
	//	} else {
	//		visitorLog.SetDNSType("MX")
	//		m.Answer = append(m.Answer, &dns.MX{Hdr: dns.RR_Header{Name: fqdn(domain), Rrtype: dns.TypeMX, Class: dns.ClassINET, Ttl: ttl}, Mx: d.mxDomain, Preference: 1})
	//	}
	case dns.TypeNS:
		visitorLog.SetDNSType("NS")
		if d.hijackCallback != nil {
			ns := d.hijackCallback("NS", domain)
			if ns != "" {
				m.Answer = append(m.Answer, &dns.NS{Hdr: dns.RR_Header{Name: fqdn(domain), Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: ttl}, Ns: ns})
			}
		}
		//nsHeader := dns.RR_Header{Name: dns.Fqdn(domain), Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: ttl}
		//m.Ns = append(m.Ns, &dns.NS{Hdr: nsHeader, Ns: d.ns1Domain})
		//m.Ns = append(m.Ns, &dns.NS{Hdr: nsHeader, Ns: d.ns2Domain})
	}

	if d.callback != nil {
		d.callback(visitorLog)
	}

	// 返回给用户
	if err := w.WriteMsg(m); err != nil {
		log.Errorf("Could not write DNS response: %s", err)
	}

	//responseMsg := m.String()
	//_ = responseMsg
	//println(responseMsg)

	// 保存所有记录，先不管
	// if root-tld is enabled stores any interaction towards the main domain
	//if d.options.RootTLD && strings.HasSuffix(domain, d.dotDomain) {
	//	correlationID := h.options.Domain
	//	host, _, _ := net.SplitHostPort(w.RemoteAddr().String())
	//	interaction := &Interaction{
	//		Protocol:      "dns",
	//		UniqueID:      domain,
	//		FullId:        domain,
	//		QType:         toQType(r.Question[0].Qtype),
	//		RawRequest:    requestMsg,
	//		RawResponse:   responseMsg,
	//		RemoteAddress: host,
	//		Timestamp:     time.Now(),
	//	}
	//	buffer := &bytes.Buffer{}
	//	if err := jsoniter.NewEncoder(buffer).Encode(interaction); err != nil {
	//		gologger.Warning().Msgf("Could not encode root tld dns interaction: %s\n", err)
	//	} else {
	//		gologger.Debug().Msgf("Root TLD DNS Interaction: \n%s\n", buffer.String())
	//		if err := h.options.Storage.AddInteractionWithId(correlationID, buffer.Bytes()); err != nil {
	//			gologger.Warning().Msgf("Could not store dns interaction: %s\n", err)
	//		}
	//	}
	//}

	if strings.HasSuffix(domain, dotDomain) {
		fullID = strings.ReplaceAll(domain, dotDomain, "")
		//parts := strings.Split(domain, ".")
		//for i, part := range parts {
		//	if len(part) == 33 {
		//		uniqueID = part
		//		fullID = part
		//		if i+1 <= len(parts) {
		//			fullID = strings.Join(parts[:i+1], ".")
		//		}
		//	}
		//}
	}

	//if uniqueID != "" {
	//	correlationID := uniqueID[:20]
	//	host, _, _ := net.SplitHostPort(w.RemoteAddr().String())
	//	interaction := &Interaction{
	//		Protocol:      "dns",
	//		UniqueID:      uniqueID,
	//		FullId:        fullID,
	//		QType:         toQType(r.Question[0].Qtype),
	//		RawRequest:    requestMsg,
	//		RawResponse:   responseMsg,
	//		RemoteAddress: host,
	//		Timestamp:     time.Now(),
	//	}
	//	buffer := &bytes.Buffer{}
	//	if err := jsoniter.NewEncoder(buffer).Encode(interaction); err != nil {
	//		gologger.Warning().Msgf("Could not encode dns interaction: %s\n", err)
	//	} else {
	//		gologger.Debug().Msgf("DNS Interaction: \n%s\n", buffer.String())
	//		if err := h.options.Storage.AddInteraction(correlationID, buffer.Bytes()); err != nil {
	//			gologger.Warning().Msgf("Could not store dns interaction: %s\n", err)
	//		}
	//	}
	//}
}

func (d *DNSServer) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("panic serve dns: %s", err)
		}
	}()

	// bail early for no queries.
	if len(r.Question) == 0 {
		return
	}
	for _, q := range r.Question {
		d.handleQuestion(q, w, r)
	}
}

func (d *DNSServer) Serve(ctx context.Context) error {
	go func() {
		select {
		case <-ctx.Done():
		}

		if d.udpCoreServer != nil {
			go d.udpCoreServer.Shutdown()
		}

		if d.tcpCoreServer != nil {
			go d.tcpCoreServer.Shutdown()
		}
	}()
	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer wg.Done()

		if d.tcpCoreServer == nil {
			return
		}
		for {
			log.Infof("enable tcp dnslog server: %v", d.tcpCoreServer.Addr)
			err := d.tcpCoreServer.ListenAndServe()
			if err != nil {
				log.Errorf("error failed (tcp dnslog server): %s", err)
			}

			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)
			}
		}
	}()
	go func() {
		defer wg.Done()

		if d.udpCoreServer == nil {
			return
		}

		for {
			log.Infof("enable udp dnslog server: %v", d.udpCoreServer.Addr)
			err := d.udpCoreServer.ListenAndServe()
			if err != nil {
				log.Errorf("error failed (tcp dnslog server): %s", err)
			}
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)
			}
		}
	}()

	wg.Wait()
	return nil
}
