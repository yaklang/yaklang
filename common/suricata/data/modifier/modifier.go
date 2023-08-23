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
