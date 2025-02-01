package netstackvm

import (
	"bytes"
	"context"
	"fmt"
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
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

type pcapEpIf interface {
	io.ReadWriter
	Close()
}

var _ pcapEpIf = (*PCAPEndpoint)(nil)

type PCAPEndpoint struct {
	*channel.Endpoint

	overrideSrcHardwareAddr net.HardwareAddr
	getawayFound            *utils.AtomicBool
	getawayHardware         net.HardwareAddr
	getawayIP               net.IP

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

	//_ = handle.SetBPFFilter("dst mac " + macAddr.String())
	ctx, cancel := context.WithCancel(ctx)
	pcapEp := &PCAPEndpoint{
		Endpoint:             channel.New(defaultOutQueueLen*100, uint32(mtu), tcpip.LinkAddress(string(macAddr))),
		stack:                stackIns,
		handle:               handle,
		mtu:                  mtu,
		writeMutex:           new(sync.Mutex),
		pcapPacketHandleOnce: sync.Once{},
		pcapPacket:           gopacket.NewPacketSource(handle, handle.LinkType()).Packets(),
		ctx:                  ctx,
		cancel:               cancel,
		wg:                   new(sync.WaitGroup),
		ipToMac:              new(sync.Map),
		getawayFound:         utils.NewAtomicBool(),
	}
	return pcapEp, nil
}

func (p *PCAPEndpoint) SetOverrideSrcHardwareAddr(hwAddr net.HardwareAddr) {
	p.overrideSrcHardwareAddr = hwAddr
}

func (p *PCAPEndpoint) SetGatewayHardwareAddr(hwAddr net.HardwareAddr) {
	p.getawayHardware = hwAddr
	p.getawayFound.Set()
}

func (p *PCAPEndpoint) SetGatewayIP(g net.IP) {
	p.getawayIP = g
	if p.getawayHardware == nil {
		macaddr, ok := p.ipToMac.Load(g.String())
		if ok {
			// log.Infof("auto set gateway hardware addr: %s -> %s", g.String(), macaddr.(net.HardwareAddr).String())
			p.getawayHardware, _ = macaddr.(net.HardwareAddr)
		}
	}
	p.getawayFound.Set()
}

func (p *PCAPEndpoint) Read(packet []byte) (n int, err error) {
	if p.pcapPacket == nil {
		return 0, fmt.Errorf("pcapPacket is nil")
	}
	select {
	case <-p.ctx.Done():
		return 0, fmt.Errorf("pcapPacket is closed")
	case pkt, ok := <-p.pcapPacket:
		if !ok {
			log.Infof("pcapPacket is closed")
			return 0, fmt.Errorf("pcapPacket is closed")
		}
		n := copy(packet, pkt.Data())
		return n, nil
	}
}

func (p *PCAPEndpoint) Write(packet []byte) (n int, err error) {
	//p.writeMutex.Lock()
	//defer p.writeMutex.Unlock()
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
		log.Info("start to attach pcap endpoint outbound loop and inboundloop")
		p.ctx, p.cancel = context.WithCancel(p.ctx)
		p.wg.Add(2)
		go func() {
			defer func() {
				log.Infof("cancel outbound loop")
				p.cancel()
				p.wg.Done()
			}()
			p.outboundLoop(p.ctx)
		}()
		go func() {
			defer func() {
				log.Infof("cancel inbound loop")
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

	defer func() {
		log.Info("inboundLoop exit")
	}()

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
		addInboundPacket()

		if !p.IsAttached() {
			continue
		}

		dataWithLink := data[:n]

		packet := gopacket.NewPacket(dataWithLink, layers.LinkTypeEthernet, gopacket.DecodeOptions{
			NoCopy: true,
			Lazy:   true,
		})

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
		networkPayloads := data[offset:n]

		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(networkPayloads),
		})
		defer func() {
			pkt.DecRef()
		}()

		networklayer := packet.NetworkLayer()
		if networklayer == nil {
			// arp
			arpLayer := packet.Layer(layers.LayerTypeARP)
			if arpLayer != nil {
				arpPacket, ok := arpLayer.(*layers.ARP)
				if !ok {
					continue
				}
				if ok && len(arpPacket.SourceHwAddress) == 6 && !bytes.Equal(arpPacket.SourceHwAddress, []byte{0, 0, 0, 0, 0, 0}) && !bytes.Equal(arpPacket.SourceHwAddress, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}) {
					ipString := net.IP(arpPacket.SourceProtAddress).String()
					_, ok := p.ipToMac.Load(ipString)
					if !ok {
						log.Infof("remember ip to mac: %s -> %s", ipString, net.HardwareAddr(arpPacket.SourceHwAddress).String())
						p.ipToMac.Store(ipString, arpPacket.SourceHwAddress)
					}
				}
				p.InjectInbound(header.ARPProtocolNumber, pkt)
			} else {
				log.Infof("recv non network layer packet: \n%s", packet.Dump())
			}
		} else {
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
					if len(srcMac) == 6 && !bytes.Equal(srcMac, []byte{0, 0, 0, 0, 0, 0}) && !bytes.Equal(srcMac, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}) {
						//log.Infof("remember ip to mac: %s -> %s", srcIp.String(), srcMac.String())
						p.ipToMac.Store(srcIp.String(), srcMac)
					}
				}
				p.InjectInbound(header.IPv4ProtocolNumber, pkt)
			case layers.LayerTypeIPv6:
				p.InjectInbound(header.IPv6ProtocolNumber, pkt)
			default:
				log.Errorf("unknown network layer type: %s", networklayer.LayerType())
			}
		}
	}
}

