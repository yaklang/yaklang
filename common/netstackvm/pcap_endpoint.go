package netstackvm

import (
	"bytes"
	"context"
	"net"
	"sync"

	"github.com/yaklang/yaklang/common/netx"
	"golang.org/x/net/ipv6"

	"github.com/davecgh/go-spew/spew"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/ipv4"
)

type PCAPEndpoint struct {
	*channel.Endpoint

	tcpKillMutex   sync.RWMutex
	tcpKillMap     map[string]struct{}
	inboundFilter  func(packet gopacket.Packet) bool
	outboundFilter func(packet gopacket.Packet) bool

	adaptor         *pcapAdaptor
	netBridge       *pcapBridge
	gatewayFound    *utils.AtomicBool
	gatewayHardware net.HardwareAddr
	gatewayIP       net.IP
	ctx             context.Context
	cancel          context.CancelFunc
	attachOnce      sync.Once
	wg              *sync.WaitGroup
	stack           *stack.Stack
	writeMutex      *sync.Mutex
	mtu             int

	// ethernet cache n arp cache
	ipToMac *sync.Map

	loopback     bool
	capabilities stack.LinkEndpointCapabilities
}

const defaultOutQueueLen = 1 << 10

func (p *PCAPEndpoint) SetPCAPInboundFilter(filter func(packet gopacket.Packet) bool) {
	p.inboundFilter = filter
}

func (p *PCAPEndpoint) SetPCAPOutboundFilter(filter func(packet gopacket.Packet) bool) {
	p.outboundFilter = filter
}

func (p *PCAPEndpoint) SetLoopback(b bool) {
	p.loopback = b
}

func NewPCAPEndpoint(ctx context.Context, stackIns *stack.Stack, device string, macAddr net.HardwareAddr, promisc bool) (*PCAPEndpoint, error) {
	iface, err := net.InterfaceByName(device)
	if err != nil {
		return nil, err
	}
	mtu := iface.MTU + 200
	if iface.Flags&net.FlagLoopback != 0 {
		mtu = 65535 // loopback mtu
	}

	internalMacAddr := macAddr
	externalMacAddr := iface.HardwareAddr
	bridge := &pcapBridge{internal: internalMacAddr, external: externalMacAddr}

	adaptor, err := NewPCAPAdaptor(device, int32(mtu), promisc)
	if err != nil {
		return nil, utils.Errorf("create pcap adaptor failed: %v", err)
	}

	//_ = handle.SetBPFFilter("dst mac " + macAddr.String())
	ctx, cancel := context.WithCancel(ctx)
	pcapEp := &PCAPEndpoint{
		//handle:               handle,
		tcpKillMap:   make(map[string]struct{}),
		adaptor:      adaptor,
		netBridge:    bridge,
		Endpoint:     channel.New(defaultOutQueueLen*100, uint32(mtu), tcpip.LinkAddress(string(macAddr))),
		stack:        stackIns,
		mtu:          mtu,
		ctx:          ctx,
		cancel:       cancel,
		wg:           new(sync.WaitGroup),
		ipToMac:      new(sync.Map),
		gatewayFound: utils.NewAtomicBool(),
		loopback:     iface.Flags&net.FlagLoopback != 0,
	}
	return pcapEp, nil
}

func (p *PCAPEndpoint) DisallowTCP(addr string) {
	hashes := p.generateKillTCPHash(addr, "")
	p.tcpKillMutex.Lock()
	defer p.tcpKillMutex.Unlock()
	for _, hash := range hashes {
		p.tcpKillMap[hash] = struct{}{}
	}
}

func (p *PCAPEndpoint) AllowTCP(addr string) {
	hashes := p.generateKillTCPHash(addr, "")
	p.tcpKillMutex.Lock()
	defer p.tcpKillMutex.Unlock()
	for _, hash := range hashes {
		delete(p.tcpKillMap, hash)
	}
}

func (p *PCAPEndpoint) DisallowTCPWithSrc(addr string, src string) {
	hashes := p.generateKillTCPHash(addr, src)
	p.tcpKillMutex.Lock()
	defer p.tcpKillMutex.Unlock()
	for _, hash := range hashes {
		p.tcpKillMap[hash] = struct{}{}
	}
}

func (p *PCAPEndpoint) AllowTCPWithSrc(addr string, src string) {
	hashes := p.generateKillTCPHash(addr, src)
	p.tcpKillMutex.Lock()
	defer p.tcpKillMutex.Unlock()
	for _, hash := range hashes {
		delete(p.tcpKillMap, hash)
	}
}

