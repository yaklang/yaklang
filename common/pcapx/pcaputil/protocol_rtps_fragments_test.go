package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type rtpsFragmentAnswer struct {
	Wires  []string
	Events []struct {
		Fields map[string]any
		Error  string
	}
	Directions, Seconds []int
	CloseIDs            [][]uint64 `json:"close_ids"`
	Default             []struct {
		Fields map[string]any
		Error  string
	}
}

func rtpsFragmentControls(t *testing.T) []rtpsControl {
	t.Helper()
	if os.Getenv("RTPS_FRAGMENT_TEST_ROOT") == "" {
		return rtpsBatchControls(t, "rtps-fragments", 44)
	}
	b, err := os.ReadFile(filepath.Join(os.Getenv("RTPS_FRAGMENT_TEST_ROOT"), "manifest.json"))
	require.NoError(t, err)
	var m struct{ Cases []rtpsControl }
	require.NoError(t, json.Unmarshal(b, &m))
	require.Len(t, m.Cases, 44)
	return m.Cases
}
func rtpsFragmentInput(t *testing.T, c rtpsControl) ([]byte, rtpsFragmentAnswer) {
	t.Helper()
	read := func(file string) []byte {
		var b []byte
		var err error
		if root := os.Getenv("RTPS_FRAGMENT_TEST_ROOT"); root != "" {
			b, err = os.ReadFile(filepath.Join(root, file))
		} else {
			b, err = trafficfixture.ReadFile(rtpsControlPath(t, c, file))
		}
		require.NoError(t, err)
		return b
	}
	b := read(c.Capture)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
	raw := read(c.Answer)
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var a rtpsFragmentAnswer
	require.NoError(t, json.Unmarshal(raw, &a))
	require.Len(t, a.Wires, c.Packets)
	return b, a
}
func rtpsFragmentEvent(t *testing.T, e *ProtocolEvent, want map[string]any, kind string) {
	t.Helper()
	require.Equal(t, "rtps", e.Protocol)
	require.Equal(t, rtpsFragmentProfile, e.Profile)
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
	tree, err := e.Decode()
	require.NoError(t, err)
	rocEqualFields(t, want, tree["fields"].(map[string]any))
	rocEqualFields(t, want, tree["session"].(map[string]any))
	mutateRTPSQuery(tree)
	e.Fields = map[string]any{"caller": true}
	e.Structured = map[string]any{"caller": true}
	clear(e.Raw)
	again, err := e.GetFields()
	require.NoError(t, err)
	rocEqualFields(t, want, again)
}
func TestRTPSFragmentsSealedReplay(t *testing.T) {
	for _, c := range rtpsFragmentControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, want := rtpsFragmentInput(t, c)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%v/observer%v", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithProtocolDecodeAs("udp", 28400, "rtps-spdp-fragments"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
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
								source, destination := "192.0.2.10:38000", "192.0.2.20:28400"
								if want.Directions[i] == 1 {
									source, destination = destination, source
								}
								require.Equal(t, source, e.Source)
								require.Equal(t, destination, e.Destination)
								require.Equal(t, want.Wires[i], hex.EncodeToString(e.Raw))
								rtpsFragmentEvent(t, e, a.Fields, a.Error)
								require.Empty(t, e.SourceBytes.ParentPDUs)
								require.Len(t, e.SourceBytes.PacketRefs, 1)
								require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
								encoded, err := hex.DecodeString(want.Wires[i])
								require.NoError(t, err)
								require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(encoded)), e.SourceBytes.SHA256)
							}
							for i, ids := range want.CloseIDs {
								e := events[len(want.Events)+i]
								require.Equal(t, ids, e.SourceBytes.ParentPDUs)
								require.Equal(t, "incomplete", e.Status)
								require.Empty(t, e.Raw)
								rtpsFragmentEvent(t, e, nil, "NeedMore")
							}
						})
					}
				}
			}
		})
	}
}
func TestRTPSFragmentsSessionAndLocal(t *testing.T) {
	for _, c := range rtpsFragmentControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, want := rtpsFragmentInput(t, c)
			session, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSFragments(28400))
			require.NoError(t, err)
			var events []*ProtocolEvent
			for i, raw := range want.Wires {
				wire, err := hex.DecodeString(raw)
				require.NoError(t, err)
				session.Probe(wire)
				require.EqualValues(t, i, session.Stats().Messages)
				result := session.Feed(want.Directions[i], time.Unix(1700000000+int64(want.Seconds[i]), 0), wire)
				require.Len(t, result.Events, 1)
				events = append(events, result.Events[0])
				clear(wire)
			}
			for i, e := range events {
				rtpsFragmentEvent(t, e, want.Events[i].Fields, want.Events[i].Error)
			}
			close := session.Close("fixture-end")
			require.Len(t, close, len(want.CloseIDs))
			for i, e := range close {
				require.Equal(t, want.CloseIDs[i], e.SourceBytes.ParentPDUs)
				rtpsFragmentEvent(t, e, nil, "NeedMore")
			}
			require.Empty(t, session.Close("again"))
			require.Zero(t, session.Stats().BufferedBytes)
			for i, a := range want.Default {
				wire, err := hex.DecodeString(want.Wires[i])
				require.NoError(t, err)
				fields, err := decodeRTPSDatagram(wire, 4096)
				if a.Error != "" {
					rocTypedError(t, a.Error, err)
					require.Nil(t, fields)
				} else {
					require.NoError(t, err)
					rocEqualFields(t, a.Fields, fields)
				}
			}
		})
	}
}

