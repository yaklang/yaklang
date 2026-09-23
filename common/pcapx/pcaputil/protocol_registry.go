package pcaputil

import "fmt"

// ProtocolProfile describes the native ingress implemented by this registry.
// Probe evidence and user DecodeAs selection are separate event properties.
type ProtocolProfile struct{ Protocol, Transport, Framing, Context string }

func NativeProtocolProfiles() []ProtocolProfile {
	return []ProtocolProfile{
		{"quic", "udp", "v1 protected datagram/coalesced packets", "capture domain/CID; Initial and ClientRandom-selected keylog"},
		{"http3", "quic", "reassembled stream frames", "authenticated h3 ALPN, peer SETTINGS and QPACK state"},
		{"doq", "quic", "one length-prefixed DNS message per direction", "authenticated doq ALPN, client stream and FIN"},
		{"mysql", "tcp", "classic packets", "observed greeting/capabilities; prepared metadata and binary rows"},
		{"postgresql", "tcp", "protocol 3.0 messages", "observed direction, statement/portal cycles and COPY state"},

		{"sip", "tcp/udp", "SIP message and observed transaction", "Via branch/CSeq plus Call-ID/tags; dialog remains observed, not authenticated"},
		{"rtp", "udp", "one RTP datagram or bounded RTCP compound", "captured SIP/SDP endpoint and payload mapping, matched RTSP SETUP UDP port pair, or explicit DecodeAs; SRTP payload opaque"},
		{"stun/turn", "udp/tcp", "STUN message or TURN ChannelData", "cookie/type or observed channel binding; ChannelData without prior binding is context-required"},
		{"socks5", "tcp", "method/auth/request/reply messages; successful CONNECT returns to carried-protocol detection", "complete valid method offer plus observed ordered negotiation; no port-only inference"},
		{"snmp", "udp", "SNMPv1/v2c/v3 datagram", "BER/version evidence and observed request IDs; v3 encrypted scoped PDU opaque"},
		{"mqtt-sn", "udp", "v1.2 one- or three-octet length datagram", "wire length, supported message type and complete type-specific fields; port hint or explicit DecodeAs"},
		{"bittorrent-dht", "udp", "complete KRPC bencoded datagram", "canonical bounded dictionary, transaction, 20-byte node ID and method-specific fields; port hint or explicit DecodeAs"},
		{"stratum", "tcp", "newline-delimited mining JSON-RPC", "complete mining request and observed request ID for response association"},
		{"gearman", "tcp", "12-byte binary header and exact payload length", "request/response magic, supported type and bounded method-specific fields"},
		{"beanstalkd", "tcp", "CRLF commands with bounded put/reserved body", "strict put syntax on port 11300 and observed response order"},
		{"bjnp", "udp", "16-byte printer datagram header and exact payload", "observed command family, direction, sequence and session; port hint or explicit DecodeAs"},
		{"dns", "udp", "datagram", "question and endpoint transaction"}, {"mdns", "udp", "datagram", "multicast observation"}, {"llmnr", "udp", "datagram", "domain/requester/question; bounded multicast responders"}, {"dhcp", "udp", "datagram", "domain/client/xid observation"}, {"dhcpv6", "udp", "datagram", "domain/relay/client/xid observation"}, {"syslog", "udp/tcp", "RFC 5424 v1, bounded RFC 3164; UDP datagram or RFC 6587", "PRI signature/port hint or explicit DecodeAs; hostname is unverified message content"},
		{"arp", "l2", "ethernet", "unverified neighbor"}, {"icmp", "network", "IP", "quoted packet observation"}, {"icmpv6", "network", "IPv6", "unverified neighbor"},
		{"http", "tcp", "ordered bytes", "observed request method"}, {"websocket", "tcp", "ordered bytes", "validated HTTP upgrade"}, {"tls", "tcp", "records", "ClientRandom/direction/epoch; TLS 1.3 non-PSK HelloRetryRequest"}, {"http2", "tcp", "frames", "preface and HPACK state"}, {"grpc", "tcp", "HTTP2 DATA", "content-type and stream"},
	}
}

