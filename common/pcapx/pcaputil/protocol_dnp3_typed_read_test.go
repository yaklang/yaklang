package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type dnpTypedAnswer = dnpDirectAnswer

func dnpTypedRead(t *testing.T, name string) []byte {
	t.Helper()
	var b []byte
	var err error
	if strings.HasPrefix(name, "common/") {
		root, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(root, name))
	} else if root := os.Getenv("DNP3_TYPED_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, name))
	} else {
		b, err = trafficfixture.ReadFile("dnp3-typed-static-read/" + name)
	}
	require.NoError(t, err)
	return b
}
func dnpTypedCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var m struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(dnpTypedRead(t, "manifest.json"), &m))
	require.Equal(t, 1, m.SchemaVersion)
	require.Len(t, m.Cases, 57)
	if os.Getenv("DNP3_TYPED_TEST_ROOT") == "" {
		batches, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		indexed := map[string]trafficfixture.FrozenCase{}
		for _, batch := range batches {
			for _, c := range batch.Cases {
				indexed[c.ID] = c
			}
		}
		for _, c := range m.Cases {
			f, ok := indexed["dnp3-typed-static-read/"+c.ID]
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
func dnpTypedInput(t *testing.T, c mmsISOCase) ([]byte, dnpTypedAnswer) {
	t.Helper()
	raw, b := dnpTypedRead(t, c.Capture), dnpTypedRead(t, c.Answer)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
	var a dnpTypedAnswer
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&a))
	require.Equal(t, c.Packets, a.Packets)
	require.Len(t, a.Events, len(a.Steps))
	return raw, a
}
func dnpTypedCheck(t *testing.T, a dnpTypedAnswer, events []*ProtocolEvent, deferred bool) {
	dnpDirectCheck(t, a, events, deferred, "dnp3-typed-static-read")
	for _, e := range events {
		if e.Error != "" {
			continue
		}
		objects, _ := e.Session["Objects"].([]any)
		for _, o := range objects {
			object := o.(map[string]any)
			g, v := object["Group"].(int), object["Variation"].(int)
			for _, p := range object["Points"].([]any) {
				point := p.(map[string]any)
				if g == 1 && v == 1 {
					require.Equal(t, "not-reported", point["Quality"])
					require.IsType(t, false, point["Value"])
					for _, key := range []string{"Flags", "Online", "Restart", "Communication Lost", "Remote Forced", "Local Forced", "Reserved Flags", "Chatter Filter"} {
						require.NotContains(t, point, key)
					}
				}
				if g == 30 && v <= 4 {
					require.IsType(t, int32(0), point["Value"])
				}
				if g == 30 && (v == 5 || v == 6) {
					bits, err := strconv.ParseUint(point["Raw Bits"].(string), 16, 64)
					require.NoError(t, err)
					if point["Value Status"] == "finite" {
						value := point["Value"].(float64)
						if v == 5 {
							require.Equal(t, uint32(bits), math.Float32bits(float32(value)))
						} else {
							require.Equal(t, bits, math.Float64bits(value))
						}
					} else {
						require.NotContains(t, point, "Value")
					}
				}
			}
		}
	}
}
func TestDNP3TypedReadsSealedSession(t *testing.T) {
	for _, c := range dnpTypedCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := dnpTypedInput(t, c)
			for _, chunk := range []int{1, 7, 100000} {
				t.Run(fmt.Sprint(chunk), func(t *testing.T) {
					options := []ProtocolSessionOption{WithSessionDNP3TypedStaticReads(true)}
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
					dnpTypedCheck(t, a, all, false)
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
func TestDNP3TypedReadsSealedReplay(t *testing.T) {
	for _, c := range dnpTypedCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := dnpTypedInput(t, c)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d-d%t-o%t", worker, deferred, observe), func(t *testing.T) {
							var all []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithDNP3TypedStaticReads(true), WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { all = append(all, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if c.ID == "transport-composition" {
								opts = append(opts, WithDNP3TransportFragments(true))
							}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							dnpTypedCheck(t, a, all, deferred)
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
func TestDNP3TypedReadsOwnedQueries(t *testing.T) {
	for _, c := range dnpTypedCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := dnpTypedInput(t, c)
			options := []ProtocolSessionOption{WithSessionDNP3TypedStaticReads(true)}
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
			dnpTypedCheck(t, a, events, false)
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestDNP3TypedReadsSelectorOwnership(t *testing.T) {
	var c mmsISOCase
	for _, x := range dnpTypedCases(t) {
		if x.ID == "range-gap-retains" {
			c = x
		}
	}
	_, a := dnpTypedInput(t, c)
	options := []ProtocolSessionOption{WithSessionDNP3TypedStaticReads(true)}
	if c.ID == "transport-composition" {
		options = append(options, WithSessionDNP3TransportFragments(true))
	}
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), options...)
	require.NoError(t, err)
	var all []*ProtocolEvent
	for i, step := range a.Steps {
		raw, err := hex.DecodeString(step.Hex)
		require.NoError(t, err)
		out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
		clear(raw)
		require.Len(t, out.Events, 1)
		e := out.Events[0]
		all = append(all, e)
		if i == 0 {
			e.Session["Read Selectors"].([]any)[0].(map[string]any)["Start"] = 0
			e.Session["Read Selectors"].([]any)[0].(map[string]any)["Stop"] = 65535
			f, err := e.GetFields()
			require.NoError(t, err)
			f["Read Selectors"].([]any)[0].(map[string]any)["Stop"] = 65535
			d, err := e.Decode()
			require.NoError(t, err)
			rocEqualFields(t, a.Events[0].Fields, protocolFields(d))
			e.Session = cloneSession(d["session"].(map[string]any))
		}
	}
	all = append(all, s.Close("end")...)
	dnpTypedCheck(t, a, all, false)
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestDNP3TypedReadsBudgets(t *testing.T) {
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
	require.NoError(t, json.Unmarshal(dnpTypedRead(t, "guard-vectors.json"), &m))
	require.Len(t, m.Cases, 2)
	for _, c := range m.Cases {
		for _, pass := range []bool{false, true} {
			t.Run(c.ID+fmt.Sprint(pass), func(t *testing.T) {
				b := c.Budget
				if pass {
					b = c.PassBudget
				}
				s, err := NewProtocolSessionWithOptions(b, WithSessionDNP3TypedStaticReads(true))
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
func TestDNP3TypedReadsOptionBoundaries(t *testing.T) {
	for _, opts := range [][]ProtocolSessionOption{{WithSessionTransport("udp"), WithSessionDNP3TypedStaticReads(true)}, {WithSessionDNP3TypedStaticReads(true), WithSessionTransport("udp")}} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.ErrorContains(t, err, "require TCP")
		require.Nil(t, s)
	}
	var c mmsISOCase
	for _, x := range dnpTypedCases(t) {
		if x.ID == "all-g30v1" {
			c = x
		}
	}
	_, a := dnpTypedInput(t, c)
	for _, opts := range [][]ProtocolSessionOption{{WithSessionDNP3ReadPoints(false), WithSessionDNP3TypedStaticReads(true)}, {WithSessionDNP3TypedStaticReads(true), WithSessionDNP3ReadPoints(false)}} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.NoError(t, err)
		var all []*ProtocolEvent
		for i, step := range a.Steps {
			raw, err := hex.DecodeString(step.Hex)
			require.NoError(t, err)
			out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
			require.Nil(t, out.Err)
			all = append(all, out.Events...)
		}
		s.Close("end")
		dnpTypedCheck(t, a, all, false)
		require.Zero(t, s.Stats().BufferedBytes)
	}
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionDNP3ReadPoints(true))
	require.NoError(t, err)
	raw, err := hex.DecodeString(a.Steps[0].Hex)
	require.NoError(t, err)
	out := s.Feed(0, time.Unix(1, 0), raw)
	rocTypedError(t, "UnsupportedFeature", out.Err)
	require.Len(t, out.Events, 1)
	require.Equal(t, "dnp3-read-points", out.Events[0].Profile)
	s.Close("end")
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestDNP3TypedReadsTransportComposition(t *testing.T) {
	var a dnpTypedAnswer
	require.NoError(t, decodeExactProtocolAnswer(dnpTypedRead(t, "transport-composition.json"), &a))
	require.Len(t, a.Steps, 4)
	require.Len(t, a.Events, 4)
	for _, chunk := range []int{1, 7, 10000} {
		t.Run(fmt.Sprint(chunk), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionDNP3TypedStaticReads(true), WithSessionDNP3TransportFragments(true))
			require.NoError(t, err)
			var all []*ProtocolEvent
			for i, step := range a.Steps {
				raw, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				for at := 0; at < len(raw); at += chunk {
					b := bytes.Clone(raw[at:min(at+chunk, len(raw))])
					out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), b)
					clear(b)
					if out.NeedMore {
						rocTypedError(t, "NeedMore", out.Err)
						require.Empty(t, out.Events)
					} else {
						require.Nil(t, out.Err)
					}
					all = append(all, out.Events...)
				}
			}
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			require.Len(t, all, 4)
			for i, e := range all {
				w := a.Events[i]
				require.Equal(t, "dnp3-typed-static-read-transport", e.Profile)
				require.Equal(t, w.ID, e.ID)
				require.Equal(t, w.Dir, e.Direction)
				require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				require.Empty(t, e.Error)
				rocEqualFields(t, w.Session, e.Session)
				fields, err := e.GetFields()
				require.NoError(t, err)
				rocEqualFields(t, w.Fields, fields)
			}
		})
	}
}

func TestDNP3TypedReadsPackedBudget(t *testing.T) {
	var c mmsISOCase
	for _, x := range dnpTypedCases(t) {
		if x.ID == "packed-cross-byte" {
			c = x
		}
	}
	_, a := dnpTypedInput(t, c)
	for _, kind := range []string{"collection", "depth"} {
		for _, pass := range []bool{false, true} {
			t.Run(kind+fmt.Sprint(pass), func(t *testing.T) {
				budget := DefaultParserBudget()
				if kind == "collection" {
					budget.MaxCollectionElements = 9
					if pass {
						budget.MaxCollectionElements = 10
					}
				} else {
					budget.MaxRecursionDepth = 2
					if pass {
						budget.MaxRecursionDepth = 3
					}
				}
				s, err := NewProtocolSessionWithOptions(budget, WithSessionDNP3TypedStaticReads(true))
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
					dnpTypedCheck(t, a, all, false)
				}
				require.Empty(t, s.Close("end"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
