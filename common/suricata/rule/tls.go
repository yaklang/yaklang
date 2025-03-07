package rule

import "github.com/yaklang/yaklang/common/suricata/data/numrange"

// TLSRule represents TLS layer rule configuration
type TLSRule struct {
	// TLS version
	Version string

	// Certificate chain length
	CertChainLen *numrange.NumRange

	// Certificate expired
	CertExpired *bool

	// Certificate valid
	CertValid *bool

	// Certificate fingerprint
	CertFingerprint string

	// Certificate subject
	CertSubject string

	// Certificate issuer
	CertIssuer string

	// Certificate serial
	CertSerial string

	// Server Name Indication
	SNI string

	// Store certificate
	Store bool

	// JA3 hash (md5)
	JA3Hash string

	// JA3 string
	JA3String string

	// JA3S hash (md5)
	JA3SHash string

	// JA3S string
	JA3SString string
}
