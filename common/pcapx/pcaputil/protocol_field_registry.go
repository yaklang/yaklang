package pcaputil

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// DisplayFieldSchemaVersion versions the stable filter/export field contract.
// ProtocolCatalog describes parsing coverage; this registry describes only the
// explicitly projected fields, not all fields of every dissector.
const DisplayFieldSchemaVersion = 1

type DisplayFieldType string

const (
	DisplayString   DisplayFieldType = "string"
	DisplayUnsigned DisplayFieldType = "unsigned"
	DisplayBoolean  DisplayFieldType = "boolean"
	DisplayAddress  DisplayFieldType = "address"
)

// DisplayFieldDescriptor is an immutable registration returned as a value copy.
// Name is the stable filter identifier, independent of translated UI labels.
// Origin distinguishes event metadata, logical PDU fields, observed connection
// context and computed facts. A PDU range is never a captured-frame offset.
type DisplayFieldDescriptor struct {
	Name        string             `json:"name"`
	Label       string             `json:"label"`
	Protocol    string             `json:"protocol"`
	Type        DisplayFieldType   `json:"type"`
	Bits        int                `json:"bits,omitempty"`
	Origin      string             `json:"origin"`
	Description string             `json:"description"`
	Values      []DisplayFieldEnum `json:"values,omitempty"`
	AllowEmpty  bool               `json:"allow_empty,omitempty"`
}

type DisplayFieldEnum struct {
	Value uint64 `json:"value"`
	Label string `json:"label"`
}

type DisplayFieldRange struct {
	Offset int `json:"offset"`
	Length int `json:"length"`
}

type DisplayFieldValue struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
	// ExactValue preserves unsigned values wider than JavaScript's safe
	// integer range even when a JSON client decodes value through float64.
	ExactValue string             `json:"exact_value,omitempty"`
	Origin     string             `json:"origin"`
	Range      *DisplayFieldRange `json:"pdu_range,omitempty"`
}

// DisplayFieldSnapshot owns its slices and values. Limits are explicit rather
// than silently presenting a sampled projection as exhaustive. Source refers
// to the whole logical PDU; byte spans are relative to ProtocolEvent.Raw only.
type DisplayFieldSnapshot struct {
	SchemaVersion int                 `json:"schema_version"`
	PDU           uint64              `json:"pdu"`
	Source        ByteSource          `json:"source"`
	Values        []DisplayFieldValue `json:"values"`
	Truncated     bool                `json:"truncated,omitempty"`
	Incomplete    bool                `json:"incomplete,omitempty"`
	Completeness  string              `json:"completeness,omitempty"`
}

const maxDisplayFieldValues = 4096

type displayFieldBinding struct {
	descriptor DisplayFieldDescriptor
	protocols  string
	path       []string
	rangeKey   string
	custom     func(*ProtocolEvent, func(any), *displayProjection)
}

func displayRoot(protocol, name, label string, typ DisplayFieldType, bits int, origin, key string) displayFieldBinding {
	return displayFieldBinding{
		descriptor: DisplayFieldDescriptor{Name: name, Label: label, Protocol: protocol, Type: typ, Bits: bits, Origin: origin, Description: "Observed " + label + "; absence is not a zero value"},
		protocols:  "|" + protocol + "|",
		path:       []string{key},
		rangeKey:   key,
	}
}

func displayCustom(protocol, name, label string, typ DisplayFieldType, bits int, origin string, fn func(*ProtocolEvent, func(any), *displayProjection)) displayFieldBinding {
	b := displayRoot(protocol, name, label, typ, bits, origin, "")
	b.path, b.custom = nil, fn
	return b
}

