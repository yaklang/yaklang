package vpnbrute

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/gopacket/gopacket"
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
	"golang.org/x/exp/rand"
	"net"
	"time"
)

var (
	PPTP_START_CONTROL_CONN_REQ = uint16(1)
	PPTP_START_CONTROL_CONN_REP = uint16(2)
	PPTP_OUTGOING_CALL_REQ      = uint16(7)
	PPTP_OUTGOING_CALL_REP      = uint16(8)
	PPTP_SER_LINK_INFO          = uint16(15)
)

type PPTPConfig struct {
	FramingCapabilities uint16
	BearerCapabilities  uint16
	MaxChannels         uint16
	FirmwareRevision    uint32
	Hostname            string
	Vendor              string

	CallId     uint16
	PeerCallId uint16

	CallSerialNumber  uint16
	MinimumBPS        uint32
	MaximumBPS        uint32
	BearerType        uint32
	FramingType       uint32
	RecvWindowSize    uint16
	ProcessingDelay   uint16
	PhoneNumberLength uint16
	Reserved          uint16
	PhoneNumber       string
	SubAddress        string

	SendAccm uint32
	RecvAccm uint32
}

func GetDefaultPPTPConfig() *PPTPConfig {
	return &PPTPConfig{
		FramingCapabilities: 0x1,
		BearerCapabilities:  0x1,
		MaxChannels:         0,
		FirmwareRevision:    0,
		Hostname:            "",
		Vendor:              "",
		CallId:              uint16(rand.Intn(65535)),
		PeerCallId:          0,
		CallSerialNumber:    1,
		MinimumBPS:          300,
		MaximumBPS:          100000000,
		BearerType:          0x3,
		FramingType:         0x3,
		RecvWindowSize:      0x0a,
		ProcessingDelay:     0,
		PhoneNumberLength:   0,
		Reserved:            0,
		PhoneNumber:         "",
		SubAddress:          "",
		SendAccm:            0xffffffff,
		RecvAccm:            0xffffffff,
	}
}

type PPTPAuthItem struct {
	ppp *ppp.PPPAuth
	cfg *PPTPConfig

	Target string

	RAddrByte []byte
	LAddrByte []byte

	RMac []byte
	LMac []byte

	SNumber int
}

func GetDefaultPPTPAuth() *PPTPAuthItem {
	return &PPTPAuthItem{
		ppp:     ppp.GetDefaultPPPAuth(),
		cfg:     GetDefaultPPTPConfig(),
		SNumber: 0,
	}
}

func PaddingBytes(data []byte, padding byte, size int) []byte {
	if len(data) >= size {
		return data
	}
	return append(data, bytes.Repeat([]byte{padding}, size-len(data))...)
}

func (c *PPTPConfig) GetStartControlConnReq() map[string]any {
	return map[string]any{
		"Length":             156,
		"MessageType":        1,
		"MagicCookie":        0x1a2b3c4d,
		"ControlMessageType": PPTP_START_CONTROL_CONN_REQ,
		"Reserved":           0,
		"Start Control Conn Req": map[string]any{
			"ProtocolVersion":     0x0100,
			"Reserved":            0,
			"FramingCapabilities": c.FramingCapabilities,
			"BearerCapabilities":  c.BearerCapabilities,
			"MaxChannels":         c.MaxChannels,
			"FirmwareRevision":    c.FirmwareRevision,
			"Hostname":            PaddingBytes([]byte(c.Hostname), 0x0, 64),
			"Vendor":              PaddingBytes([]byte(c.Vendor), 0x0, 64),
		},
	}
}

