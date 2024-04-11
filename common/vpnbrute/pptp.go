package vpnbrute

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	binparser "github.com/yaklang/yaklang/common/bin-parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/pcapx/arpx"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"github.com/yaklang/yaklang/common/vpnbrute/ppp"
	"net"
	"time"
)

type PPTPAuth struct {
	ppp       *ppp.PPPAuth
	PNSCallID uint16 // c -> s gre call id
	PASCallID uint16 // s -> c gre call id
	Target    string
	LocalAddr net.Addr

	RAddrByte []byte
	LAddrByte []byte

	RMac []byte
	LMac []byte

	SNumber int
	ctx     context.Context
	cancel  context.CancelFunc
}

func GetDefaultPPTPAuth() *PPTPAuth {
	ctx, cancel := context.WithCancel(context.Background())

	return &PPTPAuth{
		ppp:     ppp.GetDefaultPPPAuth(),
		ctx:     ctx,
		cancel:  cancel,
		SNumber: 0,
	}
}

func GetStartControlConnReq() map[string]any {
	return map[string]any{
		"Length":             156,
		"MessageType":        1,
		"MagicCookie":        0x1a2b3c4d,
		"ControlMessageType": 1,
		"Reserved":           0,
		"Start Control Conn Req": map[string]any{
			"ProtocolVersion":     0x0100,
			"Reserved":            0,
			"FramingCapabilities": 0x1,
			"BearerCapabilities":  0x1,
			"MaxChannels":         0x0,
			"FirmwareRevision":    0x0,
			"Hostname":            bytes.Repeat([]byte{0x0}, 64),
			"Vendor":              append([]byte{0x4d, 0x69, 0x63, 0x72, 0x6f, 0x73, 0x6f, 0x66, 0x74}, bytes.Repeat([]byte{0x0}, 55)...),
		},
	}
}

func GetOutgoingCallReq() map[string]any {
	return map[string]any{
		"Length":             168,
		"MessageType":        1,
		"MagicCookie":        0x1a2b3c4d,
		"ControlMessageType": 7,
		"Reserved":           0,
		"Outgoing Call Req": map[string]any{
			"CallId":            7568,
			"CallSerialNumber":  1,
			"MinimumBPS":        300,
			"MaximumBPS":        100000000,
			"BearerType":        0x3,
			"FramingType":       0x3,
			"RecvWindowSize":    0x0a,
			"ProcessingDelay":   0,
			"PhoneNumberLength": 0,
			"Reserved":          0,
			"PhoneNumber":       bytes.Repeat([]byte{0x0}, 64),
			"SubAddress":        bytes.Repeat([]byte{0x0}, 64),
		},
	}
}

func (pptp *PPTPAuth) GetSetLinkInfo() map[string]any {
	return map[string]any{
		"Length":             168,
		"MessageType":        1,
		"MagicCookie":        0x1a2b3c4d,
		"ControlMessageType": 15,
		"Reserved":           0,
		"Set Link Info": map[string]any{
			"PeerCallId": pptp.PNSCallID,
			"Reserved":   0,
			"Send Accm":  0xffffffff,
			"Recv Accm":  0xffffffff,
		},
	}
}

func (pptp *PPTPAuth) Auth() (error, bool) {
	defer pptp.cancel()
	conn, err := netx.DialX(pptp.Target, netx.DialX_WithTimeout(5*time.Second))
	if err != nil {
		return err, false
	}

	pptp.LocalAddr = conn.LocalAddr()

	startReq, err := parser.GenerateBinary(GetStartControlConnReq(), "application-layer.pptp", "PPTP")
	if err != nil {
		return err, false
	}

	_, err = conn.Write(binparser.NodeToBytes(startReq))
	if err != nil {
		return err, false
	}

	// read start control conn reply
	for {
		messageNode, err := parser.ParseBinary(conn, "application-layer.pptp", "PPTP")
		if err != nil {
			return err, false
		}
		if messageNode.Name != "PPTP" {
			return utils.Error("not PPP message"), false
		}

		messageMap := binparser.NodeToMap(messageNode).(map[string]any)
		pptpType := messageMap["ControlMessageType"]
		switch pptpType {
		case uint16(2):
			pptp.PASCallID = 7568
			outgoingReq, err := parser.GenerateBinary(GetOutgoingCallReq(), "application-layer.pptp", "PPTP")
			if err != nil {
				return err, false
			}
			_, err = conn.Write(binparser.NodeToBytes(outgoingReq))
			if err != nil {
				return err, false
			}
		case uint16(8):
			pptp.PNSCallID = messageMap["Message"].(map[string]any)["Outgoing Call Reply"].(map[string]any)["CallId"].(uint16)
			setInfo, err := parser.GenerateBinary(pptp.GetSetLinkInfo(), "application-layer.pptp", "PPTP")
			if err != nil {
				return err, false
			}
			_, err = conn.Write(binparser.NodeToBytes(setInfo))
			if err != nil {
				return err, false
			}

			go func() {
				for {
					select {
					case <-pptp.ctx.Done():
					default:
						time.Sleep(1)
						conn.Read(make([]byte, 1024))
					}
				}
			}()
			// gre
			return pptp.Tunnel()
		}
	}
}

