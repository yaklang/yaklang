package pcaputil

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	binparser "github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// ProtocolEvent owns its bytes and structured values. It never retains a
// pooled TrafficFlow/TrafficConnection. WithOnProtocolMessage serializes delivery;
// legacy WithBinParser callbacks may run concurrently for different flows.
// Messages in one TCP flow are delivered in order.
type ProtocolEvent struct {
	nativeSessionMessage               bool
	nativeNetFlowObservation           bool
	rtmpChildren                       []*ProtocolEvent
	nativeCarrierObservation           bool
	nativeOPCUAObservation             bool
	nativeMMSObservation               bool
	nativeFINSObservation              bool
	nativeENIPObservation              bool
	nativeHARTIPObservation            bool
	nativeTLSObservation               bool
	nativeDTLSObservation              bool
	nativeRADIUSObservation            bool
	radiusApplicationRefusal           bool
	nativeSNMPObservation              bool
	nativeSTUNObservation              bool
	nativeSIPObservation               bool
	nativeTFTPObservation              bool
	nativeATGObservation               bool
	nativeDHCPObservation              bool
	nativeDNSObservation               bool
	nativeDNSStreamObservation         bool
	nativeDLMSOrSLMPObservation        bool
	nativeUnrecognizedUDP              bool
	nativeBACnetRead                   bool
	nativeBACnetRoutedRead             bool
	Admission                          string
	nativeDiagnosticError              string // immutable query refusal; public caches are not authority
	nativeFieldsIncomplete             bool   // explicit partial-field scope survives status normalization/public edits
	semanticFields                     map[string]any
	semanticSession                    map[string]any
	semanticMetadata                   any
	ownedRuleEnvelope, hasRuleMetadata bool
	nativeQUICObservation              bool
	nativeQUICWire                     bool
	nativeDoIPObservation              bool
	doipAssociationRefusal             bool // private complete-frame refusal scope
	nativeCANRecord                    bool
	ocpp                               bool
	Domain                             CaptureDomain
	SourceBytes                        ByteSource
	// CarrierLayers lists observed protocol envelopes between the physical
	// transport and this PDU. It does not turn a UDP carrier into TCP.
	CarrierLayers                            []string
	Profile, Completeness, ExpertCode        string
	TransactionID, ResponseTo                uint64
	ID, FlowID                               uint64
	Timestamp                                time.Time
	Transport, Source, Destination, Protocol string
	Direction                                int    // 0: first observed endpoint -> peer, 1: reverse (not a role guess)
	Offset                                   uint64 // offset in the delivered, ordered direction, not an absolute TCP sequence
	Length                                   int
	Status, Summary, Rule, Entry, Error      string
	Raw                                      []byte
	Fields                                   map[string]any // owned protocol fields; Completeness identifies header-only views
	Metadata                                 any
	Structured                               map[string]any
	// Session is an owned snapshot of observed connection context. Decoded
	// headers have no fabricated byte spans and never imply TLS decryption.
	Session         map[string]any
	plan            *binparser.StructuredPlan
	decodeSkip      int
	decodeConfig    map[string]any
	syslogFraming   syslogFrameKind
	syslogBudget    int
	syslogMaxBytes  int
	historySequence uint64
	ipp             bool
	sessionError    *ProtocolError
}

// BinParserEvent is retained for source compatibility. Use ProtocolEvent.
type BinParserEvent = ProtocolEvent

// Decode parses a complete event on demand. Incomplete/unrecognized samples
// cannot be decoded as if they were messages. The returned object belongs to
// the caller; no JSON conversion takes place.
func (e *ProtocolEvent) Decode() (result map[string]any, err error) {
	defer func() {
		session := e.Session
		if e.nativeSessionMessage && e.semanticFields != nil {
			session = e.semanticFields
		}
		if e.semanticSession != nil {
			session = e.semanticSession
		}
		// For these native datagram observations, Session and fields describe
		// the same complete message. Queries must use the private snapshot,
		// including after a caller edits or replaces the public Session map.
		if e.Transport == "udp" && e.Profile == e.Protocol+"-native" && e.semanticFields != nil &&
			(e.Protocol == "dhcp" || e.Protocol == "dhcpv6" || e.Protocol == "mqtt-sn" || e.Protocol == "bittorrent-dht" || e.Protocol == "bjnp" || e.Protocol == "nmea" || e.Protocol == "rtps" || e.Protocol == "semtech-udp") {
			session = e.semanticFields
		}
		// These selected stateless observations expose the same accepted
		// message in fields and session. Reuse the private tree so edited or
		// replaced public Session maps cannot change a later query.
		ownedObservation := e.Protocol == "opendroneid" && (e.Profile == "opendroneid-v2-basic-location" || e.Profile == openDroneIDSystemProfile || e.Profile == openDroneIDIdentityProfile || e.Profile == openDroneIDAuthProfile) ||
			e.Protocol == "mavlink" && (e.Profile == "mavlink-common-three-messages" || e.Profile == "mavlink-common-gps-raw-int" || e.Profile == "mavlink-common-attitude")
		if e.Transport == "udp" && ownedObservation && e.semanticFields != nil {
			session = e.semanticFields
		}
		// KNX search is a stateless observation: the accepted private fields
		// are also its session view, without request association state.
		if e.Protocol == "knx" && e.Transport == "udp" && (e.Profile == "knx-basic-search" || e.Profile == "knx-extended-search") && e.semanticFields != nil {
			session = e.semanticFields
		}
		// Native HDLC includes its observed association and segmentation in
		// the private field tree already, on both byte-stream and UDP ingress.
		if e.Protocol == "dlms" && e.semanticFields != nil {
			session = e.semanticFields
		}
		// These native observations already retain their complete session
		// context in the private field tree. Public projections are not authority.
		ownedContext := e.nativeNetFlowObservation || e.nativeCarrierObservation || e.nativeSNMPObservation || e.nativeTLSObservation || e.nativeDTLSObservation || e.nativeHARTIPObservation || e.nativeENIPObservation || e.nativeFINSObservation || e.Protocol == "slmp" && (e.Profile == slmpTCPProfile || e.Profile == "slmp-binary-self-test") ||
			e.Transport == "ethernet" && (e.Protocol == "sv" && e.Profile == "iec61850-sv-raw-asdu" || e.Protocol == "ethercat" && e.Profile == "ethercat-datagram-chain")
		if ownedContext && e.semanticFields != nil {
			session = e.semanticFields
		}
		// TLS and Redis keep the accepted observation and its passive context
		// together. Public projections cannot rewrite subsequent queries.
		if (e.Protocol == "tls" || e.Protocol == "redis" || e.Entry == "MySQLPreparedFields") && e.semanticFields != nil {
			session = e.semanticFields
		}
		// SNMP retains the accepted wire and passive association together;
		// a public edit must not manufacture a matched request in queries.
		if e.Protocol == "snmp" && e.semanticFields != nil {
			session = e.semanticFields
		}
		// Native media fields include the final observed association context.
		// Public edits cannot manufacture a different media/session identity.
		if e.Protocol == "rtp" && e.Transport == "udp" && e.Profile == "rtp-udp" && e.semanticFields != nil {
			session = e.semanticFields
		}
		// C37.118 exposes its complete accepted observation in both views
		// on TCP and UDP. The public map is caller-owned, not query authority.
		if e.Protocol == "c37118" && (e.Profile == "c37118-v1-v2-cfg2-data" || e.Profile == "c37118-v2-cfg3-complete-data" || e.Profile == "c37118-v2-cfg3-continuation-data" || e.Profile == "c37118-engineering") && e.semanticFields != nil {
			session = e.semanticFields
		}
		if e.nativeDHCPObservation && e.semanticFields != nil {
			session = e.semanticFields
		}
		if (e.nativeDNSObservation || e.nativeDNSStreamObservation) && e.semanticSession != nil {
			session = e.semanticSession
		}
		if e.nativeATGObservation && e.semanticFields != nil {
			session = e.semanticFields
		}
		if (e.nativeDLMSOrSLMPObservation || e.nativeSTUNObservation || e.nativeSIPObservation || e.nativeTFTPObservation) && e.semanticSession != nil {
			session = e.semanticSession
		}
		// Native QUIC has distinct wire fields and passive Session context.
		// Public protocol aliases must not replace the latter with the former.
		if e.nativeQUICObservation && e.semanticSession != nil {
			session = e.semanticSession
		}
		if err == nil && result != nil && session != nil {
			owned := make(map[string]any, len(result)+1)
			for k, v := range result {
				owned[k] = v
			}
			owned["session"] = cloneSession(session)
			result = owned
		}
	}()
	// Native QUIC identity survives edits to all public labels and caches.
	// A refused packet cannot become a successful rule decode, and accepted
	// wire fields keep their legacy metadata envelope and private context.
	if e.nativeCarrierObservation || e.nativeNetFlowObservation {
		if e.semanticFields != nil {
			return map[string]any{"fields": cloneSession(e.semanticFields)}, nil
		}
		if e.sessionError != nil {
			owned := *e.sessionError
			return nil, &owned
		}
	}
	if e.nativeSessionMessage {
		if e.sessionError != nil {
			v := *e.sessionError
			return nil, &v
		}
		if e.semanticFields != nil {
			return map[string]any{"fields": cloneSession(e.semanticFields)}, nil
		}
	}
	if e.nativeQUICObservation {
		if e.sessionError != nil {
			owned := *e.sessionError
			return nil, &owned
		}
		if e.semanticFields != nil {
			return map[string]any{"fields": cloneSession(e.semanticFields), "metadata": nil}, nil
		}
		if e.nativeQUICWire {
			fields, err := quicWireEnvelope(e.Raw)
			return map[string]any{"fields": fields, "metadata": nil}, err
		}
	}
	if e.nativeDNSStreamObservation {
		if e.sessionError != nil {
			owned := *e.sessionError
			return nil, &owned
		}
		if e.semanticFields != nil {
			out := map[string]any{"fields": cloneSession(e.semanticFields)}
			if e.hasRuleMetadata {
				out["metadata"] = cloneSessionValue(e.semanticMetadata)
			}
			return out, nil
		}
	}
	// Accepted SIP wire fields and passive context have independent ownership.
	// A typed semantic refusal cannot become a successful raw grammar decode.
	if e.nativeSIPObservation {
		if e.sessionError != nil {
			owned := *e.sessionError
			return nil, &owned
		}
		if e.semanticFields != nil {
			result := map[string]any{"fields": cloneSession(e.semanticFields)}
			if e.hasRuleMetadata {
				result["metadata"] = cloneSessionValue(e.semanticMetadata)
			}
			return result, nil
		}
	}
	// Preserve the legacy RADIUS rule envelope and its independently owned
	// passive Session; UDP uses its complete canonical observation instead.
	if e.nativeRADIUSObservation {
		if e.sessionError != nil {
			owned := *e.sessionError
			return nil, &owned
		}
		if e.semanticFields != nil {
			out := map[string]any{"fields": cloneSession(e.semanticFields)}
			if e.hasRuleMetadata {
				out["metadata"] = cloneSessionValue(e.semanticMetadata)
			}
			return out, nil
		}
	}
	if e.nativeDNSObservation || e.nativeATGObservation || e.nativeTFTPObservation || e.nativeDLMSOrSLMPObservation {
		if e.semanticFields != nil {
			return map[string]any{"fields": cloneSession(e.semanticFields)}, nil
		}
		if e.sessionError != nil {
			owned := *e.sessionError
			return nil, &owned
		}
	}
	// STUN/TURN wire fields and passive context have separate private snapshots.
	if e.nativeSTUNObservation {
		if e.semanticFields != nil {
			return map[string]any{"fields": cloneSession(e.semanticFields)}, nil
		}
		if e.sessionError != nil {
			owned := *e.sessionError
			return nil, &owned
		}
	}
	// SNMP keeps partial envelope metadata queryable on contextual refusals.
	if e.nativeSNMPObservation {
		if e.semanticFields != nil {
			return map[string]any{"fields": cloneSession(e.semanticFields)}, nil
		}
		if e.sessionError != nil {
			owned := *e.sessionError
			return nil, &owned
		}
	}
	// Native TLS/DTLS identity survives edits to every public projection,
	// including a protocol name that would select a legacy cache below.
	if e.nativeTLSObservation || e.nativeDTLSObservation {
		if e.sessionError != nil {
			owned := *e.sessionError
			return nil, &owned
		}
		if e.semanticFields != nil {
			return map[string]any{"fields": cloneSession(e.semanticFields)}, nil
		}
	}
	if e.nativeDiagnosticError != "" {
		return nil, fmt.Errorf("%s", e.nativeDiagnosticError)
	}
	if e.nativeUnrecognizedUDP {
		return nil, fmt.Errorf("protocol parser: event is unrecognized, not an exact message")
	}
	// Native contextual queries preserve the accepted observation or its refusal;
	// the public Session may be edited or removed by the callback consumer.
	if (e.Protocol == "tls" || e.Protocol == "redis" || e.Protocol == "mysql" || e.Protocol == "mqtt") && e.sessionError != nil {
		owned := *e.sessionError
		return nil, &owned
	}
	if (e.Protocol == "tls" || e.Protocol == "redis" || e.Entry == "MySQLPreparedFields") && e.semanticFields != nil {
		return map[string]any{"fields": cloneSession(e.semanticFields)}, nil
	}
	if (e.Protocol == "tls" || e.Protocol == "redis" || e.Entry == "MySQLPreparedFields") && e.Session != nil {
		return map[string]any{"fields": cloneSession(e.Session)}, nil
	}
	// SNMP exposes independently observed envelope metadata even when the
	// scoped PDU is encrypted or association state was refused. Preserve that
	// established field-query contract; Status/Error still report the refusal.
	if e.Protocol == "snmp" && e.semanticFields != nil {
		return map[string]any{"fields": cloneSession(e.semanticFields)}, nil
	}
	if e.ownsNativeFields() && e.sessionError != nil {
		owned := *e.sessionError
		return nil, &owned
	}
	// These native caches are separate from the public event projection. A
	// caller may edit a returned tree without changing later field queries.
	if e.ownsNativeFields() && e.semanticFields != nil {
		if e.nativeQUICWire {
			return map[string]any{"fields": cloneSession(e.semanticFields), "metadata": nil}, nil
		}
		return map[string]any{"fields": cloneSession(e.semanticFields)}, nil
	}
	if e.Protocol == "stun" || e.Protocol == "turn" || e.Protocol == "tftp" {
		if e.semanticFields != nil {
			return map[string]any{"fields": cloneSession(e.semanticFields)}, nil
		}
		if e.sessionError != nil {
			owned := *e.sessionError
			return nil, &owned
		}
	}
	if e.ownedRuleEnvelope {
		result := map[string]any{"fields": cloneSession(e.semanticFields)}
		if e.hasRuleMetadata {
			result["metadata"] = cloneSessionValue(e.semanticMetadata)
		}
		return result, nil
	}
	if e.Structured != nil {
		return e.Structured, nil
	}
	if e.semanticFields != nil {
		return map[string]any{"fields": cloneSession(e.semanticFields)}, nil
	}
	if e.Status != "deferred" && e.Status != "decoded" && e.Status != "malformed" {
		return nil, fmt.Errorf("protocol parser: event is %s, not an exact message", e.Status)
	}
	if e.nativeQUICWire {
		fields, decodeErr := quicWireEnvelope(e.Raw)
		return map[string]any{"fields": fields, "metadata": nil}, decodeErr
	}
	if e.syslogFraming != "" {
		fields, decodeErr := (&binSyslog{}).consume(e.Raw, e.syslogFraming, e.syslogMaxBytes, e.syslogBudget)
		return map[string]any{"fields": fields}, decodeErr
	}
	if e.Entry == "LDAPProtectedRecord" {
		return map[string]any{"fields": map[string]any{"Security Layer": "SASL protected record", "Payload": "opaque"}}, nil
	}
	if e.Rule == "" {
		return nil, fmt.Errorf("protocol parser: no exact rule for this message")
	}
	if e.decodeSkip > len(e.Raw) {
		return nil, fmt.Errorf("protocol parser: incomplete transport prefix")
	}
	wire := e.Raw[e.decodeSkip:]
	if e.decodeConfig != nil {
		return binparser.ParseStructuredWithConfig(wire, e.Rule, e.decodeConfig, e.Entry)
	}
	if e.plan != nil {
		return e.plan.Parse(wire)
	}
	if e.Entry == "" {
		return binparser.ParseStructured(wire, e.Rule)
	}
	return binparser.ParseStructured(wire, e.Rule, e.Entry)
}

