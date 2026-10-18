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

func canISODynamicFrames(o CANISOTransport, body []byte) []canISOTapeFrame {
	fs := canISOTransferFrames(o, body, 8, 1)
	tail := append([]canISOTapeFrame(nil), fs[4:]...)
	fs = fs[:4]
	fs[3].data = []byte{0x30, 0, 10}
	for _, f := range tail {
		if f.data[0]>>4 != 3 {
			fs = append(fs, f)
		}
	}
	for i := range fs {
		fs[i].ms = i * 20
	}
	return fs
}
func TestCANISODynamicControlAndStaticCompatibility(t *testing.T) {
	body := bytes.Repeat([]byte{0x99}, 30)
	o := CANISOTransport{RequestID: 0x7e0, ResponseID: 0x7e8, DynamicFlowControl: true}
	fs := canISODynamicFrames(o, body)
	t.Run("ChangedGrant", func(t *testing.T) {
		es, stats := canISOReplay(t, o, fs)
		require.Len(t, es, len(fs))
		for _, e := range es {
			require.Nil(t, e.sessionError)
		}
		require.Equal(t, body, es[len(es)-1].Session["ISO-TP"].(map[string]any)["Payload"])
		f := es[3].Session["ISO-TP"].(map[string]any)
		require.Equal(t, int64(10000000), f["Effective STmin Nanoseconds"])
		require.Equal(t, 0, f["Block Size"])
		require.Equal(t, true, f["Dynamic Flow Control"])
		require.Zero(t, stats.BufferedBytes)
	})
	t.Run("StaticStillRefusesChangedGrant", func(t *testing.T) {
		o.DynamicFlowControl = false
		es, stats := canISOReplay(t, o, fs)
		rocTypedError(t, "UnsupportedFeature", es[3].sessionError)
		require.Zero(t, stats.BufferedBytes)
	})
	t.Run("ChangedSTminIsEnforced", func(t *testing.T) {
		o.DynamicFlowControl = true
		bad := append([]canISOTapeFrame(nil), fs...)
		bad[4].ms = fs[2].ms + 9
		bad[3].ms = fs[2].ms + 1
		es, stats := canISOReplay(t, o, bad)
		rocTypedError(t, "ContextRequired", es[4].sessionError)
		require.Zero(t, stats.BufferedBytes)
	})
}
func TestCANISOSharedSelectorIsolation(t *testing.T) {
	for _, fd := range []bool{false, true} {
		t.Run(map[bool]string{false: "Classic", true: "FD"}[fd], func(t *testing.T) {
			a := CANISOTransport{RequestID: 0x7e0, ResponseID: 0x7e8, CANFD: fd, ExtendedAddressing: true, RequestAddress: 0xa1, ResponseAddress: 0xb1}
			b := a
			b.RequestAddress, b.ResponseAddress = 0xa2, 0xb2
			var c CaptureConfig
			require.NoError(t, WithCANISOTransport(0, a)(&c))
			require.NoError(t, WithCANISOTransport(0, b)(&c))
			require.Len(t, c.canISOTransports[0], 2)
			var es []*ProtocolEvent
			p := canISOTestParser(&es, DefaultParserBudget(), a)
			p.canISOTransports[0] = []CANISOTransport{a, b}
			bodyA, bodyB := bytes.Repeat([]byte{0x11}, 25), bytes.Repeat([]byte{0x22}, 25)
			ll := 8
			if fd {
				ll = 12
			}
			fa, fb := canISOTransferFrames(a, bodyA, ll, 0), canISOTransferFrames(b, bodyB, ll, 0)
			require.Len(t, fa, len(fb))
			now := time.Unix(1700001000, 0)
			for i := range fa {
				for j, f := range []canISOTapeFrame{fa[i], fb[i]} {
					droneCANStatsDeliver(p, canISORecord(f.id, f.data, fd), uint64(i*2+j+1), CaptureDomain{}, now.Add(time.Duration(i)*20*time.Millisecond))
				}
			}
			require.Equal(t, bodyA, es[len(es)-2].Session["ISO-TP"].(map[string]any)["Payload"])
			require.Equal(t, bodyB, es[len(es)-1].Session["ISO-TP"].(map[string]any)["Payload"])
			for i, e := range es {
				require.Nil(t, e.sessionError)
				for _, ref := range e.SourceBytes.PacketRefs {
					require.EqualValues(t, (i+1)%2, ref.Number%2)
				}
			}
			p.closeCANISO()
			p.closeCANISO()
			require.Zero(t, p.buffered.Load())
		})
	}
	t.Run("AmbiguousSelectorsReject", func(t *testing.T) {
		var c CaptureConfig
		a := CANISOTransport{RequestID: 0x7e0, ResponseID: 0x7e8, ExtendedAddressing: true, RequestAddress: 0xa1, ResponseAddress: 0xb1}
		require.NoError(t, WithCANISOTransport(0, a)(&c))
		same := a
		same.ResponseAddress = 0xb2
		require.Error(t, WithCANISOTransport(0, same)(&c))
		normal := a
		normal.ExtendedAddressing = false
		require.Error(t, WithCANISOTransport(0, normal)(&c))
	})
}

