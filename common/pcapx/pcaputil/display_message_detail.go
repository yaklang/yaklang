package pcaputil

import (
	"encoding/base64"
	"maps"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// MessageDisplayInput is the saved parser result for one PDU, not a flow to
// replay. Prefix starts at byte zero; Length is the size of the stored BLOB.
type MessageDisplayInput struct {
	Protocol, Profile, Status, Transport string
	Fields, Session                      map[string]any
	Source                               ByteSource
	Prefix                               []byte
	Length                               int
}

// WirePrefixLength is how many leading bytes could contribute wire-dependent
// nodes. Stored field ranges alone need no BLOB read. Callers may supply less.
func (m MessageDisplayInput) WirePrefixLength() int {
	switch m.Protocol {
	case "http", "doh", "ipp", "quic", "doq", "http3":
		return m.Length
	case "arp", "icmp", "icmpv6", "igmp":
		off, n := m.Source.ProtocolOffset, m.Source.ProtocolLength
		if off >= 0 && n > 0 && off <= m.Length && n <= m.Length-off {
			return off + n
		}
	case "ospf":
		return min(1, m.Length)
	case "tls":
		return min(5, m.Length)
	case "esp":
		return min(8, m.Length)
	}
	return 0
}

// BuildMessageDisplayNodes builds a fresh view for a detail request. A saved
// span can point beyond the preview; a check that reads bytes may only use
// Prefix. Never hand a reconstructed observation back to the live parser.
func BuildMessageDisplayNodes(m MessageDisplayInput) ([]DisplayNode, bool) {
	if m.Length <= 0 || len(m.Prefix) > m.Length || m.Protocol == "" {
		return nil, false
	}
	if isHTTPDisplayProtocol(m.Protocol) {
		if len(m.Prefix) == 0 {
			return nil, true
		}
		return BuildHTTPDisplayNodes(m.Protocol, m.Prefix, m.Length)
	}
	e := ProtocolEvent{Protocol: m.Protocol, Profile: m.Profile, Status: m.Status, Transport: m.Transport,
		Raw: m.Prefix, Length: m.Length, storedWireLength: m.Length, Session: m.Session, Fields: m.Fields,
		SourceBytes: m.Source, semanticSession: m.Session, semanticFields: m.Fields}
	switch e.Protocol {
	case "arp", "icmp", "icmpv6", "igmp":
		e.restoreNetworkDetail()
	case "ospf":
		if len(e.Raw) != 0 && (e.Raw[0] == 2 || e.Raw[0] == 3) {
			e.retainNativeOSPFFields(e.Raw, e.Raw[0])
		}
	case "pim":
		e.retainNativePIMFields(e.Raw)
	case "rsvp":
		e.retainNativeRSVPFields(e.Raw)
	case "gtpv2":
		e.retainNativeGTPv2Fields(e.Raw)
	case "tls":
		// The registry verifies the stored record words against the literal header.
		e.nativeTLSObservation = true
	case "esp":
		e.nativeESPObservation = true
	case "quic", "http3", "doq":
		e.nativeQUICObservation = true
		if m.Protocol == "http3" {
			e.nativeQUICApplication = 1
		} else if m.Protocol == "doq" {
			e.nativeQUICApplication = 2
		}
		e.semanticSession = detailByteFields(m.Session, "DCID", "SCID", "Initial Token", "Retry Token", "Retry Integrity Tag")
	case "stp":
		e.Session = detailByteFields(m.Session, "MST Configuration Digest")
	}
	e.freezeNativeNetworkFields()
	tree := BuildDisplayFieldTree(&e)
	return tree.Nodes, tree.Truncated || len(m.Prefix) < m.WirePrefixLength()
}

// JSON preserves octet strings as base64. Restore just the known wire keys
// needed by validators; arbitrary saved string fields must stay strings.
func detailByteFields(fields map[string]any, keys ...string) map[string]any {
	fields = maps.Clone(fields)
	for _, k := range keys {
		v, present := fields[k]
		if present && v == nil {
			// JSON null round-trips a typed nil []byte. A zero-length CID is
			// real (its length octet still has a span); a missing key is not.
			fields[k] = []byte(nil)
		} else if text, ok := v.(string); ok {
			if data, err := base64.StdEncoding.DecodeString(text); err == nil {
				fields[k] = data
			}
		}
	}
	return fields
}

// ARP, ICMP and IGMP can retain link/tunnel headers as part of message.Raw.
// The ingress extent identifies the actual decoder input, even for IP
// fragments or a decapsulated message whose contributor has a different DLT.
func (e *ProtocolEvent) restoreNetworkDetail() {
	off, length := e.SourceBytes.ProtocolOffset, e.SourceBytes.ProtocolLength
	if off < 0 || off >= len(e.Raw) || length <= 0 || length > e.Length-off {
		return
	}
	wire := e.Raw[off:min(off+length, len(e.Raw))]
	declared := length
	if e.Status != "decoded" && e.Status != "deferred" {
		// A capture cut is not a version discriminator for IGMP/MLD queries.
		declared = -1
	}
	switch e.Protocol {
	case "arp":
		e.retainNetworkHeaderFields(wire, len(wire), off)
	case "icmp":
		e.retainNetworkHeaderFields(wire, min(8, len(wire)), off)
	case "icmpv6":
		e.retainNetworkHeaderFields(wire, min(4, len(wire)), off)
		if e.Profile == "icmpv6-mld" && e.nativeNetworkFields != nil {
			r, _ := stream_parser.DecodeMLDWire(wire, length, maxDisplayTreeNodes, stream_parser.MLDWireContext{DeclaredBytes: declared})
			e.retainMLDOccurrences(&r, off, len(wire))
		}
	case "igmp":
		r, _ := stream_parser.DecodeIGMPWire(wire, length, maxDisplayTreeNodes, stream_parser.IGMPWireContext{DeclaredBytes: declared})
		e.retainNativeIGMPFields(r, off, len(wire))
	}
}
