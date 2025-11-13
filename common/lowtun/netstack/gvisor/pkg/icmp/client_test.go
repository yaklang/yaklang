package icmp_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/icmp"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
)

func TestClient_ICMP(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}
	route, gateway, srcIP, err := netutil.GetPublicRoute()
	if err != nil {
		t.Fatal(err)
	}
	ifaceName := route.Name
	_ = gateway
	_ = srcIP
	userStack, err := netstackvm.NewNetStackVirtualMachineEntry(netstackvm.WithPcapDevice(ifaceName))
	require.NoError(t, err)

	err = userStack.StartDHCP()
	require.NoError(t, err)

	err = userStack.WaitDHCPFinished(context.Background())
	require.NoError(t, err)

	//target := "183.2.172.185/24,192.168.3.1/24"
	target := "183.2.172.42/24"
	//target := "192.168.3.1/24"
	res, err := icmp.NewClient(userStack.GetStack()).PingScan(context.Background(), target)
	require.NoError(t, err)
	count := 0
	for r := range res {
		if r == nil {
			continue
		}
		//fmt.Printf("[%s]: icmp type: %d, code: %d, id: %d\n", r.Address, r.MessageType, r.MessageCode, r.MessageID)
		count++
	}
	fmt.Printf("total alive: %d\n", count)
}

//func TestAbc(t *testing.T) {
//	if utils.InGithubActions() {
//		t.Skip()
//		return
//	}
//	for i := 0; i < 10; i++ {
//		TestClient_ICMP(t)
//	}
//}
