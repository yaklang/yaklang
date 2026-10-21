package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// The independent source note derives old completion floor1400476+c, with c
// the same toolchain's append capacity for the five literal APDU bytes. This
// does not query the parser to manufacture an expected budget or semantic gold.
func TestDLMSHDLCSNServiceGBTOrdinaryInterruptionBudget(t *testing.T) {
	cases := hdlcSNServiceGBTCases(t, "budget-order-witness.json", "3a6da8ea3d2cab9e9f3657843cb2081f5a28948f68ff936221492e6012d64e91", 1)
	literal := []byte{0x0c, 1, 0, 0x11, 7}
	floor := 1400476 + cap(append([]byte(nil), literal...))
	for _, extra := range []int{0, 1, 2} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("floorPlus%d/d%t", extra, deferred), func(t *testing.T) {
				encoded, err := json.Marshal(cases[0])
				require.NoError(t, err)
				var c hdlcOpCase
				require.NoError(t, json.Unmarshal(encoded, &c))
				b := DefaultParserBudget()
				b.MaxMessageBytes, b.MaxFrameBytes, b.MaxBufferedBytes = 128, 128, floor+extra
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				ids := make([]uint64, len(c.Steps))
				snapshots := make([]*ProtocolEvent, 0, len(c.Steps))
				saved := make([]*ProtocolEvent, 0, len(c.Steps))
				var flow uint64
				for i, st := range c.Steps {
					wire := wrapperWire(t, st.Wire)
					require.Equal(t, st.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
					r := s.Feed(st.Direction, time.Unix(st.At, 0), wire)
					clear(wire)
					require.Nil(t, r.Err, "ordinary E0 must retire old prefix before reserving its completed graph")
					require.False(t, r.NeedMore)
					require.Equal(t, len(wire), r.Consumed)
					require.Len(t, r.Events, 1)
					e := r.Events[0]
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, CaptureDomain{}, e.Domain)
					endpoints := []string{"192.0.2.1:41000", "192.0.2.2:4059"}
					require.Equal(t, endpoints[st.Direction], e.Source)
					require.Equal(t, endpoints[1-st.Direction], e.Destination)
					for _, prior := range ids[:i] {
						require.NotEqual(t, prior, e.ID)
					}
					ids[i] = e.ID
					require.NotZero(t, e.ID)
					if flow == 0 {
						flow = e.FlowID
					}
					require.NotZero(t, flow)
					require.Equal(t, flow, e.FlowID)
					c.Steps[i] = hdlcSNGBTBind(st, ids)
					hdlcOpQueries(t, e, c.Steps[i].Fields, "")
					fields, err := e.GetFields()
					require.NoError(t, err)
					hdlcSNServiceGBTTypes(t, fields)
					hdlcSNServiceGBTTypes(t, e.Session)
					decoded, err := e.Decode()
					require.NoError(t, err)
					for _, key := range []string{"fields", "session"} {
						v, ok := decoded[key].(map[string]any)
						require.True(t, ok, "Decode.%s type", key)
						hdlcSNServiceGBTTypes(t, v)
					}
					snapshots = append(snapshots, actionSnapshot(e))
					saved = append(saved, e)
					actionPoison(e)
					hdlcOpQueries(t, e, c.Steps[i].Fields, "")
				}
				hdlcOpCheck(t, c, snapshots, "udp", false, deferred)
				require.Empty(t, s.Close("test-close"))
				require.Empty(t, s.Close("repeat"))
				require.Zero(t, s.Stats().BufferedBytes)
				require.Zero(t, s.Stats().Incomplete)
				require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
				for i, e := range saved {
					hdlcOpQueries(t, e, c.Steps[i].Fields, "")
				}
			})
		}
	}
}
