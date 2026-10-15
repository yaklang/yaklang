package pcaputil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func finsParameterControls(t *testing.T) []finsControl {
	return finsControlBatch(t, "fins-parameter-area", "FINS_PARAMETER_AREA_TEST_ROOT", "owned-fins-parameter-area/v1", 180)
}
func TestFINSParameterAreaSealedMatrix(t *testing.T) {
	finsSealedMatrix(t, "fins-parameter-area", "FINS_PARAMETER_AREA_TEST_ROOT", finsParameterControls(t))
}
func TestFINSParameterAreaSealedChunks(t *testing.T) { finsSealedChunks(t, finsParameterControls(t)) }
func TestFINSParameterAreaOwnershipAndRefusal(t *testing.T) {
	all := finsParameterControls(t)
	for _, carrier := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			for _, name := range []string{"read498", "write498", "clear65535-literal"} {
				t.Run(fmt.Sprintf("ownership/%s/df%t/%s", carrier, deferred, name), func(t *testing.T) {
					c := finsDiagnosticFind(t, all, name+"-"+carrier)
					s := finsSession(t, carrier, DefaultParserBudget())
					s.(*captureSession).f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					for i, w := range c.Events {
						raw := wrapperWire(t, w.Raw)
						out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), raw)
						require.Nil(t, out.Err)
						require.Len(t, out.Events, 1)
						e := out.Events[0]
						events = append(events, e)
						want := finsResolve(w.Fields, events).(map[string]any)
						f, err := e.GetFields()
						require.NoError(t, err)
						rocEqualFields(t, want, f)
						rocEqualFields(t, want, e.Session)
						snapshot := *e
						snapshot.Raw = wrapperWire(t, w.Raw)
						snapshot.Session = want
						events[i] = &snapshot
						mutate := func(v map[string]any) {
							if carrier == "tcp" {
								child, ok := v["fins"].(map[string]any)
								if !ok {
									return
								} // Node allocation is not a memory operation.
								v = child
							}
							x, ok := v["parameter_area"].(map[string]any)
							if !ok {
								return
							}
							x["beginning_word"] = uint64(999999)
							x["word_count"] = uint64(999999)
							if a, ok := x["values"].([]any); ok && len(a) > 0 {
								a[0] = uint64(999999)
								a[len(a)-1] = nil
							}
							x["execution_verified"] = true
						}
						mutate(f)
						mutate(e.Session)
						clear(raw)
						clear(e.Raw)
						e.ID = 999999
						e.TransactionID = 999999
						e.ResponseTo = 999999
						e.Fields = map[string]any{"caller": true}
						e.Structured = map[string]any{"caller": true}
						e.Protocol = "caller"
						e.Status = "caller"
						again, err := e.GetFields()
						require.NoError(t, err)
						rocEqualFields(t, want, again)
						decoded, err := e.Decode()
						require.NoError(t, err)
						rocEqualFields(t, want, decoded["session"].(map[string]any))
					}
					finsAssert(t, c, events, deferred)
					require.Empty(t, s.Close("end"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
				for _, phase := range []string{"request", "response"} {
					t.Run(fmt.Sprintf("budget/%s/df%t/%s/%s", carrier, deferred, name, phase), func(t *testing.T) {
						c := finsDiagnosticFind(t, all, name+"-"+carrier)
						s := finsSession(t, carrier, DefaultParserBudget())
						a := s.(*captureSession).f.a
						a.config.Deferred = deferred
						request := 0
						if carrier == "tcp" {
							request = 2
						}
						limited := request
						if phase == "response" {
							limited++
						}
						var events []*ProtocolEvent
						for i := range c.Events {
							w := c.Events[i]
							a.budget = DefaultParserBudget()
							if i == limited {
								if name == "write498" || name == "read498" && phase == "response" {
									a.budget.MaxCollectionElements = 64
								} else {
									a.budget.MaxRecursionDepth = 4
								}
							}
							out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wrapperWire(t, w.Raw))
							events = append(events, out.Events...)
							if carrier == "tcp" && i > limited {
								require.Empty(t, out.Events, "resource refusal terminates TCP parsing")
							}
							if i == limited {
								c.Events[i].Error = "ResourceExceeded"
								c.Events[i].Fields = nil
								c.Events[i].Reply = nil
							}
							if phase == "request" && i == request+1 {
								c.Events[i].Error = "ContextRequired"
								c.Events[i].Fields = nil
								c.Events[i].Reply = nil
								if name == "read498" {
									orphan := finsDiagnosticFind(t, all, "read498-orphan-"+carrier)
									c.Events[i].Error = ""
									c.Events[i].Fields = orphan.Events[len(orphan.Events)-1].Fields
								}
							}
						}
						a.budget = DefaultParserBudget()
						late := c.Events[len(c.Events)-1]
						late.Error = "ContextRequired"
						late.Fields = nil
						late.Reply = nil
						if name == "read498" {
							orphan := finsDiagnosticFind(t, all, "read498-orphan-"+carrier)
							late.Error = ""
							late.Fields = orphan.Events[len(orphan.Events)-1].Fields
						}
						out := s.Feed(late.Direction, time.Unix(10, 0), wrapperWire(t, late.Raw))
						events = append(events, out.Events...)
						if carrier == "tcp" {
							require.Empty(t, out.Events, "no late association after terminal resource refusal")
							c.Events = c.Events[:limited+1]
						} else {
							c.Events = append(c.Events, late)
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
