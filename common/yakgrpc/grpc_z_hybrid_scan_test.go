package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestServer_HybridScan(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	stream, err := client.HybridScan(context.Background())
	if err != nil {
		t.FailNow()
	}
	stream.Send(&ypb.HybridScanRequest{
		Control:        true,
		HybridScanMode: "new",
	})
	stream.Send(&ypb.HybridScanRequest{
		Targets: &ypb.HybridScanInputTarget{
			Input: "http://www.example.com",
		},
		Plugin: &ypb.HybridScanPluginConfig{
			PluginNames: []string{"基础 XSS 检测"},
		},
	})
	for {
		rsp, err := stream.Recv()
		if err != nil {
			log.Error(err)
			break
		}
		spew.Dump(rsp)
	}
}
