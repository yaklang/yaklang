package ppp

import (
	"bytes"
	binparser "github.com/yaklang/yaklang/common/bin-parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	PPP_LCP  uint16 = 0xc021
	PPP_PAP  uint16 = 0xc023
	PPP_CHAP uint16 = 0xc223

	LCP_REQ uint8 = 0x1
	LCP_ACK uint8 = 0x2
	LCP_NCK uint8 = 0x3
	LCP_REJ uint8 = 0x4

	LCP_OPTION_AUTH uint8 = 0x3

	CHAP_MD5   = []byte{0xc2, 0x23, 0x05}
	MS_CHAP_V2 = []byte{0xc2, 0x23, 0x81}
	PAP        = []byte{0xc0, 0x23}

	CHAP_CHALLENGE uint8 = 0x1
	CHAP_SUCCESS   uint8 = 0x3
	CHAP_FAILURE   uint8 = 0x4

	PAP_ACK uint8 = 0x2
	PAP_NAK uint8 = 0x3

	SupportAuthTypeMap = map[string][]byte{
		"CHAP":     CHAP_MD5,
		"MSCHAPV2": MS_CHAP_V2,
		"PAP":      PAP,
	}
)

type PPPAuth struct {
	Username           string
	Password           string
	AuthTypeCode       []byte
	AuthTypeName       string
	CanUseAuthTypeList map[string][]byte
	AuthOk             chan bool
	NegotiateOk        chan struct{}
	MagicNumber        []byte

	recvAck bool
	sendAck bool // has send and recv ack ,High probability of successful negotiation

}

func GetDefaultPPPAuth() *PPPAuth {
	return &PPPAuth{
		AuthTypeCode:       CHAP_MD5,
		CanUseAuthTypeList: SupportAuthTypeMap,
		AuthTypeName:       "CHAP",
		AuthOk:             make(chan bool, 2),
		NegotiateOk:        make(chan struct{}, 2),
		MagicNumber:        []byte(utils.RandSecret(4)),
	}
}

func (p *PPPAuth) SetAuthType(authType []byte) error {
	if !p.AuthTypeCanUse(authType) {
		return utils.Error("no support auth type")
	}
	p.AuthTypeCode = authType
	return nil
}

func (p *PPPAuth) AuthTypeCanUse(authType []byte) bool {
	for _, code := range p.CanUseAuthTypeList {
		if bytes.Equal(code, authType) {
			return true
		}
	}
	return false
}

func (p *PPPAuth) ChangeAuthType(rejectType []byte) error {
	for name, code := range p.CanUseAuthTypeList {
		if bytes.Equal(code, rejectType) {
			delete(p.CanUseAuthTypeList, name)
			if len(p.CanUseAuthTypeList) <= 0 {
				return utils.Error("no auth type can use")
			}
			break
		}
	}
	if bytes.Equal(rejectType, p.AuthTypeCode) {
		for name, code := range p.CanUseAuthTypeList {
			p.AuthTypeName = name
			p.AuthTypeCode = code
			break
		}
	}
	return nil
}

func (p *PPPAuth) GetPPPStartReqParams() map[string]any {
	return map[string]any{
		"Address":  0xff,
		"Control":  0x03,
		"Protocol": PPP_LCP,
		//"Information": map[string]any{
		"LCP": p.GetLCPConfigReqParams(0),
		//"LCP": lcpByte,
		//},
	}
}

func (p *PPPAuth) GetPAPReqParams() map[string]any {
	return map[string]any{
		"Address":  0xff,
		"Control":  0x03,
		"Protocol": PPP_PAP,
		"PAP": map[string]any{
			"Code":       1,
			"Identifier": 0,
			"Length":     len(p.Username) + len(p.Password) + 6,
			"Request": map[string]any{
				"Peer ID Length":  len(p.Username),
				"Peer ID":         p.Username,
				"Password Length": len(p.Password),
				"Password":        p.Password,
			},
		},
	}
}

func (p *PPPAuth) GetLCPConfigReqParams(id int) map[string]any {
	return map[string]any{ // just negotiate Auth Type
		"Code":       1,
		"Identifier": id,
		"Length":     10,
		"Info": map[string]any{
			"Options": []map[string]any{
				{
					"Type":   5,
					"Length": 6,
					"Data":   p.MagicNumber,
				},
			},
		},
	}
}

func (p *PPPAuth) ProcessMessage(messageNode *base.Node) (map[string]any, error) {
	if messageNode.Name != "PPP" {
		return nil, utils.Error("not PPP message")
	}

	messageMap := binparser.NodeToMap(messageNode).(map[string]any)
	pppType := messageMap["Protocol"].(uint16)

	var resultParams = make(map[string]any)
	var err error
	var res any
	switch pppType {
	case PPP_LCP:
		res, err = p.ProcessLCPMessage(base.GetNodeByPath(messageNode, "Information.LCP"))
		resultParams["LCP"] = res
	case PPP_PAP:
		res, err = p.ProcessPAPMessage(base.GetNodeByPath(messageNode, "@PPP.PAP"))
		resultParams["PAP"] = res
	case PPP_CHAP:
		res, err = p.ProcessCHAPMessage(base.GetNodeByPath(messageNode, "Information.CHAP"))
		resultParams["CHAP"] = res
	}
	if funk.IsEmpty(res) {
		return nil, err
	}
	resultParams["Address"] = 0xff
	resultParams["Control"] = 0x03
	resultParams["Protocol"] = pppType
	return resultParams, err
}

