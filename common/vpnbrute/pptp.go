package vpnbrute

import (
	"bytes"
	"context"
	"github.com/davecgh/go-spew/spew"
	binparser "github.com/yaklang/yaklang/common/bin-parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vpnbrute/ppp"
	"io"
	"net"
	"time"
)

type PPTPAuth struct {
	ppp       *ppp.PPPAuth
	PNSCallID uint16 // c -> s gre call id
	PASCallID uint16 // s -> c gre call id
	Target    string
	LocalAddr net.Addr
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
	conn, err := netx.DialX(pptp.Target, netx.DialX_WithTimeout(5*time.Second))
	if err != nil {
		return err
	}

	pptp.LocalAddr = conn.LocalAddr()

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
		pptpType := messageMap["ControlMessageType"]
		switch pptpType {
		case uint16(2):
			pptp.PASCallID = 1
			outgoingReq, err := parser.GenerateBinary(GetOutgoingCallReq(), "application-layer.pptp", "PPTP")
			if err != nil {
				return err
			}
			_, err = conn.Write(binparser.NodeToBytes(outgoingReq))
			if err != nil {
				return err
			}
		case uint16(8):
			pptp.PNSCallID = messageMap["Message"].(map[string]any)["Outgoing Call Reply"].(map[string]any)["CallId"].(uint16)
			go func() {
				for {
					time.Sleep(1)
					conn.Read(make([]byte, 1024))
				}
			}()
			// gre
			pptp.Tunnel(context.Background())

			return nil
		}
	}
}

func (pptp *PPTPAuth) Tunnel(ctx context.Context) {
	targetHost := utils.ExtractHost(pptp.Target)
	raddr, err := net.ResolveIPAddr("ip", targetHost)
	if err != nil {
		return
	}

	laddr, err := net.ResolveIPAddr("ip", utils.ExtractHost(pptp.LocalAddr.String()))
	if err != nil {
		return
	}

	ipConn, err := net.DialIP("ip:47", laddr, raddr)
	if err != nil {
		return
	}
	defer ipConn.Close()

	for {
		var buffer []byte
		data := bytes.NewBuffer(buffer)
		go func() {
			time.Sleep(10 * time.Millisecond)
			spew.Dump(data.Bytes())
		}()
		messageNode, err := parser.ParseBinary(io.TeeReader(ipConn, data), "internet_protocol")
		if err != nil {
			return
		}

		go func(processNode *base.Node) {
			respMap, err := pptp.ProcessGre(base.GetNodeByPath(processNode, "@Internet Protocol.Payload.GRE"))
			if err != nil {
				return
			}
			pptp.SendPPPByGRE(respMap, ipConn)
		}(messageNode)
	}

}

func (pptp *PPTPAuth) SendPPPByGRE(pppMap map[string]any, ipConn net.Conn) {
	reqMap, err := pptp.encapsulateGRE(pppMap)
	if err != nil {
		log.Error(err)
		return
	}
	resNode, err := parser.GenerateBinary(reqMap, "generic_routing_encapsulation", "GRE")
	if err != nil {
		log.Error(err)
	}
	_, err = ipConn.Write(binparser.NodeToBytes(resNode))
	if err != nil {
		log.Error(err)
	}
}

func (pptp *PPTPAuth) encapsulateGRE(payload map[string]any) (map[string]any, error) {
	res, err := parser.GenerateBinary(payload, "ppp", "PPP")
	if err != nil {
		return nil, err
	}
	payloadByte := binparser.NodeToBytes(res)
	return map[string]any{
		"Flags And Version": 0x3001,
		"Protocol Type":     0x880b,
		"Payload Length":    len(payloadByte),
		"Call ID":           pptp.PNSCallID,
		"Sequence Number":   0,
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
