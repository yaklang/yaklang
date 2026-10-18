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
	"testing"
	"time"
)

func TestCANUDSParameterSelectedAPI(t *testing.T) {
	for _, c := range []struct {
		name     string
		req, ack []byte
	}{{"Routine", []byte{5, 0x31, 1, 0x12, 0x34, 0xab}, []byte{5, 0x71, 1, 0x12, 0x34, 0xcd}}, {"Communication", []byte{3, 0x28, 0, 3}, []byte{2, 0x68, 0}}, {"EnhancedNode", []byte{5, 0x28, 4, 3, 0x45, 0x67}, []byte{2, 0x68, 4}}, {"DTCSetting", []byte{3, 0x85, 1, 0xaa}, []byte{2, 0xc5, 1}}} {
		t.Run(c.name, func(t *testing.T) {
			es, st := canISOReplay(t, canISOBase(), []canISOTapeFrame{{0x7e0, c.req, 0}, {0x7e8, c.ack, 10}})
			require.Len(t, es, 2)
			require.Empty(t, es[0].Error)
			require.Empty(t, es[1].Error)
			require.EqualValues(t, 1, es[1].ResponseTo)
			require.Zero(t, st.BufferedBytes)
		})
	}
	t.Run("AdjacentTesterPresent", func(t *testing.T) {
		es, st := canISOReplay(t, canISOBase(), []canISOTapeFrame{{0x7e0, []byte{2, 0x3e, 0}, 0}, {0x7e8, []byte{2, 0x7e, 0}, 10}})
		require.Len(t, es, 2)
		require.Empty(t, es[1].Error)
		require.EqualValues(t, 1, es[1].ResponseTo)
		require.Zero(t, st.BufferedBytes)
	})
}