func TestRTPSFragmentsBudgetAndCaptureDomain(t *testing.T) {
	cases := rtpsFragmentControls(t)
	var bundle, ordered, regular rtpsFragmentAnswer
	for _, c := range cases {
		switch c.ID {
		case "single-bundle-le":
			_, bundle = rtpsFragmentInput(t, c)
		case "ordered":
			_, ordered = rtpsFragmentInput(t, c)
		case "regular-data-completes-fragments":
			_, regular = rtpsFragmentInput(t, c)
		}
	}
	wire, err := hex.DecodeString(bundle.Wires[0])
	require.NoError(t, err)
	// One submessage, seven fragments, seven non-sentinel parameters.
	for _, elements := range []int{14, 15} {
		fields, err := decodeRTPSDatagram(wire, elements)
		if elements == 14 {
			require.Nil(t, fields)
			rocTypedError(t, "ResourceExceeded", err)
		} else {
			require.NoError(t, err)
			rocEqualFields(t, bundle.Default[0].Fields, fields)
		}
	}
	for _, tc := range []struct {
		name                   string
		depth, elements, bytes int
		success                bool
	}{
		{"depth-low", 4, 4096, 1 << 20, false}, {"depth-exact", 5, 4096, 1 << 20, true}, {"elements-low", 64, 14, 1 << 20, false}, {"elements-exact", 64, 15, 1 << 20, true}, {"wire-low", 64, 4096, len(wire) - 1, false}, {"wire-exact", 64, 4096, len(wire), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget := DefaultParserBudget()
			budget.MaxRecursionDepth = tc.depth
			budget.MaxCollectionElements = tc.elements
			budget.MaxFrameBytes = tc.bytes
			budget.MaxMessageBytes = tc.bytes
			s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSFragments(28400))
			require.NoError(t, err)
			r := s.Feed(0, time.Unix(1, 0), wire)
			require.Len(t, r.Events, 1)
			if tc.success {
				rtpsFragmentEvent(t, r.Events[0], bundle.Events[0].Fields, "")
			} else {
				rtpsFragmentEvent(t, r.Events[0], nil, "ResourceExceeded")
				later := s.Feed(0, time.Unix(2, 0), wire)
				require.Len(t, later.Events, 1)
				rtpsFragmentEvent(t, later.Events[0], nil, "ContextRequired")
			}
			s.Close("end")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	t.Run("regular-data-does-not-double-charge-parameters", func(t *testing.T) {
		budget := DefaultParserBudget()
		budget.MaxCollectionElements = 8
		s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSFragments(28400))
		require.NoError(t, err)
		for i, raw := range regular.Wires {
			w, err := hex.DecodeString(raw)
			require.NoError(t, err)
			r := s.Feed(0, time.Unix(int64(i), 0), w)
			require.Len(t, r.Events, 1)
			rtpsFragmentEvent(t, r.Events[0], regular.Events[i].Fields, regular.Events[i].Error)
		}
		s.Close("end")
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("capture-domain-isolation", func(t *testing.T) {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		a := s.(*captureSession).f.a
		for i, raw := range ordered.Wires {
			w, err := hex.DecodeString(raw)
			require.NoError(t, err)
			domain := CaptureDomain{Section: 1, Interface: 2, Encapsulation: "vlan:7"}
			if i > 0 {
				domain.Interface = 3
			}
			e := &ProtocolEvent{Source: "192.0.2.10:38000", Destination: "192.0.2.20:28400", Domain: domain, Timestamp: time.Unix(int64(i), 0), Transport: "udp", SourceBytes: ByteSource{PacketRefs: []PacketReference{{Number: uint64(i + 1), Domain: domain}}}}
			a.decodeRTPSFragmentDatagram(e, w, nil)
			rtpsFragmentEvent(t, e, nil, "NeedMore")
		}
		require.Len(t, a.rtpsFragments.entries, 2)
		a.closeUDPSessions()
		require.Zero(t, a.buffered.Load())
	})
	t.Run("sample-and-projection-charge-release", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSFragments(28400))
		require.NoError(t, err)
		var active int64
		for i, raw := range ordered.Wires {
			w, err := hex.DecodeString(raw)
			require.NoError(t, err)
			r := s.Feed(0, time.Unix(int64(i), 0), w)
			require.Len(t, r.Events, 1)
			if i == 0 {
				active = s.Stats().BufferedBytes
			}
			if i == len(ordered.Wires)-1 {
				require.Less(t, s.Stats().BufferedBytes, active)
			}
		}
		s.Close("end")
		require.Zero(t, s.Stats().BufferedBytes)
	})
	for _, opts := range [][]ProtocolSessionOption{{WithSessionRTPSFragments(28400)}, {WithSessionTransport("udp"), WithSessionRTPSFragments(0)}, {WithSessionRTPSFragments(28400), WithSessionTransport("tcp")}, {WithSessionTransport("tcp"), WithSessionRTPSFragments(28400)}} {
		_, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.Error(t, err)
	}
}

