package rwendpoint

import (
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"

	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/log"
)

type PcapReadWriteCloser struct {
	handle     *pcap.Handle
	writeMutex sync.Mutex
	mtu        int

	ip4address          netip.Addr
	gatewayIp4address   netip.Addr
	deviceHardwareAddr  net.HardwareAddr
	gatewayHardwareAddr net.HardwareAddr
	packetChan          chan gopacket.Packet
}

func (p *PcapReadWriteCloser) GetIP4Address() netip.Addr {
	return p.ip4address
}

func (p *PcapReadWriteCloser) GetGatewayIP4Address() netip.Addr {
	return p.gatewayIp4address
}

func (p *PcapReadWriteCloser) GetDeviceHardwareAddr() net.HardwareAddr {
	return p.deviceHardwareAddr
}

func (p *PcapReadWriteCloser) GetGatewayHardwareAddr() net.HardwareAddr {
	return p.gatewayHardwareAddr
}

func NewPcapReadWriteCloser(device string, snaplen int32) (*PcapReadWriteCloser, error) {
	handle, err := pcap.OpenLive(device, snaplen, true, pcap.BlockForever)
	if err != nil {
		return nil, fmt.Errorf("failed to open pcap device %s: %v", device, err)
	}
	log.Infof("Successfully opened pcap device: %s", device)

	// Get MTU from device
	iface, err := net.InterfaceByName(device)
	if err != nil {
		handle.Close()
		return nil, fmt.Errorf("failed to get interface %s: %v", device, err)
	}
	log.Infof("Found network interface: %s with MTU: %d", device, iface.MTU)

	// 获取设备IP地址
	addrs, err := iface.Addrs()
	if err != nil {
		handle.Close()
		return nil, fmt.Errorf("failed to get addresses for interface %s: %v", device, err)
	}

	// 查找第一个IPv4地址
	var ip4addr netip.Addr
	var gatewayIP netip.Addr
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				ip4addr = netip.MustParseAddr(ip4.String())
				// 计算网关地址 - 使用网段的第一个可用地址作为网关
				ones, bits := ipnet.Mask.Size()
				if ones > 0 && ones < bits {
					network := ip4.Mask(ipnet.Mask)
					gateway := make(net.IP, len(network))
					copy(gateway, network)
					gateway[len(gateway)-1] = gateway[len(gateway)-1] + 1
					gatewayIP = netip.MustParseAddr(gateway.String())
				}
				log.Infof("Found IPv4 address: %s with gateway: %s", ip4addr, gatewayIP)
				break
			}
		}
	}

	if !ip4addr.IsValid() {
		handle.Close()
		return nil, fmt.Errorf("no IPv4 address found for interface %s", device)
	}

	// 获取设备硬件地址和网关硬件地址
	p := &PcapReadWriteCloser{
		handle:             handle,
		mtu:                iface.MTU,
		deviceHardwareAddr: iface.HardwareAddr,
		ip4address:         ip4addr,
		gatewayIp4address:  gatewayIP,
	}
	log.Infof("Device hardware address: %s", p.deviceHardwareAddr)

	// dhcp a new ip

	// 通过ARP获取网关硬件地址
	err = handle.SetBPFFilter("arp")
	if err != nil {
		handle.Close()
		return nil, fmt.Errorf("failed to set BPF filter: %v", err)
	}
	log.Infof("Set BPF filter for ARP packets")

	// 构造ARP请求包
	arpRequest := []byte{
		// 目标MAC(广播): ff:ff:ff:ff:ff:ff
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		// 源MAC
		p.deviceHardwareAddr[0], p.deviceHardwareAddr[1], p.deviceHardwareAddr[2],
		p.deviceHardwareAddr[3], p.deviceHardwareAddr[4], p.deviceHardwareAddr[5],
		// 类型: ARP(0x0806)
		0x08, 0x06,
		// 硬件类型:以太网(1)
		0x00, 0x01,
		// 协议类型:IP(0x0800)
		0x08, 0x00,
		// 硬件地址长度:6
		0x06,
		// 协议地址长度:4
		0x04,
		// 操作:ARP请求(1)
		0x00, 0x01,
		// 发送端MAC地址
		p.deviceHardwareAddr[0], p.deviceHardwareAddr[1], p.deviceHardwareAddr[2],
		p.deviceHardwareAddr[3], p.deviceHardwareAddr[4], p.deviceHardwareAddr[5],
		// 发送端IP地址(本机IP)
		p.ip4address.As4()[0], p.ip4address.As4()[1], p.ip4address.As4()[2], p.ip4address.As4()[3],
		// 目标MAC地址(全0)
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// 目标IP地址(网关IP)
		p.gatewayIp4address.As4()[0], p.gatewayIp4address.As4()[1], p.gatewayIp4address.As4()[2], p.gatewayIp4address.As4()[3],
	}
	// 设置5秒超时和0.5秒的发送间隔
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packetChan := packetSource.Packets()

	// 先发送第一个ARP请求
	err = handle.WritePacketData(arpRequest)
	if err != nil {
		handle.Close()
		return nil, fmt.Errorf("failed to send ARP request: %v", err)
	}
	log.Infof("Sent initial ARP request to gateway: %s", p.gatewayIp4address)

	// 等待接收ARP响应,同时每0.5秒发送一次请求
