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

func dlmsControls(t *testing.T) []bsapControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("dlms-hdlc/manifest.json")
	require.NoError(t, err)
	var m struct {
		Schema   string
		Cases    []bsapControl
		Carriers []bsapControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "pr5013-dlms-hdlc-get-normal/v1", m.Schema)
	require.Len(t, m.Cases, 93)
	require.Len(t, m.Carriers, 12)
	batches, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	bound := map[string]trafficfixture.FrozenCase{}
	for _, batch := range batches {
		for _, c := range batch.Cases {
			bound[c.ID] = c
		}
	}
	for _, c := range append(m.Cases, m.Carriers...) {
		v, ok := bound["dlms-hdlc/"+c.ID]
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
func dlmsInput(t *testing.T, c bsapControl) ([]byte, bsapAnswer) {
	t.Helper()
	raw, err := trafficfixture.ReadFile("dlms-hdlc/" + c.Capture)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	ans, err := trafficfixture.ReadFile("dlms-hdlc/" + c.Answer)
	require.NoError(t, err)
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ans)))
	var a bsapAnswer
	require.NoError(t, json.Unmarshal(ans, &a))
	if c.ID == "array" {
		// Preserve the original UnsupportedFeature answer; upgrade only this
		// independently proven legal Data form and bind both immutable hashes.
		var upgrade struct {
			CaptureSHA        string     `json:"capture_sha256"`
			OriginalAnswerSHA string     `json:"historical_answer_sha256"`
			OriginalError     string     `json:"historical_error"`
			Answer            bsapAnswer `json:"answer"`
		}
		upgraded, err := trafficfixture.ReadFile("dlms-hdlc-normal-data/historical-array-upgrade.json")
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(upgraded, &upgrade))
		require.Equal(t, c.SHA256, upgrade.CaptureSHA)
		require.Equal(t, c.AnswerSHA, upgrade.OriginalAnswerSHA)
		require.Equal(t, "UnsupportedFeature", upgrade.OriginalError)
		require.Len(t, a.Events, 2)
		require.Equal(t, upgrade.OriginalError, a.Events[1].Error)
		require.Len(t, upgrade.Answer.Events, len(a.Events))
		for i, e := range a.Events {
			require.Equal(t, e.Raw, upgrade.Answer.Events[i].Raw)
			require.Equal(t, e.Direction, upgrade.Answer.Events[i].Direction)
			require.Empty(t, upgrade.Answer.Events[i].Error)
			require.NotNil(t, upgrade.Answer.Events[i].Fields)
		}
		a = upgrade.Answer
	}
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
func dlmsWires(t *testing.T, id string) [][]byte {
	t.Helper()
	for _, c := range dlmsControls(t) {
		if c.ID == id {
			_, a := dlmsInput(t, c)
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
func TestDLMSSealedReplay(t *testing.T) {
	for _, c := range dlmsControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, want := dlmsInput(t, c)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("workers%d/deferred%t/observer%t", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var assembly TCPReassemblyStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithProtocolDecodeAs("udp", 4059, "dlms"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s })}
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
								require.Equal(t, "dlms", e.Protocol)
								require.Equal(t, "dlms-hdlc-get-normal", e.Profile)
								require.Equal(t, "udp", e.Transport)
								completion := "message"
								if w.Error == "ResourceExceeded" {
									completion = "limited"
								} else if w.Error == "MalformedMessage" {
									completion = "malformed"
								} else if w.Error == "UnsupportedFeature" || w.Error == "ContextRequired" {
									completion = "context-required"
								}
								require.Equal(t, completion, e.Completeness)
								require.Equal(t, "explicit-decode-as", e.Admission)
								require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
								require.Equal(t, w.Direction, e.Direction)
								require.Zero(t, e.TransactionID)
								src, dst := "192.0.2.10:39200", "192.0.2.20:4059"
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
									if data, ok := f["Data Value"].([]byte); ok && len(data) > 0 {
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

func dlmsCheckEvents(t *testing.T, want bsapAnswer, events []*ProtocolEvent) {
	t.Helper()
	require.Len(t, events, len(want.Events))
	for i, e := range events {
		w := want.Events[i]
		require.Equal(t, "dlms", e.Protocol)
		require.Equal(t, "dlms-hdlc-get-normal", e.Profile)
		require.Equal(t, w.Direction, e.Direction)
		require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
		if w.Reply == nil {
			require.Zero(t, e.ResponseTo)
		} else {
			require.Equal(t, events[*w.Reply].ID, e.ResponseTo)
		}
		if w.Error != "" {
			rocTypedError(t, w.Error, e.sessionError)
			require.Nil(t, e.Session)
			require.Nil(t, e.Structured)
			_, err := e.GetFields()
			rocTypedError(t, w.Error, err)
		} else {
			require.Empty(t, e.Error)
			rocEqualFields(t, w.Fields, e.Session)
			fields, err := e.GetFields()
			require.NoError(t, err)
			rocEqualFields(t, w.Fields, fields)
			fields["Observation"] = "caller-mutated"
			if value, ok := fields["Data Value"].([]byte); ok && len(value) > 0 {
				value[0] ^= 255
			}
			rocEqualFields(t, w.Fields, e.Session)
			v, err := e.Decode()
			require.NoError(t, err)
			rocEqualFields(t, w.Fields, protocolFields(v))
		}
	}
}

func TestDLMSTCPCarriersAndChunks(t *testing.T) {
	dlmsControls(t)
	b, err := trafficfixture.ReadFile("dlms-hdlc/manifest.json")
	require.NoError(t, err)
	var m struct{ Carriers []bsapControl }
	require.NoError(t, json.Unmarshal(b, &m))
	require.Len(t, m.Carriers, 12)
	for _, c := range m.Carriers {
		t.Run(c.ID, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile("dlms-hdlc/" + c.Capture)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			ab, err := trafficfixture.ReadFile("dlms-hdlc/" + c.Answer)
			require.NoError(t, err)
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ab)))
			var want bsapAnswer
			require.NoError(t, json.Unmarshal(ab, &want))
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observer := range []bool{false, true} {
						t.Run(fmt.Sprintf("workers%d/deferred%t/observer%t", workers, deferred, observer), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var assembly TCPReassemblyStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s })}
							if observer {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							dlmsCheckEvents(t, want, events)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							require.Zero(t, assembly.UnreassembledBytes)
							require.EqualValues(t, c.Packets, assembly.CapturedPackets)
							if observer {
								require.EqualValues(t, c.Packets, seen.Load())
							}
							for _, e := range events {
								require.Equal(t, "tcp", e.Transport)
								require.NotEmpty(t, e.SourceBytes.PacketRefs)
								for _, ref := range e.SourceBytes.PacketRefs {
									require.GreaterOrEqual(t, ref.Number, uint64(4))
									require.LessOrEqual(t, ref.Number, uint64(c.Packets))
									require.Equal(t, e.Domain, ref.Domain)
								}
							}
						})
					}
				}
			}
			// Feed uses the same framer/session and cannot rely on a packet boundary.
			for _, chunk := range []int{1, 3, 2049} {
				s, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("tcp"))
				require.NoError(t, err)
				var events []*ProtocolEvent
				for _, e := range want.Events {
					w, err := hex.DecodeString(e.Raw)
					require.NoError(t, err)
					for at := 0; at < len(w); at += chunk {
						v := bytes.Clone(w[at:min(len(w), at+chunk)])
						out := s.Feed(e.Direction, time.Unix(1700020000, 0), v)
						clear(v)
						events = append(events, out.Events...)
					}
				}
				events = append(events, s.Close("finished")...)
				dlmsCheckEvents(t, want, events)
				require.Zero(t, s.Stats().BufferedBytes)
				require.Empty(t, s.Close("idempotent"))
			}
		})
	}
}

