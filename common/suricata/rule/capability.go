package rule

import (
	"sort"
	"strings"
)

// KeywordCapabilityStatus describes whether the local matcher can preserve a
// Suricata keyword's matching semantics.
type KeywordCapabilityStatus string

const (
	KeywordSupported        KeywordCapabilityStatus = "supported"
	KeywordUnsupported      KeywordCapabilityStatus = "unsupported"
	KeywordStatefulRequired KeywordCapabilityStatus = "stateful-required"
)

// KeywordCapability is the machine-readable capability description for one
// spelling of a Suricata rule keyword.
type KeywordCapability struct {
	Keyword   string                  `json:"keyword"`
	Canonical string                  `json:"canonical"`
	Status    KeywordCapabilityStatus `json:"status"`
	Reason    string                  `json:"reason,omitempty"`
}

var keywordCapabilityRegistry = buildKeywordCapabilityRegistry()

func buildKeywordCapabilityRegistry() map[string]KeywordCapability {
	registry := make(map[string]KeywordCapability)
	add := func(status KeywordCapabilityStatus, canonical, reason string, keywords ...string) {
		for _, keyword := range keywords {
			keyword = normalizeKeyword(keyword)
			registry[keyword] = KeywordCapability{
				Keyword:   keyword,
				Canonical: canonical,
				Status:    status,
				Reason:    reason,
			}
		}
	}

	add(KeywordSupported, "metadata", "", "sid", "rev", "gid", "classtype", "reference", "msg", "priority", "metadata", "target")
	add(KeywordSupported, "content", "", "content")
	add(KeywordSupported, "pcre", "", "pcre")
	add(KeywordSupported, "content-modifier", "", "nocase", "depth", "offset", "startswith", "endswith", "distance", "within", "isdataat", "fast_pattern")
	add(KeywordSupported, "dotprefix", "", "dotprefix")
	add(KeywordSupported, "dns.opcode", "", "dns.opcode", "dns_opcode")
	add(KeywordSupported, "dns.rcode", "", "dns.rcode", "dns_rcode")
	add(KeywordSupported, "dns.rrtype", "", "dns.rrtype", "dns_rrtype")
	add(KeywordSupported, "tcp-sequence", "", "seq", "ack", "window", "tcp.mss")
	add(KeywordSupported, "tcp.flags", "", "flags", "tcp.flags")
	add(KeywordSupported, "ip-header", "", "ttl", "sameip", "ipopts", "ip_proto", "id", "fragbits", "fragoffset", "tos")
	add(KeywordSupported, "icmp", "", "icode", "itype", "icmp_id", "icmp_seq")
	add(KeywordSupported, "urilen", "", "urilen")

	add(KeywordSupported, "dns.query", "", "dns_query", "dns.query", "dns.query.name")
	add(KeywordSupported, "http.content_type", "", "http_content_type", "http.content_type")
	add(KeywordSupported, "http.content_len", "", "http_content_len", "http.content_len")
	add(KeywordSupported, "http.start", "", "http_start", "http.start")
	add(KeywordSupported, "http.protocol", "", "http_protocol", "http.protocol")
	add(KeywordSupported, "http.header_names", "", "http_header_names", "http.header_names")
	add(KeywordSupported, "http.request_line", "", "http_request_line", "http.request_line")
	add(KeywordSupported, "http.accept", "", "http_accept", "http.accept")
	add(KeywordSupported, "http.accept_enc", "", "http_accept_enc", "http.accept_enc")
	add(KeywordSupported, "http.referer", "", "http_referer", "http.referer")
	add(KeywordSupported, "http.connection", "", "http_connection", "http.connection")
	add(KeywordSupported, "http.response_line", "", "http_response_line", "http.response_line")
	add(KeywordSupported, "http.header", "", "http_header", "http.header")
	add(KeywordSupported, "http.header.raw", "", "http_raw_header", "http.raw_header", "http.header.raw")
	add(KeywordSupported, "http.cookie", "", "http_cookie", "http.cookie")
	add(KeywordSupported, "http.uri", "", "http_uri", "http.uri")
	add(KeywordSupported, "http.uri.raw", "", "http_raw_uri", "http.uri.raw")
	add(KeywordSupported, "http.method", "", "http_method", "http.method")
	add(KeywordSupported, "http.user_agent", "", "http_user_agent", "http.user_agent")
	add(KeywordSupported, "http.host", "", "http_host", "http.host")
	add(KeywordSupported, "http.host.raw", "", "http_raw_host", "http.raw_host", "http.host.raw")
	add(KeywordSupported, "http.stat_msg", "", "http_stat_msg", "http.stat_msg")
	add(KeywordSupported, "http.stat_code", "", "http_stat_code", "http.stat_code")
	add(KeywordSupported, "http.request_body", "", "http_client_body", "http.client_body", "http_request_body", "http.request_body")
	add(KeywordSupported, "http.response_body", "", "http_server_body", "http.server_body", "http.response_body")
	add(KeywordSupported, "http.server", "", "http_server", "http.server")
	add(KeywordSupported, "http.location", "", "http_location", "http.location")
	add(KeywordSupported, "ja3.hash", "", "ja3.hash", "ja3_hash")
	add(KeywordSupported, "ja3s.hash", "", "ja3s.hash", "ja3s_hash")
	add(KeywordSupported, "ja3.string", "", "ja3.string", "ja3_string")
	add(KeywordSupported, "ja3s.string", "", "ja3s.string", "ja3s_string")
	add(KeywordSupported, "tls.cert_subject", "", "tls.cert_subject", "tls_cert_subject")
	add(KeywordSupported, "tls.cert_issuer", "", "tls.cert_issuer", "tls_cert_issuer")
	add(KeywordSupported, "tls.cert_serial", "", "tls.cert_serial", "tls_cert_serial")
	add(KeywordSupported, "tls.cert_fingerprint", "", "tls.cert_fingerprint", "tls_cert_fingerprint")
	add(KeywordSupported, "tls.certs", "", "tls.certs", "tls_certs")
	add(KeywordSupported, "tls.version", "", "tls.version", "tls_version")
	add(KeywordSupported, "tls.sni", "", "tls.sni", "tls_sni")
	add(KeywordSupported, "tcp.hdr", "", "tcp.hdr", "tcp_hdr")
	add(KeywordSupported, "udp.hdr", "", "udp.hdr", "udp_hdr")
	add(KeywordSupported, "icmp.hdr", "", "icmpv4.hdr", "icmpv4_hdr", "icmpv6.hdr", "icmpv6_hdr")

	add(KeywordStatefulRequired, "flow", "flow direction and connection state require a stateful flow tracker", "flow")
	add(KeywordStatefulRequired, "threshold", "threshold counters require state shared across packets", "threshold", "detection_filter")
	add(KeywordStatefulRequired, "flowbits", "flowbits require state shared across rules and flows", "flowbits")
	add(KeywordStatefulRequired, "flowint", "flowint requires mutable per-flow state", "flowint")
	add(KeywordStatefulRequired, "xbits", "xbits require tracked state with expiration", "xbits")
	add(KeywordStatefulRequired, "stream-size", "stream_size requires stateful stream accounting", "stream_size")
	add(KeywordStatefulRequired, "flow-metric", "flow metrics require state accumulated across packets", "flow.age", "flow.pkts", "flow.bytes")

	add(KeywordUnsupported, "geoip", "GeoIP matching requires an external geolocation database", "geoip")
	add(KeywordSupported, "rawbytes", "", "rawbytes")
	add(KeywordSupported, "buffer-size", "", "bsize", "dsize")
	add(KeywordSupported, "byte_test", "", "byte_test")
	add(KeywordSupported, "byte_jump", "", "byte_jump")
	add(KeywordSupported, "byte_extract", "", "byte_extract")
	add(KeywordUnsupported, "byte-operation", "byte arithmetic is not implemented", "byte_math")
	add(KeywordUnsupported, "rpc", "RPC matching is not implemented", "rpc")
	add(KeywordUnsupported, "replace", "payload replacement is not implemented", "replace")
	add(KeywordStatefulRequired, "noalert", "alert suppression is a stateful rule side effect", "noalert")
	add(KeywordSupported, "base64", "", "base64_decode", "base64_data")
	add(KeywordSupported, "app-layer-event", "", "app-layer-event")
	add(KeywordSupported, "asn1", "", "asn1")
	add(KeywordSupported, "file.data", "", "file_data", "file.data")
	add(KeywordSupported, "ssh.proto", "", "ssh.proto", "ssh_proto")
	add(KeywordSupported, "ftpbounce", "", "ftpbounce")
	add(KeywordSupported, "ip-header-buffer", "", "ipv4.hdr", "ipv4_hdr", "ipv6.hdr", "ipv6_hdr")

	add(KeywordUnsupported, "stream", "stream reassembly controls are not implemented", "stream_reassemble")
	add(KeywordSupported, "app-layer-protocol", "", "app-layer-protocol")
	add(KeywordUnsupported, "dataset", "dataset-backed matching is not implemented", "dataset", "datarep")
	add(KeywordUnsupported, "external-state", "external reputation and scripting engines are not implemented", "lua", "iprep", "reputation")
	add(KeywordSupported, "tag", "", "tag")
	add(KeywordUnsupported, "rule-action", "this rule side effect is not implemented", "filestore")
	add(KeywordUnsupported, "file-property", "file property and hash matching are not implemented", "filemagic", "file.name", "fileext", "filesize", "filemd5", "filesha1", "filesha256")
	add(KeywordUnsupported, "tls-property", "this TLS property is not wired into rule parsing", "tls.cert_chain_len", "tls_cert_chain_len", "tls.cert_expired", "tls_cert_expired", "tls.cert_valid", "tls_cert_valid")
	add(KeywordUnsupported, "protocol-property", "this application protocol keyword is not implemented", "ssh.software", "ssh.protoversion")
	add(KeywordSupported, "http.accept_lang", "", "http_accept_lang", "http.accept_lang")

	return registry
}

func normalizeKeyword(keyword string) string {
	return strings.ToLower(strings.TrimSpace(keyword))
}

// LookupKeywordCapability returns the capability for keyword. Lookup is
// case-insensitive and ignores surrounding whitespace.
func LookupKeywordCapability(keyword string) (KeywordCapability, bool) {
	capability, ok := keywordCapabilityRegistry[normalizeKeyword(keyword)]
	return capability, ok
}

// KeywordCapabilities returns a copy of the complete capability registry.
func KeywordCapabilities() []KeywordCapability {
	capabilities := make([]KeywordCapability, 0, len(keywordCapabilityRegistry))
	for _, capability := range keywordCapabilityRegistry {
		capabilities = append(capabilities, capability)
	}
	sort.Slice(capabilities, func(i, j int) bool {
		return capabilities[i].Keyword < capabilities[j].Keyword
	})
	return capabilities
}
