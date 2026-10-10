package pcaputil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// These independently constructed type3 frames split LLC+GET-normal response
// before its uint16 Data. No new API is needed to demonstrate missing behavior.
func TestDLMSHDLCSegmentsExistingAPI(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		t.Run(transport, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("cleanup")
			wires := []string{"7ea0190321107fdae6e600c001c1000f0000280000ff020091537e", "7ea8102103303efee6e700c401c100eb957e", "7ea00c21033299d312002a02717e"}
			var events []*ProtocolEvent
			for i, wire := range wires {
				out := s.Feed(min(i, 1), time.Unix(int64(i+1), 0), wrapperWire(t, wire))
				require.Len(t, out.Events, 1)
				fields, err := out.Events[0].GetFields()
				require.NoError(t, err, "bounded observed response segmentation must use existing public session API")
				require.NotNil(t, fields)
				events = append(events, out.Events...)
			}
			require.Zero(t, events[1].ResponseTo, "a partial APDU is not a complete response")
			require.Equal(t, events[0].ID, events[2].ResponseTo)
			require.Zero(t, events[2].TransactionID)
			f, err := events[2].GetFields()
			require.NoError(t, err)
			require.EqualValues(t, 42, f["Data Value"])
			require.EqualValues(t, 1, f["Send Sequence"], "literal final frame sequence, not reconstructed header")
			require.Equal(t, events[2].Raw, wrapperWire(t, wires[2]))
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func dlmsSegmentControls(t *testing.T) []dlmsListControl {
	return dlmsNamedBlockControls(t, "dlms-hdlc-segments", "owned-dlms-hdlc-segments/v1", 56)
}

func TestDLMSHDLCSegmentsSealedMatrix(t *testing.T) { dlmsSealedMatrix(t, dlmsSegmentControls(t)) }
func TestDLMSHDLCSegmentsOwnershipAndChunks(t *testing.T) {
	controls := dlmsSegmentControls(t)
	dlmsOwnershipAndChunks(t, controls)
	for _, c := range controls {
		if c.ID != "normal-two-tcp" && c.ID != "normal-two-udp" {
			continue
		}
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(c.Transport), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		defer s.Close("cleanup")
		for i, w := range c.Events {
			out := s.Feed(w.Direction, time.Unix(1, 0), wrapperWire(t, w.Raw))
			require.Len(t, out.Events, 1)
			if i == 0 {
				continue
			}
			e := out.Events[0]
			fields, err := e.GetFields()
			require.NoError(t, err)
			fields["HDLC Segmentation"].(map[string]any)["information_hex"] = "returned-map-mutated"
			e.Session["HDLC Segmentation"].(map[string]any)["information_hex"] = "public-session-mutated"
			fields, err = e.GetFields()
			require.NoError(t, err)
			rocEqualFields(t, w.Fields, fields)
		}
		require.Empty(t, s.Close("complete"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func TestDLMSHDLCSegmentsCloseAndBudget(t *testing.T) {
	var normal, large dlmsListControl
	for _, c := range dlmsSegmentControls(t) {
		if c.ID == "normal-two-udp" {
			normal = c
		} else if c.ID == "information-bytes-1027-udp" {
			large = c
		}
	}
	require.NotEmpty(t, normal.ID)
	require.NotEmpty(t, large.ID)
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
				var events []*ProtocolEvent
				for _, w := range normal.Events[:2] {
					out := s.Feed(w.Direction, time.Unix(1, 0), wrapperWire(t, w.Raw))
					require.Len(t, out.Events, 1)
					events = append(events, out.Events...)
				}
				prefix := normal
				prefix.Events = normal.Events[:2]
				dlmsListAssert(t, prefix, events)
				closed := s.Close("incomplete")
				require.Len(t, closed, 1)
				require.Equal(t, "incomplete", closed[0].Completeness)
				require.Equal(t, "dlms", closed[0].Protocol)
				require.Equal(t, transport, closed[0].Transport)
				require.Equal(t, events[0].FlowID, closed[0].FlowID)
				require.Equal(t, events[0].Source, closed[0].Source)
				require.Equal(t, events[0].Destination, closed[0].Destination)
				require.Zero(t, closed[0].ResponseTo)
				require.Zero(t, closed[0].TransactionID)
				rocEqualFields(t, map[string]any{"Outstanding": float64(1), "ObservedFrames": float64(1), "InformationBytes": float64(7)}, closed[0].Session)
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
			for _, limit := range []int{9, 10} {
				t.Run(fmt.Sprintf("caller/%s/deferred%t/limit%d", transport, deferred, limit), func(t *testing.T) {
					b := DefaultParserBudget()
					b.MaxCollectionElements = limit
					s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(transport), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					for _, w := range normal.Events {
						out := s.Feed(w.Direction, time.Unix(1, 0), wrapperWire(t, w.Raw))
						require.Len(t, out.Events, 1)
						events = append(events, out.Events...)
					}
					if limit == 10 {
						dlmsListAssert(t, normal, events)
					} else {
						prefix := normal
						prefix.Events = normal.Events[:2]
						dlmsListAssert(t, prefix, events[:2])
						f, err := events[2].GetFields()
						rocTypedError(t, "ResourceExceeded", err)
						require.Nil(t, f)
						require.Nil(t, events[2].Session)
						require.Zero(t, events[2].ResponseTo)
					}
					require.Empty(t, s.Close("complete"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
	q, first, last := wrapperWire(t, large.Events[0].Raw), wrapperWire(t, large.Events[1].Raw), wrapperWire(t, large.Events[2].Raw)
	for _, delta := range []int{-1, 0} {
		b := DefaultParserBudget()
		f := &binFlow{dlms: &binDLMS{fragments: &dlmsFragments{}}, a: &binParser{budget: b}}
		b.MaxBufferedBytes = int(512+2*len(q)+128+2*(500+len(first))) + int(f.dlmsProjection(last)) + delta
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, err)
		var events []*ProtocolEvent
		for _, w := range large.Events {
			out := s.Feed(w.Direction, time.Unix(1, 0), wrapperWire(t, w.Raw))
			require.Len(t, out.Events, 1)
			events = append(events, out.Events...)
		}
		if delta == 0 {
			dlmsListAssert(t, large, events)
		} else {
			prefix := large
			prefix.Events = large.Events[:2]
			dlmsListAssert(t, prefix, events[:2])
			_, err := events[2].GetFields()
			rocTypedError(t, "ResourceExceeded", err)
			require.Nil(t, events[2].Session)
			require.Zero(t, events[2].ResponseTo)
		}
		require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
		require.Empty(t, s.Close("budget"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func TestDLMSHDLCSegmentsDomainIsolation(t *testing.T) {
	var normal, bad dlmsListControl
	for _, c := range dlmsSegmentControls(t) {
		if c.ID == "normal-two-udp" {
			normal = c
		} else if c.ID == "sequence-gap-2-udp" {
			bad = c
		}
	}
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
	bad.Events = bad.Events[:3]
	dlmsListAssert(t, bad, byDomain[0])
	for d := 1; d < 3; d++ {
		dlmsListAssert(t, normal, byDomain[d])
		require.NotEqual(t, byDomain[0][0].FlowID, byDomain[d][0].FlowID)
		for _, e := range byDomain[d] {
			require.Equal(t, domains[d], e.Domain)
		}
	}
	require.NotEqual(t, byDomain[1][0].FlowID, byDomain[2][0].FlowID)
	require.Empty(t, s.Close("domain"))
	require.Zero(t, s.Stats().BufferedBytes)
}
