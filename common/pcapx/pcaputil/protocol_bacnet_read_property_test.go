package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func bacnetRPWire(invoke byte, response bool, value []byte) []byte {
	a := []byte{0, 5, invoke, 12}
	ctrl := byte(4)
	if response {
		a = []byte{0x30, invoke, 12}
		ctrl = 0
	}
	a = append(a, 0x0c, 0, 0x80, 0, 1, 0x19, 85)
	if response {
		a = append(a, 0x3e)
		a = append(a, value...)
		a = append(a, 0x3f)
	}
	return bacnetTestPacket(10, append([]byte{1, ctrl}, a...))
}

func bacnetRPSession(t *testing.T, deferred bool) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 47808), WithSessionBACnetReadProperty())
	require.NoError(t, err)
	s.(*captureSession).f.a.config.Deferred = deferred
	t.Cleanup(func() { s.Close("test complete"); require.Zero(t, s.(*captureSession).f.a.buffered.Load()) })
	return s
}
func bacnetRPFeed(t *testing.T, s ProtocolSession, dir int, sec int64, w []byte) *ProtocolEvent {
	t.Helper()
	out := s.Feed(dir, time.Unix(sec, 0), w)
	require.Len(t, out.Events, 1)
	require.Equal(t, len(w), out.Consumed)
	return out.Events[0]
}
func bacnetRPReject(t *testing.T, e *ProtocolEvent, kind ProtocolErrorKind) {
	t.Helper()
	require.Zero(t, e.ResponseTo)
	_, err := e.GetFields()
	var pe *ProtocolError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, kind, pe.Kind)
	e.Fields = map[string]any{"forged": true}
	e.Structured = map[string]any{"fields": e.Fields}
	e.Session = e.Fields
	e.Protocol = "http"
	e.Status = "decoded"
	_, err = e.Decode()
	require.ErrorAs(t, err, &pe)
	require.Equal(t, kind, pe.Kind)
}

func TestBACnetReadPropertyFieldsAndOwnership(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("deferred=%v", deferred), func(t *testing.T) {
			s := bacnetRPSession(t, deferred)
			reqWire := bacnetRPWire(36, false, nil)
			respWire := bacnetRPWire(36, true, []byte{0x44, 0x3f, 0x80, 0, 0})
			req := bacnetRPFeed(t, s, 0, 1, reqWire)
			resp := bacnetRPFeed(t, s, 1, 2, respWire)
			for i, e := range []*ProtocolEvent{req, resp} {
				require.Equal(t, "bacnet", e.Protocol)
				require.Equal(t, bacnetReadPropertyProfile, e.Profile)
				require.Equal(t, uint64(36), e.TransactionID)
				require.Equal(t, i, e.Direction)
				require.Equal(t, req.FlowID, e.FlowID)
				require.NotZero(t, e.FlowID)
				f, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, uint32(2), f["Object Type"])
				require.Equal(t, uint32(1), f["Object Instance"])
				require.Equal(t, uint32(85), f["Property Identifier"])
				require.Equal(t, false, f["Array Index Present"])
				require.Equal(t, false, f["Authentication Verified"])
				require.Equal(t, f, e.Session)
				saved := cloneSession(f)
				f["Object Type"] = 999
				e.Session["Object Instance"] = 999
				e.Fields = map[string]any{"forged": true}
				e.Structured = map[string]any{"fields": e.Fields}
				e.Raw[0] = 0
				got, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, saved, got)
				decoded, err := e.Decode()
				require.NoError(t, err)
				require.Equal(t, saved, decoded["session"])
			}
			require.Zero(t, req.ResponseTo)
			require.Equal(t, req.ID, resp.ResponseTo)
			f, err := resp.GetFields()
			require.NoError(t, err)
			v := f["Values"].([]any)[0].(map[string]any)
			require.Equal(t, "Real", v["Type"])
			require.Equal(t, float64(1), v["Value"])
			require.Equal(t, "3f800000", v["Raw Bits"])
			require.Equal(t, []byte{0x44, 0x3f, 0x80, 0, 0}, v["Encoded"])
		})
	}
}

