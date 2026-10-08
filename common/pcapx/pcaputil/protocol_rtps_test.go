package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type rtpsControl struct {
	ID, Capture, Answer, SHA256 string
	AnswerSHA                   string `json:"answer_sha256"`
	Packets                     int
}
type rtpsControlAnswer struct {
	Wires  []string
	Events []struct {
		Protocol, Error, Raw string
		Fields               map[string]any
	}
	PureErrors      []string `json:"pure_errors"`
	MinimumElements *int     `json:"minimum_elements"`
	Packets         int
}

func rtpsControls(t *testing.T) []rtpsControl {
	t.Helper()
	raw, err := trafficfixture.ReadFile("rtps-spdp/manifest.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []rtpsControl
	}
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Equal(t, "pr5013-rtps-spdp/v1", m.Schema)
	require.Len(t, m.Cases, 78)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	found := map[string]trafficfixture.FrozenCase{}
	for _, b := range all {
		for _, c := range b.Cases {
			found[c.ID] = c
		}
	}
	for _, c := range m.Cases {
		v, ok := found["rtps-spdp/"+c.ID]
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
func rtpsInput(t *testing.T, c rtpsControl) ([]byte, rtpsControlAnswer) {
	t.Helper()
	raw, err := trafficfixture.ReadFile("rtps-spdp/" + c.Capture)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	a, err := trafficfixture.ReadFile("rtps-spdp/" + c.Answer)
	require.NoError(t, err)
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(a)))
	var want rtpsControlAnswer
	require.NoError(t, json.Unmarshal(a, &want))
	require.Equal(t, c.Packets, want.Packets)
	r, err := NewCaptureReader(bytes.NewReader(raw))
	require.NoError(t, err)
	for i, w := range want.Wires {
		b, _, err := r.ReadPacketData()
		require.NoError(t, err)
		p := gopacket.NewPacket(b, r.LinkType(), gopacket.Default)
		u := p.Layer(layers.LayerTypeUDP).(*layers.UDP)
		require.Equal(t, w, hex.EncodeToString(u.Payload), "wire %d", i)
	}
	_, _, err = r.ReadPacketData()
	require.ErrorIs(t, err, io.EOF)
	return raw, want
}
func TestRTPSSPDPSealedReplay(t *testing.T) {
	for _, c := range rtpsControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, want := rtpsInput(t, c)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						for _, explicit := range []bool{false, true} {
							t.Run(fmt.Sprintf("w%d/deferred%v/observer%v/explicit%v", workers, deferred, observe, explicit), func(t *testing.T) {
								var events []*ProtocolEvent
								var stats ProtocolStats
								var assembly TCPReassemblyStats
								var seen atomic.Int64
								opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s })}
								if observe {
									opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
								}
								if explicit {
									opts = append(opts, WithProtocolDecodeAs("udp", 28400, "rtps"))
								}
								input := bytes.Clone(raw)
								require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
								clear(input)
								require.Len(t, events, len(want.Events))
								require.Zero(t, stats.BufferedBytes)
								require.Zero(t, stats.CallbackPanics)
								require.Zero(t, assembly.UnreassembledBytes)
								require.Zero(t, assembly.UnreassembledSegments)
								require.EqualValues(t, c.Packets, assembly.CapturedPackets)
								if observe {
									require.EqualValues(t, c.Packets, seen.Load())
								}
								for i, e := range events {
									answer := want.Events[i]
									require.Equal(t, answer.Protocol, e.Protocol)
									require.Equal(t, answer.Raw, hex.EncodeToString(e.Raw), "complete admitted PDU or the documented 64-byte unknown probe prefix")
									require.Equal(t, "udp", e.Transport)
									require.Equal(t, "192.0.2.10:38000", e.Source)
									require.Equal(t, "192.0.2.20:28400", e.Destination)
									require.Len(t, e.SourceBytes.PacketRefs, 1)
									require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
									require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
									require.Zero(t, e.ResponseTo)
									require.Zero(t, e.TransactionID)
									if answer.Fields != nil {
										require.Contains(t, []string{"decoded", "deferred"}, e.Status)
										require.Empty(t, e.Error)
										if explicit {
											require.Equal(t, "explicit-decode-as", e.Admission)
										} else {
											require.Equal(t, "wire-signature", e.Admission)
										}
										f, err := e.GetFields()
										require.NoError(t, err)
										b, err := json.Marshal(f)
										require.NoError(t, err)
										var got map[string]any
										require.NoError(t, json.Unmarshal(b, &got))
										require.Equal(t, answer.Fields, got)
										// GetFields/Session/Raw are independently owned snapshots after replay.
										session, err := json.Marshal(e.Session)
										require.NoError(t, err)
										require.JSONEq(t, string(b), string(session))
										f["Observation"] = "caller-mutated"
										require.Equal(t, "unverified-participant", e.Session["Observation"])
									} else if answer.Error != "" {
										var pe *ProtocolError
										require.Error(t, e.sessionError)
										require.True(t, errors.As(e.sessionError, &pe))
										require.Equal(t, answer.Error, string(pe.Kind))
										require.NotEmpty(t, e.Error)
										require.Nil(t, e.Structured)
										require.Nil(t, e.Fields)
										require.Nil(t, e.Session)
									} else {
										require.Equal(t, "unrecognized", e.Status)
										require.Empty(t, e.Error)
										require.Nil(t, e.Structured)
									}
								}
							})
						}
					}
				}
			}
		})
	}
}

