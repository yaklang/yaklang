package license

import (
	"encoding/base64"
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"io/ioutil"
	"strings"
	"time"
)

const SignedLicenseV2Prefix = "legion-v2"

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

// SignLicenseV2 issues an authenticated Legion license without changing the
// legacy license format used by existing yaklang products.
func (m *Machine) SignLicenseV2(reqRaw string, org string, duration time.Duration, params map[string]string) (string, error) {
	reqPlaintext, err := tlsutils.Decrypt(reqRaw, m.decryptPriPEM)
	if err != nil {
		return "", utils.Errorf("decrypt license request failed: %s", err)
	}
	var req Request
	if err := json.Unmarshal(reqPlaintext, &req); err != nil {
		return "", utils.Errorf("unmarshal request failed: %s", err)
	}
	rsp := Response{
		Org:               org,
		NotAfterTimestamp: time.Now().Add(duration).Unix(),
		Params:            params,
		MachineCode:       req.MachineCode,
	}
	payload, err := json.Marshal(rsp)
	if err != nil {
		return "", utils.Errorf("marshal response failed: %s", err)
	}
	signature, err := tlsutils.PemSignSha256WithRSA(m.decryptPriPEM, payload)
	if err != nil {
		return "", utils.Errorf("sign license failed: %s", err)
	}
	return strings.Join([]string{
		SignedLicenseV2Prefix,
		base64.RawURLEncoding.EncodeToString(payload),
		base64.RawURLEncoding.EncodeToString(signature),
	}, "."), nil
}

// VerifyLicenseV2 verifies the additive Legion format with the public key.
// Legacy callers continue to use VerifyLicense unchanged.
func (m *Machine) VerifyLicenseV2(licenseRaw string) (*Response, error) {
	parts := strings.Split(licenseRaw, ".")
	if len(parts) != 3 || parts[0] != SignedLicenseV2Prefix {
		return nil, utils.Errorf("invalid signed license format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, utils.Errorf("decode license payload failed: %s", err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, utils.Errorf("decode license signature failed: %s", err)
	}
	if err := tlsutils.PemVerifySignSha256WithRSA(m.encryptPubPEM, payload, signature); err != nil {
		return nil, utils.Errorf("license signature verification failed: %s", err)
	}
	var rsp Response
	if err := json.Unmarshal(payload, &rsp); err != nil {
		return nil, utils.Errorf("unmarshal response failed: %s", err)
	}
	if m.MachineCode != rsp.MachineCode {
		return nil, utils.Errorf("invalid license")
	}
	if !time.Unix(rsp.NotAfterTimestamp, 0).After(time.Now()) {
		return &rsp, utils.Errorf("expired license")
	}
	return &rsp, nil
}

func (m *Machine) GenerateRequest() (string, error) {
	code := utils.GetMachineCode()

	log.Infof("generate with machine code: %v", code)
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
