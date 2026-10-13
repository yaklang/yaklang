package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type j1939TPAnswer struct {
	Events []struct {
		Wire       string
		Refs       []uint64
		Fields     map[string]any
		Error, Why string
		Close      bool
		Truncated  bool
		MS         int64
		Packet     int
	}
}

func j1939TPFile(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("J1939_TP_TEST_ROOT"); root != "" {
		b, e := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, e)
		return b
	}
	b, e := trafficfixture.ReadFile("j1939-transport/" + name)
	require.NoError(t, e)
	return b
}
func TestJ1939TPSealedReplay(t *testing.T) {
	var manifest struct {
		Cases []struct {
			ID, Capture, Answer, SHA256 string
			AnswerSHA                   string `json:"answer_sha256"`
			Packets                     int
		}
	}
	require.NoError(t, json.Unmarshal(j1939TPFile(t, "manifest.json"), &manifest))
	require.Len(t, manifest.Cases, 48)
	var frozen map[string]trafficfixture.FrozenCase
	if os.Getenv("J1939_TP_TEST_ROOT") == "" {
		frozen = make(map[string]trafficfixture.FrozenCase)
		all, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		for _, batch := range all {
			for _, c := range batch.Cases {
				frozen[c.ID] = c
			}
		}
	}
	for _, c := range manifest.Cases {
		t.Run(c.ID, func(t *testing.T) {
			if frozen != nil {
				v, ok := frozen["j1939-transport/"+c.ID]
				require.True(t, ok)
				require.Equal(t, c.SHA256, v.Input.SHA256)
				require.Equal(t, "j1939-transport/"+c.Capture, v.Input.File)
				require.Equal(t, c.Packets, v.Facts.PacketCount)
				require.Len(t, v.Expectations, 1)
				var binding struct {
					Answer string `json:"answer_file"`
					SHA    string `json:"answer_sha256"`
				}
				require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &binding))
				require.Equal(t, c.Answer, binding.Answer)
				require.Equal(t, c.AnswerSHA, binding.SHA)
			}
			capture, answer := j1939TPFile(t, c.Capture), j1939TPFile(t, c.Answer)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(capture)))
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
			var want j1939TPAnswer
			require.NoError(t, decodeExactProtocolAnswer(answer, &want))
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
							var es []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithCANDecodeAs(0, "j1939-tp"), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) {
									w, e := hex.DecodeString(want.Events[seen].Wire)
									require.NoError(t, e)
									require.Equal(t, w, p.Data())
									seen++

								}))
							}
							input := bytes.Clone(capture)
							err := ReplayPcap(bytes.NewReader(input), opts...)
							if c.ID == "truncated-record" {
								require.EqualError(t, err, "capture contains truncated packets (captured 16 of 17 bytes)")
							} else {
								require.NoError(t, err)
							}
							clear(input)
							require.Len(t, es, len(want.Events))
							require.Zero(t, stats.BufferedBytes)
							if observe {
								require.Equal(t, c.Packets, seen)
							}
							var decoded, incomplete, context, malformed uint64
							for i, v := range want.Events {
								e := es[i]
								require.EqualValues(t, i+1, e.ID)
								require.Equal(t, "j1939", e.Protocol)
								require.Equal(t, j1939TPProfile, e.Profile)
								require.Equal(t, "can", e.Transport)
								require.Equal(t, "explicit-interface-decode-as", e.Admission)
								require.Zero(t, e.FlowID)
								require.Zero(t, e.TransactionID)
								require.Zero(t, e.ResponseTo)
								require.Empty(t, e.Source)
								require.Empty(t, e.Destination)
								require.Equal(t, CaptureDomain{}, e.Domain)
								w, err := hex.DecodeString(v.Wire)
								require.NoError(t, err)
								if v.Close || v.Truncated {
									require.Empty(t, e.Raw)
								} else {
									require.Equal(t, w, e.Raw)
								}
								require.Len(t, e.SourceBytes.PacketRefs, len(v.Refs))
								for j, n := range v.Refs {
									require.Equal(t, n, e.SourceBytes.PacketRefs[j].Number)
									require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[j].Domain)
								}
								if v.Close || len(v.Refs) > 1 {
									require.Equal(t, "reassembled", e.SourceBytes.Kind)
								} else {
									require.Equal(t, "captured", e.SourceBytes.Kind)
								}
								if v.Error != "" {
									switch v.Error {
									case "NeedMore":
										incomplete++
									case "ContextRequired", "UnsupportedFeature":
										context++
									case "MalformedMessage":
										malformed++
									}
									rocTypedError(t, v.Error, e.sessionError)
									require.Contains(t, e.Error, v.Why)
									require.Nil(t, e.Session)
									require.Nil(t, e.Structured)
									e.Protocol, e.Error, e.Status = "http", "", "decoded"
									e.Session = map[string]any{"forged": true}
									for _, fn := range []func() (map[string]any, error){e.GetFields, e.Decode} {
										f, err := fn()
										require.Nil(t, f)
										rocTypedError(t, v.Error, err)
									}
								} else {
									decoded++
									require.Empty(t, e.Error)
									status := "decoded"
									if deferred {
										status = "deferred"
									}
									require.Equal(t, status, e.Status)
									require.Equal(t, "message", e.Completeness)
									rocEqualFields(t, v.Fields, e.Session)
									for _, fn := range []func() (map[string]any, error){e.GetFields, e.Decode} {
										f, err := fn()
										require.NoError(t, err)
										if _, ok := f["fields"]; ok {
											f = protocolFields(f)
										}
										rocEqualFields(t, v.Fields, f)
										clear(f)
									}
									f, err := e.GetFields()
									require.NoError(t, err)
									f["Data"].([]byte)[0] ^= 255
									f["J1939"].(map[string]any)["Target PGN"] = 999999
									rocEqualFields(t, v.Fields, e.semanticSession)
									rocEqualFields(t, v.Fields, e.Session)
								}
							}
							require.Equal(t, incomplete, stats.Incomplete)
							require.Equal(t, context, stats.ContextRequired)
							require.Equal(t, malformed, stats.Malformed)
							if deferred {
								require.Equal(t, decoded, stats.Deferred)
								require.Zero(t, stats.Decoded)
							} else {
								require.Equal(t, decoded, stats.Decoded)
								require.Zero(t, stats.Deferred)
							}
						})
					}
				}
			}
		})
	}
}

