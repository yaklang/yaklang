package yakgrpc

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_VulinboxAgent(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	addr, err := vulinbox.NewVulinboxAgent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := utils.ParseStringToHostPort(addr)
	addr = utils.HostPort(host, port)
	rsp, err := client.ConnectVulinboxAgent(context.Background(), &ypb.IsRemoteAddrAvailableRequest{
		Addr: addr,
	})
	_ = rsp
	if err != nil {
		t.Fatal(err)
	}
	// spew.Dump(rsp)
}
