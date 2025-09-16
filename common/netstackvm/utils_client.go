package netstackvm

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	icmpClient "github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/icmp"
	"sync"
)

var defaultICMPClient *icmpClient.Client
var defaultICMPClientMux sync.Mutex

func GetDefaultICMPClient() *icmpClient.Client {
	defaultICMPClientMux.Lock()
	defer defaultICMPClientMux.Unlock()
	if defaultICMPClient != nil {
		return defaultICMPClient
	}
	vm, err := NewSystemNetStackVMWithoutDHCP(defaultICMPClientOpt()...)
	if err != nil {
		return nil
	}
	defaultICMPClient = icmpClient.NewClient(vm.GetStack())
	return defaultICMPClient
}

func defaultICMPClientOpt() []Option {
	return []Option{
		WithPCAPInboundFilter(func(packet gopacket.Packet) bool {
			if packet.Layer(layers.LayerTypeICMPv4) == nil && packet.Layer(layers.LayerTypeICMPv6) == nil {
				return false
			}
			return true
		}),
		WithPCAPOutboundFilter(func(packet gopacket.Packet) bool {
			if packet.Layer(layers.LayerTypeICMPv4) == nil && packet.Layer(layers.LayerTypeICMPv6) == nil {
				return false
			}
			return true
		}),
	}
}

var defaultSYNScanClient *NetStackVirtualMachine
var defaultSYNScanMutex sync.Mutex

func GetDefaultSYNScanClient() *NetStackVirtualMachine {
	defaultSYNScanMutex.Lock()
	defer defaultSYNScanMutex.Unlock()
	if defaultSYNScanClient != nil {
		return defaultSYNScanClient
	}
	vm, err := NewSystemNetStackVMWithoutDHCP(DefaultSYNScanOption()...)
	if err != nil {
		return nil
	}
	defaultSYNScanClient = vm
	return defaultSYNScanClient
}

func DefaultSYNScanOption() []Option {
	return []Option{
		WithPCAPInboundFilter(func(packet gopacket.Packet) bool {
			tcpLayerRaw := packet.Layer(layers.LayerTypeTCP)
			if tcpLayerRaw == nil {
				return false
			}
			tcpLayer, ok := tcpLayerRaw.(*layers.TCP)
			if !ok {
				return false
			}
			if tcpLayer.ACK && tcpLayer.SYN {
				return true
			}
			return false
		}),
		WithPCAPOutboundFilter(func(packet gopacket.Packet) bool {
			tcpLayerRaw := packet.Layer(layers.LayerTypeTCP)
			if tcpLayerRaw == nil {
				return false
			}
			tcpLayer, ok := tcpLayerRaw.(*layers.TCP)
			if !ok {
				return false
			}
			if tcpLayer.SYN {
				return true
			}
			return false
		}),
	}
}
