package pcaputil

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// Exercise separately owned FeedResult errors and unfinished inner-group Close.
// Complete ordered fields, Session and queries reuse the independent answers.
func TestDLMSWrapperSNUploadGBTOwnedErrors(t *testing.T) {
	main := snUploadGBTControls(t, "controls.json", "26c9cc56ec540d778963aa93d79be738c6b259ea93b6aa0ea99c31f04f7b98fc", 18, false)
	for _, c := range []snWriteControl{main[11], main[12], main[14], main[16], main[17]} {
		for _, tr := range []string{"tcp", "udp"} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/d%t", c.Name, tr, deferred), func(t *testing.T) {
					b := DefaultParserBudget()
					if c.Budget.Depth > 0 {
						b.MaxRecursionDepth = c.Budget.Depth
					}
					if c.Budget.Collection > 0 {
						b.MaxCollectionElements = c.Budget.Collection
					}
					s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(tr), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = deferred
					ids := make([]uint64, len(c.Steps))
					saved := make([]*ProtocolEvent, len(c.Steps))
					var flow uint64
					for i, given := range c.Steps {
						w := snReadStep(t, given, tr)
						if tr == "udp" {
							w["direction"] = w["native_udp_direction"]
						}
						wire := wrapperWire(t, w["input_hex"].(string))
						r := s.Feed(int(w["direction"].(float64)), time.Unix(int64(w["timestamp_unix"].(float64)), 0), wire)
						clear(wire)
						require.Equal(t, int(w["Consumed"].(float64)), r.Consumed)
						require.False(t, r.NeedMore)
						fatal := false
						if tr == "tcp" {
							for _, v := range c.Overrides {
								if v.Step == i+1 {
									fatal = true
									require.Equal(t, "FatalSessionError", v.Error)
									rocTypedError(t, v.Error, r.Err)
									require.Equal(t, v.Message, r.Err.Message)
								}
							}
						}
						if fatal {
							require.Empty(t, r.Events)
							continue
						}
						require.Len(t, r.Events, 1)
						if w["error"] == nil {
							require.Nil(t, r.Err)
						} else {
							rocTypedError(t, w["query_error"].(string), r.Err)
							r.Err.Kind, r.Err.Message = ErrUnsupportedFeature, "mutated caller FeedResult"
						}
						e := r.Events[0]
						ids[i] = e.ID
						require.NotZero(t, e.ID)
						require.Equal(t, tr, e.Transport)
						expectedFlow := snReadFlow(t, e, w, tr, false, &flow)
						snReadCheckEvent(t, e, w, ids, deferred, expectedFlow)
						snWriteQueries(t, e, w, ids)
						wrapperClosePoison(e)
						snWriteQueries(t, e, w, ids)
						saved[i] = e
					}
					closed := s.Close(c.Close.Reason)
					snWriteCloseIdentity(t, closed, ids)
					snReadCheckClose(t, c.snReadControl, closed, tr, flow, false)
					for _, e := range closed {
						snReadTypes(t, e.Session)
						snReadQueryError(t, e, "NeedMore")
						wrapperClosePoison(e)
						snReadQueryError(t, e, "NeedMore")
					}
					require.Empty(t, s.Close("repeat"))
					require.Zero(t, s.Stats().BufferedBytes)
					want := c.Close
					if tr == "tcp" {
						want = c.TCPClose
					}
					require.EqualValues(t, len(want.Events), s.Stats().Incomplete)
					for i, e := range saved {
						if e != nil {
							snWriteQueries(t, e, c.Steps[i], ids)
						}
					}
				})
			}
		}
	}
}
