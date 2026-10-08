package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

type bsapControl struct {
	ID, Capture, Answer, SHA256 string
	AnswerSHA                   string `json:"answer_sha256"`
	Packets                     int
}
type bsapAnswer struct {
	Wires      []string
	Directions []int
	Packets    int
	Events     []struct {
		Fields     map[string]any
		Error, Raw string
		Direction  int
		Reply      *int `json:"response_to_index"`
	}
}

func bsapControls(t *testing.T) []bsapControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("bsap-local-rdb/manifest.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []bsapControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "pr5013-bsap-local-rdb/v1", m.Schema)
	require.Len(t, m.Cases, 59)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	bound := map[string]trafficfixture.FrozenCase{}
	for _, batch := range all {
		for _, c := range batch.Cases {
			bound[c.ID] = c
		}
	}
	for _, c := range m.Cases {
		v, ok := bound["bsap-local-rdb/"+c.ID]
		require.True(t, ok)
		require.Equal(t, c.SHA256, v.Input.SHA256)
		require.Equal(t, c.Capture, v.Input.File)
		require.Equal(t, c.Packets, v.Facts.PacketCount)
		require.Len(t, v.Expectations, 1)
		var binding struct {
			AnswerFile string `json:"answer_file"`
			AnswerSHA  string `json:"answer_sha256"`
		}
		require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &binding))
		require.Equal(t, c.Answer, binding.AnswerFile)
		require.Equal(t, c.AnswerSHA, binding.AnswerSHA)
	}
	return m.Cases
}
func bsapInput(t *testing.T, c bsapControl) ([]byte, bsapAnswer) {
	t.Helper()
	raw, err := trafficfixture.ReadFile("bsap-local-rdb/" + c.Capture)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	b, err := trafficfixture.ReadFile("bsap-local-rdb/" + c.Answer)
	require.NoError(t, err)
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
	var a bsapAnswer
	require.NoError(t, json.Unmarshal(b, &a))
	require.Equal(t, c.Packets, a.Packets)
	r, err := NewCaptureReader(bytes.NewReader(raw))
	require.NoError(t, err)
	for _, wire := range a.Wires {
		b, _, err := r.ReadPacketData()
		require.NoError(t, err)
		p := gopacket.NewPacket(b, r.LinkType(), gopacket.Default)
		u, ok := p.Layer(layers.LayerTypeUDP).(*layers.UDP)
		require.True(t, ok)
		require.Equal(t, wire, hex.EncodeToString(u.Payload))
	}
	_, _, err = r.ReadPacketData()
	require.ErrorIs(t, err, io.EOF)
	return raw, a
}
func TestBSAPLocalRDBSealedReplay(t *testing.T) {
	for _, c := range bsapControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, want := bsapInput(t, c)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("workers%d/deferred%t/observer%t", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var assembly TCPReassemblyStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithProtocolDecodeAs("udp", 28400, "bsap"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
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
								require.Equal(t, "bsap", e.Protocol)
								require.Equal(t, "udp", e.Transport)
								require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
								require.Equal(t, "explicit-decode-as", e.Admission)
								require.EqualValues(t, w.Direction, e.Direction)
								src, dst := "192.0.2.10:38000", "192.0.2.20:28400"
								if want.Directions[i] == 1 {
									src, dst = dst, src
								}
								require.Equal(t, src, e.Source)
								require.Equal(t, dst, e.Destination)
								require.Len(t, e.SourceBytes.PacketRefs, 1)
								require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
								require.Zero(t, e.TransactionID)
								if w.Reply == nil {
									require.Zero(t, e.ResponseTo)
								} else {
									require.Equal(t, events[*w.Reply].ID, e.ResponseTo)
									require.NotZero(t, e.ResponseTo)
									require.Equal(t, events[*w.Reply].FlowID, e.FlowID)
								}
								if w.Fields != nil {
									require.Contains(t, []string{"decoded", "deferred"}, e.Status)
									require.Empty(t, e.Error)
									fields, err := e.GetFields()
									require.NoError(t, err)
									encoded, err := json.Marshal(fields)
									require.NoError(t, err)
									var got map[string]any
									require.NoError(t, json.Unmarshal(encoded, &got))
									require.Equal(t, w.Fields, got)
									session, err := json.Marshal(e.Session)
									require.NoError(t, err)
									require.JSONEq(t, string(encoded), string(session))
									fields["Observation"] = "changed"
									require.Equal(t, "unverified-serial-rdb", e.Session["Observation"])
								} else {
									var pe *ProtocolError
									require.True(t, errors.As(e.sessionError, &pe))
									require.Equal(t, w.Error, string(pe.Kind))
									require.NotEmpty(t, e.Error)
									require.Nil(t, e.Session)
									require.Nil(t, e.Fields)
									require.Nil(t, e.Structured)
								}
							}
						})
					}
				}
			}
		})
	}
}
func bsapWire(t *testing.T, id string) ([][]byte, bsapAnswer) {
	t.Helper()
	for _, c := range bsapControls(t) {
		if c.ID == id {
			_, a := bsapInput(t, c)
			var w [][]byte
			for _, s := range a.Wires {
				b, err := hex.DecodeString(s)
				require.NoError(t, err)
				w = append(w, b)
			}
			return w, a
		}
	}
	t.Fatal("case not found", id)
	return nil, bsapAnswer{}
}
func TestBSAPLocalRDBBoundaries(t *testing.T) {
	wires, _ := bsapWire(t, "multiple-ordered-values")
	w := wires[0]
	m, err := decodeBSAPMessage(w, 3)
	require.NoError(t, err)
	require.Len(t, m.names, 3)
	_, err = decodeBSAPMessage(w, 2)
	var resource *ProtocolError
	require.True(t, errors.As(err, &resource))
	require.Equal(t, ErrResourceExceeded, resource.Kind)
	worked := []byte{0x81, 0x24, 0x20, 4, 0, 0, 0, 0xa0, 0x55, 0xcf, 3, 0, 4, 3, 0xf3, 0x81, 15, 1, 0x43, 0x53, 0x31, 0x53, 0x44, 0x48, 0x56, 0x2e, 0x43, 0x4c, 0x4f, 0x53, 0x45, 0x44, 0x2e, 0, 0, 0x31, 0, 0, 0, 3}
	require.EqualValues(t, 0xcdc0, bsapCRC(worked))
	for n := 0; n < len(w); n++ {
		_, err := bsapBody(w[:n])
		require.Error(t, err)
	}
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(38000, 28400))
	require.NoError(t, err)
	p := s.Probe(w)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "bsap", p.Protocol)
	require.Zero(t, s.Stats().BufferedBytes)
	r := s.Feed(0, time.Unix(1700000000, 0), w)
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.NotZero(t, s.Stats().BufferedBytes)
	clear(w)
	fields, err := r.Events[0].GetFields()
	require.NoError(t, err)
	fields["Names"].([]string)[0] = "changed"
	require.Equal(t, "MVP.L1.", r.Events[0].Session["Names"].([]string)[0])
	s.Close("test end")
	require.Zero(t, s.Stats().BufferedBytes)
	s.Close("test end")
	require.Zero(t, s.Stats().BufferedBytes)
	tcp, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"))
	require.NoError(t, err)
	require.NotEqual(t, ProbeAccept, tcp.Probe(wires[1]).Verdict)
	tcp.Close("test end")
}

