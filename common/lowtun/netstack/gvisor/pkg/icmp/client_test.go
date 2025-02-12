package icmp

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"testing"
)

//func TestClient_DoICMP(t *testing.T) {
//	target := "192.168.3.83"
//	domains := utils.ParseStringToHosts(target)
//	route, gateway, srcIP, err := netutil.GetPublicRoute()
//	if err != nil {
//		t.Fatal(err)
//	}
//	ifaceName := route.Name
//	_ = gateway
//	_ = srcIP
//	userStack, err := netstackvm.NewNetStackVirtualMachine(netstackvm.WithPcapDevice(ifaceName))
//
//	err = userStack.InheritPcapInterfaceRoute()
//	if err != nil {
//		panic(utils.Errorf("stark inherit pcap interface route %v", err))
//	}
//
//	//if err := userStack.StartDHCP(); err != nil {
//	//	panic(utils.Errorf("start dhcp failed: %v", err))
//	//}
//	//
//	//if err := userStack.WaitDHCPFinished(context.Background()); err != nil {
//	//	panic(utils.Errorf("Wait DHCP finished failed: %v", err))
//	//}
//
//	targetChan := make(chan string, 100)
//
//	go func() {
//		defer close(targetChan)
//		for _, domain := range domains {
//			targetChan <- domain
//		}
//
//	}()
//
//	res, err := NewClient(userStack.GetStack(), userStack.MainNICID()).FastPing(context.Background(), targetChan)
//	if err != nil {
//		t.Fatal(err)
//	}
//	count := 0
//	for r := range res {
//		if r == nil {
//			continue
//		}
//		if r.IsTimeout {
//			//fmt.Printf("[%s]: time out\n", r.Address)
//		} else {
//			fmt.Printf("[%s]: icmp type: %d, code: %d, id: %d\n", r.Address, r.MessageType, r.MessageCode, r.MessageID)
//			count++
//		}
//	}
//	fmt.Printf("total alive: %d\n", count)
//}

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
	userStack, err := netstackvm.NewNetStackVirtualMachine(netstackvm.WithPcapDevice(ifaceName))

	err = userStack.InheritPcapInterfaceRoute()
	if err != nil {
		panic(utils.Errorf("stark inherit pcap interface route %v", err))
	}
	target := "183.2.172.185/24,192.168.3.1/24"
	//target := "183.2.172.185/24"
	//target := "192.168.3.1/24"
	res, err := NewClient(userStack.GetStack()).PingScan(context.Background(), target, WithRetries(1))
	if err != nil {
		t.Fatal(err)
	}
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

func TestAbc(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}
	for i := 0; i < 10; i++ {
		TestClient_ICMP(t)
	}
}
