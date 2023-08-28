package yakgrpc

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
)

func TestGRPCMUSTPASS_MITM_H2_RepeatHeaderError(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(2)
	targetHost, targetPort := utils.DebugMockHTTP2(ctx, func(req []byte) []byte {
		return []byte(`HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8

`)
	})
	target := utils.HostPort(targetHost, targetPort)
	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	stream, err := client.MITM(ctx)
	if err != nil {
		panic(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host:        "127.0.0.1",
		Port:        uint32(mitmPort),
		EnableHttp2: true,
	})
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			var msg = string(data.GetMessage().GetMessage())
			fmt.Println(msg)
			if strings.Contains(msg, "starting mitm server") {
				// do sth
				packet := `GET / HTTP/2.0
Host: ` + target + `
content-type: text/plain

{"a": 1}`
				_, err := yak.Execute(`
rsp, req = poc.HTTP(packet, poc.https(true), poc.http2(true), poc.proxy(f"http://127.0.0.1:${mitmPort}"))~
dump(rsp)
`, map[string]any{"packet": packet, "mitmPort": mitmPort})
				if err != nil {
					panic(err)
				}
			}
		}
	}
}