func TestBSAPLocalRDBResourceAndIsolation(t *testing.T) {
	wires, _ := bsapWire(t, "logical-on")
	q, r := wires[0], wires[1]
	newSession := func(b ParserBudget) *captureSession {
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(38000, 28400))
		require.NoError(t, err)
		return s.(*captureSession)
	}
	// Wire admission is independent of the UDP port, but invalid CRC cannot establish a conversation.
	s := newSession(DefaultParserBudget())
	badCRC := bytes.Clone(q)
	badCRC[len(badCRC)-1] ^= 1
	require.NotEqual(t, ProbeAccept, s.Probe(badCRC).Verdict)
	a := s.Feed(0, time.Unix(1700000000, 0), badCRC)
	require.Len(t, a.Events, 1)
	require.Equal(t, "unrecognized", a.Events[0].Status)
	require.Zero(t, s.Stats().BufferedBytes)
	a = s.Feed(0, time.Unix(1700000000, 0), q)
	require.Nil(t, a.Err)
	require.Equal(t, "wire-signature", a.Events[0].Admission)
	first := a.Events[0]
	b := s.Feed(1, time.Unix(1700000001, 0), r)
	require.Nil(t, b.Err)
	require.Equal(t, first.ID, b.Events[0].ResponseTo)
	got, err := b.Events[0].GetFields()
	require.NoError(t, err)
	got["Elements"].([]map[string]any)[0]["Name"] = "changed"
	require.Equal(t, "MVP.L1.", b.Events[0].Session["Elements"].([]map[string]any)[0]["Name"])
	s.Close("completed")
	require.Zero(t, s.Stats().BufferedBytes)
	require.Empty(t, s.f.a.udpSessions)
	// State is admitted before retaining any request names. Both sides of the exact reservation boundary.
	for _, delta := range []int{-1, 0} {
		budget := DefaultParserBudget()
		budget.MaxMessageBytes = 64
		budget.MaxFrameBytes = 64
		budget.MaxBufferedBytes = 576 + 3*len(q) + delta
		s := newSession(budget)
		a := s.Feed(0, time.Unix(1700000000, 0), q)
		if delta < 0 {
			var pe *ProtocolError
			require.True(t, errors.As(a.Err, &pe))
			require.Equal(t, ErrResourceExceeded, pe.Kind)
			require.Zero(t, s.Stats().BufferedBytes)
		} else {
			require.Nil(t, a.Err)
			require.EqualValues(t, 576+3*len(q), s.Stats().BufferedBytes)
		}
		s.Close("budget")
		require.Zero(t, s.Stats().BufferedBytes)
	}
	// Independent frame and collection limits; they are never relaxed to fit an input.
	budget := DefaultParserBudget()
	budget.MaxFrameBytes = len(q) - 1
	s = newSession(budget)
	a = s.Feed(0, time.Unix(1700000000, 0), q)
	var pe *ProtocolError
	require.True(t, errors.As(a.Err, &pe))
	require.Equal(t, ErrResourceExceeded, pe.Kind)
	require.Zero(t, s.Stats().BufferedBytes)
	s.Close("frame limit")
	// Domain is part of the key. Identical endpoint/application IDs must not borrow another interface's request.
	s = newSession(DefaultParserBudget())
	parser := s.f.a
	event := func(src, dst string, domain CaptureDomain, ts time.Time) *ProtocolEvent {
		return &ProtocolEvent{Source: src, Destination: dst, Transport: "udp", Domain: domain, Timestamp: ts}
	}
	d0, d1 := CaptureDomain{Interface: 0, Section: 1}, CaptureDomain{Interface: 1, Section: 1}
	ts := time.Unix(1700000000, 0)
	e := event("192.0.2.10:38000", "192.0.2.20:28400", d0, ts)
	require.True(t, parser.decodeBSAPDatagram(e, q, false))
	require.Nil(t, e.sessionError)
	id := e.ID
	other := event(e.Destination, e.Source, d1, ts)
	require.True(t, parser.decodeBSAPDatagram(other, r, false))
	require.Equal(t, ErrContextRequired, other.sessionError.Kind)
	require.Zero(t, other.ResponseTo)
	reply := event(e.Destination, e.Source, d0, ts)
	require.True(t, parser.decodeBSAPDatagram(reply, r, false))
	require.Nil(t, reply.sessionError)
	require.Equal(t, id, reply.ResponseTo)
	// Idle expiration releases old state and an ambiguous slot cannot survive into the next conversation.
	newWires, _ := bsapWire(t, "overlapping-requests")
	nextRequest := newWires[1]
	e = event("192.0.2.10:38000", "192.0.2.20:28400", d0, ts)
	require.True(t, parser.decodeBSAPDatagram(e, nextRequest, false))
	require.Nil(t, e.sessionError)
	expired := event(e.Destination, e.Source, d0, ts.Add(bsapIdleTTL))
	require.True(t, parser.decodeBSAPDatagram(expired, r, false))
	require.Equal(t, ErrContextRequired, expired.sessionError.Kind)
	require.Zero(t, expired.ResponseTo)
	require.Zero(t, s.Stats().BufferedBytes)
	fresh := event(e.Source, e.Destination, d0, ts.Add(bsapIdleTTL))
	require.True(t, parser.decodeBSAPDatagram(fresh, q, false))
	require.Nil(t, fresh.sessionError)
	require.NotEqual(t, e.FlowID, fresh.FlowID)
	s.Close("domain cleanup")
	require.Zero(t, s.Stats().BufferedBytes)
	// An additional conversation is refused when one collection slot is already retained.
	budget = DefaultParserBudget()
	budget.MaxCollectionElements = 1
	s = newSession(budget)
	parser = s.f.a
	e = event("192.0.2.10:38000", "192.0.2.20:28400", d0, ts)
	require.True(t, parser.decodeBSAPDatagram(e, q, false))
	require.Nil(t, e.sessionError)
	e2 := event("192.0.2.30:38000", "192.0.2.40:28400", d0, ts)
	require.True(t, parser.decodeBSAPDatagram(e2, q, false))
	require.Equal(t, ErrResourceExceeded, e2.sessionError.Kind)
	require.Len(t, parser.udpSessions.entries, 1)
	s.Close("collection cleanup")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestBSAPLocalRDBSequenceReuse(t *testing.T) {
	wires, _ := bsapWire(t, "logical-on")
	q, r := wires[0], wires[1]
	m, err := decodeBSAPMessage(q, 4096)
	require.NoError(t, err)
	reply, err := decodeBSAPMessage(r, 4096)
	require.NoError(t, err)
	state := &binBSAP{}
	_, _, err = state.consume(m, 0, 11, 1)
	require.NoError(t, err)
	_, id, err := state.consume(reply, 1, 12, 1)
	require.NoError(t, err)
	require.EqualValues(t, 11, id)
	// An old reply must not be borrowed by a second request reusing that wire ID.
	_, _, err = state.consume(m, 0, 13, 1)
	var pe *ProtocolError
	require.True(t, errors.As(err, &pe))
	require.Equal(t, ErrContextRequired, pe.Kind)
	_, id, err = state.consume(reply, 1, 14, 1)
	require.Error(t, err)
	require.Zero(t, id)
	state = &binBSAP{}
	_, _, err = state.consume(m, 0, 21, 1)
	require.NoError(t, err)
	_, _, err = state.consume(reply, 1, 22, 1)
	require.NoError(t, err)
	other := *m
	other.sequence++
	other.body = bytes.Clone(m.body)
	other.body[3]++
	_, _, err = state.consume(&other, 0, 23, 1)
	require.True(t, errors.As(err, &pe))
	require.Equal(t, ErrResourceExceeded, pe.Kind)
	require.Len(t, state.seen, 1)
	require.Nil(t, state.pending)
}
