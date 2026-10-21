package pcaputil

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDLMSHDLCGetAssembledRequestClose(t *testing.T) {
	var original dlmsListControl
	for _, v := range dlmsRequestSegmentControls(t) {
		if v.ID == "normal-two-udp" {
			original = v
		}
	}
	require.NotEmpty(t, original.ID)
	require.GreaterOrEqual(t, len(original.Events), 3)
	for _, tr := range []string{"tcp", "udp"} {
		for _, d := range []bool{false, true} {
			for _, chunk := range []int{0, 1, 7} {
				for _, reverse := range []bool{false, true} {
					if tr == "udp" && chunk != 0 {
						continue
					}
					t.Run(fmt.Sprintf("%s/d%v/chunk%d/reverse%v", tr, d, chunk, reverse), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(41000, 4059))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = d
						var es []*ProtocolEvent
						var size uint64
						dir := 0
						if reverse {
							dir = 1
						}
						for i, st := range original.Events[:2] {
							wire := wrapperWire(t, st.Raw)
							size += uint64(len(wire))
							for at := 0; at < len(wire); {
								end := len(wire)
								if chunk > 0 {
									end = min(end, at+chunk)
								}
								in := bytes.Clone(wire[at:end])
								r := s.Feed(dir, time.Unix(int64(i+1), 0), in)
								clear(in)
								require.Equal(t, end-at, r.Consumed)
								if len(r.Events) > 0 {
									require.False(t, r.NeedMore)
									require.Nil(t, r.Err)
									es = append(es, r.Events...)
								} else {
									require.True(t, r.NeedMore)
									rocTypedError(t, "NeedMore", r.Err)
								}
								at = end
							}
							require.Len(t, es, i+1)
							f, err := es[i].GetFields()
							require.NoError(t, err)
							rocEqualFields(t, st.Fields, f)
							poisonWrapperStructured(f)
						}
						require.Equal(t, "Get Request Normal", es[1].Session["Frame Kind"])
						require.Equal(t, true, es[1].Session["HDLC Segmentation"].(map[string]any)["complete"])
						flow := es[0].FlowID
						require.NotZero(t, flow)
						require.Equal(t, flow, es[1].FlowID)
						want := hdlcOpCase{}
						want.Close.Events = []hdlcOpClose{{Profile: "dlms-hdlc-get-normal", Direction: 0, TimeTCP: 2, TimeUDP: 2, OffsetTCP: size, OffsetUDP: 0, SummaryUDP: "DLMS HDLC exchange ended with an unmatched request"}}
						hdlcGetCloseCheck(t, want, s.Close("test-close"), tr, flow, reverse, "test-close")
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						for i, e := range es {
							f, err := e.GetFields()
							require.NoError(t, err)
							rocEqualFields(t, original.Events[i].Fields, f)
						}
					})
				}
			}
		}
	}
}
