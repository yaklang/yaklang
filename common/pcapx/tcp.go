package pcapx

import (
	"encoding/binary"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"net"
	"time"
)

type TCPIPFrame struct {
	ToServer bool
	IP       *layers.IPv4
	TCP      *layers.TCP
}

func tsOpt() layers.TCPOption {
	// 构建 TCP 时间戳选项
	tsOption := layers.TCPOption{
		OptionType:   layers.TCPOptionKindTimestamps,
		OptionLength: 10, // 时间戳选项长度为 10 字节
		OptionData:   make([]byte, 8),
	}

	// 设置时间戳值
	currentTime := time.Now()
	tsValue := uint32(currentTime.UnixNano() / int64(time.Millisecond)) // 这里可以设置你需要的时间戳值
	tsEchoReply := tsValue                                              // 这里可以设置你需要的时间戳回显应答值
	binary.BigEndian.PutUint32(tsOption.OptionData[0:4], tsValue)
	binary.BigEndian.PutUint32(tsOption.OptionData[4:8], tsEchoReply)
	return tsOption
}

func CreateTCPFlowFromPayload(src, dst string, payload []byte) ([][]byte, error) {
	if src == "" {
		_, _, srcRaw, err := getPublicRoute()
		if err != nil {
			return nil, utils.Error("cannot found src route")
		}
		src = net.JoinHostPort(srcRaw.String(), fmt.Sprint(40000+rand.Intn(20000)))
	}
	srcIP, dstIP, srcPort, dstPort, err := ParseSrcNDstAddress(src, dst)
	if err != nil {
		return nil, err
	}
	seql := rand.Uint32()
	seqr := rand.Uint32()

	ip := &layers.IPv4{
		Version:  4,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    srcIP,
		DstIP:    dstIP,
		TTL:      64,
	}

	var flow [][]byte
	link, _ := GetPublicToServerLinkLayerIPv4()

	// SYN ->
	synTCP := &layers.TCP{
		SrcPort: layers.TCPPort(srcPort),
		DstPort: layers.TCPPort(dstPort),
		Seq:     seql,
		Ack:     0,
		SYN:     true,
		Options: []layers.TCPOption{
			{
				OptionType:   layers.TCPOptionKindMSS,
				OptionLength: 0x04,
				OptionData:   []byte{0x05, 0xb4},
			}, {
				OptionType:   layers.TCPOptionKindWindowScale,
				OptionData:   []byte{0x06},
				OptionLength: 0x03,
			},
			{
				OptionType: layers.TCPOptionKindSACKPermitted,
			},
		},
		Window: 65535,
	}
	ipTo := ip
	synTCP.SetNetworkLayerForChecksum(ipTo)

	buf, err := seriGopkt(link, ipTo, synTCP)
	if err != nil {
		return nil, errors.Wrap(err, "cannot serialize gopacket")
	}

	flow = append(flow, buf)

	ipFrom := &layers.IPv4{
		Version:  4,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    ip.DstIP,
		DstIP:    ip.SrcIP,
		TTL:      64,
	}
	// SYN ACK <-
	synAckTCP := &layers.TCP{
		SrcPort: layers.TCPPort(dstPort),
		DstPort: layers.TCPPort(srcPort),
		Seq:     seqr,
		Ack:     seql + 1,
		SYN:     true,
		ACK:     true,
		Window:  65535,
		Options: []layers.TCPOption{
			{
				OptionType:   layers.TCPOptionKindMSS,
				OptionLength: 0x04,
				OptionData:   []byte{0x05, 120},
			}, {
				OptionType:   layers.TCPOptionKindWindowScale,
				OptionData:   []byte{0x09},
				OptionLength: 0x03,
			},
			{
				OptionType: layers.TCPOptionKindSACKPermitted,
			},
		},
	}
	synAckTCP.SetNetworkLayerForChecksum(ipFrom)

	buf, err = seriGopkt(link, ipFrom, synAckTCP)
	if err != nil {
		return nil, errors.Wrap(err, "cannot serialize gopacket")
	}
	flow = append(flow, buf)
	seql++

	// ACK ->
	var nextWindow uint16 = 2060
	ackTCP := &layers.TCP{
		SrcPort: layers.TCPPort(srcPort),
		DstPort: layers.TCPPort(dstPort),
		Seq:     seql,
		Ack:     seqr + 1,
		ACK:     true,
		Window:  nextWindow,
	}
	ackTCP.SetNetworkLayerForChecksum(ipTo)

	buf, err = seriGopkt(link, ipTo, ackTCP)
	if err != nil {
		return nil, err
	}
	flow = append(flow, buf)

	var frames []*TCPIPFrame
	if len(payload) > 0 {
		var lastFrame *layers.TCP
		for _, payloadFrame := range funk.Chunk(payload, 1400).([][]byte) {
			// push ack
			ackPush := &layers.TCP{
				SrcPort: layers.TCPPort(srcPort),
				DstPort: layers.TCPPort(dstPort),
				Seq:     seql,
				Ack:     seqr + 1,
				ACK:     true,
				Window:  nextWindow,
			}
			ackPush.SetNetworkLayerForChecksum(ipTo)
			ackPush.Payload = payloadFrame
			frames = append(frames, &TCPIPFrame{ToServer: true, IP: ipTo, TCP: ackPush})
			seql += uint32(len(payloadFrame))
			lastFrame = ackPush
		}
		if lastFrame != nil {
			lastFrame.PSH = true
		}
	} else {
		ackPush := &layers.TCP{
			SrcPort: layers.TCPPort(srcPort),
			DstPort: layers.TCPPort(dstPort),
			Seq:     seql,
			Ack:     seqr + 1,
			PSH:     true,
			ACK:     true,
			Window:  nextWindow,
		}
		ackPush.SetNetworkLayerForChecksum(ipTo)
		frames = append(frames, &TCPIPFrame{ToServer: true, IP: ipTo, TCP: ackPush})
	}

	for _, frame := range frames {
		buf, err = seriGopkt(link, frame.IP, frame.TCP, gopacket.Payload(frame.TCP.Payload))
		if err != nil {
			return nil, err
		}
		flow = append(flow, buf)
	}

	finACK := &layers.TCP{
		SrcPort: layers.TCPPort(srcPort),
		DstPort: layers.TCPPort(dstPort),
		Seq:     seql,
		Ack:     seqr + 1,
		FIN:     true,
		ACK:     true,
		Window:  nextWindow,
	}
	finACK.SetNetworkLayerForChecksum(ipTo)

	buf, err = seriGopkt(link, ipTo, finACK)
	if err != nil {
		return nil, errors.Wrap(err, "cannot serialize gopacket")
	}
	flow = append(flow, buf)

	return flow, nil
}

