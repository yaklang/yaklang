package pcaputil

import (
	"encoding/binary"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDLMSWrapperSNWriteGBTBoundary(t *testing.T) {
	for _, command := range []byte{6, 13} {
		for _, tr := range []string{"tcp", "udp"} {
			for _, d := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%s/d%t", command, tr, d), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = d
					normal := snWriteControls(t)[2]
					qwire := wrapperWire(t, normal.Steps[0]["input_hex"].(string))
					q := s.Feed(0, time.Unix(100, 0), qwire)
					require.Nil(t, q.Err)
					require.Len(t, q.Events, 1)
					qe := q.Events[0]
					f, err := qe.GetFields()
					require.NoError(t, err)
					wrapperCloseEqual(t, f, normal.Steps[0]["GetFields"])
					expected := cloneSession(f)
					expected["Association"], expected["Outstanding"] = "observed-request", 1
					wrapperCloseEqual(t, qe.Session, expected)
					require.Equal(t, qe.ID, qe.TransactionID)
					require.Zero(t, qe.ResponseTo)
					flow := qe.FlowID
					require.NotZero(t, flow)
					in := wrapperWire(t, snWriteControls(t)[0].Steps[int(command/13)]["input_hex"].(string))[8:]
					p := append([]byte{0xe0, 0x81, 0, 1, 0, 0, byte(len(in))}, in...)
					w := make([]byte, 8+len(p))
					w[1] = 1
					dir := 0
					src, dst := uint16(16), uint16(1)
					if command == 13 {
						dir = 1
						src, dst = dst, src
					}
					binary.BigEndian.PutUint16(w[2:], src)
					binary.BigEndian.PutUint16(w[4:], dst)
					binary.BigEndian.PutUint16(w[6:], uint16(len(p)))
					copy(w[8:], p)
					rejected := s.Feed(dir, time.Unix(101, 0), w)
					require.Len(t, rejected.Events, 1)
					e := rejected.Events[0]
					rocTypedError(t, "UnsupportedFeature", rejected.Err)
					require.Equal(t, flow, e.FlowID)
					require.Equal(t, "dlms-wrapper-v1-gbt", e.Profile)
					require.Equal(t, w, e.Raw)
					require.Nil(t, e.Session)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
					snReadQueryError(t, e, "UnsupportedFeature")
					wrapperClosePoison(e)
					snReadQueryError(t, e, "UnsupportedFeature")
					late := s.Feed(1, time.Unix(102, 0), wrapperWire(t, normal.Steps[1]["input_hex"].(string)))
					require.Nil(t, late.Err)
					require.Len(t, late.Events, 1)
					le := late.Events[0]
					lf, err := le.GetFields()
					require.NoError(t, err)
					wrapperCloseEqual(t, lf, normal.Steps[1]["GetFields"])
					ls := cloneSession(lf)
					ls["Association"], ls["Outstanding"] = "ambiguous-conversation", 0
					wrapperCloseEqual(t, le.Session, ls)
					require.Zero(t, le.TransactionID)
					require.Zero(t, le.ResponseTo)
					require.Equal(t, flow, le.FlowID)
					require.Empty(t, s.Close("GBT-boundary"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}
