package pcaputil

import (
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
	ErrNeedMore           ProtocolErrorKind = "NeedMore"
	ErrMalformedMessage   ProtocolErrorKind = "MalformedMessage"
	ErrUnsupportedVersion ProtocolErrorKind = "UnsupportedVersion"
	ErrUnsupportedFeature ProtocolErrorKind = "UnsupportedFeature"
	ErrContextRequired    ProtocolErrorKind = "ContextRequired"
	ErrEncrypted          ProtocolErrorKind = "Encrypted"
	ErrResourceExceeded   ProtocolErrorKind = "ResourceExceeded"
	ErrDesynchronized     ProtocolErrorKind = "Desynchronized"
	ErrFatalSessionError  ProtocolErrorKind = "FatalSessionError"
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

// DefaultParserBudget matches the live capture parser defaults.
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
	f      *binFlow
	events []*ProtocolEvent
	closed bool
}

// NewProtocolSession builds an isolated capture flow on the same binFlow
// feed path as pcap replay. Ports are zero so admission cannot use 5432/389.
func NewProtocolSession(budget ParserBudget) (ProtocolSession, error) {
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
		a:         c.binParser,
		id:        1,
		endpoints: [2]string{"192.0.2.1:40000", "192.0.2.2:40001"},
		ports:     [2]uint16{40000, 40001},
	}
	return s, nil
}

func (s *captureSession) Probe(data []byte) ProbeResult {
	limit := s.f.a.config.ProbeBytes
	if limit <= 0 {
		limit = 64
	}
	if len(data) > limit {
		data = data[:limit]
	}
	probe := *s.f
	probe.protocol, probe.binding = "", nil
	probe.detect(data)
	if probe.protocol != "" {
		return probeAccept(probe.protocol, "", 90)
	}
	return probeWire(data, limit)
}

func (s *captureSession) Feed(direction int, ts time.Time, data []byte) FeedResult {
	if s.closed {
		return FeedResult{State: "closed", Err: &ProtocolError{Kind: ErrFatalSessionError, Message: "session is closed"}}
	}
	if direction != 0 && direction != 1 {
		return FeedResult{Err: &ProtocolError{Kind: ErrMalformedMessage, Message: "direction must be 0 or 1"}}
	}
	before := s.f.a.input.Load()
	s.events = nil
	s.f.feed(direction, data, ts)
	out := FeedResult{Consumed: int(s.f.a.input.Load() - before), Events: s.events, State: s.f.protocol}
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
	return f.h2 != nil || f.mysql != nil || f.pg != nil || f.ws != nil || f.ldap != nil || f.redis != nil || f.mqtt != nil || f.mongo != nil || f.kafka != nil || f.tds != nil || f.amqp != nil || f.smb2 != nil || f.dcerpc != nil || f.ssh != nil || f.nfs != nil || f.snmp != nil || f.rdp != nil || f.dot != nil || f.doh != nil || f.sip != nil
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
	if p := probeHTTP2(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeMySQL(w, limit); p.Verdict != ProbeReject {
		return p
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
	if p := probeRedis(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeWebSocket(w, limit); p.Verdict != ProbeReject {
		return p
	}
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
	if p := probeDoT(w, limit); p.Verdict != ProbeReject {
		return p
	}
	if p := probeSIP(w, limit); p.Verdict != ProbeReject {
		return p
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
	// The first five greeting bytes can resemble a short PostgreSQL header.
	// Wait for the server-version prefix before allowing another candidate.
	if len(w) == 5 && w[3] == 0 && w[4] == 10 {
		return probeNeed("mysql", "10", len(w), 6)
	}
	if len(w) < 6 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[3] != 0 || w[4] != 10 {
		return ProbeResult{Verdict: ProbeReject}
	}
	end := indexByte(w[5:], 0)
	if end < 0 {
		if len(w) < 32 {
			return probeNeed("mysql", "10", len(w), 32)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	for _, c := range w[5 : 5+end] {
		if c < 32 || c > 126 {
			return ProbeResult{Verdict: ProbeReject}
		}
	}
	return probeAccept("mysql", "10", 90)
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
