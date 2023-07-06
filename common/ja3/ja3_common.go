package ja3

type TLSVersion struct {
	Version     uint16
	VersionName string
}

// CipherSuite is a TLS cipher suite. Note that most functions in this package
// accept and expose cipher suite IDs instead of this type.
type CipherSuite struct {
	ID   uint16
	Name string

	// Supported versions is the list of TLS protocol versions that can
	// negotiate this cipher suite.
	SupportedVersions []uint16

	// Insecure is true if the cipher suite has known security issues
	// due to its primitives, design, or implementation.
	Insecure bool
}

type ExtensionsType struct {
	Type     uint16
	TypeName string
}

type EllipticCurve struct {
	CurveID   uint16
	CurveName string
}

type EllipticCurvePointFormat struct {
	CurvePoint           uint8
	CurvePointFormatName string
}

type JA3 struct {
	TLSVersion                *TLSVersion
	CipherSuites              []*CipherSuite
	ExtensionsTypes           []*ExtensionsType
	EllipticCurves            []*EllipticCurve
	EllipticCurvePointFormats []*EllipticCurvePointFormat
}

type JA3S struct {
	TLSVersion      *TLSVersion
	AcceptedCipher  *CipherSuite
	ExtensionsTypes []*ExtensionsType
}
