package netstackvm

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"net"
)

type NetStackVirtualMachineManager struct {
	vms   []*NetStackVirtualMachine
	stack *stack.Stack
}

func (m *NetStackVirtualMachineManager) GetStack() *stack.Stack {
	return m.stack
}

func NewSystemNetStackVMManager(opts ...Option) (*NetStackVirtualMachineManager, error) {
	m := &NetStackVirtualMachineManager{
		vms: make([]*NetStackVirtualMachine, 0),
	}

	// build net stack
	config := NewDefaultConfig()
	for _, opt := range opts {
		if err := opt(config); err != nil {
			return nil, err
		}
	}

	s, err := NewNetStackFromConfig(config)
	if err != nil {
		return nil, err
	}
	m.stack = s

	// find public interface
	publicIfaceName, _ := netutil.GetPublicRouteIfaceName()
	allNic, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, nic := range allNic {
		if nic.Flags&net.FlagRunning == 0 { // just get the running interface
			continue
		}
		vm, err := NewNetStackVirtualMachine(WithPcapDevice(nic.Name), WithNetStack(s))
		if err != nil {
			log.Errorf("failed to build netStackVM: %v", err)
			return nil, err
		}

		if publicIfaceName == nic.Name { // if the interface is the public interface, start dhcp, make sure gateway can use
			if err := vm.StartDHCP(); err != nil {
				log.Errorf("StartDHCP failed: %v", err)
				continue
			}
			if err := vm.WaitDHCPFinished(context.Background()); err != nil {
				log.Errorf("Wait DHCP finished failed: %v", err)
				continue
			}
		} else { // if the interface is lan interface, inherit the pcap interface ip and route, not need set default route.
			err = vm.InheritPcapInterfaceIP()
			if err != nil {
				log.Errorf("nic[%s] failed to inherit ip: %v", nic.Name, err)
				continue
			}
			err = vm.InheritPcapInterfaceNeighborRoute()
			if err != nil {
				log.Errorf("nic[%s] failed to inherit route: %v", nic.Name, err)
				continue
			}
		}
		m.vms = append(m.vms, vm)
	}
	if len(m.vms) == 0 {
		return nil, fmt.Errorf("no netStackVMManager build success")
	}
	routeTable := s.GetRouteTable()
	for _, route := range routeTable {
		fmt.Println(route.String())
	}

	return m, nil
}
