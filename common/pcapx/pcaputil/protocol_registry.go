package pcaputil

import (
	"errors"
	"fmt"
)

// ProtocolProfile describes the native ingress implemented by this registry.
// Probe evidence and user DecodeAs selection are separate event properties.
type ProtocolProfile struct{ Protocol, Transport, Framing, Context string }

func NativeProtocolProfiles() []ProtocolProfile {
	return []ProtocolProfile{
		{"bacnet-routed-read-property", "explicit-udp", "BACnet/IP NPDUv1 selected direct or routed ReadProperty", "bounded one-to-six-byte logical source/destination MAC observations, network and present-only hop count; reversed physical endpoints plus logical route addresses/domain/invoke/descriptor association; unknown/refused routes quarantine the physical relay pair; same primitive/terminal APDU scope as direct profile, no network services, broadcast peer inference, forwarded BVLC, segmentation, device/authentication or router-operation claim"},
		{"bacnet-read-property", "explicit-udp", "BACnet/IP original unicast, NPDUv1 unsegmented ReadProperty", "bounded request/ComplexACK primitive application values and Error/Reject/server Abort; exact object/property/optional index and reversed endpoint/domain/invoke observation, reuse/retry/expiry quarantined; no constructed property schemas, segmentation, routing, client Abort, authentication or device-operation success claim"},
		{"knx", "udp", "KNXnet/IP v1 basic and selected extended UDP search", "bounded HPAI, basic discovery plus extended SRP1-4 and DIB1/2/6; raw unknown DIBs, per-datagram unverified observations without multicast pairing, filter verification, tunneling, cEMI or secure-session claim"},
		{"pfcp", "udp", "PFCPv1 Heartbeat, AssociationSetup5/6 Update7/8/Release9/10 and unbundled SessionDeletion54/55 selected core", "TS29.244 v16.12.1, first-wins bounded IEs and exact selected NodeID/FQDN/Cause/features; Heartbeat Recovery is mandatory and Setup Recovery does not drive restart; shared originating-endpoint sequence namespace across peer/message kinds, exact response-type/endpoint/domain observation, ambiguity quarantine and erroneous supported reply retirement; receiver-local uint64 SEIDs, Cause2/AURI follow-on-report claims, priority and ordered raw report groups; optional independent SEID binding on an isolated UDP byte session; observed release flags and graceful timer do not prove authenticated association, live session deletion, negotiated features or restart"},
		{"c37118-engineering", "tcp/udp", "opt-in v1/v2 CFG-2 or complete/collected CFG-3 DATA conversions", "same native publisher/configuration validation; raw fields preserved; configured phasor/frequency and CFG-3 analog conversion, declared GPA status bits and digital masks; CFG-2 integer analog convention requires independent context; no authenticated source, physical quality or inferred digital-label bit association"},
		{"c37118", "tcp/udp", "v1/v2 CFG-2, v2 complete CFG-3 and bounded TCP continuation/DATA", "same observed publisher, ID/version and capture-domain configuration; exact raw channel encodings and CFG-3 UTF8/UUID/conversion/coordinate fields with bounded validated replacement; TCP fragments follow fixed GPA/GSF producer framing with cumulative CRC, ordered indices, publisher ID/time equality and 30s capture lifetime; no UDP continuation, applied scaling, authenticated source or measurement quality claim"},
		{"opendroneid", "explicit-udp", "protocol2 Basic ID/Location and selected message packs", "raw25-byte messages or exact pack; ordered unverified IDs/location, reserved fields preserved; no automatic admission, ODID prefix, BLE/Wi-Fi broadcast, authentication or aircraft/position validity claim"},
		{"opendroneid-system", "explicit-udp", "protocol2 System with optional Basic ID/Location", "one System and at most two Basic IDs/one Location in an exact ordered pack; operator coordinates/area/classification/time and reserved wire values; no automatic admission, BLE/Wi-Fi, authentication, geographic validity or aircraft association"},
		{"opendroneid-identity", "explicit-udp", "protocol2 Self ID/Operator ID with optional Basic ID/Location/System", "at least one identity message; <=1 Self ID/Operator ID/System/Location and <=2 Basic IDs; ordered decoded byte strings and original wire bytes; no charset/identity/authentication claim or automatic BLE/Wi-Fi admission"},
		{"opendroneid-auth-pages", "explicit-udp", "protocol2 Authentication page observations with optional identity/location/System", "exact25-byte pages or bounded ordered packs; page zero capacity and unsigned time, 17-byte page-zero/23-byte later-page opaque data; repeated pages preserved, never assembled or verified; no identity/authentication claim, cross-datagram association or automatic BLE/Wi-Fi admission"},
		{"dlms-wrapper", "tcp/udp", "Wrapper v1 selected unciphered LN Get-normal/Get-with-list", "exact WPDU/APDU, ordered normal/list/block descriptors and selected scalar/octet/bits/float/text/calendar/compact/array/structure/error results; list caps64 items/1024 octets, aggregate256 Data nodes/8 levels plus caller budgets; structured selective parameters without selector meaning; reversed carrier/wPort/invoke/full-flags/choice/count observations, invoke reuse ambiguous; pre-established AA may be unseen; no authentication, object model, unselected Data or cipher claim; bounded GBT headers, first unambiguous directional transfer and observed ACK references reuse complete selected Get decoders; reuse/conflict/gap/unobserved ACKs remain unverified, negotiated window semantics not claimed; selected no-authentication LN ACSE AARQ/AARE and initiate v6 fields, conformance proposal/selection observations and first unambiguous reversed wPort response; missing initiate, reused/idle/refused contexts stay unverified; selected RLRQ/RLRE/ABRT reasons, optional selected initiate/literal abort information, scoped eligibility retirement and release request links; selected LOW mechanism2.16.756.5.8.2.1 calling-value observation is bounded to64 bytes with exact requirements/component order, never credential verification; orphan AARE retains charged canonical endpoint history so it cannot own later AARQ; no release-device-success claim"},
		{"dlms", "tcp/udp", "complete standalone HDLC type3 tunnel frames", "CRC/address/link controls and selected unciphered LN Get-normal scalars with observed endpoints, invoke and link sequence; ambiguity blocks association; ordered bounded GET lists with exact full invoke/choice/count/address/NSNR observations; bounded unciphered LN ACSE AARQ/AARE/initiate v6 with full logical addresses, observed modulo8 sequence, first-use history and orphan/refusal/Close ownership; selected RLRQ/RLRE/ABRT lifecycle with charged address/sequence history, release/abort retirement and typed unmatched Close; selected AARQ/AARE information assembly across at most8 distinct sequence frames and4099 information bytes capped further by caller budgets; partial information never installs/consumes identity, conflicting/gapped/orphan generations retain refusal; selected LOW calling authentication values/requirements/mechanism remain literal unverified observations with owned proposals; no IP wrapper, credential verification/HLS/ciphering, segmented release/abort or meter object semantics"},
		{"slmp", "tcp/udp", "binary3E/4E connected-station Self Test0619/0000", "exact timer/count/loopback/error fields and observed requester; UDP one outstanding route/serial/echo match with ambiguous overlap quarantined; TCP bounded distinct-serial 4E pipeline and sequential distinct-echo 3E success; completed identifier reuse and later unsequenced TCP errors remain context-required; no ASCII, other commands, private MELSOFT or authenticated device claim"},
		{"dronecan", "can", "explicit-interface v0 single-frame NodeStatus", "standard message type341 complete fields; strict tail and DSDL bit order; reserved Mode/nonzero SubMode preserved; no multi-frame CRC/reassembly, anonymous/services, CAN FD application, Cyphal v1 or authenticated/online node inference"},
		{"socketcan", "can", "DLT227 controller records", "network-order CAN IDs; classical data/RTR/controller-error details and canonical72-byteFD; opaque padding, no bus CRC/authenticated peer or transaction claim"},
		{"can-isotp", "can", "explicit physical RX/TX CAN ID pair, classical or FD, SF/FF/CF/FC", "selected normal/extended/mixed addressing, explicit static/dynamic flow control, disjoint address/carrier selectors and capture-time policy; optional DSC, TesterPresent, ECUReset types 1..5, RoutineControl types 1..3 with raw records, CommunicationControl types 0..5, DTC setting types 1..2 and single-DID RDBI wire observations; no physical ECU success, authenticated identity, arbitrary DID schema or indistinguishable stale-generation guarantee"},
		{"j1939-tp", "can", "explicit-interface classical TP BAM and RTS/CTS/EOM ACK/Abort", "bounded capture-domain/directed-address transport observations, complete literal payload/padding and matching control PGN; no ETP, FD application, device delivery/authentication or application PGN semantics"},
		{"j1939", "can", "explicit-interface classical CAN", "Request PGN and unverified Address Claim NAME fields; other parameter groups stay opaque; no TP reassembly, FD application, address-claim success, ISO-TP/UDS/OBD or automatic extended-ID admission"},
		{"profinet-dcp", "l2", "bounded Identify/Get/Set/Hello wire properties", "selected service3/4/5/6 layouts; Get selectors, Set qualifiers/control status, Hello BlockInfo; Identify VLAN/interface passive candidates; no authenticated device, configuration success or IO claim"},
		{"lldp", "l2", "ended Ethernet discovery TLVs", "ordered mandatory identity/TTL and bounded optional/management fields; selected PNO Port Status/Chassis MAC and IEEE802.3 MAC/PHY; unverified neighbor observation, no topology cache or PROFINET DCP/RT claim"},
		{"genisys", "tcp", "bounded escaped serial wire messages", "pinned reverse-engineered CRC0 and received-trace CRCFFFF dialects stay distinct and fixed per flow; ordered raw address/value pairs; checksum-free controls need prior strong admission; no signal meaning, authentication or checkback/request association claim"},
		{"bsap", "udp", "Jan-2022 local serial-carried RDB ReadByName", "DLE/CRC validation, endpoint/application sequence and one bounded request; selected type/logical/string values and errors; value-only responses require type context; no analog interpretation, global routing or native BSAP-IP"},
		{"roc-plus", "tcp", "October 2022 Read/Set Real-time Clock and Error Indicator frames", "observed TCP initiator and reversed logical station addresses with one pending request; overlaps ambiguous until close, Ethernet CRC diagnostic, reported clock unauthenticated; no other opcodes or serial receiver"},
		{"rtps", "udp", "RTPS2.0 keyed/no-key DATA wire fields and context; RTPS2.1-2.5 SPDP discovery, inline lifecycle DATA, complete same-datagram DATA_FRAG and receiver context", "version-specific key/status/QoS layout and owned opaque CDR/PL_CDR bytes, or SPDP GUID/sequence/endpoints/lease/locators; repeated claims remain ordered; logical source/destination and timestamp reset are message-local; UNKNOWN destination remains unknown; no RTPS1.3, unselected cross-datagram assembly, application type/schema, reliable-writer or DDS Security inference"},
		{"rtps-control", "explicit-udp", "RTPS2.0-2.5 common ACKNACK/GAP/HEARTBEAT/NACK_FRAG/HEARTBEAT_FRAG wire reports, wire2.4/2.5 group and wire2.5 filtered-count fields", "ordered bounded bitmap words/selected offsets, exact sequence words, reader/writer logical direction and message-local context; unknown prefixes stay unknown, owned fields and caller budgets; no inferred delivery, response link, authentication, DATA schema or reliable state machine"},
		{"rtps20-rti-filter", "explicit-udp", "RTPS2.0 reported RTI0101 inline locator-filter-list PID8006", "bounded literal channel/string-byte/padding and multichannel kind-address-port records; logical source vendor is required, unknown locator kinds remain raw; shared collection/depth/projection budgets; no filter evaluation, usable endpoint, authenticated vendor or device claim"},
		{"rtps13-parameters", "explicit-udp", "RTPS1.3 inline locator PIDs0x2f-0x33 and content-filter word records PID0x55", "owned literal kind/uint32 port/address and bounded ordered filter bitmap/four-word records; unknown kind/extensions preserved; shared wire/projection/collection/depth budgets; no inferred usable endpoint, filter truth, cryptographic authentication, schema or reliability"},
		{"rtps13-wire", "explicit-udp", "RTPS1.3 DATA/NOKEY, ACKNACK/GAP/HEARTBEAT and literal INFO_TS/SRC/DST", "fixed legacy layout observer with8-byte reported prefixes, DATA hash-before-sequence, literal zero/UNKNOWN sequence words and signed counters, bounded ordered bitmaps/parameters including non-four-byte values/PAD/repeats; INFO_SRC1.0-1.2 claims can be observed but their following DATA/control layout needs context; no inferred schema, reliable endpoint, confirmed key identity, negotiated QoS or DDS security; extended parameter encodings remain unselected"},
		{"rtps-wire", "explicit-udp", "RTPS2.0 legacy DATA/NOKEY and2.1-2.5 DATA with ordered reliability control reports", "owned opaque serialized payload/key and inline parameters, exact sequence words and logical source layouts; one shared message-local receiver context and atomic errors/budgets; optional exact caller-selected wire2.4/2.5 ShapeType PLAIN_CDR1 data/key fields; no inferred schema, reliable delivery, endpoint state or security"},
		{"semtech-downlink-session", "explicit-udp", "Semtech v2 PULL_RESP/TX_ACK passive association", "opt-in exact endpoints/capture domain/token and full JSON observation; capture-lifetime token quarantine,30second pending expiry and fail-closed bounded state; no authentication, gateway identity binding or RF delivery proof"},
		{"semtech-udp", "udp", "Semtech packet-forwarder v2 upstream and PULL_RESP/TX_ACK observations", "fixed v4.0.1 sender fields, exact JSON numbers/base64 and unresolved sender defaults; observed gateway/token only, no token association, radio config/GPS validation, RF delivery or LoRaWAN PHY/security"},
		{"mavlink", "udp", "MAVLink v1/v2 HEARTBEAT, SYS_STATUS, GPS_RAW_INT, ATTITUDE and GLOBAL_POSITION_INT", "pinned common-message CRC_EXTRA and complete datagram; GPS literal integers/sentinels and v2 extensions, ATTITUDE binary32 wire bits and zero-fill; v2 signatures opaque by default; opt-in caller-supplied key/clock verifies MAC and capture-domain/system/component/link replay under bounded retained history, no authenticated endpoint identity, navigation or physical attitude claim"},
		{"rtps-shape-fragments", "explicit-udp", "RTPS wire2.4/2.5 exact caller-selected final ShapeType PLAIN_CDR1 DATA/key DATA_FRAG", "bounded directed endpoint/domain and logical writer/reader/sequence reconstruction, consistent full DATA replacement, ordered literal messages and separately owned reconstructed bytes; metadata/overlap conflict and capture-lifetime retired identity quarantine,30second pending policy, byte/count/depth limits and Close release; other writers remain opaque; no inferred schema, reliable DDS reader/writer, authenticated association or security"},
		{"rtps-spdp-fragments", "explicit-udp", "RTPS2.1-2.5 SPDP serialized DATA across bounded complete DATA_FRAG ranges or an observed regular DATA replacement", "directed endpoints/domain plus logical writer/reader prefix/sequence; conflict and capture-lifetime identity quarantine,30second pending policy, owned full fields/contributors, byte/count/depth budgets and Close release; no DDS reliable-writer state, topic schema, authenticated sender or security claim"},
		{"nmea-ais-multipart", "explicit-udp", "Type5 across two ordered whole AIVDM/AIVDO sentences", "nonempty digit/channel labels and directed endpoints/capture domain;30second pending observation, capture-lifetime quarantine and bounded history; complete wire fields/private Session/contributors, no verified sender/generation/vessel or generic UDP stream joining"},
		{"nmea", "udp", "complete NMEA 0183 GGA/RMC, unit-labelled VTG, twelve-slot GSA, per-sentence GSV, single-fragment AIVDM/AIVDO Class A types1/2/3 and Class B Type18/19 and independent Type24 static parts and 96-bit Type27 long-range positions, same-datagram two-fragment Type5", "XOR checksum and bounded literal fields; optional VTG FAA modes A/D/E/M/N/S; GSA ordered PRN slots/DOP and System ID0..6; GSV ordered sparse slots and hexadecimal Signal ID; AIS 168-bit raw/scaled fields with unavailable sentinels, conditional SOTDMA/ITDMA or Type18 CS-fill and literal channel/sequence labels; same-datagram Type5 literal identity/dimensions/ETA/draught with unavailable/lower-bound markers; Type19 complete 312-bit navigation/name/dimensions/raw fix-type/DTE/RAIM reports; Type24 literal Part Number, both 160/168-bit Part A names and 168-bit Part B vendor/model/serial/callsign with dimensions or auxiliary mother-MMSI; Type27 signed 1/10-minute coordinates, integer speed/course, unavailable sentinels and position latency (not GNSS validity); no constellation association, GSV cycle or cross-datagram AIS association, ship identity, device capability, unlabelled VTG or authenticated physical fix"},
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
		case "bacnet-routed-read-property", "bacnet-read-property", "dns", "mdns", "llmnr", "dhcp", "dhcpv6", "syslog", "snmp", "mqtt-sn", "bittorrent-dht", "bjnp", "rtp", "sip", "stun", "turn", "dtls", "mavlink", "nmea", "nmea-ais-multipart", "semtech-udp", "semtech-downlink-session", "rtps", "rtps-control", "rtps-wire", "rtps13-wire", "rtps13-parameters", "rtps20-rti-filter", "rtps-shape-fragments", "rtps-spdp-fragments", "bsap", "slmp", "dlms", "opendroneid", "opendroneid-system", "opendroneid-identity", "opendroneid-auth-pages", "c37118", "c37118-engineering", "knx", "pfcp", "dlms-wrapper", "doip":
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
// j1939-tp additionally observes bounded classical BAM and RTS/CTS/EOM ACK/Abort
// transfers. It does not infer authenticated ECU sessions or application PGN values.
// dronecan-stats additionally selects the bounded v0 GetTransportStats response
// receiver; dronecan-services also observes GetNodeInfo responses. Neither
// selection verifies invocation association or node identity. dronecan preserves stateless NodeStatus/RestartNode observations.
func WithCANDecodeAs(captureInterface int, protocol string) CaptureOption {
	return func(c *CaptureConfig) error {
		if captureInterface < 0 || (protocol != "j1939" && protocol != "j1939-tp" && protocol != "dronecan" && protocol != "dronecan-stats" && protocol != "dronecan-services") {
			return fmt.Errorf("CAN DecodeAs requires a nonnegative capture interface and a supported j1939/j1939-tp/dronecan/dronecan-stats/dronecan-services profile")
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
	{"rtps-control", nil, rtpsDatagramHeader, decodeRTPSControl},
	{"rtps-wire", nil, rtpsDatagramHeader, decodeRTPSWire},
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
	if explicit == "bacnet-read-property" {
		return a.decodeBACnetReadDatagram(e, w)
	}
	if explicit == "bacnet-routed-read-property" {
		return a.decodeBACnetReadDatagram(e, w, true)
	}
	if explicit == "rtps20-rti-filter" {
		return a.decodeRTPS20RTIFilterInput(e, w, nil)
	}
	if explicit == "rtps13-parameters" {
		return a.decodeRTPS13Input(e, w, nil, true)
	}
	if explicit == "rtps13-wire" {
		return a.decodeRTPS13WireDatagram(e, w)
	}
	if explicit == "rtps-shape-fragments" {
		return a.decodeRTPSShapeFragmentDatagram(e, w, nil, src, dst)
	}
	if explicit == "rtps-spdp-fragments" {
		return a.decodeRTPSFragmentDatagram(e, w, nil)
	}
	if explicit == "nmea-ais-multipart" {
		return a.decodeAISMultipartDatagram(e, w, nil)
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
	if explicit == "c37118" || explicit == "c37118-engineering" || explicit == "" && c37118DatagramEvidence(w) {
		return a.decodeC37118Datagram(e, w, explicit != "", explicit == "c37118-engineering")
	}
	if explicit == "opendroneid-auth-pages" {
		return a.decodeOpenDroneIDProfileDatagram(e, w, odidAuthPages)
	}
	if explicit == "opendroneid-identity" {
		return a.decodeOpenDroneIDProfileDatagram(e, w, odidIdentity)
	}
	if explicit == "opendroneid-system" {
		return a.decodeOpenDroneIDSelectedDatagram(e, w, true)
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
		if (p.name == "rtps-control" || p.name == "rtps-wire") && explicit != p.name {
			continue
		}
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
		if p.name == "rtps-control" || p.name == "rtps-wire" {
			e.Protocol = "rtps"
		}
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
		} else if p.name == "bittorrent-dht" {
			fields, err = decodeDHTMessageWithBudget(w, a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth)
		} else if p.name == "rtps-wire" {
			fields, err = a.decodeRTPSWireSelected(w, src, dst)
		} else if p.name == "rtps-control" {
			fields, err = decodeRTPSControlBudget(w, a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth)
		} else if p.name == "nmea" {
			fields, err = decodeNMEADatagramBudget(w, a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth)
		} else if p.name == "rtps" && rtpsDatagramHeader(w) && w[5] == 0 {
			fields, err = decodeRTPS20DatagramBudget(w, a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth)
		} else {
			fields, err = p.decode(w, a.budget.MaxCollectionElements)
		}
		if p.name == "bittorrent-dht" && err == nil {
			dhtObservePeerPort(e, fields)
		}
		if p.name == "dns" || p.name == "mdns" || p.name == "llmnr" || p.name == "dhcp" || p.name == "dhcpv6" {
			// The canonical codec also has ordinary structural errors (e.g.
			// a truncated QCLASS or missing DHCP END). A UDP datagram cannot acquire more
			// bytes later; retain the refusal for deferred/native field queries.
			if err != nil {
				var typed *ProtocolError
				if !errors.As(err, &typed) {
					err = protocolError(ErrMalformedMessage, "%s", err)
				}
			}
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
		if p.name == "dns" || p.name == "mdns" || p.name == "llmnr" {
			// DNS association context is distinct from the canonical message
			// fields; both query projections need their own private snapshot.
			e.semanticSession = cloneSession(e.Session)
		}
		if p.name != "rtps" && p.name != "rtps-control" && p.name != "rtps-wire" || err == nil {
			projection := e.semanticFields
			if e.ownsNativeFields() {
				// fields is already the caller-owned Session projection. Keep
				// the one private clone separate without allocating another tree.
				projection = fields
			}
			e.Structured = map[string]any{"fields": projection}
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
