package yakgrpc

import (
	"context"
	"testing"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func TestCriticalQueryYakScript(t *testing.T) {
	client, err := NewLocalClient()
	die(err)

	rsp, err := client.QueryYakScript(context.Background(), &ypb.QueryYakScriptRequest{
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 10,
		},
	})
	die(err)
	_ = rsp

}
