package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func hartIPControls(t *testing.T) []finsControl {
	return finsControlBatch(t, "hart-ip-universal", "HART_IP_TEST_ROOT", "owned-hart-ip-universal/v1", 124)
}
func hartIPSession(t *testing.T, carrier string, b ParserBudget) ProtocolSession {
	t.Helper()
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(carrier), WithSessionPorts(40000, 5094))
	require.NoError(t, err)
	return s
}
func hartIPAssert(t *testing.T, c finsControl, events []*ProtocolEvent, deferred bool) {
	t.Helper()
	require.Len(t, events, len(c.Events)+len(c.Close), c.ID)
	ids := map[uint64]bool{}
	for i, w := range c.Events {
		e := events[i]
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID])
		ids[e.ID] = true
		require.Equal(t, "hart-ip", e.Protocol)
		require.Equal(t, hartIPProfile, e.Profile)
		require.Equal(t, c.Transport, e.Transport)
		require.Equal(t, w.Direction, e.Direction)
		require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
		require.Equal(t, len(w.Raw)/2, e.Length)
		reply := uint64(0)
		if w.Reply != nil {
			reply = events[*w.Reply].ID
			require.Equal(t, events[*w.Reply].FlowID, e.FlowID)
		}
		require.Equal(t, reply, e.ResponseTo)
		f, err := e.GetFields()
		rocTypedError(t, w.Error, err)
		if w.Error != "" {
			require.Nil(t, f)
			require.Nil(t, e.Session)
			require.Zero(t, e.TransactionID)
			assertOwnedProtocolRefusal(t, e, w.Error, err)
			continue
		}
		require.Empty(t, e.Error)
		want := finsResolve(w.Fields, events).(map[string]any)
		rocEqualFields(t, want, f)
		rocEqualFields(t, want, e.Session)
		txn, _ := want["observed_request_id"].(uint64)
		require.Equal(t, txn, e.TransactionID)
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		require.Equal(t, status, e.Status)
		original := cloneSession(e.Session)
		clear(f)
		clear(e.Session)
		e.Fields = map[string]any{"forged": true}
		e.Structured = map[string]any{"fields": e.Fields}
		e.Raw = bytes.Repeat([]byte{0xff}, len(e.Raw))
		e.Protocol = "forged"
		e.Profile = "forged"
		e.Status = "forged"
		f, err = e.GetFields()
		require.NoError(t, err)
		rocEqualFields(t, want, f)
		v, err := e.Decode()
		require.NoError(t, err)
		rocEqualFields(t, want, v["session"].(map[string]any))
		e.Protocol, e.Profile, e.Status = "hart-ip", hartIPProfile, status
		e.Session = original
		e.Raw = wrapperWire(t, w.Raw)
	}
	for i, w := range c.Close {
		e := events[len(c.Events)+i]
		require.Equal(t, "hart-ip", e.Protocol)
		require.Equal(t, hartIPProfile, e.Profile)
		require.Equal(t, w.Direction, e.Direction)
		require.Equal(t, "incomplete", e.Status)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		_, err := e.GetFields()
		rocTypedError(t, "NeedMore", err)
		rocEqualFields(t, map[string]any{"Outstanding": w.Count}, e.Session)
	}
}
func TestHARTIPUniversalSealedMatrix(t *testing.T) {
	for _, c := range hartIPControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw := finsReadBatch(t, "hart-ip-universal", "HART_IP_TEST_ROOT", c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := discoveryReplay(t, raw, workers, deferred, observe)
				expected := c
				if c.Transport == "udp" && c.Events[0].Direction == 1 {
					expected.Events = append([]finsAnswer(nil), c.Events...)
					for i := range expected.Events {
						expected.Events[i].Direction = 1 - expected.Events[i].Direction
					}
				}
				hartIPAssert(t, expected, events, deferred)
				require.Zero(t, stats.BufferedBytes)
				for i, w := range c.Events {
					e := events[i]
					src, dst := "192.0.2.1:40000", "192.0.2.2:5094"
					if w.Direction == 1 {
						src, dst = dst, src
					}
					require.Equal(t, src, e.Source)
					require.Equal(t, dst, e.Destination)
					require.Len(t, e.SourceBytes.PacketRefs, len(w.PacketRefs))
					for j, ref := range e.SourceBytes.PacketRefs {
						require.EqualValues(t, w.PacketRefs[j], ref.Number)
						require.Equal(t, e.Domain, ref.Domain)
					}
				}
			})
		})
	}
}
func TestHARTIPUniversalSealedChunks(t *testing.T) {
	for _, c := range hartIPControls(t) {
		for _, deferred := range []bool{false, true} {
			for _, chunk := range []int{1, 7, 64, 4096} {
				if c.Transport == "udp" && chunk != 4096 {
					continue
				}
				t.Run(fmt.Sprintf("%s/%t/%d", c.ID, deferred, chunk), func(t *testing.T) {
					s := hartIPSession(t, c.Transport, DefaultParserBudget())
					s.(*captureSession).f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					for _, w := range c.Events {
						wire := wrapperWire(t, w.Raw)
						for at := 0; at < len(wire); at += chunk {
							in := bytes.Clone(wire[at:min(len(wire), at+chunk)])
							out := s.Feed(w.Direction, time.Unix(1, 0), in)
							clear(in)
							events = append(events, out.Events...)
						}
					}
					events = append(events, s.Close("end")...)
					hartIPAssert(t, c, events, deferred)
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}
func TestHARTIPBudgetRefusalRetiresOwner(t *testing.T) {
	all := hartIPControls(t)
	for _, carrier := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			for _, phase := range []string{"request", "response"} {
				for _, limit := range []string{"depth", "collection", "memory"} {
					t.Run(fmt.Sprintf("%s/%t/%s/%s", carrier, deferred, phase, limit), func(t *testing.T) {
						c := finsDiagnosticFind(t, all, "primary-"+carrier)
						s := hartIPSession(t, carrier, DefaultParserBudget())
						a := s.(*captureSession).f.a
						a.config.Deferred = deferred
						var events []*ProtocolEvent
						stop := 2
						if phase == "response" {
							stop = 3
						}
						for i, w := range c.Events {
							if i == stop {
								switch limit {
								case "depth":
									a.budget.MaxRecursionDepth = 6
								case "collection":
									a.budget.MaxCollectionElements = 127
								case "memory":
									a.config.MaxBufferedBytes = int(a.buffered.Load())
								}
							}
							out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wrapperWire(t, w.Raw))
							events = append(events, out.Events...)
							a.budget = DefaultParserBudget()
							a.config.MaxBufferedBytes = a.budget.MaxBufferedBytes
						}
						require.Len(t, events, 4)
						_, err := events[stop].GetFields()
						rocTypedError(t, "ResourceExceeded", err)
						require.Zero(t, events[stop].TransactionID)
						require.Zero(t, events[stop].ResponseTo)
						if carrier == "udp" {
							for _, e := range events[stop+1:] {
								require.Zero(t, e.ResponseTo)
								require.Zero(t, e.TransactionID)
							}
							last := c.Events[3]
							out := s.Feed(1, time.Unix(5, 0), wrapperWire(t, last.Raw))
							require.Len(t, out.Events, 1)
							require.Zero(t, out.Events[0].ResponseTo)
							require.Zero(t, out.Events[0].TransactionID)
						}
						require.Empty(t, s.Close("end"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		}
	}
}
func TestHARTIPProjectionAndProbe(t *testing.T) {
	all := hartIPControls(t)
	for _, carrier := range []string{"tcp", "udp"} {
		t.Run(carrier, func(t *testing.T) {
			s := hartIPSession(t, carrier, DefaultParserBudget())
			c := finsDiagnosticFind(t, all, "identity7-"+carrier)
			wire := wrapperWire(t, c.Events[0].Raw)
			require.Equal(t, ProbeAccept, s.Probe(wire).Verdict)
			if carrier == "tcp" {
				require.Equal(t, ProbeNeedMore, s.Probe(wire[:5]).Verdict)
			}
			require.Empty(t, s.Close("end"))
			require.Zero(t, s.Stats().BufferedBytes)
			if carrier == "udp" {
				alt, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionHARTIP(5095), WithSessionTransport("udp"), WithSessionPorts(40000, 5095))
				require.NoError(t, err)
				var events []*ProtocolEvent
				for _, w := range c.Events {
					out := alt.Feed(w.Direction, time.Unix(1, 0), wrapperWire(t, w.Raw))
					require.Nil(t, out.Err)
					events = append(events, out.Events...)
				}
				hartIPAssert(t, c, events, false)
				require.Empty(t, alt.Close("end"))
				require.Zero(t, alt.Stats().BufferedBytes)
				bad, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionHARTIP(5095))
				require.Error(t, err)
				require.Nil(t, bad)
			}

		})
	}
}
func TestHARTIPConversationClockIsolation(t *testing.T) {
	var d struct {
		Schema string
		Cases  []struct {
			ID, Capture, SHA256 string
			Events              []struct {
				Raw                                                string
				Fields                                             map[string]any
				Reply, Transaction                                 *int
				Direction, Interface, OtherPort, FlowOwner, Packet int
				TimestampUS                                        int64
			}
			Close []struct {
				FlowOwner, Count, Interface, OtherPort int
				TimestampUS                            int64
			}
		}
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "hart-ip-universal", "HART_IP_TEST_ROOT", "clock-controls.json"), &d))
	require.Equal(t, "owned-hart-ip-clock/v1", d.Schema)
	require.Len(t, d.Cases, 9)
	for _, c := range d.Cases {
		t.Run(c.ID, func(t *testing.T) {
			raw := finsReadBatch(t, "hart-ip-universal", "HART_IP_TEST_ROOT", c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := discoveryReplay(t, raw, workers, deferred, observe)
				require.Len(t, events, len(c.Events)+len(c.Close))
				for i, w := range c.Events {
					e := events[i]
					want := finsResolve(w.Fields, events).(map[string]any)
					fields, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want, fields)
					rocEqualFields(t, want, e.Session)
					require.Equal(t, wrapperWire(t, w.Raw), e.Raw)
					require.Equal(t, "hart-ip", e.Protocol)
					require.Equal(t, hartIPProfile, e.Profile)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, w.Direction, e.Direction)
					require.EqualValues(t, w.Interface, e.Domain.Interface)
					require.True(t, time.UnixMicro(w.TimestampUS).Equal(e.Timestamp))
					require.Equal(t, events[w.FlowOwner].FlowID, e.FlowID)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.EqualValues(t, w.Packet, e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
					reply, txn := uint64(0), uint64(0)
					if w.Reply != nil {
						reply = events[*w.Reply].ID
					}
					if w.Transaction != nil {
						txn = events[*w.Transaction].ID
					}
					require.Equal(t, reply, e.ResponseTo)
					require.Equal(t, txn, e.TransactionID)
					src, dst := fmt.Sprintf("192.0.2.1:%d", 40000+w.OtherPort), "192.0.2.2:5094"
					if w.Direction == 1 {
						src, dst = dst, src
					}
					require.Equal(t, src, e.Source)
					require.Equal(t, dst, e.Destination)
				}
				closes := map[uint64]*ProtocolEvent{}
				for _, e := range events[len(c.Events):] {
					require.Nil(t, closes[e.FlowID])
					closes[e.FlowID] = e
				}
				for _, w := range c.Close {
					e := closes[events[w.FlowOwner].FlowID]
					require.NotNil(t, e)
					require.Equal(t, "incomplete", e.Status)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					require.EqualValues(t, w.Interface, e.Domain.Interface)
					require.True(t, time.UnixMicro(w.TimestampUS).Equal(e.Timestamp))
					rocEqualFields(t, map[string]any{"Outstanding": w.Count}, e.Session)
					_, err := e.GetFields()
					rocTypedError(t, "NeedMore", err)
				}
				require.Zero(t, stats.BufferedBytes)
			})
		})
	}
}
func TestHARTIPLiveOwnership(t *testing.T) {
	all := hartIPControls(t)
	for _, carrier := range []string{"tcp", "udp"} {
		t.Run(carrier, func(t *testing.T) {
			c := finsDiagnosticFind(t, all, "pipeline-reverse-"+carrier)
			s := hartIPSession(t, carrier, DefaultParserBudget())
			var shadows []*ProtocolEvent
			for i, w := range c.Events {
				raw := wrapperWire(t, w.Raw)
				out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), raw)
				require.Nil(t, out.Err)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				shadow := *e
				shadow.Raw = bytes.Clone(e.Raw)
				shadow.Session = cloneSession(e.Session)
				shadows = append(shadows, &shadow)
				want := finsResolve(w.Fields, shadows).(map[string]any)
				clear(raw)
				clear(e.Raw)
				clear(e.Session)
				e.ID = 999999
				e.ResponseTo = 999999
				e.TransactionID = 999999
				e.Protocol = "forged"
				e.Fields = map[string]any{"observed_request_id": 999999}
				e.Structured = map[string]any{"fields": e.Fields}
				f, err := e.GetFields()
				require.NoError(t, err)
				rocEqualFields(t, want, f)
			}
			hartIPAssert(t, c, shadows, false)
			require.Empty(t, s.Close("end"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestHARTIPBoundedHistory(t *testing.T) {
	for _, carrier := range []string{"tcp", "udp"} {
		t.Run(carrier, func(t *testing.T) {
			s := hartIPSession(t, carrier, DefaultParserBudget())
			a := s.(*captureSession).f.a
			a.budget.MaxCollectionElements = 128
			for i := 0; i < 17; i++ {
				q := []byte{1, 0, 2, 0, 0, byte(i), 0, 8}
				r := bytes.Clone(q)
				r[1] = 1
				out := s.Feed(0, time.Unix(1, 0), q)
				require.Len(t, out.Events, 1)
				if i == 16 {
					_, err := out.Events[0].GetFields()
					rocTypedError(t, "ResourceExceeded", err)
					require.Zero(t, out.Events[0].ResponseTo)
					require.Zero(t, out.Events[0].TransactionID)
					break
				}
				id := out.Events[0].ID
				out = s.Feed(1, time.Unix(2, 0), r)
				require.Len(t, out.Events, 1)
				require.Equal(t, id, out.Events[0].ResponseTo)
				require.Equal(t, id, out.Events[0].TransactionID)
			}
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestHARTIPObservedTCPRole(t *testing.T) {
	all := hartIPControls(t)
	for _, client := range []int{0, 1} {
		t.Run(fmt.Sprint(client), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 5094), WithSessionClientDirection(client))
			require.NoError(t, err)
			c := finsDiagnosticFind(t, all, "primary-tcp")
			var events []*ProtocolEvent
			for i := range c.Events {
				c.Events[i].Direction ^= client
				w := c.Events[i]
				out := s.Feed(w.Direction, time.Unix(1, 0), wrapperWire(t, w.Raw))
				require.Nil(t, out.Err)
				events = append(events, out.Events...)
			}
			hartIPAssert(t, c, events, false)
			require.Empty(t, s.Close("end"))
			require.Equal(t, ProbeReject, s.Probe(wrapperWire(t, c.Events[0].Raw)).Verdict)
			require.Zero(t, s.Stats().BufferedBytes)
			s, err = NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 5094), WithSessionClientDirection(client))
			require.NoError(t, err)
			out := s.Feed(1-client, time.Unix(1, 0), wrapperWire(t, c.Events[0].Raw))
			require.Len(t, out.Events, 1)
			_, err = out.Events[0].GetFields()
			rocTypedError(t, "ContextRequired", err)
			require.Zero(t, out.Events[0].ResponseTo)
			require.Zero(t, out.Events[0].TransactionID)
			require.Empty(t, s.Close("end"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestHARTIPCoalescedPipeline(t *testing.T) {
	c := finsDiagnosticFind(t, hartIPControls(t), "pipeline-reverse-tcp")
	s := hartIPSession(t, "tcp", DefaultParserBudget())
	var events []*ProtocolEvent
	for _, group := range [][2]int{{0, 1}, {1, 2}, {2, 4}, {4, 6}} {
		var wire []byte
		for _, w := range c.Events[group[0]:group[1]] {
			wire = append(wire, wrapperWire(t, w.Raw)...)
		}
		out := s.Feed(c.Events[group[0]].Direction, time.Unix(1, 0), wire)
		require.Nil(t, out.Err)
		clear(wire)
		events = append(events, out.Events...)
	}
	hartIPAssert(t, c, events, false)
	require.Empty(t, s.Close("end"))
	require.Zero(t, s.Stats().BufferedBytes)
}
