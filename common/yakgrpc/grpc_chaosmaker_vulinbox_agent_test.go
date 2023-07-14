package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_VulinboxAgent(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	addr, err := vulinbox.NewVulinboxAgent(context.Background())
	if err != nil {
		panic(err)
	}
	host, port, _ := utils.ParseStringToHostPort(addr)
	addr = utils.HostPort(host, port)
	spew.Dump(addr)
	rsp, err := client.ConnectVulinboxAgent(context.Background(), &ypb.IsRemoteAddrAvailableRequest{
		Addr: addr,
	})
	if err != nil {
		panic(err)
	}
	spew.Dump(rsp)
}
