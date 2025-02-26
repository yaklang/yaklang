package arp

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"testing"
)

func TestArpClient(t *testing.T) {
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

	err = userStack.InheritPcapInterfaceIP()
	require.NoError(t, err)

	client, err := NewClient(userStack.GetStack(), userStack.MainNICID())
	require.NoError(t, err)
	reply, err := client.ArpRequest(utils.TimeoutContextSeconds(5), "192.168.3.212", "")
	require.NoError(t, err)
	spew.Dump(reply)

	reply, err = client.ArpRequest(utils.TimeoutContextSeconds(5), "192.168.3.1", "")
	require.NoError(t, err)
	spew.Dump(reply)
}