func buildDisplayFieldBindings() []displayFieldBinding {
	var out []displayFieldBinding
	meta := func(name, label string, typ DisplayFieldType, bits int, fn func(*ProtocolEvent) any) {
		b := displayCustom("", name, label, typ, bits, "event", func(e *ProtocolEvent, emit func(any), p *displayProjection) { emit(fn(e)) })
		b.protocols = ""
		out = append(out, b)
	}
	meta("protocol", "Protocol", DisplayString, 0, func(e *ProtocolEvent) any { return e.Protocol })
	meta("status", "Decode status", DisplayString, 0, func(e *ProtocolEvent) any { return e.Status })
	meta("pdu.id", "PDU identifier", DisplayUnsigned, 64, func(e *ProtocolEvent) any { return e.ID })
	meta("pdu.length", "PDU length", DisplayUnsigned, 64, func(e *ProtocolEvent) any { return e.Length })
	meta("pdu.completed_in", "Complete message byte contributor", DisplayUnsigned, 64, func(e *ProtocolEvent) any {
		if e.SourceBytes.CompletedIn != nil {
			return e.SourceBytes.CompletedIn.Number
		}
		return nil
	})
	meta("source", "Source endpoint", DisplayString, 0, func(e *ProtocolEvent) any { return e.Source })
	meta("destination", "Destination endpoint", DisplayString, 0, func(e *ProtocolEvent) any { return e.Destination })
	meta("flow.id", "Observed flow identifier", DisplayUnsigned, 64, func(e *ProtocolEvent) any { return e.FlowID })
	meta("pdu.response_to", "Response to PDU", DisplayUnsigned, 64, func(e *ProtocolEvent) any {
		if e.ResponseTo != 0 {
			return e.ResponseTo
		}
		return nil
	})
	b := displayCustom("ip", "ip.addr", "Endpoint address", DisplayAddress, 0, "event", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
		for _, endpoint := range []string{e.Source, e.Destination} {
			host, _, err := net.SplitHostPort(endpoint)
			if err != nil {
				host = endpoint
			}
			if addr, err := netip.ParseAddr(host); err == nil {
				emit(addr.Unmap().String())
			}
		}
	})
	b.protocols = ""
	out = append(out, b)
	path := func(protocols, owner, name, label string, typ DisplayFieldType, bits int, origin string, keys ...string) {
		b := displayRoot(owner, name, label, typ, bits, origin, "")
		b.protocols, b.path = "|"+protocols+"|", keys
		out = append(out, b)
	}
	for _, f := range []struct {
		name, label, key string
		typ              DisplayFieldType
		bits             int
	}{
		{"dns.id", "DNS transaction ID", "ID", DisplayUnsigned, 16},
		{"dns.qry.name", "DNS question name", "Name", DisplayString, 0},
		{"dns.qry.type", "DNS question type", "Type", DisplayUnsigned, 16},
	} {
		keys := []string{"DNS", "Questions", "*", f.key}
		if f.key == "ID" {
			keys = []string{"DNS", "ID"}
		}
		path("dns|mdns|llmnr|dot|doh|doq", "dns", f.name, f.label, f.typ, f.bits, "pdu", keys...)
		if f.key == "Name" {
			out[len(out)-1].custom = func(e *ProtocolEvent, emit func(any), p *displayProjection) {
				p.walk(e.Session, []string{"DNS", "Questions", "*"}, func(v any) {
					if q, ok := v.(map[string]any); ok {
						if name, exists := q["Display Name"]; exists {
							emit(name)
						} else {
							emit(q["Name"])
						}
					}
				})
			}
		}
	}
	path("dns|mdns|llmnr|dot|doh|doq", "dns", "dns.qry.name.escaped", "Lossless escaped DNS question name", DisplayString, 0, "pdu", "DNS", "Questions", "*", "Name")
	out = append(out, routingDisplayBindings()...)
	out = append(out, pimDisplayBindings()...)
	out = append(out, l2tp3DisplayBindings()...)
	out = append(out, stpDisplayBindings()...)
	out = append(out, ripngDisplayBindings()...)
	out = append(out, isisDisplayBindings()...)
	out = append(out, netflowV9DisplayBindings()...)
	out = append(out, ldpSessionDisplayBindings()...)
	for _, f := range []struct {
		name, key string
		typ       DisplayFieldType
		bits      int
	}{
		{"ldp.hdr.version", "Version", DisplayUnsigned, 16}, {"ldp.hdr.pdu_len", "PDU Length", DisplayUnsigned, 16},
		{"ldp.hdr.ldpid.lsr", "LSR ID", DisplayAddress, 0}, {"ldp.hdr.ldpid.lsid", "Label Space ID", DisplayUnsigned, 16},
	} {
		path("ldp", "ldp", f.name, f.key, f.typ, f.bits, "pdu", f.key)
	}
	for _, f := range []struct {
		name, key string
		bits      int
	}{
		{"ldp.msg.type", "Message Type", 16}, {"ldp.msg.len", "Message Length", 16}, {"ldp.msg.id", "Message ID", 32},
	} {
		path("ldp", "ldp", f.name, f.key, DisplayUnsigned, f.bits, "pdu", "Messages", "*", f.key)
	}
	for _, f := range []struct {
		name, key string
		typ       DisplayFieldType
		bits      int
	}{
		{"ldp.msg.tlv.type", "TLV Type", DisplayUnsigned, 16}, {"ldp.msg.tlv.len", "TLV Length", DisplayUnsigned, 16},
		{"ldp.msg.tlv.hello.hold", "Hold Time", DisplayUnsigned, 16}, {"ldp.msg.tlv.hello.targeted", "Targeted", DisplayBoolean, 0},
		{"ldp.msg.tlv.hello.requested", "Requested", DisplayBoolean, 0}, {"ldp.msg.tlv.hello.gtsm", "GTSM", DisplayBoolean, 0},
		{"ldp.msg.tlv.hello.res", "Reserved", DisplayUnsigned, 13}, {"ldp.msg.tlv.hello.cnf_seqno", "Configuration Sequence Number", DisplayUnsigned, 32},
		{"ldp.msg.tlv.ipv4.taddr", "IPv4 Transport Address", DisplayAddress, 0}, {"ldp.msg.tlv.ipv6.taddr", "IPv6 Transport Address", DisplayAddress, 0},
	} {
		path("ldp", "ldp", f.name, f.key, f.typ, f.bits, "pdu", "Messages", "*", "TLVs", "*", f.key)
	}

	for _, f := range []struct {
		protocol, name, label, key, origin string
		typ                                DisplayFieldType
		bits                               int
	}{
		{"tls", "tls.handshake.extensions_server_name", "TLS server name", "SNI", "context", DisplayString, 0},
		{"rdpudp", "rdpudp.flags", "RDPUDP selected header flags", "Flags", "pdu", DisplayUnsigned, 16},
		{"bgp", "bgp.length", "BGP message length", "Length", "pdu", DisplayUnsigned, 16},
		{"bgp", "bgp.type", "BGP message type", "Type", "pdu", DisplayUnsigned, 8},
		{"bgp", "bgp.open.version", "BGP version", "Version", "pdu", DisplayUnsigned, 8},
		{"bgp", "bgp.open.myas", "BGP two-octet AS", "My AS", "pdu", DisplayUnsigned, 16},
		{"bgp", "bgp.open.holdtime", "BGP hold time", "Hold Time", "pdu", DisplayUnsigned, 16},
		{"bgp", "bgp.open.identifier", "BGP identifier", "BGP Identifier", "pdu", DisplayAddress, 0},
		{"ajp", "ajp13.magic", "AJP directional magic", "Magic", "pdu", DisplayUnsigned, 16},
		{"ajp", "ajp13.len", "AJP envelope payload length", "Length", "pdu", DisplayUnsigned, 16},
		{"ajp", "ajp13.code", "AJP typed message code", "Code", "pdu", DisplayUnsigned, 8},
		{"ajp", "ajp13.uri", "AJP request URI", "URI", "pdu", DisplayString, 0},
		{"ajp", "ajp13.ver", "AJP forwarded HTTP version", "Protocol", "pdu", DisplayString, 0},
		{"ajp", "ajp13.rstatus", "AJP response status", "Status Code", "pdu", DisplayUnsigned, 16},
		{"ajp", "ajp13.reusep", "AJP response permits connection reuse", "Reuse", "pdu", DisplayBoolean, 0},
		{"rtmp", "rtmp.chunk.csid", "RTMP chunk stream", "Chunk Stream ID", "pdu", DisplayUnsigned, 32},
		{"rtmp", "rtmp.chunk.format", "RTMP header format", "Format", "pdu", DisplayUnsigned, 8},
		{"rtmp", "rtmp.message.type", "RTMP message type", "Message Type", "pdu", DisplayUnsigned, 8},
		{"rtmp", "rtmp.message.length", "RTMP logical message length", "Message Length", "pdu", DisplayUnsigned, 24},
		{"rtmp", "rtmp.message.stream", "RTMP message stream", "Message Stream ID", "pdu", DisplayUnsigned, 32},
		{"rtmp", "rtmp.timestamp", "RTMP timestamp milliseconds", "Timestamp", "pdu", DisplayUnsigned, 32},
		{"rtmp", "rtmp.command", "RTMP AMF command", "Command", "pdu", DisplayString, 0},
		{"amf0", "amf0.command", "AMF0 RTMP command", "Command", "pdu", DisplayString, 0},
		{"rdpudp", "rdpudp.synex.version", "RDPUDP offered version", "Offered Version", "pdu", DisplayUnsigned, 16},
		{"rdpudp", "rdpudp.data.seqnum", "RDPUDP2 transport data sequence", "Data Sequence", "pdu", DisplayUnsigned, 16},
		{"rdpudp", "rdpudp.data.channelseqnumber", "RDPUDP2 reliable channel sequence", "Channel Sequence", "pdu", DisplayUnsigned, 16},
		{"rdpudp", "rdpudp.data.sourceStart", "RDPUDP reliable source sequence", "Source Sequence", "pdu", DisplayUnsigned, 32},
		{"tls", "tls.version", "Negotiated TLS version", "Negotiated Version", "context", DisplayUnsigned, 16},
		{"tls", "tls.record.authenticated", "TLS record authentication verified", "Authentication Verified", "derived", DisplayBoolean, 0},
		{"tls", "tls.record.wire_type", "TLS on-wire record type", "Record Type", "pdu", DisplayUnsigned, 8},
		{"tls", "tls.record.version", "TLS legacy record version", "Record Version", "pdu", DisplayUnsigned, 16},
		{"tls", "tls.record.length", "TLS record payload length", "Record Length", "pdu", DisplayUnsigned, 16},
		{"tls", "tls.handshake.ciphersuite", "Negotiated TLS cipher suite", "Cipher Suite", "context", DisplayUnsigned, 16},
		{"quic", "quic.packet.authenticated", "QUIC packet authentication verified", "Authentication Verified", "derived", DisplayBoolean, 0},
		{"ssh", "ssh.protocol", "SSH identification", "Identification", "context", DisplayString, 0},
		{"ssh", "ssh.message_code", "SSH message code", "Message Number", "pdu", DisplayUnsigned, 8},
		{"ssh", "ssh.kex_algorithms", "SSH advertised key exchanges", "Kex Algorithms", "pdu", DisplayString, 0},
		{"ssh", "ssh.server_host_key_algorithms", "SSH advertised host keys", "Host Key Algorithms", "pdu", DisplayString, 0},
		{"ssh", "ssh.kex.selected", "Observed SSH key exchange", "Kex Algorithm", "context", DisplayString, 0},
		{"ssh", "ssh.host_key.selected", "Observed SSH host-key algorithm", "Host Key Algorithm", "context", DisplayString, 0},
		{"ssh", "ssh.host_key.sha256", "SSH public-key fingerprint", "Host Key Fingerprint", "derived", DisplayString, 0},
		{"ssh", "ssh.signature.verified", "SSH signature verified", "Signature Verified", "derived", DisplayBoolean, 0},
		{"ssh", "ssh.peer_identity.verified", "SSH peer identity verified", "Peer Identity Verified", "derived", DisplayBoolean, 0},
		{"wireguard", "wg.type", "WireGuard message type", "Message Type", "pdu", DisplayUnsigned, 32},
		{"wireguard", "wg.sender", "WireGuard sender index", "Sender Index", "pdu", DisplayUnsigned, 32},
		{"wireguard", "wg.receiver", "WireGuard receiver index", "Receiver Index", "pdu", DisplayUnsigned, 32},
		{"wireguard", "wg.counter", "WireGuard counter", "Counter", "pdu", DisplayUnsigned, 64},
		{"wireguard", "wg.authentication.verified", "WireGuard authentication verified", "Authentication Verified", "derived", DisplayBoolean, 0},
		{"ike", "isakmp.version", "IKE version", "Version", "pdu", DisplayUnsigned, 8},
		{"ike", "isakmp.exchangetype", "IKE exchange type", "Exchange Type", "pdu", DisplayUnsigned, 8},
		{"ike", "isakmp.flags", "IKE flags", "Flags", "pdu", DisplayUnsigned, 8},
		{"ike", "isakmp.messageid", "IKE message ID", "Message ID", "pdu", DisplayUnsigned, 32},
		{"ike", "isakmp.length", "IKE message length", "Length", "pdu", DisplayUnsigned, 32},
		{"rip", "rip.command", "RIP command", "Command", "pdu", DisplayUnsigned, 8},
		{"rip", "rip.version", "RIP version", "Version", "pdu", DisplayUnsigned, 8},
		{"nbt-dg", "nbdgm.type", "NetBIOS datagram type", "Message Type", "pdu", DisplayUnsigned, 8},
		{"nbt-dg", "nbdgm.flags", "NetBIOS datagram flags", "Flags", "pdu", DisplayUnsigned, 8},
		{"nbt-dg", "nbdgm.dgram_id", "NetBIOS datagram ID", "Datagram ID", "pdu", DisplayUnsigned, 16},
		{"nbt-dg", "nbdgm.dgram_len", "NetBIOS datagram length", "Datagram Length", "pdu", DisplayUnsigned, 16},
		{"nbt-dg", "nbdgm.source_name", "NetBIOS source name", "Source Name", "pdu", DisplayString, 0},
		{"nbt-dg", "nbdgm.destination_name", "NetBIOS destination name", "Destination Name", "pdu", DisplayString, 0},
		{"onc-rpc", "rpc.xid", "RPC transaction ID", "XID", "pdu", DisplayUnsigned, 32},
		{"onc-rpc", "rpc.msgtyp", "RPC message type", "Message Type", "pdu", DisplayUnsigned, 32},
		{"onc-rpc", "rpc.program", "RPC program", "Program", "pdu", DisplayUnsigned, 32},
		{"onc-rpc", "rpc.programversion", "RPC program version", "Program Version", "pdu", DisplayUnsigned, 32},
		{"onc-rpc", "rpc.procedure", "RPC procedure", "Procedure", "pdu", DisplayUnsigned, 32},
		{"l2tp", "l2tp.flags", "L2TP flags", "Flags", "pdu", DisplayUnsigned, 16},
		{"l2tp", "l2tp.length", "L2TP length", "Length", "pdu", DisplayUnsigned, 16},
		{"l2tp", "l2tp.tunnel", "L2TP tunnel ID", "Tunnel ID", "pdu", DisplayUnsigned, 16},
		{"l2tp", "l2tp.Ns", "L2TP sequence number", "Ns", "pdu", DisplayUnsigned, 16},
		{"l2tp", "l2tp.Nr", "L2TP receive sequence", "Nr", "pdu", DisplayUnsigned, 16},
		{"l2tp", "l2tp.avp.message_type", "L2TP message type", "Message Type", "pdu", DisplayUnsigned, 16},
		{"openvpn", "openvpn.opcode", "OpenVPN opcode", "Opcode", "pdu", DisplayUnsigned, 8},
		{"openvpn", "openvpn.keyid", "OpenVPN key ID", "Key ID", "pdu", DisplayUnsigned, 8},
		{"ipmi", "rmcp.version", "RMCP version", "RMCP Version", "pdu", DisplayUnsigned, 8},
		{"ipmi", "rmcp.class", "RMCP class", "RMCP Class", "pdu", DisplayUnsigned, 8},
		{"ipmi", "ipmi.payload.encrypted", "IPMI payload encryption flag", "Payload Encrypted", "pdu", DisplayBoolean, 0},
		{"telnet", "telnet.command", "Telnet command", "Command", "pdu", DisplayUnsigned, 8},
		{"telnet", "telnet.option", "Telnet option", "Option", "pdu", DisplayUnsigned, 8},
		{"nbss", "nbss.type", "NBSS message type", "Type", "pdu", DisplayUnsigned, 8},
		{"nbss", "nbss.length", "NBSS payload length", "Length", "pdu", DisplayUnsigned, 17},
		{"nbss", "nbss.called_name", "NBSS called name", "Called Name", "pdu", DisplayString, 0},
		{"nbss", "nbss.calling_name", "NBSS calling name", "Calling Name", "pdu", DisplayString, 0},
	} {
		binding := displayRoot(f.protocol, f.name, f.label, f.typ, f.bits, f.origin, f.key)
		if f.protocol == "rtmp" {
			binding.protocols = "|rtmp|amf0|"
		}
		out = append(out, binding)
	}
	b = displayCustom("tls", "tls.record.content_type", "TLS interpreted content type", DisplayUnsigned, 8, "derived", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
		version, _ := displayUint(e.Session["Negotiated Version"])
		typ, _ := displayUint(e.Session["Record Type"])
		if version == 0 && typ == 23 {
			return // midstream framing cannot establish TLS 1.2 vs TLS 1.3 semantics
		}
		if version == 0x304 && typ == 23 {
			// TLS 1.3 outer application_data is an opaque envelope, not an
			// authenticated statement about its TLSInnerPlaintext content type.
			if e.Session["Authentication Verified"] == true {
				emit(e.Session["Inner Content Type"])
			}
			return
		}
		emit(e.Session["Record Type"])
	})
	b.descriptor.Description = "TLS 1.3 inner type only after authenticated decryption; otherwise clear/known legacy type. Midstream encrypted records without negotiation expose only tls.record.wire_type."
	out = append(out, b)
	b = displayCustom("tls", "tls.record.opaque_type", "TLS 1.3 opaque record type", DisplayUnsigned, 8, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
		version, _ := displayUint(e.Session["Negotiated Version"])
		typ, _ := displayUint(e.Session["Record Type"])
		if version == 0x304 && typ == 23 {
			emit(typ)
		}
	})
	out = append(out, b)
	path("rip", "rip", "rip.metric", "RIP route metric", DisplayUnsigned, 32, "pdu", "Entries", "*", "Metric")
	path("http2|grpc|doh", "http2", "http2.streamid", "HTTP/2 stream ID", DisplayUnsigned, 31, "pdu", "Stream ID")
	path("grpc", "grpc", "grpc.status", "gRPC status", DisplayUnsigned, 32, "pdu", "GRPC Status")
	for _, f := range []struct {
		name, label string
		typ         DisplayFieldType
		bits        int
	}{
		{"http.request.method", "HTTP request method", DisplayString, 0},
		{"http.response.code", "HTTP response status", DisplayUnsigned, 16},
	} {
		request := f.typ == DisplayString
		b := displayCustom("http", f.name, f.label, f.typ, f.bits, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) { displayHTTPValues(e, request, emit, p) })
		b.protocols = "|http|ipp|http2|grpc|doh|http3|"
		out = append(out, b)
	}
	b = displayCustom("http3", "http3.streamid", "HTTP/3 stream ID", DisplayUnsigned, 62, "pdu", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
		p.maps(e.Session["Frames"], func(fr map[string]any) {
			if fr["HTTP3"] == true {
				emit(fr["Stream ID"])
			}
		})
	})
	out = append(out, b)
	sort.Slice(out, func(i, j int) bool { return out[i].descriptor.Name < out[j].descriptor.Name })
	seen := map[string]bool{}
	for i, b := range out {
		out[i].descriptor.Values = displayFieldEnums(b.descriptor.Name)
		if b.descriptor.Name == "" || seen[b.descriptor.Name] {
			panic("duplicate or empty display field registration")
		}
		seen[b.descriptor.Name] = true
	}
	return out
}

