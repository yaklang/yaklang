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

func TestTCPCapturedFINWindowCompleteness(t *testing.T) {
	var raw []byte
	var err error
	if root := os.Getenv("CAPTURE_FIN_COMPLETENESS_TEST_ROOT"); root != "" {
		raw, err = os.ReadFile(filepath.Join(root, "controls.json"))
	} else {
		raw, err = trafficfixture.ReadFile("tcp-fin-completeness/controls.json")
	}
	require.NoError(t, err)
	var d struct {
		Schema string
		Cases  []struct {
			ID            string         `json:"id"`
			Capture       string         `json:"capture"`
			SHA           string         `json:"sha256"`
			Packets       int            `json:"packets"`
			Gap           int            `json:"max_sequence_gap"`
			PendingBytes  int            `json:"max_pending_bytes"`
			Historical    bool           `json:"historical"`
			Truncated     int            `json:"expected_truncated"`
			Resources     int            `json:"expected_resource_events"`
			EventCount    int            `json:"expected_event_count"`
			CloseReason   string         `json:"expected_close_reason"`
			Stream        string         `json:"expected_stream_hex"`
			Invalid       int            `json:"expected_invalid_segments"`
			Unreassembled int            `json:"expected_unreassembled"`
			Pending       bool           `json:"expected_pending_gaps"`
			Decoded       int            `json:"decoded"`
			FrameRefs     []uint64       `json:"expected_frame_refs"`
			Fields        map[string]any `json:"expected_fields"`
			Session       map[string]any `json:"expected_session"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Equal(t, "capture-fin-completeness/v1", d.Schema)
	require.Len(t, d.Cases, 20)
	for _, c := range d.Cases {
		t.Run(c.ID, func(t *testing.T) {
			var capture []byte
			if c.Historical {
				capture = tcpCapturePrefixFile(t, c.Capture)
			} else if root := os.Getenv("CAPTURE_FIN_COMPLETENESS_TEST_ROOT"); root != "" {
				capture, err = os.ReadFile(filepath.Join(root, c.Capture))
				require.NoError(t, err)
			} else {
				capture, err = trafficfixture.ReadFile("tcp-fin-completeness/" + c.Capture)
				require.NoError(t, err)
			}
			require.Equal(t, c.SHA, fmt.Sprintf("%x", sha256.Sum256(capture)))
			want, err := hex.DecodeString(c.Stream)
			require.NoError(t, err)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/df%t/observe%t", worker, deferred, observe), func(t *testing.T) {
							var delivered []byte
							var closed []bool
							var frameRefs []uint64
							var packetRefs []uint64
							var transportRefs []uint64
							var assembly TCPReassemblyStats
							var flowID uint64
							opts := []CaptureOption{
								WithProtocolDeferred(deferred),
								WithTCPReassemblyOptions(TCPReassemblyOptions{Workers: worker, MaxSequenceGap: c.Gap, MaxPendingBytes: c.PendingBytes, AllowIncomplete: true}),
								WithOnTCPReassemblyStats(func(v TCPReassemblyStats) { assembly = v }),
								WithOnTrafficFlowClosed(func(reason TrafficFlowCloseReason, f *TrafficFlow) {
									if c.CloseReason != "" {
										require.Equal(t, c.CloseReason, string(reason))
									}
									closed = append(closed, f.HasPendingGaps())
									require.Equal(t, CaptureDomain{}, f.CaptureDomain())
								}),
								WithOnTrafficFlowPacket(func(f *TrafficFlow, _ *TrafficConnection, refs []PacketReference, _ time.Time) {
									require.NotZero(t, f.Index)
									if flowID == 0 {
										flowID = f.Index
									}
									require.Equal(t, flowID, f.Index)
									require.Equal(t, CaptureDomain{}, f.CaptureDomain())
									require.Len(t, refs, 1)
									require.Equal(t, CaptureDomain{}, refs[0].Domain)
									packetRefs = append(packetRefs, refs[0].Number)
									refs[0].Number = 999999
								}),
								WithOnTrafficFlowOnDataFrameArrived(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) {
									delivered = append(delivered, f.Payload...)
									if len(f.Payload) == 0 {
										return
									}
									refs := f.PacketReferences()
									require.NotEmpty(t, refs)
									for _, r := range refs {
										require.Equal(t, CaptureDomain{}, r.Domain)
										require.GreaterOrEqual(t, r.Number, uint64(1))
										require.LessOrEqual(t, r.Number, uint64(c.Packets))
										frameRefs = append(frameRefs, r.Number)
									}
									saved := append([]PacketReference(nil), refs...)
									refs[0].Number = 999999
									require.Equal(t, saved, f.PacketReferences())
								}),
							}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}), WithBeforeTransportPacket(func(p gopacket.Packet) {
									refs := CapturePacketReferences(p)
									require.Len(t, refs, 1)
									require.Equal(t, CaptureDomain{}, refs[0].Domain)
									transportRefs = append(transportRefs, refs[0].Number)
									saved := append([]PacketReference(nil), refs...)
									refs[0].Number = 999999
									require.Equal(t, saved, CapturePacketReferences(p))
									require.Equal(t, saved[0], CapturePacketEvidence(p))
								}))
							}
							events, stats, err := binReplay(t, capture, worker, opts...)
							require.NoError(t, err, "explicit AllowIncomplete retains diagnostics without failing replay")
							require.True(t, bytes.Equal(want, delivered), "exact captured stream byte sequence")
							require.Len(t, delivered, len(want))
							require.Equal(t, []bool{c.Pending}, closed, "invalid/missing FIN bytes must not look complete to close observers")
							require.EqualValues(t, c.Packets, assembly.CapturedPackets)
							require.EqualValues(t, c.Truncated, assembly.TruncatedCaptures)
							require.EqualValues(t, c.Resources, assembly.ResourceLimitEvents)
							require.Zero(t, assembly.DecodeErrors)
							require.EqualValues(t, c.Invalid, assembly.InvalidSegments)
							require.EqualValues(t, c.Unreassembled, assembly.UnreassembledSegments)
							require.Zero(t, stats.BufferedBytes)
							require.EqualValues(t, len(want), stats.InputBytes)
							expectedRefs := make([]uint64, c.Packets)
							for i := range expectedRefs {
								expectedRefs[i] = uint64(i + 1)
							}
							require.Equal(t, expectedRefs, packetRefs)
							if observe {
								require.Equal(t, expectedRefs, transportRefs)
							} else {
								require.Empty(t, transportRefs)
							}
							if c.EventCount == 0 {
								require.Empty(t, frameRefs)
								require.Empty(t, events)
							} else if c.Pending {
								require.Equal(t, c.FrameRefs, frameRefs)
								require.Len(t, events, 2)
								require.Equal(t, "incomplete", events[0].Status)
								require.Equal(t, want, events[0].Raw)
								require.Equal(t, len(want), events[0].Length)
								require.Equal(t, "incomplete", events[1].Status)
								require.Empty(t, events[1].Raw)
								require.Zero(t, events[1].Length)
								require.Contains(t, events[1].Summary, "unfilled sequence gap")
								require.Empty(t, events[1].Error)
								require.Nil(t, events[1].Session)
								require.NotEqual(t, events[0].ID, events[1].ID)
								require.Equal(t, events[0].FlowID, events[1].FlowID)
								require.Zero(t, events[0].TransactionID)
								require.Zero(t, events[0].ResponseTo)
								require.Zero(t, events[1].TransactionID)
								require.Zero(t, events[1].ResponseTo)
							} else {
								require.Equal(t, c.FrameRefs, frameRefs)
								require.Len(t, events, 1)
								e := events[0]
								require.Equal(t, want, e.Raw)
								require.Equal(t, "mqtt", e.Protocol)
								require.Empty(t, e.Error)
								require.Zero(t, e.TransactionID)
								require.Zero(t, e.ResponseTo)
								rocEqualFields(t, c.Session, e.Session)
								f, err := e.GetFields()
								require.NoError(t, err)
								rocEqualFields(t, c.Fields, protocolFields(f))
							}
							if deferred {
								require.EqualValues(t, c.Decoded, stats.Deferred)
								require.Zero(t, stats.Decoded)
							} else {
								require.EqualValues(t, c.Decoded, stats.Decoded)
								require.Zero(t, stats.Deferred)
							}
						})
					}
				}
			}
		})
	}
}

// Allowing incomplete input does not allow an observer/programming failure to
// become a successful replay. Drop only the last packet through this callback.
func TestTCPAllowIncompleteCallbackFailure(t *testing.T) {
	c := finsCase(t, "cpu-option0-tcp")
	raw := finsRead(t, c.Capture)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	for _, workers := range []int{1, 2, 4} {
		for _, allow := range []bool{false, true} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("w%d/allow%t/df%t", workers, allow, deferred), func(t *testing.T) {
					var seen int
					var assembly TCPReassemblyStats
					events, stats, err := binReplay(t, raw, workers, WithTCPReassemblyOptions(TCPReassemblyOptions{Workers: workers, AllowIncomplete: allow}), WithProtocolDeferred(deferred), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s }), WithBeforeTransportPacket(func(gopacket.Packet) {
						seen++
						if seen == c.Packets {
							panic("fin-operational-error")
						}
					}))
					require.ErrorContains(t, err, "callback panic")
					require.ErrorContains(t, err, "fin-operational-error")
					require.Equal(t, c.Packets, seen)
					require.EqualValues(t, c.Packets, assembly.CapturedPackets)
					require.EqualValues(t, 1, assembly.CallbackPanics)
					require.Zero(t, assembly.TruncatedCaptures)
					require.Zero(t, assembly.InvalidSegments)
					require.Zero(t, stats.BufferedBytes)
					expected := c
					expected.Events = append([]finsAnswer(nil), c.Events[:len(c.Events)-1]...)
					expected.Close = append(expected.Close, struct{ Direction, Count int }{0, 1})
					finsAssert(t, expected, events, deferred)
				})
			}
		}
	}
}