func TestBACnetReadPropertyAssociationBoundaries(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, scenario := range []string{"interleave", "retry", "reuse", "wrong-direction", "wrong-property", "orphan", "expiry", "backdated", "malformed", "oversize"} {
			t.Run(fmt.Sprintf("%s/deferred=%v", scenario, deferred), func(t *testing.T) {
				s := bacnetRPSession(t, deferred)
				req := bacnetRPFeed(t, s, 0, 100, bacnetRPWire(0, false, nil))
				require.Zero(t, req.TransactionID)
				require.Zero(t, req.ResponseTo)
				ack := bacnetRPWire(0, true, []byte{0})
				switch scenario {
				case "interleave":
					r2 := bacnetRPFeed(t, s, 0, 101, bacnetRPWire(255, false, nil))
					e2 := bacnetRPFeed(t, s, 1, 102, bacnetRPWire(255, true, []byte{0x11}))
					require.Equal(t, r2.ID, e2.ResponseTo)
					require.Equal(t, uint64(255), e2.TransactionID)
					e := bacnetRPFeed(t, s, 1, 103, ack)
					require.Equal(t, req.ID, e.ResponseTo)
					require.Equal(t, uint64(0), e.TransactionID)
					return
				case "retry":
					bacnetRPReject(t, bacnetRPFeed(t, s, 0, 101, bacnetRPWire(0, false, nil)), ErrContextRequired)
				case "reuse":
					e := bacnetRPFeed(t, s, 1, 101, ack)
					require.Equal(t, req.ID, e.ResponseTo)
					bacnetRPReject(t, bacnetRPFeed(t, s, 0, 102, bacnetRPWire(0, false, nil)), ErrContextRequired)
				case "wrong-direction":
					bacnetRPReject(t, bacnetRPFeed(t, s, 0, 101, ack), ErrContextRequired)
					return
				case "wrong-property":
					wrong := bytes.Clone(ack)
					wrong[15] = 86
					bacnetRPReject(t, bacnetRPFeed(t, s, 1, 101, wrong), ErrContextRequired)
				case "orphan":
					bacnetRPReject(t, bacnetRPFeed(t, s, 1, 101, bacnetRPWire(1, true, []byte{0})), ErrContextRequired)
					e := bacnetRPFeed(t, s, 1, 102, ack)
					require.Equal(t, req.ID, e.ResponseTo)
					return
				case "expiry":
					bacnetRPReject(t, bacnetRPFeed(t, s, 1, 130, ack), ErrContextRequired)
					return
				case "backdated":
					bacnetRPReject(t, bacnetRPFeed(t, s, 1, 99, ack), ErrContextRequired)
				case "malformed":
					bad := bytes.Clone(ack)
					bad[len(bad)-1] = 0x2f
					bacnetRPReject(t, bacnetRPFeed(t, s, 1, 101, bad), ErrMalformedMessage)
				case "oversize":
					s.(*captureSession).f.a.config.MaxMessageBytes = 25
					large := bacnetRPWire(0, true, append([]byte{0x65, 30}, make([]byte, 30)...))
					bacnetRPReject(t, bacnetRPFeed(t, s, 1, 101, large), ErrResourceExceeded)
				}
				e := bacnetRPFeed(t, s, 1, 103, ack)
				want := ErrContextRequired
				if scenario == "malformed" || scenario == "oversize" {
					want = ErrResourceExceeded
				}
				bacnetRPReject(t, e, want)
			})
		}
	}
}