func TestDLMSResourceIsolationAndClose(t *testing.T) {
	w := dlmsWires(t, "get-u8")
	q, r := w[0], w[1]
	newSession := func(b ParserBudget) *captureSession {
		s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(39200, 4059))
		require.NoError(t, err)
		return s.(*captureSession)
	}
	base := 512 + 6*len(q)
	for _, delta := range []int{-1, 0} {
		b := DefaultParserBudget()
		b.MaxFrameBytes, b.MaxMessageBytes = 64, 64
		b.MaxBufferedBytes = base + delta
		s := newSession(b)
		out := s.Feed(0, time.Unix(1700020000, 0), q)
		if delta < 0 {
			rocTypedError(t, "ResourceExceeded", out.Err)
			require.Zero(t, s.Stats().BufferedBytes)
		} else {
			require.Nil(t, out.Err)
			require.EqualValues(t, base, s.Stats().BufferedBytes)
		}
		s.Close("bounded")
		require.Zero(t, s.Stats().BufferedBytes)
		require.Nil(t, s.f.a.udpSessions)
		s.Close("idempotent")
		require.Zero(t, s.Stats().BufferedBytes)
	}
	t.Run("resource-denied-new-request-retires-pending", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxFrameBytes, b.MaxMessageBytes = 64, 64
		b.MaxBufferedBytes = base
		s := newSession(b)
		a := s.Feed(0, time.Unix(1700020000, 0), q)
		require.Nil(t, a.Err)
		big := dlmsWires(t, "link-negotiated")[0]
		out := s.Feed(0, time.Unix(1700020001, 0), big)
		rocTypedError(t, "ResourceExceeded", out.Err)
		late := s.Feed(1, time.Unix(1700020002, 0), r)
		rocTypedError(t, "ContextRequired", late.Err)
		require.Zero(t, late.Events[0].ResponseTo)
		require.Nil(t, late.Events[0].Session)
		s.Close("pressure")
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("frame-limit-omits-raw", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxFrameBytes = len(q) - 1
		s := newSession(b)
		e := &ProtocolEvent{Source: "a:39200", Destination: "b:4059", Timestamp: time.Unix(1700020000, 0)}
		require.True(t, s.f.a.decodeDLMSDatagram(e, q, true))
		rocTypedError(t, "ResourceExceeded", e.sessionError)
		require.Empty(t, e.Raw)
		require.Nil(t, e.Session)
		s.Close("bound")
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("retained-token-collection-limit", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxCollectionElements = 1
		s := newSession(b)
		w := dlmsWires(t, "invoke-reuse")
		require.Nil(t, s.Feed(0, time.Unix(1700020000, 0), w[0]).Err)
		require.Nil(t, s.Feed(1, time.Unix(1700020001, 0), w[1]).Err)
		out := s.Feed(0, time.Unix(1700020002, 0), w[2])
		rocTypedError(t, "ResourceExceeded", out.Err)
		late := s.Feed(1, time.Unix(1700020003, 0), w[3])
		rocTypedError(t, "ContextRequired", late.Err)
		require.Zero(t, late.Events[0].ResponseTo)
		s.Close("collection")
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("observed-domain-endpoint-flow-limit-idle", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxCollectionElements = 1
		s := newSession(b)
		a := s.f.a
		ts := time.Unix(1700020000, 0)
		makeEvent := func(src, dst string, d CaptureDomain, stamp time.Time) *ProtocolEvent {
			return &ProtocolEvent{Source: src, Destination: dst, Domain: d, Transport: "udp", Timestamp: stamp}
		}
		d0, d1 := CaptureDomain{Section: 1, Interface: 0}, CaptureDomain{Section: 1, Interface: 1}
		first := makeEvent("a:39200", "b:4059", d0, ts)
		require.True(t, a.decodeDLMSDatagram(first, q, false))
		require.Nil(t, first.sessionError)
		for _, wrong := range []*ProtocolEvent{makeEvent("b:4059", "a:39200", d1, ts), makeEvent("c:4059", "a:39200", d0, ts)} {
			require.True(t, a.decodeDLMSDatagram(wrong, r, true))
			rocTypedError(t, "ContextRequired", wrong.sessionError)
			require.Zero(t, wrong.ResponseTo)
		}
		full := makeEvent("c:39200", "d:4059", d0, ts)
		require.True(t, a.decodeDLMSDatagram(full, q, false))
		rocTypedError(t, "ResourceExceeded", full.sessionError)
		require.Len(t, a.udpSessions.entries, 1)
		good := makeEvent("b:4059", "a:39200", d0, ts)
		require.True(t, a.decodeDLMSDatagram(good, r, false))
		require.Nil(t, good.sessionError)
		require.Equal(t, first.ID, good.ResponseTo)
		expired := makeEvent("b:4059", "a:39200", d0, ts.Add(dlmsIdleTTL))
		require.True(t, a.decodeDLMSDatagram(expired, r, true))
		rocTypedError(t, "ContextRequired", expired.sessionError)
		require.Empty(t, a.udpSessions.entries)
		require.Zero(t, a.stats().BufferedBytes)
		s.Close("domains")
	})
	t.Run("unmatched-TCP-close", func(t *testing.T) {
		s, err := NewProtocolSessionWithOptions(ParserBudget{}, WithSessionTransport("tcp"))
		require.NoError(t, err)
		out := s.Feed(0, time.Unix(1700020000, 0), q)
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		closed := s.Close("FIN")
		require.Len(t, closed, 1)
		require.Equal(t, "incomplete", closed[0].Status)
		require.EqualValues(t, 1, closed[0].Session["Outstanding"])
		require.Zero(t, s.Stats().BufferedBytes)
		require.Empty(t, s.Close("RST"))
	})
}
