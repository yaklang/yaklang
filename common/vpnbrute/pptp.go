package vpnbrute

import (
	"bytes"
	"context"
	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	binparser "github.com/yaklang/yaklang/common/bin-parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"github.com/yaklang/yaklang/common/vpnbrute/ppp"
	"time"
)

type PPTPAuth struct {
	ppp       ppp.PPPAuth
	PNSCallID uint16 // c -> s gre call id
	PASCallID uint16 // s -> c gre call id
	target    string
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
			"Vendor":              bytes.Repeat([]byte{0x0}, 64),
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
			"CallId":            1,
			"CallSerialNumber":  0,
			"MinimumBPS":        0x8000,
			"MaximumBPS":        0x80000000,
			"BearerType":        0x1,
			"FramingType":       0x1,
			"RecvWindowSize":    0x0a,
			"ProcessingDelay":   0,
			"PhoneNumberLength": 0,
			"Reserved":          0,
			"PhoneNumber":       bytes.Repeat([]byte{0x0}, 64),
			"SubAddress":        bytes.Repeat([]byte{0x0}, 64),
		},
	}
}

func (pptp *PPTPAuth) Auth() error {
	conn, err := netx.DialX(pptp.target, netx.DialX_WithTimeout(5*time.Second))
	if err != nil {
		return err
	}

	startReq, err := parser.GenerateBinary(GetStartControlConnReq(), "application-layer.pptp", "PPTP")
	if err != nil {
		return err
	}

	_, err = conn.Write(binparser.NodeToBytes(startReq))
	if err != nil {
		return err
	}

	// read start control conn reply
	for {
		messageNode, err := parser.ParseBinary(conn, "application-layer.pptp", "PPTP")
		if err != nil {
			return err
		}
		if messageNode.Name != "PPTP" {
			return utils.Error("not PPP message")
		}

		messageMap := binparser.NodeToMap(messageNode).(map[string]any)
		pptpType := messageMap["ControlMessageType"].(uint16)
		switch pptpType {
		case 2:
			outgoingReq, err := parser.GenerateBinary(GetOutgoingCallReq(), "application-layer.pptp", "PPTP")
			if err != nil {
				return err
			}
			_, err = conn.Write(binparser.NodeToBytes(outgoingReq))
			if err != nil {
				return err
			}
		case 4:
			pptp.PNSCallID = messageMap["Outgoing Call Reply"].(map[string]any)["CallId"].(uint16)
			// gre
		}
	}
}

func (pptp *PPTPAuth) Tunnel(ctx context.Context) {

	target := utils.ExtractHost(pptp.target)
	iface, _, _, err := netutil.Route(5*time.Second, target)
	if err != nil {

	}

	sendChan := make(chan []byte, 10)
	//recvChan := make(chan []byte, 10)

	go func() {
		_ = pcaputil.Start(
			pcaputil.WithDevice(iface.Name),
			pcaputil.WithEnableCache(true),
			pcaputil.WithBPFFilter("GRE"),
			pcaputil.WithContext(ctx),
			pcaputil.WithNetInterfaceCreated(func(handle *pcap.Handle) {
				go func() {
					for {
						select {
						case <-ctx.Done():
						case packet, ok := <-sendChan:
							if !ok {
								continue
							}
							packet = append([]byte{0x0, 0x0, 0x0, 0x0}, packet...)

						}
					}
				}()
			}),
			pcaputil.WithEveryPacket(func(packet gopacket.Packet) {

			}),
		)

	}()

}

func (pptp *PPTPAuth) encapsulateGRE(payload map[string]any) (map[string]any, error) {
	res, err := parser.GenerateBinary(payload, "ppp", "PPP")
	if err != nil {
		return nil, err
	}
	payloadByte := binparser.NodeToBytes(res)
	return map[string]any{
		"Flags And Version": 0x3081,
		"Protocol Type":     0x880b,
		"Payload Length":    len(payloadByte),
		"Call ID":           pptp.PNSCallID,
		"Number":            0,
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
	return pptp.encapsulateGRE(pppParamMap)
}
