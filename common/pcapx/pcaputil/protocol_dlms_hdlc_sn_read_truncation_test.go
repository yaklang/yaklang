package pcaputil

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// The APDU command is a known SN Read service; its missing count is a
// malformed complete message, independently of the HDLC CRC/length validity.
func TestDLMSHDLCSNReadTruncatedExistingAPI(t *testing.T) {
	for _, tc := range []struct {
		name, wire string
		dir        int
	}{{"request05", "7ea00d032110326be6e600054a807e", 0}, {"response0c", "7ea00d21033030ece6e7000c57477e", 1}} {
		for _, tr := range []string{"tcp", "udp"} {
			for _, d := range []bool{false, true} {
				for _, chunk := range []int{0, 1, 7} {
					if tr == "udp" && chunk != 0 {
						continue
					}
					t.Run(fmt.Sprintf("%s/%s/d%v/chunk%d", tc.name, tr, d, chunk), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(41000, 4059))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = d
						for i, w := range []string{"7ea0190321107fdae6e600c001cf000100002a0000ff0200cfc97e", "7ea012210330689de6e700c401cf001107e6977e"} {
							r := s.Feed(i, time.Unix(int64(100+i), 0), wrapperWire(t, w))
							require.Nil(t, r.Err)
							require.Len(t, r.Events, 1)
						}
						wire := wrapperWire(t, tc.wire)
						var es []*ProtocolEvent
						var pe *ProtocolError
						for at := 0; at < len(wire); {
							end := len(wire)
							if chunk > 0 {
								end = min(end, at+chunk)
							}
							r := s.Feed(tc.dir, time.Unix(102, 0), wire[at:end])
							require.Equal(t, end-at, r.Consumed)
							if len(r.Events) == 0 {
								rocTypedError(t, "NeedMore", r.Err)
							} else {
								pe = r.Err
								es = append(es, r.Events...)
							}
							at = end
						}
						rocTypedError(t, "MalformedMessage", pe)
						require.Len(t, es, 1)
						e := es[0]
						require.Equal(t, "dlms-hdlc-sn-read", e.Profile)
						require.Equal(t, wire, e.Raw)
						require.Nil(t, e.Session)
						require.Zero(t, e.TransactionID)
						require.Zero(t, e.ResponseTo)
						snReadQueryError(t, e, "MalformedMessage")
						require.Empty(t, s.Close("done"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		}
	}
}
