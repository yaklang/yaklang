package mustpass

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/facades"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakdns"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBASIC_SPECIFIC_DNS(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	domain := strings.ToLower(utils.RandStringBytes(40) + "." + "com")
	addr := facades.MockDNSServerDefault(domain, func(record string, domain string) string {
		spew.Dump(record, domain)
		return "1.2.3.5"
	})

	time.Sleep(time.Second)
	var start = time.Now()
	var result = yakdns.LookupFirst(domain,
		yakdns.WithDNSDisableSystemResolver(true),
		yakdns.WithDNSServers(addr), yakdns.WithDNSFallbackTCP(false))
	log.Infof("LookupFirst %s cost %s", domain, time.Since(start))
	if time.Now().Sub(start).Milliseconds() > 300 {
		t.Errorf("LookupFirst %s cost %s", domain, time.Since(start))
		t.FailNow()
	}
	if result != "1.2.3.5" {
		t.Log("LookupFirst failed")
		t.FailNow()
	}
	spew.Dump(result)
}

func TestBASIC_SPECIFIC_TCP_FALLBACK_DNS(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	domain := strings.ToLower(utils.RandStringBytes(40) + "." + "com")
	addr := facades.MockTCPDNSServerDefault(domain, func(record string, domain string) string {
		spew.Dump(record, domain)
		return "1.2.3.5"
	})
	_ = addr

	time.Sleep(time.Second)
	var start = time.Now()
	var result = yakdns.LookupFirst(domain,
		yakdns.WithDNSDisableSystemResolver(true),
		yakdns.WithDNSServers(addr), yakdns.WithDNSFallbackTCP(true),
		yakdns.WithDNSPreferTCP(true),
	)
	log.Infof("LookupFirst %s cost %s", domain, time.Since(start))
	if time.Now().Sub(start).Milliseconds() > 300 {
		t.Errorf("LookupFirst %s cost %s", domain, time.Since(start))
		t.FailNow()
	}
	if result != "1.2.3.5" {
		t.Log("LookupFirst failed")
		t.FailNow()
	}
	spew.Dump(result)
}

func TestNotExisted_OnlyDoH(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(`{"Status":0,"TC":false,"RD":true,"RA":true,"AD":false,"CD":false,"Question":{"name":"baidu.com.","type":1},"Answer":[{"name":"baidu.com.","TTL":403,"type":1,"data":"1.2.3.4"}]}`))
	})
	log.SetLevel(log.DebugLevel)
	token := utils.RandStringBytes(20)
	_ = token
	var a = yakdns.LookupFirst(
		strings.ToLower(token)+".com",
		yakdns.WithDNSPreferDoH(true),
		yakdns.WithDNSDisableSystemResolver(true),
		yakdns.WithDNSSpecificDoH("http://"+utils.HostPort(host, port)+"/dns-query"),
	)
	if a != "1.2.3.4" {
		t.Errorf("DoH Failed")
		t.FailNow()
	}
}
