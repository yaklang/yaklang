package pcaputil

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// ProbeVerdict is the bounded protocol-admission result. Probe never runs a
// full decoder until it errors.
type ProbeVerdict int

const (
	ProbeReject ProbeVerdict = iota
	ProbeNeedMore
	ProbeAccept
)

// ProbeResult is the look-ahead admission decision for a byte prefix.
type ProbeResult struct {
	Verdict    ProbeVerdict
	Confidence uint8
	NeedBytes  int
	Protocol   string
	Version    string
	Reason     string
}

// ProtocolErrorKind is the unified session error taxonomy.
type ProtocolErrorKind string

const (
	ErrNeedMore             ProtocolErrorKind = "NeedMore"
	ErrMalformedMessage     ProtocolErrorKind = "MalformedMessage"
	ErrUnsupportedVersion   ProtocolErrorKind = "UnsupportedVersion"
	ErrUnsupportedFeature   ProtocolErrorKind = "UnsupportedFeature"
	ErrContextRequired      ProtocolErrorKind = "ContextRequired"
	ErrEncrypted            ProtocolErrorKind = "Encrypted"
	ErrAuthenticationFailed ProtocolErrorKind = "AuthenticationFailed"
	ErrResourceExceeded     ProtocolErrorKind = "ResourceExceeded"
	ErrDesynchronized       ProtocolErrorKind = "Desynchronized"
	ErrFatalSessionError    ProtocolErrorKind = "FatalSessionError"
)

// ProtocolError is a typed session failure. It is never a successful tree.
type ProtocolError struct {
	Kind    ProtocolErrorKind
	Message string
}

func (e *ProtocolError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return string(e.Kind)
	}
	return string(e.Kind) + ": " + e.Message
}

// FeedResult is one Feed call's consumption, events, and typed error.
type FeedResult struct {
	Consumed int
	NeedMore bool
	Events   []*ProtocolEvent
	State    string
	Err      *ProtocolError
}

// ParserBudget is the shared resource contract. Zero fields use defaults.
type ParserBudget struct {
	MaxFrameBytes, MaxMessageBytes, MaxBufferedBytes int
	MaxRecursionDepth, MaxCollectionElements         int
	ProbeBytes                                       int
}

// DefaultParserBudget is the conservative standalone session budget.
// Live captures use larger defaults and can configure WithProtocolBudget.
func DefaultParserBudget() ParserBudget {
	return ParserBudget{
		MaxFrameBytes:         1 << 20,
		MaxMessageBytes:       1 << 20,
		MaxBufferedBytes:      32 << 20,
		MaxRecursionDepth:     64,
		MaxCollectionElements: 4096,
		ProbeBytes:            64,
	}
}

// ProtocolSession is the single Probe/Feed/Close path used by live replay
// and by M1 protocol tests. Probe does not consume; Feed starts at byte 0.
// Calls on a session must be serialized in TCP delivery order.
type ProtocolSession interface {
	Probe(data []byte) ProbeResult
	Feed(direction int, ts time.Time, data []byte) FeedResult
	Close(reason string) []*ProtocolEvent
	Stats() ProtocolStats
}

type captureSession struct {
	f         *binFlow
	events    []*ProtocolEvent
	closed    bool
	transport string
}

// NewProtocolSession builds an isolated capture flow on the same binFlow
// feed path as pcap replay. Ports are zero so admission cannot use 5432/389.
func NewProtocolSession(budget ParserBudget) (ProtocolSession, error) {
	return NewProtocolSessionWithOptions(budget)
}