func j1939TPParser(es *[]*ProtocolEvent, b ParserBudget) *binParser {
	return &binParser{budget: b, canDecodeAs: map[int]string{0: "j1939-tp", 1: "j1939-tp"}, config: BinParserConfig{MaxBufferedBytes: b.MaxBufferedBytes, OnEvent: func(e *ProtocolEvent) { *es = append(*es, e) }}}
}
func j1939TPFrames(t *testing.T, name string) [][]byte {
	var a j1939TPAnswer
	require.NoError(t, decodeExactProtocolAnswer(j1939TPFile(t, "answers/"+name+".json"), &a))
	var out [][]byte
	for _, e := range a.Events {
		if !e.Close {
			w, err := hex.DecodeString(e.Wire)
			require.NoError(t, err)
			out = append(out, w)
		}
	}
	return out
}
func TestJ1939TPDomainsResourcesAndOwnership(t *testing.T) {
	frames := j1939TPFrames(t, "bam-nine")
	now := time.Unix(1700001000, 0)
	t.Run("Domains", func(t *testing.T) {
		var es []*ProtocolEvent
		a := j1939TPParser(&es, DefaultParserBudget())
		ds := []CaptureDomain{{}, {Interface: 1}, {Section: 1}, {Encapsulation: "vpn/1"}}
		for i, w := range frames {
			for j, d := range ds {
				raw := bytes.Clone(w)
				droneCANStatsDeliver(a, raw, uint64(i*4+j+1), d, now.Add(time.Duration(i)*50*time.Millisecond))
				clear(raw)
			}
		}
		require.Len(t, es, 12)
		for j, d := range ds {
			e := es[8+j]
			require.Equal(t, d, e.Domain)
			require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}, e.Session["J1939"].(map[string]any)["Transfer Payload"])
			require.Len(t, e.SourceBytes.PacketRefs, 3)
			for i, r := range e.SourceBytes.PacketRefs {
				require.Equal(t, d, r.Domain)
				require.EqualValues(t, i*4+j+1, r.Number)
			}
		}
		a.closeJ1939TP()
		a.closeJ1939TP()
		require.Zero(t, a.buffered.Load())
		require.Len(t, es, 12)
	})
	t.Run("RefusedDomainIsolation", func(t *testing.T) {
		var es []*ProtocolEvent
		a := j1939TPParser(&es, DefaultParserBudget())
		ds := []CaptureDomain{{}, {Interface: 1}}
		for j, d := range ds {
			droneCANStatsDeliver(a, frames[0], uint64(j+1), d, now)
		}
		a.decodeCANRecord(frames[1], captureEvidence{Ref: PacketReference{Number: 3, Domain: ds[0]}}, gopacket.CaptureInfo{Timestamp: now, CaptureLength: 16, Length: 17})
		for i, w := range frames[1:] {
			droneCANStatsDeliver(a, w, uint64(i+4), ds[1], now.Add(time.Duration(i+1)*50*time.Millisecond))
		}
		require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}, es[len(es)-1].Session["J1939"].(map[string]any)["Transfer Payload"])
		a.closeJ1939TP()
		require.Zero(t, a.buffered.Load())
	})
	for _, name := range []string{"Message", "Buffer", "Collection", "Contributors"} {
		t.Run(name, func(t *testing.T) {
			b := DefaultParserBudget()
			switch name {
			case "Message":
				b.MaxMessageBytes = 8
			case "Buffer":
				b.MaxBufferedBytes = 300
			case "Collection":
				b.MaxCollectionElements = 1
			case "Contributors":
				b.MaxCollectionElements = 2
			}
			var es []*ProtocolEvent
			a := j1939TPParser(&es, b)
			for i, w := range frames {
				droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now.Add(time.Duration(i)*50*time.Millisecond))
			}
			rocTypedError(t, "ResourceExceeded", es[2].sessionError)
			for _, e := range es {
				if e.Session != nil {
					jf := e.Session["J1939"].(map[string]any)
					require.NotEqual(t, true, jf["Transfer Complete"])
				}
			}
			require.LessOrEqual(t, a.peak.Load(), int64(b.MaxBufferedBytes))
			a.closeJ1939TP()
			a.closeJ1939TP()
			require.Zero(t, a.buffered.Load())
			require.Nil(t, a.j1939TP)
		})
	}
}

