package ja3

import (
	"fmt"
	"strconv"
)

// CurveID is the type of a TLS identifier for an elliptic curve. See
// https://www.iana.org/assignments/tls-parameters/tls-parameters.xml#tls-parameters-8.
//
// In TLS 1.3, this type is called NamedGroup, but at this time this library
// only supports Elliptic Curve based groups. See RFC 8446, Section 4.2.7.
type CurveID uint16

// TLS extension numbers
const (
	extensionServerName              uint16 = 0
	extensionStatusRequest           uint16 = 5
	extensionSupportedCurves         uint16 = 10 // supported_groups in TLS 1.3, see RFC 8446, Section 4.2.7
	extensionSupportedPoints         uint16 = 11
	extensionSignatureAlgorithms     uint16 = 13
	extensionALPN                    uint16 = 16
	extensionSCT                     uint16 = 18
	extensionMasterSecret            uint16 = 23
	extensionSessionTicket           uint16 = 35
	extensionPreSharedKey            uint16 = 41
	extensionEarlyData               uint16 = 42
	extensionSupportedVersions       uint16 = 43
	extensionCookie                  uint16 = 44
	extensionPSKModes                uint16 = 45
	extensionCertificateAuthorities  uint16 = 47
	extensionSignatureAlgorithmsCert uint16 = 50
	extensionKeyShare                uint16 = 51
	extensionRenegotiationInfo       uint16 = 0xff01
)

// TLS version numbers
const (
	VersionTLS10 = 0x0301
	VersionTLS11 = 0x0302
	VersionTLS12 = 0x0303
	VersionTLS13 = 0x0304

	VersionGMSSL = 0x0101 // GM/T 0024-2014

	// Deprecated: SSLv3 is cryptographically broken, and is no longer
	// supported by this package. See golang.org/issue/32716.
	VersionSSL30 = 0x0300
)

// CurveID
const (
	CurveP256 CurveID = 23
	CurveP384 CurveID = 24
	CurveP521 CurveID = 25
	X25519    CurveID = 29
)

// A list of cipher suite IDs that are, or have been, implemented by this
// package.
//
// See https://www.iana.org/assignments/tls-parameters/tls-parameters.xml
const (
	// TLS 1.0 - 1.2 cipher suites.
	TLS_RSA_WITH_RC4_128_SHA                      uint16 = 0x0005
	TLS_RSA_WITH_3DES_EDE_CBC_SHA                 uint16 = 0x000a
	TLS_RSA_WITH_AES_128_CBC_SHA                  uint16 = 0x002f
	TLS_RSA_WITH_AES_256_CBC_SHA                  uint16 = 0x0035
	TLS_RSA_WITH_AES_128_CBC_SHA256               uint16 = 0x003c
	TLS_RSA_WITH_AES_128_GCM_SHA256               uint16 = 0x009c
	TLS_RSA_WITH_AES_256_GCM_SHA384               uint16 = 0x009d
	TLS_ECDHE_ECDSA_WITH_RC4_128_SHA              uint16 = 0xc007
	TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA          uint16 = 0xc009
	TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA          uint16 = 0xc00a
	TLS_ECDHE_RSA_WITH_RC4_128_SHA                uint16 = 0xc011
	TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA           uint16 = 0xc012
	TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA            uint16 = 0xc013
	TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA            uint16 = 0xc014
	TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256       uint16 = 0xc023
	TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256         uint16 = 0xc027
	TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256         uint16 = 0xc02f
	TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256       uint16 = 0xc02b
	TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384         uint16 = 0xc030
	TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384       uint16 = 0xc02c
	TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256   uint16 = 0xcca8
	TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256 uint16 = 0xcca9

	// TLS 1.3 cipher suites.
	TLS_AES_128_GCM_SHA256       uint16 = 0x1301
	TLS_AES_256_GCM_SHA384       uint16 = 0x1302
	TLS_CHACHA20_POLY1305_SHA256 uint16 = 0x1303

	// TLS_FALLBACK_SCSV isn't a standard cipher suite but an indicator
	// that the client is doing version fallback. See RFC 7507.
	TLS_FALLBACK_SCSV uint16 = 0x5600

	// Legacy names for the corresponding cipher suites with the correct _SHA256
	// suffix, retained for backward compatibility.
	TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305   = TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
	TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305 = TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
	//GM crypto suites ID  Taken from GM/T 0024-2014
	GMTLS_ECDHE_SM2_WITH_SM1_SM3 uint16 = 0xe001
	GMTLS_SM2_WITH_SM1_SM3       uint16 = 0xe003
	GMTLS_IBSDH_WITH_SM1_SM3     uint16 = 0xe005
	GMTLS_IBC_WITH_SM1_SM3       uint16 = 0xe007
	GMTLS_RSA_WITH_SM1_SM3       uint16 = 0xe009
	GMTLS_RSA_WITH_SM1_SHA1      uint16 = 0xe00a
	GMTLS_ECDHE_SM2_WITH_SM4_SM3 uint16 = 0xe011
	GMTLS_ECDHE_SM4_CBC_SM3      uint16 = 0xe011
	GMTLS_ECDHE_SM4_GCM_SM3      uint16 = 0xe051
	GMTLS_SM2_WITH_SM4_SM3       uint16 = 0xe013
	GMTLS_ECC_SM4_CBC_SM3        uint16 = 0xe013
	GMTLS_ECC_SM4_GCM_SM3        uint16 = 0xe053
	GMTLS_IBSDH_WITH_SM4_SM3     uint16 = 0xe015
	GMTLS_IBC_WITH_SM4_SM3       uint16 = 0xe017
	GMTLS_RSA_WITH_SM4_SM3       uint16 = 0xe019
	GMTLS_RSA_WITH_SM4_SHA1      uint16 = 0xe01a
)

