package ppp

import (
	"bytes"
	binparser "github.com/yaklang/yaklang/common/bin-parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	LCPTypeReq         uint8 = 0x1
	LCPTypeAck         uint8 = 0x2
	LCPTypeNak         uint8 = 0x3
	LCPTypeRej         uint8 = 0x4
	CHAP_MD5                 = []byte{0xc2, 0x23, 0x05}
	MS_CHAP_V2               = []byte{0xc2, 0x23, 0x80}
	PAP                      = []byte{0xc0, 0x23}
	SupportAuthTypeMap       = map[string][]byte{
		"CHAP":     {0xc2, 0x23, 0x05},
		"MSCHAPV2": {0xc2, 0x23, 0x80},
		"PAP":      {0xc0, 0x23},
	}
)

type PPPAuth struct {
	Username           string
	Password           string
	AuthTypeCode       []byte
	AuthTypeName       string
	CanUseAuthTypeList map[string][]byte
	AuthOk             chan bool
	MagicNumber        []byte
}

func GetDefaultPPPAuth() *PPPAuth {
	return &PPPAuth{
		AuthTypeCode:       CHAP_MD5,
		CanUseAuthTypeList: SupportAuthTypeMap,
		AuthTypeName:       "CHAP",
		AuthOk:             make(chan bool, 2),
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

func (p *PPPAuth) GetPPPReqParams() map[string]any {
	//lcpByte, _ := codec.DecodeHex("0100001501040578050626a73d32070208020d0306")
	return map[string]any{
		"Address":  0xff,
		"Control":  0x03,
		"Protocol": 0xc021,
		//"Information": map[string]any{
		"LCP": p.GetLCPConfigReqParams(0),
		//"LCP": lcpByte,
		//},
	}
}

func (p *PPPAuth) GetLCPConfigReqParams(id int) map[string]any {
	return map[string]any{ // just negotiate Auth Type
		"Code":       1,
		"Identifier": id,
		"Length":     12 + len(p.AuthTypeCode),
		"Info": map[string]any{
			"Options": []map[string]any{
				{
					"Type":   3,
					"Length": len(p.AuthTypeCode) + 2,
					"Data":   p.AuthTypeCode,
				},
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
	case 0xc021:
		res, err = p.ProcessLCPMessage(base.GetNodeByPath(messageNode, "Information.LCP"))
		resultParams["LCP"] = res
	//case 0xc023:
	//	res, err = p.ProcessPAPMessage(base.GetNodeByPath(messageNode, "@PPP.PAP"))
	//	resultParams["PAP"] = res
	case 0xc223:
		res, err = p.ProcessCHAPMessage(base.GetNodeByPath(messageNode, "Information.CHAP"))
		resultParams["CHAP"] = res
	}
	if len(resultParams) == 0 {
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
	messageMap := binparser.NodeToMap(messageNode).(map[string]any)
	lcpType := messageMap["Code"].(uint8)
	if lcpType == LCPTypeAck { // ack
		return nil, nil
	}
	lcpId := messageMap["Identifier"].(uint8)

	options := messageMap["Info"].(map[string]any)["Options"].([]any)

	var authCode []byte
	var hasAuthCode bool
	for _, option := range options {
		option := option.(map[string]any)
		if option["Type"] == uint8(3) {
			hasAuthCode = true
			authCode = option["Data"].([]byte)
		}
	}
	if lcpType == LCPTypeRej {
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
	case LCPTypeReq: // req
		messageMap["Code"] = LCPTypeAck
		return messageMap, nil
	case LCPTypeNak: // nak
		messageMap["Code"] = LCPTypeReq
		return messageMap, nil
	}
	return nil, nil // not process no support message
}

func (p *PPPAuth) ProcessPAPMessage(messageNode *base.Node) {

}

func (p *PPPAuth) ProcessCHAPMessage(messageNode *base.Node) (map[string]any, error) {
	if messageNode.Name != "CHAP" {
		return nil, utils.Errorf("not LCP message")
	}

	messageMap := binparser.NodeToMap(messageNode).(map[string]any)

	chapType := messageMap["Code"].(uint8)
	id := messageMap["Identifier"].([]byte)
	switch chapType {
	case 1: // req - challenge
		challenge := messageMap["Data"].(map[string]any)["Value"].([]byte)
		response, err := GenerateCHAPResponse(id, challenge, []byte(p.Username), []byte(p.Password), p.AuthTypeCode)
		if err != nil {
			return nil, err
		}
		messageMap["Code"] = 2
		messageMap["Data"] = map[string]any{
			"Value Size": len(response),
			"Value":      response,
			"Name":       []byte(p.Username),
		}
		return messageMap, nil
	case 3: // success
		p.AuthOk <- true
		return nil, nil
	case 4: // failure
		p.AuthOk <- false
		return nil, nil
	}

	return nil, nil
}
