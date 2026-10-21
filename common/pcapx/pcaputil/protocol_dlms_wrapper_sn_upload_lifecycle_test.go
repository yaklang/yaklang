package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDLMSWrapperSNUploadIdleAndRefusal(t *testing.T) {
	raw := finsReadBatch(t, "dlms-sn-read-upload", "DLMS_SN_UPLOAD_TEST_ROOT", "lifecycle-controls.json")
	require.Equal(t, "fe9aaf83d76c045e07ea0c99da3422b35a9239d3ce2b8df10156e115e5a34428", fmt.Sprintf("%x", sha256.Sum256(raw)))
	type row struct {
		Name  string
		Steps []map[string]any `json:"metadata"`
		Frame int              `json:"frame_bytes"`
	}
	var gold struct{ Cases []row }
	require.NoError(t, json.Unmarshal(raw, &gold))
	require.Len(t, gold.Cases, 3)
	for _, original := range gold.Cases {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/d%t", original.Name, deferred), func(t *testing.T) {
				var fresh struct{ Cases []row }
				require.NoError(t, json.Unmarshal(raw, &fresh))
				var c row
				for _, v := range fresh.Cases {
					if v.Name == original.Name {
						c = v
					}
				}
				b := DefaultParserBudget()
				if c.Frame > 0 {
					b.MaxFrameBytes = c.Frame
					b.MaxMessageBytes = c.Frame
				}
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				ids := make([]uint64, len(c.Steps))
				saved := make([]*ProtocolEvent, len(c.Steps))
				var flow uint64
				for i, w := range c.Steps {
					w["direction"] = w["native_udp_direction"]
					wire := wrapperWire(t, w["input_hex"].(string))
					require.Equal(t, w["input_sha256"], fmt.Sprintf("%x", sha256.Sum256(wire)))
					out := s.Feed(int(w["direction"].(float64)), time.Unix(int64(w["timestamp_unix"].(float64)), 0), wire)
					clear(wire)
					require.Equal(t, len(wire), out.Consumed)
					require.False(t, out.NeedMore)
					if w["error"] == nil {
						require.Nil(t, out.Err)
					} else {
						rocTypedError(t, w["query_error"].(string), out.Err)
					}
					require.Len(t, out.Events, 1)
					e := out.Events[0]
					ids[i] = e.ID
					require.NotZero(t, e.ID)
					for j := 0; j < i; j++ {
						require.NotEqual(t, ids[j], e.ID)
					}
					expectedFlow := snReadFlow(t, e, w, "udp", false, &flow)
					snReadCheckEvent(t, e, w, ids, deferred, expectedFlow)
					snWriteQueries(t, e, w, ids)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, CaptureDomain{}, e.Domain)
					saved[i] = e
					wrapperClosePoison(e)
					snWriteQueries(t, e, w, ids)
				}
				require.Empty(t, s.Close("test-close"))
				require.Empty(t, s.Close("repeat"))
				require.Zero(t, s.Stats().BufferedBytes)
				require.Zero(t, s.Stats().Incomplete)
				if c.Frame > 0 {
					require.EqualValues(t, 65, s.Stats().LimitedBytes)
				} else {
					require.Zero(t, s.Stats().LimitedBytes)
				}
				require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
				for i, e := range saved {
					snWriteQueries(t, e, c.Steps[i], ids)
				}
			})
		}
	}
}