var inboundPacket = new(int64)
var outboundPacket = new(int64)

func addInboundPacket() {
	atomic.AddInt64(inboundPacket, 1)
	//log.Infof("inbound packet: %d", atomic.LoadInt64(inboundPacket))
}

func addOutboundPacket() {
	atomic.AddInt64(outboundPacket, 1)
	//log.Infof("outbound packet: %d", atomic.LoadInt64(outboundPacket))
}

func (p *PCAPEndpoint) outboundLoop(ctx context.Context) {
	for {
		pkt := p.ReadContext(ctx)
		if pkt == nil {
			log.Infof("outboundLoop exit")
			break
		}
		addOutboundPacket()
		if !p.IsAttached() {
			continue
		}
		p.writePacket(pkt)
	}
}

func (p *PCAPEndpoint) fallbackDefaultMac() net.HardwareAddr {
	if p.getawayFound.IsSet() {
		return p.getawayHardware
	}
	return net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
}

func (p *PCAPEndpoint) writePacket(pkt *stack.PacketBuffer) tcpip.Error {
	defer pkt.DecRef()

	buf := pkt.ToBuffer()
	defer buf.Release()

	payloads := buf.Flatten()

	getDefaultEthernetByDest := func(nextLayers layers.EthernetType, dst string, broadcast bool) *layers.Ethernet {
		var dstMac net.HardwareAddr
		if !broadcast {
			if dst == "" {
				dstMac = p.fallbackDefaultMac()
			} else {
				macAddr, existed := p.ipToMac.Load(dst)
				if existed {
					dstMac = macAddr.(net.HardwareAddr)
				} else {
					dstMac = p.fallbackDefaultMac()
				}
			}
		} else {
			dstMac = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		}
		return &layers.Ethernet{
			SrcMAC:       net.HardwareAddr(p.LinkAddress()),
			DstMAC:       dstMac,
			EthernetType: nextLayers,
		}
	}

	isArp := false
	var eth *layers.Ethernet
	switch ret := header.IPVersion(payloads); ret {
	case header.IPv4Version:
		if v4header, err := ipv4.ParseHeader(payloads); err == nil {
			eth = getDefaultEthernetByDest(layers.EthernetTypeIPv4, v4header.Dst.String(), false)
		} else {
			log.Errorf("failed to parse ipv4 header: %s", err)
		}
	case header.IPv6Version:
		if v6header, err := ipv6.ParseHeader(payloads); err == nil {
			eth = getDefaultEthernetByDest(layers.EthernetTypeIPv6, v6header.Dst.String(), false)
		} else {
			log.Errorf("failed to parse ipv6 header: %s", err)
		}
	default:
		if arpHeader := header.ARP(payloads); arpHeader != nil && arpHeader.IsValid() {
			if net.IP(arpHeader.ProtocolAddressSender()).String() == p.getawayIP.String() {
				return nil
			}
			eth = getDefaultEthernetByDest(layers.EthernetTypeARP, "", true)
			isArp = true
		}
	}

	if eth != nil {
		buf := gopacket.NewSerializeBuffer()
		opts := gopacket.SerializeOptions{
			FixLengths:       true,
			ComputeChecksums: true,
		}
		if p.overrideSrcHardwareAddr != nil {
			eth.SrcMAC = p.overrideSrcHardwareAddr
		}

		_ = isArp
		//if isArp {
		//	log.Infof("s")
		//}
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