func TestRTPSSPDPProbeBudgetAndLifetime(t *testing.T) {
	for _, c := range rtpsControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, want := rtpsInput(t, c)
			for i, encoded := range want.Wires {
				wire, err := hex.DecodeString(encoded)
				require.NoError(t, err)
				result, err := decodeRTPSDatagram(wire, 4096)
				if want.PureErrors[i] != "" {
					require.Nil(t, result)
					var pe *ProtocolError
					require.Error(t, err)
					require.True(t, errors.As(err, &pe))
					require.Equal(t, want.PureErrors[i], string(pe.Kind))
				} else {
					require.NoError(t, err)
					b, err := json.Marshal(result)
					require.NoError(t, err)
					var got map[string]any
					require.NoError(t, json.Unmarshal(b, &got))
					require.Equal(t, want.Events[i].Fields, got)
				}
				budget := DefaultParserBudget()
				s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"))
				require.NoError(t, err)
				for probe := 0; probe < 3; probe++ {
					p := s.Probe(wire)
					if want.PureErrors[i] == "" {
						require.Equal(t, ProbeAccept, p.Verdict)
						require.Equal(t, "rtps", p.Protocol)
					} else {
						require.Equal(t, ProbeReject, p.Verdict)
					}
					require.Zero(t, s.Stats().Messages)
					require.Zero(t, s.Stats().BufferedBytes)
				}
				ts := time.Unix(1700000000, 0)
				invalid := s.Feed(2, ts, wire)
				require.Zero(t, invalid.Consumed)
				require.Empty(t, invalid.Events)
				require.NotNil(t, invalid.Err)
				require.Equal(t, ErrMalformedMessage, invalid.Err.Kind)
				require.Zero(t, s.Stats().Messages)
				r := s.Feed(i%2, ts, wire)
				require.Len(t, r.Events, 1)
				require.Equal(t, want.Events[i].Protocol, r.Events[0].Protocol)
				clear(wire)
				require.Empty(t, s.Close("fixture-end"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
				closed := s.Feed(0, ts, []byte{1})
				require.Equal(t, "closed", closed.State)
				require.NotNil(t, closed.Err)
				require.Equal(t, ErrFatalSessionError, closed.Err.Kind)
			}
			if want.MinimumElements != nil {
				wire, err := hex.DecodeString(want.Wires[0])
				require.NoError(t, err)
				n := *want.MinimumElements
				for _, limit := range []int{n - 1, n} {
					out, err := decodeRTPSDatagram(wire, limit)
					if limit < n {
						require.Nil(t, out)
						var pe *ProtocolError
						require.Error(t, err)
						require.True(t, errors.As(err, &pe))
						require.Equal(t, ErrResourceExceeded, pe.Kind)
					} else {
						require.NoError(t, err)
						require.NotNil(t, out)
					}
				}
				for _, limit := range []int{len(wire) - 1, len(wire)} {
					budget := DefaultParserBudget()
					budget.MaxMessageBytes = limit
					s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"))
					require.NoError(t, err)
					r := s.Feed(0, time.Unix(1700000000, 0), wire)
					require.Len(t, r.Events, 1)
					if limit < len(wire) {
						require.Equal(t, "limited", r.Events[0].Status)
						require.NotEqual(t, "rtps", r.Events[0].Protocol)
					} else {
						require.Equal(t, "rtps", r.Events[0].Protocol)
						require.Equal(t, "decoded", r.Events[0].Status)
					}
					s.Close("end")
					require.Zero(t, s.Stats().BufferedBytes)
				}
				tcp, err := NewProtocolSession(DefaultParserBudget())
				require.NoError(t, err)
				require.NotEqual(t, ProbeAccept, tcp.Probe(wire).Verdict)
				r := tcp.Feed(0, time.Unix(1700000000, 0), wire)
				for _, e := range r.Events {
					require.NotEqual(t, "rtps", e.Protocol)
				}
				tcp.Close("end")
				require.Zero(t, tcp.Stats().BufferedBytes)
			}
		})
	}
}
