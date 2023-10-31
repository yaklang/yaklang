package yakgrpc

import (
	"context"
	"fmt"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_LARGE_RESPOSNE(t *testing.T) {
	var port int
	var ctx, cancel = context.WithCancel(utils.TimeoutContextSeconds(60))
	defer cancel()
	addr, err := vulinbox.NewVulinServerEx(ctx, true, false, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := utils.ParseStringToHostPort(addr)
	vulinboxAddr := utils.HostPort(host, port)
	token := ksuid.New().String()
	NewMITMTestCase(
		t,
		CaseWithMaxContentLength(100),
		CaseWithContext(ctx),
		CaseWithPort(func(i int) {
			port = i
		}),
		CaseWithServerStart(func() {
			_, err := yak.Execute(
				`rsp, req = poc.HTTP(packet, poc.proxy(mitmProxy), poc.save(false))~;
cancel()
assert len(rsp) > 1111100`,
				map[string]any{
					"packet": `GET /misc/response/content_length?cl=111110000&c=` + token + ` HTTP/1.1
Host: ` + vulinboxAddr + "\r\n\r\n",
					`cancel`:    cancel,
					"mitmProxy": fmt.Sprintf(`http://127.0.0.1:%v`, port),
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			cancel()
			_, data, err := yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
				Keyword: token,
			})
			if err != nil {
				t.Fatal("query taged flow failed", err)
			}
			if len(data) != 1 {
				t.Fatal("query taged flow failed(count is not right)")
			}
			if data[0].BodyLength < 111110000 {
				t.Fatal("query taged flow failed")
			}
			if !data[0].IsTooLargeResponse {
				t.Fatal("too-large-response tag not found")
			}
		}),
	)
}
