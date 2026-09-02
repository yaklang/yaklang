package nla

import (
	"crypto/sha256"
	"encoding/asn1"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
)

// CredSSP protocol versions (MS-CSSP).
// v5+ switches pubKeyAuth from encrypted SubjectPublicKey to a SHA-256
// binding hash (CVE-2018-0886). Windows 10/2016+ speak v5 or v6.
const (
	CredSSPVersion2       = 2
	CredSSPVersion5       = 5
	CredSSPVersion6       = 6
	DefaultCredSSPVersion = CredSSPVersion6
)

const maxDERFrameSize = 65536

const (
	clientServerHashMagic = "CredSSP Client-To-Server Binding Hash\x00"
	serverClientHashMagic = "CredSSP Server-To-Client Binding Hash\x00"
)

type NegoToken struct {
	Data []byte `asn1:"explicit,tag:0"`
}

type TSRequest struct {
	Version     int         `asn1:"explicit,tag:0"`
	NegoTokens  []NegoToken `asn1:"optional,explicit,tag:1"`
	AuthInfo    []byte      `asn1:"optional,explicit,tag:2"`
	PubKeyAuth  []byte      `asn1:"optional,explicit,tag:3"`
	ErrorCode   int64       `asn1:"optional,explicit,tag:4"`
	ClientNonce []byte      `asn1:"optional,explicit,tag:5"`
}

type TSCredentials struct {
	CredType    int    `asn1:"explicit,tag:0"`
	Credentials []byte `asn1:"explicit,tag:1"`
}

type TSPasswordCreds struct {
	DomainName []byte `asn1:"explicit,tag:0"`
	UserName   []byte `asn1:"explicit,tag:1"`
	Password   []byte `asn1:"explicit,tag:2"`
}

type TSCspDataDetail struct {
	KeySpec       int    `asn1:"explicit,tag:0"`
	CardName      string `asn1:"explicit,tag:1"`
	ReaderName    string `asn1:"explicit,tag:2"`
	ContainerName string `asn1:"explicit,tag:3"`
	CspName       string `asn1:"explicit,tag:4"`
}

type TSSmartCardCreds struct {
	Pin        string            `asn1:"explicit,tag:0"`
	CspData    []TSCspDataDetail `asn1:"explicit,tag:1"`
	UserHint   string            `asn1:"explicit,tag:2"`
	DomainHint string            `asn1:"explicit,tag:3"`
}

// CredSSPError is a server-side NTSTATUS carried in TSRequest.errorCode
// (CredSSP v3/v4/v6). Wrong-password brute attempts typically surface as
// STATUS_LOGON_FAILURE rather than a TLS/EOF hang.
type CredSSPError struct {
	Code uint32
}

func (e *CredSSPError) Error() string {
	return fmt.Sprintf("rdp nla: CredSSP NTSTATUS 0x%08X (%s)", e.Code, ntstatusName(e.Code))
}

func (e *CredSSPError) AccountLocked() bool { return e.Code == statusAccountLockedOut }
func (e *CredSSPError) AuthFailed() bool {
	switch e.Code {
	case statusLogonFailure, statusWrongPassword, statusNoSuchUser,
		statusAccountDisabled, statusAccountRestriction, statusAccessDenied,
		statusPasswordExpired, secELogonDenied, secEDelegationPolicy:
		return true
	default:
		return e.Code != 0
	}
}

const (
	statusAccessDenied       = 0xC0000022
	statusNoSuchUser         = 0xC0000064
	statusWrongPassword      = 0xC000006A
	statusLogonFailure       = 0xC000006D
	statusAccountRestriction = 0xC000006E
	statusPasswordExpired    = 0xC0000071
	statusAccountDisabled    = 0xC0000072
	statusAccountLockedOut   = 0xC0000234
	statusInvalidServerState = 0xC00000DC
	secELogonDenied          = 0x8009030C
	secEDelegationPolicy     = 0x80090346
)

func ntstatusName(code uint32) string {
	switch code {
	case statusAccessDenied:
		return "STATUS_ACCESS_DENIED"
	case statusNoSuchUser:
		return "STATUS_NO_SUCH_USER"
	case statusWrongPassword:
		return "STATUS_WRONG_PASSWORD"
	case statusLogonFailure:
		return "STATUS_LOGON_FAILURE"
	case statusAccountRestriction:
		return "STATUS_ACCOUNT_RESTRICTION"
	case statusPasswordExpired:
		return "STATUS_PASSWORD_EXPIRED"
	case statusAccountDisabled:
		return "STATUS_ACCOUNT_DISABLED"
	case statusAccountLockedOut:
		return "STATUS_ACCOUNT_LOCKED_OUT"
	case statusInvalidServerState:
		return "STATUS_INVALID_SERVER_STATE"
	case secELogonDenied:
		return "SEC_E_LOGON_DENIED"
	case secEDelegationPolicy:
		return "SEC_E_DELEGATION_POLICY"
	default:
		return "unknown"
	}
}

// AuthError returns a CredSSPError when the TSRequest carries a non-zero
// errorCode; otherwise nil.
func (t *TSRequest) AuthError() error {
	if t == nil || t.ErrorCode == 0 {
		return nil
	}
	return &CredSSPError{Code: uint32(t.ErrorCode)}
}