// GetFields returns the protocol field tree directly, also for deferred events.
// Decode remains the compatibility API returning the fields/metadata envelope.
func (e *ProtocolEvent) GetFields() (map[string]any, error) {
	if e.Fields != nil && !e.ownedRuleEnvelope && !e.ownsNativeFields() && e.Protocol != "stun" && e.Protocol != "turn" && e.Protocol != "tftp" {
		return e.Fields, nil
	}
	result, err := e.Decode()
	if err != nil {
		return nil, err
	}
	return protocolFields(result), nil
}

// Retain accepted legacy rule fields separately from public projections. Rule
// metadata is independent of observed session context; preserve its key even
// when the accepted parser deliberately returns a nil metadata value.
func (e *ProtocolEvent) retainRuleEnvelope(result map[string]any) {
	e.semanticFields = cloneSession(protocolFields(result))
	metadata, present := result["metadata"]
	e.semanticMetadata = cloneSessionValue(metadata)
	e.ownedRuleEnvelope, e.hasRuleMetadata = true, present
}

func (e *ProtocolEvent) ownsNativeFields() bool {
	if e.nativeSessionMessage {
		return true
	}
	if e.nativeCarrierObservation {
		return true
	}
	if e.nativeQUICObservation {
		return true
	}
	if e.nativeRADIUSObservation || e.nativeDHCPObservation || e.nativeDNSStreamObservation || e.nativeDNSObservation || e.nativeATGObservation || e.nativeDLMSOrSLMPObservation || e.nativeTFTPObservation || e.nativeSIPObservation || e.nativeSTUNObservation || e.nativeSNMPObservation || e.nativeTLSObservation || e.nativeDTLSObservation || e.nativeHARTIPObservation || e.nativeENIPObservation || e.nativeFINSObservation || e.nativeDiagnosticError != "" {
		return true
	}
	if e.nativeDoIPObservation {
		return true
	}
	if e.nativeBACnetRead {
		return true
	}
	if e.nativeUnrecognizedUDP {
		return true
	}
	// Preserve accepted IEC104/DNP3 typed refusals before any mutable public cache.
	if (e.Protocol == "iec104" || e.Protocol == "dnp3") && e.sessionError != nil {
		return true
	}
	if e.nativeMMSObservation {
		return true
	}
	if e.nativeOPCUAObservation {
		return true
	}
	if e.nativeCANRecord {
		return true
	}
	if e.ocpp && e.sessionError != nil {
		return true
	}
	if e.Protocol == "mqtt" && e.sessionError != nil {
		return true
	}
	// A MySQL framing refusal may have no prepared entry at all. Its typed
	// private error remains authoritative instead of public cache/status edits.
	if e.Protocol == "mysql" && e.sessionError != nil {
		return true
	}
	if (e.Protocol == "tls" || e.Protocol == "redis" || e.Entry == "MySQLPreparedFields") && (e.semanticFields != nil || e.sessionError != nil) {
		return true
	}
	// The selected UDP media decoder owns its field tree and typed refusals;
	// legacy stream RTP keeps its existing rule envelope and metadata shape.
	if e.Protocol == "rtp" && e.Transport == "udp" && e.Profile == "rtp-udp" && (e.semanticFields != nil || e.sessionError != nil) {
		return true
	}
	// A datagram refused before admission has no field tree. Keep its private
	// resource refusal authoritative even if callers populate public caches.
	if e.Transport == "udp" && e.Protocol == "" && e.sessionError != nil {
		return true
	}
	// Native network observations own query trees just like native datagrams.
	// A caller editing public Fields, Structured or Session must not rewrite
	// the captured message used by later queries.
	// Preserve caller-provided field caches on manually constructed events.
	if e.Transport == "network" && (e.semanticFields != nil || e.sessionError != nil) && (e.Protocol == "arp" || e.Protocol == "icmp" || e.Protocol == "icmpv6") {
		return true
	}
	// UDP DNS-family projections use the canonical native codec; TCP DNS
	// keeps its length-prefixed rule envelope. Do not expose the native cache
	// through mutable Fields/Structured aliases or hide its parse refusal.
	if e.Transport == "udp" && e.Profile == e.Protocol+"-native" && (e.Protocol == "dns" || e.Protocol == "mdns" || e.Protocol == "llmnr") {
		return true
	}
	// DHCP's native UDP projection is a bounded observation, separate from
	// the legacy stream/rule envelope. Its queries own both fields and errors.
	if e.Transport == "udp" && (e.Protocol == "dhcp" || e.Protocol == "dhcpv6") && e.Profile == e.Protocol+"-native" {
		return true
	}
	if e.Transport == "udp" && (e.Protocol == "mqtt-sn" || e.Protocol == "bittorrent-dht" || e.Protocol == "bjnp") && e.Profile == e.Protocol+"-native" {
		return true
	}
	if e.Profile == "quic-v1-native" || e.Profile == "http3-native" || e.Profile == "doq-native" {
		return true
	}
	if e.Protocol == "semtech-udp" && (e.Profile == semtechDownlinkProfile || e.Profile == semtechCorrelationProfile) {
		return true
	}
	if e.Protocol == "profinet" && e.Profile == "profinet-carrier-diagnostic" {
		return true
	}
	if e.Protocol == "slmp" && (e.Profile == slmpTCPProfile || e.Profile == "slmp-binary-self-test") || e.Protocol == "doip" && e.Profile == doipDiscoveryProfile {
		return true
	}
	switch e.Protocol {
	case "fins", "someip", "snmp", "dlms-wrapper", "dlms", "opendroneid", "c37118", "sv", "ethercat", "knx", "pfcp", "profinet-dcp", "mavlink", "rtps", "nmea", "semtech-udp", "iso8583":
		return true
	}
	return false
}

func protocolFields(result map[string]any) map[string]any {
	if fields, ok := result["fields"].(map[string]any); ok {
		return fields
	}
	return result
}

func (e *ProtocolEvent) setStructured(result map[string]any) {
	e.Structured = result
	e.Fields, e.Metadata = protocolFields(result), result["metadata"]
}

// BinParserBinding admits an explicit protocol profile. Frame returns the
// complete wire length, 0 for more bytes, or an error. It must validate lengths
// before allocating and must not retain the borrowed input. A port is only a
// routing hint: Probe must confirm the wire. Profiles requiring negotiated
// capabilities/direction belong in a stateful upstream resolver, not a port
// guess. Bindings are tested once per direction, then cached on that flow.
type BinParserBinding struct {
	Protocol, Rule, Entry string
	Port                  uint16 // 0: any port
	Probe                 func([]byte) bool
	Frame                 func([]byte) (int, error)
}

