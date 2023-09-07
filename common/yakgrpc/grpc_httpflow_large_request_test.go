package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_QueryHTTPFlow_Oversize_Request(t *testing.T) {
	var client, err = NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	yakit.DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{
		DeleteAll: false,
		Id:        nil,
		ItemHash:  nil,
		URLPrefix: "",
		Filter: &ypb.QueryHTTPFlowRequest{
			SourceType: "cccc",
		},
		URLPrefixBatch: nil,
	})

	rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Server: test
`))

	var flow *yakit.HTTPFlow
	flow, err = yakit.CreateHTTPFlowFromHTTPWithBodySavedFromRaw(true, lowhttp.FixHTTPRequest([]byte(
		`GET / HTTP/1.1
Host: www.example.com

`+strings.Repeat("b", 1000*1000*3))), lowhttp.ReplaceHTTPPacketBodyFast(rsp, []byte(strings.Repeat(strings.Repeat("a", 1000), 1000))), "cccc",
		"https://www.example.com", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	flow.CalcHash()
	consts.GetGormProjectDatabase().Save(flow)

	resp, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   100,
			OrderBy: "body_length",
			Order:   "desc",
		},
		Full:       false,
		SourceType: "cccc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetData()) <= 0 {
		t.Fatal("resp should not be empty")
	}

	var checkLargeBodyId int64
	for _, r := range resp.GetData() {
		if r.BodyLength > 800*1000 {
			checkLargeBodyId = int64(r.GetId())
			if len(r.Response) != 0 {
				t.Fatal("response should be empty")
			}
		} else if r.BodyLength < 100*1000 {
			if len(r.Response) == 0 {
				spew.Dump(r.Response)
				println(string(r.Response))
				t.Fatal("response should not be empty")
			}
		}

		if !funk.IsEmpty(r.Request) {
			spew.Dump(r.Request)
			t.Fatal("request should be empty")
		}
	}

	if checkLargeBodyId <= 0 {
		t.Fatal("no large body found")
	}

	start := time.Now()
	response, err := client.GetHTTPFlowById(utils.TimeoutContext(3*time.Second), &ypb.GetHTTPFlowByIdRequest{Id: checkLargeBodyId})
	if err != nil {
		spew.Dump(err)
		t.Fatal("cannot found large response")
	}
	if time.Now().Sub(start).Seconds() > 500 {
		t.Fatal("should be cached")
	}
	_ = response
	if len(response.GetResponse()) < 1000*800 {
		t.Fatal("response is missed")
	}

	if len(response.GetRequest()) < 1000*1000 {
		t.Fatal("request is missed")
	}
}
