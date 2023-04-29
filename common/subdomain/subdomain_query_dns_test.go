package subdomain

import (
	"fmt"
	"github.com/miekg/dns"
	"math/rand"
	"net"
	"testing"
	"time"
)

type testDomainServerHandler func(writer dns.ResponseWriter, msg *dns.Msg)
type testDomainServer struct {
	handlers []testDomainServerHandler
}

func (t *testDomainServer) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	t.handle(w, r)
}
func (t *testDomainServer) handle(w dns.ResponseWriter, r *dns.Msg) {
	for _, h := range t.handlers {
		h(w, r)
	}
}
func (t *testDomainServer) AddHandler(cb testDomainServerHandler) {
	t.handlers = append(t.handlers, cb)
}

func TestQueryDNS(t *testing.T) {

	rand.Seed(time.Now().UnixNano())

	handler := &testDomainServer{}
	flag := "11.22.33.44"
	handler.AddHandler(func(w dns.ResponseWriter, r *dns.Msg) {
		msg := &dns.Msg{}
		msg.SetReply(r)
		msg.Answer = append(msg.Answer, &dns.A{
			Hdr: dns.RR_Header{
				Name:   "aaa.com.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			A: net.ParseIP(flag),
		})
		w.WriteMsg(msg)
	})

	port := rand.Intn(2000) + 60029
	server := &dns.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Net:     "udp",
		Handler: handler,
	}
	go func() {
		err := server.ListenAndServe()
		if err != nil {
			t.Logf("serve dns server failed: %s", err)
			t.FailNow()
		}
	}()
	time.Sleep(1 * time.Second)
	defer server.Shutdown()

	dnsClient := &dns.Client{}
	msg := &dns.Msg{}
	msg.SetQuestion("aaa.com.", dns.TypeA)
	r, _, err := dnsClient.Exchange(msg, fmt.Sprintf("127.0.0.1:%v", port))
	if err != nil {
		t.Logf("query failed: %s", err)
		t.FailNow()
	}

	suc := false
	for _, a := range r.Answer {
		switch aRecord := a.(type) {
		case *dns.A:
			if aRecord.A.String() == flag {
				suc = true
			}
		}
	}

	if !suc {
		t.FailNow()
	}
}