func cloneDisplayFieldDescriptor(d DisplayFieldDescriptor) DisplayFieldDescriptor {
	d.Values = append([]DisplayFieldEnum(nil), d.Values...)
	return d
}

var displayFieldBindings = buildDisplayFieldBindings()
var displayFieldIndex = func() map[string]int {
	m := make(map[string]int, len(displayFieldBindings))
	for i, b := range displayFieldBindings {
		m[b.descriptor.Name] = i
	}
	return m
}()

func RegisteredDisplayFields() []DisplayFieldDescriptor {
	out := make([]DisplayFieldDescriptor, len(displayFieldBindings))
	for i, b := range displayFieldBindings {
		out[i] = cloneDisplayFieldDescriptor(b.descriptor)
	}
	return out
}

func LookupDisplayField(name string) (DisplayFieldDescriptor, bool) {
	i, ok := displayFieldIndex[name]
	if !ok {
		return DisplayFieldDescriptor{}, false
	}
	return cloneDisplayFieldDescriptor(displayFieldBindings[i].descriptor), true
}

// DisplayFields is the compatibility map projection of the same registration
// consumed by the typed filter compiler and exported occurrence snapshots.
func (e *ProtocolEvent) DisplayFields() map[string][]any {
	return e.DisplayFieldValues().FieldMap()
}

