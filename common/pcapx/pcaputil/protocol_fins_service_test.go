package pcaputil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func finsServiceControls(t *testing.T) []finsControl {
	return finsControlBatch(t, "fins-services", "FINS_SERVICE_TEST_ROOT", "owned-fins-services/v1", 112)
}
func TestFINSServicesSealedMatrix(t *testing.T) {
	finsSealedMatrix(t, "fins-services", "FINS_SERVICE_TEST_ROOT", finsServiceControls(t))
}
func TestFINSServicesSealedChunks(t *testing.T) { finsSealedChunks(t, finsServiceControls(t)) }
func finsServiceCase(t *testing.T, id string) finsControl {
	t.Helper()
	for _, c := range finsServiceControls(t) {
		if c.ID == id {
			return c
		}
	}
	t.Fatal("missing independent control", id)
	return finsControl{}
}
func TestFINSServicesLiveOwnershipAndRefusal(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", transport, deferred), func(t *testing.T) {
				c := finsServiceCase(t, "clock-two-digit-year-"+transport)
				s := finsSession(t, transport, DefaultParserBudget())
				s.(*captureSession).f.a.config.Deferred = deferred
				// Include the same observed node exchange before the selected TCP request.
				request := 0
				if transport == "tcp" {
					request = 2
				}
				var first uint64
				for i := 0; i <= request; i++ {
					w := c.Events[i]
					raw := wrapperWire(t, w.Raw)
					out := s.Feed(w.Direction, time.Unix(1, 0), raw)
					require.Len(t, out.Events, 1)
					require.Nil(t, out.Err)
					if i == request {
						e := out.Events[0]
						first = e.ID
						clear(e.Session)
						clear(e.Raw)
						clear(raw)
						e.ID, e.TransactionID = 999999, 999999
						e.Protocol = "caller-mutated"
					}
				}
				w := c.Events[request+1]
				out := s.Feed(w.Direction, time.Unix(2, 0), wrapperWire(t, w.Raw))
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				require.Equal(t, first, e.ResponseTo)
				require.Equal(t, first, e.TransactionID)
				f, err := e.GetFields()
				require.NoError(t, err)
				inner := f
				if transport == "tcp" {
					inner = f["fins"].(map[string]any)
				}
				clock := inner["clock"].(map[string]any)
				require.EqualValues(t, 25, clock["year_two_digits"])
				require.False(t, clock["century_observed"].(bool))
				clock["year_two_digits"] = uint64(99)
				again, err := e.GetFields()
				require.NoError(t, err)
				inner = again
				if transport == "tcp" {
					inner = again["fins"].(map[string]any)
				}
				require.EqualValues(t, 25, inner["clock"].(map[string]any)["year_two_digits"])
				require.Empty(t, s.Close("end"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
	// Resource refusal consumes the old eligibility but never invents a fresh
	// generation when the same SID is observed after a caller-budget change.
	for _, name := range []string{"status-run-udp", "cycle-read-udp", "clock-two-digit-year-udp"} {
		t.Run(name, func(t *testing.T) {
			c := finsServiceCase(t, name)
			s := finsSession(t, "udp", DefaultParserBudget())
			a := s.(*captureSession).f.a
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, c.Events[0].Raw))
			require.Len(t, q.Events, 1)
			require.Nil(t, q.Err)
			a.budget.MaxCollectionElements = 63
			r := s.Feed(1, time.Unix(2, 0), wrapperWire(t, c.Events[1].Raw))
			require.Len(t, r.Events, 1)
			_, err := r.Events[0].GetFields()
			rocTypedError(t, "ResourceExceeded", err)
			require.Zero(t, r.Events[0].ResponseTo)
			a.budget.MaxCollectionElements = 4096
			q = s.Feed(0, time.Unix(3, 0), wrapperWire(t, c.Events[0].Raw))
			require.Len(t, q.Events, 1)
			require.Zero(t, q.Events[0].TransactionID)
			r = s.Feed(1, time.Unix(4, 0), wrapperWire(t, c.Events[1].Raw))
			require.Len(t, r.Events, 1)
			require.Zero(t, r.Events[0].ResponseTo)
			require.Zero(t, r.Events[0].TransactionID)
			require.Empty(t, s.Close("end"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
