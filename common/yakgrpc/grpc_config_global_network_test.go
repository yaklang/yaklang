package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"os"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_GLOBAL_NETWORK_DNS_CONFIG(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})

	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		t.Fatal(err)
	}
	config.CustomDNSServers = []string{"127.0.0.1"}
	defer client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	check := false
	for _, i := range netx.NewDefaultReliableDNSConfig().SpecificDNSServers {
		if !check {
			if i == "127.0.0.1" {
				check = true
			}
		}
	}
	if !check {
		t.Fatal("set global network dns config failed")
	}
	client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	check = false
	for _, i := range netx.NewDefaultReliableDNSConfig().SpecificDNSServers {
		if !check {
			if i == "127.0.0.1" {
				check = true
			}
		}
	}
	if check {
		t.Fatal("set (reset) global network dns config failed")
	}
}

func TestGRPCMUSTPASS_RPOXY_FROM_ENV(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	var triggerProxy = false
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		spew.Dump(req)
		if strings.Contains(string(req), "CONNECT 8.8.8.8:80") {
			triggerProxy = true
		}
		return nil
	})

	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		panic(err)
	}

	config.GlobalProxy = nil
	config.EnableSystemProxyFromEnv = true
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	if err != nil {
		panic(err)
	}
	defer func() {
		_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	}()
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")
	os.Unsetenv("all_proxy")
	os.Unsetenv("proxy")
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("ALL_PROXY")

	os.Setenv("HTTP_PROXY", "http://"+utils.HostPort(host, port))
	_, err = yak.Execute(`
try {
	poc.Get("http://8.8.8.8")~
	die("unexpected result")
} catch e {
	if f"${e}".Contains("no proxy available") {
		dump(e)
		return
	} else{
		die(e)
	}
}
`)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}

	if !triggerProxy {
		t.Fatal("proxy not triggered")
	}
	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
}

func TestGRPCMUSTPASS_GLOBAL_RPOXY(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	var triggerProxy = false
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		spew.Dump(req)
		if strings.Contains(string(req), "CONNECT 8.8.8.8:80") {
			triggerProxy = true
		}
		return nil
	})

	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		panic(err)
	}

	config.GlobalProxy = []string{"http://" + utils.HostPort(host, port)}
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	if err != nil {
		panic(err)
	}
	defer func() {
		_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	}()
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")
	os.Unsetenv("all_proxy")
	os.Unsetenv("proxy")
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("ALL_PROXY")

	_, err = yak.Execute(`
try {
	poc.Get("http://8.8.8.8")~
	die("unexpected result")
} catch e {
	if f"${e}".Contains("no proxy available") {
		dump(e)
		return
	} else{
		die(e)
	}
}
`)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}

	if !triggerProxy {
		t.Fatal("proxy not triggered")
	}
}

func TestGRPCMUSTPASS_DISALLOW_ADDRESS(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	if err != nil {
		panic(err)
	}

	config.DisallowIPAddress = []string{"8.8.8.8"}
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	if err != nil {
		panic(err)
	}
	defer func() {
		_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	}()
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")
	os.Unsetenv("all_proxy")
	os.Unsetenv("proxy")
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("ALL_PROXY")

	_, err = yak.Execute(`
try {
	poc.Get("http://8.8.8.8")~
	die("unexpected result")
} catch e {
	if f"${e}".Contains("disallow address") {
		dump(e)
		return
	} else{
		die(e)
	}
}
`)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
}

func TestValidP12PassWord(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	ca, key, err := tlsutils.GenerateSelfSignedCertKey("127.0.0.1", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cert, sKey, err := tlsutils.SignServerCrtNKeyEx(ca, key, "", false)
	if err != nil {
		t.Fatal(err)
	}
	p12Bytes, err := tlsutils.BuildP12(cert, sKey, "123456", ca)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ValidP12PassWord(context.Background(), &ypb.ValidP12PassWordRequest{Pkcs12Bytes: p12Bytes, Pkcs12Password: []byte("123456")})
	if err != nil {
		log.Error(err)
		t.FailNow()
	}

}

//func TestHTTPAuth(t *testing.T) {
//	client, err := NewLocalClient()
//
//	_, _ = client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
//	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
//	if err != nil {
//		panic(err)
//	}
//
//	config.AuthInfos = []*ypb.AuthInfo{{
//		AuthType:     "any",
//		AuthUsername: "test",
//		AuthPassword: "test",
//		Host:         "47.120.44.219:8087",
//	}}
//	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
//	if err != nil {
//		panic(err)
//	}
//
//	rsp, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: 47.120.44.219:8087\r\n\r\n")))
//	if err != nil {
//		t.Fatal(err)
//	}
//	spew.Dump(rsp)
//}