func TestBACnetReadPropertyPrimitiveValues(t *testing.T) {
	for _, c := range []struct {
		name  string
		wire  []byte
		value any
	}{
		{"null", []byte{0}, nil}, {"false", []byte{0x10}, false}, {"true", []byte{0x11}, true},
		{"unsigned-max", []byte{0x24, 255, 255, 255, 255}, uint64(0xffffffff)},
		{"unsigned64-max", []byte{0x25, 8, 255, 255, 255, 255, 255, 255, 255, 255}, ^uint64(0)},
		{"signed-negative", []byte{0x31, 255}, int64(-1)},
		{"signed-min", []byte{0x34, 128, 0, 0, 0}, int64(-2147483648)},
		{"real", []byte{0x44, 0x3f, 0x80, 0, 0}, float64(1)},
		{"double", []byte{0x55, 8, 0x3f, 0xf0, 0, 0, 0, 0, 0, 0}, float64(1)},
		{"octets", []byte{0x63, 0, 128, 255}, []byte{0, 128, 255}},
		{"characters-encoding-preserved", []byte{0x73, 5, 128, 255}, []byte{128, 255}},
		{"bits", []byte{0x82, 3, 0xa8}, []byte{0xa8}},
		{"enumerated", []byte{0x92, 1, 2}, uint32(258)},
		{"date-special", []byte{0xa4, 255, 13, 32, 255}, []byte{255, 13, 32, 255}},
		{"time-wildcard", []byte{0xb4, 255, 255, 255, 255}, []byte{255, 255, 255, 255}},
	} {
		t.Run(c.name, func(t *testing.T) {
			v, n, err := bacnetApplicationValue(c.wire)
			require.NoError(t, err)
			require.Equal(t, len(c.wire), n)
			require.Equal(t, c.value, v["Value"])
			require.Equal(t, c.wire, v["Encoded"])
		})
	}
	for _, w := range [][]byte{{0x01, 0}, {0x12}, {0x20}, {0x25, 255, 255, 255, 255, 255}, {0x35, 9, 0, 0, 0, 0, 0, 0, 0, 0, 0}, {0x43, 0, 0, 0}, {0x50}, {0x70}, {0x82, 8, 0}, {0x82, 1, 1}, {0xa3, 0, 0, 0}, {0xb5, 5, 0, 0, 0, 0, 0}, {0xc3, 0, 0, 0}, {0x3e}, {0xf0}} {
		_, _, err := bacnetApplicationValue(w)
		require.Error(t, err, "%x", w)
	}
}

