package netstackvm

import (
	"context"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/arp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/icmp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/udp"
	"net"
	"sync"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/dhcp"
	icmpClient "github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/icmp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv6"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/utils"
)

var DefaultNetStackVirtualMachine *NetStackVirtualMachine
var DefaultNetStackVirtualMachineMutex sync.Mutex

func GetDefaultNetStackVirtualMachine() (*NetStackVirtualMachine, error) {
	DefaultNetStackVirtualMachineMutex.Lock()
	defer DefaultNetStackVirtualMachineMutex.Unlock()
	if DefaultNetStackVirtualMachine == nil {
		vm, err := NewSystemNetStackVM()
		if err != nil {
			return nil, utils.Errorf("create netstack virtual machine failed: %v", err)
		}
		DefaultNetStackVirtualMachine = vm
	}
	return DefaultNetStackVirtualMachine, nil
}

func GetDefaultNetStackVirtualMachineWithoutDHCP() (*NetStackVirtualMachine, error) {
	DefaultNetStackVirtualMachineMutex.Lock()
	defer DefaultNetStackVirtualMachineMutex.Unlock()
	if DefaultNetStackVirtualMachine == nil {
		vm, err := NewSystemNetStackVM(WithForceSystemNetStack(true))
		if err != nil {
			return nil, utils.Errorf("create netstack virtual machine failed: %v", err)
		}
		DefaultNetStackVirtualMachine = vm
	}
	return DefaultNetStackVirtualMachine, nil
}

func GetDefaultICMPClient() *icmpClient.Client {
	vm, err := GetDefaultNetStackVirtualMachineWithoutDHCP()
	if err != nil {
		return nil
	}
	return icmpClient.NewClient(vm.GetStack())
}

type NetStackVirtualMachineEntry struct {
	systemIface *net.Interface
	mtu         int

	stack  *stack.Stack
	config *Config

	driver *PCAPEndpoint

	mainNICID tcpip.NICID

	// dhcp only have one client
	dhcpStarted *utils.AtomicBool
	dhcpClient  *dhcp.Client
	dhcpSuccess *utils.AtomicBool

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

func NewNetStackVirtualMachineEntry(opts ...Option) (*NetStackVirtualMachineEntry, error) {
	config := NewDefaultConfig()
	for _, opt := range opts {
		if err := opt(config); err != nil {
			return nil, err
		}
	}
	// 获取网络接口
	iface, err := net.InterfaceByName(config.pcapDevice)
	if err != nil {
		log.Debugf("failed to get interface %s: %v", config.pcapDevice, err)
		return nil, utils.Errorf("failed to get interface %s: %v", config.pcapDevice, err)
	}

	if config.ctx == nil {
		config.ctx, config.cancel = context.WithCancel(context.Background())
	}
	vm := &NetStackVirtualMachineEntry{}
	stackIns := config.stack
	if stackIns == nil {
		stackIns, err = NewNetStackFromConfig(config)
		if err != nil {
			return nil, err
		}
	}

	if string(config.MainNICLinkAddress) == "" {
		err := WithRandomMainNICLinkAddress()(config)
		if err != nil {
			return nil, utils.Errorf("failed with random main nic link address: %s", err)
		}
	}

	mtu := iface.MTU

	log.Infof("start to create pcap endpoint default mac: %v", config.MainNICLinkAddress.String())
	pcapEp, err := NewPCAPEndpoint(config.ctx, stackIns, config.pcapDevice, config.MainNICLinkAddress, config.pcapPromisc)
	if err != nil {
		return nil, err
	}

	pcapEp.SetPCAPOutboundFilter(config.pcapOutboundFilter)
	pcapEp.SetPCAPInboundFilter(config.pcapInboundFilter)
	pcapEp.SetCapabilities(pcapEp.capabilities | config.pcapCapabilities)

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
		if _, err := stackIns.SetNICForwarding(mainNicID, header.IPv4ProtocolNumber, true); err != nil {
			return nil, utils.Errorf("set forwarding: %s", err)
		}
		if _, err := stackIns.SetNICForwarding(mainNicID, header.IPv6ProtocolNumber, true); err != nil {
			return nil, utils.Errorf("set forwarding: %s", err)
		}
	}

	vm.stack = stackIns
	vm.dhcpStarted = utils.NewAtomicBool()
	vm.dhcpSuccess = utils.NewAtomicBool()
	vm.mainNICID = mainNicID
	vm.config = config
	vm.mainNICLinkAddress = config.MainNICLinkAddress
	vm.arpServiceStarted = utils.NewAtomicBool()
	vm.arpPersistentMap = new(sync.Map)
	vm.arpPersistentMutex = sync.Mutex{}
	vm.arpPersistentTrigger = utils.NewAtomicBool()
	vm.driver = pcapEp
	vm.mtu = mtu
	vm.systemIface = iface
	return vm, nil
}

func (vm *NetStackVirtualMachineEntry) GetMTU() int {
	return vm.mtu
}

func (vm *NetStackVirtualMachineEntry) GetSystemInterface() *net.Interface {
	return vm.systemIface
}

func (vm *NetStackVirtualMachineEntry) GetStack() *stack.Stack {
	return vm.stack
}

func (vm *NetStackVirtualMachineEntry) MainNICID() tcpip.NICID {
	return vm.mainNICID
}

func (vm *NetStackVirtualMachineEntry) Wait() {
	vm.stack.Wait()
}

func NewNetStackFromConfig(c *Config) (*stack.Stack, error) {
	stackOpt := stack.Options{}

	if !c.DisallowPacketEndpointWrite {
		stackOpt.AllowPacketEndpointWrite = true
	}

	if !c.IPv4Disabled {
		stackOpt.NetworkProtocols = append(stackOpt.NetworkProtocols, ipv4.NewProtocol)
	}
	if !c.IPv6Disabled {
		stackOpt.NetworkProtocols = append(stackOpt.NetworkProtocols, ipv6.NewProtocol)
	}
	if !c.ARPDisabled {
		stackOpt.NetworkProtocols = append(stackOpt.NetworkProtocols, arp.NewProtocol)
	}

	// dhcp is beyond udp, ignore it in stack opt
	if !c.UDPDisabled {
		stackOpt.TransportProtocols = append(stackOpt.TransportProtocols, udp.NewProtocol)
	}
	if !c.TCPDisabled {
		stackOpt.TransportProtocols = append(stackOpt.TransportProtocols, tcp.NewProtocol)
	}

	if !c.ICMPDisabled {
		stackOpt.TransportProtocols = append(stackOpt.TransportProtocols, icmp.NewProtocol4)
		stackOpt.TransportProtocols = append(stackOpt.TransportProtocols, icmp.NewProtocol6)
	}
	stackOpt.HandleLocal = c.HandleLocal
	stackIns := stack.New(stackOpt)
	if configStackErr := loadStackOptions(c, stackIns); configStackErr != nil {
		return nil, utils.Errorf("load stack options: %s", configStackErr)
	}
	return stackIns, nil
}
