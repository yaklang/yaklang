package netstackvm

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"net"
	"net/netip"
	"sync"
	"time"
)

type NetStackVirtualMachine struct {
	entries map[tcpip.NICID]*NetStackVirtualMachineEntry
	mux     sync.Mutex
	stack   *stack.Stack
}

func (m *NetStackVirtualMachine) GetEntry(id tcpip.NICID) (*NetStackVirtualMachineEntry, bool) {
	m.mux.Lock()
	defer m.mux.Unlock()
	entry, ok := m.entries[id]
	return entry, ok
}

func (m *NetStackVirtualMachine) SetEntry(id tcpip.NICID, vm *NetStackVirtualMachineEntry) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.entries[id] = vm
}

func (m *NetStackVirtualMachine) GetStack() *stack.Stack {
	return m.stack
}

func (m *NetStackVirtualMachine) DialTCP(timeout time.Duration, target string) (net.Conn, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("panic: %v", err)
		}
	}()
	host, port, err := utils.ParseStringToHostPort(target)
	if err != nil {
		return nil, err
	}
	if !utils.IsIPv4(host) {
		host = netx.LookupFirst(host)
	}

	r, routeErr := m.stack.FindRoute(0, tcpip.Address{}, tcpip.AddrFrom4(netip.MustParseAddr(host).As4()), header.IPv4ProtocolNumber, false)
	if routeErr != nil {
		return nil, utils.Errorf("failed to find route: %v", routeErr)
	}
	defer r.Release()

	dialEntryID := r.NICID()

	entry, ok := m.GetEntry(dialEntryID)
	if !ok {
		return nil, utils.Errorf("failed to find vm: %d", dialEntryID)
	}
	return entry.DialTCP(timeout, utils.HostPort(host, port))
}

func (m *NetStackVirtualMachine) ListenTCP(addr string) (net.Listener, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("panic: %v", err)
		}
	}()
	host, port, err := utils.ParseStringToHostPort(addr)
	if err != nil {
		return nil, err
	}
	if !utils.IsIPv4(host) {
		host = netx.LookupFirst(host)
	}

	r, routeErr := m.stack.FindRoute(0, tcpip.Address{}, tcpip.AddrFrom4(netip.MustParseAddr(host).As4()), header.IPv4ProtocolNumber, false)
	if routeErr != nil {
		return nil, utils.Errorf("failed to find route: %v", routeErr)
	}
	defer r.Release()

	dialEntryID := r.NICID()

	entry, ok := m.GetEntry(dialEntryID)
	if !ok {
		return nil, utils.Errorf("failed to find vm: %d", dialEntryID)
	}
	return entry.ListenTCP(utils.HostPort(host, port))
}

func NewSystemNetStackVM(opts ...Option) (*NetStackVirtualMachine, error) {
	m := &NetStackVirtualMachine{
		entries: make(map[tcpip.NICID]*NetStackVirtualMachineEntry),
		mux:     sync.Mutex{},
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

	// 启动原则 ： 如果用户指定了需要启动的网卡则添加指定的网卡，如果没有则只启动localhost和默认路由网卡，除非开启option open
	// find public interface
	selectDevice := config.selectedDeviceName
	if selectDevice == "" {
		selectDevice, _ = netutil.GetPublicRouteIfaceName()
	}

	allNic, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, nic := range allNic {
		if nic.Flags&net.FlagRunning == 0 { // just get the running interface
			continue
		}

		if nic.Flags&net.FlagLoopback == 0 && nic.Name != selectDevice && !config.openAllPcapDevice { // if not loopback and not public interface, skip it
			continue
		}

		vm, err := NewNetStackVirtualMachineEntry(append(opts, WithPcapDevice(nic.Name), WithNetStack(s))...)
		if err != nil {
			log.Errorf("failed to build netStackVM: %v", err)
			continue
		}

		if selectDevice == nic.Name { // if the interface is the select interface, start dhcp, make sure gateway can use
			if config.ForceSystemNetStack {
				err = vm.InheritPcapInterfaceConfig()
				if err != nil {
					log.Errorf("nic[%s] failed to inherit public config: %v", nic.Name, err)
					continue
				}
			} else {
				if err := vm.StartDHCP(); err != nil {
					log.Errorf("StartDHCP failed: %v", err)
					continue
				}
				if err := vm.WaitDHCPFinished(context.Background()); err != nil {
					log.Errorf("Wait DHCP finished failed: %v", err)
					continue
				}
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
		m.SetEntry(vm.MainNICID(), vm)
	}
	if len(m.entries) == 0 {
		return nil, fmt.Errorf("no netStackVMManager build success")
	}
	return m, nil
}