func (e *ProtocolEvent) DisplayFieldValues() DisplayFieldSnapshot {
	s := DisplayFieldSnapshot{SchemaVersion: DisplayFieldSchemaVersion, Values: []DisplayFieldValue{}}
	if e == nil {
		return s
	}
	if e.nativeSessionMessage && e.semanticSession != nil {
		owned := *e
		owned.Session = e.semanticSession
		e = &owned
	}
	s.PDU, s.Source, s.Completeness = e.ID, e.SourceBytes, e.Completeness
	if e.nativeFieldsIncomplete {
		s.Incomplete = true
	}
	if e.SourceBytes.CompletedIn != nil {
		owned := *e.SourceBytes.CompletedIn
		s.Source.CompletedIn = &owned
	}
	s.Source.ParentPDUs = append([]uint64(nil), e.SourceBytes.ParentPDUs...)
	s.Source.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
	p := &displayProjection{snapshot: &s, remaining: maxDisplayFieldValues * 4}
	// Reserve event facts before spending the projection budget on PDU arrays.
	for _, origin := range []string{"event", "protocol"} {
		for _, b := range displayFieldBindings {
			if (b.descriptor.Origin == "event") != (origin == "event") {
				continue
			}
			if b.protocols != "" && !strings.Contains(b.protocols, "|"+e.Protocol+"|") {
				continue
			}
			emit := func(value any) {
				if value == nil || value == "" && !b.descriptor.AllowEmpty {
					return
				}
				if b.descriptor.Name == "tls.version" || b.descriptor.Name == "tls.handshake.ciphersuite" {
					if v, ok := displayUint(value); ok && v == 0 {
						return
					}
				}
				v, ok := normalizeDisplayValue(value, b.descriptor)
				if !ok {
					s.Incomplete = true
					return
				}
				if len(s.Values) >= maxDisplayFieldValues {
					s.Truncated = true
					return
				}
				item := DisplayFieldValue{Name: b.descriptor.Name, Value: v, Origin: b.descriptor.Origin}
				if b.descriptor.Type == DisplayUnsigned && b.descriptor.Bits > 53 {
					item.ExactValue = strconv.FormatUint(v.(uint64), 10)
				}
				if b.descriptor.Origin == "pdu" {
					item.Range = e.displayPDUFieldRange(b.rangeKey)
				}
				s.Values = append(s.Values, item)
			}
			if b.custom != nil {
				b.custom(e, emit, p)
			} else {
				p.walk(e.Session, b.path, emit)
			}
		}
	}

	return s
}

