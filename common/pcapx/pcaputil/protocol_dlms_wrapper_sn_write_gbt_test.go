package pcaputil

import (
	"crypto/sha256"
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
					qinput := append([]byte(nil), qwire...)
					q := s.Feed(0, time.Unix(100, 0), qinput)
					clear(qinput)
					require.Nil(t, q.Err)
					require.Equal(t, len(qwire), q.Consumed)
					require.False(t, q.NeedMore)
					require.Len(t, q.Events, 1)
					qe := q.Events[0]
					qid := qe.ID
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
					control := snGBTLegacyBoundaryCase(t, command)
					require.Equal(t, control.Steps[0]["input_sha256"], fmt.Sprintf("%x", sha256.Sum256(qwire)))
					input := append([]byte(nil), w...)
					observed := s.Feed(dir, time.Unix(101, 0), input)
					clear(input)
					require.Nil(t, observed.Err)
					require.Equal(t, len(w), observed.Consumed)
					require.False(t, observed.NeedMore)
					require.Len(t, observed.Events, 1)
					e := observed.Events[0]
					eid := e.ID
					snGBTLegacyBoundaryEvent(t, e, control.Steps[1], tr, d, flow, []uint64{qid, eid}, w)
					lateWire := wrapperWire(t, normal.Steps[1]["input_hex"].(string))
					lateInput := append([]byte(nil), lateWire...)
					late := s.Feed(1, time.Unix(102, 0), lateInput)
					clear(lateInput)
					require.Nil(t, late.Err)
					require.Equal(t, len(lateWire), late.Consumed)
					require.False(t, late.NeedMore)
					require.Len(t, late.Events, 1)
					le := late.Events[0]
					require.Equal(t, qid, le.TransactionID)
					require.Equal(t, qid, le.ResponseTo)
					snGBTLegacyBoundaryEvent(t, le, control.Steps[2], tr, d, flow, []uint64{qid, eid, le.ID}, lateWire)
					snGBTLegacyBoundaryEvent(t, qe, control.Steps[0], tr, d, flow, []uint64{qid}, qwire)
					require.Empty(t, s.Close("GBT-boundary"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
					require.Zero(t, s.Stats().Incomplete)
				})
			}
		}
	}
}
