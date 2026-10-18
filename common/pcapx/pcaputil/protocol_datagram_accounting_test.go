package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// Reuse immutable independently answered captures. A truncated datagram is an
// incomplete PDU, not a malformed counter merely because no later UDP packet
// can complete it. Error kinds and successfully decoded adjacent content stay.
func TestNativeDatagramIncompleteAccounting(t *testing.T) {
	t.Run("oversize-query-error-and-recovery", testUDPOversizeQueryErrorAndRecovery)
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

func testUDPOversizeQueryErrorAndRecovery(t *testing.T) {
	b, err := trafficfixture.ReadFile("incremental-mvp/nmea/controls.json")
	require.NoError(t, err)
	var historical struct {
		Cases []struct {
			Name     string
			Steps    []struct{ Hex string }
			Expected []map[string]any `json:"expected_messages"`
		}
	}
	require.NoError(t, json.Unmarshal(b, &historical))
	var wire []byte
	var want map[string]any
	for _, c := range historical.Cases {
		if c.Name == "nmea-gga-positive" {
			require.Len(t, c.Steps, 1)
			wire, err = hex.DecodeString(c.Steps[0].Hex)
			require.NoError(t, err)
			require.Len(t, c.Expected, 1)
			want = c.Expected[0]
		}
	}
	require.NotEmpty(t, want)
	for _, deferred := range []bool{false, true} {
		s, err := NewProtocolSessionWithOptions(ParserBudget{MaxMessageBytes: 96}, WithSessionTransport("udp"), WithSessionPorts(38000, 10110))
		require.NoError(t, err)
		s.(*captureSession).f.a.config.Deferred = deferred
		out := s.Feed(0, time.Unix(1, 0), bytes.Repeat([]byte("x"), 97))
		require.Equal(t, 97, out.Consumed)
		rocTypedError(t, "ResourceExceeded", out.Err)
		require.Len(t, out.Events, 1)
		e := out.Events[0]
		require.Equal(t, "limited", e.Status)
		require.Empty(t, e.Protocol)
		require.Nil(t, e.Session)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		for i := 0; i < 2; i++ {
			for _, query := range []func() (map[string]any, error){e.Decode, e.GetFields} {
				f, err := query()
				require.Nil(t, f)
				rocTypedError(t, "ResourceExceeded", err)
				err.(*ProtocolError).Kind = ErrMalformedMessage // Error copies cannot rewrite the refusal.
			}
			e.Fields, e.Structured, e.Session = map[string]any{"forged": true}, map[string]any{"fields": "forged"}, map[string]any{"forged": true}
		}
		for direction := 0; direction < 2; direction++ {
			next := s.Feed(direction, time.Unix(int64(direction+2), 0), wire)
			require.Nil(t, next.Err)
			require.Len(t, next.Events, 1)
			testNativeObservationSessionQueries(t, next.Events[0], want)
			require.Zero(t, next.Events[0].ResponseTo)
			require.Zero(t, next.Events[0].TransactionID)
		}
		require.Empty(t, s.Close("done"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
		require.EqualValues(t, 97, s.Stats().LimitedBytes)
	}
}