type BinParserConfig struct {
	TFTPFilePort       uint16
	TFTPFileBytesLimit int
	// DecodeDNP3ReadPoints opts into bounded class READ/static point observations.
	DecodeDNP3ReadPoints bool
	// DecodeDNP3TransportFragments enables bounded TCP DNP3 transport assembly.
	DecodeDNP3TransportFragments bool
	// DecodeDNP3DirectStaticReads adds selected G30V0 all/range READ context.
	DecodeDNP3DirectStaticReads bool
	// DecodeDNP3TypedStaticReads adds selected static variants and ordered coverage.
	DecodeDNP3TypedStaticReads bool
	// DecodeDNP3CounterPoints adds selected unsigned static/frozen counter observations.
	DecodeDNP3CounterPoints bool
	// DecodeDNP3CounterEvents additionally selects bounded indexed counter events.
	DecodeDNP3CounterEvents bool
	// DecodeDNP3EventCountReads also selects bounded direct event count READs.
	DecodeDNP3EventCountReads bool
	// DecodeDNP3StaticStatusPoints selects packed/double-bit and output status points.
	DecodeDNP3StaticStatusPoints bool
	// DecodeDNP3BinaryEvents selects indexed binary events and message-local CTO.
	DecodeDNP3BinaryEvents bool
	// DecodeDNP3AnalogEvents selects indexed G32/G42 analog events.
	DecodeDNP3AnalogEvents bool
	// MMSISOPorts explicitly selects bounded RFC1006/COTP/presentation/MMS observations.
	MMSISOPorts             []uint16
	DecodeMMSServiceValues  bool
	ValidateMMSDescriptions bool
	// DecodeOPCUARead opts into bounded UA Binary Read scalar observations under an observed None policy.
	DecodeOPCUARead bool
	// DecodeOPCUABrowse opts into bounded Browse body observations under an observed None policy.
	DecodeOPCUABrowse bool
	// DecodeOPCUAWrite opts into bounded scalar Write body observations under an observed None policy.
	DecodeOPCUAWrite bool
	// DecodeOPCUABrowseNext opts into bounded continuation message observations, without certifying token identity.
	DecodeOPCUABrowseNext bool
	// DecodeOPCUACall opts into bounded scalar method argument/result observations, without certifying invocation success.
	DecodeOPCUACall bool
	// DecodeOPCUAVariantArrays extends selected Read/Write/Call observations with bounded arrays of supported scalar types.
	DecodeOPCUAVariantArrays bool
	// DecodeOPCUAStructuredValues enables bounded recursive values and full diagnostics; encoded ExtensionObject bodies remain opaque.
	DecodeOPCUAStructuredValues                   bool
	OnEvent                                       func(*BinParserEvent)
	OnStats                                       func(BinParserStats)
	Deferred                                      bool // explicit capture-first mode; full Decode() is done by the viewer
	MaxMessageBytes, MaxBufferedBytes, ProbeBytes int
	Bindings                                      []BinParserBinding
}

// ProtocolStats separates recognized message/diagnostic accounting from
// unrecognized datagrams, which increment Unknown rather than Messages.
type ProtocolStats struct {
	ContextRequired                                                                uint64
	Flows, ProbeCalls, Messages, Decoded, Deferred, Malformed, Incomplete, Unknown uint64
	InputBytes, MessageBytes, UnclassifiedBytes, LimitedBytes, CallbackPanics      uint64
	BufferedBytes, PeakBufferedBytes                                               int64
}

// BinParserStats is retained for source compatibility. Use ProtocolStats.
type BinParserStats = ProtocolStats

// WithBinParser enables full structured parsing after TCP framing and for
// supported UDP datagrams. It reuses pcapx's flow workers, applies bounded
// buffering, and disables unbounded legacy stream retention. Slow callbacks
// backpressure the capture; callers should use a bounded inspector for viewing.
func WithBinParser(callback func(*BinParserEvent)) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.OnEvent = callback
		return nil
	}
}

func WithBinParserConfig(config BinParserConfig) CaptureOption {
	return func(c *CaptureConfig) error {
		copyConfig := config
		copyConfig.Bindings = append([]BinParserBinding(nil), config.Bindings...)
		copyConfig.MMSISOPorts = append([]uint16(nil), config.MMSISOPorts...)
		c.binParserConfig = &copyConfig
		return nil
	}
}

// These options compose with WithBinParser in either order.
func WithBinParserDeferred(deferred bool) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.Deferred = deferred
		return nil
	}
}

func WithBinParserStats(callback func(BinParserStats)) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.OnStats = callback
		return nil
	}
}

type binSpec struct {
	rule, entry string
	plan        *binparser.StructuredPlan
}
type binParser struct {
	l2tp3Mu                                                                    sync.Mutex
	l2tp3                                                                      *l2tp3Store
	c37118Engineering                                                          bool
	mavlinkSigning                                                             *mavlinkSigningState
	ftpMu                                                                      sync.Mutex
	ftpClock                                                                   time.Time
	ftpEndpoints                                                               map[ftpDataKey]ftpDataBinding
	pfcpSessionIdentity                                                        *pfcpSessionIdentity
	semtechDisabled                                                            bool                                       // udpMu; bounded opt-in correlation fails closed after global state exhaustion
	semtechSessions                                                            *binUDPStore                               // udpMu; token quarantine must survive unrelated UDP expiry
	bacnetReadDisabled                                                         bool                                       // udpMu; lost invoke history cannot establish a new generation
	bacnetRoutedReadDisabled                                                   bool                                       // udpMu; unretained route identity loss
	bacnetReadSessions                                                         *binUDPStore                               // udpMu; bounded capture-lifetime invoke quarantine
	bacnetInvokeOrigins                                                        map[bacnetInvokeOrigin]bacnetInvokeHistory // udpMu; shared originating logical device namespace
	aisMultipart                                                               *aisMultipartStore                         // udpMu; directed, capture-domain fragment history
	someIPTP                                                                   *someIPStore                               // udpMu; directed bounded passive transfer history
	rtpsFragments                                                              *rtpsFragmentStore                         // udpMu; directed domain/sample history
	rtpsShapeFragments                                                         *rtpsFragmentStore                         // udpMu; separately selected application sample history
	aisMultipartDisabled                                                       bool                                       // udpMu; untracked history fails closed
	datagramDecodeAs                                                           map[uint16]string
	rtpsShapeWriters                                                           map[uint16]RTPSShapeWriter
	canISOTransports                                                           map[int][]CANISOTransport
	canISOMu                                                                   sync.Mutex
	canISO                                                                     *canISOStore
	canISODisabled                                                             bool
	canDecodeAs                                                                map[int]string
	j1939TPMu                                                                  sync.Mutex
	j1939TP                                                                    *j1939TPStore
	j1939TPRefusedDomains                                                      map[CaptureDomain]int64
	j1939TPDisabled                                                            bool
	droneCANMu                                                                 sync.Mutex
	droneCANStats                                                              *droneCANStatsStore
	droneCANStatsDisabled                                                      bool // refused untracked transfer history fails closed
	fragments                                                                  fragmentStore
	dnsMu                                                                      sync.Mutex
	dns                                                                        dnsObservationCorrelation
	lan                                                                        dnsCorrelation
	tlsSecrets                                                                 TLSSecretProvider
	udpMu                                                                      sync.Mutex
	carrierMu                                                                  sync.Mutex
	rdpUDP                                                                     *rdpUDPStore
	eapTLS                                                                     *eapTLSStore
	udpSessions                                                                *binUDPStore
	netflowV9                                                                  *netflowV9Store // udpMu; directed exporter/domain template observations
	tftpUntrackedRefusal                                                       bool            // udpMu; unknown refused client/server identity
	sipUntrackedRefusal                                                        bool            // udpMu; uncertainty for identities that could not be retained
	stunUntrackedRefusal                                                       bool            // constant-size quarantine when an admitted UDP key cannot be retained
	pfcpUntrackedRefusal                                                       bool            // udpMu; forgotten rejected identity cannot verify a newly admitted PFCP key
	dlmsUntrackedRefusal                                                       bool            // udpMu; no fresh HDLC association after an unretained admitted refusal
	hartUntrackedRefusal                                                       bool            // udpMu; unidentified refusals cannot start a fresh association
	radiusUntrackedRefusal                                                     bool            // udpMu; unretained RADIUS history
	snmpUntrackedRefusal                                                       bool            // udpMu; only newly untrackable SNMP conversations are conservative
	finsUntrackedRefusal                                                       bool            // udpMu; unretained identity cannot become a fresh generation
	slmpUntrackedRefusal                                                       bool            // udpMu; refused unretained SLMP identity has no verified generation
	wrapperUntrackedRefusal                                                    bool            // udpMu; missing Wrapper history cannot prove a fresh invoke generation
	dcpMu                                                                      sync.Mutex
	dcpClock                                                                   time.Time
	dcpObservations                                                            map[dcpKey]*dcpObservation
	mediaMu                                                                    sync.Mutex
	mediaIndex                                                                 map[mediaEndpointKey][]*sipMediaAssociation
	mediaEntries                                                               []*sipMediaAssociation
	rtspMedia                                                                  *rtspMediaRegistry
	budget                                                                     ParserBudget
	bindings                                                                   map[uint16][]*BinParserBinding
	contextRequired                                                            atomic.Uint64
	config                                                                     BinParserConfig
	specs                                                                      map[string]*binSpec
	flows, probes, messages, decoded, deferred, malformed, incomplete, unknown atomic.Uint64
	input, messageBytes, unclassified, limited, panics, ids                    atomic.Uint64
	buffered, peak                                                             atomic.Int64
	err                                                                        atomic.Pointer[binParserError]
	decryptedQUICSource                                                        string
	quicKeys                                                                   *quicKeyring
}
type binParserError struct{ err error }

func (c *CaptureConfig) prepareBinParser() error {
	if c.binParserConfig != nil && c.binParserConfig.ValidateMMSDescriptions && (!c.binParserConfig.DecodeMMSServiceValues || len(c.binParserConfig.MMSISOPorts) == 0) {
		return fmt.Errorf("MMS description validation requires explicit service/value ports")
	}
	if c.binParserConfig == nil || (c.binParserConfig.OnEvent == nil && c.binParserConfig.OnStats == nil) {
		return nil
	}
	if c.DisableAssembly || c.EnableCache || c.requiresFullStream {
		return fmt.Errorf("protocol parser requires exclusive streaming reassembly; disable capture cache and built-in full-stream HTTP/TLS helpers")
	}
	config := *c.binParserConfig
	config.MMSISOPorts = append([]uint16(nil), config.MMSISOPorts...)
	for _, port := range config.MMSISOPorts {
		if port == 0 {
			return fmt.Errorf("MMS ISO requires nonzero ports")
		}
	}
	if config.MaxMessageBytes == 0 {
		config.MaxMessageBytes = 16 << 20
	}
	if config.MaxBufferedBytes == 0 {
		config.MaxBufferedBytes = 128 << 20
	}
	if config.ProbeBytes == 0 {
		config.ProbeBytes = 64
	}
	if config.TFTPFilePort != 0 && c.datagramDecodeAs[config.TFTPFilePort] != "" {
		return fmt.Errorf("TFTP file service conflicts with explicit DecodeAs")
	}
	if config.TFTPFileBytesLimit < 0 || config.TFTPFileBytesLimit > tftpMaximumFileBytes || (config.TFTPFilePort == 0) != (config.TFTPFileBytesLimit == 0) {
		return fmt.Errorf("invalid TFTP file assembly configuration")
	}
	if config.MaxMessageBytes < 64 || config.MaxMessageBytes > 64<<20 || config.MaxBufferedBytes < config.MaxMessageBytes || config.ProbeBytes < 16 || config.ProbeBytes > config.MaxMessageBytes {
		return fmt.Errorf("invalid protocol parser limits: message=%d (64..67108864), buffered=%d (must cover message), probe=%d (16..message)", config.MaxMessageBytes, config.MaxBufferedBytes, config.ProbeBytes)
	}
	a := &binParser{config: config, specs: make(map[string]*binSpec), budget: DefaultParserBudget(), rtspMedia: &rtspMediaRegistry{index: make(map[rtspMediaTuple][]rtspMediaBinding)}}
	a.budget.MaxMessageBytes, a.budget.MaxFrameBytes = config.MaxMessageBytes, config.MaxMessageBytes
	a.budget.MaxBufferedBytes, a.budget.ProbeBytes = config.MaxBufferedBytes, config.ProbeBytes
	a.bindings = make(map[uint16][]*BinParserBinding)
	for _, s := range builtinBinSpecs {
		a.addSpec(s[0], s[1])
	}
	for _, b := range config.Bindings {
		if b.Protocol == "" || b.Rule == "" || b.Probe == nil || b.Frame == nil {
			return fmt.Errorf("protocol binding requires protocol, rule, probe and frame")
		}
		a.addSpec(b.Rule, b.Entry)
	}
	for i := range a.config.Bindings {
		b := &a.config.Bindings[i]
		a.bindings[b.Port] = append(a.bindings[b.Port], b)
	}
	a.tlsSecrets = c.tlsSecrets
	a.c37118Engineering = c.c37118Engineering
	if c.mavlinkSigning != nil {
		a.mavlinkSigning = &mavlinkSigningState{options: *c.mavlinkSigning}
	}
	a.datagramDecodeAs = make(map[uint16]string, len(c.datagramDecodeAs))
	for k, v := range c.datagramDecodeAs {
		a.datagramDecodeAs[k] = v
	}
	a.rtpsShapeWriters = make(map[uint16]RTPSShapeWriter, len(c.rtpsShapeWriters))
	for port, binding := range c.rtpsShapeWriters {
		a.rtpsShapeWriters[port] = binding
	}
	a.canISOTransports = make(map[int][]CANISOTransport, len(c.canISOTransports))
	for k, v := range c.canISOTransports {
		a.canISOTransports[k] = append([]CANISOTransport(nil), v...)
	}
	a.canDecodeAs = make(map[int]string, len(c.canDecodeAs))
	for k, v := range c.canDecodeAs {
		a.canDecodeAs[k] = v
	}
	c.binParser = a
	c.reassemblyOptions.Stream = true
	return nil
}

