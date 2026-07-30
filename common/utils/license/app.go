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

// License format (v2 — signed):
//
//	license = base64(jsonResponse) + "." + base64(rsaSignPKCS1v15(sha256(jsonResponse)))
//
// The verifier only needs the PUBLIC key. The signer holds the PRIVATE key.
// This fixes the v1 design flaw where the verifier had to hold the private key
// (because verification was RSA-decrypt-with-private-key), which meant the
// private key was embedded in every shipped binary and anyone could self-sign.
//
// The license request blob (from GenerateRequest) is still pub-encrypted so
// only the signer can read the machine code — that direction is correct and
// unchanged.

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

// Machine holds the keypair used for license signing/verification.
//
// In a production client (verifier-only), signPriPEM is nil and only the
// public key is present. In the off-box signing tool (xlic) and in tests,
// signPriPEM holds the private key for signing.
type Machine struct {
	encryptPubPEM []byte // public key — used by GenerateRequest (encrypt) + VerifyLicense (verify signature)
	signPriPEM    []byte // private key — used by SignLicense (sign) + decryptRequest; nil in prod client
	MachineCode   string
}

// NewVerifier builds a verification-only Machine from the public key. This is
// the production path: the shipped binary holds only the public key and can
// verify licenses but never sign them.
func NewVerifier(pubPem []byte) *Machine {
	return &Machine{
		encryptPubPEM: pubPem,
		MachineCode:   utils.GetMachineCode(),
	}
}

func (m *Machine) VerifyLicense(license string) (*Response, error) {
	// license = base64(jsonResponse) + "." + base64(signature)
	parts := strings.SplitN(license, ".", 2)
	if len(parts) != 2 {
		return nil, utils.Errorf("invalid license format: expected 'payload.signature'")
	}
	raw, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, utils.Errorf("decode license payload: %s", err)
	}
	sig, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, utils.Errorf("decode license signature: %s", err)
	}

	// Verify the signature with the public key.
	if err := tlsutils.PemVerifySignSha256WithRSA(m.encryptPubPEM, raw, sig); err != nil {
		return nil, utils.Errorf("license signature verification failed: %s", err)
	}

	var rsp Response
	if err := json.Unmarshal(raw, &rsp); err != nil {
		return nil, utils.Errorf("unmarshal response failed: %s", err)
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
	// Decrypt the incoming request with the private key to recover machine code.
	reqPlaintext, err := tlsutils.Decrypt(reqRaw, m.signPriPEM)
	if err != nil {
		return "", utils.Errorf("decrypt license request failed: %s", err)
	}

	var req Request
	err = json.Unmarshal(reqPlaintext, &req)
	if err != nil {
		return "", utils.Errorf("unmarshal request failed: %s", err)
	}

	rsp := Response{
		Org:               org,
		NotAfterTimestamp: time.Unix(req.Timestamp, 0).Add(duration).Unix(),
		Params:            params,
		MachineCode:       req.MachineCode,
	}
	raw, err := json.Marshal(rsp)
	if err != nil {
		return "", utils.Errorf("marshal response failed: %s", err)
	}

	// Sign the JSON with the private key (SHA256 + PKCS#1 v1.5).
	sig, err := tlsutils.PemSignSha256WithRSA(m.signPriPEM, raw)
	if err != nil {
		return "", utils.Errorf("sign license failed: %s", err)
	}

	// license = base64(jsonResponse) + "." + base64(signature)
	return base64.StdEncoding.EncodeToString(raw) + "." + base64.StdEncoding.EncodeToString(sig), nil
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

// NewMachine builds a full Machine from both key halves. The private key is
// required for SignLicense; if only verification is needed use NewVerifier.
// Kept for backward compatibility with callers (tests, xlic CLI) that pass both.
func NewMachine(pubPem, priPem []byte) *Machine {
	return &Machine{
		encryptPubPEM: pubPem,
		signPriPEM:    priPem,
		MachineCode:   utils.GetMachineCode(),
	}
}

// NewSignerFromFile builds a signing-capable Machine by loading both keys from
// disk. Used by the xlic signing CLI. The public key encrypts the request; the
// private key decrypts the request and signs the license.
func NewSignerFromFile(pubFile, priFile string) (*Machine, error) {
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

// NewMachineFromFile is kept for backward compatibility but delegates to
// NewSignerFromFile. Prefer NewSignerFromFile (signer) or NewVerifier (verifier)
// to make the key role explicit.
func NewMachineFromFile(pubFile, priFile string) (*Machine, error) {
	return NewSignerFromFile(pubFile, priFile)
}