func TestRTPSFragmentsRefusalAndProbeState(t *testing.T) {
	var ordered, qos rtpsFragmentAnswer
	for _, c := range rtpsFragmentControls(t) {
		switch c.ID {
		case "ordered":
			_, ordered = rtpsFragmentInput(t, c)
		case "inline-qos-only-first":
			_, qos = rtpsFragmentInput(t, c)
		}
	}
	t.Run("probe-never-installs-fragment-state", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSFragments(28400))
		require.NoError(t, err)
		wire, err := hex.DecodeString(ordered.Wires[0])
		require.NoError(t, err)
		for i := 0; i < 3; i++ {
			p := s.Probe(wire)
			require.Equal(t, ProbeAccept, p.Verdict)
			require.Equal(t, "rtps", p.Protocol)
			require.Zero(t, s.Stats().Messages)
			require.Zero(t, s.Stats().BufferedBytes)
			require.Nil(t, s.(*captureSession).f.a.rtpsFragments)
		}
		r := s.Feed(2, time.Unix(1, 0), wire)
		require.Empty(t, r.Events)
		require.Zero(t, r.Consumed)
		require.NotNil(t, r.Err)
		require.Equal(t, ErrMalformedMessage, r.Err.Kind)
		require.Nil(t, s.(*captureSession).f.a.rtpsFragments)
		s.Close("end")
	})
	t.Run("retained-qos-is-in-final-element-budget", func(t *testing.T) {
		for _, limit := range []int{10, 11} {
			t.Run(fmt.Sprint(limit), func(t *testing.T) {
				b := DefaultParserBudget()
				b.MaxCollectionElements = limit
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSFragments(28400))
				require.NoError(t, err)
				for i, raw := range qos.Wires {
					wire, err := hex.DecodeString(raw)
					require.NoError(t, err)
					r := s.Feed(0, time.Unix(int64(i), 0), wire)
					require.Len(t, r.Events, 1)
					if i < len(qos.Wires)-1 {
						rtpsFragmentEvent(t, r.Events[0], nil, "NeedMore")
					} else if limit == 10 {
						rtpsFragmentEvent(t, r.Events[0], nil, "ResourceExceeded")
					} else {
						rtpsFragmentEvent(t, r.Events[0], qos.Events[i].Fields, "")
					}
				}
				s.Close("end")
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	})
	t.Run("untracked-global-byte-refusal-fails-closed", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxBufferedBytes = 256
		b.MaxMessageBytes = 256
		b.MaxFrameBytes = 256
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSFragments(28400))
		require.NoError(t, err)
		for i, raw := range ordered.Wires {
			wire, err := hex.DecodeString(raw)
			require.NoError(t, err)
			r := s.Feed(0, time.Unix(int64(i), 0), wire)
			require.Len(t, r.Events, 1)
			rtpsFragmentEvent(t, r.Events[0], nil, "ResourceExceeded")
			require.Zero(t, s.Stats().BufferedBytes)
		}
		s.Close("end")
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("carrier-refusal-retires-pending-history", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(38000, 28400), WithSessionRTPSFragments(28400))
		require.NoError(t, err)
		first, err := hex.DecodeString(ordered.Wires[0])
		require.NoError(t, err)
		r := s.Feed(0, time.Unix(1, 0), first)
		rtpsFragmentEvent(t, r.Events[0], nil, "NeedMore")
		e := &ProtocolEvent{Source: "192.0.2.1:38000", Destination: "192.0.2.2:28400", Timestamp: time.Unix(2, 0), Transport: "udp"}
		a := s.(*captureSession).f.a
		a.decodeRTPSFragmentDatagram(e, first[:30], protocolError(ErrNeedMore, "independent capture-truncation control"))
		require.Empty(t, e.Raw)
		rtpsFragmentEvent(t, e, nil, "NeedMore")
		later, err := hex.DecodeString(ordered.Wires[1])
		require.NoError(t, err)
		r = s.Feed(0, time.Unix(3, 0), later)
		rtpsFragmentEvent(t, r.Events[0], nil, "ContextRequired")
		require.Empty(t, s.Close("end"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}