// WithProtocolDecodeAs selects a native UDP codec on a port. Selection does not
// bypass validation or grant authenticated/decrypted status. Only listed native
// datagram profiles are accepted; legacy TCP Bindings remain source compatible.
func WithProtocolDecodeAs(transport string, port uint16, protocol string) CaptureOption {
	return func(c *CaptureConfig) error {
		if transport != "udp" || port == 0 {
			return fmt.Errorf("DecodeAs requires UDP and a nonzero port")
		}
		switch protocol {
		case "dns", "mdns", "llmnr", "dhcp", "dhcpv6", "syslog", "snmp", "mqtt-sn", "bittorrent-dht", "bjnp", "rtp", "sip", "stun", "turn":
		default:
			return fmt.Errorf("unsupported native DecodeAs profile")
		}
		if c.datagramDecodeAs == nil {
			c.datagramDecodeAs = map[uint16]string{}
		}
		if old := c.datagramDecodeAs[port]; old != "" && old != protocol {
			return fmt.Errorf("conflicting DecodeAs profile")
		}
		c.datagramDecodeAs[port] = protocol
		return nil
	}
}

type nativeDatagramProfile struct {
	name     string
	ports    []uint16
	validate func([]byte) bool
	decode   func([]byte, int) (map[string]any, error)
}

var nativeDatagrams = []nativeDatagramProfile{
	{"mdns", []uint16{5353}, dnsHeader, DecodeDNSMessage},
	{"llmnr", []uint16{5355}, dnsHeader, DecodeDNSMessage},
	{"dns", []uint16{53}, dnsHeader, DecodeDNSMessage},
	{"dhcpv6", []uint16{546, 547}, func(w []byte) bool { return len(w) >= 4 && w[0] >= 1 && w[0] <= 13 }, func(w []byte, n int) (map[string]any, error) { return decodeDHCPv6(w, 0, n) }},
	{"snmp", []uint16{161, 162}, func(w []byte) bool { return probeSNMP(w, len(w)).Verdict == ProbeAccept }, func(w []byte, n int) (map[string]any, error) { return (&binSNMP{}).consume(w, n, 0) }},
	{"mqtt-sn", []uint16{1883, 1884}, validMQTTSNMessage, decodeMQTTSNMessage},
	{"bittorrent-dht", []uint16{6881}, validDHTMessage, decodeDHTMessage},
	{"bjnp", []uint16{8611}, validBJNPMessage, decodeBJNPMessage},
	{"dhcp", nil, func(w []byte) bool { return len(w) >= 240 && probeDHCP(w, len(w)).Verdict == ProbeAccept }, decodeDHCP4},
	{"syslog", []uint16{514}, syslogValidDatagramStart, func(w []byte, n int) (map[string]any, error) {
		return (&binSyslog{}).consume(w, syslogDatagram, DefaultParserBudget().MaxMessageBytes, n)
	}},
}

