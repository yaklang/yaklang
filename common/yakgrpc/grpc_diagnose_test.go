package yakgrpc

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_COMMON_DiagnoseNetwork(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip in github actions, because it may not have access to the internet")
	}
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.DiagnoseNetwork(utils.TimeoutContextSeconds(30), &ypb.DiagnoseNetworkRequest{
		NetworkTimeout:    5,
		ConnectTarget:     "baidu.com,feishu.cn:443",
		Proxy:             "http://127.0.0.1:7890",
		ProxyAuthUsername: "",
		ProxyAuthPassword: "",
		ProxyToAddr:       "google.com",
		Domain:            "jianshu.cn",
		DNSServers:        nil,
	})
	if err != nil {
		t.Fatal(err)
	}

	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.LogLevel != "" {
			t.Logf("log: [%v]: (%v)%v", data.LogLevel, data.Title, data.DiagnoseResult)
		} else {
			t.Logf("[%v]:  %v\n%v", data.DiagnoseType, data.Title, data.DiagnoseResult)
		}
	}
}

func TestGRPCMUSTPASS_COMMON_DIAGNOSE_DNS(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip in github actions, because it may not have access to the internet")
	}
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.DiagnoseNetworkDNS(context.Background(), &ypb.DiagnoseNetworkDNSRequest{
		Domain: "baidu.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(data)
	}
}