func TestCANISOSharedUnknownContextAndCarrier(t *testing.T) {
	a := CANISOTransport{RequestID: 0x7e0, ResponseID: 0x7e8, ExtendedAddressing: true, RequestAddress: 0xa1, ResponseAddress: 0xb1}
	b := a
	b.RequestAddress, b.ResponseAddress = 0xa2, 0xb2
	body := bytes.Repeat([]byte{0x44}, 20)
	frames := canISOTransferFrames(a, body, 8, 0)
	now := time.Unix(1700001000, 0)
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "ForeignAddressKeepsEligible", true: "AbsentAddressCannotSelect"}[missing], func(t *testing.T) {
			var es []*ProtocolEvent
			p := canISOTestParser(&es, DefaultParserBudget(), a)
			p.canISOTransports[0] = []CANISOTransport{a, b}
			droneCANStatsDeliver(p, canISORecord(frames[0].id, frames[0].data, false), 1, CaptureDomain{}, now)
			data := []byte{0xc1, 0x21, 1, 2, 3, 4, 5, 6}
			if missing {
				data = nil
			}
			droneCANStatsDeliver(p, canISORecord(a.RequestID, data, false), 2, CaptureDomain{}, now.Add(time.Millisecond))
			if missing {
				rocTypedError(t, "MalformedMessage", es[1].sessionError)
			} else {
				rocTypedError(t, "ContextRequired", es[1].sessionError)
			}
			for i, f := range frames[1:] {
				droneCANStatsDeliver(p, canISORecord(f.id, f.data, false), uint64(i+3), CaptureDomain{}, now.Add(time.Duration(f.ms)*time.Millisecond))
			}
			if missing {
				for _, e := range es[2:] {
					require.Nil(t, e.Session)
					require.Zero(t, e.ResponseTo)
				}
			} else {
				require.Equal(t, body, es[len(es)-1].Session["ISO-TP"].(map[string]any)["Payload"])
			}
			p.closeCANISO()
			p.closeCANISO()
			require.Zero(t, p.buffered.Load())
		})
	}
	t.Run("ClassicalAndFDWithEqualIDs", func(t *testing.T) {
		classic := CANISOTransport{RequestID: 0x7e0, ResponseID: 0x7e8}
		fd := classic
		fd.CANFD = true
		var c CaptureConfig
		require.NoError(t, WithCANISOTransport(0, classic)(&c))
		require.NoError(t, WithCANISOTransport(0, fd)(&c))
		var es []*ProtocolEvent
		p := canISOTestParser(&es, DefaultParserBudget(), classic)
		p.canISOTransports[0] = []CANISOTransport{classic, fd}
		fc := canISOTransferFrames(classic, []byte{1, 2, 3}, 8, 0)
		ff := canISOTransferFrames(fd, []byte{4, 5, 6}, 8, 0)
		droneCANStatsDeliver(p, canISORecord(fc[0].id, fc[0].data, false), 1, CaptureDomain{}, now)
		droneCANStatsDeliver(p, canISORecord(ff[0].id, ff[0].data, true), 2, CaptureDomain{}, now)
		require.Equal(t, []byte{1, 2, 3}, es[0].Session["ISO-TP"].(map[string]any)["Payload"])
		require.Equal(t, []byte{4, 5, 6}, es[1].Session["ISO-TP"].(map[string]any)["Payload"])
		require.Equal(t, "classic", es[0].Session["ISO-TP"].(map[string]any)["Carrier"])
		require.Equal(t, "fd", es[1].Session["ISO-TP"].(map[string]any)["Carrier"])
		p.closeCANISO()
		require.Zero(t, p.buffered.Load())
	})
}
func TestCANISODynamicWaitAndTwoDirections(t *testing.T) {
	o := CANISOTransport{RequestID: 0x7e0, ResponseID: 0x7e8, DynamicFlowControl: true, MaxWaitFrames: 2}
	body := bytes.Repeat([]byte{0x55}, 20)
	frames := canISOTransferFrames(o, body, 8, 0)
	t.Run("WaitTupleAppliedButNoGrant", func(t *testing.T) {
		wait := canISOTapeFrame{o.ResponseID, []byte{0x31, 1, 10}, 5}
		fs := append([]canISOTapeFrame{frames[0], wait}, frames[1:]...)
		es, stats := canISOReplay(t, o, fs)
		require.Len(t, es, len(fs))
		for _, e := range es {
			require.Nil(t, e.sessionError)
		}
		f := es[1].Session["ISO-TP"].(map[string]any)
		require.Equal(t, true, f["Parameters Applied"])
		require.Equal(t, int64(10000000), f["Effective STmin Nanoseconds"])
		require.Equal(t, byte(1), f["Flow Status"])
		require.Equal(t, body, es[len(es)-1].Session["ISO-TP"].(map[string]any)["Payload"])
		require.Zero(t, stats.BufferedBytes)
	})
	t.Run("WaitDoesNotPermitData", func(t *testing.T) {
		es, stats := canISOReplay(t, o, []canISOTapeFrame{frames[0], {o.ResponseID, []byte{0x31, 1, 10}, 5}, frames[2]})
		rocTypedError(t, "ContextRequired", es[2].sessionError)
		require.Zero(t, stats.BufferedBytes)
	})
	t.Run("TwoIndependentTransportDirections", func(t *testing.T) {
		var es []*ProtocolEvent
		p := canISOTestParser(&es, DefaultParserBudget(), o)
		reverse := o
		reverse.RequestID, reverse.ResponseID = o.ResponseID, o.RequestID
		fa := canISOTransferFrames(o, body, 8, 0)
		bb := bytes.Repeat([]byte{0x66}, 20)
		fb := canISOTransferFrames(reverse, bb, 8, 0)
		now := time.Unix(1700001000, 0)
		for i := range fa {
			for j, f := range []canISOTapeFrame{fa[i], fb[i]} {
				droneCANStatsDeliver(p, canISORecord(f.id, f.data, false), uint64(i*2+j+1), CaptureDomain{}, now.Add(time.Duration(i)*20*time.Millisecond))
			}
		}
		for _, e := range es {
			require.Nil(t, e.sessionError)
		}
		require.Equal(t, body, es[len(es)-2].Session["ISO-TP"].(map[string]any)["Payload"])
		require.Equal(t, bb, es[len(es)-1].Session["ISO-TP"].(map[string]any)["Payload"])
		p.closeCANISO()
		require.Zero(t, p.buffered.Load())
	})
}

