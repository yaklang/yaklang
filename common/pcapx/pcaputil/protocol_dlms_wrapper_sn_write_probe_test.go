package pcaputil

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDLMSWrapperSNWriteOffPort(t *testing.T) {
	for _, wire := range []string{"00010010000100080601021008011107", "00010001001000030d0100"} {
		for _, tr := range []string{"tcp", "udp"} {
			for _, deferred := range []bool{false, true} {
				for _, chunk := range []int{0, 1, 7} {
					if tr == "udp" && chunk != 0 {
						continue
					}
					t.Run(tr+"/"+wire+"/"+map[bool]string{true: "deferred", false: "full"}[deferred]+"/chunk"+fmt.Sprint(chunk), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(41000, 42000))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = deferred
						in := wrapperWire(t, wire)
						var events []*ProtocolEvent
						size := chunk
						if size == 0 {
							size = len(in)
						}
						for at := 0; at < len(in); {
							end := min(at+size, len(in))
							part := append([]byte(nil), in[at:end]...)
							out := s.Feed(0, time.Unix(100, 0), part)
							clear(part)
							require.Equal(t, end-at, out.Consumed)
							if out.Err != nil {
								require.True(t, out.NeedMore)
								rocTypedError(t, "NeedMore", out.Err)
								require.Empty(t, out.Events)
							}
							if len(out.Events) > 0 {
								require.False(t, out.NeedMore)
							}
							events = append(events, out.Events...)
							at = end
						}
						clear(in)
						require.Len(t, events, 1)
						e := events[0]
						require.NotZero(t, e.ID)
						require.Equal(t, "dlms-wrapper", e.Protocol)
						require.Equal(t, "dlms-wrapper-v1-sn-write", e.Profile)
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						if tr == "udp" {
							require.Zero(t, e.FlowID)
						} else {
							require.NotZero(t, e.FlowID)
						}
						f, err := e.GetFields()
						require.NoError(t, err)
						require.Equal(t, "short-name", f["reference_mode"])
						require.Equal(t, "unverified-sn-write-observation", e.Session["Association"])
						require.Equal(t, 0, e.Session["Outstanding"])
						// Use the exact independent first-trace fields for each isolated wire.
						c := snWriteControls(t)[0]
						i := 0
						if wire[16:18] == "0d" {
							i = 1
						}
						wrapperCloseEqual(t, f, c.Steps[i]["GetFields"])
						expected := cloneSession(f)
						expected["Association"], expected["Outstanding"] = "unverified-sn-write-observation", 0
						wrapperCloseEqual(t, e.Session, expected)
						snWriteTypes(t, f)
						snWriteTypes(t, e.Session)
						wrapperClosePoison(e)
						ids := make([]uint64, len(c.Steps))
						snWriteQueries(t, e, c.Steps[i], ids)
						require.Empty(t, s.Close("off-port"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		}
	}
}
