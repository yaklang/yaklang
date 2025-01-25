package netstackvm

import (
	"context"
	"net"
	"sync"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/dhcp"
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
	stack  *stack.Stack
	config *Config

	driver *PCAPEndpoint

	mainNICID tcpip.NICID

	// dhcp only have one client
	dhcpStarted *utils.AtomicBool
	dhcpClient  *dhcp.Client

	// arp only have one client too
	arpServiceStarted    *utils.AtomicBool
	arpPersistentMap     *sync.Map
	arpPersistentMutex   sync.Mutex
	arpPersistentTrigger *utils.AtomicBool

	mainNICLinkAddress net.HardwareAddr
	mainNICIPv4Address net.IP
	mainNICIPv4Netmask *net.IPNet
	mainNICIPv4Gateway net.IP
}

func NewNetStackVirtualMachine(opts ...Option) (*NetStackVirtualMachine, error) {
	config := NewDefaultConfig()
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

	if configStackErr := loadStackOptions(config, stackIns); configStackErr != nil {
		return nil, utils.Errorf("load stack options: %s", configStackErr)
	}

	if string(config.MainNICLinkAddress) == "" {
		err := WithRandomMainNICLinkAddress()(config)
		if err != nil {
			return nil, utils.Errorf("failed with random main nic link address: %s", err)
		}
	}

	log.Infof("start to create pcap endpoint default mac: %v", config.MainNICLinkAddress.String())
	pcapEp, err := NewPCAPEndpoint(config.ctx, stackIns, config.pcapPromisc, config.pcapDevice, config.MainNICLinkAddress)
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

	if !config.DisableForwarding {
		if err := stackIns.SetForwardingDefaultAndAllNICs(header.IPv4ProtocolNumber, true); err != nil {
			return nil, utils.Errorf("set forwarding: %s", err)
		}
		if err := stackIns.SetForwardingDefaultAndAllNICs(header.IPv6ProtocolNumber, true); err != nil {
			return nil, utils.Errorf("set forwarding: %s", err)
		}
	}

	vm.stack = stackIns
	vm.dhcpStarted = utils.NewAtomicBool()
	vm.mainNICID = mainNicID
	vm.config = config
	vm.mainNICLinkAddress = config.MainNICLinkAddress
	vm.arpServiceStarted = utils.NewAtomicBool()
	vm.arpPersistentMap = new(sync.Map)
	vm.arpPersistentMutex = sync.Mutex{}
	vm.arpPersistentTrigger = utils.NewAtomicBool()
	vm.driver = pcapEp
	return vm, nil
}

func (vm *NetStackVirtualMachine) GetStack() *stack.Stack {
	return vm.stack
}

func (vm *NetStackVirtualMachine) MainNICID() tcpip.NICID {
	return vm.mainNICID
}

func (vm *NetStackVirtualMachine) Wait() {
	vm.stack.Wait()
}
