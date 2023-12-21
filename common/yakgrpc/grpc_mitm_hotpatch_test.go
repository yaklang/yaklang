package yakgrpc

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_MITM_HotPatch_Drop(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(5))
	defer cancel()

	mockHost, mockPort := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(500)
	})

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
			msg := string(data.GetMessage().GetMessage())
			if !strings.Contains(msg, "starting mitm server") {
				continue
			}
			// load hot-patch mitm plugin
			stream.Send(&ypb.MITMRequest{
				SetYakScript:     true,
				YakScriptContent: `hijackHTTPResponseEx = func(isHttps, url, req, rsp, forward, drop) { drop() }`,
			})
		} else if data.GetCurrentHook && len(data.GetHooks()) > 0 {
			// send packet
			packet := `GET / HTTP/1.1
Host: ` + utils.HostPort(mockHost, mockPort) + `

`
			packetBytes := lowhttp.FixHTTPRequest([]byte(packet))
			_, err := yak.Execute(`
rsp, req, err = poc.HTTPEx(packet, poc.proxy(mitmProxy))
assert err.Error() == "EOF"
`, map[string]any{
				"packet":    string(packetBytes),
				"mitmProxy": `http://` + utils.HostPort("127.0.0.1", mitmPort),
			})
			if err != nil {
				t.Fatal(err)
			}
			cancel()
		}
	}
}