func (p *PPPAuth) ProcessLCPMessage(messageNode *base.Node) (map[string]any, error) {
	if messageNode.Name != "LCP" {
		return nil, utils.Errorf("not LCP message")
	}
	messageMap, ok := binparser.NodeToMap(messageNode).(map[string]any)
	if !ok {
		return nil, utils.Error("process lcp message convert Node to map failed")
	}

	var lcpType, lcpId uint8
	err := base.UnmarshalSubData(messageMap, "Code", &lcpType)
	if err != nil {
		return nil, utils.Wrap(err, "lcp get lcp code failed")
	}
	err = base.UnmarshalSubData(messageMap, "Identifier", &lcpId)
	if err != nil {
		return nil, utils.Wrap(err, "lcp get lcp Identifier code failed")
	}

	if lcpType > 0x4 {
		return nil, nil
	}

	if lcpType == LCP_ACK { // ack not need send resp message
		p.recvAck = true
		if p.sendAck {
			p.NegotiateOk <- struct{}{}
		}
		return nil, nil
	}

	var options []any
	err = base.UnmarshalSubData(messageMap, "Info.Options", &options)
	if err != nil {
		return nil, utils.Wrap(err, "lcp get lcp options failed")
	}

	var authCode []byte
	var hasAuthCode bool
	for _, option := range options {
		option, ok := option.(map[string]any)
		if !ok {
			return nil, utils.Error("convert lcp option to map failed")
		}
		if option["Type"] == LCP_OPTION_AUTH {
			hasAuthCode = true
			err = base.UnmarshalSubData(option, "Data", &authCode)
			if err != nil {
				return nil, utils.Wrap(err, "lcp get auth code failed")
			}
		}
	}
	if lcpType == LCP_REJ {
		err := p.ChangeAuthType(authCode)
		if err != nil {
			return nil, err
		}
		return p.GetLCPConfigReqParams(int(lcpId) + 1), nil
	}

	if hasAuthCode {
		err := p.SetAuthType(authCode)
		if err != nil {
			return nil, err
		}
	}

	switch lcpType {
	case LCP_REQ: // req
		p.sendAck = true
		if p.recvAck {
			p.NegotiateOk <- struct{}{}
		}
		messageMap["Code"] = LCP_ACK
		return messageMap, nil
	case LCP_NCK: // nak
		messageMap["Code"] = LCP_REQ
		return messageMap, nil
	}
	return nil, nil // not process no support message
}

func (p *PPPAuth) ProcessPAPMessage(messageNode *base.Node) (map[string]any, error) {
	if messageNode.Name != "PAP" {
		return nil, utils.Errorf("not PAP message")
	}

	messageMap, ok := binparser.NodeToMap(messageNode).(map[string]any)
	if !ok {
		return nil, utils.Error("convert PAP message to map failed")
	}

	var papType uint8
	err := base.UnmarshalSubData(messageMap, "Code", &papType)
	if err != nil {
		return nil, utils.Wrap(err, "CHAP get CHAP AUTH code failed")
	}

	switch papType {
	case PAP_ACK:
		p.AuthOk <- true
		return nil, nil
	case PAP_NAK:
		p.AuthOk <- false
		return nil, nil
	}
	return nil, nil
}

func (p *PPPAuth) ProcessCHAPMessage(messageNode *base.Node) (map[string]any, error) {
	if messageNode.Name != "CHAP" {
		return nil, utils.Errorf("not CHAP message")
	}

	messageMap, ok := binparser.NodeToMap(messageNode).(map[string]any)
	if !ok {
		return nil, utils.Error("convert CHAP message to map failed")
	}

	var chapType, id uint8
	err := base.UnmarshalSubData(messageMap, "Code", &chapType)
	if err != nil {
		return nil, utils.Wrap(err, "CHAP get CHAP AUTH code failed")
	}
	err = base.UnmarshalSubData(messageMap, "Identifier", &id)
	if err != nil {
		return nil, utils.Wrap(err, "CHAP get CHAP Identifier code failed")
	}

	switch chapType {
	case CHAP_CHALLENGE: // req - challenge
		var challenge []byte
		err = base.UnmarshalSubData(messageMap, "Info.Data.Value", &challenge)
		if err != nil {
			return nil, utils.Wrap(err, "CHAP get CHAP challenge failed")
		}
		response, err := GenerateCHAPResponse([]byte{id}, challenge, []byte(p.Username), []byte(p.Password), p.AuthTypeCode)
		if err != nil {
			return nil, utils.Wrap(err, "CHAP generate CHAP response failed")
		}
		return map[string]any{
			"Code":       2,
			"Identifier": id,
			"Length":     5 + len(response) + len(p.Username),
			"Data": map[string]any{
				"Value Size": len(response),
				"Value":      response,
				"Name":       []byte(p.Username),
			},
		}, nil
	case CHAP_SUCCESS: // success
		p.AuthOk <- true
		return nil, nil
	case CHAP_FAILURE: // failure
		p.AuthOk <- false
		return nil, nil
	}

	return nil, nil
}
