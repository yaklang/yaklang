package netstackvm

import (
	"context"
	"fmt"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"io"
	"net"
	"sync"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type pcapEpIf interface {
	io.ReadWriter
	Close()
}

var _ pcapEpIf = (*PCAPEndpoint)(nil)

type PCAPEndpoint struct {
	*channel.Endpoint

	ctx                  context.Context
	cancel               context.CancelFunc
	attachOnce           sync.Once
	wg                   *sync.WaitGroup
	stack                *stack.Stack
	handle               *pcap.Handle
	writeMutex           *sync.Mutex
	pcapPacketHandleOnce sync.Once
	pcapPacket           chan gopacket.Packet
	mtu                  int

	// ethernet cache n arp cache
	ipToMac *sync.Map
}

const defaultOutQueueLen = 1 << 10

func NewPCAPEndpoint(ctx context.Context, stackIns *stack.Stack, promisc bool, device string, macAddr net.HardwareAddr) (*PCAPEndpoint, error) {
	pcapName, err := pcaputil.IfaceNameToPcapIfaceName(device)
	if err != nil {
		return nil, err
	}
	handle, err := pcap.OpenLive(pcapName, 1600, promisc, pcap.BlockForever)
	if err != nil {
		return nil, err
	}

	iface, err := net.InterfaceByName(device)
	if err != nil {
		return nil, err
	}
	mtu := iface.MTU

	ctx, cancel := context.WithCancel(ctx)
	pcapEp := &PCAPEndpoint{
		Endpoint:             channel.New(defaultOutQueueLen, uint32(mtu), tcpip.LinkAddress(string(macAddr))),
		stack:                stackIns,
		handle:               handle,
		mtu:                  mtu,
		writeMutex:           new(sync.Mutex),
		pcapPacketHandleOnce: sync.Once{},
		pcapPacket:           make(chan gopacket.Packet, 1024),
		ctx:                  ctx,
		cancel:               cancel,
		wg:                   new(sync.WaitGroup),
		ipToMac:              new(sync.Map),
	}
	packetChan := gopacket.NewPacketSource(handle, handle.LinkType()).Packets()

	pcapEp.pcapPacketHandleOnce.Do(func() {
		go func() {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("failed to handle pcap packet: %s", err)
				}
			}()
			defer func() {
				close(pcapEp.pcapPacket)
			}()
			for packet := range packetChan {
				pcapEp.pcapPacket <- packet
			}
		}()
	})
	return pcapEp, nil
}

func (p *PCAPEndpoint) Read(packet []byte) (n int, err error) {
	if p.pcapPacket == nil {
		return 0, fmt.Errorf("pcapPacket is nil")
	}
	pkt, ok := <-p.pcapPacket
	if !ok {
		return 0, fmt.Errorf("pcapPacket is closed")
	}
	return copy(packet, pkt.Data()), nil
}

func (p *PCAPEndpoint) Write(packet []byte) (n int, err error) {
	p.writeMutex.Lock()
	defer p.writeMutex.Unlock()
	err = p.handle.WritePacketData(packet)
	if err != nil {
		return 0, err
	}
	return len(packet), nil
}

func (p *PCAPEndpoint) Close() {
	p.cancel()
	p.handle.Close()
}

func (p *PCAPEndpoint) Attach(dispatcher stack.NetworkDispatcher) {
	p.Endpoint.Attach(dispatcher)
	p.attachOnce.Do(func() {
		p.ctx, p.cancel = context.WithCancel(p.ctx)
		p.wg.Add(2)
		go func() {
			defer func() {
				p.cancel()
				p.wg.Done()
			}()
			p.outboundLoop(p.ctx)
		}()
		go func() {
			defer func() {
				p.cancel()
				p.wg.Done()
			}()
			p.inboundLoop(p.ctx)
		}()
	})
}

func (p *PCAPEndpoint) Wait() {
	p.wg.Wait()
}

