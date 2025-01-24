package netstackvm

import (
	"context"
	"net"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/arp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv6"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/udp"
	"github.com/yaklang/yaklang/common/utils"
)

type NetStackVirtualMachine struct {
	stack *stack.Stack
}

func NewNetStackVirtualMachine(opts ...Option) (*NetStackVirtualMachine, error) {
	config := NewConfig()
	for _, opt := range opts {
		if err := opt(config); err != nil {
			return nil, err
		}
	}

	if config.ctx == nil {
		config.ctx, config.cancel = context.WithCancel(context.Background())
	}

	vm := &NetStackVirtualMachine{}

	stackOpt := stack.Options{}

	if !config.DisallowPacketEndpointWrite {
		stackOpt.AllowPacketEndpointWrite = true
	}

	if !config.IPv4Disabled {
		stackOpt.NetworkProtocols = append(stackOpt.NetworkProtocols, ipv4.NewProtocol)
	}
	if !config.IPv6Disabled {
		stackOpt.NetworkProtocols = append(stackOpt.NetworkProtocols, ipv6.NewProtocol)
	}
	if !config.ARPDisabled {
		stackOpt.NetworkProtocols = append(stackOpt.NetworkProtocols, arp.NewProtocol)
	}

	// dhcp is beyond udp, ignore it in stack opt
	if !config.UDPDisabled {
		stackOpt.TransportProtocols = append(stackOpt.TransportProtocols, udp.NewProtocol)
	}
	if !config.TCPDisabled {
		stackOpt.TransportProtocols = append(stackOpt.TransportProtocols, tcp.NewProtocol)
	}

	stackOpt.HandleLocal = config.HandleLocal

	stackIns := stack.New(stackOpt)
	pcapEp, err := NewPCAPEndpoint(config.ctx, stackIns, config.pcapPromisc, config.pcapDevice)
	if err != nil {
		return nil, err
	}

	mainNicID := stackIns.NextNICID()
	tcpErr := stackIns.CreateNICWithOptions(mainNicID, pcapEp, stack.NICOptions{
		DeliverLinkPackets: config.EnableLinkLayer,
	})
	if tcpErr != nil {
		return nil, utils.Errorf("create NIC: %s", tcpErr)
	}
	stackIns.SetPromiscuousMode(mainNicID, config.pcapPromisc)

	if config.MainNICIPv4Address != "" {
		ip, _, err := net.ParseCIDR(config.MainNICIPv4Address)
		if err != nil {
			return nil, err
		}
		netmask, _, err := net.ParseCIDR(config.MainNICIPv4AddressNetmask)
		if err != nil {
			return nil, err
		}
		_ = netmask
		tcpErr := stackIns.AddProtocolAddress(mainNicID, tcpip.ProtocolAddress{
			Protocol: ipv4.ProtocolNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   tcpip.AddrFrom4([4]byte(ip.To4())),
				PrefixLen: 24, // uint8(net.IPv4len - len(netmask.Mask)),
			},
		}, stack.AddressProperties{})
		if tcpErr != nil {
			return nil, utils.Errorf("add protocol address: %s", tcpErr)
		}
	}
	if config.MainNICIPv6Address != "" {
		ip, _, err := net.ParseCIDR(config.MainNICIPv6Address)
		if err != nil {
			return nil, err
		}
		tcpErr := stackIns.AddProtocolAddress(mainNicID, tcpip.ProtocolAddress{
			Protocol: ipv6.ProtocolNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   tcpip.AddrFrom16([16]byte(ip.To16())),
				PrefixLen: 64,
			},
		}, stack.AddressProperties{})
		if tcpErr != nil {
			return nil, utils.Errorf("add protocol address: %s", tcpErr)
		}
	}

	if config.MainNICLinkAddress != nil {
		tcpErr := stackIns.SetNICAddress(mainNicID, tcpip.LinkAddress(config.MainNICLinkAddress))
		if tcpErr != nil {
			return nil, utils.Errorf("set nic address: %s", tcpErr)
		}
	}

	vm.stack = stackIns
	return vm, nil
}
