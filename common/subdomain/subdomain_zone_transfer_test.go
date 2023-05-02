package subdomain

import (
	"context"
	"fmt"
	"github.com/miekg/dns"
	"math/rand"
	"strings"
	"testing"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func TestAXFR(t *testing.T) {
	scanner, err := NewSubdomainScanner(
		NewSubdomainScannerConfig(
			WithModes(ZONE_TRANSFER),
			WithDNSServers([]string{
				"10.3.0.3",
			}),
		),
		"vulhub.org",
	)
	if err != nil {
		t.Logf("create subdomain scanner failed: %s", err)
		t.FailNow()
	}

	count := 0
	scanner.OnResult(func(result *SubdomainResult) {
		count++
	})

	err = scanner.Run()
	if err != nil {
		t.Logf("run failed: %s", err)
		t.FailNow()
	}

	if count <= 0 {
		t.FailNow()
	}
}

func TestQueryNS(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	handler := &testDomainServer{}
	flag := "ns.aaa.com."
	handler.AddHandler(func(w dns.ResponseWriter, r *dns.Msg) {
		//pp.Println(r)
		msg := &dns.Msg{}
		msg.SetReply(r)
		msg.Answer = append(msg.Answer, &dns.NS{
			Hdr: dns.RR_Header{
				Name:   "aaa.com.",
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns: flag,
		})
		w.WriteMsg(msg)
	})

	port := rand.Intn(2000) + 60029
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for !utils.IsUDPPortAvailable(port) {
		port = rand.Intn(2000) + 60029
		addr = fmt.Sprintf("127.0.0.1:%d", port)
		log.Infof("port: %v is unavailable", port)
	}
	server := &dns.Server{
		Addr:    addr,
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

	ns, err := queryNs(&dns.Client{}, context.Background(), "aaa.com", []string{addr})
	if err != nil {
		t.Logf("BUG: query local dns server ns failed: %s", err)
		t.FailNow()
	}

	if len(ns) != 1 {
		t.Logf("query ns failed: %s", strings.Join(ns, " | "))
		t.FailNow()
	}

	if ns[0] != flag {
		t.Logf("query ns server failed: %s", flag)
		t.FailNow()
	}
}
