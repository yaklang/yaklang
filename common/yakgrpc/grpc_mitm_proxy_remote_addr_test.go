package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_MITM_RemoteAddr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token := utils.RandStringBytes(100)
	targetHost, targetPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("Hello Token" + "   " + token))
	})
	var targetUrl = "http://" + utils.HostPort(targetHost, targetPort)
	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			var msg = string(data.GetMessage().GetMessage())
			if strings.Contains(msg, "starting mitm server") {
				stream.Send(&ypb.MITMRequest{SetYakScript: true, YakScriptContent: `
mirrorNewWebsite = (tls, url, req, rsp, body) => {
	yakit.StatusCard("abc", 1)
}
`})
			}
		}

		if data.GetMessage().GetIsMessage() && strings.Contains(string(data.GetMessage().GetMessage()), `HotPatched MITM HOOKS`) {
			// do sth
			_, err := yak.Execute(`rsp, req := poc.Get(targetUrl, poc.proxy(mitmProxy), poc.save(false))~
assert string(rsp.RawPacket).Contains("Hello Token")
cancel()
`, map[string]any{"targetUrl": targetUrl, "mitmProxy": `http://` + utils.HostPort("127.0.0.1", mitmPort), "cancel": cancel})
			if err != nil {
				t.Fatal(err)
			}
		}

	}

	time.Sleep(time.Second)
	var data []*schema.HTTPFlow
	err = utils.AttemptWithDelayFast(func() error {
		_, data, _ = yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
			Keyword: token,
		})
		if len(data) <= 0 {
			return utils.Errorf("query empty")
		}
		return nil
	})
	require.NoError(t, err)

	spew.Dump(data[0].RemoteAddr)
	spew.Dump(targetHost, targetPort)
	spew.Dump(data[0].Response)
	if data[0].RemoteAddr != utils.HostPort(targetHost, targetPort) {
		t.Fatal("remote addr not match")
	}
	if len(data) != 1 {
		t.Fatal("data must be one!")
	}
}

func TestGRPCMUSTPASS_MITMV2_RemoteAddr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token := utils.RandStringBytes(100)
	targetHost, targetPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("Hello Token" + "   " + token))
	})
	var targetUrl = "http://" + utils.HostPort(targetHost, targetPort)
	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.MITMV2(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMV2Request{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			var msg = string(data.GetMessage().GetMessage())
			if strings.Contains(msg, "starting mitm server") {
				stream.Send(&ypb.MITMV2Request{SetYakScript: true, YakScriptContent: `
mirrorNewWebsite = (tls, url, req, rsp, body) => {
	yakit.StatusCard("abc", 1)
}
`})
			}
		}

		if data.GetMessage().GetIsMessage() && strings.Contains(string(data.GetMessage().GetMessage()), `HotPatched MITM HOOKS`) {
			// do sth
			_, err := yak.Execute(`rsp, req := poc.Get(targetUrl, poc.proxy(mitmProxy), poc.save(false))~
assert string(rsp.RawPacket).Contains("Hello Token")
cancel()
`, map[string]any{"targetUrl": targetUrl, "mitmProxy": `http://` + utils.HostPort("127.0.0.1", mitmPort), "cancel": cancel})
			if err != nil {
				t.Fatal(err)
			}
		}

	}

	time.Sleep(time.Second)
	var data []*schema.HTTPFlow
	err = utils.AttemptWithDelayFast(func() error {
		_, data, _ = yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
			Keyword: token,
		})
		if len(data) <= 0 {
			return utils.Errorf("query empty")
		}
		return nil
	})
	require.NoError(t, err)

	spew.Dump(data[0].RemoteAddr)
	spew.Dump(targetHost, targetPort)
	spew.Dump(data[0].Response)
	if data[0].RemoteAddr != utils.HostPort(targetHost, targetPort) {
		t.Fatal("remote addr not match")
	}
	if len(data) != 1 {
		t.Fatal("data must be one!")
	}
}
