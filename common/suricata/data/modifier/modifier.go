package modifier

type Modifier uint32

const (
	Default Modifier = iota

	// http req
	HTTPUri
	HTTPUriRaw
	HTTPMethod
	HTTPRequestLine
	HTTPRequestBody
	HTTPUserAgent
	HTTPHost
	HTTPHostRaw
	HTTPAccept
	HTTPAcceptLang
	HTTPAcceptEnc
	HTTPReferer

	// http resp
	HTTPStatMsg
	HTTPStatCode
	HTTPResponseLine
	HTTPResponseBody
	HTTPServer
	HTTPLocation

	// http common
	HTTPHeader
	HTTPHeaderRaw
	HTTPCookie
	HTTPConnection
	FileData
	HTTPContentType
	HTTPContentLen
	HTTPStart
	HTTPProtocol
	HTTPHeaderNames

	// DNS
	DNSQuery

	// IP
	IPv4HDR
	IPv6HDR

	// TCP
	TCPHDR

	// UDP
	UDPHDR

	// ICMP
	ICMPV4HDR
	ICMPV6HDR

	// TLS
	TLSCertSubject
	TLSCertIssuer
	TLSCertSerial
	TLSCertFingerprint
	TLSSNI
	TLSRandom
	TLSRandomTime
	TLSRandomBytes

	// JA3
	JA3Hash
	JA3String
	JA3SHash
	JA3SString
)

var HTTP_REQ_ONLY = []Modifier{
	HTTPUri,
	HTTPUriRaw,
	HTTPMethod,
	HTTPRequestLine,
	HTTPRequestBody,
	HTTPUserAgent,
	HTTPHost,
	HTTPHostRaw,
	HTTPAccept,
	HTTPAcceptLang,
	HTTPAcceptEnc,
	HTTPReferer,
}

var HTTP_RESP_ONLY = []Modifier{
	HTTPStatMsg,
	HTTPStatCode,
	HTTPResponseLine,
	HTTPResponseBody,
	HTTPServer,
	HTTPLocation,
}

func IsHTTPModifier(mdf Modifier) bool {
	return mdf >= HTTPUri && mdf <= HTTPHeaderNames
}
