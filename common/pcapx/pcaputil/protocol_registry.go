package pcaputil

import "fmt"

// ProtocolProfile describes the native ingress implemented by this registry.
// Probe evidence and user DecodeAs selection are separate event properties.
type ProtocolProfile struct{ Protocol, Transport, Framing, Context string }

func NativeProtocolProfiles() []ProtocolProfile {
	return []ProtocolProfile{
		{"knx", "udp", "KNXnet/IP v1 basic and selected extended UDP search", "bounded HPAI, basic discovery plus extended SRP1-4 and DIB1/2/6; raw unknown DIBs, per-datagram unverified observations without multicast pairing, filter verification, tunneling, cEMI or secure-session claim"},
		{"pfcp", "udp", "PFCPv1 Heartbeat, AssociationSetup5/6 Update7/8/Release9/10 and unbundled SessionDeletion54/55 selected core", "TS29.244 v16.12.1, first-wins bounded IEs and exact selected NodeID/FQDN/Cause/features; Heartbeat Recovery is mandatory and Setup Recovery does not drive restart; shared originating-endpoint sequence namespace across peer/message kinds, exact response-type/endpoint/domain observation, ambiguity quarantine and erroneous supported reply retirement; receiver-local uint64 SEIDs, Cause2/AURI follow-on-report claims, priority and ordered raw report groups; optional independent SEID binding on an isolated UDP byte session; observed release flags and graceful timer do not prove authenticated association, live session deletion, negotiated features or restart"},
		{"c37118", "tcp/udp", "v1/v2 complete CFG-2 and configuration-dependent DATA", "same observed publisher and capture-domain configuration, raw channel encodings and bounded validated replacement; no CFG-3, scaling, authenticated source or measurement quality claim"},
		{"opendroneid", "explicit-udp", "protocol2 Basic ID/Location and selected message packs", "raw25-byte messages or exact pack; ordered unverified IDs/location, reserved fields preserved; no automatic admission, ODID prefix, BLE/Wi-Fi broadcast, authentication or aircraft/position validity claim"},
		{"dlms-wrapper", "tcp/udp", "Wrapper v1 selected unciphered LN Get-normal/Get-with-list", "exact WPDU/APDU, ordered normal/list/block descriptors and selected scalar/octet/bits/float/text/calendar/compact/array/structure/error results; list caps64 items/1024 octets, aggregate256 Data nodes/8 levels plus caller budgets; structured selective parameters without selector meaning; reversed carrier/wPort/invoke/full-flags/choice/count observations, invoke reuse ambiguous; pre-established AA may be unseen; no authentication, negotiated conformance, object model, unselected Data, GBT/cipher/ACSE claim"},
		{"dlms", "tcp/udp", "complete standalone HDLC type3 tunnel frames", "CRC/address/link controls and selected unciphered LN Get-normal scalars with observed endpoints, invoke and link sequence; ambiguity blocks association; ordered bounded GET lists with exact full invoke/choice/count/address/NSNR observations; no IP wrapper, ACSE authentication, ciphering, block/segmented transfer or meter object semantics"},
		{"slmp", "tcp/udp", "binary3E/4E connected-station Self Test0619/0000", "exact timer/count/loopback/error fields and observed requester; UDP one outstanding route/serial/echo match with ambiguous overlap quarantined; TCP bounded distinct-serial 4E pipeline and sequential distinct-echo 3E success; completed identifier reuse and later unsequenced TCP errors remain context-required; no ASCII, other commands, private MELSOFT or authenticated device claim"},
		{"dronecan", "can", "explicit-interface v0 single-frame NodeStatus", "standard message type341 complete fields; strict tail and DSDL bit order; reserved Mode/nonzero SubMode preserved; no multi-frame CRC/reassembly, anonymous/services, CAN FD application, Cyphal v1 or authenticated/online node inference"},
		{"socketcan", "can", "DLT227 controller records", "network-order CAN IDs; classical data/RTR/controller-error details and canonical72-byteFD; opaque padding, no bus CRC/authenticated peer or transaction claim"},
		{"j1939", "can", "explicit-interface classical CAN", "Request PGN and unverified Address Claim NAME fields; other parameter groups stay opaque; no TP reassembly, FD application, address-claim success, ISO-TP/UDS/OBD or automatic extended-ID admission"},
		{"profinet-dcp", "l2", "bounded Identify request/successresponse properties", "FrameID fefe/feff and service5; request filters omit BlockInfo; VLAN/interface-scoped passive candidates, unknown blocks raw; no authenticated device or IO/Set/Get/Hello claim"},
		{"lldp", "l2", "ended Ethernet discovery TLVs", "ordered mandatory identity/TTL and bounded optional/management fields; selected PNO Port Status/Chassis MAC and IEEE802.3 MAC/PHY; unverified neighbor observation, no topology cache or PROFINET DCP/RT claim"},
		{"genisys", "tcp", "bounded escaped serial wire messages", "pinned reverse-engineered CRC0 and received-trace CRCFFFF dialects stay distinct and fixed per flow; ordered raw address/value pairs; checksum-free controls need prior strong admission; no signal meaning, authentication or checkback/request association claim"},
		{"bsap", "udp", "Jan-2022 local serial-carried RDB ReadByName", "DLE/CRC validation, endpoint/application sequence and one bounded request; selected type/logical/string values and errors; value-only responses require type context; no analog interpretation, global routing or native BSAP-IP"},
		{"roc-plus", "tcp", "October 2022 Read/Set Real-time Clock and Error Indicator frames", "observed TCP initiator and reversed logical station addresses with one pending request; overlaps ambiguous until close, Ethernet CRC diagnostic, reported clock unauthenticated; no other opcodes or serial receiver"},
		{"rtps", "udp", "RTPS2.1-2.5 SPDP parameter-list discovery and inline KeyHash/StatusInfo lifecycle DATA, INFO_TS and PAD", "observed GUID, sequence, endpoint set, lease and locators; repeated claims remain ordered and conflicting scalars ambiguous; no topic CDR, DATA_FRAG, reliable-writer or DDS Security inference"},
		{"semtech-downlink-session", "explicit-udp", "Semtech v2 PULL_RESP/TX_ACK passive association", "opt-in exact endpoints/capture domain/token and full JSON observation; capture-lifetime token quarantine,30second pending expiry and fail-closed bounded state; no authentication, gateway identity binding or RF delivery proof"},
		{"semtech-udp", "udp", "Semtech packet-forwarder v2 upstream and PULL_RESP/TX_ACK observations", "fixed v4.0.1 sender fields, exact JSON numbers/base64 and unresolved sender defaults; observed gateway/token only, no token association, radio config/GPS validation, RF delivery or LoRaWAN PHY/security"},
		{"mavlink", "udp", "MAVLink v1/v2 HEARTBEAT, SYS_STATUS and GLOBAL_POSITION_INT", "pinned common-message CRC_EXTRA and complete datagram; v2 signatures opaque and unverified"},
		{"nmea", "udp", "complete NMEA 0183 GGA/RMC sentences", "XOR checksum, bounded fields and coordinates; position is observed and unauthenticated; no AIS"},
		{"memcached", "tcp", "text get/gets/set and binary GET/SET", "observed direction, text reply order or binary opaque ID; values bounded by declared length; legacy stats layout remains available"},
		{"doip", "tcp/udp", "DoIP v2/v3 TCP routing activation/alive/diagnostic and UDP VehicleIdentification0001-0004", "TCP observed routing/logical addresses and UDS DiagnosticSessionControl positive/negative/pending only; UDP exact datagram discovery fields, request-only FF and raw invalidity/unknown values, unassociated unverified observations; no vehicle identity/synchronization verification, TLS, manufacturer policy or v4 claim"},
		{"nats", "tcp", "plaintext client control/payload frames including HPUB/HMSG", "observed INFO/CONNECT capabilities, client direction and subscription; no TLS plaintext inference"},
		{"stomp", "tcp", "STOMP 1.0/1.1/1.2 commands and binary bodies", "observed CONNECT/CONNECTED version, escapes, content-length and transaction state"},
		{"opcua", "tcp", "UA TCP HEL/ACK/RHE and bounded secure-channel chunks", "observed endpoints/channel/policy; protected OPN/MSG/CLO payload remains opaque"},
		{"goose", "l2", "IEC 61850 Ethernet GOOSE BER", "EtherType/application ID and bounded BER fields; subscriber identity remains unverified"},
		{"sv", "l2", "IEC 61850 Sampled Values bounded raw ASDUs", "EtherType, declared length, ordered mandatory/optional fields; dataset octets without SCL interpretation; extensions require another profile"},
		{"ethercat", "l2", "EtherCAT type 1 datagram chain, commands 0..14", "declared bounded frame, command address layout, R/C/M flags and raw WKC; no device topology or operation-success inference"},
		{"dtls", "udp", "bounded DTLS 1.0/1.2 records and handshake fragments", "endpoint pair/domain/epoch/direction/message_seq; authenticated identity and payload decryption are not inferred"},
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
		{"atg", "tcp", "TLS-450 SOH+i201TT computer-format inventory", "observed TCP initiator, command echo, bounded tank records, checksum and request order; units unobserved"},
		{"gearman", "tcp", "12-byte binary header and exact payload length", "request/response magic, supported type and bounded method-specific fields"},
		{"beanstalkd", "tcp", "CRLF commands with bounded put/reserved body", "strict put syntax on port 11300 and observed response order"},
		{"bjnp", "udp", "16-byte printer datagram header and exact payload", "observed command family, direction, sequence and session; port hint or explicit DecodeAs"},
		{"dns", "udp/tcp", "datagram or two-byte TCP length prefix", "question and endpoint transaction"}, {"mdns", "udp", "datagram", "multicast observation"}, {"llmnr", "udp", "datagram", "domain/requester/question; bounded multicast responders"}, {"dhcp", "udp", "datagram", "domain/client/xid observation"}, {"dhcpv6", "udp", "datagram", "domain/relay/client/xid observation"}, {"syslog", "udp/tcp", "RFC 5424 v1, bounded RFC 3164; UDP datagram or RFC 6587", "PRI signature/port hint or explicit DecodeAs; hostname is unverified message content"},
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
		case "dns", "mdns", "llmnr", "dhcp", "dhcpv6", "syslog", "snmp", "mqtt-sn", "bittorrent-dht", "bjnp", "rtp", "sip", "stun", "turn", "dtls", "mavlink", "nmea", "semtech-udp", "semtech-downlink-session", "rtps", "bsap", "slmp", "dlms", "opendroneid", "c37118", "knx", "pfcp", "dlms-wrapper", "doip":
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

// WithCANDecodeAs selects a bounded J1939 or DroneCAN classical profile on one
// capture interface. Selection is required because extended CAN IDs are also
// used by other protocols. It does not authenticate an address claim or enable
// automotive ISO-TP (the existing isotp rule describes RFC1006/COTP).
func WithCANDecodeAs(captureInterface int, protocol string) CaptureOption {
	return func(c *CaptureConfig) error {
		if captureInterface < 0 || (protocol != "j1939" && protocol != "dronecan") {
			return fmt.Errorf("CAN DecodeAs requires a nonnegative capture interface and a supported j1939/dronecan profile")
		}
		if c.canDecodeAs == nil {
			c.canDecodeAs = make(map[int]string)
		}
		if old := c.canDecodeAs[captureInterface]; old != "" && old != protocol {
			return fmt.Errorf("conflicting CAN DecodeAs profile")
		}
		c.canDecodeAs[captureInterface] = protocol
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
	{"rtps", nil, rtpsDatagramHeader, decodeRTPSDatagram},
	{"semtech-udp", []uint16{1700}, validSemtechDatagram, decodeSemtechDatagram},
	{"nmea", nil, validNMEADatagram, decodeNMEADatagram},
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
	if explicit == "semtech-downlink-session" {
		return a.decodeSemtechCorrelatedDatagram(e, w)
	}
	if (explicit == "" || explicit == "semtech-udp") && a.decodeSemtechDownlinkDatagram(e, w, src, dst, explicit == "semtech-udp") {
		return true
	}
	if (explicit == "" || explicit == "doip") && a.decodeDoIPDiscoveryDatagram(e, w, src, dst, explicit == "doip") {
		return true
	}
	if (explicit == "" || explicit == "knx" || explicit == "pfcp") && a.decodeDiscoveryDatagram(e, w, src, dst, explicit) {
		return true
	}
	if explicit == "c37118" || explicit == "" && c37118DatagramEvidence(w) {
		return a.decodeC37118Datagram(e, w, explicit == "c37118")
	}
	if explicit == "opendroneid" {
		return a.decodeOpenDroneIDDatagram(e, w)
	}
	if (explicit == "" || explicit == "dlms-wrapper") && a.decodeWrapperDatagram(e, w, src, dst, explicit == "dlms-wrapper") {
		return true
	}
	if explicit == "dlms" || explicit == "" && len(w) > 0 && w[0] == 0x7e {
		if a.decodeDLMSDatagram(e, w, explicit == "dlms") {
			return true
		}
	}
	if explicit == "slmp" || explicit == "" && slmpStart(w) {
		if a.decodeSLMPDatagram(e, w, explicit == "slmp") {
			return true
		}
	}
	if explicit == "mavlink" {
		return false // multi-message framing is handled before this registry
	}
	if explicit == "bsap" || explicit == "" && bsapStart(w) {
		if a.decodeBSAPDatagram(e, w, explicit == "bsap") {
			return true
		}
	}
	if explicit == "" && a.decodeBACnetDatagram(e, w) {
		return true
	}
	if explicit == "dtls" {
		return a.decodeDTLSDatagram(e, w, true)
	}
	if explicit == "" && a.decodeDTLSDatagram(e, w, false) {
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
	if explicit == "" && len(w) == 48 && probeNTP(w, len(w)).Verdict == ProbeAccept {
		return a.decodeSessionDatagram(e, w)
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
		} else if !p.validate(w) || !hint && !(p.name == "semtech-udp" && len(w) > 12 && w[3] == 0 || p.name == "dns" && dnsDatagramEvidence(w) || p.name == "snmp" && probeDatagram(w, a.config.MaxMessageBytes).Protocol == "snmp") {
			continue
		}
		e.Protocol = p.name
		e.Profile = p.name + "-native"
		e.Admission = "wire-and-port-hint"
		if len(p.ports) == 0 || !hint {
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
		if p.name != "rtps" || err == nil {
			e.Structured = map[string]any{"fields": e.semanticFields}
		}
		e.Status = "decoded"
		e.Summary = p.name
		a.messages.Add(1)
		a.messageBytes.Add(uint64(len(w)))
		if err != nil {
			e.Status, e.sessionError = classifySessionError(err)
			e.Error = err.Error()
			switch e.Status {
			case "limited":
				a.limited.Add(uint64(len(w)))
			case "context-required":
				a.contextRequired.Add(1)
			case "incomplete":
				a.incomplete.Add(1)
			default:
				a.malformed.Add(1)
			}
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