// NewProtocolSessionWithOptions adds observed transport and endpoint context
// without changing the original constructor or its function type.
func NewProtocolSessionWithOptions(budget ParserBudget, options ...ProtocolSessionOption) (ProtocolSession, error) {
	defaults := DefaultParserBudget()
	for _, pair := range [][2]*int{
		{&budget.MaxFrameBytes, &defaults.MaxFrameBytes}, {&budget.MaxMessageBytes, &defaults.MaxMessageBytes},
		{&budget.MaxBufferedBytes, &defaults.MaxBufferedBytes}, {&budget.ProbeBytes, &defaults.ProbeBytes},
		{&budget.MaxRecursionDepth, &defaults.MaxRecursionDepth}, {&budget.MaxCollectionElements, &defaults.MaxCollectionElements},
	} {
		if *pair[0] < 0 {
			return nil, fmt.Errorf("negative parser budget")
		}
		if *pair[0] == 0 {
			*pair[0] = *pair[1]
		}
	}
	budget.MaxFrameBytes = min(budget.MaxFrameBytes, budget.MaxMessageBytes)
	if budget.MaxRecursionDepth > 64 || budget.MaxCollectionElements > 4096 {
		return nil, fmt.Errorf("parser structural budget exceeds supported maximum")
	}
	c := NewDefaultConfig()
	s := &captureSession{}
	err := WithBinParserConfig(BinParserConfig{
		MaxMessageBytes:  budget.MaxMessageBytes,
		MaxBufferedBytes: budget.MaxBufferedBytes,
		ProbeBytes:       budget.ProbeBytes,
		OnEvent: func(e *ProtocolEvent) {
			s.events = append(s.events, e)
		},
	})(c)
	if err != nil {
		return nil, err
	}
	if err = c.prepareBinParser(); err != nil {
		return nil, err
	}
	c.binParser.budget = budget
	c.binParser.flows.Add(1)
	s.f = &binFlow{
		a:                 c.binParser,
		id:                1,
		endpoints:         [2]string{"192.0.2.1:40000", "192.0.2.2:40001"},
		ports:             [2]uint16{40000, 40001},
		syslogStreamProbe: true,
	}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("nil session option")
		}
		if err := option(s); err != nil {
			return nil, err
		}
	}
	if b := s.f.a.pfcpSessionIdentity; b != nil {
		if s.transport != "udp" {
			return nil, fmt.Errorf("PFCP receiver identity requires a UDP session")
		}
		b.endpoints = s.f.endpoints
	}
	if s.transport == "udp" && s.f.tcpClientKnown {
		return nil, fmt.Errorf("TCP initiator direction cannot be set on a UDP session")
	}
	return s, nil
}

// NewDecryptedQUICSession accepts caller-authenticated/decrypted long-header
// packets with unprotected packet numbers and plaintext frame payloads. It is
// NOT a capture/wire decoder. The source label is required and emitted on every
// plaintext event; Authentication Verified remains false (verification belongs
// to the caller). Capture configuration never enables this mode.
func NewDecryptedQUICSession(budget ParserBudget, source string) (ProtocolSession, error) {
	if source == "" || len(source) > 256 {
		return nil, fmt.Errorf("QUIC plaintext requires a bounded source label")
	}
	session, err := NewProtocolSession(budget)
	if err != nil {
		return nil, err
	}
	session.(*captureSession).f.a.decryptedQUICSource = source
	return session, nil
}

