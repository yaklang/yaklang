package msrdp

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
const (
	WINDOWS_MINOR_VERSION_0 = 0x00
	WINDOWS_MINOR_VERSION_1 = 0x01
	WINDOWS_MINOR_VERSION_2 = 0x02
	WINDOWS_MINOR_VERSION_3 = 0x03

	WINDOWS_MAJOR_VERSION_5 = 0x05
	WINDOWS_MAJOR_VERSION_6 = 0x06
	NTLMSSP_REVISION_W2K3   = 0x0F
)

type NegoToken struct {
	Data []byte `asn1:"explicit,tag:0"`
}

type TSRequest struct {
	Version    int         `asn1:"explicit,tag:0"`
	NegoTokens []NegoToken `asn1:"optional,explicit,tag:1"`
	AuthInfo   []byte      `asn1:"optional,explicit,tag:2"`
	PubKeyAuth []byte      `asn1:"optional,explicit,tag:3"`
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

type TSRemoteGuardCreds struct {
	LogonCred         TSRemoteGuardPackageCred   `asn1:"explicit,tag:0"`
	SupplementalCreds []TSRemoteGuardPackageCred `asn1:"explicit,tag:1"`
}

type TSRemoteGuardPackageCred struct {
	PackageName string `asn1:"explicit,tag:0"`
	CredBuffer  string `asn1:"explicit,tag:1"`
}