func TestBACnetReadPropertyBudgetAndClose(t *testing.T) {
	for _, kind := range []string{"memory", "fields", "depth", "identity-count"} {
		t.Run(kind, func(t *testing.T) {
			s := bacnetRPSession(t, false)
			cs := s.(*captureSession)
			a := cs.f.a
			if kind == "memory" {
				a.config.MaxBufferedBytes = 10000
			}
			if kind == "fields" {
				a.budget.MaxCollectionElements = 16
			}
			if kind == "depth" {
				a.budget.MaxRecursionDepth = 1
			}
			if kind == "identity-count" {
				a.budget.MaxCollectionElements = 32
				for i := 0; i < 32; i++ {
					e := bacnetRPFeed(t, s, 0, int64(1+i), bacnetRPWire(byte(i), false, nil))
					require.Empty(t, e.Error)
				}
			}
			invoke := byte(100)
			before := s.Stats()
			probe := s.Probe(bacnetRPWire(invoke, false, nil))
			if kind == "identity-count" {
				require.Equal(t, ProbeAccept, probe.Verdict, "wire probe does not establish association eligibility")
			} else {
				require.Equal(t, ProbeReject, probe.Verdict)
			}
			require.Equal(t, before, s.Stats())
			e := bacnetRPFeed(t, s, 0, 100, bacnetRPWire(invoke, false, nil))
			bacnetRPReject(t, e, ErrResourceExceeded)
			// Increasing the caller budget cannot manufacture forgotten on-wire
			// history or associate a reply to the refused request.
			a.config.MaxBufferedBytes = 32 << 20
			a.budget.MaxCollectionElements = 4096
			a.budget.MaxRecursionDepth = 64
			bacnetRPReject(t, bacnetRPFeed(t, s, 1, 101, bacnetRPWire(invoke, true, []byte{0})), ErrResourceExceeded)
			s.Close("complete")
			require.Zero(t, a.buffered.Load())
			require.Nil(t, a.bacnetReadSessions)
			require.Empty(t, s.Close("again"))
			require.Empty(t, s.Feed(0, time.Unix(200, 0), bacnetRPWire(1, false, nil)).Events)
		})
	}
}
func TestBACnetReadPropertyDomainAndClockIsolation(t *testing.T) {
	s := bacnetRPSession(t, false)
	a := s.(*captureSession).f.a
	q := bacnetRPWire(36, false, nil)
	ack := bacnetRPWire(36, true, []byte{0})
	first := &ProtocolEvent{ID: 1, Source: "192.0.2.1:40000", Destination: "192.0.2.2:47808", Transport: "udp", Timestamp: time.Unix(1000, 0)}
	a.decodeBACnetReadDatagram(first, q)
	require.Empty(t, first.Error)
	for i, domain := range []CaptureDomain{{Interface: 1}, {Section: 1}, {Encapsulation: "vxlan:1"}} {
		other := *first
		other.ID = uint64(2 + i*2)
		other.Timestamp = time.Unix(1, 0)
		other.Domain = domain
		other.Fields = nil
		other.Structured = nil
		other.Session = nil
		other.semanticFields = nil
		other.semanticSession = nil
		a.decodeBACnetReadDatagram(&other, q)
		require.Empty(t, other.Error)
		require.NotEqual(t, first.FlowID, other.FlowID)
		r := other
		r.ID = other.ID + 1
		r.Source, r.Destination = other.Destination, other.Source
		r.Timestamp = time.Unix(2, 0)
		a.decodeBACnetReadDatagram(&r, ack)
		require.Empty(t, r.Error)
		require.Equal(t, other.ID, r.ResponseTo)
	}
	r2 := *first
	r2.ID = 8
	r2.Source, r2.Destination = first.Destination, first.Source
	r2.Timestamp = time.Unix(1001, 0)
	a.decodeBACnetReadDatagram(&r2, ack)
	require.Empty(t, r2.Error)
	require.Equal(t, first.ID, r2.ResponseTo)
}
func TestBACnetReadPropertyTruncatedCarrierQuarantine(t *testing.T) {
	q, ack := bacnetRPWire(36, false, nil), bacnetRPWire(36, true, []byte{0})
	raw := sessionDatagramPCAP(t, []sessionStep{{0, q}, {1, ack}, {1, ack}}, 47808)
	// Second record's UDP declared length is one octet longer than captured.
	second := 24 + 16 + int(binary.LittleEndian.Uint32(raw[32:36]))
	udpLength := second + 16 + 14 + 20 + 4
	binary.BigEndian.PutUint16(raw[udpLength:udpLength+2], uint16(len(ack)+9))
	var events []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithProtocolDecodeAs("udp", 47808, "bacnet-read-property"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 3)
	require.Empty(t, events[0].Error)
	bacnetRPReject(t, events[1], ErrNeedMore)
	bacnetRPReject(t, events[2], ErrResourceExceeded)
}
func TestBACnetReadPropertyOptionsAndDatagramBoundary(t *testing.T) {
	_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionBACnetReadProperty(), WithSessionTransport("tcp"))
	require.Error(t, err)
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionBACnetReadProperty(), WithSessionTransport("udp"), WithSessionPorts(40000, 47808))
	require.NoError(t, err)
	defer s.Close("done")
	q := bacnetRPWire(1, false, nil)
	first := s.Feed(0, time.Unix(1, 0), q[:5])
	require.Len(t, first.Events, 1)
	bacnetRPReject(t, first.Events[0], ErrMalformedMessage)
	second := s.Feed(0, time.Unix(2, 0), q[5:])
	require.Len(t, second.Events, 1)
	require.Zero(t, second.Events[0].ResponseTo)
	require.Error(t, second.Err)
}

func TestBACnetUnconfirmedDoesNotPoisonReadExistingAPI(t *testing.T) {
	q, ack := bacnetRPWire(36, false, nil), bacnetRPWire(36, true, []byte{0})
	whois := bacnetTestPacket(10, []byte{1, 0, 0x10, 8})
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprintf("mixed=%v", mixed), func(t *testing.T) {
			steps := []sessionStep{{0, q}, {1, ack}}
			if mixed {
				steps = append([]sessionStep{{0, whois}}, steps...)
			}
			var events []*ProtocolEvent
			require.NoError(t, ReplayPcap(bytes.NewReader(sessionDatagramPCAP(t, steps, 47808)), WithProtocolDecodeAs("udp", 47808, "bacnet-read-property"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
			require.Len(t, events, len(steps))
			req := events[len(events)-2]
			resp := events[len(events)-1]
			f, err := req.GetFields()
			require.NoError(t, err)
			require.Equal(t, uint32(85), f["Property Identifier"])
			f, err = resp.GetFields()
			require.NoError(t, err)
			require.Equal(t, uint32(85), f["Property Identifier"])
			require.Equal(t, req.ID, resp.ResponseTo)
		})
	}
}
