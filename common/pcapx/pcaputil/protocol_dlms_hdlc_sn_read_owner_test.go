package pcaputil

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// No invoke-id in SN does not authorize preserving a live LN request owner
// across a second different request on the same observed HDLC logical link.
// The SN frame stays a successful wire observation with zero owners.
func TestDLMSHDLCSNReadPendingOwnerExistingAPI(t *testing.T) {
	c := hdlcSNReadControls(t)[2]
	require.Len(t, c.Steps, 5)
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
						feed := func(dir int, at int64, w string) (*ProtocolEvent, *ProtocolError) {
							if reverse {
								dir = 1 - dir
							}
							wire := wrapperWire(t, w)
							var es []*ProtocolEvent
							var final *ProtocolError
							for i := 0; i < len(wire); {
								end := len(wire)
								if chunk > 0 {
									end = min(end, i+chunk)
								}
								in := bytes.Clone(wire[i:end])
								r := s.Feed(dir, time.Unix(at, 0), in)
								clear(in)
								require.Equal(t, end-i, r.Consumed)
								if len(r.Events) == 0 {
									require.True(t, r.NeedMore)
									rocTypedError(t, "NeedMore", r.Err)
								} else {
									require.False(t, r.NeedMore)
									final = r.Err
									es = append(es, r.Events...)
								}
								i = end
							}
							require.Len(t, es, 1)
							return es[0], final
						}
						q, pe := feed(0, 100, c.Steps[0].Wire)
						require.Nil(t, pe)
						owner := q.ID
						flow := q.FlowID
						require.NotZero(t, owner)
						require.NotZero(t, flow)
						hdlcOpQueries(t, q, c.Steps[0].Fields, "")
						actionPoison(q)
						sn, pe := feed(0, 101, c.Steps[1].Wire)
						require.Nil(t, pe)
						require.Equal(t, flow, sn.FlowID)
						require.Zero(t, sn.TransactionID)
						require.Zero(t, sn.ResponseTo)
						require.Equal(t, "dlms-hdlc-sn-read", sn.Profile)
						hdlcSNReadQueries(t, sn, c.Steps[1])
						actionPoison(sn)
						r, pe := feed(1, 102, "7ea012210330689de6e700c401cf001107e6977e")
						require.Zero(t, r.ResponseTo, "second distinct same-link request cannot leave original owner %d usable", owner)
						require.Zero(t, r.TransactionID)
						rocTypedError(t, "ContextRequired", pe)
						require.Equal(t, "context-required", r.Status)
						require.Equal(t, "context-required", r.Completeness)
						require.Equal(t, "dlms-hdlc-get-normal", r.Profile)
						require.Nil(t, r.Session)
						require.Equal(t, flow, r.FlowID)
						require.Equal(t, wrapperWire(t, "7ea012210330689de6e700c401cf001107e6977e"), r.Raw)
						require.Equal(t, time.Unix(102, 0), r.Timestamp)
						require.Equal(t, CaptureDomain{}, r.Domain)
						require.Zero(t, r.Offset)
						require.Equal(t, 20, r.Length)
						snReadQueryError(t, r, "ContextRequired")
						actionPoison(r)
						snReadQueryError(t, r, "ContextRequired")
						require.Empty(t, s.Close("done"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						hdlcOpQueries(t, q, c.Steps[0].Fields, "")
						hdlcSNReadQueries(t, sn, c.Steps[1])
					})
				}
			}
		}
	}
}