ARPLOOP:
	for {
		select {
		case packet := <-packetChan:
			if packet == nil {
				continue
			}

			arpLayer := packet.Layer(layers.LayerTypeARP)
			if arpLayer == nil {
				continue
			}

			arp := arpLayer.(*layers.ARP)
			if arp.Operation != layers.ARPReply {
				continue
			}

			// 检查是否是网关的响应
			if net.IP(arp.SourceProtAddress).Equal(net.IP(p.gatewayIp4address.AsSlice())) {
				p.gatewayHardwareAddr = net.HardwareAddr(arp.SourceHwAddress)
				log.Infof("Received ARP reply from gateway, MAC address: %s", p.gatewayHardwareAddr)
				break ARPLOOP
			}

		case <-ticker.C:
			// 每0.5秒发送一次ARP请求
			err = handle.WritePacketData(arpRequest)
			if err != nil {
				handle.Close()
				return nil, fmt.Errorf("failed to send ARP request: %v", err)
			}
			log.Debugf("Sent ARP request to gateway")

		case <-timeout:
			handle.Close()
			return nil, fmt.Errorf("timeout waiting for ARP reply from gateway")
		}
	}

	// 重置过滤器以捕获所有流量
	err = handle.SetBPFFilter("")
	if err != nil {
		handle.Close()
		return nil, fmt.Errorf("failed to reset BPF filter: %v", err)
	}
	p.packetChan = packetChan
	log.Infof("Reset BPF filter to capture all traffic")
	return p, nil
}

