package yakgrpc

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_DiagnoseNetwork(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
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
		panic(err)
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
