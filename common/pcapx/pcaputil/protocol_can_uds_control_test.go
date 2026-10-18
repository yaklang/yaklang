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
)

func TestCANUDSControlSelectedAPI(t *testing.T) {
	for _, c := range []struct {
		name              string
		request, response []byte
	}{{"TesterPresent", []byte{2, 0x3e, 0}, []byte{2, 0x7e, 0}}, {"ECUReset", []byte{2, 0x11, 1}, []byte{2, 0x51, 1}}, {"RapidShutdown", []byte{2, 0x11, 4}, []byte{3, 0x51, 4, 42}}} {
		t.Run(c.name, func(t *testing.T) {
			es, stats := canISOReplay(t, canISOBase(), []canISOTapeFrame{{0x7e0, c.request, 0}, {0x7e8, c.response, 10}})
			require.Len(t, es, 2)
			require.Empty(t, es[0].Error, "valid selected control request")
			require.Empty(t, es[1].Error, "echoed selected control response")
			require.EqualValues(t, 1, es[1].ResponseTo)
			require.Zero(t, stats.BufferedBytes)
		})
	}
	t.Run("AdjacentDSC", func(t *testing.T) {
		es, stats := canISOReplay(t, canISOBase(), []canISOTapeFrame{{0x7e0, []byte{2, 0x10, 3}, 0}, {0x7e8, []byte{6, 0x50, 3, 0, 50, 1, 244}, 10}})
		require.Len(t, es, 2)
		require.Empty(t, es[1].Error)
		require.EqualValues(t, 1, es[1].ResponseTo)
		require.Zero(t, stats.BufferedBytes)
	})
}

func TestCANUDSControlBoundaries(t *testing.T) {
	request := func(sid, sub byte, ms int) canISOTapeFrame { return canISOTapeFrame{0x7e0, []byte{2, sid, sub}, ms} }
	response := func(sid, sub byte, ms int) canISOTapeFrame { return canISOTapeFrame{0x7e8, []byte{2, sid, sub}, ms} }
	t.Run("WrongEchoDoesNotConsumeRequest", func(t *testing.T) {
		es, st := canISOReplay(t, canISOBase(), []canISOTapeFrame{request(0x11, 1, 0), response(0x51, 2, 10), response(0x51, 1, 20)})
		require.Len(t, es, 3)
		rocTypedError(t, "MalformedMessage", es[1].sessionError)
		require.Zero(t, es[1].ResponseTo)
		require.Nil(t, es[1].Session)
		require.Nil(t, es[2].sessionError)
		require.EqualValues(t, 1, es[2].ResponseTo)
		require.Zero(t, st.BufferedBytes)
	})
	t.Run("RapidShutdownRequiresTime", func(t *testing.T) {
		es, st := canISOReplay(t, canISOBase(), []canISOTapeFrame{request(0x11, 4, 0), response(0x51, 4, 10), {0x7e8, []byte{3, 0x51, 4, 255}, 20}})
		require.Len(t, es, 3)
		rocTypedError(t, "MalformedMessage", es[1].sessionError)
		require.EqualValues(t, 1, es[2].ResponseTo)
		u := es[2].Session["UDS"].(map[string]any)
		require.Equal(t, byte(255), u["Power Down Time Raw"])
		require.Equal(t, false, u["Power Down Time Known"])
		require.NotContains(t, u, "Power Down Time Seconds")
		require.Zero(t, st.BufferedBytes)
	})
	t.Run("SuppressedStillAllowsNegative", func(t *testing.T) {
		es, st := canISOReplay(t, canISOBase(), []canISOTapeFrame{request(0x3e, 0x80, 0), response(0x7e, 0, 10), {0x7e8, []byte{3, 0x7f, 0x3e, 0x22}, 20}})
		require.Len(t, es, 3)
		rocTypedError(t, "MalformedMessage", es[1].sessionError)
		require.Nil(t, es[2].sessionError)
		require.EqualValues(t, 1, es[2].ResponseTo)
		require.Zero(t, st.BufferedBytes)
	})
	t.Run("SuppressedClose", func(t *testing.T) {
		es, st := canISOReplay(t, canISOBase(), []canISOTapeFrame{request(0x3e, 0x80, 0)})
		require.Len(t, es, 1)
		require.Nil(t, es[0].sessionError)
		require.Zero(t, st.Incomplete)
		require.Zero(t, st.BufferedBytes)
	})
	t.Run("ReusedIdentityCannotMatchLateResponse", func(t *testing.T) {
		es, st := canISOReplay(t, canISOBase(), []canISOTapeFrame{request(0x3e, 0, 0), response(0x7e, 0, 10), request(0x3e, 0, 20), response(0x7e, 0, 30)})
		require.Len(t, es, 4)
		rocTypedError(t, "ContextRequired", es[2].sessionError)
		rocTypedError(t, "ContextRequired", es[3].sessionError)
		require.Zero(t, es[3].ResponseTo)
		require.Zero(t, st.BufferedBytes)
	})
	t.Run("UnsupportedRequestPoisonsService", func(t *testing.T) {
		es, st := canISOReplay(t, canISOBase(), []canISOTapeFrame{request(0x11, 6, 0), request(0x11, 1, 10), response(0x51, 1, 20)})
		require.Len(t, es, 3)
		rocTypedError(t, "UnsupportedFeature", es[0].sessionError)
		rocTypedError(t, "ContextRequired", es[1].sessionError)
		rocTypedError(t, "ContextRequired", es[2].sessionError)
		require.Zero(t, es[2].ResponseTo)
		require.Zero(t, st.BufferedBytes)
	})
	t.Run("ActiveRetryRetainsOriginal", func(t *testing.T) {
		es, st := canISOReplay(t, canISOBase(), []canISOTapeFrame{request(0x11, 2, 0), request(0x11, 2, 10), response(0x51, 2, 20)})
		require.Len(t, es, 3)
		for _, e := range es {
			require.Nil(t, e.sessionError)
		}
		require.EqualValues(t, 1, es[2].ResponseTo)
		require.Zero(t, st.BufferedBytes)
	})
}

func canUDSControlFile(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("CAN_UDS_CONTROL_TEST_ROOT"); root != "" {
		b, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return b
	}
	b, err := trafficfixture.ReadFile("can-uds-control/" + name)
	require.NoError(t, err)
	return b
}
func TestCANUDSControlSealedMatrix(t *testing.T) {
	var m struct {
		Cases []struct {
			ID, Capture, Answer, SHA256 string
			AnswerSHA                   string `json:"answer_sha256"`
			Packets                     int
		}
	}
	require.NoError(t, json.Unmarshal(canUDSControlFile(t, "manifest.json"), &m))
	require.Len(t, m.Cases, 23)
	var frozen map[string]trafficfixture.FrozenCase
	if os.Getenv("CAN_UDS_CONTROL_TEST_ROOT") == "" {
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
				v, ok := frozen["can-uds-control/"+c.ID]
				require.True(t, ok)
				require.Equal(t, c.SHA256, v.Input.SHA256)
				require.Equal(t, "can-uds-control/"+c.Capture, v.Input.File)
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
			capture, answer := canUDSControlFile(t, c.Capture), canUDSControlFile(t, c.Answer)
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
								rocEqualFields(t, want.Fields, e.Session)
								rocEqualFields(t, want.Fields, e.semanticSession)
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
