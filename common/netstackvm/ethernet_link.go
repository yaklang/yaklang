package netstackvm

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
)

// EthernetLink adapts a frame-oriented device (e.g. TAP) to a bridge-mode VM.
// Each Read/Write must transfer one complete Ethernet frame; a stream socket
// needs framing before it can be used here. Close must unblock Read and Write.
// It preserves the VM MAC, performs ARP/neighbor resolution through gVisor and
// never rewrites frames to the host's MAC or modifies host interface settings.
type EthernetLink struct {
	*channel.Endpoint
	device    io.ReadWriteCloser
	ctx       context.Context
	cancel    context.CancelFunc
	start     sync.Once
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func NewEthernetLink(ctx context.Context, device io.ReadWriteCloser, mac net.HardwareAddr, mtu uint32) (*EthernetLink, error) {
	if ctx == nil || device == nil {
		return nil, errors.New("Ethernet link requires context and device")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !header.IsValidUnicastEthernetAddress(tcpip.LinkAddress(mac)) {
		return nil, errors.New("Ethernet link requires a unicast MAC")
	}
	if mtu < 1280 || mtu > 65535 {
		return nil, errors.New("Ethernet MTU must be between 1280 and 65535")
	}
	ctx, cancel := context.WithCancel(ctx)
	e := &EthernetLink{Endpoint: channel.New(1024, mtu, tcpip.LinkAddress(mac)), ctx: ctx, cancel: cancel, device: device}
	e.LinkEPCapabilities = stack.CapabilityResolutionRequired
	// Closing the device interrupts blocked I/O even if attachment never happens.
	context.AfterFunc(ctx, e.Close)
	return e, nil
}
func (e *EthernetLink) MaxHeaderLength() uint16                 { return header.EthernetMinimumSize }
func (e *EthernetLink) ARPHardwareType() header.ARPHardwareType { return header.ARPHardwareEther }
func (e *EthernetLink) AddHeader(pkt *stack.PacketBuffer) {
	header.Ethernet(pkt.LinkHeader().Push(header.EthernetMinimumSize)).Encode(&header.EthernetFields{SrcAddr: e.LinkAddress(), DstAddr: pkt.EgressRoute.RemoteLinkAddress, Type: pkt.NetworkProtocolNumber})
}
func (e *EthernetLink) ParseHeader(pkt *stack.PacketBuffer) bool {
	_, ok := pkt.LinkHeader().Consume(header.EthernetMinimumSize)
	return ok
}
func (e *EthernetLink) Attach(d stack.NetworkDispatcher) {
	e.Endpoint.Attach(d)
	if d == nil {
		return
	}
	e.start.Do(func() {
		e.wg.Add(2)
		go func() { defer e.wg.Done(); defer e.Close(); e.readFrames() }()
		go func() { defer e.wg.Done(); defer e.Close(); e.writeFrames() }()
	})
}
func (e *EthernetLink) readFrames() {
	raw := make([]byte, 65535+header.EthernetMinimumSize)
	for {
		n, err := e.device.Read(raw)
		if err != nil {
			return
		}
		if n < header.EthernetMinimumSize {
			continue
		}
		eth := header.Ethernet(raw[:n])
		dst := eth.DestinationAddress()
		if dst != e.LinkAddress() && !header.IsMulticastEthernetAddress(dst) {
			continue
		}
		protocol := eth.Type()
		if protocol != header.IPv4ProtocolNumber && protocol != header.IPv6ProtocolNumber && protocol != header.ARPProtocolNumber {
			continue
		}
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(append([]byte(nil), raw[:n]...))})
		e.ParseHeader(pkt)
		e.InjectInbound(protocol, pkt)
		pkt.DecRef()
	}
}
func (e *EthernetLink) writeFrames() {
	for {
		pkt := e.ReadContext(e.ctx)
		if pkt == nil {
			return
		}
		raw, err := packetBytes(pkt)
		if err != nil {
			continue
		}
		n, err := e.device.Write(raw)
		if err != nil || n != len(raw) {
			return
		}
	}
}
func (e *EthernetLink) Close() {
	e.closeOnce.Do(func() { e.cancel(); e.Endpoint.Close(); e.device.Close() })
}
func (e *EthernetLink) Wait() { e.wg.Wait() }
