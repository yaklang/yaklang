package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_QueryHTTPFlow_Oversize(t *testing.T) {
	var client, err = NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   100,
			OrderBy: "body_length",
			Order:   "desc",
		},
		Full: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resp.GetData() {
		if r.BodyLength > 1000*1000 {
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
	}
}