func (a *binParser) addSpec(rule, entry string) {
	key := rule + "/" + entry
	if a.specs[key] != nil {
		return
	}
	plan, _ := binparser.PrepareStructured(rule, entry)
	a.specs[key] = &binSpec{rule, entry, plan}
}

func (a *binParser) stats() BinParserStats {
	return BinParserStats{ContextRequired: a.contextRequired.Load(), Flows: a.flows.Load(), ProbeCalls: a.probes.Load(), Messages: a.messages.Load(), Decoded: a.decoded.Load(), Deferred: a.deferred.Load(), Malformed: a.malformed.Load(), Incomplete: a.incomplete.Load(), Unknown: a.unknown.Load(), InputBytes: a.input.Load(), MessageBytes: a.messageBytes.Load(), UnclassifiedBytes: a.unclassified.Load(), LimitedBytes: a.limited.Load(), CallbackPanics: a.panics.Load(), BufferedBytes: a.buffered.Load(), PeakBufferedBytes: a.peak.Load()}
}

func (c *CaptureConfig) finishBinParser() error {
	if c.binParser == nil {
		return nil
	}
	a := c.binParser
	a.closeCANISO()
	a.closeJ1939TP()
	a.closeDroneCANStats()
	a.closeUDPSessions()
	a.closeFTPEndpoints()
	a.closeDCPObservations()
	a.closeMediaAssociations()
	a.closeRTSPMedia()
	a.closeFragments()
	if a.config.OnStats != nil {
		a.config.OnStats(a.stats())
	}
	if e := a.err.Load(); e != nil {
		return e.err
	}
	return nil
}

func (a *binParser) emit(e *BinParserEvent) {
	if e == nil {
		return
	}
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	if e.Completeness == "limited" || e.Completeness == "evicted" {
		e.nativeFieldsIncomplete = true
	}
	// Capture diagnostics cannot become parsed messages through public cache,
	// protocol or status edits. Manually constructed events retain legacy caches.
	if e.Transport == "udp" && e.Protocol == "" && e.Status == "unrecognized" {
		e.nativeUnrecognizedUDP = true
	}

	// All emitted IP diagnostics, including incomplete fragment Close, keep
	// the original field-query refusal independent of editable public caches.
	if e.Transport == "network" && e.Protocol == "ip" && e.Error != "" && e.semanticFields == nil {
		e.nativeDiagnosticError = fmt.Sprintf("protocol parser: event is %s, not an exact message", e.Status)
		if e.Status == "malformed" || e.Status == "decoded" || e.Status == "deferred" {
			e.nativeDiagnosticError = "protocol parser: no exact rule for this message"
		}
	}
	if e.Protocol == "sip" {
		a.observeSIPMedia(e)
	}
	if e.Protocol == "rtsp" {
		a.observeRTSPMedia(e)
	}
	if a.config.OnEvent == nil {
		return
	}
	e.setStructured(e.Structured)
	e.finalizeEvidence()
	e.summarizeProtocol()
	// IEC104, DNP3, GOOSE, MQTT and legacy stream media keep rule fields distinct from observed
	// session context. Freeze the final context before handing out public maps.
	if ((e.Protocol == "iec104" || e.Protocol == "dnp3") || e.Protocol == "goose" || e.Protocol == "mqtt" || e.ocpp || e.Protocol == "rtp" && e.Transport != "udp") && e.semanticSession == nil && e.Session != nil {
		e.semanticSession = cloneSession(e.Session)
	}
	if (e.nativeDNSStreamObservation || e.nativeDLMSOrSLMPObservation || e.nativeSTUNObservation || e.nativeSIPObservation || e.nativeTFTPObservation) && e.semanticSession == nil && e.Session != nil {
		e.semanticSession = cloneSession(e.Session)
	}
	defer func() {
		if p := recover(); p != nil {
			a.panics.Add(1)
			a.err.CompareAndSwap(nil, &binParserError{fmt.Errorf("protocol callback panic: %v", p)})
		}
	}()
	a.config.OnEvent(e)
}

type binDirection struct {
	tlsPlaintextGap bool
	contributions   []contribution
	evidenceLimited bool
	headerScan      int
	http            *binHTTPState
	buffer          []byte
	offset          uint64
	ts              time.Time
	stopped         bool
	// RFC4253 section 7.3 switches SSH keys independently in each direction.
	// Only ErrEncrypted after observed NEWKEYS preserves the peer's plaintext
	// phase; malformed data, resource exhaustion and panics still invalidate it.
	sshOpaque bool
}
type binFlow struct {
	ajp               *binAJP
	bgp               *binBGP
	ldp               *binLDP
	rtmp              *binRTMP
	routing           *routingDatagramState
	mms               *binMMS
	plaintextGap      [2]bool
	domain            CaptureDomain
	tls               *binTLS
	dtls              *binDTLS
	byteSource        string
	captureCarriers   []string
	parentID          uint64
	captureTCP        bool
	orderedTLSCarrier bool // separately validated reliable UDP carrier; never a TCP identity
	tcpClientKnown    bool
	tcpClientDir      int
	syslogStreamProbe bool
	lastSessionError  *ProtocolError
	sessionBytes      int64
	h2                *binHTTP2
	mysql             *binMySQL
	pg                *binPostgres
	ws                *binWebSocket
	ldap              *binLDAP
	redis             *binRedis
	mqtt              *binMQTT
	nats              *binNATS
	mongo             *binMongo
	kafka             *binKafka
	tds               *binTDS
	amqp              *binAMQP
	smb2              *binSMB2
	dcerpc            *binDCERPC
	ssh               *binSSH
	nbss              bool
	control           *controlDatagramState
	nfs               *binNFS
	snmp              *binSNMP
	rdp               *binRDP
	dot               *binDoT
	doh               *binDoH
	sip               *binSIP
	rtp               *binRTP
	quic              *binQUIC
	smtp              *binSMTP
	imap              *binIMAP
	pop3              *binPOP3
	ftp               *binFTP
	ftpData           *ftpDataBinding
	tns               *binTNS
	socks5            *binSOCKS5
	scgi              *binSCGI
	msgpackRPC        *binMsgpackRPC
	textInternet      *binTextInternet
	radius            *binRADIUS
	dhcp              *binDHCP
	ntp               *binNTP
	coap              *binCoAP
	modbus            *binModbus
	enip              *binENIP
	hart              *binHARTIP
	atg               *binATG
	doip              *binDoIP
	genisys           *binGenisys
	rocplus           *binROCPlus
	bsap              *binBSAP
	pfcp              *binPFCP
	semtech           *binSemtechDownlink
	bacnetRead        *binBACnetRead
	slmp              *binSLMP
	slmpTCP           *binSLMPTCP
	fins              *binFINS
	wrapper           *binDLMSWrapper
	dlms              *binDLMS
	memcached         *binMemcached
	iso8583           *iso8583Session
	stratum           *binStratum
	gearman           *binGearman
	beanstalk         *binBeanstalk
	zookeeper         *binZooKeeper
	clickhouse        *binClickHouse
	stomp             *binSTOMP
	rfb               *binRFB
	diameter          *binDiameter
	iec104            *binIEC104
	s7                *binS7
	opcua             *binOPCUA
	ipp               *binIPP
	rtsp              *binRTSP
	stun              *binSTUN
	tftp              *binTFTP
	dnp3              *binDNP3
	c37118            *binC37118
	goose             *binGOOSE
	syslog            *binSyslog
	httpUpgrades      []bool
	httpUpgradeIssues []string
	httpDoH           []bool
	httpIPP           []bool
	httpIDs           []uint64
	httpTimes         []time.Time
	httpWSKeys        []string
	httpWSExtensions  []string
	httpWSProtocols   []string
	httpMethods       []string
	a                 *binParser
	transportFlow     *TrafficFlow // serialized native capture context; nil for explicit sessions
	id                uint64
	endpoints         [2]string
	ports             [2]uint16
	protocol          string
	level             byte
	binding           *BinParserBinding
	directions        [2]binDirection
}

func (a *binParser) newFlow(t *TrafficFlow) *binFlow {
	return &binFlow{domain: t.key.domain, a: a, transportFlow: t, captureTCP: true, tcpClientKnown: !t.IsHalfOpen, id: a.flows.Add(1), endpoints: [2]string{t.ClientConn.LocalAddr().String(), t.ServerConn.LocalAddr().String()}, ports: [2]uint16{uint16(t.ClientConn.LocalPort()), uint16(t.ServerConn.LocalPort())}}
}

