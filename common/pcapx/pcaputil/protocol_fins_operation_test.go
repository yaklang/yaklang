package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func finsOperationControls(t *testing.T) []finsControl {
	return finsControlBatch(t, "fins-memory-operations", "FINS_MEMORY_OPERATIONS_TEST_ROOT", "owned-fins-memory-operations/v1", 192)
}
func TestFINSMemoryOperationsSealedMatrix(t *testing.T) {
	finsSealedMatrix(t, "fins-memory-operations", "FINS_MEMORY_OPERATIONS_TEST_ROOT", finsOperationControls(t))
}
func TestFINSMemoryOperationsSealedChunks(t *testing.T) {
	finsSealedChunks(t, finsOperationControls(t))
}
func TestFINSMemoryOperationsOwnershipAndRefusal(t *testing.T) {
	all := finsOperationControls(t)
	for _, carrier := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			for _, name := range []string{"write997-words", "fill65535", "transfer65535"} {
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
							x, ok := v["memory_operation"].(map[string]any)
							if !ok {
								return
							}
							key := "target"
							if name == "transfer65535" {
								key = "source"
							}
							x[key].(map[string]any)["word_address"] = uint64(999999)
							x["item_count"] = uint64(999999)
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
								if name == "write997-words" {
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
							}
						}
						a.budget = DefaultParserBudget()
						late := c.Events[len(c.Events)-1]
						late.Error = "ContextRequired"
						late.Fields = nil
						late.Reply = nil
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

// The write request overhead is18 bytes: the existing2012-byte inner limit
// permits997 words or1994 bits. Do not relax the transport cap to fit an oracle.
func TestFINSMemoryOperationsFrameBoundary(t *testing.T) {
	all := finsOperationControls(t)
	for _, carrier := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			for _, x := range []struct {
				name  string
				width int
				count uint16
			}{{"write997-words", 2, 997}, {"write1994-bits", 1, 1994}} {
				t.Run(fmt.Sprintf("%s/df%t/%s", carrier, deferred, x.name), func(t *testing.T) {
					c := finsDiagnosticFind(t, all, x.name+"-"+carrier)
					b := DefaultParserBudget()
					b.ProbeBytes = 128
					s := finsSession(t, carrier, b)
					s.(*captureSession).f.a.config.Deferred = deferred
					request := 0
					offset := 0
					if carrier == "tcp" {
						request = 2
						offset = 16
						var handshake []*ProtocolEvent
						for i := 0; i < request; i++ {
							out := s.Feed(c.Events[i].Direction, time.Unix(int64(i+1), 0), wrapperWire(t, c.Events[i].Raw))
							handshake = append(handshake, out.Events...)
						}
						prefix := c
						prefix.Events = prefix.Events[:request]
						prefix.Close = nil
						finsAssert(t, prefix, handshake, deferred)
					}
					raw := wrapperWire(t, c.Events[request].Raw)
					require.Equal(t, 2012+offset, len(raw))
					binary.BigEndian.PutUint16(raw[offset+16:offset+18], x.count+1)
					raw = append(raw, make([]byte, x.width)...)
					if carrier == "tcp" {
						binary.BigEndian.PutUint32(raw[4:8], uint32(len(raw)-8))
					}
					out := s.Feed(0, time.Unix(3, 0), raw)
					require.Len(t, out.Events, 1)
					e := out.Events[0]
					expected := raw
					kind := "ResourceExceeded"
					if carrier == "tcp" {
						kind = "MalformedMessage"
						expected = raw[:128]
					}
					require.Equal(t, expected, e.Raw)
					require.Equal(t, len(raw), e.Length) // Diagnostic preview retains the full refused wire length.
					require.Equal(t, 0, e.Direction)
					require.Equal(t, "fins", e.Protocol)
					require.Equal(t, carrier, e.Transport)
					require.NotZero(t, e.ID)
					require.NotZero(t, e.FlowID)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					f, err := e.GetFields()
					rocTypedError(t, kind, err)
					require.Nil(t, f)
					require.Nil(t, e.Session)
					assertOwnedProtocolRefusal(t, e, kind, err)
					require.Empty(t, s.Close("end"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}

// Keep historical ZIPs/answers immutable. A newly selected0102 command with
// just one00 body byte is now malformed, rather than an unknown service.
func finsOperationLegacyControls(t *testing.T, all []finsControl) []finsControl {
	t.Helper()
	raw := finsReadBatch(t, "fins-memory-operations", "FINS_MEMORY_OPERATIONS_TEST_ROOT", "legacy-overrides.json")
	var d struct {
		Schema string
		Cases  []struct {
			ID          string `json:"case_id"`
			CaptureSHA  string `json:"capture_sha256"`
			ControlsSHA string `json:"controls_sha256"`
			Index       int    `json:"event_index"`
			Count       int    `json:"event_count"`
			Raw         string `json:"raw_hex"`
			RawSHA      string `json:"raw_sha256"`
			From        string `json:"from_error"`
			Current     string `json:"current_error"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Equal(t, "owned-fins-memory-operations/legacy-oracle-v1", d.Schema)
	require.Len(t, d.Cases, 4)
	old := finsRead(t, "controls.json")
	for _, x := range d.Cases {
		require.Equal(t, x.ControlsSHA, fmt.Sprintf("%x", sha256.Sum256(old)))
		found := 0
		for i := range all {
			c := &all[i]
			if c.ID != x.ID {
				continue
			}
			found++
			require.Equal(t, x.CaptureSHA, c.SHA256)
			require.Equal(t, x.CaptureSHA, fmt.Sprintf("%x", sha256.Sum256(finsRead(t, c.Capture))))
			require.Len(t, c.Events, x.Count)
			require.GreaterOrEqual(t, x.Index, 0)
			require.Less(t, x.Index, len(c.Events))
			e := &c.Events[x.Index]
			require.Equal(t, x.Raw, e.Raw)
			wire := wrapperWire(t, e.Raw)
			require.Equal(t, x.RawSHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
			if c.Transport == "tcp" {
				wire = wire[16:]
			}
			require.Equal(t, wrapperWire(t, "010200"), wire[10:])
			require.Equal(t, "UnsupportedFeature", x.From)
			require.Equal(t, x.From, e.Error)
			require.Nil(t, e.Fields)
			require.Nil(t, e.Reply)
			require.Equal(t, "MalformedMessage", x.Current)
			e.Error = x.Current
		}
		require.Equal(t, 1, found)
	}
	return all
}