func (s *captureSession) Probe(data []byte) ProbeResult {
	if s.transport == "udp" {
		if p, ok := s.probeSemtechDownlink(data); ok {
			return p
		}
		if p, ok := s.probeDoIPDiscovery(data); ok {
			return p
		}
		if p := s.f.a.probeWrapperDatagram(data); p.Verdict == ProbeAccept {
			return p
		}
		if p, ok := s.probeDiscovery(data); ok {
			return p
		}
		return probeDatagram(data, s.f.a.budget.MaxFrameBytes)
	}
	limit := s.f.a.config.ProbeBytes
	if limit <= 0 {
		limit = 64
	}
	input := data
	if p := probeSLMPTCP(input, min(s.f.a.budget.MaxFrameBytes, s.f.a.config.MaxMessageBytes), s.f.port(5000)); p.Verdict != ProbeReject {
		return p
	}
	if s.f.tcpClientKnown && len(input) > 0 && input[0] == 1 {
		if p := probeATG(input, s.f.a.budget.MaxFrameBytes); p.Verdict != ProbeReject {
			return p
		}
	}
	if p := probeRDP(input, s.f.a.budget.MaxFrameBytes); p.Verdict == ProbeAccept {
		return p
	}
	if p := probeS7(input, s.f.a.budget.MaxFrameBytes); p.Verdict != ProbeReject {
		if p.Verdict == ProbeNeedMore && len(input) >= 4 && int(binary.BigEndian.Uint16(input[2:4])) == len(input) {
			return ProbeResult{Verdict: ProbeReject, Reason: "COTP carrier alone does not establish S7"}
		}
		return p
	}
	if p := s.f.a.probeWrapper(input); p.Verdict != ProbeReject {
		return p
	}
	if p := probeDLMS(input, s.f.a.budget.MaxFrameBytes); p.Verdict != ProbeReject {
		return p
	}
	if p := probeGenisys(input, s.f.a.budget.MaxFrameBytes); p.Verdict != ProbeReject {
		return p
	}
	if p := probeROCPlus(input, s.f.a.budget.MaxFrameBytes); p.Verdict != ProbeReject {
		return p
	}
	if p := probeDoIP(input, s.f.a.budget.MaxFrameBytes); p.Verdict != ProbeReject {
		return p
	}
	if isHTTPStartLineCandidate(input) {
		if p := probeRTSP(input, limit); p.Verdict == ProbeAccept {
			return p
		}
		// CONNECT/INFO can be long NATS control lines. Preserve their bounded
		// exact admission before testing the more general HTTP method syntax.
		if p := initialProtocolNeedMore(input, s.f.a.budget.MaxFrameBytes); p.Verdict != ProbeReject {
			return p
		}
		if result := s.f.probeBoundedText(input); result.Verdict != ProbeReject {
			return result
		}
		return s.f.probeHTTPStartLine(input)
	}
	if len(data) > limit {
		data = data[:limit]
	}
	probe := *s.f
	probe.protocol, probe.binding = "", nil
	probe.detect(input)
	if probe.protocol != "" {
		if probe.protocol == "cassandra" && len(data) >= 9 {
			total := 9 + int(binary.BigEndian.Uint32(data[5:9]))
			if len(input) < total {
				return ProbeResult{Verdict: ProbeNeedMore, Protocol: "cassandra", NeedBytes: total - len(input), Confidence: 40, Reason: "initial envelope body is incomplete"}
			}
		}
		return probeAccept(probe.protocol, "", 90)
	}
	if s.f.captureTCP {
		if p := probeDNSTCP(input, s.f.a.budget.MaxFrameBytes); p.Verdict != ProbeReject {
			return p
		}
	}
	if result := s.f.probeBoundedText(input); result.Verdict != ProbeReject {
		return result
	}
	if probe := probeOpenWire(input, s.f.a.budget.MaxFrameBytes); probe.Verdict != ProbeReject {
		return probe
	}
	return probeWireTransport(data, limit, s.f.captureTCP)
}

func (s *captureSession) Feed(direction int, ts time.Time, data []byte) FeedResult {
	if s.closed {
		return FeedResult{State: "closed", Err: &ProtocolError{Kind: ErrFatalSessionError, Message: "session is closed"}}
	}
	if direction != 0 && direction != 1 {
		return FeedResult{Err: &ProtocolError{Kind: ErrMalformedMessage, Message: "direction must be 0 or 1"}}
	}
	if s.transport == "udp" {
		return s.feedDatagram(direction, ts, data)
	}
	before := s.f.a.input.Load()
	s.events = nil
	s.f.feed(direction, data, ts)
	out := FeedResult{Consumed: int(s.f.a.input.Load() - before), Events: s.events, State: s.f.protocol}
	out.Err = sessionErrorFromEvents(out.Events)
	s.events = nil
	if out.State == "" {
		out.State = "undetected"
	}
	d := &s.f.directions[direction]
	if d.stopped {
		out.Err = sessionErrorFromEvents(out.Events)
		if out.Err == nil {
			out.Err = &ProtocolError{Kind: ErrFatalSessionError, Message: "session stopped"}
		}
		return out
	}
	if len(d.buffer) > 0 {
		out.NeedMore = true
		if len(out.Events) == 0 {
			out.Err = &ProtocolError{Kind: ErrNeedMore, Message: "incomplete message"}
		}
	}
	return out
}

func (s *captureSession) Close(reason string) []*ProtocolEvent {
	if s.closed {
		return nil
	}
	s.closed = true
	s.events = nil
	s.f.close(TrafficFlowCloseReason(reason))
	s.f.a.closeUDPSessions()
	s.f.a.closeDCPObservations()
	s.f.a.closeMediaAssociations()
	s.f.a.closeRTSPMedia()
	s.f.a.flows.Store(0)
	events := s.events
	s.events = nil
	return events
}