func (f *binFlow) event(dir int, raw []byte, status, detail string) *BinParserEvent {
	d := &f.directions[dir]
	e := &BinParserEvent{FlowID: f.id, Timestamp: d.ts, Transport: "tcp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Offset: d.offset, Length: len(raw), Protocol: f.protocol, Status: status, Summary: detail, Raw: append([]byte(nil), raw...)}
	if f.protocol == "dnp3" && f.a.config.DecodeDNP3ReadPoints {
		e.Profile = "dnp3-read-points"
	}
	if f.protocol == "dnp3" && f.a.config.DecodeDNP3TransportFragments {
		e.Profile = "dnp3-transport"
		if f.a.config.DecodeDNP3ReadPoints {
			e.Profile = "dnp3-read-points-transport"
		}
	}
	if f.protocol == "dnp3" && f.a.config.DecodeDNP3DirectStaticReads {
		e.Profile = "dnp3-direct-static-read"
		if f.a.config.DecodeDNP3TransportFragments {
			e.Profile += "-transport"
		}
	}
	if f.protocol == "dnp3" && f.a.config.DecodeDNP3TypedStaticReads {
		e.Profile = "dnp3-typed-static-read"
		if f.a.config.DecodeDNP3TransportFragments {
			e.Profile += "-transport"
		}
	}
	if f.protocol == "dnp3" && f.a.config.DecodeDNP3CounterPoints {
		e.Profile = "dnp3-counter-points"
		if f.a.config.DecodeDNP3TransportFragments {
			e.Profile += "-transport"
		}
	}
	if f.protocol == "dnp3" && f.a.config.DecodeDNP3CounterEvents {
		e.Profile = "dnp3-counter-events"
		if f.a.config.DecodeDNP3TransportFragments {
			e.Profile += "-transport"
		}
	}
	if f.protocol == "dnp3" && f.a.config.DecodeDNP3EventCountReads {
		e.Profile = "dnp3-event-count-read"
		if f.a.config.DecodeDNP3TransportFragments {
			e.Profile += "-transport"
		}
	}
	if f.protocol == "dnp3" && f.a.config.DecodeDNP3StaticStatusPoints {
		e.Profile = "dnp3-static-status"
		if f.a.config.DecodeDNP3TransportFragments {
			e.Profile += "-transport"
		}
	}
	if f.protocol == "dnp3" && f.a.config.DecodeDNP3BinaryEvents {
		e.Profile = "dnp3-binary-events"
		if f.a.config.DecodeDNP3TransportFragments {
			e.Profile += "-transport"
		}
	}
	if f.protocol == "dnp3" && f.a.config.DecodeDNP3AnalogEvents {
		e.Profile = "dnp3-analog-events"
		if f.a.config.DecodeDNP3TransportFragments {
			e.Profile += "-transport"
		}
	}
	if f.protocol == "opcua" && (f.a.config.DecodeOPCUARead || f.a.config.DecodeOPCUABrowse || f.a.config.DecodeOPCUAWrite || f.a.config.DecodeOPCUABrowseNext || f.a.config.DecodeOPCUACall) {
		e.nativeOPCUAObservation = true
		e.Profile = (&binOPCUA{read: f.a.config.DecodeOPCUARead, browse: f.a.config.DecodeOPCUABrowse, write: f.a.config.DecodeOPCUAWrite, next: f.a.config.DecodeOPCUABrowseNext, call: f.a.config.DecodeOPCUACall, arrays: f.a.config.DecodeOPCUAVariantArrays || f.a.config.DecodeOPCUAStructuredValues, structured: f.a.config.DecodeOPCUAStructuredValues}).observationProfile()
	}
	if f.mms != nil {
		e.nativeMMSObservation = true
		e.Profile = f.mms.profile()
	}
	if f.doip != nil {
		e.nativeDoIPObservation = true
		e.Profile = "doip-routing-dsc"
	}
	f.eventEvidence(dir, e)
	if f.protocol == "dns" || f.protocol == "dot" {
		e.nativeDNSStreamObservation = true
		e.Profile = "dns-tcp"
		if f.protocol == "dot" {
			e.Profile = "dns-over-tls"
		}
	}
	if f.atg != nil {
		e.nativeATGObservation = true
		e.Profile = "atg-tls450-i201"
		if len(raw) >= 5 && string(raw[1:5]) == "i202" || len(raw) == 0 && f.atg.count > 0 && f.atg.pending[0].function == '2' {
			e.Profile = "atg-tls450-i202"
		}
	}
	if f.binding == nil && (f.protocol == "slmp" || f.protocol == "dlms" || f.protocol == "dlms-wrapper") {
		e.nativeDLMSOrSLMPObservation = true
	}
	if f.protocol == "sip" && f.binding == nil {
		e.nativeSIPObservation = true
	}
	if f.protocol == "stun" && f.binding == nil {
		e.nativeSTUNObservation = true
	}
	if f.protocol == "snmp" && f.binding == nil {
		e.nativeSNMPObservation = true
	}
	if f.protocol == "tls" && f.binding == nil {
		e.nativeTLSObservation = true
	}
	if f.hart != nil {
		e.nativeHARTIPObservation = true
		e.Profile, e.Completeness = hartIPProfile, "message"
	}
	if f.enip != nil {
		e.nativeENIPObservation = true
		e.Profile, e.Completeness = "ethernet-ip-cip-explicit", "message"
	}
	if f.fins != nil {
		e.nativeFINSObservation = true
		e.Profile, e.Completeness = finsProfile, "message"
	}
	if f.slmpTCP != nil {
		e.Profile, e.Completeness = slmpTCPProfile, "message"
	}
	return e
}

func (f *binFlow) release(d *binDirection) {
	f.a.buffered.Add(-int64(cap(d.buffer)) - int64(len(d.contributions))*80)
	d.contributions = nil
	d.buffer = nil
	d.headerScan = 0
}

// Capacity, not length, is charged to the shared capture budget. Partial
// messages grow geometrically; complete messages bypass this buffer entirely.
func (f *binFlow) retain(d *binDirection, data []byte) bool {
	if len(data) > f.a.config.MaxMessageBytes {
		return false
	}
	if cap(d.buffer) < len(data) {
		capacity := max(len(data), min(2*cap(d.buffer), f.a.config.MaxMessageBytes))
		delta := int64(capacity - cap(d.buffer))
		for {
			current := f.a.buffered.Load()
			if current+delta > int64(f.a.config.MaxBufferedBytes) {
				return false
			}
			if f.a.buffered.CompareAndSwap(current, current+delta) {
				for peak := f.a.peak.Load(); current+delta > peak; peak = f.a.peak.Load() {
					if f.a.peak.CompareAndSwap(peak, current+delta) {
						break
					}
				}
				break
			}
		}
		grown := make([]byte, len(data), capacity)
		copy(grown, data)
		d.buffer = grown
	} else {
		d.buffer = d.buffer[:len(data)]
		copy(d.buffer, data)
	}
	return true
}

func (f *binFlow) stop(dir int, wire []byte, status, reason string) {
	d := &f.directions[dir]
	preview := wire[:min(len(wire), f.a.config.ProbeBytes)]
	retainedHTTPHeader := f.protocol == "http" && d.http != nil && !d.http.omitted && len(d.http.rawHeader) > 0 && d.http.headerFields != nil
	if retainedHTTPHeader {
		// Build provenance over all retained header bytes, not the shorter
		// diagnostic preview. The header may span several captured packets.
		preview = d.http.rawHeader
	}
	e := f.event(dir, preview, status, reason)
	if f.radius != nil {
		e.nativeRADIUSObservation = true
		e.Profile = f.radius.profile()
		if radiusUDPHeader(wire) {
			e.Profile = radiusProfile(wire[0])
		}
	}
	e.sessionError, f.lastSessionError = f.lastSessionError, nil
	if e.nativeRADIUSObservation || e.nativeDNSStreamObservation || e.nativeDNSObservation || e.nativeATGObservation || e.nativeDLMSOrSLMPObservation || e.nativeTFTPObservation || e.nativeSIPObservation || e.nativeSTUNObservation || e.nativeSNMPObservation || e.nativeTLSObservation || e.nativeHARTIPObservation || e.nativeFINSObservation || e.nativeENIPObservation {
		if e.sessionError == nil {
			kind := ErrMalformedMessage
			switch status {
			case "limited":
				kind = ErrResourceExceeded
			case "context-required":
				kind = ErrContextRequired
			case "incomplete":
				kind = ErrNeedMore
			}
			e.sessionError = &ProtocolError{Kind: kind, Message: reason}
		}
		e.Error = e.sessionError.Error()
		if status == "incomplete" {
			e.Completeness = "incomplete"
		}
	}
	if e.nativeOPCUAObservation && e.sessionError == nil {
		kind := ErrMalformedMessage
		switch status {
		case "limited":
			kind = ErrResourceExceeded
		case "context-required":
			kind = ErrContextRequired
		case "incomplete":
			kind = ErrNeedMore
		}
		e.sessionError = &ProtocolError{Kind: kind, Message: reason}
	}
	d.sshOpaque = f.protocol == "ssh" && e.sessionError != nil && e.sessionError.Kind == ErrEncrypted
	e.Length = len(wire)
	if f.protocol == "http" && d.http != nil {
		f.httpEvidence(dir, e)
		if d.http.omitted && !d.http.response {
			e.ID = 0
		}
		e.Session = map[string]any{"Method": d.http.method, "Declared Message Bytes": d.http.declared}
		if d.http.response && d.http.status == 101 {
			e.ExpertCode = "HTTPUpgradeUnverified"
			e.Session["Upgrade Validated"] = false
			if headers, ok := d.http.headerFields["Headers"].(map[string]any); ok {
				e.Session["Upgrade"] = cloneSessionValue(headers["Upgrade"])
			}
		}
		if retainedHTTPHeader {
			e.Length = len(e.Raw)
			e.Completeness = "headers"
			e.semanticFields = d.http.retainedHeaderFields()
			e.Structured = map[string]any{"fields": cloneSession(e.semanticFields)}
		}
	}
	if status == "limited" {
		if e.sessionError == nil {
			e.ExpertCode = string(ErrResourceExceeded)
		}
	}
	if status == "unrecognized" {
		e.ExpertCode = "ProtocolNotRecognized"
		e.Session = map[string]any{
			"Detection Window Bytes": f.a.config.ProbeBytes,
			"Observed Bytes":         len(wire),
			"Detection Stopped":      true,
		}
		e.Summary = fmt.Sprintf("%s (%s -> %s, flow %d, %d-byte prefix)", reason, e.Source, e.Destination, e.FlowID, len(preview))
		if f.captureTCP {
			e.Session["TCP Initiator Observed"] = f.tcpClientKnown
			if !f.tcpClientKnown {
				e.Summary += "; TCP initiator not observed"
			}
		}
		f.a.unknown.Add(1)
		f.a.unclassified.Add(uint64(len(wire)))
	} else if status == "limited" {
		f.a.limited.Add(uint64(len(wire)))
	} else if status == "context-required" {
		f.a.contextRequired.Add(1)
		f.a.unclassified.Add(uint64(len(wire)))
	} else {
		f.a.malformed.Add(1)
	}
	d.offset += uint64(len(wire))
	d.stopped = true
	f.release(d)
	f.a.emit(e)
}

func (f *binFlow) feed(dir int, data []byte, ts time.Time) {
	a, d := f.a, &f.directions[dir]
	defer func() {
		if p := recover(); p != nil {
			a.err.CompareAndSwap(nil, &binParserError{fmt.Errorf("protocol flow panic: %v", p)})
			a.malformed.Add(1)
			d.stopped = true
			d.sshOpaque = false
			f.release(d)
		}
		// Resource exhaustion and panic containment invalidate connection-wide
		// dictionaries/sequence state just as a malformed session message does.
		if d.stopped && f.hasSession() && !d.sshOpaque && !d.tlsPlaintextGap {
			f.invalidateSession(1 - dir)
			f.closeSession()
		}
		if d.stopped {
			// Emitted events own their header evidence. A stopped direction
			// must not retain another copy until the TCP flow is evicted.
			d.http = nil
		}
	}()
	a.input.Add(uint64(len(data)))
	if d.stopped {
		a.unclassified.Add(uint64(len(data)))
		d.offset += uint64(len(data))
		return
	}
	if f.protocol == "" && f.ftpData == nil && len(data) > 0 && f.captureTCP {
		f.ftpData = f.a.matchFTPData(f, ts)
		if f.ftpData != nil {
			if f.ftpData.protected {
				f.protocol = "tls"
			} else {
				f.protocol = "ftp-data"
			}
		}
	}
	if f.protocol == "ftp-data" && f.ftpData != nil {
		f.emitFTPData(dir, data, ts)
		return
	}
	if len(d.buffer) == 0 {
		d.ts = ts
	}
	wire := data
	bufferedPrefix := len(d.buffer)
	detected := false
	if len(d.buffer) != 0 {
		old := len(d.buffer)
		if old+len(data) > a.config.MaxMessageBytes {
			room := a.config.MaxMessageBytes - old
			if room == 0 && f.omitHTTPPayload(dir, d.buffer, "message buffer limit reached") {
				// Process already buffered bytes first, then this arrival. Neither
				// is retried through the VM or retained as a large body.
				a.input.Add(^uint64(len(data) - 1))
				f.feed(dir, nil, ts)
				f.feed(dir, data, ts)
				return
			}
			if room == 0 {
				f.stop(dir, d.buffer, "limited", "message buffer limit reached")
				a.limited.Add(uint64(len(data)))
				return
			}
			// Split an arrival which finishes one large message and starts another.
			a.input.Add(^uint64(len(data) - 1))
			f.feed(dir, data[:room], ts)
			f.feed(dir, data[room:], ts)
			return
		}
		// Reserve without a temporary concatenation allocation.
		if cap(d.buffer) < old+len(data) {
			capacity := max(old+len(data), min(cap(d.buffer)*2, a.config.MaxMessageBytes))
			delta := int64(capacity - cap(d.buffer))
			current := a.buffered.Add(delta)
			if current > int64(a.config.MaxBufferedBytes) {
				a.buffered.Add(-delta)
				if f.omitHTTPPayload(dir, d.buffer, "capture buffer limit reached") {
					a.input.Add(^uint64(len(data) - 1))
					f.feed(dir, nil, ts)
					f.feed(dir, data, ts)
					return
				}
				f.stop(dir, d.buffer, "limited", "capture buffer limit reached")
				a.limited.Add(uint64(len(data)))
				return
			}
			for peak := a.peak.Load(); current > peak; peak = a.peak.Load() {
				if a.peak.CompareAndSwap(peak, current) {
					break
				}
			}
			grown := make([]byte, old, capacity)
			copy(grown, d.buffer)
			d.buffer = grown
		}
		d.buffer = append(d.buffer, data...)
		wire = d.buffer
	}
	if bufferedPrefix != 0 && (f.protocol == "" || f.dlms != nil && f.binding == nil) {
		window := wire
		if f.protocol == "" {
			window = window[:min(len(window), a.config.ProbeBytes+1)]
		}
		if idle := dlmsIdlePrefix(window); idle >= bufferedPrefix {
			// The surviving opening flag arrived in this Feed, after all
			// previously retained idle bytes. Keep its timestamp, not the
			// timestamp of an earlier idle-only arrival.
			d.ts = ts
		}
	}
	for len(wire) > 0 {
		if h := d.http; f.protocol == "http" && h != nil && h.omitted {
			n, done, err := h.consumeOmitted(wire)
			d.offset += uint64(n)
			a.limited.Add(uint64(n))
			wire = wire[n:]
			if err != nil {
				f.stop(dir, wire, "malformed", err.Error())
				return
			}
			if done {
				f.finishHTTP(dir)
				d.ts = ts
				continue
			}
			if n == 0 {
				break
			}
			continue
		}
		if f.protocol == "" {
			a.probes.Add(1)
			f.detectDirection(dir, wire)
			detected = f.protocol != ""
			if detected && f.protocol == "gopher" && f.textInternet != nil && dir != f.textInternet.clientDir {
				// Gopher replies carry no request identifier. Apply any buffered
				// selector first so the response event can include its observed context.
				client := &f.directions[f.textInternet.clientDir]
				if len(client.buffer) > 0 && !client.stopped {
					f.feed(f.textInternet.clientDir, nil, client.ts)
				}
			}
			if f.protocol == "" {
				if len(wire) >= a.config.ProbeBytes {
					if len(wire) < a.config.MaxMessageBytes && probeDLMSStream(wire, a.budget.MaxFrameBytes, a.config.ProbeBytes).Verdict == ProbeNeedMore {
						break
					}
					if len(wire) < a.budget.MaxFrameBytes && (needsMoreNBSSPrefix(wire, a.budget.MaxFrameBytes) || needsMoreTelnetPrefix(wire) || probeSLMPTCP(wire, min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes), false).Verdict == ProbeNeedMore || wrapperIncompletePrefix(wire, a.budget.MaxFrameBytes) || f.captureTCP && probeDNSTCP(wire, a.budget.MaxFrameBytes).Verdict == ProbeNeedMore || needsMoreS7Prefix(wire, a.budget.MaxFrameBytes) || f.needsMoreSSHServerPreamble(dir, wire) || initialProtocolNeedMore(wire, a.budget.MaxFrameBytes).Verdict == ProbeNeedMore || needsMoreHTTPStartLine(wire) || f.needsMorePortProtocolPrefix(wire) || f.needsMoreBoundedText(wire) || f.needsMoreOpenWire(wire) || !f.captureTCP && probeRADIUSAccountingByte(wire, min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)).Verdict == ProbeNeedMore) {
						break
					}
					f.stop(dir, wire, "unrecognized", "TCP application protocol not recognized")
					return
				}
				break
			}
		}
		if f.dlms != nil && f.binding == nil {
			// TCP-only feed: idle flags are received bytes, but not message
			// bytes. Advance provenance as well as the unclassified-byte ledger;
			// the event retains only the actual frame and its packet ranges.
			if idle := dlmsIdlePrefix(wire); idle != 0 {
				d.offset += uint64(idle)
				a.unclassified.Add(uint64(idle))
				d.pruneContributions(a)
				wire = wire[idle:]
			}
		}
		if f.protocol == "snmp" && f.snmp != nil {
			// Prune expired transactions before framing charges their retained
			// slots against the per-flow memory budget.
			f.snmp.expirePending(ts)
		}
		n, spec, err := f.frameDirection(dir, wire)
		if err != nil {
			status, typed := classifySessionError(err)
			f.lastSessionError = typed
			f.stop(dir, wire, status, err.Error())
			if h := d.http; f.protocol == "http" && h != nil && h.response && h.status == 101 {
				// Even an unverified 101 is an explicit HTTP protocol boundary.
				// Do not misdiagnose the subsequent opaque bytes as HTTP headers.
				f.directions[1-dir].stopped = true
				f.release(&f.directions[1-dir])
				f.closeSession()
				return
			}
			if f.hasSession() && !d.sshOpaque {
				f.invalidateSession(1 - dir)
				f.closeSession()
			}
			return
		}
		if n < 0 || n > a.config.MaxMessageBytes || f.protocol != "http2" && n > a.budget.MaxFrameBytes {
			reason := fmt.Sprintf("declared message exceeds limit: message_bytes=%d max_message_bytes=%d", n, a.config.MaxMessageBytes)
			if h := d.http; f.protocol == "http" && h != nil && h.declared > 0 {
				reason = fmt.Sprintf("HTTP declared message exceeds limit: message_bytes=%d max_message_bytes=%d; increase pcap_protocolBudget to decode the body", h.declared, a.config.MaxMessageBytes)
			}
			if f.omitHTTPPayload(dir, wire, reason) {
				continue
			}
			f.stop(dir, wire, "limited", reason)
			if f.hasSession() {
				f.invalidateSession(1 - dir)
				f.closeSession()
			}
			return
		}
		if n == 0 || n > len(wire) {
			break
		}
		if spec == nil {
			f.stop(dir, wire, "context-required", "protocol recognized, but no supported exact profile for this phase")
			return
		}
		e := f.event(dir, wire[:n], "deferred", f.protocol)
		e.Rule, e.Entry, e.plan = spec.rule, spec.entry, spec.plan
		if f.protocol == "nats" {
			e.Profile, e.Completeness = "nats-plaintext-client", "message"
		}
		if f.protocol == "openwire" {
			e.Profile = "openwire-length-prefixed"
			e.Completeness = "message"
			e.Admission = "validated-wireformatinfo-first-frame"
		}
		if f.protocol == "syslog" {
			e.syslogFraming = syslogStreamFraming(e.Raw)
			e.syslogBudget = a.budget.MaxCollectionElements
			e.syslogMaxBytes = a.config.MaxMessageBytes
			e.Completeness = "message"
			e.Profile = syslogProfileForWire(e.Raw, e.syslogFraming)
		}
		if f.protocol == "dns" || f.protocol == "dot" {
			e.decodeSkip = 2
		}
		var dnsFields map[string]any
		var dnsErr error
		if f.protocol == "dns" || f.protocol == "dot" {
			// Enforce native collection budgets before the rule VM can retry a
			// rejected message. Reuse accepted fields for DNS association below.
			dnsFields, dnsErr = DecodeDNSMessage(e.Raw[e.decodeSkip:], a.budget.MaxCollectionElements)
		}
		if f.protocol == "smb2" {
			e.decodeSkip = 4
		}
		httpSession := false
		if f.protocol == "http" {
			httpSession = d.http.doh || d.http.ipp
			e.ipp = d.http.ipp
			e.decodeConfig = d.http.config(f.a.config.MaxMessageBytes)
			e.Summary = d.http.summary
			f.httpEvidence(dir, e)
			f.finishHTTP(dir)
		}
		a.messages.Add(1)
		a.messageBytes.Add(uint64(n))
		stateful := httpSession || f.protocol != "http" && f.hasSession()
		// Syslog has no connection state. Deferred capture must retain only
		// framed bytes and defer its semantic parser until the viewer calls
		// Decode/GetFields.
		if f.protocol == "syslog" && a.config.Deferred {
			stateful = false
		}
		// DNS publishes request/response state below. Validate the same rule
		// projection first in both modes so deferred errors cannot publish a
		// successful association that eager decoding would have rejected.
		if !a.config.Deferred || stateful || f.protocol == "tls" || f.protocol == "dns" || f.protocol == "dot" {
			rawCopy := e.Raw
			if f.protocol == "quic" {
				rawCopy = append([]byte(nil), e.Raw...)
			}
			var result map[string]any
			var err error
			if f.bgp != nil || f.ajp != nil || f.rtmp != nil || f.ldp != nil {
				result = map[string]any{}
			} else if f.protocol == "tls" {
				if f.byteSource == "decrypted" {
					err = protocolError(ErrUnsupportedFeature, "nested TLS carrier")
				} else {
					result, err = f.consumeTLS(dir, e)
				}
			} else if f.protocol == "iso8583" || f.protocol == "fins" || f.protocol == "mms" || f.protocol == "slmp" || f.protocol == "redis" || f.protocol == "syslog" || f.protocol == "snmp" || f.protocol == "smb2" || f.protocol == "hart-ip" || f.protocol == "enip" || f.protocol == "doip" || f.protocol == "genisys" || f.protocol == "dlms-wrapper" || f.protocol == "dlms" || f.protocol == "c37118" || f.protocol == "roc-plus" || f.protocol == "atg" || f.memcached != nil || (e.Entry == "SSHIdentification" || e.Entry == "SSHPreIdentification") || f.protocol == "stratum" || f.protocol == "gearman" || f.protocol == "beanstalkd" || f.protocol == "scgi" || f.protocol == "msgpack-rpc" || f.protocol == "zookeeper" || f.protocol == "clickhouse" || f.protocol == "stomp" || f.protocol == "nats" || f.textInternet != nil || e.Entry == "MySQLPreparedFields" {
				result = map[string]any{}
			} else if e.Protocol == "rtsp" && rtspAppleControlWire(e.Raw, a.budget.MaxCollectionElements) {
				result = map[string]any{}
			} else if e.Protocol == "websocket" && e.Entry == "WebSocket" && f.ws != nil && f.ws.deflate {
				result = map[string]any{"fields": map[string]any{}}
			} else {
				var budgetErr *ProtocolError
				if errors.As(dnsErr, &budgetErr) && budgetErr.Kind == ErrResourceExceeded {
					err = dnsErr
				} else {
					result, err = e.Decode()
					// Preserve the shared codec's established error classification.
					if err != nil && dnsErr != nil {
						err = dnsErr
					}
				}
			}
			if err == nil && stateful {
				if f.protocol == "quic" {
					e.Raw = rawCopy
				}
				err = f.consumeSession(dir, e, result)
				if e.nativeSessionMessage {
					result = map[string]any{"fields": cloneSession(e.Session)}
				}
				if e.Profile == "rtsp-apple-control" {
					result = map[string]any{"fields": cloneSession(e.Session)}
				}
				if f.protocol == "redis" || f.protocol == "syslog" || e.Entry == "MySQLPreparedFields" || e.Entry == "SSHIdentification" || e.Entry == "SSHPreIdentification" {
					result = map[string]any{"fields": e.Session}
				}
				if f.protocol == "smb2" && e.Session != nil {
					e.semanticFields = cloneSession(e.Session)
					result = map[string]any{"fields": e.semanticFields}
				}
				if f.protocol == "snmp" && e.Session != nil {
					e.semanticFields = cloneSession(e.Session)
					result = map[string]any{"fields": cloneSession(e.semanticFields)}
				}
				if (f.protocol == "fins" || f.protocol == "mms" || f.protocol == "dlms-wrapper" || f.protocol == "slmp") && e.Session != nil {
					result = map[string]any{"fields": cloneSession(e.semanticFields)}
				}
				if (f.protocol == "hart-ip" || f.protocol == "enip" || f.protocol == "doip" || f.protocol == "genisys" || f.protocol == "dlms" || f.protocol == "c37118" || f.protocol == "roc-plus" || f.protocol == "atg" || f.memcached != nil) && e.Session != nil {
					e.semanticFields = cloneSession(e.Session)
					result = map[string]any{"fields": e.semanticFields}
				}
				if f.protocol == "stratum" && e.Session != nil {
					e.semanticFields = cloneSession(e.Session)
					result = map[string]any{"fields": e.semanticFields}
				}
				if f.protocol == "gearman" && e.Session != nil {
					e.semanticFields = cloneSession(e.Session)
					result = map[string]any{"fields": e.semanticFields}
				}
				if f.protocol == "beanstalkd" && e.Session != nil {
					e.semanticFields = cloneSession(e.Session)
					result = map[string]any{"fields": e.semanticFields}
				}
				if (f.protocol == "scgi" || f.protocol == "msgpack-rpc") && e.Session != nil {
					e.semanticFields = cloneSession(e.Session)
					result = map[string]any{"fields": e.semanticFields}
				}
				if f.protocol == "zookeeper" && e.Session != nil {
					e.semanticFields = cloneSession(e.Session)
					result = map[string]any{"fields": e.semanticFields}
				}
				if f.protocol == "clickhouse" && e.Session != nil {
					e.semanticFields = cloneSession(e.Session)
					result = map[string]any{"fields": e.semanticFields}
				}
				if (f.protocol == "stomp" || f.protocol == "nats") && e.Session != nil {
					e.semanticFields = cloneSession(e.Session)
					result = map[string]any{"fields": e.semanticFields}
				}
				if f.textInternet != nil && e.Session != nil {
					e.semanticFields = cloneSession(e.Session)
					result = map[string]any{"fields": e.semanticFields}
				}
				if e.Protocol == "iso8583" && e.Session != nil {
					result = map[string]any{"fields": cloneSession(e.semanticFields)}
				}
				if (e.Protocol == "ldap" || e.Entry == "LDAPProtectedRecord") && e.semanticFields != nil {
					result = map[string]any{"fields": cloneSession(e.semanticFields)}
				}
				if e.Protocol == "websocket" && f.ws != nil && f.ws.deflate {
					e.semanticFields = cloneSession(e.Session)
					result = map[string]any{"fields": e.semanticFields}
				}
			} else if err != nil && stateful && f.mailLike() {
				if cerr := f.consumeSession(dir, e, result); cerr == nil && e.Session != nil {
					err = nil
					// The session codecs also handle argumentless commands and
					// multiline replies missing from the older field grammar.
					// Retain the accepted result instead of retrying that grammar
					// when a deferred viewer asks for fields.
					e.semanticFields = cloneSession(e.Session)
					result = map[string]any{"fields": e.semanticFields}
				} else if cerr != nil {
					err = cerr
				}
			}
			if err != nil && f.radius != nil && !e.nativeRADIUSObservation {
				_, typed := classifySessionError(err)
				if typed != nil && typed.Kind == ErrResourceExceeded || radiusCompleteByteFrame(e.Raw) {
					if refused := f.consumeRADIUS(dir, e, e.Raw); refused != nil {
						err = refused
					} else {
						// Never turn a failed legacy projection into an accepted owner.
						f.radius.retire(dir, e.Raw, false)
						e.ResponseTo, e.TransactionID = 0, 0
						e.Session, e.semanticFields, e.semanticSession = nil, nil, nil
						if typed == nil {
							err = protocolError(ErrMalformedMessage, "%s", err)
						}
					}
				}
			}
			if err != nil {
				if errors.Is(err, stream_parser.ErrKafkaUnsupported) {
					err = protocolError(ErrUnsupportedFeature, err.Error())
				}
				e.Status, e.sessionError = classifySessionError(err)
				if ((e.Protocol == "iec104" || e.Protocol == "dnp3") || e.Entry == "MySQLPreparedFields" || e.Protocol == "mqtt" || e.nativeOPCUAObservation || e.nativeTLSObservation || e.nativeSNMPObservation || e.nativeSTUNObservation || e.nativeSIPObservation) && e.sessionError == nil {
					// These legacy codecs also return wrapped context or
					// grammar errors. Preserve their status/message privately so
					// later queries cannot use a caller-supplied successful cache.
					kind := ErrMalformedMessage
					if errors.Is(err, errBinContext) {
						kind = ErrContextRequired
					}
					e.sessionError = &ProtocolError{Kind: kind, Message: err.Error()}
				}
				e.Error = err.Error()
			} else if a.config.Deferred {
				if e.Protocol == "tls" || e.Protocol == "hart-ip" || e.Protocol == "enip" || e.Protocol == "doip" || e.Protocol == "genisys" || e.Protocol == "dlms-wrapper" || e.Protocol == "dlms" || e.Protocol == "c37118" || e.Protocol == "roc-plus" || e.Protocol == "atg" || e.Protocol == "stratum" || e.Protocol == "gearman" || e.Protocol == "beanstalkd" || e.Protocol == "scgi" || e.Protocol == "msgpack-rpc" {
					e.Structured = result
				}
			} else {
				e.Status, e.Structured = "decoded", result
			}
			if err == nil && e.nativeDNSStreamObservation {
				e.retainRuleEnvelope(result)
			}
			if err == nil && (stateful || e.Protocol == "tls") {
				// This exact result has already been accepted by the bounded
				// session codec. Retain its owned field projection for deferred
				// decoding and history; retrying a different grammar loses fields
				// or rejects valid argumentless/multiline messages.
				e.semanticFields = protocolFields(result)
				if e.nativeSIPObservation || (e.Protocol == "iec104" || e.Protocol == "dnp3") || e.Protocol == "goose" || e.Protocol == "mqtt" || e.nativeOPCUAObservation || e.Protocol == "websocket" && e.Session["OCPP"] != nil || e.Protocol == "rtp" && e.Transport != "udp" {
					e.retainRuleEnvelope(result)
				}
				if e.nativeRADIUSObservation || e.nativeDNSStreamObservation || e.nativeATGObservation || e.nativeHARTIPObservation || e.nativeENIPObservation || e.Protocol == "tls" || e.Protocol == "redis" || e.Entry == "MySQLPreparedFields" || e.Protocol == "stun" || e.Protocol == "turn" || e.Protocol == "snmp" || e.Protocol == "slmp" || e.Protocol == "dlms-wrapper" || e.Protocol == "dlms" || e.Protocol == "c37118" {
					e.semanticFields = cloneSession(e.semanticFields)
				}
			}
		}
		if e.Protocol == "dns" || e.Protocol == "dot" {
			if f.byteSource != "decrypted" {
				e.Protocol = "dns"
				e.Profile = "dns-tcp"
				if f.protocol == "dot" {
					e.Summary = fmt.Sprintf("DNS TCP %v id %v %v", e.Session["Packet Name"], e.Session["Transaction ID"], e.Session["QNAME"])
				}
			} else {
				e.Profile = "dns-over-tls"
			}
			// Preserve earlier framing/semantic errors. DNS association must not
			// publish a successful request for an already failed message.
			if e.Error == "" {
				var err error
				if dns, ok := e.Session["DNS"].(map[string]any); ok {
					err = a.dnsEventDecoded(e, dns)
				} else if dnsErr != nil {
					err = dnsErr
				} else if dnsFields != nil {
					err = a.dnsEventDecoded(e, dnsFields)
				} else {
					err = a.dnsEvent(e, e.Raw[2:])
				}
				if err != nil {
					e.Status, e.sessionError = classifySessionError(err)
					e.Error = err.Error()
					e.Structured = nil
				}
			}
		}
		if e.nativeDNSStreamObservation && e.Error != "" && e.sessionError == nil {
			kind := ErrMalformedMessage
			switch e.Status {
			case "limited":
				kind = ErrResourceExceeded
			case "context-required":
				kind = ErrContextRequired
			case "incomplete":
				kind = ErrNeedMore
			}
			e.sessionError = &ProtocolError{Kind: kind, Message: e.Error}
			if e.ExpertCode == "" {
				e.ExpertCode = string(kind)
			}
		}
		// Count the final outcome after all semantic validation, including DNS
		// RDATA validation in deferred mode. Each framed message counts once.
		switch e.Status {
		case "decoded":
			a.decoded.Add(1)
		case "deferred":
			a.deferred.Add(1)
		case "limited":
			a.limited.Add(uint64(n))
		case "context-required":
			a.contextRequired.Add(1)
		case "incomplete":
			a.incomplete.Add(1)
		default:
			a.malformed.Add(1)
		}
		d.offset += uint64(n)
		d.pruneContributions(a)
		wire = wire[n:]
		failed := e.Error != ""
		// Only complete frames with an independently retained boundary carry this private
		// application-level refusal scope. Snapshot before public callbacks.
		recoverableHARTIP := e.nativeHARTIPObservation && hartIPRecoverable(e.Raw, e.sessionError)
		recoverableRADIUS := e.nativeRADIUSObservation && radiusRecoverableFrame(e.Raw, e.sessionError, e.radiusApplicationRefusal)
		recoverableDoIP := e.Protocol == "doip" && e.doipAssociationRefusal
		recoverableDNP3 := e.Protocol == "dnp3" && e.Session["Error Scope"] == "application"
		tlsParent := e
		if !failed && f.tls != nil && f.tls.plaintext != nil {
			if e.ID == 0 {
				e.ID = a.ids.Add(1)
			}
			// The callback owns its event. Child evidence must retain the
			// emitted parent's identity, capture time and packet references.
			owned := *e
			owned.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
			tlsParent = &owned
		}
		// Recovery is engine policy, not a caller-editable event projection.
		stopAfterFailure := failed && !(e.Protocol == "tls" && e.Session["Record Failure Isolated"] == true) && !recoverableRADIUS && !recoverableHARTIP && !(e.Protocol == "fins" && finsRecoverable(e.sessionError)) && !recoverableDNP3 && !recoverableDoIP && !(e.Protocol == "slmp" && slmpTCPRecoverable(e.sessionError)) && !(e.Protocol == "dlms-wrapper" && dlmsRecoverableMessageError(e.sessionError)) && !(e.Protocol == "c37118" && c37118RecoverableMessageError(e.sessionError)) && !(e.Protocol == "dlms" && dlmsRecoverableMessageError(e.sessionError)) && !(e.Protocol == "roc-plus" && rocRecoverableMessageError(e.sessionError)) && !(e.Protocol == "genisys" && genisysRecoverableMessageError(e.sessionError)) && !((e.Protocol == "http2" || e.Protocol == "grpc" || e.Protocol == "doh") && e.Session["Error Scope"] == "stream")
		if e.nativeSessionMessage && e.Session["Error Scope"] == "message" {
			stopAfterFailure = false
		}
		children := e.rtmpChildren
		e.rtmpChildren = nil
		a.emit(e)
		for _, child := range children {
			a.messages.Add(1)
			a.messageBytes.Add(uint64(child.Length))
			switch child.Status {
			case "decoded":
				a.decoded.Add(1)
			case "incomplete":
				a.incomplete.Add(1)
			case "context-required":
				a.contextRequired.Add(1)
			case "limited":
				a.limited.Add(uint64(child.Length))
			default:
				a.malformed.Add(1)
			}
			a.emit(child)
		}
		if !failed {
			f.deliverTLS(dir, tlsParent)
		}
		if stopAfterFailure {
			if stateful {
				f.invalidateSession(1 - dir)
				f.closeSession()
			}
			if len(wire) > 0 {
				f.stop(dir, wire, "context-required", "decode failure invalidated stream affinity")
			} else {
				d.stopped = true
				f.release(d)
			}
			return
		}
		// A peer SETTINGS already captured before a complete client preface
		// must be applied before subsequent coalesced request header blocks.
		// Never replay the peer after only a partial greeting/preface.
		if detected || stateful && (strings.HasSuffix(fmt.Sprint(e.Session["Protocol Transition"]), "->tls") || e.Entry == "MySQLGreetingFields" || e.Entry == "HTTP2InitialClientStream" || e.Entry == "PostgreSQLStartupFields" || e.Entry == "PostgreSQLSSLRequestFields" || e.Protocol == "socks5" && f.protocol == "") {
			detected = false
			other := &f.directions[1-dir]
			if len(other.buffer) > 0 && !other.stopped {
				f.feed(1-dir, nil, other.ts)
			}
			if d.stopped {
				return
			}
		}
		d.ts = ts
	}
	if len(wire) == 0 {
		f.release(d)
	} else if !f.retain(d, wire) {
		if f.omitHTTPPayload(dir, wire, "capture buffer limit reached") {
			f.release(d)
			f.feed(dir, wire, ts)
			// feed accounts input itself; these bytes were already accounted.
			a.input.Add(^uint64(len(wire) - 1))
			return
		}
		f.stop(dir, wire, "limited", "capture buffer limit reached")
	}
	if detected && f.protocol != "" && !f.hasSession() {
		other := &f.directions[1-dir]
		if len(other.buffer) > 0 && !other.stopped {
			f.feed(1-dir, nil, other.ts)
		}
	}
}