func canUDSParameterFile(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("CAN_UDS_PARAMETER_TEST_ROOT"); root != "" {
		b, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return b
	}
	b, err := trafficfixture.ReadFile("can-uds-parameter/" + name)
	require.NoError(t, err)
	return b
}
func TestCANUDSParameterSealedMatrix(t *testing.T) {
	var m struct {
		Cases []struct {
			ID, Capture, Answer, SHA256 string
			AnswerSHA                   string `json:"answer_sha256"`
			Packets                     int
		}
	}
	require.NoError(t, json.Unmarshal(canUDSParameterFile(t, "manifest.json"), &m))
	require.Len(t, m.Cases, 40)
	var frozen map[string]trafficfixture.FrozenCase
	if os.Getenv("CAN_UDS_PARAMETER_TEST_ROOT") == "" {
		frozen = make(map[string]trafficfixture.FrozenCase)
		all, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		for _, batch := range all {
			for _, c := range batch.Cases {
				frozen[c.ID] = c
			}
		}
	}
	for _, c := range m.Cases {
		t.Run(c.ID, func(t *testing.T) {
			if frozen != nil {
				v, ok := frozen["can-uds-parameter/"+c.ID]
				require.True(t, ok)
				require.Equal(t, c.SHA256, v.Input.SHA256)
				require.Equal(t, "can-uds-parameter/"+c.Capture, v.Input.File)
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
			capture, answer := canUDSParameterFile(t, c.Capture), canUDSParameterFile(t, c.Answer)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(capture)))
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
			var a struct {
				Options CANISOTransport
				Events  []struct {
					Wire, Error string
					Fields      map[string]any
					Refs        []uint64
					Direction   int
					ResponseTo  uint64 `json:"response_to"`
					Why         string
					Close       bool
				}
			}
			require.NoError(t, decodeExactProtocolAnswer(answer, &a))
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
							var es []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithCANISOTransport(0, a.Options), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) {
									w, err := hex.DecodeString(a.Events[seen].Wire)
									require.NoError(t, err)
									require.Equal(t, w, p.Data())
									seen++
								}))
							}
							input := bytes.Clone(capture)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							require.Len(t, es, len(a.Events))
							require.Zero(t, stats.BufferedBytes)
							if observe {
								require.Equal(t, c.Packets, seen)
							}
							profile := canISOProfile
							if a.Options.DecodeUDS {
								profile = "can-iso15765-2-selected-uds"
							}
							success := 0
							for i, want := range a.Events {
								e := es[i]
								require.EqualValues(t, i+1, e.ID)
								require.Equal(t, want.Direction, e.Direction)
								require.Equal(t, want.ResponseTo, e.ResponseTo)
								require.Zero(t, e.TransactionID)
								require.Zero(t, e.FlowID)
								require.Equal(t, profile, e.Profile)
								require.Equal(t, "can-isotp", e.Protocol)
								require.Equal(t, "can", e.Transport)
								require.Equal(t, "explicit-interface-id-pair", e.Admission)
								require.Equal(t, CaptureDomain{}, e.Domain)
								if want.Error != "" {
									rocTypedError(t, want.Error, e.sessionError)
									require.Contains(t, e.Error, want.Why)
									require.Nil(t, e.Session)
									require.Nil(t, e.Structured)
									if want.Close {
										require.Empty(t, e.Raw)
									} else {
										raw, err := hex.DecodeString(want.Wire)
										require.NoError(t, err)
										require.Equal(t, raw, e.Raw)
									}
									require.Len(t, e.SourceBytes.PacketRefs, len(want.Refs))
									for j, n := range want.Refs {
										require.Equal(t, n, e.SourceBytes.PacketRefs[j].Number)
									}
									e.Session = map[string]any{"forged": true}
									e.Protocol = "http"
									e.Status = "decoded"
									e.Error = ""
									for _, fn := range []func() (map[string]any, error){e.GetFields, e.Decode} {
										f, err := fn()
										require.Nil(t, f)
										rocTypedError(t, want.Error, err)
									}
									continue
								}
								success++
								require.Empty(t, e.Error)
								raw, err := hex.DecodeString(want.Wire)
								require.NoError(t, err)
								require.Equal(t, raw, e.Raw)
								require.Len(t, e.SourceBytes.PacketRefs, len(want.Refs))
								for j, n := range want.Refs {
									require.Equal(t, n, e.SourceBytes.PacketRefs[j].Number)
									require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[j].Domain)
								}
								if len(want.Refs) > 1 {
									require.Equal(t, "reassembled", e.SourceBytes.Kind)
								} else {
									require.Equal(t, "captured", e.SourceBytes.Kind)
								}
								status := "decoded"
								if deferred {
									status = "deferred"
								}
								require.Equal(t, status, e.Status)
								require.Equal(t, "message", e.Completeness)
								rocEqualFields(t, want.Fields, e.Session)
								for _, fn := range []func() (map[string]any, error){e.GetFields, e.Decode} {
									f, err := fn()
									require.NoError(t, err)
									rocEqualFields(t, want.Fields, protocolFields(f))
									clear(f)
								}
								f, err := e.GetFields()
								require.NoError(t, err)
								f["Data"].([]byte)[0] ^= 255
								clear(f["ISO-TP"].(map[string]any))
								for _, v := range f["UDS"].(map[string]any) {
									if b, ok := v.([]byte); ok && len(b) > 0 {
										b[0] ^= 255
									}
								}
								clear(f["UDS"].(map[string]any))
								rocEqualFields(t, want.Fields, e.Session)
								rocEqualFields(t, want.Fields, e.semanticSession)
								clear(e.Session["UDS"].(map[string]any))
								again, err := e.GetFields()
								require.NoError(t, err)
								rocEqualFields(t, want.Fields, protocolFields(again))
							}
							if deferred {
								require.EqualValues(t, success, stats.Deferred)
							} else {
								require.EqualValues(t, success, stats.Decoded)
							}
						})
					}
				}
			}
		})
	}
}

