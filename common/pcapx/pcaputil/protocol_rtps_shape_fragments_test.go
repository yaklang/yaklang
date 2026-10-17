package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func rtpsShapeFragmentCases(t *testing.T) []rtpsControl {
	t.Helper()
	if root := os.Getenv("RTPS_SHAPE_FRAGMENT_TEST_ROOT"); root != "" {
		raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
		require.NoError(t, err)
		var m struct{ Cases []rtpsControl }
		require.NoError(t, json.Unmarshal(raw, &m))
		require.Len(t, m.Cases, 160)
		return m.Cases
	}
	return rtpsBatchControls(t, "rtps-shape-fragments", 160)
}
func rtpsShapeFragmentInput(t *testing.T, c rtpsControl) ([]byte, rtpsFragmentAnswer) {
	t.Helper()
	read := func(file string) []byte {
		var raw []byte
		var err error
		if root := os.Getenv("RTPS_SHAPE_FRAGMENT_TEST_ROOT"); root != "" {
			raw, err = os.ReadFile(filepath.Join(root, file))
		} else {
			raw, err = trafficfixture.ReadFile(rtpsControlPath(t, c, file))
		}
		require.NoError(t, err)
		return raw
	}
	raw, ans := read(c.Capture), read(c.Answer)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ans)))
	var want rtpsFragmentAnswer
	require.NoError(t, json.Unmarshal(ans, &want))
	require.Len(t, want.Wires, c.Packets)
	require.Len(t, want.Events, c.Packets)
	require.Len(t, want.Directions, c.Packets)
	require.Len(t, want.Seconds, c.Packets)
	r, err := NewCaptureReader(bytes.NewReader(raw))
	require.NoError(t, err)
	for i, wire := range want.Wires {
		b, ci, err := r.ReadPacketData()
		require.NoError(t, err)
		p := gopacket.NewPacket(b, r.LinkType(), gopacket.Default)
		u := p.Layer(layers.LayerTypeUDP).(*layers.UDP)
		require.Equal(t, wire, hex.EncodeToString(u.Payload))
		require.Equal(t, time.Unix(1700000000+int64(want.Seconds[i]), 0), ci.Timestamp)
	}
	_, _, err = r.ReadPacketData()
	require.ErrorIs(t, err, io.EOF)
	return raw, want
}
func newRTPSShapeFragmentSession(t *testing.T, b ParserBudget) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSShapeFragments(28400, rtpsShapeTestBinding()))
	require.NoError(t, err)
	return s
}
func rtpsShapeFragmentEvent(t *testing.T, e *ProtocolEvent, want map[string]any, kind string) {
	t.Helper()
	require.Equal(t, "rtps", e.Protocol)
	require.Equal(t, rtpsShapeFragmentProfile, e.Profile)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID)
	f, err := e.GetFields()
	if kind != "" {
		require.Nil(t, f)
		assertOwnedProtocolRefusal(t, e, kind, err)
		require.Nil(t, e.Session)
		return
	}
	require.NoError(t, err)
	rocEqualFields(t, want, f)
	rocEqualFields(t, want, e.Session)
	mutateRTPSQuery(f)
	mutateRTPSQuery(e.Session)
	testNativeObservationSessionQueries(t, e, want)
	e.Fields = map[string]any{"caller": true}
	e.Structured = map[string]any{"caller": true}
	clear(e.Raw)
	f, err = e.GetFields()
	require.NoError(t, err)
	rocEqualFields(t, want, f)
}
func TestRTPSShapeFragmentsSession(t *testing.T) {
	for _, c := range rtpsShapeFragmentCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, want := rtpsShapeFragmentInput(t, c)
			s := newRTPSShapeFragmentSession(t, DefaultParserBudget())
			for i, encoded := range want.Wires {
				wire, err := hex.DecodeString(encoded)
				require.NoError(t, err)
				before := s.Stats()
				s.Probe(wire)
				s.Probe(wire)
				require.Equal(t, before, s.Stats(), "Probe is stateless")
				r := s.Feed(want.Directions[i], time.Unix(1700000000+int64(want.Seconds[i]), 0), wire)
				clear(wire)
				require.Len(t, r.Events, 1)
				require.EqualValues(t, i+1, r.Events[0].ID)
				rtpsShapeFragmentEvent(t, r.Events[0], want.Events[i].Fields, want.Events[i].Error)
			}
			close := s.Close("fixture-end")
			require.Len(t, close, len(want.CloseIDs))
			for i, e := range close {
				require.Equal(t, want.CloseIDs[i], e.SourceBytes.ParentPDUs)
				rtpsShapeFragmentEvent(t, e, nil, "NeedMore")
			}
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			r := s.Feed(0, time.Unix(1700000099, 0), []byte("late"))
			require.Empty(t, r.Events)
			require.Zero(t, r.Consumed)
			rocTypedError(t, "FatalSessionError", r.Err)
		})
	}
}
func TestRTPSShapeFragmentsReplay(t *testing.T) {
	for _, c := range rtpsShapeFragmentCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, want := rtpsShapeFragmentInput(t, c)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%v/observer%v", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithProtocolRTPSShapeFragments(28400, rtpsShapeTestBinding()), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							require.Len(t, events, len(want.Events)+len(want.CloseIDs))
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observe {
								require.EqualValues(t, c.Packets, seen.Load())
							}
							for i, a := range want.Events {
								e := events[i]
								require.EqualValues(t, i+1, e.ID)
								require.Equal(t, "udp", e.Transport)
								src, dst := "192.0.2.10:38000", "192.0.2.20:28400"
								if want.Directions[i] == 1 {
									src, dst = dst, src
								}
								require.Equal(t, src, e.Source)
								require.Equal(t, dst, e.Destination)
								require.Equal(t, want.Wires[i], hex.EncodeToString(e.Raw))
								require.Empty(t, e.SourceBytes.ParentPDUs)
								require.Len(t, e.SourceBytes.PacketRefs, 1)
								require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
								wire, err := hex.DecodeString(want.Wires[i])
								require.NoError(t, err)
								require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wire)), e.SourceBytes.SHA256)
								rtpsShapeFragmentEvent(t, e, a.Fields, a.Error)
							}
							for i, ids := range want.CloseIDs {
								e := events[len(want.Events)+i]
								require.Equal(t, ids, e.SourceBytes.ParentPDUs)
								require.Equal(t, "incomplete", e.Status)
								require.Empty(t, e.Raw)
								require.Len(t, e.SourceBytes.PacketRefs, len(ids))
								for j, id := range ids {
									require.Equal(t, id, e.SourceBytes.PacketRefs[j].Number)
								}
								rtpsShapeFragmentEvent(t, e, nil, "NeedMore")
							}
						})
					}
				}
			}
		})
	}
}