func (p *PCAPEndpoint) generateKillTCPHash(to, from string) []string {
	var fromHosts []string
	var toHosts []string

	if from != "" {
		fromHost, fromPort, _ := utils.ParseStringToHostPort(from)
		if fromPort <= 0 {
			fromHost = from
		}
		fromIp := net.ParseIP(fromHost)
		if fromIp != nil {
			fromHosts = append(fromHosts, fromIp.String())
		} else {
			ips := netx.LookupAll(fromHost)
			for _, ip := range ips {
				fromHosts = append(fromHosts, ip)
			}
		}
	}

	toHost, toPort, _ := utils.ParseStringToHostPort(to)
	if toPort <= 0 {
		toHost = to
	}

	toIp := net.ParseIP(toHost)
	if toIp != nil {
		toHosts = append(toHosts, toIp.String())
	} else {
		ips := netx.LookupAll(toHost)
		toHosts = append(toHosts, ips...)
	}

	var newAddrs []string
	for _, toAddr := range toHosts {
		target := toAddr
		if toPort > 0 {
			target = utils.HostPort(toAddr, toPort)
		}

		if len(fromHosts) > 0 {
			for _, fromAddr := range fromHosts {
				src := fromAddr
				if toPort > 0 {
					src = utils.HostPort(fromAddr, toPort)
				}
				newAddr := utils.CalcSha256(target, src)
				newAddrs = append(newAddrs, newAddr)
			}
		} else {
			newAddr := utils.CalcSha256(target)
			newAddrs = append(newAddrs, newAddr)
		}
	}
	return newAddrs
}

func (p *PCAPEndpoint) SetGatewayHardwareAddr(hwAddr net.HardwareAddr) {
	p.gatewayHardware = hwAddr
	p.gatewayFound.Set()
}

func (p *PCAPEndpoint) SetGatewayIP(g net.IP) {
	p.gatewayIP = g
	if p.gatewayHardware == nil {
		macaddr, ok := p.ipToMac.Load(g.String())
		if ok {
			// log.Infof("auto set gateway hardware addr: %s -> %s", g.String(), macaddr.(net.HardwareAddr).String())
			p.gatewayHardware, _ = macaddr.(net.HardwareAddr)
		}
	}
	p.gatewayFound.Set()
}

