package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type dnpEventCountAnswer = dnpDirectAnswer

func dnpEventCountRead(t *testing.T, name string) []byte {
	t.Helper()
	var b []byte
	var err error
	if strings.HasPrefix(name, "common/") {
		root, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(root, name))
	} else if root := os.Getenv("DNP3_EVENT_COUNT_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, name))
	} else {
		b, err = trafficfixture.ReadFile("dnp3-event-count-reads/" + name)
	}
	require.NoError(t, err)
	return b
}
func dnpEventCountCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var m struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(dnpEventCountRead(t, "manifest.json"), &m))
	require.Equal(t, 1, m.SchemaVersion)
	require.Len(t, m.Cases, 52)
	if os.Getenv("DNP3_EVENT_COUNT_TEST_ROOT") == "" {
		batches, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		indexed := map[string]trafficfixture.FrozenCase{}
		for _, batch := range batches {
			for _, c := range batch.Cases {
				indexed[c.ID] = c
			}
		}
		for _, c := range m.Cases {
			f, ok := indexed["dnp3-event-count-reads/"+c.ID]
			require.True(t, ok)
			require.Equal(t, c.SHA256, f.Input.SHA256)
			require.Equal(t, c.Packets, f.Facts.PacketCount)
			require.Len(t, f.Expectations, 1)
			var binding struct {
				Answer string `json:"answer_file"`
				SHA    string `json:"answer_sha256"`
			}
			require.NoError(t, json.Unmarshal(f.Expectations[0].PayloadConstraints, &binding))
			require.Equal(t, c.Answer, binding.Answer)
			require.Equal(t, c.AnswerSHA, binding.SHA)
		}
	}
	return m.Cases
}
func dnpEventCountInput(t *testing.T, c mmsISOCase) ([]byte, dnpEventCountAnswer) {
	t.Helper()
	raw, b := dnpEventCountRead(t, c.Capture), dnpEventCountRead(t, c.Answer)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
	var a dnpEventCountAnswer
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&a))
	require.Equal(t, c.Packets, a.Packets)
	require.Len(t, a.Events, len(a.Steps))
	return raw, a
}
func dnpEventCountCheck(t *testing.T, a dnpEventCountAnswer, events []*ProtocolEvent, deferred bool) {
	dnpDirectCheck(t, a, events, deferred, "dnp3-event-count-read")
	for _, e := range events {
		if e.Error != "" {
			continue
		}
		objects, _ := e.Session["Objects"].([]any)
		for _, o := range objects {
			object := o.(map[string]any)
			g, v := object["Group"].(int), object["Variation"].(int)
			if g != 20 && g != 21 && g != 22 && g != 23 {
				continue
			}
			for _, p := range object["Points"].([]any) {
				point := p.(map[string]any)
				require.IsType(t, uint32(0), point["Value"])
				require.NotContains(t, point, "Over Range")
				require.NotContains(t, point, "Reference Error")
				noFlags := (g == 20 && (v == 5 || v == 6)) || (g == 21 && (v == 9 || v == 10))
				if noFlags {
					require.Equal(t, "not-reported", point["Quality"])
					for _, k := range []string{"Flags", "Reserved Flags", "Online", "Restart", "Communication Lost", "Remote Forced", "Local Forced", "Rollover", "Discontinuity"} {
						require.NotContains(t, point, k)
					}
				}
				if (g == 21 || g == 22 || g == 23) && (v == 5 || v == 6) {
					require.IsType(t, uint64(0), point["Timestamp [ms]"])
					require.LessOrEqual(t, point["Timestamp [ms]"].(uint64), uint64(1<<48-1))
					require.Equal(t, "not-reported", point["Timestamp Quality"])
				} else {
					require.NotContains(t, point, "Timestamp [ms]")
					require.NotContains(t, point, "Timestamp Quality")
				}
			}
		}
	}
}
func TestDNP3EventCountReadsSealedSession(t *testing.T) {
	for _, c := range dnpEventCountCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := dnpEventCountInput(t, c)
			for _, chunk := range []int{1, 7, 100000} {
				t.Run(fmt.Sprint(chunk), func(t *testing.T) {
					options := []ProtocolSessionOption{WithSessionDNP3EventCountReads(true)}
					if c.ID == "transport-composition" {
						options = append(options, WithSessionDNP3TransportFragments(true))
					}
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), options...)
					require.NoError(t, err)
					var all []*ProtocolEvent
					for i, step := range a.Steps {
						raw, err := hex.DecodeString(step.Hex)
						require.NoError(t, err)
						before := s.Stats()
						s.Probe(raw)
						require.Equal(t, before, s.Stats())
						for at := 0; at < len(raw); at += chunk {
							part := bytes.Clone(raw[at:min(at+chunk, len(raw))])
							out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), part)
							if out.NeedMore {
								rocTypedError(t, "NeedMore", out.Err)
								require.Empty(t, out.Events)
							} else {
								if a.Events[i].Error == "" {
									require.Nil(t, out.Err)
								} else {
									rocTypedError(t, a.Events[i].Error, out.Err)
								}
							}
							clear(part)
							all = append(all, out.Events...)
						}
					}
					all = append(all, s.Close("end")...)
					dnpEventCountCheck(t, a, all, false)
					require.Zero(t, s.Stats().BufferedBytes)
					require.Empty(t, s.Close("again"))
					out := s.Feed(1, time.Unix(100, 0), []byte{104})
					require.Empty(t, out.Events)
					rocTypedError(t, "FatalSessionError", out.Err)
				})
			}
		})
	}
}
func TestDNP3EventCountReadsSealedReplay(t *testing.T) {
	for _, c := range dnpEventCountCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := dnpEventCountInput(t, c)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d-d%t-o%t", worker, deferred, observe), func(t *testing.T) {
							var all []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithDNP3EventCountReads(true), WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { all = append(all, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if c.ID == "transport-composition" {
								opts = append(opts, WithDNP3TransportFragments(true))
							}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							dnpEventCountCheck(t, a, all, deferred)
							if observe {
								require.Equal(t, c.Packets, seen)
							}
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
						})
					}
				}
			}
		})
	}
}
func TestDNP3EventCountReadsOwnedQueries(t *testing.T) {
	for _, c := range dnpEventCountCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := dnpEventCountInput(t, c)
			options := []ProtocolSessionOption{WithSessionDNP3EventCountReads(true)}
			if c.ID == "transport-composition" {
				options = append(options, WithSessionDNP3TransportFragments(true))
			}
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), options...)
			require.NoError(t, err)
			var events []*ProtocolEvent
			for i, step := range a.Steps {
				raw, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
				if a.Events[i].Error == "" {
					require.Nil(t, out.Err)
				} else {
					rocTypedError(t, a.Events[i].Error, out.Err)
				}
				clear(raw)
				for _, e := range out.Events {
					before, err := e.Decode()
					if a.Events[i].Error != "" {
						rocTypedError(t, a.Events[i].Error, err)
						require.Nil(t, before)
						e.Fields = map[string]any{"CALLER": "fake"}
						e.Structured = map[string]any{"fields": e.Fields}
						e.Status = "decoded"
						fields, again := e.GetFields()
						require.Nil(t, fields)
						rocTypedError(t, a.Events[i].Error, again)
						var pe *ProtocolError
						require.True(t, errors.As(again, &pe))
						pe.Kind = ErrFatalSessionError
						pe.Message = "CALLER"
						_, again = e.GetFields()
						rocTypedError(t, a.Events[i].Error, again)
						e.Status = "malformed"
						if a.Events[i].Error == "ContextRequired" || a.Events[i].Error == "UnsupportedFeature" {
							e.Status = "context-required"
						}
						events = append(events, e)
						continue
					}
					require.NoError(t, err)
					want := cloneSession(before)
					rawSaved, sessionSaved := bytes.Clone(e.Raw), cloneSession(e.Session)
					mutateRTPSQuery(protocolFields(before))
					mutateRTPSQuery(e.Fields)
					mutateRTPSQuery(e.Session)
					clear(e.Raw)
					after, err := e.Decode()
					require.NoError(t, err)
					rocEqualFields(t, want, after)
					e.Raw = rawSaved
					e.Session = sessionSaved
					events = append(events, e)
				}
			}
			events = append(events, s.Close("end")...)
			dnpEventCountCheck(t, a, events, false)
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDNP3EventCountReadsSelectorOwnership(t *testing.T) {
	var c mmsISOCase
	for _, x := range dnpEventCountCases(t) {
		if x.ID == "excess-retains-g22" {
			c = x
		}
	}
	require.NotEmpty(t, c.ID)
	_, a := dnpEventCountInput(t, c)
	session, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionDNP3EventCountReads(true))
	require.NoError(t, err)
	var all []*ProtocolEvent
	for i, step := range a.Steps {
		raw, err := hex.DecodeString(step.Hex)
		require.NoError(t, err)
		out := session.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
		clear(raw)
		require.Len(t, out.Events, 1)
		if a.Events[i].Error == "" {
			require.Nil(t, out.Err)
		} else {
			rocTypedError(t, a.Events[i].Error, out.Err)
		}
		e := out.Events[0]
		all = append(all, e)
		if i == 0 {
			e.Session["Read Selectors"].([]any)[0].(map[string]any)["Count"] = 99999
			fields, err := e.GetFields()
			require.NoError(t, err)
			fields["Read Selectors"].([]any)[0].(map[string]any)["Count"] = 99999
			decoded, err := e.Decode()
			require.NoError(t, err)
			rocEqualFields(t, a.Events[0].Fields, protocolFields(decoded))
			e.Session = cloneSession(decoded["session"].(map[string]any))
		}
	}
	all = append(all, session.Close("end")...)
	dnpEventCountCheck(t, a, all, false)
	require.Zero(t, session.Stats().BufferedBytes)
	require.Empty(t, session.Close("again"))
}
func TestDNP3EventCountReadsBudgets(t *testing.T) {
	var m struct {
		Cases []struct {
			ID            string
			Dir           int
			Request       string
			ReplyDir      int    `json:"reply_dir"`
			EmptyReply    string `json:"empty_reply"`
			Budget        ParserBudget
			PassBudget    ParserBudget `json:"pass_budget"`
			Error, Status string
		}
	}
	require.NoError(t, json.Unmarshal(dnpEventCountRead(t, "guard-vectors.json"), &m))
	require.Len(t, m.Cases, 2)
	for _, c := range m.Cases {
		for _, pass := range []bool{false, true} {
			t.Run(c.ID+fmt.Sprint(pass), func(t *testing.T) {
				b := c.Budget
				if pass {
					b = c.PassBudget
				}
				s, err := NewProtocolSessionWithOptions(b, WithSessionDNP3EventCountReads(true))
				require.NoError(t, err)
				raw, err := hex.DecodeString(c.Request)
				require.NoError(t, err)
				out := s.Feed(c.Dir, time.Unix(1, 0), raw)
				clear(raw)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				require.Equal(t, uint64(1), e.ID)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				reply, err := hex.DecodeString(c.EmptyReply)
				require.NoError(t, err)
				if !pass {
					rocTypedError(t, "ResourceExceeded", out.Err)
					require.Equal(t, c.Status, e.Status)
					require.Zero(t, s.Stats().BufferedBytes)
					e.Fields = map[string]any{"CALLER": true}
					e.Status = "decoded"
					fields, err := e.GetFields()
					require.Nil(t, fields)
					rocTypedError(t, "ResourceExceeded", err)
					late := s.Feed(c.ReplyDir, time.Unix(2, 0), reply)
					require.Empty(t, late.Events)
					rocTypedError(t, "FatalSessionError", late.Err)
				} else {
					require.Nil(t, out.Err)
					require.Equal(t, "request", e.Session["Role"])
					late := s.Feed(c.ReplyDir, time.Unix(2, 0), reply)
					require.Nil(t, late.Err)
					require.Len(t, late.Events, 1)
					require.Equal(t, "READ", late.Events[0].Session["In Reply To"])
					require.Equal(t, "no-static-data", late.Events[0].Session["Read Coverage"])
				}
				require.Empty(t, s.Close("end"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func TestDNP3EventCountReadsOptionBoundaries(t *testing.T) {
	for _, opts := range [][]ProtocolSessionOption{{WithSessionTransport("udp"), WithSessionDNP3EventCountReads(true)}, {WithSessionDNP3EventCountReads(true), WithSessionTransport("udp")}} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.ErrorContains(t, err, "require TCP")
		require.Nil(t, s)
	}
	wire := dnp3Link(0xc4, 2, 3, []byte{0xc0, 0xc8, 1, 22, 1, 7, 2})
	for _, opts := range [][]ProtocolSessionOption{{WithSessionDNP3CounterEvents(true)}, {WithSessionDNP3EventCountReads(false), WithSessionDNP3CounterEvents(true)}, {WithSessionDNP3CounterEvents(true), WithSessionDNP3EventCountReads(false)}} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.NoError(t, err)
		out := s.Feed(0, time.Unix(1, 0), wire)
		rocTypedError(t, "UnsupportedFeature", out.Err)
		require.Len(t, out.Events, 1)
		require.NotContains(t, out.Events[0].Session, "Read Selectors")
		require.Equal(t, "dnp3-counter-events", out.Events[0].Profile)
		s.Close("end")
		require.Zero(t, s.Stats().BufferedBytes)
	}
	for _, opts := range [][]ProtocolSessionOption{{WithSessionDNP3CounterEvents(false), WithSessionDNP3EventCountReads(true)}, {WithSessionDNP3EventCountReads(true), WithSessionDNP3CounterEvents(false)}} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.NoError(t, err)
		out := s.Feed(0, time.Unix(1, 0), wire)
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		require.Equal(t, "dnp3-event-count-read", out.Events[0].Profile)
		require.Equal(t, 2, out.Events[0].Session["Read Selectors"].([]any)[0].(map[string]any)["Count"])
		s.Close("end")
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func TestDNP3EventCountReadsCollectionDepthBudget(t *testing.T) {
	var c mmsISOCase
	for _, x := range dnpEventCountCases(t) {
		if x.ID == "count8-g22v1" {
			c = x
		}
	}
	_, a := dnpEventCountInput(t, c)
	for _, kind := range []string{"collection", "depth"} {
		for _, pass := range []bool{false, true} {
			t.Run(kind+fmt.Sprint(pass), func(t *testing.T) {
				budget := DefaultParserBudget()
				if kind == "collection" {
					budget.MaxCollectionElements = 2
					if pass {
						budget.MaxCollectionElements = 3
					}
				} else {
					budget.MaxRecursionDepth = 2
					if pass {
						budget.MaxRecursionDepth = 3
					}
				}
				s, err := NewProtocolSessionWithOptions(budget, WithSessionDNP3EventCountReads(true))
				require.NoError(t, err)
				var all []*ProtocolEvent
				for i, step := range a.Steps {
					raw, err := hex.DecodeString(step.Hex)
					require.NoError(t, err)
					out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
					clear(raw)
					require.Len(t, out.Events, 1)
					e := out.Events[0]
					require.Equal(t, uint64(i+1), e.ID)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					if i == 1 && !pass {
						rocTypedError(t, "ResourceExceeded", out.Err)
						require.Equal(t, "limited", e.Status)
						require.Zero(t, s.Stats().BufferedBytes)
						e.Fields = map[string]any{"CALLER": true}
						fields, err := e.GetFields()
						require.Nil(t, fields)
						rocTypedError(t, "ResourceExceeded", err)
						late := s.Feed(step.Dir, time.Unix(3, 0), []byte{5, 100})
						require.Empty(t, late.Events)
						rocTypedError(t, "FatalSessionError", late.Err)
					} else {
						require.Nil(t, out.Err)
						all = append(all, e)
					}
				}
				if pass {
					dnpEventCountCheck(t, a, all, false)
				}
				require.Empty(t, s.Close("end"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
