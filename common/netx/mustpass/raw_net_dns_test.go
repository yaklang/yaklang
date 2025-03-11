package mustpass

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/facades"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBASIC_SPECIFIC_DNS_2(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	domain := strings.ToLower(utils.RandStringBytes(40) + "." + "com")
	addr := facades.MockDNSServerDefault(domain, func(record string, domain string) string {
		spew.Dump(record, domain)
		return "1.2.3.5"
	})

	time.Sleep(time.Second)
	var start = time.Now()
	var result = netx.LookupFirst(domain+":80",
		netx.WithDNSDisableSystemResolver(true),
		netx.WithDNSServers(addr), netx.WithDNSFallbackTCP(false))
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

	if netx.LookupFirst("1.2.3.4") != "1.2.3.4" {
		t.Fatal("LookupFirst ip failed")
	}
	if netx.LookupFirst("1.2.3.4:443") != "1.2.3.4" {
		t.Fatal("LookupFirst ip failed")
	}
}

func TestBASIC_SPECIFIC_DNS(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	domain := strings.ToLower(utils.RandStringBytes(40) + "." + "com")
	addr := facades.MockDNSServerDefault(domain, func(record string, domain string) string {
		spew.Dump(record, domain)
		return "1.2.3.5"
	})

	time.Sleep(time.Second)
	var start = time.Now()
	var result = netx.LookupFirst(domain,
		netx.WithDNSDisableSystemResolver(true),
		netx.WithDNSServers(addr), netx.WithDNSFallbackTCP(false))
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
	var result = netx.LookupFirst(domain,
		netx.WithDNSDisableSystemResolver(true),
		netx.WithDNSServers(addr), netx.WithDNSFallbackTCP(true),
		netx.WithDNSPreferTCP(true),
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
	var a = netx.LookupFirst(
		strings.ToLower(token)+".com",
		netx.WithDNSPreferDoH(true),
		netx.WithDNSDisableSystemResolver(true),
		netx.WithDNSSpecificDoH("http://"+utils.HostPort(host, port)+"/dns-query"),
	)
	if a != "1.2.3.4" {
		t.Errorf("DoH Failed")
		t.FailNow()
	}
}

func TestDialRfuseRetry(t *testing.T) {
	port := utils.GetRandomAvailableTCPPort()
	go func() {
		time.AfterFunc(1*time.Second, func() {
			listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			require.NoError(t, err)
			for {
				_, err := listener.Accept()
				require.NoError(t, err)
			}
		})
	}()
	conn, err := netx.DialX(fmt.Sprintf("127.0.0.1:%d", port), netx.DialX_WithTimeoutRetryWaitRange(2*time.Second, 5*time.Second))
	spew.Dump(conn)
	fmt.Println(err)
	require.NoError(t, err)
}