func normalizeDisplayValue(value any, d DisplayFieldDescriptor) (any, bool) {
	if value == nil {
		return nil, false
	}
	switch d.Type {
	case DisplayString:
		v, ok := value.(string)
		return v, ok && (v != "" || d.AllowEmpty) && utf8.ValidString(v)
	case DisplayBoolean:
		v, ok := value.(bool)
		return v, ok
	case DisplayAddress:
		v, ok := value.(string)
		if !ok {
			return nil, false
		}
		a, err := netip.ParseAddr(v)
		if err != nil {
			return nil, false
		}
		if d.Bits == 128 {
			// A literal IPv6 wire field must retain all 128 bits, including
			// mapped addresses; generic endpoint fields still use Unmap.
			return a.String(), a.Is6()
		}
		return a.Unmap().String(), true
	case DisplayUnsigned:
		v, ok := displayUint(value)
		if !ok || d.Bits < 64 && v >= uint64(1)<<d.Bits {
			return nil, false
		}
		return v, true
	}
	return nil, false
}

func displayUint(value any) (uint64, bool) {
	switch v := value.(type) {
	case uint64:
		return v, true
	case uint:
		return uint64(v), true
	case uint32:
		return uint64(v), true
	case uint16:
		return uint64(v), true
	case uint8:
		return uint64(v), true
	case int:
		return uint64(v), v >= 0
	case int64:
		return uint64(v), v >= 0
	case int32:
		return uint64(v), v >= 0
	case int16:
		return uint64(v), v >= 0
	case int8:
		return uint64(v), v >= 0
	case float64:
		// JSON users must retain json.Number for larger integers. A rounded
		// float is not evidence of an exact 64-bit protocol value.
		return uint64(v), v >= 0 && v <= 1<<53-1 && math.Trunc(v) == v
	case json.Number:
		n, err := strconv.ParseUint(string(v), 10, 64)
		return n, err == nil
	case string:
		n, err := strconv.ParseUint(v, 10, 64)
		return n, err == nil
	}
	return 0, false
}

