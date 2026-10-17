package pcaputil

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// The public counter is a retained reservation high-water, not the bare
// fragment storage formula. An exact repeat may first raise that reservation
// to include the existing prefix. Further identical repeats must be bounded;
// input/query/result mutation cannot alter it and Close releases it exactly.
func TestDLMSHDLCSNGBTInformationReservationLifetime(t *testing.T) {
	cases, _ := hdlcSNGBTInformationLoad(t, "full-controls.json", "c2a45190a86c00e4a12404182e04d3efb10d77e424752be94c3f2d61415a60b9", 16)
	for _, tr := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/d%t", tr, deferred), func(t *testing.T) {
				budget := DefaultParserBudget()
				s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport(tr), WithSessionPorts(41000, 4059))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = deferred
				for _, st := range cases[0].Steps[:2] {
					wire := wrapperWire(t, st.Wire)
					r := s.Feed(st.Direction, time.Unix(st.At, 0), wire)
					clear(wire)
					require.Nil(t, r.Err)
				}
				require.Positive(t, s.Stats().BufferedBytes)
				st := cases[0].Steps[2]
				wire := wrapperWire(t, st.Wire)
				require.Len(t, wire, 24)
				var previous int64
				for i := 0; i < 3; i++ {
					input := append([]byte(nil), wire...)
					r := s.Feed(st.Direction, time.Unix(st.At+int64(i), 0), input)
					clear(input)
					require.Nil(t, r.Err)
					require.Len(t, r.Events, 1)
					reserved := s.Stats().BufferedBytes
					if i == 2 {
						require.Equal(t, previous, reserved, "repeated identical group must not grow its reservation")
					}
					previous = reserved
					fields, err := r.Events[0].GetFields()
					require.NoError(t, err)
					poisonWrapperStructured(fields)
					actionPoison(r.Events[0])
					require.Equal(t, reserved, s.Stats().BufferedBytes)
				}
				require.Len(t, s.Close("test-close"), 1)
				require.Empty(t, s.Close("repeat"))
				require.Zero(t, s.Stats().BufferedBytes)
				require.EqualValues(t, 1, s.Stats().Incomplete)
				require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(budget.MaxBufferedBytes))
			})
		}
	}
}