func TestCANUDSParameterLifecycleAndIsolation(t *testing.T) {
	o := canISOBase()
	o.UDSResponseTimeout = 30 * time.Second // same normalized policy as the public capture option
	req := []byte{5, 0x31, 1, 0x12, 0x34, 0xaa}
	ack := []byte{5, 0x71, 1, 0x12, 0x34, 0xbb}
	at := time.Unix(1700001000, 0)
	t.Run("DomainsAndOwnedOptionRecords", func(t *testing.T) {
		var es []*ProtocolEvent
		p := canISOTestParser(&es, DefaultParserBudget(), o)
		ds := []CaptureDomain{{}, {Interface: 1}, {Section: 1}, {Encapsulation: "vpn/2"}}
		for j, d := range ds {
			w := canISORecord(o.RequestID, req, false)
			droneCANStatsDeliver(p, w, uint64(j+1), d, at)
			clear(w)
		}
		for j, d := range ds {
			w := canISORecord(o.ResponseID, ack, false)
			droneCANStatsDeliver(p, w, uint64(j+5), d, at.Add(10*time.Millisecond))
			clear(w)
		}
		require.Len(t, es, 8)
		for j, d := range ds {
			e := es[4+j]
			require.Nil(t, e.sessionError)
			require.Equal(t, d, e.Domain)
			require.EqualValues(t, j+1, e.ResponseTo)
			u := e.Session["UDS"].(map[string]any)
			require.Equal(t, []byte{0xbb}, u["Routine Status Record"])
			u["Routine Status Record"].([]byte)[0] = 0
			f, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, []byte{0xbb}, f["UDS"].(map[string]any)["Routine Status Record"])
		}
		p.closeCANISO()
		p.closeCANISO()
		require.Zero(t, p.buffered.Load())
	})
	t.Run("IdentityLimitRefusesLateResponse", func(t *testing.T) {
		b := DefaultParserBudget()
		b.MaxCollectionElements = 2
		var es []*ProtocolEvent
		p := canISOTestParser(&es, b, o)
		frames := []canISOTapeFrame{{o.RequestID, req, 0}, {o.ResponseID, ack, 10}, {o.RequestID, []byte{5, 0x31, 1, 0x56, 0x78, 0xaa}, 20}, {o.ResponseID, []byte{5, 0x71, 1, 0x56, 0x78, 0xbb}, 30}, {o.RequestID, []byte{5, 0x31, 1, 0x9a, 0xbc, 0xaa}, 40}, {o.ResponseID, []byte{5, 0x71, 1, 0x9a, 0xbc, 0xbb}, 50}}
		for i, f := range frames {
			droneCANStatsDeliver(p, canISORecord(f.id, f.data, false), uint64(i+1), CaptureDomain{}, at.Add(time.Duration(f.ms)*time.Millisecond))
		}
		require.Len(t, es, 6)
		require.Nil(t, es[0].sessionError)
		require.EqualValues(t, 1, es[1].ResponseTo)
		require.Nil(t, es[2].sessionError)
		require.EqualValues(t, 3, es[3].ResponseTo)
		rocTypedError(t, "ResourceExceeded", es[4].sessionError)
		rocTypedError(t, "ResourceExceeded", es[5].sessionError)
		require.Zero(t, es[5].ResponseTo)
		require.Nil(t, es[5].Session)
		p.closeCANISO()
		p.closeCANISO()
		require.Zero(t, p.buffered.Load())
	})
	t.Run("CloseKeepsOtherConsumerReservation", func(t *testing.T) {
		var es []*ProtocolEvent
		p := canISOTestParser(&es, DefaultParserBudget(), o)
		require.True(t, p.reserveEvidence(777))
		droneCANStatsDeliver(p, canISORecord(o.RequestID, req, false), 1, CaptureDomain{}, at)
		require.Nil(t, es[0].sessionError)
		require.Greater(t, p.buffered.Load(), int64(777))
		p.closeCANISO()
		p.closeCANISO()
		require.Equal(t, int64(777), p.buffered.Load())
		p.buffered.Add(-777)
		require.Zero(t, p.buffered.Load())
	})
}
