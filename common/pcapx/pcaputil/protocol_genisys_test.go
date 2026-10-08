package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	binparser "github.com/yaklang/yaklang/common/bin-parser"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type genisysAnswer struct {
	Steps []struct {
		Dir  int
		Wire string
	}
	Events []struct {
		Fields             map[string]any
		Error, Raw, Status string
	}
	Packets int
}

func genisysControls(t *testing.T) []rocControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("genisys-wire/manifest.json")
	require.NoError(t, err)
	var m struct {
		Schema          string
		Cases, Adjacent []rocControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "pr5013-genisys-wire/v1", m.Schema)
	require.Len(t, m.Cases, 40)
	require.Len(t, m.Adjacent, 3)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	found := map[string]trafficfixture.FrozenCase{}
	for _, batch := range all {
		for _, c := range batch.Cases {
			found[c.ID] = c
		}
	}
	for _, c := range append(m.Cases, m.Adjacent...) {
		v, ok := found["genisys-wire/"+c.ID]
		require.True(t, ok)
		require.Equal(t, c.Capture, v.Input.File)
		require.Equal(t, c.SHA256, v.Input.SHA256)
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
func TestGenisysWireAdjacentAdmission(t *testing.T) {
	genisysControls(t) // Check every adjacent input/answer binding against the frozen inventory.
	b, err := trafficfixture.ReadFile("genisys-wire/manifest.json")
	require.NoError(t, err)
	var manifest struct{ Adjacent []rocControl }
	require.NoError(t, json.Unmarshal(b, &manifest))
	for _, c := range manifest.Adjacent {
		t.Run(c.ID, func(t *testing.T) {
			raw, _ := genisysInput(t, c)
			answer, err := trafficfixture.ReadFile("genisys-wire/" + c.Answer)
			require.NoError(t, err)
			var want struct {
				Metadata, Session, Fields map[string]any
				Steps                     []struct{ Wire string }
			}
			require.NoError(t, json.Unmarshal(answer, &want))
			w, err := hex.DecodeString(want.Steps[0].Wire)
			require.NoError(t, err)
			require.Equal(t, ProbeReject, probeGenisys(w, len(w)).Verdict)
			// Independently authored metadata checks the complete greeting.
			// Existing rule output also fixes its field projection across capture modes.
			greeting, err := binparser.ParseStructured(w, "application-layer.mysql_fields", "MySQLGreetingFields")
			require.NoError(t, err)
			metadata, ok := greeting["metadata"].(map[string]any)
			require.True(t, ok)
			rocEqualFields(t, want.Metadata, metadata)
			rocEqualFields(t, want.Fields, protocolFields(greeting))
			s, err := NewProtocolSession(ParserBudget{})
			require.NoError(t, err)
			require.Equal(t, "mysql", s.Probe(w).Protocol)
			s.Close("probe only")
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						var events []*ProtocolEvent
						var stats ProtocolStats
						opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolStats(func(v ProtocolStats) { stats = v }), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })}
						if observe {
							opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
						}
						require.NoError(t, ReplayPcap(bytes.NewReader(raw), opts...))
						require.Len(t, events, 2, "greeting plus unfinished handshake at capture close")
						e := events[0]
						require.Equal(t, "mysql", e.Protocol)
						require.Empty(t, e.Error)
						require.Equal(t, w, e.Raw)
						require.Equal(t, 1, e.Direction)
						rocEqualFields(t, want.Session, e.Session)
						decoded, err := e.Decode()
						require.NoError(t, err)
						rocEqualFields(t, want.Fields, protocolFields(decoded))
						require.Equal(t, "mysql", events[1].Protocol)
						require.Equal(t, "incomplete", events[1].Status)
						require.Equal(t, "handshake", events[1].Session["Phase"])
						require.Zero(t, stats.BufferedBytes)
					}
				}
			}
		})
	}
}
func genisysInput(t *testing.T, c rocControl) ([]byte, genisysAnswer) {
	t.Helper()
	b, err := trafficfixture.ReadFile("genisys-wire/" + c.Capture)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
	a, err := trafficfixture.ReadFile("genisys-wire/" + c.Answer)
	require.NoError(t, err)
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(a)))
	var want genisysAnswer
	require.NoError(t, json.Unmarshal(a, &want))
	return b, want
}
func genisysCheck(t *testing.T, want genisysAnswer, events []*ProtocolEvent) {
	t.Helper()
	require.Len(t, events, len(want.Events))
	var flow uint64
	for i, a := range want.Events {
		e := events[i]
		require.Equal(t, "genisys", e.Protocol)
		require.Equal(t, a.Raw, hex.EncodeToString(e.Raw))
		require.Equal(t, want.Steps[i].Dir, e.Direction)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo, "wire-only profile must not invent an acknowledgement association")
		if flow == 0 {
			flow = e.FlowID
		}
		require.Equal(t, flow, e.FlowID)
		if a.Fields != nil {
			require.Empty(t, e.Error)
			rocEqualFields(t, a.Fields, e.Session)
			fields, err := e.GetFields()
			require.NoError(t, err)
			rocEqualFields(t, a.Fields, fields)
			decoded, err := e.Decode()
			require.NoError(t, err)
			rocEqualFields(t, a.Fields, protocolFields(decoded))
			require.Equal(t, "genisys-tcp-observed-wire", e.Profile)
			fields["Observation"] = "caller-mutated"
			require.Equal(t, "unverified-wire-values", e.Session["Observation"])
			if pairs, ok := fields["Pairs"].([]map[string]any); ok && len(pairs) > 0 {
				pairs[0]["Value"] = 999
				require.NotEqualValues(t, 999, e.Session["Pairs"].([]map[string]any)[0]["Value"])
			}
		} else if a.Error != "" {
			rocTypedError(t, a.Error, e.sessionError)
			require.NotEmpty(t, e.Error)
			require.Nil(t, e.Session)
			require.Nil(t, e.Structured)
		} else {
			require.Equal(t, a.Status, e.Status)
			require.Nil(t, e.Session)
			require.Empty(t, e.Error)
		}
	}
}
func TestGenisysWireSealedReplay(t *testing.T) {
	for _, c := range genisysControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, want := genisysInput(t, c)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/deferred%v/observer%v", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var asm TCPReassemblyStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { asm = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
							}
							require.NoError(t, ReplayPcap(bytes.NewReader(raw), opts...))
							require.EqualValues(t, want.Packets, asm.CapturedPackets)
							if observe {
								require.EqualValues(t, want.Packets, seen.Load())
							}
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, asm.UnreassembledBytes)
							require.Zero(t, asm.UnreassembledSegments)
							for _, e := range events {
								require.NotEmpty(t, e.SourceBytes.PacketRefs)
								for _, ref := range e.SourceBytes.PacketRefs {
									require.Equal(t, e.Domain, ref.Domain)
									require.GreaterOrEqual(t, ref.Number, uint64(4))
									require.LessOrEqual(t, ref.Number, uint64(want.Packets))
								}
							}
							genisysCheck(t, want, events)
						})
					}
				}
			}
		})
	}
}
func TestGenisysWireBoundaries(t *testing.T) {
	require.Equal(t, uint16(0xbb3d), genisysCRC([]byte("123456789"), 0))
	require.Equal(t, uint16(0x4b37), genisysCRC([]byte("123456789"), 65535))
	require.Equal(t, uint16(0x4083), genisysCRC([]byte{251, 1}, 65535))
	for _, c := range genisysControls(t) {
		_, want := genisysInput(t, c)
		for _, a := range want.Events {
			if a.Fields == nil {
				continue
			}
			w, err := hex.DecodeString(a.Raw)
			require.NoError(t, err)
			for n := 0; n < len(w); n++ {
				p := probeGenisys(w[:n], len(w))
				require.NotEqual(t, ProbeAccept, p.Verdict)
				f, _, err := decodeGenisys(w[:n], 4096, -1)
				require.Error(t, err)
				require.Nil(t, f)
			}
			f, _, err := decodeGenisys(w, 4096, -1)
			require.NoError(t, err)
			rocEqualFields(t, a.Fields, f)
		}
	}
	// A short control cannot establish a protocol or consume a session.
	for _, w := range [][]byte{{241, 1, 246}, {251, 1, 246}, {244, 1, 246}} {
		s, err := NewProtocolSession(ParserBudget{})
		require.NoError(t, err)
		require.Equal(t, ProbeReject, s.Probe(w).Verdict)
		require.Zero(t, s.Stats().Messages)
		require.Empty(t, s.Close("probe-only"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}
func TestGenisysWireResourcesAndLifetime(t *testing.T) {
	var want genisysAnswer
	for _, c := range genisysControls(t) {
		if c.ID == "ordered-duplicate-addresses" {
			_, want = genisysInput(t, c)
		}
	}
	require.Len(t, want.Steps, 2)
	q, _ := hex.DecodeString(want.Steps[0].Wire)
	p, _ := hex.DecodeString(want.Steps[1].Wire)
	for _, chunks := range []int{1, 7, len(q) + len(p)} {
		s, err := NewProtocolSession(ParserBudget{})
		require.NoError(t, err)
		var events []*ProtocolEvent
		for _, st := range want.Steps {
			w, _ := hex.DecodeString(st.Wire)
			before := append([]byte(nil), w...)
			require.NotEqual(t, ProbeReject, s.Probe(w).Verdict)
			require.Equal(t, before, w)
			for at := 0; at < len(w); at += chunks {
				e := s.Feed(st.Dir, time.Unix(1700000000, 0), w[at:min(at+chunks, len(w))])
				if at+chunks < len(w) {
					require.NotNil(t, e.Err)
					require.Equal(t, ErrNeedMore, e.Err.Kind)
					require.True(t, e.NeedMore)
					require.Empty(t, e.Events)
				} else {
					require.Nil(t, e.Err)
				}
				require.Equal(t, min(chunks, len(w)-at), e.Consumed)
				events = append(events, e.Events...)
			}
			for i := range w {
				w[i] = 0
			}
		}
		events = append(events, s.Close("done")...)
		genisysCheck(t, want, events)
		require.Zero(t, s.Stats().BufferedBytes)
		require.Empty(t, s.Close("again"))
	}
	s, err := NewProtocolSession(ParserBudget{MaxCollectionElements: 1})
	require.NoError(t, err)
	require.Nil(t, s.Feed(0, time.Time{}, q).Err)
	got := s.Feed(1, time.Time{}, p)
	require.NotNil(t, got.Err)
	require.Equal(t, ErrResourceExceeded, got.Err.Kind)
	require.Nil(t, got.Events[0].Session)
	s.Close("limit")
	require.Zero(t, s.Stats().BufferedBytes)
	// The fixed dialect/direction state shares the same global budget as raw bytes.
	for _, limit := range []int{255, 256} {
		s, err = NewProtocolSession(ParserBudget{MaxFrameBytes: 64, MaxMessageBytes: 64, ProbeBytes: 64, MaxBufferedBytes: limit})
		require.NoError(t, err)
		got = s.Feed(0, time.Time{}, q)
		if limit == 255 {
			require.NotNil(t, got.Err)
			require.Equal(t, ErrResourceExceeded, got.Err.Kind)
		} else {
			require.Nil(t, got.Err)
			require.EqualValues(t, 256, s.Stats().BufferedBytes)
		}
		s.Close("limit")
		require.Zero(t, s.Stats().BufferedBytes)
	}
	s, err = NewProtocolSession(ParserBudget{MaxFrameBytes: len(q) - 1})
	require.NoError(t, err)
	require.Equal(t, ProbeReject, s.Probe(q).Verdict)
	s.Feed(0, time.Time{}, q)
	s.Close("frame limit")
	require.Zero(t, s.Stats().BufferedBytes)
	// Two independently created sessions can select different dialects without sharing state.
	a, _ := NewProtocolSession(ParserBudget{})
	b, _ := NewProtocolSession(ParserBudget{})
	q0 := []byte{251, 1, 130, 240, 0, 246}
	require.Nil(t, a.Feed(0, time.Time{}, q).Err)
	require.Nil(t, b.Feed(1, time.Time{}, q0).Err)
	require.NotNil(t, a.Feed(0, time.Time{}, q0).Err)
	require.Nil(t, b.Feed(1, time.Time{}, q0).Err)
	a.Close("done")
	b.Close("done")
	require.Zero(t, a.Stats().BufferedBytes)
	require.Zero(t, b.Stats().BufferedBytes)
}