func GetStartControlConnReq() map[string]any {
	return map[string]any{
		"Length":             156,
		"MessageType":        1,
		"MagicCookie":        0x1a2b3c4d,
		"ControlMessageType": PPTP_START_CONTROL_CONN_REQ,
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

func (c *PPTPConfig) GetOutgoingCallReq() map[string]any {
	return map[string]any{
		"Length":             168,
		"MessageType":        1,
		"MagicCookie":        0x1a2b3c4d,
		"ControlMessageType": PPTP_OUTGOING_CALL_REQ,
		"Reserved":           0,
		"Outgoing Call Req": map[string]any{
			"CallId":            c.CallId,
			"CallSerialNumber":  c.CallSerialNumber,
			"MinimumBPS":        c.MinimumBPS,
			"MaximumBPS":        c.MaximumBPS,
			"BearerType":        c.BearerType,
			"FramingType":       c.FramingType,
			"RecvWindowSize":    c.RecvWindowSize,
			"ProcessingDelay":   c.ProcessingDelay,
			"PhoneNumberLength": c.PhoneNumberLength,
			"Reserved":          0,
			"PhoneNumber":       PaddingBytes([]byte(c.PhoneNumber), 0x0, 64),
			"SubAddress":        PaddingBytes([]byte(c.SubAddress), 0x0, 64),
		},
	}
}

func GetOutgoingCallReq() map[string]any {
	return map[string]any{
		"Length":             168,
		"MessageType":        1,
		"MagicCookie":        0x1a2b3c4d,
		"ControlMessageType": PPTP_OUTGOING_CALL_REQ,
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

func (c *PPTPConfig) GetSetLinkInfo() map[string]any {
	return map[string]any{
		"Length":             24,
		"MessageType":        1,
		"MagicCookie":        0x1a2b3c4d,
		"ControlMessageType": PPTP_SER_LINK_INFO,
		"Reserved":           0,
		"Set Link Info": map[string]any{
			"PeerCallId": c.PeerCallId,
			"Reserved":   0,
			"Send Accm":  c.SendAccm,
			"Recv Accm":  c.RecvAccm,
		},
	}
}

func (pptp *PPTPAuthItem) GetSetLinkInfo() map[string]any {
	return map[string]any{
		"Length":             24,
		"MessageType":        1,
		"MagicCookie":        0x1a2b3c4d,
		"ControlMessageType": PPTP_SER_LINK_INFO,
		"Reserved":           0,
		"Set Link Info": map[string]any{
			"PeerCallId": pptp.cfg.PeerCallId,
			"Reserved":   0,
			"Send Accm":  0xffffffff,
			"Recv Accm":  0xffffffff,
		},
	}
}

func (pptp *PPTPAuthItem) PPTPNegotiate() (net.Conn, error) {
	// connect pptp server
	conn, err := netx.DialX(pptp.Target, netx.DialX_WithTimeout(5*time.Second))
	if err != nil {
		return nil, err
	}

	startReq, err := parser.GenerateBinary(pptp.cfg.GetStartControlConnReq(), "application-layer.pptp", "PPTP")
	if err != nil {
		return nil, err
	}

	_, err = conn.Write(binparser.NodeToBytes(startReq))
	if err != nil {
		return nil, err
	}

	// read start control conn reply
	for {
		messageNode, err := parser.ParseBinary(conn, "application-layer.pptp", "PPTP")
		if err != nil {
			return nil, err
		}
		if messageNode.Name != "PPTP" {
			return nil, utils.Error("not PPP message")
		}

		messageMap := binparser.NodeToMap(messageNode).(map[string]any)

		pptpType, ok := base.GetSubData(messageMap, "ControlMessageType")
		if !ok {
			return nil, utils.Error("ControlMessageType not found")
		}

		switch pptpType.(uint16) {
		case PPTP_START_CONTROL_CONN_REP:
			outgoingReq, err := parser.GenerateBinary(pptp.cfg.GetOutgoingCallReq(), "application-layer.pptp", "PPTP")
			if err != nil {
				return nil, err
			}
			_, err = conn.Write(binparser.NodeToBytes(outgoingReq))
			if err != nil {
				return nil, err
			}
		case PPTP_OUTGOING_CALL_REP:
			var peerCallId uint16
			err = base.UnmarshalSubData(messageMap, "Message.Outgoing Call Reply.CallId", &peerCallId)
			if err != nil {
				return nil, utils.Error("peerCallId not found")
			}
			pptp.cfg.PeerCallId = peerCallId
			return conn, nil
		}
	}
}

func PPTPAuth(ctx context.Context, target, username, password string) (error, bool) {
	pptp := GetDefaultPPTPAuth()
	pptp.Target = target
	pptp.ppp.Username = username
	pptp.ppp.Password = password
	return pptp.auth(ctx)
}

func (pptp *PPTPAuthItem) auth(ctx context.Context) (error, bool) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	conn, err := pptp.PPTPNegotiate()
	if err != nil {
		return err, false
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
			default:
				setInfo, err := parser.GenerateBinary(pptp.cfg.GetSetLinkInfo(), "application-layer.pptp", "PPTP")
				if err != nil {
					log.Error(err)
				}
				_, err = conn.Write(binparser.NodeToBytes(setInfo))
				if err != nil {
					log.Error(err)
					cancel()
					return
				}
				time.Sleep(5 * time.Second)
			}
		}
	}()

	return pptp.Tunnel(ctx)
}

func (pptp *PPTPAuthItem) Tunnel(ctx context.Context) (error, bool) {
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
			pcaputil.WithContext(ctx),
			pcaputil.WithNetInterfaceCreated(func(handle *pcaputil.PcapHandleWrapper) {
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
						case <-ctx.Done():
							return
						}
					}
				}()
			}),
			pcaputil.WithEveryPacket(func(packet gopacket.Packet) {
				packetData := bytes.Clone(packet.Data())
				packetReader := bytes.NewReader(packetData)
				node, err := parser.ParseBinary(packetReader, "ethernet")
				if err != nil {
					log.Error(err)
					errorCh <- err
					return
				}

				GREnode := base.GetNodeByPath(node, "@Ethernet.Payload.IP.Payload.GRE")
				if GREnode == nil {
					log.Error("GRE node is nil")
					return
				}
				err = pptp.ProcessGre(GREnode, sendPacketCh)
				if err != nil {
					log.Error(err)
					errorCh <- err
					return
				}

			}),
		)
		if err != nil {
			errorCh <- err
		}
	}()

	packet, err := pptp.pppMapToPacket(pptp.ppp.GetPPPStartReqParams())
	if err != nil {
		return err, false
	}
	sendPacketCh <- packet

	go func() {
		for {
			select {
			case <-ctx.Done():
			case <-pptp.ppp.NegotiateOk:
				if bytes.Equal(pptp.ppp.AuthTypeCode, ppp.PAP) {
					packet, _ := pptp.pppMapToPacket(pptp.ppp.GetPAPReqParams())
					sendPacketCh <- packet
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
		return utils.Error("context done"), false
	case <-errorCh:
		return err, false
	case ok := <-pptp.ppp.AuthOk:
		return nil, ok
	}

}

func (pptp *PPTPAuthItem) pppMapToPacket(pppMap map[string]any) ([]byte, error) { // encapsulate ppp to ethernet packet
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

func (pptp *PPTPAuthItem) encapsulateGRE(payload map[string]any) (map[string]any, error) {
	res, err := parser.GenerateBinary(payload, "ppp", "PPP")
	if err != nil {
		return nil, err
	}
	payloadByte := binparser.NodeToBytes(res)
	snumber := pptp.SNumber
	pptp.SNumber = snumber + 2
	return map[string]any{
		"Flags And Version": 0x3001,
		"Protocol Type":     0x880b,
		"Payload Length":    len(payloadByte),
		"Call ID":           pptp.cfg.PeerCallId,
		"Sequence Number":   snumber,
		//"Acknowledgment Number": pptp.AckNumber,
		"Payload": map[string]any{
			"PPP": payloadByte,
		},
	}, nil

}

func (pptp *PPTPAuthItem) ProcessGre(messageNode *base.Node, replyChan chan []byte) error {
	if messageNode.Name != "GRE" {
		return utils.Error("not GRE message")
	}

	messageMap := binparser.NodeToMap(messageNode).(map[string]any)
	if messageMap["Call ID"].(uint16) != pptp.cfg.CallId {
		return nil
	}

	//snumber, ok := messageMap["Optional"].(map[string]any)["Sequence Number"]
	//if ok {
	//	pptp.AckNumber = int(snumber.(uint32))
	//}

	pppNode := base.GetNodeByPath(messageNode, "@GRE.Payload.PPP")
	if pppNode == nil {
		log.Infof("not ppp gre message")
		return nil
	}

	pppParamMap, err := pptp.ppp.ProcessMessage(pppNode)
	if err != nil || pppParamMap == nil {
		return err
	}

	packet, err := pptp.pppMapToPacket(pppParamMap)
	if err != nil {
		return err
	}
	replyChan <- packet
	return nil
}

func getDefaultIPV4Header() map[string]any {
	return map[string]any{
		"Version":                   4,
		"Header Length":             5,
		"Type of Service":           byte(0),
		"Identification":            []byte{0x00, 0x00},
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

func (pptp *PPTPAuthItem) encapsulateIPEthernet(payload map[string]any) (map[string]any, error) {
	res, err := parser.GenerateBinary(payload, "generic_routing_encapsulation", "GRE")
	if err != nil {
		return nil, err
	}
	payloadByte := binparser.NodeToBytes(res)
	totalLength := uint16(20 + len(payloadByte))
	checkSum := ipChecksum(totalLength, 0x2f, pptp.LAddrByte, pptp.RAddrByte)

	ipMap := getDefaultIPV4Header()
	ipMap["Total Length"] = totalLength
	ipMap["Header Checksum"] = checkSum
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
