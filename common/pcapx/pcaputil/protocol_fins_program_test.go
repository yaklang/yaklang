package pcaputil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func finsProgramControls(t *testing.T) []finsControl {
	return finsControlBatch(t, "fins-program-area", "FINS_PROGRAM_AREA_TEST_ROOT", "owned-fins-program-area/v1", 112)
}
func TestFINSProgramAreaSealedMatrix(t *testing.T) {
	finsSealedMatrix(t, "fins-program-area", "FINS_PROGRAM_AREA_TEST_ROOT", finsProgramControls(t))
}
func TestFINSProgramAreaSealedChunks(t *testing.T) { finsSealedChunks(t, finsProgramControls(t)) }
func TestFINSProgramAreaOwnershipAndRefusal(t *testing.T) {
	all := finsProgramControls(t)
	for _, carrier := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			for _, name := range []string{"read992", "write996", "clear10"} {
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
							x, ok := v["program_area"].(map[string]any)
							if !ok {
								return
							}
							x["beginning_address"] = uint64(999999)
							x["byte_count"] = uint64(999999)
							x["data_hex"] = "forged"
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
						original := c.Events[len(c.Events)-1]
						var events []*ProtocolEvent
						for i := range c.Events {
							w := c.Events[i]
							a.budget = DefaultParserBudget()
							if i == limited {
								if name == "write996" || name == "read992" && phase == "response" {
									a.budget.MaxCollectionElements = 63
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
								c.Events[i] = finsProgramUnpaired(original)
							}
						}
						a.budget = DefaultParserBudget()
						late := c.Events[len(c.Events)-1]
						late.Error = "ContextRequired"
						late.Fields = nil
						late.Reply = nil
						late = finsProgramUnpaired(original)
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

// Derive an unpaired view from the independent full wire answer, never IUT output.
func finsProgramUnpaired(e finsAnswer) finsAnswer {
	e.Reply = nil
	f := cloneSession(e.Fields)
	if child, ok := f["fins"].(map[string]any); ok {
		f = child
	}
	area := f["program_area"].(map[string]any)
	if area["operation"] == "clear-observation" {
		e.Error = "ContextRequired"
		e.Fields = nil
		return e
	}
	e.Fields = cloneSession(e.Fields)
	f = e.Fields
	if child, ok := f["fins"].(map[string]any); ok {
		f = child
	}
	f["association_status"] = "unverified-response"
	delete(f, "observed_request_id")
	area = f["program_area"].(map[string]any)
	area["request_context_observed"] = false
	delete(area, "requested_data_hex")
	e.Error = ""
	return e
}