func TestJ1939TPControlIsolationAndPhaseBounds(t *testing.T) {
	t.Run("CompletedWindowWaitsForCTS", func(t *testing.T) {
		var es []*ProtocolEvent
		a := j1939TPParser(&es, DefaultParserBudget())
		frames := j1939TPFrames(t, "rts-two-windows")
		now := time.Unix(1700001000, 0)
		times := []time.Duration{0, 50 * time.Millisecond, 100 * time.Millisecond, 1100 * time.Millisecond, 1150 * time.Millisecond, 1200 * time.Millisecond}
		for i, w := range frames {
			droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now.Add(times[i]))
		}
		require.Len(t, es, 6)
		require.Nil(t, es[3].sessionError, "Linux v6.6 waits 1250ms for control after a complete granted window")
		require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}, es[5].Session["J1939"].(map[string]any)["Transfer Payload"])
		a.closeJ1939TP()
		require.Zero(t, a.buffered.Load())
	})

	frames := j1939TPFrames(t, "rts-complete")
	now := time.Unix(1700001000, 0)
	t.Run("OppositeTransfersAbortAmbiguity", func(t *testing.T) {
		var es []*ProtocolEvent
		a := j1939TPParser(&es, DefaultParserBudget())
		forward := frames[0]
		reverse := bytes.Clone(forward)
		reverse[2], reverse[3] = reverse[3], reverse[2]
		droneCANStatsDeliver(a, forward, 1, CaptureDomain{}, now)
		droneCANStatsDeliver(a, reverse, 2, CaptureDomain{}, now)
		abort := j1939TPFrames(t, "abort-129")[1]
		droneCANStatsDeliver(a, abort, 3, CaptureDomain{}, now)
		rocTypedError(t, "ContextRequired", es[2].sessionError)
		require.Contains(t, es[2].Error, "Abort direction is ambiguous")
		for _, s := range a.j1939TP.entries {
			require.False(t, s.active)
			require.True(t, s.blocked)
			require.Zero(t, s.payloadCost)
		}
		a.closeJ1939TP()
		require.Len(t, es, 3)
		require.Zero(t, a.buffered.Load())
	})
	t.Run("ActiveStartConflictCannotRestart", func(t *testing.T) {
		var es []*ProtocolEvent
		a := j1939TPParser(&es, DefaultParserBudget())
		for i, w := range [][]byte{frames[0], frames[0], frames[0], frames[1], frames[2]} {
			droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now.Add(time.Duration(i)*time.Millisecond))
		}
		for _, e := range es[1:] {
			require.Nil(t, e.Session)
			rocTypedError(t, "ContextRequired", e.sessionError)
		}
		a.closeJ1939TP()
		require.Zero(t, a.buffered.Load())
	})
	for _, delta := range []time.Duration{1250 * time.Millisecond, 1250*time.Millisecond + time.Microsecond} {
		t.Run(fmt.Sprintf("RTS-%d", delta), func(t *testing.T) {
			var es []*ProtocolEvent
			a := j1939TPParser(&es, DefaultParserBudget())
			droneCANStatsDeliver(a, frames[0], 1, CaptureDomain{}, now)
			droneCANStatsDeliver(a, frames[1], 2, CaptureDomain{}, now.Add(delta))
			if delta > 1250*time.Millisecond {
				rocTypedError(t, "ContextRequired", es[1].sessionError)
			} else {
				require.Nil(t, es[1].sessionError)
			}
			a.closeJ1939TP()
			require.Zero(t, a.buffered.Load())
		})
	}
	pause := j1939TPFrames(t, "rts-pause-resume")
	for _, delta := range []time.Duration{550 * time.Millisecond, 550*time.Millisecond + time.Microsecond} {
		t.Run(fmt.Sprintf("Pause-%d", delta), func(t *testing.T) {
			var es []*ProtocolEvent
			a := j1939TPParser(&es, DefaultParserBudget())
			droneCANStatsDeliver(a, pause[0], 1, CaptureDomain{}, now)
			droneCANStatsDeliver(a, pause[1], 2, CaptureDomain{}, now)
			droneCANStatsDeliver(a, pause[2], 3, CaptureDomain{}, now.Add(delta))
			if delta > 550*time.Millisecond {
				rocTypedError(t, "ContextRequired", es[2].sessionError)
			} else {
				require.Nil(t, es[2].sessionError)
			}
			a.closeJ1939TP()
			require.Zero(t, a.buffered.Load())
		})
	}
	t.Run("CompletedPayloadReleased", func(t *testing.T) {
		var es []*ProtocolEvent
		a := j1939TPParser(&es, DefaultParserBudget())
		for i, w := range frames {
			droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now.Add(time.Duration(i)*50*time.Millisecond))
		}
		s := a.j1939TP.entries[j1939TPKey{CaptureDomain{}, 128, 129}]
		require.Zero(t, s.payloadCost)
		require.Nil(t, s.body)
		require.Nil(t, s.refs)
		require.False(t, s.active)
		es[4].Session["J1939"].(map[string]any)["Transfer Payload"].([]byte)[0] = 255
		f, err := es[4].GetFields()
		require.NoError(t, err)
		require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}, f["J1939"].(map[string]any)["Transfer Payload"])
		a.closeJ1939TP()
		a.closeJ1939TP()
		require.Zero(t, a.buffered.Load())
	})
}