func (p *PCAPEndpoint) Close() {
	p.cancel()
	p.adaptor.Close()
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

	packetChan := p.adaptor.PacketSource()
	if packetChan == nil {
		log.Errorf("failed to get packet source: nil packet source")
		return
	}

	defer func() {
		log.Info("inboundLoop exit")
	}()

	log.Infof("start to execute inbound loop with mtu: %v", mtu)
	var packet gopacket.Packet
	var ok bool
	for {
		select {
		case <-ctx.Done():
			return
		case packet, ok = <-packetChan:
			if !ok {
				log.Errorf("failed to get packet from packet source: %v", ok)
				return
			}
		}

		if !p.IsAttached() {
			continue
		}

		if dropped, _ := p.generateRSTFromPacket(packet); dropped {
			continue
		}

		if p.inboundFilter != nil && !p.inboundFilter(packet) {
			continue
		}

		var srcMac net.HardwareAddr
		var dstMac net.HardwareAddr
		data := packet.Data()
		offset := 0
		if p.loopback {
			loopbackLayer := packet.Layer(layers.LayerTypeLoopback)
			if loopbackLayer != nil {
				offset = len(loopbackLayer.LayerContents())
			}
		} else {
			linkLayer := packet.LinkLayer()
			if linkLayer != nil {
				offset = len(linkLayer.LayerContents())
				switch eth := linkLayer.(type) {
				case *layers.Ethernet:
					eth = p.netBridge.handleInbound(eth)
					srcMac = eth.SrcMAC
					dstMac = eth.DstMAC
					_ = dstMac
				}
			}
		}

		// 检查数据是否有效
		if len(data) < offset {
			log.Errorf("invalid packet data: offset %d exceeds data length %d", offset, len(data))
			continue
		}
		networkPayloads := data[offset:]

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
						//log.Infof("remember ip to mac: %s -> %s", ipString, net.HardwareAddr(arpPacket.SourceHwAddress).String())
						p.ipToMac.Store(ipString, net.HardwareAddr(arpPacket.SourceHwAddress))
					}
				}
				p.InjectInbound(header.ARPProtocolNumber, pkt)
			} else {
				log.Infof("recv non network layer packet: \n%s", spew.Sdump(data))
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

func (p *PCAPEndpoint) outboundLoop(ctx context.Context) {
	for {
		pkt := p.ReadContext(ctx)
		if pkt == nil {
			log.Infof("outboundLoop exit")
			break
		}
		if !p.IsAttached() {
			continue
		}
		err := p.writePacket(pkt)
		if err != nil {
			log.Errorf("failed to write packet (PCAPEndpoint): %v", err)
		}
	}
}

func (p *PCAPEndpoint) fallbackDefaultMac() net.HardwareAddr {
	if p.gatewayFound.IsSet() {
		return p.gatewayHardware
	}
	return net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
}

func (p *PCAPEndpoint) writePacket(pkt *stack.PacketBuffer) error {
	defer pkt.DecRef()

	buf := pkt.ToBuffer()
	defer buf.Release()

	var payloads = buf.Flatten()
	var linkLayerType gopacket.LayerType
	var err error
	if p.loopback {
		payloads, linkLayerType, err = p.encapsulatePayloadLoopback(payloads)
		if err != nil {
			return err
		}
	} else {
		payloads, linkLayerType, err = p.encapsulatePayload(payloads)
		if err != nil {
			return err
		}
	}

	if p.outboundFilter != nil {
		packet := gopacket.NewPacket(payloads, linkLayerType, gopacket.Default)
		if !p.outboundFilter(packet) {
			return nil
		}
	}

	if err := p.adaptor.WritePacketData(payloads); err != nil {
		return utils.Errorf("adaptor.WritePacketData in PCAPEndpoint failed: %v", err)
	}
	return nil
}

func (p *PCAPEndpoint) encapsulatePayload(payloads []byte) ([]byte, gopacket.LayerType, error) {
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

	var eth *layers.Ethernet
	var isArp bool
	switch ret := header.IPVersion(payloads); ret {
	case header.IPv4Version:
		if v4header, err := ipv4.ParseHeader(payloads); err == nil {
			eth = getDefaultEthernetByDest(layers.EthernetTypeIPv4, v4header.Dst.String(), false)
		} else {
			return nil, layers.LayerTypeEthernet, utils.Errorf("failed to parse ipv4 header: %v", err)
		}
	case header.IPv6Version:
		if v6header, err := ipv6.ParseHeader(payloads); err == nil {
			eth = getDefaultEthernetByDest(layers.EthernetTypeIPv6, v6header.Dst.String(), false)
		} else {
			return nil, layers.LayerTypeEthernet, utils.Errorf("failed to parse ipv6 header: %v", err)
		}
	default:
		if arpHeader := header.ARP(payloads); arpHeader != nil && arpHeader.IsValid() {
			arpHeader = p.netBridge.handleOutboundARP(arpHeader)
			payloads = arpHeader
			if arpHeader.Op() == header.ARPReply {
				eth = &layers.Ethernet{
					SrcMAC:       net.HardwareAddr(p.LinkAddress()),
					DstMAC:       arpHeader.HardwareAddressTarget(),
					EthernetType: layers.EthernetTypeARP,
				}
			} else {
				eth = getDefaultEthernetByDest(layers.EthernetTypeARP, "", true)
			}
			isArp = true
		}
	}
	if eth != nil {
		eth = p.netBridge.handleOutbound(eth)
		if !isArp {
			buf := gopacket.NewSerializeBuffer()
			opts := gopacket.SerializeOptions{
				FixLengths:       true,
				ComputeChecksums: true,
			}
			err := gopacket.SerializeLayers(buf, opts, eth, gopacket.Payload(payloads))
			if err != nil {
				return nil, layers.LayerTypeEthernet, utils.Errorf("failed to serialize layers: %v", err)
			}
			payloads = buf.Bytes()
		} else {
			ethBytes := make([]byte, 0, 14)
			ethBytes = append(ethBytes, eth.DstMAC...)
			ethBytes = append(ethBytes, eth.SrcMAC...)
			ethBytes = append(ethBytes, 0x08, 0x06)
			newPayloads := make([]byte, 0, len(payloads)+14)
			newPayloads = append(newPayloads, ethBytes...)
			newPayloads = append(newPayloads, payloads...)
			payloads = newPayloads
		}
	}
	return payloads, layers.LayerTypeEthernet, nil
}

func (p *PCAPEndpoint) encapsulatePayloadLoopback(payloads []byte) ([]byte, gopacket.LayerType, error) {
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	err := gopacket.SerializeLayers(buf, opts,
		&layers.Loopback{
			//TODO loopback ipv6 handle
			Family: layers.ProtocolFamilyIPv4,
		},
		gopacket.Payload(payloads))
	if err != nil {
		return nil, layers.LayerTypeLoopback, utils.Errorf("failed to serialize layers: %v", err)
	}
	return buf.Bytes(), layers.LayerTypeLoopback, nil
}

func (p *PCAPEndpoint) Capabilities() stack.LinkEndpointCapabilities {
	return p.capabilities
}

func (p *PCAPEndpoint) SetCapabilities(flag stack.LinkEndpointCapabilities) {
	p.capabilities = flag
}