func (p *PcapReadWriteCloser) Read(packet []byte) (n int, err error) {
	if p.packetChan == nil {
		return 0, fmt.Errorf("packetChan is nil")
	}
	pkt, ok := <-p.packetChan
	if !ok {
		return 0, fmt.Errorf("packetChan is closed")
	}
	data := pkt.Data()
	rawLayer := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default)
	if rawLayer != nil {
		ethernetLayer := rawLayer.Layer(layers.LayerTypeEthernet)
		if _, ok := ethernetLayer.(*layers.Ethernet); ok {
			data = data[14:]
		}

		netLayer := rawLayer.NetworkLayer()
		if netLayer != nil {
			tcpLayer := rawLayer.Layer(layers.LayerTypeTCP)
			if tcpLayer != nil {
				if tcp, ok := tcpLayer.(*layers.TCP); ok {
					if tcp.RST {
						// networkLayer, ok := netLayer.(*layers.IPv4)
						// if ok && networkLayer.DstIP.Equal(net.ParseIP("47.52.100.44")) {
						// 	log.Infof("skip tcp rst packet from: %v:%s <- %v:%s", networkLayer.NetworkFlow().Src(), tcp.SrcPort, networkLayer.NetworkFlow().Dst(), tcp.DstPort)
						// }
						return 0, nil
					}

					// if tcp.SYN && tcp.ACK {
					// 	networkLayer, ok := netLayer.(*layers.IPv4)
					// 	if ok && networkLayer.SrcIP.Equal(net.ParseIP("47.52.100.84")) {
					// 		log.Infof("recv tcp syn-ack packet from: %v:%s <- %v:%s", networkLayer.NetworkFlow().Src(), tcp.SrcPort, networkLayer.NetworkFlow().Dst(), tcp.DstPort)
					// 	}
					// }
				}
			} else if dhcpv4 := rawLayer.Layer(layers.LayerTypeDHCPv4); dhcpv4 != nil {
				if dhcp, ok := dhcpv4.(*layers.DHCPv4); ok {
					if dhcp.Operation == layers.DHCPOpReply {
						if len(dhcp.Options) > 0 {
							for _, opt := range dhcp.Options {
								if opt.Type == layers.DHCPOptMessageType && len(opt.Data) > 0 {
									if opt.Data[0] == byte(layers.DHCPMsgTypeOffer) {
										log.Infof("收到DHCP Offer消息: 服务器IP=%v, 提供IP=%v", dhcp.NextServerIP, dhcp.YourClientIP)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	n = copy(packet, data)
	if n < len(data) {
		return n, nil
	}
	return len(data), nil

}

func (p *PcapReadWriteCloser) Write(packet []byte) (n int, err error) {
	p.writeMutex.Lock()
	defer p.writeMutex.Unlock()

	if len(packet) > p.mtu {
		// Truncate packet to MTU size
		packet = packet[:p.mtu]
	}

	// 解析数据包
	rawLayer := gopacket.NewPacket(packet, layers.LayerTypeIPv4, gopacket.Default)
	if rawLayer != nil {
		tcpLayer := rawLayer.Layer(layers.LayerTypeTCP)
		if tcpLayer != nil {
			if tcp, ok := tcpLayer.(*layers.TCP); ok {
				if tcp.RST {
					return len(packet), nil
				}
			}
		}
	}

	// Check if this is a network layer packet that needs ethernet header
	if len(packet) > 0 {
		version := packet[0] >> 4
		if version == 4 || version == 6 {
			// 检查是否为 TCP RST 包
			if len(packet) >= 20 { // IPv4 header
				var isTCP bool
				var tcpHeaderOffset int

				if version == 4 {
					ihl := packet[0] & 0x0F
					protocol := packet[9]
					tcpHeaderOffset = int(ihl) * 4
					isTCP = protocol == 6 // TCP protocol number
				} else if version == 6 {
					protocol := packet[6]
					tcpHeaderOffset = 40 // IPv6 header size
					isTCP = protocol == 6
				}

				// 如果是 TCP 包且长度足够包含 TCP 头
				if isTCP && len(packet) >= tcpHeaderOffset+13 {
					tcpFlags := packet[tcpHeaderOffset+13]
					if tcpFlags&0x04 != 0 { // RST flag is set
						return len(packet), nil // 直接返回,不发送 RST 包
					}
				}
			}

			// Create ethernet header
			eth := &layers.Ethernet{
				SrcMAC:       p.deviceHardwareAddr,
				DstMAC:       p.gatewayHardwareAddr,
				EthernetType: layers.EthernetTypeIPv4,
			}
			if version == 6 {
				eth.EthernetType = layers.EthernetTypeIPv6
			}

			// Serialize ethernet header
			buf := gopacket.NewSerializeBuffer()
			opts := gopacket.SerializeOptions{}
			err := gopacket.SerializeLayers(buf, opts,
				eth,
				gopacket.Payload(packet),
			)
			if err != nil {
				return 0, err
			}
			packet = buf.Bytes()
		}
		err = p.handle.WritePacketData(packet)
		if err != nil {
			return 0, err
		}
		return len(packet), nil
	}
	return 0, fmt.Errorf("unsupported packet type")
}

func (p *PcapReadWriteCloser) Close() error {
	p.handle.Close()
	return nil
}

func NewPcapReadWriteCloserEndpoint(device string, snaplen int32) (*ReadWriteEndpoint, error) {
	rwc, err := NewPcapReadWriteCloser(device, snaplen)
	if err != nil {
		return nil, err
	}
	return NewReadWriteCloserEndpoint(rwc, uint32(rwc.mtu), 0)
}

func NewPcapReadWriteCloserEndpointEx(device string, snaplen int32) (*PcapReadWriteCloser, *ReadWriteEndpoint, error) {
	rwc, err := NewPcapReadWriteCloser(device, snaplen)
	if err != nil {
		return nil, nil, err
	}
	ep, err := NewReadWriteCloserEndpoint(rwc, uint32(rwc.mtu), 0)
	if err != nil {
		return nil, nil, err
	}
	return rwc, ep, nil
}