func (p *PCAPEndpoint) inboundLoop(ctx context.Context) {
	mtu := p.mtu

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		data := make([]byte, mtu)
		n, err := p.Read(data)
		if err != nil {
			log.Errorf("failed to read from pcap: %s", err)
			return
		}
		if n == 0 || n > mtu {
			continue
		}

		if !p.IsAttached() {
			continue
		}

		dataWithLink := data[:n]
		packet := gopacket.NewPacket(dataWithLink, layers.LinkTypeEthernet, gopacket.DecodeOptions{
			NoCopy: true,
			Lazy:   true,
		})

		networklayer := packet.NetworkLayer()
		if networklayer == nil {
			continue
		}

		linkLayer := packet.LinkLayer()
		offset := 0
		var srcMac net.HardwareAddr
		var dstMac net.HardwareAddr
		if linkLayer != nil {
			offset = len(linkLayer.LayerContents())
			switch ret := linkLayer.(type) {
			case *layers.Ethernet:
				srcMac = ret.SrcMAC
				dstMac = ret.DstMAC
				_ = dstMac
			}
		}

		//if dhcp4 := packet.Layer(layers.LayerTypeDHCPv4); dhcp4 != nil {
		//	if dhcp4ins, ok := dhcp4.(*layers.DHCPv4); ok {
		//	}
		//}

		networkPayloads := data[offset:n]
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(networkPayloads),
		})
		switch networklayer.LayerType() {
		case layers.LayerTypeIPv4:
			var srcIp net.IP
			var dstIp net.IP
			if v4header, err := ipv4.ParseHeader(networkPayloads); err == nil {
				srcIp = v4header.Src
				dstIp = v4header.Dst
				_ = dstIp
			}
			if !srcIp.IsUnspecified() {
				//log.Infof("remember ip to mac: %s -> %s", srcIp.String(), srcMac.String())
				p.ipToMac.Store(srcIp.String(), srcMac)
			}
			p.InjectInbound(header.IPv4ProtocolNumber, pkt)
		case layers.LayerTypeIPv6:
			p.InjectInbound(header.IPv6ProtocolNumber, pkt)
		case layers.LayerTypeARP:
			p.InjectInbound(header.ARPProtocolNumber, pkt)
		default:
			log.Errorf("unknown network layer type: %s", networklayer.LayerType())
		}
	}
}

func (p *PCAPEndpoint) outboundLoop(ctx context.Context) {
	for {
		pkt := p.ReadContext(ctx)
		if pkt == nil {
			break
		}
		if !p.IsAttached() {
			continue
		}
		p.writePacket(pkt)
	}
}

func (p *PCAPEndpoint) writePacket(pkt *stack.PacketBuffer) tcpip.Error {
	defer pkt.DecRef()

	buf := pkt.ToBuffer()
	defer buf.Release()

	payloads := buf.Flatten()

	getDefaultEthernetByDest := func(nextLayers layers.EthernetType, dst string) *layers.Ethernet {
		var dstMac net.HardwareAddr
		if dst == "" {
			dstMac = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		} else {
			macAddr, existed := p.ipToMac.Load(dst)
			if existed {
				dstMac = macAddr.(net.HardwareAddr)
			} else {
				dstMac = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
			}
		}
		return &layers.Ethernet{
			SrcMAC:       net.HardwareAddr(p.LinkAddress()),
			DstMAC:       dstMac,
			EthernetType: nextLayers,
		}
	}

	var eth *layers.Ethernet
	switch ret := header.IPVersion(payloads); ret {
	case header.IPv4Version:
		if v4header, err := ipv4.ParseHeader(payloads); err == nil {
			eth = getDefaultEthernetByDest(layers.EthernetTypeIPv4, v4header.Dst.String())
		} else {
			log.Errorf("failed to parse ipv4 header: %s", err)
		}
	case header.IPv6Version:
		if v6header, err := ipv6.ParseHeader(payloads); err == nil {
			eth = getDefaultEthernetByDest(layers.EthernetTypeIPv6, v6header.Dst.String())
		} else {
			log.Errorf("failed to parse ipv6 header: %s", err)
		}
	default:
		if arpHeader := header.ARP(payloads); arpHeader != nil && arpHeader.IsValid() {
			eth = getDefaultEthernetByDest(layers.EthernetTypeARP, "")
		}
	}

	if eth != nil {
		buf := gopacket.NewSerializeBuffer()
		opts := gopacket.SerializeOptions{
			FixLengths:       true,
			ComputeChecksums: true,
		}
		err := gopacket.SerializeLayers(buf, opts, eth, gopacket.Payload(payloads))
		if err != nil {
			log.Warnf("failed to serialize layers: %s", err)
			return &tcpip.ErrInvalidEndpointState{}
		}
		payloads = buf.Bytes()
	}
	if _, err := p.Write(payloads); err != nil {
		return &tcpip.ErrInvalidEndpointState{}
	}
	return nil
}

func (p *PCAPEndpoint) Capabilities() stack.LinkEndpointCapabilities {
	return stack.CapabilityResolutionRequired
}