func CompleteTCPFlow(raw []byte) ([][]byte, error) {
	flows := make([][]byte, 0, 3)

	pk := gopacket.NewPacket(raw, layers.LayerTypeEthernet, gopacket.Default)
	if pk == nil {
		return nil, fmt.Errorf("cannot parse packet")
	}

	linkLy, ok := pk.LinkLayer().(*layers.Ethernet)
	if !ok || linkLy == nil {
		return nil, fmt.Errorf("cannot parse link layer")
	}

	networkLy, ok := pk.NetworkLayer().(*layers.IPv4)
	if !ok || networkLy == nil {
		return nil, fmt.Errorf("cannot parse network layer")
	}

	tcpLy, ok := pk.TransportLayer().(*layers.TCP)
	if !ok || tcpLy == nil {
		return nil, fmt.Errorf("cannot parse tcp layer")
	}

	if tcpLy.SYN || tcpLy.FIN {
		return [][]byte{raw}, nil
	}

	// 1 SYN ->
	synTCP := &layers.TCP{
		SrcPort: tcpLy.SrcPort,
		DstPort: tcpLy.DstPort,
		Seq:     tcpLy.Seq - 1,
		SYN:     true,
		Window:  tcpLy.Window,
		Options: []layers.TCPOption{
			{
				OptionType:   layers.TCPOptionKindMSS,
				OptionLength: 0x04,
				OptionData:   []byte{0x05, 120},
			}, {
				OptionType:   layers.TCPOptionKindWindowScale,
				OptionData:   []byte{0x09},
				OptionLength: 0x03,
			},
			{
				OptionType: layers.TCPOptionKindSACKPermitted,
			},
		},
	}

	_ = synTCP.SetNetworkLayerForChecksum(networkLy)
	pkt, err := seriGopkt(linkLy, networkLy, synTCP)
	if err != nil {
		return nil, err
	}
	flows = append(flows, pkt)

	// 2 SYN ACK <-
	ipre := &layers.IPv4{
		Version:  4,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    networkLy.DstIP,
		DstIP:    networkLy.SrcIP,
		TTL:      networkLy.TTL,
	}
	synAckTCP := &layers.TCP{
		SrcPort: tcpLy.DstPort,
		DstPort: tcpLy.SrcPort,
		Seq:     tcpLy.Ack - 1,
		Ack:     tcpLy.Seq,
		SYN:     true,
		ACK:     true,
		Window:  tcpLy.Window,
		Options: []layers.TCPOption{
			{
				OptionType:   layers.TCPOptionKindMSS,
				OptionLength: 0x04,
				OptionData:   []byte{0x05, 120},
			}, {
				OptionType:   layers.TCPOptionKindWindowScale,
				OptionData:   []byte{0x09},
				OptionLength: 0x03,
			},
			{
				OptionType: layers.TCPOptionKindSACKPermitted,
			},
		},
	}

	_ = synAckTCP.SetNetworkLayerForChecksum(ipre)
	pkt, err = seriGopkt(linkLy, ipre, synAckTCP)
	if err != nil {
		return nil, err
	}
	flows = append(flows, pkt)

	// 3 ACK ->
	ackTCP := &layers.TCP{
		SrcPort: tcpLy.SrcPort,
		DstPort: tcpLy.DstPort,
		Ack:     tcpLy.Ack,
		ACK:     true,
		Seq:     tcpLy.Seq,
		Window:  tcpLy.Window,
		Options: []layers.TCPOption{
			{
				OptionType:   layers.TCPOptionKindWindowScale,
				OptionData:   []byte{0x09},
				OptionLength: 0x03,
			},
		},
	}

	_ = ackTCP.SetNetworkLayerForChecksum(networkLy)
	pkt, err = seriGopkt(linkLy, networkLy, ackTCP)
	if err != nil {
		return nil, err
	}
	flows = append(flows, pkt)

	// 3 sending payload ->
	_ = tcpLy.SetNetworkLayerForChecksum(networkLy)
	syn2 := &layers.TCP{
		SrcPort: tcpLy.SrcPort,
		DstPort: tcpLy.DstPort,
		Seq:     tcpLy.Seq,
		Ack:     tcpLy.Ack,
		FIN:     tcpLy.FIN,
		SYN:     false,
		RST:     tcpLy.RST,
		PSH:     tcpLy.PSH,
		ACK:     tcpLy.ACK,
		URG:     tcpLy.URG,
		ECE:     tcpLy.ECE,
		CWR:     tcpLy.CWR,
		NS:      tcpLy.NS,
		Window:  tcpLy.Window,
		Urgent:  tcpLy.Urgent,
		Options: tcpLy.Options,
	}
	_ = syn2.SetNetworkLayerForChecksum(networkLy)
	pkt, err = seriGopkt(linkLy, networkLy, syn2, gopacket.Payload(tcpLy.Payload))
	if err != nil {
		return nil, err
	}
	flows = append(flows, pkt)

	// 4 fin ack ->
	finACK := &layers.TCP{
		SrcPort: tcpLy.SrcPort,
		DstPort: tcpLy.DstPort,
		Seq:     tcpLy.Seq + uint32(len(tcpLy.Payload)),
		Ack:     tcpLy.Ack,
		FIN:     true,
		ACK:     true,
		Window:  tcpLy.Window,
	}
	finACK.SetNetworkLayerForChecksum(networkLy)

	pkt, err = seriGopkt(linkLy, networkLy, finACK)
	if err != nil {
		return nil, errors.Wrap(err, "cannot serialize gopacket")
	}
	flows = append(flows, pkt)

	return flows, nil
}