// TLS supported version range
var (
	supportedUpToTLS12 = []uint16{VersionTLS10, VersionTLS11, VersionTLS12}
	supportedOnlyTLS12 = []uint16{VersionTLS12}
	supportedOnlyTLS13 = []uint16{VersionTLS13}
	supportedOnlyGMSSL = []uint16{VersionGMSSL}
)

// TLS Elliptic Curve Point Formats
// https://www.iana.org/assignments/tls-parameters/tls-parameters.xml#tls-parameters-9
const (
	pointFormatUncompressed uint8 = 0
	ansiX962CompressedPrime uint8 = 1
	ansiX962CompressedChar2 uint8 = 2
)

func CipherSuites() []*CipherSuite {
	return []*CipherSuite{
		{TLS_RSA_WITH_AES_128_CBC_SHA, "TLS_RSA_WITH_AES_128_CBC_SHA", supportedUpToTLS12, false},
		{TLS_RSA_WITH_AES_256_CBC_SHA, "TLS_RSA_WITH_AES_256_CBC_SHA", supportedUpToTLS12, false},
		{TLS_RSA_WITH_AES_128_GCM_SHA256, "TLS_RSA_WITH_AES_128_GCM_SHA256", supportedOnlyTLS12, false},
		{TLS_RSA_WITH_AES_256_GCM_SHA384, "TLS_RSA_WITH_AES_256_GCM_SHA384", supportedOnlyTLS12, false},

		{TLS_AES_128_GCM_SHA256, "TLS_AES_128_GCM_SHA256", supportedOnlyTLS13, false},
		{TLS_AES_256_GCM_SHA384, "TLS_AES_256_GCM_SHA384", supportedOnlyTLS13, false},
		{TLS_CHACHA20_POLY1305_SHA256, "TLS_CHACHA20_POLY1305_SHA256", supportedOnlyTLS13, false},

		{TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA, "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA", supportedUpToTLS12, false},
		{TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA, "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA", supportedUpToTLS12, false},
		{TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA, "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA", supportedUpToTLS12, false},
		{TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA, "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA", supportedUpToTLS12, false},
		{TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256", supportedOnlyTLS12, false},
		{TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384", supportedOnlyTLS12, false},
		{TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", supportedOnlyTLS12, false},
		{TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384, "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384", supportedOnlyTLS12, false},
		{TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256, "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256", supportedOnlyTLS12, false},
		{TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305, "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305", supportedOnlyTLS12, false},
		{TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256", supportedOnlyTLS12, false},
		{TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305", supportedOnlyTLS12, false},

		{GMTLS_ECC_SM4_CBC_SM3, "GMTLS_ECC_SM4_CBC_SM3", supportedOnlyGMSSL, false},
		{GMTLS_ECC_SM4_GCM_SM3, "GMTLS_ECC_SM4_GCM_SM3", supportedOnlyGMSSL, false},
		{GMTLS_ECDHE_SM4_CBC_SM3, "GMTLS_ECDHE_SM4_CBC_SM3", supportedOnlyGMSSL, false},
		{GMTLS_ECDHE_SM4_GCM_SM3, "GMTLS_ECDHE_SM4_GCM_SM3", supportedOnlyGMSSL, false},
		{GMTLS_ECDHE_SM2_WITH_SM1_SM3, "GMTLS_ECDHE_SM2_WITH_SM1_SM3", supportedOnlyGMSSL, false},
		{GMTLS_SM2_WITH_SM1_SM3, "GMTLS_SM2_WITH_SM1_SM3", supportedOnlyGMSSL, false},
		{GMTLS_IBSDH_WITH_SM1_SM3, "GMTLS_IBSDH_WITH_SM1_SM3", supportedOnlyGMSSL, false},
		{GMTLS_IBC_WITH_SM1_SM3, "GMTLS_IBC_WITH_SM1_SM3", supportedOnlyGMSSL, false},
		{GMTLS_RSA_WITH_SM1_SM3, "GMTLS_RSA_WITH_SM1_SM3", supportedOnlyGMSSL, false},
		{GMTLS_RSA_WITH_SM1_SHA1, "GMTLS_RSA_WITH_SM1_SHA1", supportedOnlyGMSSL, false},
		{GMTLS_ECDHE_SM2_WITH_SM4_SM3, "GMTLS_ECDHE_SM2_WITH_SM4_SM3", supportedOnlyGMSSL, false},
		{GMTLS_SM2_WITH_SM4_SM3, "GMTLS_SM2_WITH_SM4_SM3", supportedOnlyGMSSL, false},
		{GMTLS_IBSDH_WITH_SM4_SM3, "GMTLS_IBSDH_WITH_SM4_SM3", supportedOnlyGMSSL, false},
		{GMTLS_IBC_WITH_SM4_SM3, "GMTLS_IBC_WITH_SM4_SM3", supportedOnlyGMSSL, false},
		{GMTLS_RSA_WITH_SM4_SM3, "GMTLS_RSA_WITH_SM4_SM3", supportedOnlyGMSSL, false},
		{GMTLS_RSA_WITH_SM4_SHA1, "GMTLS_RSA_WITH_SM4_SHA1", supportedOnlyGMSSL, false},

		{TLS_RSA_WITH_RC4_128_SHA, "TLS_RSA_WITH_RC4_128_SHA", supportedUpToTLS12, true},
		{TLS_RSA_WITH_3DES_EDE_CBC_SHA, "TLS_RSA_WITH_3DES_EDE_CBC_SHA", supportedUpToTLS12, true},
		{TLS_RSA_WITH_AES_128_CBC_SHA256, "TLS_RSA_WITH_AES_128_CBC_SHA256", supportedOnlyTLS12, true},
		{TLS_ECDHE_ECDSA_WITH_RC4_128_SHA, "TLS_ECDHE_ECDSA_WITH_RC4_128_SHA", supportedUpToTLS12, true},
		{TLS_ECDHE_RSA_WITH_RC4_128_SHA, "TLS_ECDHE_RSA_WITH_RC4_128_SHA", supportedUpToTLS12, true},
		{TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA, "TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA", supportedUpToTLS12, true},
		{TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256, "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256", supportedOnlyTLS12, true},
		{TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256, "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256", supportedOnlyTLS12, true},
	}
}

// CipherSuiteName returns the standard name for the passed cipher suite ID
// (e.g. "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"), or a fallback representation
// of the ID value if the cipher suite is not implemented by this package.
func CipherSuiteName(id uint16) string {
	for _, c := range CipherSuites() {
		if c.ID == id {
			return c.Name
		}
	}

	name := NotImplementedCipherSuites(id)
	if name != "" {
		return name
	}

	return fmt.Sprintf("0x%04X", id)
}

func GetCipherSuiteByID(id string) *CipherSuite {
	var cipherID uint16
	_, err := fmt.Sscan(id, &cipherID)
	if err != nil {
		return &CipherSuite{Name: "Unknown"}
	}
	for _, c := range CipherSuites() {
		if c.ID == cipherID {
			return c
		}
	}
	name := NotImplementedCipherSuites(cipherID)
	if name != "" {
		return &CipherSuite{ID: cipherID, Name: name}
	}
	return &CipherSuite{ID: cipherID, Name: "Unknown"}
}

func GetEllipticCurvesByID(id string) *EllipticCurve {
	var curveID CurveID
	_, err := fmt.Sscan(id, &curveID)
	if err != nil {
		return &EllipticCurve{
			CurveName: "Unknown",
		}
	}
	switch curveID {
	case CurveP256:
		return &EllipticCurve{
			CurveID:   uint16(CurveP256),
			CurveName: "CurveP256",
		}
	case CurveP384:
		return &EllipticCurve{
			CurveID:   uint16(CurveP384),
			CurveName: "CurveP384",
		}
	case CurveP521:
		return &EllipticCurve{
			CurveID:   uint16(CurveP521),
			CurveName: "CurveP521",
		}
	case X25519:
		return &EllipticCurve{
			CurveID:   uint16(X25519),
			CurveName: "X25519",
		}
	}
	return &EllipticCurve{
		CurveName: "Unknown",
	}
}

func GetExtensionByType(typeNum string) *ExtensionsType {
	extensionType, err := strconv.Atoi(typeNum)
	if err != nil {
		return &ExtensionsType{
			TypeName: "Unknown",
		}
	}
	switch uint16(extensionType) {
	case extensionServerName:
		return &ExtensionsType{
			Type:     extensionServerName,
			TypeName: "extensionServerName",
		}
	case extensionStatusRequest:
		return &ExtensionsType{
			Type:     extensionStatusRequest,
			TypeName: "extensionStatusRequest",
		}
	case extensionSupportedCurves:
		return &ExtensionsType{
			Type:     extensionSupportedCurves,
			TypeName: "extensionSupportedCurves",
		}
	case extensionSupportedPoints:
		return &ExtensionsType{
			Type:     extensionSupportedPoints,
			TypeName: "extensionSupportedPoints",
		}
	case extensionSignatureAlgorithms:
		return &ExtensionsType{
			Type:     extensionSignatureAlgorithms,
			TypeName: "extensionSignatureAlgorithms",
		}
	case extensionALPN:
		return &ExtensionsType{
			Type:     extensionALPN,
			TypeName: "extensionALPN",
		}
	case extensionSCT:
		return &ExtensionsType{
			Type:     extensionSCT,
			TypeName: "extensionSCT",
		}
	case extensionMasterSecret:
		return &ExtensionsType{
			Type:     extensionMasterSecret,
			TypeName: "extensionMasterSecret",
		}
	case extensionSessionTicket:
		return &ExtensionsType{
			Type:     extensionSessionTicket,
			TypeName: "extensionSessionTicket",
		}
	case extensionPreSharedKey:
		return &ExtensionsType{
			Type:     extensionPreSharedKey,
			TypeName: "extensionPreSharedKey",
		}
	case extensionEarlyData:
		return &ExtensionsType{
			Type:     extensionEarlyData,
			TypeName: "extensionEarlyData",
		}
	case extensionSupportedVersions:
		return &ExtensionsType{
			Type:     extensionSupportedVersions,
			TypeName: "extensionSupportedVersions",
		}
	case extensionCookie:
		return &ExtensionsType{
			Type:     extensionCookie,
			TypeName: "extensionCookie",
		}
	case extensionPSKModes:
		return &ExtensionsType{
			Type:     extensionPSKModes,
			TypeName: "extensionPSKModes",
		}
	case extensionCertificateAuthorities:
		return &ExtensionsType{
			Type:     extensionCertificateAuthorities,
			TypeName: "extensionCertificateAuthorities",
		}
	case extensionKeyShare:
		return &ExtensionsType{
			Type:     extensionKeyShare,
			TypeName: "extensionKeyShare",
		}
	case extensionSignatureAlgorithmsCert:
		return &ExtensionsType{
			Type:     extensionSignatureAlgorithmsCert,
			TypeName: "extensionSignatureAlgorithmsCert",
		}
	case extensionRenegotiationInfo:
		return &ExtensionsType{
			Type:     extensionRenegotiationInfo,
			TypeName: "extensionRenegotiationInfo",
		}
	}
	return &ExtensionsType{
		TypeName: "Unknown",
	}
}

func GetEllipticCurvePointFormatByID(id string) *EllipticCurvePointFormat {
	var curvePointFormat uint8
	_, err := fmt.Sscan(id, &curvePointFormat)
	if err != nil {
		return &EllipticCurvePointFormat{
			CurvePointFormatName: "Unknown",
		}
	}
	switch curvePointFormat {
	case pointFormatUncompressed:
		return &EllipticCurvePointFormat{
			CurvePoint:           pointFormatUncompressed,
			CurvePointFormatName: "pointFormatUncompressed",
		}
	case ansiX962CompressedPrime:
		return &EllipticCurvePointFormat{
			CurvePoint:           ansiX962CompressedPrime,
			CurvePointFormatName: "ansiX962CompressedPrime",
		}
	case ansiX962CompressedChar2:
		return &EllipticCurvePointFormat{
			CurvePoint:           ansiX962CompressedChar2,
			CurvePointFormatName: "ansiX962CompressedChar2",
		}
	}
	if curvePointFormat >= 3 && curvePointFormat <= 247 {
		return &EllipticCurvePointFormat{
			CurvePoint:           curvePointFormat,
			CurvePointFormatName: "Unassigned",
		}
	}
	if curvePointFormat >= 248 {
		return &EllipticCurvePointFormat{
			CurvePoint:           curvePointFormat,
			CurvePointFormatName: "Reserved Private",
		}
	}
	return &EllipticCurvePointFormat{
		CurvePointFormatName: "Unknown",
	}
}
