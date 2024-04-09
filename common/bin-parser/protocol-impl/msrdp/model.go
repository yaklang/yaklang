package msrdp

import (
	"bytes"
	"github.com/lunixbochs/struc"
)

type NegotiationType byte

const (
	TPDU_CONNECTION_REQUEST                                 = 0xE0
	TPDU_CONNECTION_CONFIRM                                 = 0xD0
	TPDU_DISCONNECT_REQUEST                                 = 0x80
	TPDU_DATA                                               = 0xF0
	TPDU_ERROR                                              = 0x70
	TYPE_RDP_NEG_REQ                        NegotiationType = 0x01
	TYPE_RDP_NEG_RSP                                        = 0x02
	TYPE_RDP_NEG_FAILURE                                    = 0x03
	RESTRICTED_ADMIN_MODE_REQUIRED                          = 0x01
	REDIRECTED_AUTHENTICATION_MODE_REQUIRED                 = 0x02
	CORRELATION_INFO_PRESENT                                = 0x03
	PROTOCOL_RDP                            uint32          = 0x00000000
	PROTOCOL_SSL                                            = 0x00000001
	PROTOCOL_HYBRID                                         = 0x00000002
	PROTOCOL_RDSTLS                                         = 0x00000004
	PROTOCOL_HYBRID_EX                                      = 0x00000008
	PROTOCOL_RDSAAD                                         = 0x00000010
)
const (
	NTLMSSP_NEGOTIATE_56                       = 0x80000000
	NTLMSSP_NEGOTIATE_KEY_EXCH                 = 0x40000000
	NTLMSSP_NEGOTIATE_128                      = 0x20000000
	NTLMSSP_NEGOTIATE_VERSION                  = 0x02000000
	NTLMSSP_NEGOTIATE_TARGET_INFO              = 0x00800000
	NTLMSSP_REQUEST_NON_NT_SESSION_KEY         = 0x00400000
	NTLMSSP_NEGOTIATE_IDENTIFY                 = 0x00100000
	NTLMSSP_NEGOTIATE_EXTENDED_SESSIONSECURITY = 0x00080000
	NTLMSSP_TARGET_TYPE_SERVER                 = 0x00020000
	NTLMSSP_TARGET_TYPE_DOMAIN                 = 0x00010000
	NTLMSSP_NEGOTIATE_ALWAYS_SIGN              = 0x00008000
	NTLMSSP_NEGOTIATE_OEM_WORKSTATION_SUPPLIED = 0x00002000
	NTLMSSP_NEGOTIATE_OEM_DOMAIN_SUPPLIED      = 0x00001000
	NTLMSSP_NEGOTIATE_NTLM                     = 0x00000200
	NTLMSSP_NEGOTIATE_LM_KEY                   = 0x00000080
	NTLMSSP_NEGOTIATE_DATAGRAM                 = 0x00000040
	NTLMSSP_NEGOTIATE_SEAL                     = 0x00000020
	NTLMSSP_NEGOTIATE_SIGN                     = 0x00000010
	NTLMSSP_REQUEST_TARGET                     = 0x00000004
	NTLM_NEGOTIATE_OEM                         = 0x00000002
	NTLMSSP_NEGOTIATE_UNICODE                  = 0x00000001
)

type Negotiation struct {
	Type   NegotiationType `struc:"byte"`
	Flag   uint8           `struc:"uint8"`
	Length uint16          `struc:"little"`
	Result uint32          `struc:"little"`
}

var CRLF = []byte{0x0a, 0x0d}

type NegoToken struct {
	Data []byte `asn1:"explicit,tag:0"`
}

type TSRequest struct {
	Version    int         `asn1:"explicit,tag:0"`
	NegoTokens []NegoToken `asn1:"optional,explicit,tag:1"`
	AuthInfo   []byte      `asn1:"optional,explicit,tag:2"`
	PubKeyAuth []byte      `asn1:"optional,explicit,tag:3"`
}
type NegotiateMessage struct {
	Signature               [8]byte  `struc:"little"`
	MessageType             uint32   `struc:"little"`
	NegotiateFlags          uint32   `struc:"little"`
	DomainNameLen           uint16   `struc:"little"`
	DomainNameMaxLen        uint16   `struc:"little"`
	DomainNameBufferOffset  uint32   `struc:"little"`
	WorkstationLen          uint16   `struc:"little"`
	WorkstationMaxLen       uint16   `struc:"little"`
	WorkstationBufferOffset uint32   `struc:"little"`
	Version                 NVersion `struc:"little"`
	Payload                 [32]byte `struc:"skip"`
}
type NVersion struct {
	ProductMajorVersion uint8   `struc:"little"`
	ProductMinorVersion uint8   `struc:"little"`
	ProductBuild        uint16  `struc:"little"`
	Reserved            [3]byte `struc:"little"`
	NTLMRevisionCurrent uint8   `struc:"little"`
}

const (
	WINDOWS_MINOR_VERSION_0 = 0x00
	WINDOWS_MINOR_VERSION_1 = 0x01
	WINDOWS_MINOR_VERSION_2 = 0x02
	WINDOWS_MINOR_VERSION_3 = 0x03

	WINDOWS_MAJOR_VERSION_5 = 0x05
	WINDOWS_MAJOR_VERSION_6 = 0x06
	NTLMSSP_REVISION_W2K3   = 0x0F
)

func NewNVersion() NVersion {
	return NVersion{
		ProductMajorVersion: WINDOWS_MAJOR_VERSION_6,
		ProductMinorVersion: WINDOWS_MINOR_VERSION_0,
		ProductBuild:        6002,
		NTLMRevisionCurrent: NTLMSSP_REVISION_W2K3,
	}
}

func NewNegotiateMessage() *NegotiateMessage {
	return &NegotiateMessage{
		Signature:   [8]byte{'N', 'T', 'L', 'M', 'S', 'S', 'P', 0x00},
		MessageType: 0x00000001,
	}
}

type ChallengeMessage struct {
	Signature              []byte   `struc:"[8]byte"`
	MessageType            uint32   `struc:"little"`
	TargetNameLen          uint16   `struc:"little"`
	TargetNameMaxLen       uint16   `struc:"little"`
	TargetNameBufferOffset uint32   `struc:"little"`
	NegotiateFlags         uint32   `struc:"little"`
	ServerChallenge        [8]byte  `struc:"little"`
	Reserved               [8]byte  `struc:"little"`
	TargetInfoLen          uint16   `struc:"little"`
	TargetInfoMaxLen       uint16   `struc:"little"`
	TargetInfoBufferOffset uint32   `struc:"little"`
	Version                NVersion `struc:"skip"`
	Payload                []byte   `struc:"skip"`
}

func (m *ChallengeMessage) Serialize() []byte {
	buff := &bytes.Buffer{}
	struc.Pack(buff, m)
	if (m.NegotiateFlags & NTLMSSP_NEGOTIATE_VERSION) != 0 {
		struc.Pack(buff, m.Version)
	}
	buff.Write(m.Payload)
	return buff.Bytes()
}

func (m *NegotiateMessage) Serialize() []byte {
	if (m.NegotiateFlags & NTLMSSP_NEGOTIATE_VERSION) != 0 {
		m.Version = NewNVersion()
	}
	buff := &bytes.Buffer{}
	struc.Pack(buff, m)

	return buff.Bytes()
}