type displayProjection struct {
	snapshot  *DisplayFieldSnapshot
	remaining int
}

func (p *displayProjection) spend() bool {
	if p.remaining <= 0 {
		p.snapshot.Truncated = true
		return false
	}
	p.remaining--
	return true
}

func (p *displayProjection) walk(value any, path []string, emit func(any)) {
	if !p.spend() {
		return
	}
	if len(path) == 0 {
		emit(value)
		return
	}
	if path[0] == "*" {
		p.maps(value, func(m map[string]any) { p.walk(m, path[1:], emit) })
		return
	}
	if m, ok := value.(map[string]any); ok {
		p.walk(m[path[0]], path[1:], emit)
	}
}

func (p *displayProjection) maps(value any, fn func(map[string]any)) {
	switch v := value.(type) {
	case []map[string]any:
		for _, m := range v {
			if !p.spend() {
				return
			}
			fn(m)
		}
	case []any:
		for _, item := range v {
			if !p.spend() {
				return
			}
			if m, ok := item.(map[string]any); ok {
				fn(m)
			}
		}
	}
}

func displayHTTPValues(e *ProtocolEvent, request bool, emit func(any), p *displayProjection) {
	header := ":status"
	if request {
		header = ":method"
	}
	headers := func(value any) {
		p.maps(value, func(h map[string]any) {
			if h["Name"] == header {
				emit(h["Value"])
			}
		})
	}
	if e.Protocol == "http3" {
		p.maps(e.Session["Frames"], func(fr map[string]any) {
			if fr["HTTP3"] != true {
				return
			}
			p.maps(fr["HTTP3 Frames"], func(f map[string]any) { headers(f["Headers"]) })
		})
		return
	}
	if _, h2 := e.Session["Stream ID"]; h2 {
		headers(e.Session["Headers"])
		return
	}
	if e.Protocol != "http" && e.Protocol != "doh" && e.Protocol != "ipp" {
		return
	}
	// Read only an already-admitted HTTP/1 PDU's start line. No re-dissection,
	// heuristic port lookup or interpretation of HTTP/2 frame bytes occurs here.
	line := e.Raw[:min(len(e.Raw), 8192)]
	if at := strings.IndexByte(string(line), '\n'); at >= 0 {
		line = line[:at]
	}
	parts := strings.SplitN(string(line), " ", 3)
	if len(parts) < 2 {
		return
	}
	if strings.HasPrefix(parts[0], "HTTP/") {
		if !request {
			emit(parts[1])
		}
	} else if request {
		emit(parts[0])
	}
}

