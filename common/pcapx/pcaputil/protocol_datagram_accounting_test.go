package pcaputil

import (
	"bytes"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// Reuse immutable independently answered captures. A truncated datagram is an
// incomplete PDU, not a malformed counter merely because no later UDP packet
// can complete it. Error kinds and successfully decoded adjacent content stay.
func TestNativeDatagramIncompleteAccounting(t *testing.T) {
	for _, tc := range []struct {
		name, status, kind             string
		decoded, incomplete, malformed uint64
	}{
		{"short-signature", "incomplete", "NeedMore", 0, 1, 0},
		{"v1-heartbeat", "decoded", "", 1, 0, 0},
		{"bad-crc", "malformed", "MalformedMessage", 0, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile("incremental-mvp/mavlink/captures/" + tc.name + ".pcap")
			require.NoError(t, err)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
							}
							require.NoError(t, ReplayPcap(bytes.NewReader(raw), opts...))
							require.Len(t, events, 1)
							e := events[0]
							require.Equal(t, "mavlink", e.Protocol)
							if tc.kind != "" {
								require.Equal(t, tc.status, e.Status)
								rocTypedError(t, tc.kind, e.sessionError)
								require.Nil(t, e.Session)
							} else {
								status := tc.status
								if deferred {
									status = "deferred"
								}
								require.Equal(t, status, e.Status)
								require.Empty(t, e.Error)
								require.Equal(t, "HEARTBEAT", e.Session["Message Name"])
								require.EqualValues(t, 42, e.Session["Message Fields"].(map[string]any)["custom_mode"])
							}
							require.Equal(t, tc.incomplete, stats.Incomplete)
							require.Equal(t, tc.malformed, stats.Malformed)
							require.Zero(t, stats.ContextRequired)
							require.Zero(t, stats.LimitedBytes)
							require.Zero(t, stats.BufferedBytes)
							require.EqualValues(t, 1, stats.Messages)
							if tc.decoded != 0 && deferred {
								require.EqualValues(t, 1, stats.Deferred)
								require.Zero(t, stats.Decoded)
							} else {
								require.Equal(t, tc.decoded, stats.Decoded)
								require.Zero(t, stats.Deferred)
							}
							if observe {
								require.EqualValues(t, 1, seen.Load())
							}
						})
					}
				}
			}
		})
	}
}