func rtpsShapeFragmentCase(t *testing.T, id string) rtpsFragmentAnswer {
	t.Helper()
	for _, c := range rtpsShapeFragmentCases(t) {
		if c.ID == id {
			_, a := rtpsShapeFragmentInput(t, c)
			return a
		}
	}
	t.Fatalf("missing case %s", id)
	return rtpsFragmentAnswer{}
}
func TestRTPSShapeFragmentsBudgetsAndLifecycle(t *testing.T) {
	ordered := rtpsShapeFragmentCase(t, "le-ordered")
	qos := rtpsShapeFragmentCase(t, "le-qos-timestamp")
	bundle := rtpsShapeFragmentCase(t, "le-le-data-normal-bundle")
	wire, err := hex.DecodeString(bundle.Wires[0])
	require.NoError(t, err)
	// One submessage + four fragments + four ShapeType scalar/string nodes.
	for _, tc := range []struct {
		name                   string
		depth, elements, bytes int
		success                bool
	}{
		{"depth-low", 4, 4096, 1 << 20, false}, {"depth-exact", 5, 4096, 1 << 20, true},
		{"collection-low", 64, 8, 1 << 20, false}, {"collection-exact", 64, 9, 1 << 20, true},
		{"bytes-low", 64, 4096, len(wire) - 1, false}, {"bytes-exact", 64, 4096, len(wire), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxRecursionDepth, b.MaxCollectionElements, b.MaxFrameBytes, b.MaxMessageBytes = tc.depth, tc.elements, tc.bytes, tc.bytes
			s := newRTPSShapeFragmentSession(t, b)
			before := s.Stats()
			p := s.Probe(wire)
			require.Equal(t, before, s.Stats())
			verdict := ProbeReject
			if tc.success {
				verdict = ProbeAccept
			}
			require.Equal(t, verdict, p.Verdict)
			r := s.Feed(0, time.Unix(1, 0), wire)
			require.Len(t, r.Events, 1)
			if tc.success {
				rtpsShapeFragmentEvent(t, r.Events[0], bundle.Events[0].Fields, "")
			} else {
				rtpsShapeFragmentEvent(t, r.Events[0], nil, "ResourceExceeded")
			}
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
		})
	}
	t.Run("retained-qos-projection-boundary", func(t *testing.T) {
		// Second datagram has one header + three fragment indices + retained PAD
		// and two repeated parameters + four application fields:eleven elements.
		for _, limit := range []int{10, 11} {
			s := newRTPSShapeFragmentSession(t, ParserBudget{MaxCollectionElements: limit})
			for i, raw := range qos.Wires {
				w, err := hex.DecodeString(raw)
				require.NoError(t, err)
				r := s.Feed(0, time.Unix(int64(i), 0), w)
				require.Len(t, r.Events, 1)
				if i == 1 && limit == 10 {
					rtpsShapeFragmentEvent(t, r.Events[0], nil, "ResourceExceeded")
				} else {
					rtpsShapeFragmentEvent(t, r.Events[0], qos.Events[i].Fields, "")
				}
			}
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
		}
	})
	t.Run("refusal-quarantines-and-releases-sample", func(t *testing.T) {
		s := newRTPSShapeFragmentSession(t, DefaultParserBudget())
		a := s.(*captureSession).f.a
		first, _ := hex.DecodeString(ordered.Wires[0])
		last, _ := hex.DecodeString(ordered.Wires[1])
		r := s.Feed(0, time.Unix(1, 0), first)
		rtpsShapeFragmentEvent(t, r.Events[0], ordered.Events[0].Fields, "")
		active := s.Stats().BufferedBytes
		a.config.MaxBufferedBytes = int(active)
		r = s.Feed(0, time.Unix(2, 0), last)
		rtpsShapeFragmentEvent(t, r.Events[0], nil, "ResourceExceeded")
		require.Less(t, s.Stats().BufferedBytes, active)
		a.config.MaxBufferedBytes = DefaultParserBudget().MaxBufferedBytes
		r = s.Feed(0, time.Unix(3, 0), last)
		rtpsShapeFragmentEvent(t, r.Events[0], nil, "ContextRequired")
		require.Empty(t, s.Close("end"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("carrier-truncation-retirement", func(t *testing.T) {
		s := newRTPSShapeFragmentSession(t, DefaultParserBudget())
		a := s.(*captureSession).f.a
		first, _ := hex.DecodeString(ordered.Wires[0])
		last, _ := hex.DecodeString(ordered.Wires[1])
		r := s.Feed(0, time.Unix(1, 0), first)
		e := r.Events[0]
		refused := &ProtocolEvent{Source: e.Source, Destination: e.Destination, Domain: e.Domain, Timestamp: time.Unix(2, 0), Transport: "udp"}
		require.True(t, a.decodeRTPSSelectedFragmentDatagram(refused, first[:30], protocolError(ErrNeedMore, "capture truncated"), 38000, 28400))
		require.Empty(t, refused.Raw)
		rtpsShapeFragmentEvent(t, refused, nil, "NeedMore")
		r = s.Feed(0, time.Unix(3, 0), last)
		rtpsShapeFragmentEvent(t, r.Events[0], nil, "ContextRequired")
		require.Empty(t, s.Close("end"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("probe-retains-nothing-and-never-authenticates", func(t *testing.T) {
		s := newRTPSShapeFragmentSession(t, DefaultParserBudget())
		w, _ := hex.DecodeString(ordered.Wires[0])
		for i := 0; i < 3; i++ {
			require.Equal(t, ProbeAccept, s.Probe(w).Verdict)
			require.Nil(t, s.(*captureSession).f.a.rtpsShapeFragments)
			require.Zero(t, s.Stats().BufferedBytes)
		}
		r := s.Feed(2, time.Unix(1, 0), w)
		require.Empty(t, r.Events)
		require.Zero(t, r.Consumed)
		rocTypedError(t, "MalformedMessage", r.Err)
		require.Nil(t, s.(*captureSession).f.a.rtpsShapeFragments)
		s.Close("end")
	})
	t.Run("domain-section-interface-tunnel-isolation", func(t *testing.T) {
		domains := []CaptureDomain{{Section: 1, Interface: 2, Encapsulation: "vlan:7"}, {Section: 2, Interface: 2, Encapsulation: "vlan:7"}, {Section: 1, Interface: 3, Encapsulation: "vlan:7"}, {Section: 1, Interface: 2, Encapsulation: "vlan:8"}}
		s := newRTPSShapeFragmentSession(t, DefaultParserBudget())
		a := s.(*captureSession).f.a
		first, _ := hex.DecodeString(ordered.Wires[0])
		last, _ := hex.DecodeString(ordered.Wires[1])
		for i, domain := range domains {
			w := last
			if i == 0 {
				w = first
			}
			e := &ProtocolEvent{Source: "192.0.2.10:38000", Destination: "192.0.2.20:28400", Domain: domain, Timestamp: time.Unix(int64(i), 0), Transport: "udp", SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: uint64(i + 1), Domain: domain}}}}
			require.True(t, a.decodeRTPSSelectedFragmentDatagram(e, w, nil, 38000, 28400))
			want := ordered.Events[0].Fields
			if i > 0 {
				want = cloneSession(ordered.Events[1].Fields)
				m := want["Messages"].([]any)[0].(map[string]any)
				delete(m, "Application Type")
				delete(m, "Application Context")
				delete(m, "Application Fields")
				delete(m, "Reconstructed Payload")
				m["Assembly State"], m["Observed Fragment Count"], m["Contributor Event IDs"] = "pending", uint32(3), []uint64{uint64(i + 1)}
			}
			rtpsShapeFragmentEvent(t, e, want, "")
		}
		require.Len(t, a.rtpsShapeFragments.entries, len(domains))
		a.closeUDPSessions()
		require.Zero(t, a.buffered.Load())
	})
}
func TestRTPSShapeFragmentsSelectionAndOwnership(t *testing.T) {
	for _, opts := range [][]ProtocolSessionOption{{WithSessionRTPSShapeFragments(28400, rtpsShapeTestBinding())}, {WithSessionTransport("tcp"), WithSessionRTPSShapeFragments(28400, rtpsShapeTestBinding())}, {WithSessionTransport("udp"), WithSessionRTPSShapeFragments(0, rtpsShapeTestBinding())}, {WithSessionTransport("udp"), WithSessionRTPSShapeFragments(28400, RTPSShapeWriter{})}, {WithSessionTransport("udp"), WithSessionRTPSWire(28400), WithSessionRTPSShapeFragments(28400, rtpsShapeTestBinding())}} {
		_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.Error(t, err)
	}
	b := rtpsShapeTestBinding()
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSShapeFragments(28400, b))
	require.NoError(t, err)
	clear(b.SourceGUIDPrefix[:])
	clear(b.WriterID[:])
	bundle := rtpsShapeFragmentCase(t, "le-le-data-normal-bundle")
	wire, _ := hex.DecodeString(bundle.Wires[0])
	r := s.Feed(0, time.Unix(1, 0), wire)
	rtpsShapeFragmentEvent(t, r.Events[0], bundle.Events[0].Fields, "")
	s.Close("end")
	// A wire-only destination wins over a typed fragment source selector.
	s, err = NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSShapeFragments(38000, rtpsShapeTestBinding()), WithSessionRTPSWire(28400))
	require.NoError(t, err)
	wire, _ = hex.DecodeString(bundle.Wires[0])
	r = s.Feed(0, time.Unix(1, 0), wire)
	require.Equal(t, "rtps-wire-native", r.Events[0].Profile)
	_, err = r.Events[0].GetFields()
	rocTypedError(t, "UnsupportedFeature", err)
	require.Nil(t, s.(*captureSession).f.a.rtpsShapeFragments)
	s.Close("end")
	// Explicit DecodeAs without caller type context cannot guess the schema.
	var events []*ProtocolEvent
	raw, _ := rtpsShapeFragmentInput(t, rtpsShapeFragmentCases(t)[0])
	require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithProtocolDecodeAs("udp", 28400, "rtps-shape-fragments"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
	require.NotEmpty(t, events)
	for _, e := range events {
		rtpsShapeFragmentEvent(t, e, nil, "ContextRequired")
	}
}