func (pptp *PPTPAuth) Tunnel() (error, bool) {
	targetHost := utils.ExtractHost(pptp.Target)
	iface, _, sIp, err := netutil.Route(5*time.Second, targetHost)
	if err != nil {
		return err, false
	}

	pptp.LAddrByte = sIp.To4()
	pptp.RAddrByte = net.ParseIP(targetHost).To4()

	pptp.LMac = iface.HardwareAddr
	pptp.RMac, err = arpx.Arp(iface.Name, targetHost)
	if err != nil {
		//log.Error(err)
		return err, false
	}

	sendPacketCh := make(chan []byte, 1024)
	errorCh := make(chan error, 10)

	go func() {
		err = pcaputil.Start(
			pcaputil.WithDevice(iface.Name),
			pcaputil.WithEnableCache(true),
			pcaputil.WithBPFFilter("proto 47"),
			pcaputil.WithContext(pptp.ctx),
			pcaputil.WithNetInterfaceCreated(func(handle *pcap.Handle) {
				go func() {
					for {
						select {
						case packets, ok := <-sendPacketCh:
							if !ok {
								continue
							}
							err := handle.WritePacketData(packets)
							if err != nil {
								errorCh <- err
								log.Error(err)
							}
						case <-pptp.ctx.Done():
							return
						}
					}
				}()
			}),
			pcaputil.WithEveryPacket(func(packet gopacket.Packet) {
				go func(itemPacket gopacket.Packet) {
					node, err := parser.ParseBinary(bytes.NewReader(packet.Data()), "ethernet")
					if err != nil {
						log.Error(err)
						errorCh <- err
						return
					}
					respMap, err := pptp.ProcessGre(base.GetNodeByPath(node, "@Ethernet.Payload.IP.Payload.GRE"))
					if err != nil {
						log.Error(err)
						errorCh <- err
						return
					}

					if respMap == nil {
						log.Infof("not need send resp")
						return
					}

					sendPacket, err := pptp.getSendPacket(respMap)
					if err != nil {
						log.Error(err)
						errorCh <- err
						return
					}
					if sendPacket == nil {
						log.Error("send packet is nil")
						return
					}

					sendPacketCh <- sendPacket
				}(packet)
			}),
		)
		if err != nil {
			errorCh <- err
		}
	}()

	packet, err := pptp.getSendPacket(pptp.ppp.GetPPPReqParams())
	if err != nil {
		return err, false
	}
	sendPacketCh <- packet

	select {
	case <-pptp.ctx.Done():
		return utils.Error("context done"), false
	case <-errorCh:
		return err, false
	case ok := <-pptp.ppp.AuthOk:
		return nil, ok
	}

	//targetHost := utils.ExtractHost(pptp.Target)
	//raddr, err := net.ResolveIPAddr("ip", targetHost)
	//if err != nil {
	//	return
	//}
	//
	//laddr, err := net.ResolveIPAddr("ip", utils.ExtractHost(pptp.LocalAddr.String()))
	//if err != nil {
	//	return
	//}
	//
	//ipConn, err := net.DialIP("ip:47", laddr, raddr)
	//if err != nil {
	//	return
	//}
	//defer ipConn.Close()
	//
	//for {
	//	var buffer []byte
	//	data := bytes.NewBuffer(buffer)
	//	go func() {
	//		time.Sleep(10 * time.Millisecond)
	//		spew.Dump(data.Bytes())
	//	}()
	//	messageNode, err := parser.ParseBinary(io.TeeReader(ipConn, data), "internet_protocol")
	//	if err != nil {
	//		return
	//	}
	//
	//	go func(processNode *base.Node) {
	//		respMap, err := pptp.ProcessGre(base.GetNodeByPath(processNode, "@Internet Protocol.Payload.GRE"))
	//		if err != nil {
	//			return
	//		}
	//		pptp.SendPPPByGRE(respMap, ipConn)
	//	}(messageNode)
	//}

}