func TestJ1939TPRetainedHistoryAndForeignLedger(t *testing.T) {
	frames := j1939TPFrames(t, "bam-nine")
	now := time.Unix(1700001000, 0)
	t.Run("IdentityExhaustion", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxCollectionElements = 2
		var es []*ProtocolEvent
		a := j1939TPParser(&es, b)
		for i := 0; i < 3; i++ {
			w := bytes.Clone(frames[0])
			w[3] += byte(i)
			droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now)
		}
		rocTypedError(t, "ResourceExceeded", es[2].sessionError)
		droneCANStatsDeliver(a, frames[1], 4, CaptureDomain{}, now)
		rocTypedError(t, "ResourceExceeded", es[3].sessionError)
		a.closeJ1939TP()
		require.Zero(t, a.buffered.Load())
	})
	t.Run("DoesNotReleaseForeignReservation", func(t *testing.T) {
		var es []*ProtocolEvent
		a := j1939TPParser(&es, DefaultParserBudget())
		require.True(t, a.reserveEvidence(57))
		for i, w := range frames {
			droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now.Add(time.Duration(i)*50*time.Millisecond))
		}
		a.closeJ1939TP()
		a.closeJ1939TP()
		require.EqualValues(t, 57, a.buffered.Load())
		a.buffered.Add(-57)
		require.Zero(t, a.buffered.Load())
	})
	t.Run("NewExplicitStartOwnsNewPayload", func(t *testing.T) {
		var es []*ProtocolEvent
		a := j1939TPParser(&es, DefaultParserBudget())
		for i, w := range frames {
			droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now.Add(time.Duration(i)*50*time.Millisecond))
		}
		for i, w := range frames {
			w = bytes.Clone(w)
			if i > 0 {
				for j := 9; j < len(w); j++ {
					if w[j] != 255 {
						w[j] += 10
					}
				}
			}
			droneCANStatsDeliver(a, w, uint64(i+4), CaptureDomain{}, now.Add(time.Duration(i+4)*50*time.Millisecond))
		}
		require.Equal(t, []byte{11, 12, 13, 14, 15, 16, 17, 18, 19}, es[5].Session["J1939"].(map[string]any)["Transfer Payload"])
		require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}, es[2].Session["J1939"].(map[string]any)["Transfer Payload"])
		a.closeJ1939TP()
		require.Zero(t, a.buffered.Load())
	})
}
