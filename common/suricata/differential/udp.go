package differential

import (
	"fmt"
	"net"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

type UDPDatagram struct {
	SrcIP   net.IP
	DstIP   net.IP
	SrcPort layers.UDPPort
	DstPort layers.UDPPort
	Payload gopacket.SerializableLayer
}

type UDPFlow struct {
	Datagrams []UDPDatagram
}

func (flow UDPFlow) Frames() ([][]byte, error) {
	frames := make([][]byte, 0, len(flow.Datagrams))
	for index, datagram := range flow.Datagrams {
		if datagram.SrcIP.To4() == nil || datagram.DstIP.To4() == nil {
			return nil, fmt.Errorf("UDP datagram %d requires IPv4 addresses", index)
		}
		ethernet := &layers.Ethernet{
			SrcMAC:       net.HardwareAddr{0x02, 0, 0, 0, 0, 3},
			DstMAC:       net.HardwareAddr{0x02, 0, 0, 0, 0, 4},
			EthernetType: layers.EthernetTypeIPv4,
		}
		ipv4 := &layers.IPv4{
			Version:  4,
			IHL:      5,
			Id:       uint16(500 + index),
			TTL:      64,
			Protocol: layers.IPProtocolUDP,
			SrcIP:    datagram.SrcIP,
			DstIP:    datagram.DstIP,
		}
		udp := &layers.UDP{SrcPort: datagram.SrcPort, DstPort: datagram.DstPort}
		if err := udp.SetNetworkLayerForChecksum(ipv4); err != nil {
			return nil, err
		}
		buffer := gopacket.NewSerializeBuffer()
		if err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ethernet, ipv4, udp, datagram.Payload); err != nil {
			return nil, fmt.Errorf("serialize UDP datagram %d: %w", index, err)
		}
		frames = append(frames, append([]byte(nil), buffer.Bytes()...))
	}
	return frames, nil
}

func DNSResponseFirstFlow() UDPFlow {
	dns := &layers.DNS{
		ID:           7,
		QR:           true,
		ResponseCode: layers.DNSResponseCodeNoErr,
		Questions: []layers.DNSQuestion{{
			Name:  []byte("response-first.test"),
			Type:  layers.DNSTypeA,
			Class: layers.DNSClassIN,
		}},
	}
	return UDPFlow{Datagrams: []UDPDatagram{{
		SrcIP:   net.IPv4(198, 51, 100, 53),
		DstIP:   net.IPv4(192, 0, 2, 10),
		SrcPort: 53,
		DstPort: 53000,
		Payload: dns,
	}}}
}
