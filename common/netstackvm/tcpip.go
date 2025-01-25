package netstackvm

import (
	"net"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/utils"
)

func (vm *NetStackVirtualMachine) SetDefaultRoute(gateway net.IP) error {
	vm.stack.SetRouteTable([]tcpip.Route{
		{
			Destination: header.IPv4EmptySubnet,
			Gateway:     tcpip.AddrFromSlice(gateway),
			NIC:         vm.MainNICID(),
		},
		{
			Destination: header.IPv6EmptySubnet,
			Gateway:     tcpip.AddrFromSlice(gateway),
			NIC:         vm.MainNICID(),
		},
	})
	return nil
}

func (vm *NetStackVirtualMachine) GetMainNICIPv4Address() net.IP {
	return vm.mainNICIPv4Address
}

func (vm *NetStackVirtualMachine) GetMainNICIPv4Netmask() *net.IPNet {
	return vm.mainNICIPv4Netmask
}

func (vm *NetStackVirtualMachine) GetMainNICIPv4Gateway() net.IP {
	return vm.mainNICIPv4Gateway
}

func (vm *NetStackVirtualMachine) GetMainNICLinkAddress() net.HardwareAddr {
	return vm.mainNICLinkAddress
}

func (vm *NetStackVirtualMachine) SetMainNICv4(ipAddr net.IP, netmask *net.IPNet, getaway net.IP) error {
	if vm.mainNICID == 0 {
		return utils.Error("main nic id not set")
	}
	if ipAddr == nil {
		return utils.Error("ip address not set")
	}
	if ipAddr.IsUnspecified() {
		return utils.Errorf("ip address is unspecified: %v", ipAddr)
	}

	if netmask == nil {
		log.Warnf("netmask not set, use default netmask /24")
		netmask = &net.IPNet{
			IP:   ipAddr,
			Mask: net.CIDRMask(24, 32),
		}
	}
	if getaway == nil {
		// 计算网段第一个地址作为默认网关
		firstIP := make(net.IP, len(ipAddr))
		copy(firstIP, ipAddr)
		for i := range firstIP {
			firstIP[i] &= netmask.Mask[i]
		}
		firstIP[len(firstIP)-1]++
		getaway = firstIP
		log.Warnf("gateway not set, use default gateway via netmask: %v", getaway)
	}

	ones, bits := netmask.Mask.Size()
	_ = bits
	tcpErr := vm.stack.AddProtocolAddress(vm.MainNICID(), tcpip.ProtocolAddress{
		Protocol: header.IPv4ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   tcpip.AddrFromSlice(ipAddr.To4()),
			PrefixLen: ones,
		},
	}, stack.AddressProperties{})
	if tcpErr != nil {
		return utils.Errorf("failed to add protocol address: %v", tcpErr)
	}

	vm.mainNICIPv4Address = ipAddr
	vm.mainNICIPv4Netmask = netmask
	vm.mainNICIPv4Gateway = getaway

	return nil
}
