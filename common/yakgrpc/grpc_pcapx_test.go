package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestServer_PcapX(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.GetPcapMetadata(context.Background(), &ypb.PcapMetadataRequest{})
	if err != nil {
		panic(err)
	}
	spew.Dump(rsp)
}