func (pptp *PPTPAuth) getSendPacket(pppMap map[string]any) ([]byte, error) {
	greMap, err := pptp.encapsulateGRE(pppMap)
	if err != nil {
		return nil, err
	}

	ethernetMap, err := pptp.encapsulateIPEthernet(greMap)
	if err != nil {
		return nil, err
	}

	ethNode, err := parser.GenerateBinary(ethernetMap, "ethernet", "Ethernet")
	if err != nil {
		log.Error(err)
	}

	return binparser.NodeToBytes(ethNode), nil
}

func (pptp *PPTPAuth) encapsulateGRE(payload map[string]any) (map[string]any, error) {
	res, err := parser.GenerateBinary(payload, "ppp", "PPP")
	if err != nil {
		return nil, err
	}
	payloadByte := binparser.NodeToBytes(res)
	snumber := pptp.SNumber
	pptp.SNumber++
	return map[string]any{
		"Flags And Version": 0x3001,
		"Protocol Type":     0x880b,
		"Payload Length":    len(payloadByte),
		"Call ID":           pptp.PNSCallID,
		"Sequence Number":   snumber,
		"Payload": map[string]any{
			"PPP": payloadByte,
		},
	}, nil

}

func (pptp *PPTPAuth) ProcessGre(messageNode *base.Node) (map[string]any, error) {
	if messageNode.Name != "GRE" {
		return nil, utils.Error("not GRE message")
	}

	messageMap := binparser.NodeToMap(messageNode).(map[string]any)
	if messageMap["Call ID"].(uint16) != pptp.PASCallID {
		return nil, nil
	}
	pppNode := base.GetNodeByPath(messageNode, "@GRE.Payload.PPP")
	pppParamMap, err := pptp.ppp.ProcessMessage(pppNode)
	if err != nil || pppParamMap == nil {
		return nil, err
	}
	return pppParamMap, nil
}

func getDefaultIPV4Header() map[string]any {
	return map[string]any{
		"Version":                   4,
		"Header Length":             5,
		"Type of Service":           byte(0),
		"Identification":            []byte{0x40, 0xb3},
		"Flags And Fragment Offset": []byte{0x00, 0x00},
		"Time to Live":              0x80,
	}
}

func ipChecksum(totalLength uint16, protocol uint8, src, dst []byte) uint16 {
	checkList := []uint32{ // sum default header checksum
		0x4500,
		uint32(totalLength),
		uint32(0x4000),
		uint32(0x40)<<8 + uint32(protocol),
		uint32(binary.BigEndian.Uint16(src[:2])),
		uint32(binary.BigEndian.Uint16(src[2:])),
		uint32(binary.BigEndian.Uint16(dst[2:])),
		uint32(binary.BigEndian.Uint16(dst[:2])),
	}

	sum := uint32(0)
	for _, item := range checkList {
		sum += item
		// Add carry if any.
		sum = (sum >> 16) + (sum & 0xFFFF)
	}
	result := ^uint16(sum & 0xFFFF)

	return result
}

func (pptp *PPTPAuth) encapsulateIPEthernet(payload map[string]any) (map[string]any, error) {
	res, err := parser.GenerateBinary(payload, "generic_routing_encapsulation", "GRE")
	if err != nil {
		return nil, err
	}
	payloadByte := binparser.NodeToBytes(res)
	totalLength := uint16(20 + len(payloadByte))
	//checkSum := ipChecksum(totalLength, 0x2f, pptp.LAddrByte, pptp.RAddrByte)

	ipMap := getDefaultIPV4Header()
	ipMap["Total Length"] = totalLength
	ipMap["Header Checksum"] = 0x0
	ipMap["Protocol"] = 0x2f
	ipMap["Source"] = pptp.LAddrByte
	ipMap["Destination"] = pptp.RAddrByte
	ipMap["GRE"] = payloadByte

	return map[string]any{
		"Destination": pptp.RMac,
		"Source":      pptp.LMac,
		"Type":        0x0800,
		"IP":          ipMap,
	}, nil
}
