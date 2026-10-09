package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

func slmpControls(t *testing.T) []bsapControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("slmp-self-test/manifest.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []bsapControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "pr5013-slmp-self-test/v1", m.Schema)
	require.Len(t, m.Cases, 86)
	batches, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	bound := map[string]trafficfixture.FrozenCase{}
	for _, batch := range batches {
		for _, c := range batch.Cases {
			bound[c.ID] = c
		}
	}
	for _, c := range m.Cases {
		v, ok := bound["slmp-self-test/"+c.ID]
		require.True(t, ok)
		require.Equal(t, c.SHA256, v.Input.SHA256)
		require.Equal(t, c.Capture, v.Input.File)
		require.Equal(t, c.Packets, v.Facts.PacketCount)
		require.Len(t, v.Expectations, 1)
		var a struct {
			File string `json:"answer_file"`
			SHA  string `json:"answer_sha256"`
		}
		require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &a))
		require.Equal(t, c.Answer, a.File)
		require.Equal(t, c.AnswerSHA, a.SHA)
	}
	return m.Cases
}
func slmpInput(t *testing.T, c bsapControl) ([]byte, bsapAnswer) {
	t.Helper()
	raw, err := trafficfixture.ReadFile("slmp-self-test/" + c.Capture)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	ans, err := trafficfixture.ReadFile("slmp-self-test/" + c.Answer)
	require.NoError(t, err)
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ans)))
	var a bsapAnswer
	require.NoError(t, json.Unmarshal(ans, &a))
	require.Equal(t, c.Packets, a.Packets)
	r, err := NewCaptureReader(bytes.NewReader(raw))
	require.NoError(t, err)
	for _, e := range a.Events {
		b, ci, err := r.ReadPacketData()
		require.NoError(t, err)
		require.Equal(t, len(b), ci.CaptureLength)
		require.Equal(t, ci.Length, ci.CaptureLength)
		p := gopacket.NewPacket(b, r.LinkType(), gopacket.Default)
		u, ok := p.Layer(layers.LayerTypeUDP).(*layers.UDP)
		require.True(t, ok)
		require.Equal(t, e.Raw, hex.EncodeToString(u.Payload))
	}
	_, _, err = r.ReadPacketData()
	require.ErrorIs(t, err, io.EOF)
	return raw, a
}
func slmpWires(t *testing.T, id string) [][]byte {
	t.Helper()
	for _, c := range slmpControls(t) {
		if c.ID == id {
			_, a := slmpInput(t, c)
			out := make([][]byte, len(a.Events))
			for i, e := range a.Events {
				v, err := hex.DecodeString(e.Raw)
				require.NoError(t, err)
				out[i] = v
			}
			return out
		}
	}
	t.Fatal("missing sealed control", id)
	return nil
}
func TestSLMPSealedReplay(t *testing.T) {
	for _, c := range slmpControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, want := slmpInput(t, c)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("workers%d/deferred%t/observer%t", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var assembly TCPReassemblyStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithProtocolDecodeAs("udp", 5000, "slmp"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.NotEmpty(t, p.Data()); seen.Add(1) }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							require.Len(t, events, len(want.Events))
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							require.Zero(t, assembly.UnreassembledBytes)
							require.EqualValues(t, c.Packets, assembly.CapturedPackets)
							if observe {
								require.EqualValues(t, c.Packets, seen.Load())
							}
							for i, e := range events {
								w := want.Events[i]
								require.Equal(t, "slmp", e.Protocol)
								require.Equal(t, "slmp-binary-self-test", e.Profile)
								require.Equal(t, "udp", e.Transport)
								completion := "message"
								if w.Error == "MalformedMessage" {
									completion = "malformed"
								} else if w.Error == "UnsupportedFeature" || w.Error == "ContextRequired" {
									completion = "context-required"
								}
								require.Equal(t, completion, e.Completeness)
								require.Equal(t, "explicit-decode-as", e.Admission)
								require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
								require.Equal(t, w.Direction, e.Direction)
								require.Zero(t, e.TransactionID)
								src, dst := "192.0.2.10:39000", "192.0.2.20:5000"
								if w.Direction == 1 {
									src, dst = dst, src
								}
								require.Equal(t, src, e.Source)
								require.Equal(t, dst, e.Destination)
								require.Len(t, e.SourceBytes.PacketRefs, 1)
								require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
								if w.Reply == nil {
									require.Zero(t, e.ResponseTo)
								} else {
									require.NotZero(t, e.ResponseTo)
									require.Equal(t, events[*w.Reply].ID, e.ResponseTo)
									require.Equal(t, events[*w.Reply].FlowID, e.FlowID)
								}
								if w.Fields == nil {
									rocTypedError(t, w.Error, e.sessionError)
									require.NotEmpty(t, e.Error)
									require.Nil(t, e.Session)
									require.Nil(t, e.Fields)
									require.Nil(t, e.Structured)
								} else {
									status := "decoded"
									if deferred {
										status = "deferred"
									}
									require.Equal(t, status, e.Status)
									require.Empty(t, e.Error)
									rocEqualFields(t, w.Fields, e.Session)
									f, err := e.GetFields()
									require.NoError(t, err)
									rocEqualFields(t, w.Fields, f)
									v, err := e.Decode()
									require.NoError(t, err)
									rocEqualFields(t, w.Fields, protocolFields(v))
									f["Observation"] = "changed"
									if data, ok := f["Loopback Data"].([]byte); ok {
										data[0] ^= 255
									}
									rocEqualFields(t, w.Fields, e.Session)
									rocEqualFields(t, w.Fields, e.semanticFields)
								}
							}
						})
					}
				}
			}
		})
	}
}
func TestSLMPResourceAndIsolation(t *testing.T) {
	wires := slmpWires(t, "4e-manual-ABCDE")
	q, r := wires[0], wires[1]
	newSession := func(b ParserBudget) *captureSession {
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(39000, 5000))
		require.NoError(t, err)
		return s.(*captureSession)
	}
	s := newSession(DefaultParserBudget())
	p := s.Probe(q)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "slmp", p.Protocol)
	require.Zero(t, s.Stats().BufferedBytes)
	a := s.Feed(0, time.Unix(1700000000, 0), bytes.Clone(q))
	require.Nil(t, a.Err)
	require.Len(t, a.Events, 1)
	require.NotZero(t, s.Stats().BufferedBytes)
	first := a.Events[0]
	b := s.Feed(1, time.Unix(1700000001, 0), r)
	require.Nil(t, b.Err)
	require.Equal(t, first.ID, b.Events[0].ResponseTo)
	s.Close("completed")
	require.Zero(t, s.Stats().BufferedBytes)
	require.Nil(t, s.f.a.udpSessions)
	s.Close("idempotent")
	require.Zero(t, s.Stats().BufferedBytes)
	for _, delta := range []int{-1, 0} {
		budget := DefaultParserBudget()
		budget.MaxMessageBytes = 64
		budget.MaxFrameBytes = 64
		budget.MaxBufferedBytes = 576 + 3*len(q) + delta
		s := newSession(budget)
		out := s.Feed(0, time.Unix(1700000000, 0), q)
		if delta < 0 {
			rocTypedError(t, "ResourceExceeded", out.Err)
			require.Zero(t, s.Stats().BufferedBytes)
		} else {
			require.Nil(t, out.Err)
			require.EqualValues(t, 576+3*len(q), s.Stats().BufferedBytes)
		}
		s.Close("budget")
		require.Zero(t, s.Stats().BufferedBytes)
	}
	budget := DefaultParserBudget()
	budget.MaxFrameBytes = len(q) - 1
	s = newSession(budget)
	a = s.Feed(0, time.Unix(1700000000, 0), q)
	rocTypedError(t, "ResourceExceeded", a.Err)
	require.Empty(t, a.Events[0].Raw)
	require.Nil(t, a.Events[0].Session)
	require.Zero(t, s.Stats().BufferedBytes)
	s.Close("frame bound")
	budget = DefaultParserBudget()
	budget.MaxCollectionElements = 1
	s = newSession(budget)
	parser := s.f.a
	ts := time.Unix(1700000000, 0)
	event := func(src, dst string, domain CaptureDomain, stamp time.Time) *ProtocolEvent {
		return &ProtocolEvent{Source: src, Destination: dst, Transport: "udp", Domain: domain, Timestamp: stamp}
	}
	d0, d1 := CaptureDomain{Interface: 0, Section: 1}, CaptureDomain{Interface: 1, Section: 1}
	e := event("192.0.2.10:39000", "192.0.2.20:5000", d0, ts)
	require.True(t, parser.decodeSLMPDatagram(e, q, false))
	require.Nil(t, e.sessionError)
	wrong := event(e.Destination, e.Source, d1, ts)
	require.True(t, parser.decodeSLMPDatagram(wrong, r, true))
	rocTypedError(t, "ContextRequired", wrong.sessionError)
	require.Zero(t, wrong.FlowID)
	require.Zero(t, wrong.ResponseTo)
	full := event("192.0.2.30:39000", "192.0.2.40:5000", d0, ts)
	require.True(t, parser.decodeSLMPDatagram(full, q, false))
	rocTypedError(t, "ResourceExceeded", full.sessionError)
	require.Len(t, parser.udpSessions.entries, 1)
	good := event(e.Destination, e.Source, d0, ts)
	require.True(t, parser.decodeSLMPDatagram(good, r, false))
	require.Nil(t, good.sessionError)
	require.Equal(t, e.ID, good.ResponseTo)
	// A distinct serial is refused at the collection boundary, without retaining it.
	next := bytes.Clone(q)
	next[2]++
	n := event(e.Source, e.Destination, d0, ts)
	require.True(t, parser.decodeSLMPDatagram(n, next, false))
	rocTypedError(t, "ResourceExceeded", n.sessionError)
	for _, el := range parser.udpSessions.entries {
		require.Len(t, el.Value.(*binUDPEntry).flow.slmp.seen, 1)
		require.Nil(t, el.Value.(*binUDPEntry).flow.slmp.pending)
	}
	expired := event(e.Destination, e.Source, d0, ts.Add(slmpIdleTTL))
	require.True(t, parser.decodeSLMPDatagram(expired, r, true))
	rocTypedError(t, "ContextRequired", expired.sessionError)
	require.Zero(t, s.Stats().BufferedBytes)
	fresh := event(e.Source, e.Destination, d0, ts.Add(slmpIdleTTL))
	borrowed := bytes.Clone(q)
	require.True(t, parser.decodeSLMPDatagram(fresh, borrowed, false))
	require.Nil(t, fresh.sessionError)
	clear(borrowed)
	f, err := fresh.GetFields()
	require.NoError(t, err)
	f["Loopback Data"].([]byte)[0] ^= 255
	require.Equal(t, []byte("ABCDE"), fresh.Session["Loopback Data"])
	require.NotEqual(t, e.FlowID, fresh.FlowID)
	s.Close("cleanup")
	require.Zero(t, s.Stats().BufferedBytes)
	t.Run("resource-denied-overlap", func(t *testing.T) {
		old := slmpWires(t, "3e-manual-ABCDE")[0]
		newer := slmpWires(t, "3e-maximum")[0]
		reply := slmpWires(t, "3e-error")[1]
		budget := DefaultParserBudget()
		budget.MaxBufferedBytes = 2048
		budget.MaxMessageBytes = 1024
		budget.MaxFrameBytes = 1024
		x, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(39000, 5000))
		require.NoError(t, err)
		first := x.Feed(0, time.Unix(1700000000, 0), bytes.Clone(old))
		require.Nil(t, first.Err)
		require.NotZero(t, first.Events[0].ID)
		rejected := x.Feed(0, time.Unix(1700000001, 0), newer)
		rocTypedError(t, "ResourceExceeded", rejected.Err)
		late := x.Feed(1, time.Unix(1700000002, 0), reply)
		require.NotNil(t, late.Err, "budget-denied distinct request must retire old association: oldID=%d, actualResponseTo=%d", first.Events[0].ID, late.Events[0].ResponseTo)
		rocTypedError(t, "ContextRequired", late.Err)
		require.Zero(t, late.Events[0].ResponseTo)
		require.Nil(t, late.Events[0].Session)
		x.Close("pressure cleanup")
		require.Zero(t, x.Stats().BufferedBytes)

	})
}
func TestSLMPAdmissionAndDirections(t *testing.T) {
	wires := slmpWires(t, "3e-manual-ABCDE")
	q, r := wires[0], wires[1]
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
	require.NoError(t, err)
	require.Equal(t, "slmp", s.Probe(q).Protocol)
	// A complete validated request now establishes the TCP Self Test profile,
	// including off-port admission. It must not become PostgreSQL Parse.
	feed := s.Feed(0, time.Unix(1700000000, 0), q)
	require.Len(t, feed.Events, 1)
	require.Equal(t, "slmp", feed.Events[0].Protocol)
	require.Equal(t, slmpTCPProfile, feed.Events[0].Profile)
	require.Empty(t, feed.Events[0].Error)
	reply := s.Feed(1, time.Unix(1700000001, 0), r)
	require.Len(t, reply.Events, 1)
	require.Empty(t, reply.Events[0].Error)
	require.Equal(t, feed.Events[0].ID, reply.Events[0].ResponseTo)
	s.Close("TCP complete exchange")
	require.Zero(t, s.Stats().BufferedBytes)
	for n := 0; n < len(q); n++ {
		_, err := decodeSLMPMessage(q[:n])
		require.Error(t, err)
		require.False(t, slmpRequestEvidence(q[:n]))
	}
	// Port choice alone cannot identify MELSOFT, ASCII SLMP or a malformed request.
	for _, id := range []string{"ascii-outside-profile", "melsoft-discovery-outside", "3e-character-61"} {
		w := slmpWires(t, id)[0]
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(39000, 5000))
		require.NoError(t, err)
		require.NotEqual(t, ProbeAccept, s.Probe(w).Verdict)
		out := s.Feed(0, time.Unix(1700000000, 0), w)
		require.Len(t, out.Events, 1)
		require.Equal(t, "unrecognized", out.Events[0].Status)
		require.Zero(t, s.Stats().BufferedBytes)
		s.Close("unrecognized")
	}
	s, err = NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(39000, 5000))
	require.NoError(t, err)
	first := s.Feed(0, time.Unix(1700000000, 0), q)
	require.Nil(t, first.Err)
	wrong := s.Feed(0, time.Unix(1700000001, 0), r)
	rocTypedError(t, "ContextRequired", wrong.Err)
	require.Zero(t, wrong.Events[0].ResponseTo)
	good := s.Feed(1, time.Unix(1700000002, 0), r)
	require.Nil(t, good.Err)
	require.Equal(t, first.Events[0].ID, good.Events[0].ResponseTo)
	s.Close("direction cleanup")
	require.Zero(t, s.Stats().BufferedBytes)
}