func (s *captureSession) Stats() ProtocolStats { return s.f.a.stats() }

func sessionErrorFromEvents(events []*ProtocolEvent) *ProtocolError {
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Error == "" && (e.Status == "decoded" || e.Status == "deferred") {
			continue
		}
		if e.sessionError != nil {
			copyError := *e.sessionError
			return &copyError
		}
		kind := ErrMalformedMessage
		switch e.Status {
		case "context-required":
			kind = ErrContextRequired
		case "limited":
			kind = ErrResourceExceeded
		case "unrecognized":
			kind = ErrUnsupportedFeature
		case "incomplete":
			kind = ErrNeedMore
		}
		return &ProtocolError{Kind: kind, Message: e.Error}
	}
	return nil
}

func (f *binFlow) hasSession() bool {
	if f.protocol == "nats" {
		return true // NATS allocates its budgeted negotiation state at consumption.
	}
	return f.slmpTCP != nil || f.wrapper != nil || f.tls != nil || f.dnp3 != nil || f.c37118 != nil || f.goose != nil || f.syslog != nil || f.rfb != nil || f.diameter != nil || f.iec104 != nil || f.s7 != nil || f.opcua != nil || f.ipp != nil || f.rtsp != nil || f.stun != nil || f.h2 != nil || f.mysql != nil || f.pg != nil || f.ws != nil || f.ldap != nil || f.redis != nil || f.mqtt != nil || f.nats != nil || f.mongo != nil || f.kafka != nil || f.tds != nil || f.amqp != nil || f.smb2 != nil || f.dcerpc != nil || f.ssh != nil || f.nfs != nil || f.snmp != nil || f.rdp != nil || f.dot != nil || f.doh != nil || f.sip != nil || f.rtp != nil || f.quic != nil || f.smtp != nil || f.imap != nil || f.pop3 != nil || f.ftp != nil || f.tns != nil || f.socks5 != nil || f.scgi != nil || f.msgpackRPC != nil || f.textInternet != nil || f.radius != nil || f.dhcp != nil || f.ntp != nil || f.coap != nil || f.modbus != nil || f.memcached != nil || f.enip != nil || f.doip != nil || f.genisys != nil || f.dlms != nil || f.rocplus != nil || f.atg != nil || f.stratum != nil || f.gearman != nil || f.beanstalk != nil || f.zookeeper != nil || f.clickhouse != nil || f.stomp != nil
}

func (f *binFlow) mailLike() bool {
	switch f.protocol {
	case "smtp", "imap", "pop3", "ftp", "tns", "radius", "dhcp", "ntp", "coap", "modbus", "iec104", "dnp3", "c37118", "goose":
		return true
	}
	return false
}

func probeNeed(protocol, version string, have, want int) ProbeResult {
	if have >= want {
		return ProbeResult{Verdict: ProbeReject, Protocol: protocol, Reason: "prefix exhausted"}
	}
	return ProbeResult{Verdict: ProbeNeedMore, Protocol: protocol, Version: version, NeedBytes: want - have, Confidence: 40}
}

func probeAccept(protocol, version string, conf uint8) ProbeResult {
	return ProbeResult{Verdict: ProbeAccept, Protocol: protocol, Version: version, Confidence: conf}
}

func probeWire(w []byte, limit int) ProbeResult {
	return probeWireTransport(w, limit, false)
}