func (e *ProtocolEvent) displayPDUFieldRange(key string) *DisplayFieldRange {
	if key == "" {
		return nil
	}
	ranges, _ := e.Session["Wire Field Ranges"].(map[string]any)
	span, _ := ranges[key].([]any)
	if len(span) != 2 {
		return nil
	}
	lo, ok := displayUint(span[0])
	hi, ok2 := displayUint(span[1])
	if !ok || !ok2 || hi < lo || hi > uint64(len(e.Raw)) {
		return nil
	}
	return &DisplayFieldRange{Offset: int(lo), Length: int(hi - lo)}
}

func validateDisplayFieldLiteral(d DisplayFieldDescriptor, op string, token filterToken) (any, error) {
	fail := func() (any, error) {
		return nil, fmt.Errorf("display field %s (%s) does not support %s %q", d.Name, d.Type, op, token.s)
	}
	if op == "in" {
		if d.Type != DisplayAddress {
			return fail()
		}
		p, err := netip.ParsePrefix(token.s)
		if err != nil {
			return fail()
		}
		if d.Bits == 128 && !p.Addr().Is6() {
			return fail()
		}
		if d.Bits != 128 && p.Addr().Is4In6() && p.Bits() >= 96 {
			p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
		}
		return p.Masked(), nil
	}
	if op == "contains" {
		if d.Type != DisplayString {
			return fail()
		}
		return token.s, nil
	}
	if (d.Type == DisplayBoolean || d.Type == DisplayAddress) && op != "==" && op != "!=" {
		return fail()
	}
	switch d.Type {
	case DisplayUnsigned:
		if token.quoted {
			return fail()
		}
		base := 10
		if strings.HasPrefix(token.s, "0x") || strings.HasPrefix(token.s, "0X") {
			base = 0
		}
		v, err := strconv.ParseUint(token.s, base, d.Bits)
		if err != nil {
			return fail()
		}
		return v, nil
	case DisplayBoolean:
		if token.quoted || token.s != "true" && token.s != "false" {
			return fail()
		}
		return token.s == "true", nil
	case DisplayAddress:
		a, err := netip.ParseAddr(token.s)
		if err != nil {
			return fail()
		}
		if d.Bits == 128 {
			if !a.Is6() {
				return fail()
			}
			return a, nil
		}
		return a.Unmap(), nil
	case DisplayString:
		return token.s, nil
	}
	return fail()
}

