package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestNasl(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	client.GetNaslFamilys(ctx, &ypb.GetNaslFamilysRequest{Name: ""})
	scripts, err := client.QueryNaslScript(ctx, &ypb.QueryNaslScriptRequest{
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 10000,
		},
		Family: "AIX Local Security Checks",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = scripts
}
