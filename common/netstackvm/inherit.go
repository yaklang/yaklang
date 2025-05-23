package netstackvm

import (
	"encoding/hex"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/utils/arptable"
	"net"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
)

func (vm *NetStackVirtualMachineEntry) GetOSNetStackIPv4() (net.IP, net.IP, net.IPMask) {
	iface := vm.GetSystemInterface()
	addrs, err := iface.Addrs()
	if err != nil {
		log.Warnf("failed to get addresses for interface %s: %v", vm.config.pcapDevice, err)
		return nil, nil, nil
	}

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ipv4 := ipnet.IP.To4(); ipv4 != nil {
			// 计算网关地址 - 使用网段的第一个地址作为网关
			gateway := make(net.IP, len(ipv4))
			copy(gateway, ipv4)

			// 通过掩码计算网段第一个地址作为网关
			for i := range gateway {
				gateway[i] = ipv4[i] & ipnet.Mask[i]
			}
			gateway[3]++

			return ipv4, gateway, ipnet.Mask
		}
	}
	return nil, nil, nil
}

func (vm *NetStackVirtualMachineEntry) GetOSNetStackIPv6() (net.IP, net.IP, net.IPMask) {
	// 获取接口地址列表
	addrs, err := vm.GetSystemInterface().Addrs()
	if err != nil {
		log.Debugf("failed to get addresses for interface %s: %v", vm.config.pcapDevice, err)
		return nil, nil, nil
	}

	// 遍历所有地址找到IPv6地址
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		// 确保是IPv6地址而不是IPv4地址
		if ipv6 := ipnet.IP.To16(); ipv6 != nil && ipnet.IP.To4() == nil {
			// 计算网关地址 - 使用网段的第一个可用地址作为网关
			gateway := make(net.IP, net.IPv6len)
			copy(gateway, ipv6)

			// 通过掩码计算网段第一个地址作为网关
			for i := range gateway {
				gateway[i] = ipv6[i] & ipnet.Mask[i]
			}

			// 将最后一个字节加1得到网关地址
			gateway[net.IPv6len-1]++

			log.Debugf("found IPv6 address: %v, gateway: %v, mask: %v", ipv6, gateway, ipnet.Mask)
			return ipv6, gateway, ipnet.Mask
		}
	}

	log.Debug("no IPv6 address found")
	return nil, nil, nil
}

// InheritPcapInterfaceIP inherits the IP address of the pcap interface, set default mac address for driver(pcap endpoint)
func (vm *NetStackVirtualMachineEntry) InheritPcapInterfaceIP() error {
	ipv4, gateway4, mask4 := vm.GetOSNetStackIPv4()
	vm.driver.SetGatewayIP(gateway4)
	err := vm.SetMainNICv4(ipv4, &net.IPNet{
		IP:   ipv4,
		Mask: mask4,
	}, gateway4)
	if err != nil {
		return err
	}
	if macAddr, err := arptable.SearchHardware(vm.mainNICIPv4Gateway.String()); err == nil {
		vm.driver.SetGatewayHardwareAddr(macAddr)
		tcpErr := vm.stack.AddStaticNeighbor(
			vm.MainNICID(),
			header.IPv4ProtocolNumber,
			tcpip.AddrFrom4([4]byte(vm.mainNICIPv4Gateway.To4())),
			tcpip.LinkAddress(string(macAddr)),
		)
		if tcpErr != nil {
			log.Errorf("add static neighbor failed: %v", tcpErr)
		}
	}
	return nil
}

// GetPcapInterfaceNeighborRoute returns the route of the pcap interface, should call after set main nic ip
func (vm *NetStackVirtualMachineEntry) GetPcapInterfaceNeighborRoute() (tcpip.Route, error) {
	mask4 := vm.GetMainNICIPv4Netmask().Mask
	ipv4 := vm.GetMainNICIPv4Address().To4()

	networkSegment := make([]byte, 4)
	for i := 0; i < len(ipv4); i++ {
		networkSegment[i] = ipv4[i] & mask4[i]
	}

	mask4Byte, _ := hex.DecodeString(mask4.String())
	neigh, err := tcpip.NewSubnet(tcpip.AddrFrom4([4]byte(networkSegment)), tcpip.MaskFrom(string(mask4Byte)))
	if err != nil {
		return tcpip.Route{}, err
	}

	return tcpip.Route{
		Destination: neigh,
		NIC:         vm.MainNICID(),
		MTU:         uint32(vm.mtu),
	}, nil
}

// InheritPcapInterfaceNeighborRoute inherits the route of the pcap interface
func (vm *NetStackVirtualMachineEntry) InheritPcapInterfaceNeighborRoute() error {
	route, err := vm.GetPcapInterfaceNeighborRoute()
	if err != nil {
		return err
	}
	vm.stack.AddRoute(route)
	return nil
}

// InheritPcapInterfaceConfig inherits the IP address and route of the pcap interface( will set default route, netx hop is gateway)
func (vm *NetStackVirtualMachineEntry) InheritPcapInterfaceConfig() error {
	err := vm.InheritPcapInterfaceIP()
	if err != nil {
		return err
	}
	vm.stack.AddStaticNeighbor(
		vm.MainNICID(),
		header.IPv4ProtocolNumber,
		tcpip.AddrFrom4([4]byte(vm.GetMainNICIPv4Address())), "")
	gateway4 := vm.GetMainNICIPv4Gateway()
	err = vm.SetDefaultRoute(gateway4)
	if err != nil {
		return err
	}
	err = vm.InheritPcapInterfaceNeighborRoute()
	if err != nil {
		return err
	}
	return err
}
