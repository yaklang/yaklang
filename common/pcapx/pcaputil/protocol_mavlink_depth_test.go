package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// Reuse historical captures and their independent complete field goldens. All
// common messages have a root and a nested Message Fields projection; the
// original three-message profile must honor the same depth limit as GPS.
func testMAVLinkCommonDepthBudget(t *testing.T) {
	for _, id := range []string{
		"v1-heartbeat", "v2-heartbeat", "v2-signed-heartbeat",
		"v1-sys_status", "v2-sys_status", "v2-zero-tail-1",
		"v1-global_position_int", "v2-global_position_int", "v2-zero-tail-33",
		"copacked-heartbeat-position",
	} {
		t.Run(id, func(t *testing.T) {
			b, err := trafficfixture.ReadFile("incremental-mvp/mavlink/answers/" + id + ".json")
			require.NoError(t, err)
			var answer struct {
				Fields []map[string]any `json:"native_expected_messages"`
				Raw    []string         `json:"native_raw_hex"`
			}
			require.NoError(t, json.Unmarshal(b, &answer))
			require.NotEmpty(t, answer.Fields)
			require.Len(t, answer.Raw, len(answer.Fields))
			var wire []byte
			for _, h := range answer.Raw {
				w, err := hex.DecodeString(h)
				require.NoError(t, err)
				wire = append(wire, w...)
			}
			for _, depth := range []int{1, 2} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("depth%d/deferred%t", depth, deferred), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(ParserBudget{MaxRecursionDepth: depth}, WithSessionTransport("udp"), WithSessionPorts(40000, 14550))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = deferred
						var previous uint64
						for dir := 0; dir < 2; dir++ {
							input := bytes.Clone(wire)
							out := s.Feed(dir, time.Unix(int64(dir+1), 0), input)
							clear(input)
							require.Equal(t, len(wire), out.Consumed)
							if depth == 1 {
								require.NotNil(t, out.Err, "nested common fields must not bypass the configured projection depth")
								rocTypedError(t, "ResourceExceeded", out.Err)
								require.Len(t, out.Events, 1, "no decoded co-packed prefix may escape")
								e := out.Events[0]
								require.Nil(t, e.Session)
								require.Nil(t, e.Fields)
								require.Nil(t, e.Structured)
								require.Equal(t, wire, e.Raw)
								require.Zero(t, e.Offset)
								f, err := e.GetFields()
								require.Nil(t, f)
								rocTypedError(t, "ResourceExceeded", err)
								assertOwnedProtocolRefusal(t, e, "ResourceExceeded", err)
							} else {
								require.Nil(t, out.Err)
								require.Len(t, out.Events, len(answer.Fields))
								offset := uint64(0)
								for i, e := range out.Events {
									raw, err := hex.DecodeString(answer.Raw[i])
									require.NoError(t, err)
									require.Equal(t, raw, e.Raw)
									require.Equal(t, offset, e.Offset)
									require.Equal(t, len(raw), e.Length)
									testNativeObservationSessionQueries(t, e, answer.Fields[i])
									offset += uint64(len(raw))
								}
							}
							for _, e := range out.Events {
								require.Greater(t, e.ID, previous)
								previous = e.ID
								require.Equal(t, dir, e.Direction)
								require.Zero(t, e.ResponseTo)
								require.Zero(t, e.TransactionID)
								require.Zero(t, e.FlowID)
							}
							require.Zero(t, s.Stats().BufferedBytes)
						}
						// CRC validation stays authoritative before projection admission.
						// Mutate the first payload byte for either wire-header size.
						bad := bytes.Clone(wire)
						header := 6
						if bad[0] == 0xfd {
							header = 10
						}
						bad[header] ^= 1
						out := s.Feed(0, time.Unix(3, 0), bad)
						rocTypedError(t, "MalformedMessage", out.Err)
						require.Len(t, out.Events, 1)
						require.Nil(t, out.Events[0].Session)
						require.EqualValues(t, 1, s.Stats().Malformed)
						require.Zero(t, s.Stats().Incomplete)
						if depth == 1 {
							require.EqualValues(t, 2*len(wire), s.Stats().LimitedBytes)
						} else {
							require.Zero(t, s.Stats().LimitedBytes)
							// A bad CRC cannot poison subsequent complete telemetry.
							next := s.Feed(1, time.Unix(4, 0), wire)
							require.Nil(t, next.Err)
							require.Len(t, next.Events, len(answer.Fields))
							for i, e := range next.Events {
								testNativeObservationSessionQueries(t, e, answer.Fields[i])
								require.Zero(t, e.ResponseTo)
								require.Zero(t, e.TransactionID)
							}
						}
						require.Empty(t, s.Close("done"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		})
	}
}