func (f *binFlow) close(reason TrafficFlowCloseReason) {
	if f.dlms != nil && f.binding == nil {
		for i := range f.directions {
			d := &f.directions[i]
			// A proven HDLC stream may finish with idle fill. The last flag
			// was retained in case a frame followed; it is not a truncated
			// frame when no format byte ever arrived. Pending APDU context
			// is still finalized below and cannot be hidden by idle fill.
			if len(d.buffer) == 1 && d.buffer[0] == 0x7e {
				f.a.unclassified.Add(1)
				d.offset++
				d.pruneContributions(f.a)
				f.release(d)
			}
		}
	}
	hadBuffered := len(f.directions[0].buffer) > 0 || len(f.directions[1].buffer) > 0
	defer func() {
		if (!hadBuffered || f.rocplus != nil && f.rocplus.pending != nil) && !f.directions[0].stopped && !f.directions[1].stopped {
			f.finishSession(reason)
		}
		f.closeSession()
	}()
	for dir := range f.directions {
		d := &f.directions[dir]
		if h := d.http; h != nil && h.omitted {
			if h.closeDelimited && reason == TrafficFlowCloseReason_FIN {
				f.finishHTTP(dir)
			} else {
				e := f.event(dir, nil, "incomplete", string(reason)+": omitted HTTP body boundary was not reached")
				e.TransactionID = h.requestID
				if h.response && len(f.httpIDs) > 0 {
					e.TransactionID = f.httpIDs[0]
					e.ResponseTo = f.httpIDs[0]
				}
				f.a.incomplete.Add(1)
				f.a.emit(e)
			}
			f.release(d)
			continue
		}
		if len(d.buffer) != 0 {
			if h := d.http; h != nil && h.closeDelimited && reason == TrafficFlowCloseReason_FIN {
				e := f.event(dir, d.buffer, "deferred", h.summary)
				e.Rule, e.Entry, e.decodeConfig = "application-layer.http", "HTTPExact", h.config(f.a.config.MaxMessageBytes)
				e.ipp = h.ipp
				f.httpEvidence(dir, e)
				f.a.messages.Add(1)
				f.a.messageBytes.Add(uint64(len(d.buffer)))
				if !f.a.config.Deferred || h.doh || h.ipp {
					result, err := e.Decode()
					if err == nil && (h.doh || h.ipp) {
						err = f.consumeSession(dir, e, result)
					}
					if err != nil {
						e.Status, e.sessionError = classifySessionError(err)
						f.lastSessionError = e.sessionError
						e.Error = err.Error()
					} else if !f.a.config.Deferred {
						e.Status, e.Structured = "decoded", result
					}
				}
				switch e.Status {
				case "decoded":
					f.a.decoded.Add(1)
				case "deferred":
					f.a.deferred.Add(1)
				case "limited":
					f.a.limited.Add(uint64(len(d.buffer)))
				case "context-required":
					f.a.contextRequired.Add(1)
				default:
					f.a.malformed.Add(1)
				}
				f.release(d)
				f.finishHTTP(dir)
				f.a.emit(e)
				continue
			}
			e := f.event(dir, d.buffer, "incomplete", string(reason)+": exact message boundary was not reached")
			if f.radius != nil {
				e.nativeRADIUSObservation = true
				e.Profile, e.Completeness = f.radius.profile(), "incomplete"
				e.sessionError = &ProtocolError{Kind: ErrNeedMore, Message: e.Summary}
				e.Error = e.Summary
			}
			if e.nativeFINSObservation {
				e.Completeness = "incomplete"
				e.sessionError = &ProtocolError{Kind: ErrNeedMore, Message: e.Summary}
				e.Error = e.Summary
			}
			f.a.incomplete.Add(1)
			f.release(d)
			f.a.emit(e)
		} else {
			f.release(d)
		}
	}
}

