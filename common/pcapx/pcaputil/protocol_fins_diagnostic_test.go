package pcaputil

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func finsDiagnosticControls(t *testing.T) []finsControl {
	return finsControlBatch(t, "fins-diagnostic", "FINS_DIAGNOSTIC_TEST_ROOT", "owned-fins-diagnostic/v1", 198)
}
func finsDiagnosticFind(t *testing.T, all []finsControl, id string) finsControl {
	t.Helper()
	for _, v := range all {
		if v.ID == id {
			raw, err := json.Marshal(v)
			require.NoError(t, err)
			var c finsControl
			require.NoError(t, json.Unmarshal(raw, &c))
			return c
		}
	}
	t.Fatal("missing independent diagnostic case", id)
	return finsControl{}
}
func TestFINSDiagnosticSealedMatrix(t *testing.T) {
	finsSealedMatrix(t, "fins-diagnostic", "FINS_DIAGNOSTIC_TEST_ROOT", finsDiagnosticControls(t))
}
func TestFINSDiagnosticSealedChunks(t *testing.T) { finsSealedChunks(t, finsDiagnosticControls(t)) }
func TestFINSDiagnosticDepthBudget(t *testing.T) {
	all := finsDiagnosticControls(t)
	for _, carrier := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			for _, x := range []struct {
				name string
				low  int
			}{{"unit-explicit-limit", 4}, {"message-sparse", 4}, {"log-one", 5}} {
				for _, limited := range []bool{true, false} {
					t.Run(fmt.Sprintf("%s/df%t/%s/limited%t", carrier, deferred, x.name, limited), func(t *testing.T) {
						c := finsDiagnosticFind(t, all, x.name+"-"+carrier)
						s := finsSession(t, carrier, DefaultParserBudget())
						s.(*captureSession).f.a.config.Deferred = deferred
						var events []*ProtocolEvent
						for i, w := range c.Events {
							if i == len(c.Events)-1 {
								s.(*captureSession).f.a.budget.MaxRecursionDepth = 6
								if limited {
									s.(*captureSession).f.a.budget.MaxRecursionDepth = x.low
								}
							}
							out := s.Feed(w.Direction, time.Unix(1, 0), wrapperWire(t, w.Raw))
							events = append(events, out.Events...)
						}
						if limited {
							last := &c.Events[len(c.Events)-1]
							last.Error = "ResourceExceeded"
							last.Fields = nil
							last.Reply = nil
						}
						finsAssert(t, c, events, deferred)
						require.Empty(t, s.Close("end"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		}
	}
}
func TestFINSDiagnosticLiveOwnershipAndRefusal(t *testing.T) {
	all := finsDiagnosticControls(t)
	for _, carrier := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			for _, name := range []string{"unit25", "message-eight", "log20"} {
				t.Run(fmt.Sprintf("%s/df%t/%s", carrier, deferred, name), func(t *testing.T) {
					c := finsDiagnosticFind(t, all, name+"-"+carrier)
					s := finsSession(t, carrier, DefaultParserBudget())
					s.(*captureSession).f.a.config.Deferred = deferred
					request := 0
					if carrier == "tcp" {
						request = 2
					}
					var events []*ProtocolEvent
					var originalIDs []uint64
					shadow := func() []*ProtocolEvent {
						out := make([]*ProtocolEvent, len(events))
						for i, e := range events {
							v := *e
							v.ID = originalIDs[i]
							out[i] = &v
						}
						return out
					}
					mutate := func(f map[string]any) {
						if carrier == "tcp" {
							f = f["fins"].(map[string]any)
						}
						switch name {
						case "unit25":
							a := f["connection_data"].(map[string]any)["units"].([]any)
							a[0].(map[string]any)["model_hex"] = "caller"
							a[len(a)-1] = nil
						case "message-eight":
							a := f["message_data"].(map[string]any)["messages"].([]any)
							a[0].(map[string]any)["data_hex"] = "caller"
							a[len(a)-1] = nil
						case "log20":
							a := f["error_log"].(map[string]any)["records"].([]any)
							a[0].(map[string]any)["reported_time"].(map[string]any)["month"] = uint64(99)
							a[len(a)-1] = nil
						}
					}
					for i, w := range c.Events {
						raw := wrapperWire(t, w.Raw)
						out := s.Feed(w.Direction, time.Unix(1+int64(i), 0), raw)
						require.Len(t, out.Events, 1)
						require.Nil(t, out.Err)
						e := out.Events[0]
						events = append(events, e)
						originalIDs = append(originalIDs, e.ID)
						if w.Reply != nil {
							require.Equal(t, originalIDs[*w.Reply], e.ResponseTo)
							require.Equal(t, originalIDs[*w.Reply], e.TransactionID)
						}
						if i == request {
							clear(raw)
							clear(e.Raw)
							clear(e.Session)
							e.ID = 999999
							e.TransactionID = 999999
							e.Protocol = "caller-mutated"
							e.Status = "caller-mutated"
							e.Fields = map[string]any{"caller": true}
							e.Structured = map[string]any{"caller": true}
						}
						want := finsResolve(w.Fields, shadow()).(map[string]any)
						f, err := e.GetFields()
						require.NoError(t, err)
						rocEqualFields(t, want, f)
						if i == len(c.Events)-1 {
							mutate(f)
							mutate(e.Session)
							again, err := e.GetFields()
							require.NoError(t, err)
							rocEqualFields(t, want, again)
						}
						decoded, err := e.Decode()
						require.NoError(t, err)
						rocEqualFields(t, want, decoded["session"].(map[string]any))
					}
					require.Empty(t, s.Close("end"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
	for _, deferred := range []bool{false, true} {
		for _, x := range []struct {
			name, orphan string
			low          int
			contextError bool
		}{{"unit-explicit-limit", "unit-orphan", 35, false}, {"message-sparse", "", 63, true}, {"clear-mask1", "", 63, true}, {"log-one", "log-orphan", 47, false}} {
			t.Run(fmt.Sprintf("resource/df%t/%s", deferred, x.name), func(t *testing.T) {
				c := finsDiagnosticFind(t, all, x.name+"-udp")
				s := finsSession(t, "udp", DefaultParserBudget())
				a := s.(*captureSession).f.a
				a.config.Deferred = deferred
				q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, c.Events[0].Raw))
				require.Len(t, q.Events, 1)
				require.Nil(t, q.Err)
				a.budget.MaxCollectionElements = x.low
				r := s.Feed(1, time.Unix(2, 0), wrapperWire(t, c.Events[1].Raw))
				require.Len(t, r.Events, 1)
				_, err := r.Events[0].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Zero(t, r.Events[0].ResponseTo)
				require.Zero(t, r.Events[0].TransactionID)
				a.budget.MaxCollectionElements = 4096
				q = s.Feed(0, time.Unix(3, 0), wrapperWire(t, c.Events[0].Raw))
				require.Len(t, q.Events, 1)
				require.Nil(t, q.Err)
				require.Zero(t, q.Events[0].TransactionID)
				r = s.Feed(1, time.Unix(4, 0), wrapperWire(t, c.Events[1].Raw))
				require.Len(t, r.Events, 1)
				require.Zero(t, r.Events[0].ResponseTo)
				require.Zero(t, r.Events[0].TransactionID)
				if x.contextError {
					f, err := r.Events[0].GetFields()
					rocTypedError(t, "ContextRequired", err)
					require.Nil(t, f)
					require.Nil(t, r.Events[0].Session)
				} else {
					orphan := finsDiagnosticFind(t, all, x.orphan+"-udp")
					finsAssert(t, orphan, r.Events, deferred)
				}
				require.Empty(t, s.Close("end"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprintf("interleaved-depth-refusal/df%t", deferred), func(t *testing.T) {
			c := finsDiagnosticFind(t, all, "interleaved-commands-udp")
			c.Events[3].Error = "ResourceExceeded"
			c.Events[3].Fields = nil
			c.Events[3].Reply = nil
			late := finsDiagnosticFind(t, all, "log-orphan-udp").Events[0]
			late.Raw = c.Events[3].Raw
			late.Fields["sid"] = uint64(2)
			c.Events = append(c.Events, late)
			s := finsSession(t, "udp", DefaultParserBudget())
			a := s.(*captureSession).f.a
			a.config.Deferred = deferred
			var events []*ProtocolEvent
			for i, w := range c.Events {
				a.budget.MaxRecursionDepth = 6
				if i == 3 {
					a.budget.MaxRecursionDepth = 5
				}
				out := s.Feed(w.Direction, time.Unix(1+int64(i), 0), wrapperWire(t, w.Raw))
				events = append(events, out.Events...)
			}
			finsAssert(t, c, events, deferred)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}

}