func (a *binParser) decodeNativeDatagram(e *ProtocolEvent, w []byte, src, dst uint16) bool {
	explicit := a.datagramDecodeAs[dst]
	if explicit == "" {
		explicit = a.datagramDecodeAs[src]
	}
	if explicit == "" && (src == bacnetIPv4UDPPort || dst == bacnetIPv4UDPPort) && a.decodeBACnetDatagram(e, w) {
		return true
	}
	if explicit == "rtp" {
		match := sipMediaMatch{}
		rtcp, pt, err := inspectRTPDatagram(w, a.config.MaxMessageBytes, a.budget.MaxCollectionElements)
		if err == nil {
			match = a.matchSDPMedia(e.Domain, e.Source, e.Destination, pt, rtcp, e.Timestamp)
		}
		return a.decodeRTPDatagram(e, w, true, rtcp, match)
	}
	if explicit == "stun" || explicit == "turn" {
		if a.decodeSTUNDatagram(e, w, explicit == "turn") {
			e.Admission = "explicit-decode-as"
			return true
		}
		return false
	}
	if explicit == "sip" || explicit == "" && probeSIP(w, len(w)).Verdict == ProbeAccept {
		return a.decodeSIPDatagram(e, w, explicit == "sip")
	}
	if explicit == "" {
		// RTSP's matched SETUP response carries an exact bidirectional UDP
		// port pair. Consult it before weak RTP/RTCP payload heuristics so that
		// ambiguous mappings never get guessed into a media session.
		if media := a.matchRTSPMedia(e.Domain, e.Source, e.Destination, e.Timestamp); media.association != nil || media.ambiguous {
			return a.decodeRTSPMediaDatagram(e, w, media)
		}
	}
	if rtpIsRTCP(w) {
		// An RTP marker plus payload type 72-76 has the same second octet as
		// RTCP types 200-204. Prefer a valid RTCP packet only when captured SDP
		// signals this endpoint as RTCP; otherwise try a valid RTP packet on
		// the separately signaled RTP endpoint, even when RTCP length parsing
		// fails because those bytes are actually an RTP sequence number.
		if _, _, err := inspectRTPDatagramAs(w, a.config.MaxMessageBytes, a.budget.MaxCollectionElements, true); err == nil {
			match := a.matchSDPMedia(e.Domain, e.Source, e.Destination, 0, true, e.Timestamp)
			if match.status == "matched" || match.status == "observed-offer" || match.status == "ambiguous-sdp-match" {
				return a.decodeRTPDatagram(e, w, false, true, match)
			}
		}
		if _, pt, err := inspectRTPDatagramAs(w, a.config.MaxMessageBytes, a.budget.MaxCollectionElements, false); err == nil {
			match := a.matchSDPMedia(e.Domain, e.Source, e.Destination, pt, false, e.Timestamp)
			if match.status == "matched" || match.status == "observed-offer" || match.status == "ambiguous-sdp-match" {
				return a.decodeRTPDatagram(e, w, false, false, match)
			}
		}
	} else if _, pt, err := inspectRTPDatagram(w, a.config.MaxMessageBytes, a.budget.MaxCollectionElements); err == nil {
		match := a.matchSDPMedia(e.Domain, e.Source, e.Destination, pt, false, e.Timestamp)
		if match.status == "matched" || match.status == "observed-offer" || match.status == "ambiguous-sdp-match" {
			return a.decodeRTPDatagram(e, w, false, false, match)
		}
	}
	for _, p := range nativeDatagrams {
		hint := len(p.ports) == 0
		for _, port := range p.ports {
			hint = hint || src == port || dst == port
		}
		if explicit != "" {
			if explicit != p.name || !p.validate(w) {
				continue
			}
		} else if !hint || !p.validate(w) {
			continue
		}
		e.Protocol = p.name
		e.Profile = p.name + "-native"
		e.Admission = "wire-and-port-hint"
		if len(p.ports) == 0 {
			e.Admission = "wire-signature"
		}
		if explicit != "" {
			e.Admission = "explicit-decode-as"
		}
		e.Raw = append([]byte(nil), w...)
		if p.name == "syslog" {
			e.syslogFraming, e.syslogBudget = syslogDatagram, a.budget.MaxCollectionElements
			e.syslogMaxBytes = a.config.MaxMessageBytes
			e.Completeness = "message"
			e.Profile = syslogProfileForWire(w, syslogDatagram)
		}
		if p.name == "snmp" {
			return a.decodeSNMPDatagram(e, w, explicit)
		}
		if p.name == "syslog" && a.config.Deferred && len(w) <= a.config.MaxMessageBytes {
			if spec := a.specs["syslog/Syslog"]; spec != nil {
				e.Rule, e.Entry, e.plan = spec.rule, spec.entry, spec.plan
			}
			e.Status, e.Summary = "deferred", "syslog"
			a.messages.Add(1)
			a.messageBytes.Add(uint64(len(w)))
			a.deferred.Add(1)
			return true
		}
		var fields map[string]any
		var err error
		if p.name == "syslog" {
			fields, err = (&binSyslog{}).consume(w, syslogDatagram, a.config.MaxMessageBytes, a.budget.MaxCollectionElements)
		} else {
			fields, err = p.decode(w, a.budget.MaxCollectionElements)
		}
		e.Session = fields
		e.Completeness = "message"
		if err == nil {
			switch p.name {
			case "dns", "mdns", "llmnr":
				e.Session = map[string]any{}
				err = a.dnsEventDecoded(e, fields)
			case "dhcp":
				a.dhcpObservation(e)
			case "dhcpv6":
				a.dhcp6Observation(e)
			}
		}
		e.semanticFields = cloneSession(fields)
		e.Structured = map[string]any{"fields": e.semanticFields}
		e.Status = "decoded"
		e.Summary = p.name
		a.messages.Add(1)
		a.messageBytes.Add(uint64(len(w)))
		if err != nil {
			e.Status, e.sessionError = classifySessionError(err)
			e.Error = err.Error()
			a.malformed.Add(1)
		} else if a.config.Deferred {
			e.Status = "deferred"
			e.Structured = nil
			a.deferred.Add(1)
		} else {
			a.decoded.Add(1)
		}
		return true
	}
	return false
}
