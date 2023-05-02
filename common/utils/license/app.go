package license

import (
	"encoding/json"
	"io/ioutil"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/tlsutils"
)

type Request struct {
	Timestamp   int64  `json:"timestamp"`
	MachineCode string `json:"machine_code"`
}

type Response struct {
	Org               string            `json:"org"`
	NotAfterTimestamp int64             `json:"not_after_timestamp"`
	Params            map[string]string `json:"params"`
	MachineCode       string            `json:"machine_code"`
}

type Machine struct {
	encryptPubPEM []byte
	decryptPriPEM []byte
	MachineCode   string
}

func (m *Machine) VerifyLicense(license string) (*Response, error) {
	//return &Response{Org: "123", NotAfterTimestamp: time.Now().Add(time.Hour * 24 * 365).Unix(), Params: map[string]string{"a": "b"}}, nil
	raw, err := tlsutils.Decrypt(license, m.decryptPriPEM)
	if err != nil {
		return nil, utils.Errorf("decrypt license failed: %s", err)
	}

	var rsp Response
	err = json.Unmarshal(raw, &rsp)
	if err != nil {
		return nil, utils.Errorf("marshal response failed: %s", err)
	}

	if m.MachineCode != rsp.MachineCode {
		log.Errorf("invalid license for current machine: %v", m.MachineCode)
		return nil, utils.Errorf("invalid license")
	}

	if time.Unix(rsp.NotAfterTimestamp, 0).After(time.Now()) {
		return &rsp, nil
	}

	return nil, utils.Errorf("expired license")
}

func (m *Machine) SignLicense(reqRaw string, org string, duration time.Duration, params map[string]string) (string, error) {
	raw, err := tlsutils.Decrypt(reqRaw, m.decryptPriPEM)
	if err != nil {
		return "", utils.Errorf("decrypt license request failed: %s", err)
	}

	var req Request
	err = json.Unmarshal(raw, &req)
	if err != nil {
		return "", utils.Errorf("unmarshal request failed: %s", err)
	}

	rsp := Response{
		Org:               org,
		NotAfterTimestamp: time.Unix(req.Timestamp, 0).Add(duration).Unix(),
		Params:            params,
		MachineCode:       req.MachineCode,
	}
	raw, err = json.Marshal(rsp)
	if err != nil {
		return "", utils.Errorf("marshal response failed: %s", err)
	}
	return tlsutils.Encrypt(raw, m.encryptPubPEM)
}

func (m *Machine) GenerateRequest() (string, error) {
	code := utils.GetMachineCode()

	log.Infof("generate with machine code: %", code)
	req := &Request{
		Timestamp:   time.Now().Unix(),
		MachineCode: code,
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return "", utils.Errorf("marshal license req failed: %s", err)
	}

	return tlsutils.Encrypt(raw, m.encryptPubPEM)
}

func NewMachine(pubPem, priPem []byte) *Machine {
	return &Machine{
		encryptPubPEM: pubPem,
		decryptPriPEM: priPem,
		MachineCode:   utils.GetMachineCode(),
	}
}

func NewMachineFromFile(pubFile, priFile string) (*Machine, error) {
	pubPem, err := ioutil.ReadFile(pubFile)
	if err != nil {
		return nil, err
	}

	priPem, err := ioutil.ReadFile(priFile)
	if err != nil {
		return nil, err
	}

	return NewMachine(pubPem, priPem), nil
}