func probeWireTransport(w []byte, limit int, tcp bool) ProbeResult {
	memcached := probeMemcached(w, limit)
	if memcached.Verdict == ProbeAccept {
		return memcached
	}
	if tcp {
		if p := probeDLMS(w, limit); p.Verdict != ProbeReject {
			return p
		}
		if p := probeROCPlus(w, limit); p.Verdict != ProbeReject {
			return p
		}
	}
	if p := probeDoIP(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeENIP(w); p.Verdict != ProbeReject {
		return p
	}
	if p := probeGearman(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeZooKeeper(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeClickHouse(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeSTOMP(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeSOCKS5(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeRFB(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeOPCUA(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeS7(w, limit); p.Verdict == ProbeAccept {
		return p
	}
	if p := probeIEC104(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeDiameter(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeRTSP(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeSTUN(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeHTTP2(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeMySQL(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if tcp {
		if p := probeGenisys(w, limit); p.Verdict != ProbeReject {
			return p
		}
	}
	if p := probePostgres(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeLDAP(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeSNMP(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeSMTP(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeFTP(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeDHCP(w, limit); !tcp && p.Verdict != ProbeReject {
		return p
	}
	if p := probeRADIUS(w, limit); !tcp && p.Verdict != ProbeReject {
		return p
	}
	if p := probeDNP3(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeTNS(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeModbus(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeC37118(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeGOOSE(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeIMAP(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probePOP3(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeRedis(w, limit); p.Verdict != ProbeReject {
		return p
	}
	// A WebSocket frame has no standalone wire signature. Admission belongs
	// to the validated HTTP upgrade; otherwise many unrelated binary messages
	// match its two-byte header.
	if p := probeMQTT(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeMongo(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeNFS(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeKafka(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeTDS(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeAMQP(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeSMB2(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeDCERPC(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeSSH(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeRDP(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeDoT(w, limit); !tcp && p.Verdict != ProbeReject {
		return p
	}
	if p := probeSIP(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeNTP(w, limit); !tcp && p.Verdict != ProbeReject {
		return p
	}
	if p := probeCoAP(w, limit); !tcp && p.Verdict != ProbeReject {
		return p
	}
	if p := probeRTP(w, limit); !tcp && p.Verdict != ProbeReject {
		return p
	}
	if p := probeQUIC(w, limit); !tcp && p.Verdict != ProbeReject {
		return p
	}
	if p := probeStratum(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if memcached.Verdict == ProbeNeedMore {
		return memcached
	}
	return ProbeResult{Verdict: ProbeReject, Reason: "no protocol match"}
}

func probeHTTP2(w []byte, limit int) ProbeResult {
	preface := []byte(binH2Preface)
	if bytesHasPrefix(w, preface) {
		return probeAccept("http2", "2", 100)
	}
	if len(w) < len(preface) && bytesHasPrefix(preface, w) {
		return probeNeed("http2", "2", len(w), len(preface))
	}
	_ = limit
	return ProbeResult{Verdict: ProbeReject}
}

func probeMySQL(w []byte, _ int) ProbeResult {
	if len(w) < 5 || w[3] != 0 || w[4] != 10 {
		return ProbeResult{Verdict: ProbeReject}
	}
	payload := int(w[0]) | int(w[1])<<8 | int(w[2])<<16
	if payload < 31 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 6 {
		return probeNeed("mysql", "10", len(w), 6)
	}
	// A numeric server version, connection ID, salt, zero filler and protocol-41
	// capability distinguish a greeting from an OP_REPLY or arbitrary packet.
	if w[5] < '0' || w[5] > '9' {
		return ProbeResult{Verdict: ProbeReject}
	}
	end := indexByte(w[5:], 0)
	if end < 0 {
		if len(w) < 64 {
			return probeNeed("mysql", "10", len(w), len(w)+1)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	for _, c := range w[5 : 5+end] {
		if c < 32 || c > 126 {
			return ProbeResult{Verdict: ProbeReject}
		}
	}
	at := 6 + end
	if at+15 > payload+4 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < at+15 {
		return probeNeed("mysql", "10", len(w), at+15)
	}
	if w[at+12] != 0 || binary.LittleEndian.Uint16(w[at+13:at+15])&0x0200 == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("mysql", "10", 98)
}

func bytesHasPrefix(b, prefix []byte) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i := range prefix {
		if b[i] != prefix[i] {
			return false
		}
	}
	return true
}

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

func protocolError(kind ProtocolErrorKind, format string, args ...any) error {
	return &ProtocolError{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

func classifySessionError(err error) (string, *ProtocolError) {
	var typed *ProtocolError
	if errors.As(err, &typed) {
		// Preserve the established capture status for connection-state budgets.
		if errors.Is(err, errBinContext) {
			return "context-required", typed
		}
		switch typed.Kind {
		case ErrResourceExceeded:
			return "limited", typed
		case ErrContextRequired, ErrEncrypted, ErrUnsupportedFeature, ErrUnsupportedVersion:
			return "context-required", typed
		case ErrNeedMore:
			return "incomplete", typed
		default:
			return "malformed", typed
		}
	}
	if errors.Is(err, errBinContext) {
		return "context-required", nil
	}
	return "malformed", nil
}
