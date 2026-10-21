package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Own CRC-valid type3 frames split a confirmed LN GET-normal descriptor.
// This test uses only the existing public session API on the old baseline.
func TestDLMSHDLCRequestSegmentsExistingAPI(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		t.Run(transport, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("cleanup")
			wires := []string{"7ea8100321103c79e6e600c001c1002ce37e", "7ea01203211278390f0000280000ff020087c87e", "7ea013210350d5e2e6e700c401c10012002a5b3f7e"}
			var events []*ProtocolEvent
			for i, wire := range wires {
				dir := 0
				if i == 2 {
					dir = 1
				}
				out := s.Feed(dir, time.Unix(int64(i+1), 0), wrapperWire(t, wire))
				require.Len(t, out.Events, 1, "selected initial request segmentation must retain each carrier observation")
				fields, err := out.Events[0].GetFields()
				require.NoError(t, err)
				require.NotNil(t, fields)
				events = append(events, out.Events...)
			}
			require.Zero(t, events[0].ResponseTo)
			require.Zero(t, events[0].TransactionID)
			require.Zero(t, events[1].ResponseTo)
			require.Equal(t, events[1].ID, events[2].ResponseTo, "response references complete request, never partial fragment")
			f, err := events[1].GetFields()
			require.NoError(t, err)
			require.EqualValues(t, 15, f["Class ID"])
			require.Equal(t, "0.0.40.0.0.255", f["Logical Name"])
			require.EqualValues(t, 2, f["Attribute ID"])
			require.EqualValues(t, 1, f["Send Sequence"], "captured final header remains literal")
			require.Equal(t, wrapperWire(t, wires[1]), events[1].Raw)
			f, err = events[2].GetFields()
			require.NoError(t, err)
			require.EqualValues(t, 42, f["Data Value"])
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func dlmsRequestSegmentControls(t *testing.T) []dlmsListControl {
	return dlmsNamedBlockControls(t, "dlms-request-segments", "owned-dlms-request-segments/v1", 37)
}

func TestDLMSUntrackedRefusalExistingAPI(t *testing.T) {
	large := "7ea05803211073d0e6e600c001c1000f0000280000ff020101093c5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5ae0ff7e"
	for _, what := range []string{"association", "bytes", "ingress-association", "ingress-bytes"} {
		t.Run(what, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxMessageBytes, b.MaxFrameBytes, b.ProbeBytes = 128, 64, 64
			if what == "ingress-association" || what == "ingress-bytes" {
				b.MaxMessageBytes = 64
			}
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("cleanup")
			out := s.Feed(0, time.Unix(1, 0), wrapperWire(t, large))
			rocTypedError(t, "ResourceExceeded", out.Err)
			require.Len(t, out.Events, 1)
			require.Empty(t, out.Events[0].Raw)
			if what == "bytes" || what == "ingress-bytes" {
				require.EqualValues(t, 90, s.Stats().LimitedBytes, "unretained input still belongs in the refused byte ledger")
				return
			}
			out = s.Feed(0, time.Unix(2, 0), wrapperWire(t, "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e"))
			require.Len(t, out.Events, 1)
			request := out.Events[0]
			out = s.Feed(1, time.Unix(3, 0), wrapperWire(t, "7ea013210330d381e6e700c401c10012002a5b3f7e"))
			require.Len(t, out.Events, 1)
			require.Zero(t, out.Events[0].ResponseTo, "late reply must not bind to a request after an untracked refusal")
			require.Equal(t, "unrecognized", out.Events[0].Status, "automatic admission must not label an unsolicited reply after the key was unretained")
			require.Empty(t, out.Events[0].Protocol)
			require.Nil(t, out.Events[0].Session)
			_, err = request.GetFields()
			rocTypedError(t, "ContextRequired", err)
			require.Zero(t, request.ResponseTo)
			require.Zero(t, out.Events[0].TransactionID)
			require.Empty(t, s.Close("complete"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func testDLMSRequestSegmentsCloseAndBudget(t *testing.T) {
	var normal dlmsListControl
	for _, c := range dlmsRequestSegmentControls(t) {
		if c.ID == "normal-two-udp" {
			normal = c
		}
	}
	require.NotEmpty(t, normal.ID)
	for _, transport := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("close/%s/deferred%t", transport, deferred), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				a := s.(*captureSession).f.a
				a.config.Deferred = deferred
				if transport == "udp" {
					callback := a.config.OnEvent
					a.config.OnEvent = func(e *ProtocolEvent) {
						if e.Completeness == "incomplete" {
							unlocked := a.udpMu.TryLock()
							require.True(t, unlocked)
							if unlocked {
								a.udpMu.Unlock()
							}
						}
						callback(e)
					}
				}
				wire := wrapperWire(t, normal.Events[0].Raw)
				out := s.Feed(0, time.Unix(1, 0), wire)
				clear(wire)
				require.Len(t, out.Events, 1)
				fields, err := out.Events[0].GetFields()
				require.NoError(t, err)
				rocEqualFields(t, normal.Events[0].Fields, fields)
				fields["HDLC Segmentation"].(map[string]any)["information_hex"] = "mutated"
				out.Events[0].Session["HDLC Segmentation"].(map[string]any)["information_hex"] = "public-mutated"
				fields, err = out.Events[0].GetFields()
				require.NoError(t, err)
				rocEqualFields(t, normal.Events[0].Fields, fields)
				closed := s.Close("incomplete")
				require.Len(t, closed, 1)
				require.Equal(t, "incomplete", closed[0].Completeness)
				require.Zero(t, closed[0].Direction)
				require.Equal(t, out.Events[0].FlowID, closed[0].FlowID)
				require.Zero(t, closed[0].ResponseTo)
				require.Zero(t, closed[0].TransactionID)
				rocEqualFields(t, map[string]any{"Outstanding": float64(1), "ObservedFrames": float64(1), "InformationBytes": float64(7)}, closed[0].Session)
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
			for _, limit := range []int{15, 16} {
				t.Run(fmt.Sprintf("collection/%s/deferred%t/limit%d", transport, deferred, limit), func(t *testing.T) {
					b := DefaultParserBudget()
					b.MaxCollectionElements = limit
					s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(transport), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					for _, w := range normal.Events {
						wire := bytes.Clone(wrapperWire(t, w.Raw))
						out := s.Feed(w.Direction, time.Unix(1, 0), wire)
						clear(wire)
						events = append(events, out.Events...)
					}
					if limit == 15 && transport == "tcp" {
						require.Len(t, events, 2)
						require.EqualValues(t, len(wrapperWire(t, normal.Events[2].Raw)), s.Stats().UnclassifiedBytes)
					} else {
						require.Len(t, events, 3)
					}
					if limit == 16 {
						dlmsListAssert(t, normal, events)
					} else {
						_, err := events[1].GetFields()
						rocTypedError(t, "ResourceExceeded", err)
						require.Nil(t, events[1].Session)
						require.Zero(t, events[1].ResponseTo)
						if transport == "udp" {
							_, err = events[2].GetFields()
							rocTypedError(t, "ContextRequired", err)
							require.Zero(t, events[2].ResponseTo)
						}
					}
					require.Empty(t, s.Close("complete"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}

// A forgotten refused key cannot verify a new generation. Retained independent
// keys and a separate capture still have their own complete observation history.
func testDLMSUntrackedRefusalIsolation(t *testing.T) {
	q := wrapperWire(t, "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e")
	r := wrapperWire(t, "7ea013210330d381e6e700c401c10012002a5b3f7e")
	for _, refusal := range []string{"frame", "shared-bytes", "slots", "ingress"} {
		t.Run(refusal, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxMessageBytes, b.MaxFrameBytes = 128, 64
			if refusal == "slots" {
				b.MaxCollectionElements = 1
			}
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"))
			require.NoError(t, err)
			defer s.Close("cleanup")
			a := s.(*captureSession).f.a
			feed := func(src, dst string, w []byte) *ProtocolEvent {
				e := &ProtocolEvent{Source: src, Destination: dst, Domain: CaptureDomain{Section: 1, Interface: 0}, Transport: "udp", Timestamp: time.Unix(1, 0)}
				require.True(t, a.decodeDLMSDatagram(e, w, true))
				return e
			}
			retained := feed("a:40000", "b:4059", q)
			require.Nil(t, retained.sessionError)
			deniedWire := q
			if refusal == "frame" || refusal == "ingress" {
				deniedWire = bytes.Repeat([]byte{0}, 90)
			}
			previous := a.config.MaxBufferedBytes
			if refusal == "shared-bytes" {
				a.config.MaxBufferedBytes = int(a.buffered.Load())
			}
			if refusal == "ingress" {
				e := &ProtocolEvent{Source: "c:40000", Destination: "d:4059", Transport: "udp", Timestamp: time.Unix(1, 0)}
				a.datagramDecodeAs = map[uint16]string{4059: "dlms"}
				require.True(t, a.refuseUDPOversizeAssociation(e, deniedWire, 40000, 4059))
				rocTypedError(t, "ResourceExceeded", e.sessionError)
			} else {
				e := feed("c:40000", "d:4059", deniedWire)
				rocTypedError(t, "ResourceExceeded", e.sessionError)
			}
			a.config.MaxBufferedBytes = previous
			require.True(t, a.dlmsUntrackedRefusal)
			require.Len(t, a.udpSessions.entries, 1)
			reply := feed("b:4059", "a:40000", r)
			require.Nil(t, reply.sessionError)
			require.Equal(t, retained.ID, reply.ResponseTo)
			require.Equal(t, retained.FlowID, reply.FlowID)
			if refusal == "slots" {
				a.budget.MaxCollectionElements = 2
			}
			newRequest := feed("c:40000", "d:4059", q)
			rocTypedError(t, "ContextRequired", newRequest.sessionError)
			require.Zero(t, newRequest.ResponseTo)
			late := feed("d:4059", "c:40000", r)
			rocTypedError(t, "ContextRequired", late.sessionError)
			require.Zero(t, late.ResponseTo)
			require.Zero(t, late.TransactionID)
			require.Len(t, a.udpSessions.entries, 1)
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, err)
	request := s.Feed(0, time.Unix(1, 0), q)
	require.Len(t, request.Events, 1)
	require.Nil(t, request.Err)
	response := s.Feed(1, time.Unix(1, 0), r)
	require.Len(t, response.Events, 1)
	require.Nil(t, response.Err)
	require.Equal(t, request.Events[0].ID, response.Events[0].ResponseTo)
	require.Empty(t, s.Close("normal"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func testDLMSRequestSegmentsDomainsAndBytes(t *testing.T) {
	var normal, bad dlmsListControl
	for _, c := range dlmsRequestSegmentControls(t) {
		if c.ID == "normal-two-udp" {
			normal = c
		}
		if c.ID == "sequence-gap-retires-udp" {
			bad = c
		}
	}
	require.NotEmpty(t, normal.ID)
	require.NotEmpty(t, bad.ID)
	t.Run("domains", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"))
		require.NoError(t, err)
		defer s.Close("cleanup")
		a := s.(*captureSession).f.a
		domains := []CaptureDomain{{Section: 1, Interface: 0}, {Section: 1, Interface: 1}, {Section: 2, Interface: 0}}
		byDomain := make([][]*ProtocolEvent, 3)
		for i := range normal.Events {
			for d, domain := range domains {
				w := normal.Events[i]
				if d == 0 {
					w = bad.Events[i]
				}
				src, dst := "192.0.2.1:40000", "192.0.2.2:4059"
				if w.Direction == 1 {
					src, dst = dst, src
				}
				e := &ProtocolEvent{Source: src, Destination: dst, Domain: domain, Transport: "udp", Timestamp: time.Unix(1, 0)}
				require.True(t, a.decodeDLMSDatagram(e, wrapperWire(t, w.Raw), true))
				byDomain[d] = append(byDomain[d], e)
			}
		}
		bad.Events = bad.Events[:len(normal.Events)]
		dlmsListAssert(t, bad, byDomain[0])
		for d := 1; d < 3; d++ {
			dlmsListAssert(t, normal, byDomain[d])
			require.NotEqual(t, byDomain[0][0].FlowID, byDomain[d][0].FlowID)
			for _, e := range byDomain[d] {
				require.Equal(t, domains[d], e.Domain)
			}
		}
		require.NotEqual(t, byDomain[1][0].FlowID, byDomain[2][0].FlowID)
		require.Empty(t, s.Close("domains"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	first, last := wrapperWire(t, normal.Events[0].Raw), wrapperWire(t, normal.Events[1].Raw)
	// Fixed caller ledger includes the partial information and literal wire. A
	// one-byte-short cap rejects before allocating the final projection.
	for _, delta := range []int{-1, 0} {
		t.Run(fmt.Sprintf("shared-bytes/%d", delta), func(t *testing.T) {
			b := DefaultParserBudget()
			f := &binFlow{dlms: &binDLMS{fragments: &dlmsFragments{}}, a: &binParser{budget: b}}
			b.MaxBufferedBytes = int(512+128+2*(7+len(first))+int(f.dlmsProjection(last))) + delta
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("cleanup")
			var ev []*ProtocolEvent
			for _, w := range normal.Events {
				out := s.Feed(w.Direction, time.Unix(1, 0), wrapperWire(t, w.Raw))
				require.Len(t, out.Events, 1)
				ev = append(ev, out.Events...)
			}
			if delta == 0 {
				dlmsListAssert(t, normal, ev)
			} else {
				_, err := ev[1].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				_, err = ev[2].GetFields()
				rocTypedError(t, "ContextRequired", err)
				require.Zero(t, ev[2].ResponseTo)
			}
			require.Empty(t, s.Close("complete"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

// Original capture and UnsupportedFeature answer stay immutable. This additive
// hash-bound contract preserves partial observation and requires exact Close
// diagnostic fields rather than pretending the segmented APDU is complete.
func testDLMSHistoricalInitialSegment(t *testing.T, c bsapControl, raw []byte, old bsapAnswer) {
	var want struct {
		CaptureSHA      string         `json:"capture_sha256"`
		AnswerSHA       string         `json:"historical_answer_sha256"`
		HistoricalError string         `json:"historical_error"`
		Wire            string         `json:"wire_hex"`
		Partial         map[string]any `json:"partial_fields"`
		Closed          map[string]any `json:"close_fields"`
	}
	b, err := trafficfixture.ReadFile("dlms-request-segments/historical-segmented-upgrade.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &want))
	require.Equal(t, c.SHA256, want.CaptureSHA)
	require.Equal(t, c.AnswerSHA, want.AnswerSHA)
	require.Len(t, old.Events, 1)
	require.Equal(t, "UnsupportedFeature", want.HistoricalError)
	require.Equal(t, want.HistoricalError, old.Events[0].Error)
	require.Equal(t, old.Events[0].Raw, want.Wire)
	discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
		ev, stats := discoveryReplay(t, raw, workers, deferred, observe, WithProtocolDecodeAs("udp", 4059, "dlms"))
		require.Len(t, ev, 2)
		first, last := ev[0], ev[1]
		require.Equal(t, wrapperWire(t, want.Wire), first.Raw)
		require.Equal(t, "dlms", first.Protocol)
		require.Equal(t, "dlms-hdlc-get-normal", first.Profile)
		require.Equal(t, "explicit-decode-as", first.Admission)
		require.Equal(t, "message", first.Completeness)
		fields, err := first.GetFields()
		require.NoError(t, err)
		rocEqualFields(t, want.Partial, fields)
		rocEqualFields(t, want.Partial, first.Session)
		require.NotZero(t, first.ID)
		require.NotEqual(t, first.ID, last.ID)
		require.NotZero(t, first.FlowID)
		require.Equal(t, first.FlowID, last.FlowID)
		require.Equal(t, first.Domain, last.Domain)
		require.Equal(t, "dlms-hdlc-segmented-request", last.Profile)
		require.Equal(t, "incomplete", last.Status)
		require.Equal(t, "incomplete", last.Completeness)
		rocEqualFields(t, want.Closed, last.Session)
		require.Empty(t, last.Raw)
		require.Empty(t, last.Error)
		require.Empty(t, last.SourceBytes.PacketRefs)
		require.Equal(t, "192.0.2.10:39200", first.Source)
		require.Equal(t, "192.0.2.20:4059", first.Destination)
		require.Equal(t, first.Source, last.Source)
		require.Equal(t, first.Destination, last.Destination)
		require.Len(t, first.SourceBytes.PacketRefs, 1)
		require.EqualValues(t, 1, first.SourceBytes.PacketRefs[0].Number)
		for _, e := range ev {
			require.Equal(t, "udp", e.Transport)
			require.Equal(t, "dlms", e.Protocol)
			require.Zero(t, e.Direction)
			require.Zero(t, e.ResponseTo)
			require.Zero(t, e.TransactionID)
		}
		require.Zero(t, stats.BufferedBytes)
		require.Zero(t, stats.CallbackPanics)
		require.EqualValues(t, 1, stats.Incomplete)
	})
}

func testDLMSRefusalHintNeighbors(t *testing.T) {
	var raw []byte
	for _, c := range dlmsRequestSegmentControls(t) {
		if c.ID == "first-ingress-refusal-udp" {
			raw = wrapperWire(t, c.Events[0].Raw)
		}
	}
	require.Len(t, raw, 90)
	for _, neighbor := range []string{"invalid-hcs", "off-port"} {
		t.Run(neighbor, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxMessageBytes, b.MaxFrameBytes, b.ProbeBytes = 64, 64, 64
			port := uint16(4059)
			wire := bytes.Clone(raw)
			if neighbor == "invalid-hcs" {
				wire[6] ^= 1
			} else {
				port = 4058
			}
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, port))
			require.NoError(t, err)
			defer s.Close("cleanup")
			out := s.Feed(0, time.Unix(1, 0), wire)
			require.Len(t, out.Events, 1)
			require.Equal(t, "limited", out.Events[0].Status)
			require.Empty(t, out.Events[0].Protocol)
			require.False(t, s.(*captureSession).f.a.dlmsUntrackedRefusal, "a malformed header/non-DLMS port is not admitted HDLC refusal context")
			require.EqualValues(t, 90, s.Stats().LimitedBytes)
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e"))
			require.Len(t, q.Events, 1)
			fields, err := q.Events[0].GetFields()
			require.NoError(t, err)
			require.EqualValues(t, 15, fields["Class ID"])
			require.Equal(t, "0.0.40.0.0.255", fields["Logical Name"])
			require.EqualValues(t, 2, fields["Attribute ID"])
			r := s.Feed(1, time.Unix(1, 0), wrapperWire(t, "7ea013210330d381e6e700c401c10012002a5b3f7e"))
			require.Len(t, r.Events, 1)
			fields, err = r.Events[0].GetFields()
			require.NoError(t, err)
			require.EqualValues(t, 42, fields["Data Value"])
			require.Equal(t, q.Events[0].ID, r.Events[0].ResponseTo)
			require.Zero(t, r.Events[0].TransactionID)
			require.Empty(t, s.Close("normal"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
