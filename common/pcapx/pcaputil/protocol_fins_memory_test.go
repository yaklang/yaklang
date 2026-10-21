package pcaputil

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func finsMemoryControls(t *testing.T) []finsControl {
	return finsControlBatch(t, "fins-memory", "FINS_MEMORY_TEST_ROOT", "owned-fins-memory/v1", 112)
}
func TestFINSMemorySealedMatrix(t *testing.T) {
	finsSealedMatrix(t, "fins-memory", "FINS_MEMORY_TEST_ROOT", finsMemoryControls(t))
}
func TestFINSMemorySealedChunks(t *testing.T) { finsSealedChunks(t, finsMemoryControls(t)) }
func finsMemoryCase(t *testing.T, id string) finsControl {
	t.Helper()
	for _, c := range finsMemoryControls(t) {
		if c.ID == id {
			return c
		}
	}
	t.Fatal("missing independent case", id)
	return finsControl{}
}
func TestFINSMemoryLiveOwnershipAndRefusal(t *testing.T) {
	for _, carrier := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", carrier, deferred), func(t *testing.T) {
				c := finsMemoryCase(t, "multiple-mixed-"+carrier)
				s := finsSession(t, carrier, DefaultParserBudget())
				s.(*captureSession).f.a.config.Deferred = deferred
				request := 0
				if carrier == "tcp" {
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
						clear(raw)
						clear(e.Raw)
						clear(e.Session)
						f, err := e.GetFields()
						require.NoError(t, err)
						clear(f)
						e.ID = 999999
						e.TransactionID = 999999
						e.Protocol = "caller-mutated"
					}
				}
				w := c.Events[request+1]
				out := s.Feed(w.Direction, time.Unix(2, 0), wrapperWire(t, w.Raw))
				require.Len(t, out.Events, 1)
				require.Nil(t, out.Err)
				e := out.Events[0]
				require.Equal(t, first, e.ResponseTo)
				require.Equal(t, first, e.TransactionID)
				get := func() map[string]any {
					f, err := e.GetFields()
					require.NoError(t, err)
					if carrier == "tcp" {
						f = f["fins"].(map[string]any)
					}
					return f
				}
				f := get()
				items := f["memory_items"].([]any)
				require.Len(t, items, 4)
				require.Equal(t, true, items[0].(map[string]any)["value"])
				require.EqualValues(t, 0x1234, items[1].(map[string]any)["value"])
				require.Equal(t, true, items[2].(map[string]any)["forced"])
				require.EqualValues(t, 0x8000, items[3].(map[string]any)["forced_mask"])
				clear(items[0].(map[string]any))
				items[1] = nil
				clear(items[3].(map[string]any))
				again := get()["memory_items"].([]any)
				require.Equal(t, true, again[0].(map[string]any)["value"])
				require.EqualValues(t, 0x1234, again[1].(map[string]any)["value"])
				require.EqualValues(t, 0x8000, again[3].(map[string]any)["forced_mask"])
				require.Empty(t, s.Close("end"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
	for _, phase := range []string{"request", "response"} {
		t.Run("budget-"+phase, func(t *testing.T) {
			c := finsMemoryCase(t, "multiple-mixed-udp")
			s := finsSession(t, "udp", DefaultParserBudget())
			a := s.(*captureSession).f.a
			if phase == "request" {
				a.budget.MaxCollectionElements = 63
			}
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, c.Events[0].Raw))
			require.Len(t, q.Events, 1)
			if phase == "request" {
				_, err := q.Events[0].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
			} else {
				require.Nil(t, q.Err)
				a.budget.MaxCollectionElements = 63
				r := s.Feed(1, time.Unix(2, 0), wrapperWire(t, c.Events[1].Raw))
				require.Len(t, r.Events, 1)
				_, err := r.Events[0].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Zero(t, r.Events[0].ResponseTo)
			}
			a.budget.MaxCollectionElements = 4096
			q = s.Feed(0, time.Unix(3, 0), wrapperWire(t, c.Events[0].Raw))
			require.Len(t, q.Events, 1)
			require.Nil(t, q.Err)
			require.Zero(t, q.Events[0].TransactionID)
			r := s.Feed(1, time.Unix(4, 0), wrapperWire(t, c.Events[1].Raw))
			require.Len(t, r.Events, 1)
			_, err := r.Events[0].GetFields()
			rocTypedError(t, "ContextRequired", err)
			require.Zero(t, r.Events[0].ResponseTo)
			require.Zero(t, r.Events[0].TransactionID)
			require.Empty(t, s.Close("end"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
