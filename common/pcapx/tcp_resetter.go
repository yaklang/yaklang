package pcapx

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func buildRST(ether *layers.Ethernet, iplayer gopacket.SerializableLayer, trans *layers.TCP) ([][]gopacket.SerializableLayer, error) {
	var base = make([]gopacket.SerializableLayer, 0, 3)
	var reverse = make([]gopacket.SerializableLayer, 0, 3)
	base = append(base, ether)
	reverseEther := CopyEthernetLayer(ether)
	reverseEther.SrcMAC, reverseEther.DstMAC = ether.DstMAC, ether.SrcMAC
	reverse = append(reverse, reverseEther)

	// ------------------------------------------------------------------------------------
	// remove payload data from packet
	// start to build rst
	trans.Payload = make([]byte, 0)
	trans.RST = true
	// others are false (FIN, SYN, RST, PSH, ACK, URG, ECE, CWR, NS)
	trans.FIN = false
	trans.SYN = false
	trans.PSH = false
	trans.ACK = false
	trans.URG = false
	trans.ECE = false
	trans.CWR = false
	trans.NS = false

	// active reset
	// seq: trans.ack
	originACK := trans.Ack
	originSeq := trans.Seq
	trans.Seq = originACK

	reverseTrans := CopyTCP(trans)
	reverseTrans.SrcPort, reverseTrans.DstPort = reverseTrans.DstPort, reverseTrans.SrcPort
	reverseTrans.Seq = originSeq + 1
	// -------------------------------------------------------------------------------------

	switch ret := iplayer.(type) {
	case *layers.IPv4:
		base = append(base, ret)
		reverseNet := CopyIPv4Layer(ret)
		reverseNet.SrcIP, reverseNet.DstIP = reverseNet.DstIP, reverseNet.SrcIP
		reverse = append(reverse, reverseNet)

		err := trans.SetNetworkLayerForChecksum(ret)
		if err != nil {
			log.Errorf("set network layer for checksum failed: %s", err)
		}
		err = reverseTrans.SetNetworkLayerForChecksum(reverseNet)
		if err != nil {
			log.Errorf("(reverse) set network layer for checksum failed: %s", err)
		}
	case *layers.IPv6:
		base = append(base, ret)
		reverseNet := CopyIPv6Layer(ret)
		reverseNet.SrcIP, reverseNet.DstIP = reverseNet.DstIP, reverseNet.SrcIP
		reverse = append(reverse, reverseNet)

		err := trans.SetNetworkLayerForChecksum(ret)
		if err != nil {
			log.Errorf("set network layer for checksum failed: %s", err)
		}
		err = reverseTrans.SetNetworkLayerForChecksum(reverseNet)
		if err != nil {
			log.Errorf("(reverse) set network layer for checksum failed: %s", err)
		}
	default:
		return nil, utils.Errorf("not a valid ip layer %T", iplayer)
	}

	base = append(base, trans)
	reverse = append(reverse, reverseTrans)

	return [][]gopacket.SerializableLayer{
		base, reverse,
	}, nil
}

func GenerateTCPRST(i any) ([][]byte, error) {
	switch ret := i.(type) {
	case gopacket.Packet:
		packet := ret
		etherlayer, ok := packet.LinkLayer().(*layers.Ethernet)
		if !ok {
			return nil, utils.Errorf("not a ethernet packet, but %T", packet.LinkLayer())
		}

		ipLayer := packet.Layer(layers.LayerTypeIPv4)
		tcpLayer := packet.Layer(layers.LayerTypeTCP)
		if ipLayer == nil && tcpLayer == nil {
			log.Debug("not a tcp/ip not found")
			return nil, nil
		}

		tcpInstance, ok := tcpLayer.(*layers.TCP)
		if !ok {
			return nil, utils.Errorf("not a tcp packet, but %T", tcpInstance)
		}

		var networkLayer gopacket.SerializableLayer

		switch ret := ipLayer.(type) {
		case *layers.IPv4:
			log.Infof("ipv4 packet %v -> %v", ret.SrcIP, ret.DstIP)
			networkLayer = ret
		case *layers.IPv6:
			log.Infof("ip6 packet %v -> %v", ret.SrcIP, ret.DstIP)
			networkLayer = ret
		default:
			return nil, utils.Errorf("not a ip packet, but %T", ipLayer)
		}

		packets, err := buildRST(etherlayer, networkLayer, tcpInstance)
		if err != nil {
			return nil, err
		}

		var results = make([][]byte, 0, len(packets))
		for _, p := range packets {
			packetRaw, err := AutoSerializeLayers(p...)
			if err != nil {
				log.Warnf("serialize packet failed: %s", err)
				continue
			}
			//verifyPacket := gopacket.NewPacket(packetRaw, layers.LayerTypeEthernet, gopacket.DecodeOptions{
			//	// default config for packet
			//})
			//if verifyPacket.ErrorLayer() != nil {
			//	log.Warnf("verify packet failed: %s", verifyPacket.ErrorLayer())
			//	continue
			//}
			//log.Infof("packet verified: %v", verifyPacket.String())
			results = append(results, packetRaw)
		}
		return results, nil
	case []byte:
		// build packet from gopacket
		packet := gopacket.NewPacket(ret, layers.LayerTypeEthernet, gopacket.DecodeOptions{
			// default config for packet
		})
		return GenerateTCPRST(packet)
	case string:
		return GenerateTCPRST([]byte(ret))
	default:
		return nil, utils.Errorf("not a valid type %T", i)
	}
}
