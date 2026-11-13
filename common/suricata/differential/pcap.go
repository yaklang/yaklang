package differential

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

type TCPPacket struct {
	ToServer bool
	Seq      uint32
	Ack      uint32
	SYN      bool
	ACK      bool
	FIN      bool
	RST      bool
	PSH      bool
	Payload  []byte
}

type TCPFlow struct {
	ClientIP   net.IP
	ServerIP   net.IP
	ClientPort layers.TCPPort
	ServerPort layers.TCPPort
	Packets    []TCPPacket
}

func NewTCPFlow(clientPort, serverPort uint16, clientPayloads ...[]byte) TCPFlow {
	const clientSeq = 1000
	const serverSeq = 9000

	flow := TCPFlow{
		ClientIP:   net.IPv4(192, 0, 2, 10),
		ServerIP:   net.IPv4(198, 51, 100, 20),
		ClientPort: layers.TCPPort(clientPort),
		ServerPort: layers.TCPPort(serverPort),
		Packets: []TCPPacket{
			{ToServer: true, Seq: clientSeq, SYN: true},
			{ToServer: false, Seq: serverSeq, Ack: clientSeq + 1, SYN: true, ACK: true},
			{ToServer: true, Seq: clientSeq + 1, Ack: serverSeq + 1, ACK: true},
		},
	}

	seq := uint32(clientSeq + 1)
	for i, payload := range clientPayloads {
		flow.Packets = append(flow.Packets, TCPPacket{
			ToServer: true,
			Seq:      seq,
			Ack:      serverSeq + 1,
			ACK:      true,
			PSH:      i == len(clientPayloads)-1,
			Payload:  payload,
		})
		seq += uint32(len(payload))
	}
	flow.Packets = append(flow.Packets, TCPPacket{
		ToServer: true,
		Seq:      seq,
		Ack:      serverSeq + 1,
		ACK:      true,
		FIN:      true,
	})
	return flow
}

func (flow TCPFlow) Frames() ([][]byte, error) {
	if flow.ClientIP.To4() == nil || flow.ServerIP.To4() == nil {
		return nil, fmt.Errorf("TCP flow requires IPv4 addresses")
	}

	frames := make([][]byte, 0, len(flow.Packets))
	for index, packet := range flow.Packets {
		srcIP, dstIP := flow.ClientIP, flow.ServerIP
		srcPort, dstPort := flow.ClientPort, flow.ServerPort
		srcMAC := net.HardwareAddr{0x02, 0, 0, 0, 0, 1}
		dstMAC := net.HardwareAddr{0x02, 0, 0, 0, 0, 2}
		if !packet.ToServer {
			srcIP, dstIP = dstIP, srcIP
			srcPort, dstPort = dstPort, srcPort
			srcMAC, dstMAC = dstMAC, srcMAC
		}

		ethernet := &layers.Ethernet{
			SrcMAC:       srcMAC,
			DstMAC:       dstMAC,
			EthernetType: layers.EthernetTypeIPv4,
		}
		ipv4 := &layers.IPv4{
			Version:  4,
			IHL:      5,
			Id:       uint16(100 + index),
			TTL:      64,
			Protocol: layers.IPProtocolTCP,
			SrcIP:    srcIP,
			DstIP:    dstIP,
		}
		tcp := &layers.TCP{
			SrcPort: srcPort,
			DstPort: dstPort,
			Seq:     packet.Seq,
			Ack:     packet.Ack,
			SYN:     packet.SYN,
			ACK:     packet.ACK,
			FIN:     packet.FIN,
			RST:     packet.RST,
			PSH:     packet.PSH,
			Window:  64240,
		}
		if err := tcp.SetNetworkLayerForChecksum(ipv4); err != nil {
			return nil, fmt.Errorf("set TCP checksum layer: %w", err)
		}
		buffer := gopacket.NewSerializeBuffer()
		options := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
		if err := gopacket.SerializeLayers(buffer, options, ethernet, ipv4, tcp, gopacket.Payload(packet.Payload)); err != nil {
			return nil, fmt.Errorf("serialize packet %d: %w", index, err)
		}
		frames = append(frames, append([]byte(nil), buffer.Bytes()...))
	}
	return frames, nil
}

func WritePCAP(path string, frames [][]byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := pcapgo.NewWriter(file)
	if err := writer.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
		return err
	}
	base := time.Date(2026, time.November, 10, 0, 0, 0, 0, time.UTC)
	for index, frame := range frames {
		captureInfo := gopacket.CaptureInfo{
			Timestamp:     base.Add(time.Duration(index) * time.Millisecond),
			CaptureLength: len(frame),
			Length:        len(frame),
		}
		if err := writer.WritePacket(captureInfo, frame); err != nil {
			return err
		}
	}
	return file.Close()
}