// ProtocolInspector is a bounded message history for CLI/Yak viewing. It
// retains raw messages and routing metadata, not eager field trees. Details
// decode outside the capture lock. Eviction is observable, never capture loss.
type ProtocolInspector struct {
	mu                           sync.Mutex
	rows                         []*protocolHistoryEntry
	byID                         map[uint64][]*protocolHistoryEntry
	byPacket                     map[PacketReference][]*protocolHistoryEntry
	byPhysical                   map[PacketReference][]*protocolHistoryEntry
	packetIndexLinks, unindexed  int
	packetIndexLimit             int
	head, count, bytes, maxBytes int
	evicted                      uint64
	received                     uint64
}

type protocolHistoryEntry struct {
	BinParserEvent
	retainedCost  int
	packetIndexed bool
}

// BinParserInspector is retained for source compatibility. Use ProtocolInspector.
type BinParserInspector = ProtocolInspector

func NewBinParserInspector(messages, bytes int) (*BinParserInspector, error) {
	if messages <= 0 || messages > 1000000 || bytes <= 0 {
		return nil, errors.New("inspector requires positive bounded message/byte limits")
	}
	return &BinParserInspector{rows: make([]*protocolHistoryEntry, messages), maxBytes: bytes, byID: make(map[uint64][]*protocolHistoryEntry), byPacket: make(map[PacketReference][]*protocolHistoryEntry), byPhysical: make(map[PacketReference][]*protocolHistoryEntry), packetIndexLimit: min(256<<10, max(1, bytes/128))}, nil
}