// EffectiveVersion is min(client, server) as specified by MS-CSSP.
func EffectiveVersion(client, server int) int {
	if client < server {
		return client
	}
	return server
}

// ComputePubKeyHash is the CredSSP v5+ binding:
// SHA256(magic || nonce || SubjectPublicKey). magic MUST include the NUL.
func ComputePubKeyHash(clientToServer bool, nonce, pubkey []byte) []byte {
	magic := serverClientHashMagic
	if clientToServer {
		magic = clientServerHashMagic
	}
	h := sha256.New()
	h.Write([]byte(magic))
	h.Write(nonce)
	h.Write(pubkey)
	return h.Sum(nil)
}

// EncodeDERTRequest encodes a CredSSP TSRequest.
// version 0 means DefaultCredSSPVersion (6). clientNonce is sent for v5+.
func EncodeDERTRequest(version int, msgs []Message, authInfo, pubKeyAuth, clientNonce []byte) []byte {
	if version == 0 {
		version = DefaultCredSSPVersion
	}
	req := TSRequest{Version: version}

	if len(msgs) > 0 {
		req.NegoTokens = make([]NegoToken, 0, len(msgs))
		for _, msg := range msgs {
			req.NegoTokens = append(req.NegoTokens, NegoToken{msg.Serialize()})
		}
	}
	if len(authInfo) > 0 {
		req.AuthInfo = authInfo
	}
	if len(pubKeyAuth) > 0 {
		req.PubKeyAuth = pubKeyAuth
	}
	if len(clientNonce) > 0 {
		req.ClientNonce = clientNonce
	}

	result, err := asn1.Marshal(req)
	if err != nil {
		glog.Error(err)
	}
	return result
}

func DecodeDERTRequest(s []byte) (*TSRequest, error) {
	treq := &TSRequest{}
	_, err := asn1.Unmarshal(s, treq)
	return treq, err
}

// EncodeTSRequestError builds a TSRequest that carries only version + errorCode
// (the v6 NLA failure signal used by Windows 10/2016+).
func EncodeTSRequestError(version int, code uint32) []byte {
	if version == 0 {
		version = DefaultCredSSPVersion
	}
	result, err := asn1.Marshal(TSRequest{Version: version, ErrorCode: int64(code)})
	if err != nil {
		glog.Error(err)
	}
	return result
}

// ReadTSRequest reads one complete DER-encoded TSRequest from r.
// A single Conn.Read is not enough: Windows NTLM Challenge TSRequests
// routinely exceed one TLS record and 1024 bytes.
func ReadTSRequest(r io.Reader) (*TSRequest, error) {
	frame, err := readDERFrame(r)
	if err != nil {
		return nil, err
	}
	return DecodeDERTRequest(frame)
}

func readDERFrame(r io.Reader) ([]byte, error) {
	tag := make([]byte, 1)
	if _, err := io.ReadFull(r, tag); err != nil {
		return nil, fmt.Errorf("rdp nla: read DER tag: %w", err)
	}
	lenBuf := make([]byte, 1)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, fmt.Errorf("rdp nla: read DER length: %w", err)
	}

	header := []byte{tag[0], lenBuf[0]}
	var totalLen int
	if lenBuf[0] < 0x80 {
		totalLen = int(lenBuf[0])
	} else if lenBuf[0] == 0x80 {
		return nil, fmt.Errorf("rdp nla: indefinite DER length not supported")
	} else {
		numLenBytes := int(lenBuf[0] & 0x7f)
		if numLenBytes == 0 || numLenBytes > 4 {
			return nil, fmt.Errorf("rdp nla: DER length too large: %d bytes", numLenBytes)
		}
		lenBytes := make([]byte, numLenBytes)
		if _, err := io.ReadFull(r, lenBytes); err != nil {
			return nil, fmt.Errorf("rdp nla: read DER length bytes: %w", err)
		}
		header = append(header, lenBytes...)
		for _, b := range lenBytes {
			totalLen = (totalLen << 8) | int(b)
		}
	}
	if totalLen < 0 || totalLen > maxDERFrameSize {
		return nil, fmt.Errorf("rdp nla: DER frame too large: %d bytes", totalLen)
	}
	value := make([]byte, totalLen)
	if _, err := io.ReadFull(r, value); err != nil {
		return nil, fmt.Errorf("rdp nla: read DER value: %w", err)
	}
	out := make([]byte, 0, len(header)+totalLen)
	out = append(out, header...)
	out = append(out, value...)
	return out, nil
}

func EncodeDERTCredentials(domain, username, password []byte) []byte {
	tpas := TSPasswordCreds{domain, username, password}
	result, err := asn1.Marshal(tpas)
	if err != nil {
		glog.Error(err)
	}
	tcre := TSCredentials{1, result}
	result, err = asn1.Marshal(tcre)
	if err != nil {
		glog.Error(err)
	}
	return result
}

func DecodeDERTCredentials(s []byte) (*TSCredentials, error) {
	tcre := &TSCredentials{}
	_, err := asn1.Unmarshal(s, tcre)
	return tcre, err
}
