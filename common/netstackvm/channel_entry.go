package netstackvm

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/utils"
)

const channelNetstackMTU = 1500

// NewChannelNetStackVirtualMachineEntry builds a gVisor-backed entry whose
// packets stay in a channel instead of a host NIC. DHCP is marked ready so
// DialTCP uses the gVisor stack. Nothing forwards packets until StartTCPProbe
// or BridgeChannelNetStacks is called.
func NewChannelNetStackVirtualMachineEntry(ipv4Addr string) (*NetStackVirtualMachineEntry, error) {
	ip := net.ParseIP(ipv4Addr)
	if ip == nil || ip.To4() == nil {
		return nil, fmt.Errorf("channel netstack wants an ipv4 address, got %q", ipv4Addr)
	}
	ip = ip.To4()
	if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() {
		return nil, fmt.Errorf("channel netstack address %s is not usable", ip)
	}

	config := NewDefaultConfig()
	config.ctx, config.cancel = context.WithCancel(context.Background())
	config.DHCPDisabled = true
	config.pcapCapabilities = 0

	stackIns, err := NewNetStackFromConfig(config)
	if err != nil {
		return nil, err
	}

	link := channel.New(1<<10, channelNetstackMTU, "")
	// Checksums are offloaded so a cloned packet can be injected into the
	// peer stack without recomputing them. Sequence numbers and flags are
	// still produced by gVisor's TCP stack.
	link.LinkEPCapabilities = stack.CapabilityRXChecksumOffload | stack.CapabilityTXChecksumOffload

	nicID := stackIns.NextNICID()
	if tcpErr := stackIns.CreateNIC(nicID, link); tcpErr != nil {
		return nil, utils.Errorf("create channel NIC: %s", tcpErr)
	}

	mask := net.CIDRMask(24, 32)
	if tcpErr := stackIns.AddProtocolAddress(nicID, tcpip.ProtocolAddress{
		Protocol: ipv4.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   tcpip.AddrFrom4([4]byte(ip)),
			PrefixLen: 24,
		},
	}, stack.AddressProperties{}); tcpErr != nil {
		return nil, utils.Errorf("add channel address: %s", tcpErr)
	}

	network := make([]byte, 4)
	for i := 0; i < 4; i++ {
		network[i] = ip[i] & mask[i]
	}
	subnet, err := tcpip.NewSubnet(tcpip.AddrFrom4([4]byte(network)), tcpip.MaskFromBytes([]byte(mask)))
	if err != nil {
		return nil, utils.Errorf("channel subnet: %v", err)
	}
	stackIns.AddRoute(tcpip.Route{
		Destination: subnet,
		NIC:         nicID,
		MTU:         channelNetstackMTU,
	})

	vm := &NetStackVirtualMachineEntry{
		mtu:                  channelNetstackMTU,
		stack:                stackIns,
		config:               config,
		link:                 link,
		mainNICID:            nicID,
		dhcpStarted:          utils.NewAtomicBool(),
		dhcpSuccess:          utils.NewAtomicBool(),
		arpServiceStarted:    utils.NewAtomicBool(),
		arpPersistentMap:     new(sync.Map),
		arpPersistentTrigger: utils.NewAtomicBool(),
		mainNICIPv4Address:   append(net.IP(nil), ip...),
		mainNICIPv4Netmask: &net.IPNet{
			IP:   append(net.IP(nil), network...),
			Mask: mask,
		},
	}
	vm.dhcpSuccess.Set()
	return vm, nil
}

// BridgeChannelNetStacks forwards every packet between two channel-backed
// entries until ctx is cancelled. Use it for one-shot DialTCP. A stepped
// handshake must use StartTCPProbe instead; the two must not run together
// on the same entries.
func BridgeChannelNetStacks(ctx context.Context, a, b *NetStackVirtualMachineEntry) error {
	if ctx == nil {
		return fmt.Errorf("BridgeChannelNetStacks: nil context")
	}
	if a == nil || b == nil || a == b || a.link == nil || b.link == nil {
		return fmt.Errorf("BridgeChannelNetStacks: need two distinct channel-backed virtual machines")
	}
	go forwardChannel(ctx, a.link, b.link)
	go forwardChannel(ctx, b.link, a.link)
	return nil
}

func forwardChannel(ctx context.Context, src, dst *channel.Endpoint) {
	for {
		pkt := src.ReadContext(ctx)
		if pkt == nil {
			return
		}
		raw, err := packetBytes(pkt)
		if err != nil {
			continue
		}
		injectIPv4(dst, raw)
	}
}

func packetBytes(pkt *stack.PacketBuffer) ([]byte, error) {
	if pkt == nil {
		return nil, fmt.Errorf("nil packet")
	}
	defer pkt.DecRef()
	view := pkt.ToView()
	defer view.Release()
	raw := append([]byte(nil), view.AsSlice()...)
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty packet")
	}
	return raw, nil
}

func injectIPv4(ep *channel.Endpoint, raw []byte) {
	if ep == nil || len(raw) == 0 {
		return
	}
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buffer.MakeWithData(append([]byte(nil), raw...)),
	})
	defer pkt.DecRef()
	ep.InjectInbound(header.IPv4ProtocolNumber, pkt)
}