func canISOContextFile(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("CAN_ISOTP_CONTEXT_TEST_ROOT"); root != "" {
		b, e := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, e)
		return b
	}
	b, e := trafficfixture.ReadFile("can-isotp-context/" + name)
	require.NoError(t, e)
	return b
}
func TestCANISOContextSealedMatrix(t *testing.T) {
	var m struct {
		Cases []struct {
			ID, Capture, Answer, SHA256 string
			AnswerSHA                   string `json:"answer_sha256"`
			Packets                     int
		}
	}
	require.NoError(t, json.Unmarshal(canISOContextFile(t, "manifest.json"), &m))
	require.Len(t, m.Cases, 7)
	var frozen map[string]trafficfixture.FrozenCase
	if os.Getenv("CAN_ISOTP_CONTEXT_TEST_ROOT") == "" {
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
				v, ok := frozen["can-isotp-context/"+c.ID]
				require.True(t, ok)
				require.Equal(t, c.SHA256, v.Input.SHA256)
				require.Equal(t, "can-isotp-context/"+c.Capture, v.Input.File)
				require.Equal(t, c.Packets, v.Facts.PacketCount)
				require.Len(t, v.Expectations, 1)
				var bind struct {
					Answer string `json:"answer_file"`
					SHA    string `json:"answer_sha256"`
				}
				require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &bind))
				require.Equal(t, c.Answer, bind.Answer)
				require.Equal(t, c.AnswerSHA, bind.SHA)
			}
			capture, answer := canISOContextFile(t, c.Capture), canISOContextFile(t, c.Answer)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(capture)))
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
			var a struct {
				Options []CANISOTransport
				Events  []struct {
					Wire, Error, Why string
					Fields           map[string]any
					Refs             []uint64
					Direction        int
					ResponseTo       uint64 `json:"response_to"`
				}
			}
			require.NoError(t, decodeExactProtocolAnswer(answer, &a))
			for _, w := range []int{1, 2, 4} {
				for _, d := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", w, d, observe), func(t *testing.T) {
							var es []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(w), WithProtocolDeferred(d), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							for _, o := range a.Options {
								opts = append(opts, WithCANISOTransport(0, o))
							}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) {
									w, e := hex.DecodeString(a.Events[seen].Wire)
									require.NoError(t, e)
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
							for i, want := range a.Events {
								e := es[i]
								require.EqualValues(t, i+1, e.ID)
								require.Equal(t, want.Direction, e.Direction)
								require.Zero(t, e.FlowID)
								require.Zero(t, e.TransactionID)
								require.Equal(t, want.ResponseTo, e.ResponseTo)
								require.Equal(t, "can-isotp", e.Protocol)
								require.Equal(t, canISOProfile, e.Profile)
								require.Equal(t, "explicit-interface-id-pair", e.Admission)
								raw, err := hex.DecodeString(want.Wire)
								require.NoError(t, err)
								require.Equal(t, raw, e.Raw)
								require.Len(t, e.SourceBytes.PacketRefs, len(want.Refs))
								for j, n := range want.Refs {
									require.Equal(t, n, e.SourceBytes.PacketRefs[j].Number)
									require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[j].Domain)
								}
								if want.Error != "" {
									rocTypedError(t, want.Error, e.sessionError)
									require.Contains(t, e.Error, want.Why)
									require.Nil(t, e.Session)
									e.Protocol, e.Error, e.Status = "http", "", "decoded"
									e.Session = map[string]any{"forged": true}
									for _, fn := range []func() (map[string]any, error){e.GetFields, e.Decode} {
										f, err := fn()
										require.Nil(t, f)
										rocTypedError(t, want.Error, err)
									}
									continue
								}
								require.Empty(t, e.Error)
								require.Nil(t, e.sessionError)
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
						})
					}
				}
			}
		})
	}
}