func displayFieldEnums(name string) []DisplayFieldEnum {
	switch name {
	case "wg.type":
		return []DisplayFieldEnum{{1, "Handshake Initiation"}, {2, "Handshake Response"}, {3, "Cookie Reply"}, {4, "Transport Data"}}
	case "tls.record.content_type", "tls.record.wire_type":
		return []DisplayFieldEnum{{20, "Change Cipher Spec"}, {21, "Alert"}, {22, "Handshake"}, {23, "Application Data"}}
	case "tls.record.opaque_type":
		return []DisplayFieldEnum{{23, "TLS 1.3 opaque encrypted record"}}
	case "isakmp.exchangetype":
		return []DisplayFieldEnum{{34, "IKE_SA_INIT"}, {35, "IKE_AUTH"}, {36, "CREATE_CHILD_SA"}, {37, "INFORMATIONAL"}}
	case "rip.command", "ripng.cmd":
		return []DisplayFieldEnum{{1, "Request"}, {2, "Response"}}
	case "telnet.command":
		return []DisplayFieldEnum{{250, "Subnegotiation"}, {251, "WILL"}, {252, "WONT"}, {253, "DO"}, {254, "DONT"}, {255, "Escaped IAC"}}
	case "nbss.type":
		return []DisplayFieldEnum{{129, "Session Request"}, {130, "Positive Session Response"}, {131, "Negative Session Response"}, {132, "Retarget Session Response"}, {133, "Session Keepalive"}}
	}
	return nil
}

func (s DisplayFieldSnapshot) FieldMap() map[string][]any {
	m := map[string][]any{}
	for _, v := range s.Values {
		value := v.Value
		if d, ok := LookupDisplayField(v.Name); ok && d.Type == DisplayUnsigned && d.Bits > 53 && v.ExactValue != "" {
			if n, err := strconv.ParseUint(v.ExactValue, 10, d.Bits); err == nil {
				value = n
			}
		}
		m[v.Name] = append(m[v.Name], value)
	}
	return m
}