// Identical IP IDs and four-tuples on different capture interfaces must neither
// join fragments nor borrow a request. Carrier answers are sealed with the wire.
func TestSLMPFragmentCaptureDomains(t *testing.T) {
	data, err := trafficfixture.ReadFile("slmp-self-test/manifest.json")
	require.NoError(t, err)
	var m struct{ Carriers []bsapControl }
	require.NoError(t, json.Unmarshal(data, &m))
	require.Len(t, m.Carriers, 2)
	for _, c := range m.Carriers {
		t.Run(c.ID, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile("slmp-self-test/" + c.Capture)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			b, err := trafficfixture.ReadFile("slmp-self-test/" + c.Answer)
			require.NoError(t, err)
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
			var a struct {
				Packets int
				Events  []struct {
					Raw       string
					Direction int
					Fields    map[string]any
					Error     string
					Reply     *int `json:"response_to_index"`
					Interface int
					Refs      []uint64 `json:"packet_refs"`
				}
			}
			require.NoError(t, json.Unmarshal(b, &a))
			require.Equal(t, c.Packets, a.Packets)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observer := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%v/o%v", workers, deferred, observer), func(t *testing.T) {
							var ev []*ProtocolEvent
							var stats ProtocolStats
							var asm TCPReassemblyStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithProtocolDecodeAs("udp", 5000, "slmp"), WithOnProtocolMessage(func(e *ProtocolEvent) { ev = append(ev, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { asm = s })}
							if observer {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							require.Len(t, ev, len(a.Events))
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							require.Zero(t, asm.UnreassembledBytes)
							require.Zero(t, asm.DecodeErrors)
							require.EqualValues(t, c.Packets, asm.CapturedPackets)
							if observer {
								require.Equal(t, c.Packets, seen)
							}
							for i, e := range ev {
								w := a.Events[i]
								require.Equal(t, "slmp", e.Protocol)
								require.Equal(t, "slmp-binary-self-test", e.Profile)
								require.Equal(t, "udp", e.Transport)
								require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
								require.Equal(t, w.Direction, e.Direction)
								require.Equal(t, w.Interface, e.Domain.Interface)
								require.Zero(t, e.TransactionID)
								var refs []uint64
								for _, ref := range e.SourceBytes.PacketRefs {
									refs = append(refs, ref.Number)
									require.Equal(t, e.Domain, ref.Domain)
								}
								require.Equal(t, w.Refs, refs)
								if w.Reply == nil {
									require.Zero(t, e.ResponseTo)
								} else {
									require.Equal(t, ev[*w.Reply].ID, e.ResponseTo)
									require.Equal(t, ev[*w.Reply].FlowID, e.FlowID)
									require.Equal(t, ev[*w.Reply].Domain, e.Domain)
								}
								if w.Fields == nil {
									rocTypedError(t, w.Error, e.sessionError)
									require.Nil(t, e.Session)
									require.Nil(t, e.Structured)
								} else {
									require.Empty(t, e.Error)
									rocEqualFields(t, w.Fields, e.Session)
									f, err := e.GetFields()
									require.NoError(t, err)
									rocEqualFields(t, w.Fields, f)
									v, err := e.Decode()
									require.NoError(t, err)
									rocEqualFields(t, w.Fields, protocolFields(v))
								}
							}
						})
					}
				}
			}
		})
	}
}