func (v *BinParserInspector) OnEvent(e *BinParserEvent) {
	// The callback input remains caller-owned. Reject obviously oversized rows
	// before cloning. The logical retained cost is computed once per arrival.
	cost := protocolHistoryBytes(e)
	if cost > v.maxBytes {
		v.mu.Lock()
		v.received++
		v.evicted++
		v.mu.Unlock()
		return
	}
	row := protocolHistoryEntry{BinParserEvent: *e, retainedCost: cost}
	row.semanticFields = cloneSession(e.semanticFields)
	row.semanticSession = cloneSession(e.semanticSession)
	row.semanticMetadata = cloneSessionValue(e.semanticMetadata)
	row.Session = cloneSession(e.Session)
	cloneEvidence(&row.BinParserEvent)
	row.Structured, row.Fields, row.Metadata = nil, nil, nil
	row.Raw = append([]byte(nil), e.Raw...)
	v.mu.Lock()
	defer v.mu.Unlock()
	v.received++
	row.historySequence = v.received
	for v.count > 0 && (v.count == len(v.rows) || row.retainedCost > v.maxBytes-v.bytes) {
		v.unindex(v.rows[v.head])
		v.bytes -= v.rows[v.head].retainedCost
		v.rows[v.head] = nil
		v.head = (v.head + 1) % len(v.rows)
		v.count--
		v.evicted++
	}
	v.rows[(v.head+v.count)%len(v.rows)] = &row
	v.count++
	v.bytes += row.retainedCost
	v.index(&row)
}

// Rows returns metadata only, oldest first, optionally filtered by protocol and
// flow. Details(id) returns owned bytes plus the complete structured result.
func (v *BinParserInspector) Rows(protocol string, flow uint64) []*BinParserEvent {
	v.mu.Lock()
	defer v.mu.Unlock()
	var rows []*BinParserEvent
	for i := 0; i < v.count; i++ {
		e := &v.rows[(v.head+i)%len(v.rows)].BinParserEvent
		if (protocol == "" || protocol == e.Protocol) && (flow == 0 || flow == e.FlowID) {
			row := *e
			cloneEvidence(&row)
			row.Raw, row.Session, row.Structured, row.semanticFields, row.semanticSession, row.semanticMetadata = nil, nil, nil, nil, nil, nil
			row.ownedRuleEnvelope, row.hasRuleMetadata = false, false
			rows = append(rows, &row)
		}
	}
	return rows
}

func (v *BinParserInspector) Details(id uint64) (*BinParserEvent, error) {
	v.mu.Lock()
	var row *BinParserEvent
	if entries := v.byID[id]; len(entries) != 0 {
		e := &entries[0].BinParserEvent
		copyEvent := *e
		cloneEvidence(&copyEvent)
		copyEvent.Structured = cloneSession(e.Structured)
		copyEvent.semanticFields = cloneSession(e.semanticFields)
		copyEvent.semanticSession = cloneSession(e.semanticSession)
		copyEvent.semanticMetadata = cloneSessionValue(e.semanticMetadata)
		copyEvent.Session = cloneSession(e.Session)
		copyEvent.Raw = append([]byte(nil), e.Raw...)
		row = &copyEvent
	}
	v.mu.Unlock()
	if row == nil {
		return nil, fmt.Errorf("message %d is absent or evicted", id)
	}
	if row.Rule != "" || row.semanticFields != nil {
		result, err := row.Decode()
		if err != nil {
			return row, err
		}
		row.setStructured(result)
		if row.Status == "deferred" {
			row.Status = "decoded"
		}
	}
	return row, nil
}

func (v *BinParserInspector) Evicted() uint64 { v.mu.Lock(); defer v.mu.Unlock(); return v.evicted }

// Revision includes arrivals that evict old entries or exceed retention limits.
// A frozen packet list can gain contributor fields when a stream closes.
func (v *BinParserInspector) Revision() uint64 { v.mu.Lock(); defer v.mu.Unlock(); return v.received }

// RowsAfter returns at most limit newest matching arrivals after a viewer
// cursor. The cursor follows arrival order, not concurrently assigned event IDs.
// Omitted counts evicted history plus rows exceeding the presentation limit,
// never lost analysis. Filters only affect presentation. Returned rows own their
// metadata and omit Raw/Structured, just like Rows.
func (v *BinParserInspector) RowsAfter(after uint64, limit int, protocol string, flow uint64) (rows []*BinParserEvent, cursor, omitted uint64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	cursor = v.received
	if after >= cursor || limit <= 0 {
		return
	}
	var retained, matching uint64
	for i := v.count - 1; i >= 0; i-- {
		e := &v.rows[(v.head+i)%len(v.rows)].BinParserEvent
		if e.historySequence <= after {
			break
		}
		retained++
		if (protocol == "" || protocol == e.Protocol) && (flow == 0 || flow == e.FlowID) {
			matching++
			if len(rows) < limit {
				row := *e
				cloneEvidence(&row)
				row.Raw, row.Structured, row.Session, row.semanticFields, row.semanticSession, row.semanticMetadata = nil, nil, nil, nil, nil, nil
				row.ownedRuleEnvelope, row.hasRuleMetadata = false, false
				rows = append(rows, &row)
			}
		}
	}
	omitted = cursor - after - retained + matching - uint64(len(rows))
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return
}

// RetainedBytes estimates owned evidence storage for bounded consumers.
func (e *ProtocolEvent) RetainedBytes() int { return protocolHistoryBytes(e) }
