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

type dnpContextAnswer struct {
	Packets int `json:"packets"`
	Steps   []struct {
		Dir int
		Hex string
	} `json:"steps"`
	Events []struct {
		ID              uint64 `json:"id"`
		Dir             int    `json:"dir"`
		Raw             string `json:"raw"`
		Fields, Session map[string]any
		ResponseTo      uint64 `json:"response_to"`
		TransactionID   uint64 `json:"transaction_id"`
		Error, Message  string
	} `json:"events"`
}

func dnpContextRead(t *testing.T, name string) []byte {
	t.Helper()
	var b []byte
	var err error
	if strings.HasPrefix(name, "common/") {
		root, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(root, name))
	} else if root := os.Getenv("DNP3_CONTEXT_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, name))
	} else {
		b, err = trafficfixture.ReadFile("dnp3-application-context/" + name)
	}
	require.NoError(t, err)
	return b
}
func dnpContextCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var m struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(dnpContextRead(t, "manifest.json"), &m))
	require.Equal(t, 1, m.SchemaVersion)
	require.Len(t, m.Cases, 28)
	if os.Getenv("DNP3_CONTEXT_TEST_ROOT") == "" {
		batches, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		indexed := map[string]trafficfixture.FrozenCase{}
		for _, batch := range batches {
			for _, c := range batch.Cases {
				indexed[c.ID] = c
			}
		}
		for _, c := range m.Cases {
			f, ok := indexed["dnp3-application-context/"+c.ID]
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
func dnpContextInput(t *testing.T, c mmsISOCase) ([]byte, dnpContextAnswer) {
	t.Helper()
	raw, b := dnpContextRead(t, c.Capture), dnpContextRead(t, c.Answer)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
	var a dnpContextAnswer
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&a))
	require.Equal(t, c.Packets, a.Packets)
	require.Len(t, a.Events, len(a.Steps))
	return raw, a
}
func dnpContextCheck(t *testing.T, a dnpContextAnswer, events []*ProtocolEvent, deferred bool) {
	t.Helper()
	require.Len(t, events, len(a.Events))
	for i, e := range events {
		w := a.Events[i]
		require.Equal(t, "dnp3", e.Protocol)
		require.Equal(t, "dnp3", e.Profile)
		require.Equal(t, "tcp", e.Transport)
		require.Equal(t, w.ID, e.ID)
		require.Equal(t, w.Dir, e.Direction)
		require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
		require.Equal(t, len(e.Raw), e.Length)
		require.Equal(t, w.ResponseTo, e.ResponseTo)
		require.Equal(t, w.TransactionID, e.TransactionID)
		if w.Error != "" {
			rocTypedError(t, w.Error, e.sessionError)
			require.Contains(t, e.Error, w.Message)
			status := "malformed"
			if w.Error == "ContextRequired" {
				status = "context-required"
			}
			require.Equal(t, status, e.Status)
			rocEqualFields(t, w.Session, e.Session)
			fields, err := e.GetFields()
			require.Nil(t, fields)
			rocTypedError(t, w.Error, err)
			continue
		}
		require.Empty(t, e.Error)
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		require.Equal(t, status, e.Status)
		rocEqualFields(t, w.Session, e.Session)
		decoded, err := e.Decode()
		require.NoError(t, err)
		rocEqualFields(t, w.Fields, protocolFields(decoded))
		rocEqualFields(t, w.Session, decoded["session"].(map[string]any))
		require.Contains(t, decoded, "metadata")
		require.Nil(t, decoded["metadata"])
	}
}
func TestDNP3ContextSealedSession(t *testing.T) {
	for _, c := range dnpContextCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := dnpContextInput(t, c)
			for _, chunk := range []int{1, 7, 100000} {
				t.Run(fmt.Sprint(chunk), func(t *testing.T) {
					s, err := NewProtocolSession(DefaultParserBudget())
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
					dnpContextCheck(t, a, all, false)
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
func TestDNP3ContextSealedReplay(t *testing.T) {
	for _, c := range dnpContextCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := dnpContextInput(t, c)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d-d%t-o%t", worker, deferred, observe), func(t *testing.T) {
							var all []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { all = append(all, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							dnpContextCheck(t, a, all, deferred)
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
func TestDNP3ContextOwnedQueries(t *testing.T) {
	for _, c := range dnpContextCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := dnpContextInput(t, c)
			s, err := NewProtocolSession(DefaultParserBudget())
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
						if a.Events[i].Error == "ContextRequired" {
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
			dnpContextCheck(t, a, events, false)
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDNP3ContextBudgetAndLateReply(t *testing.T) {
	for _, mode := range []string{"sequence", "link-address"} {
		t.Run(mode, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxCollectionElements = 2
			s, err := NewProtocolSession(b)
			require.NoError(t, err)
			for i := 0; i < 3; i++ {
				seq, source := byte(i), uint16(2)
				if mode == "link-address" {
					seq = 1
					source += uint16(i)
				}
				raw := dnp3Link(0xc4, 1, source, []byte{0xc0, 0xc0 | seq, 1})
				out := s.Feed(0, time.Unix(int64(i+1), 0), raw)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				require.Equal(t, raw, e.Raw)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				if i < 2 {
					require.Nil(t, out.Err)
					require.Equal(t, "request", e.Session["Role"])
					continue
				}
				require.NotNil(t, out.Err)
				rocTypedError(t, "ResourceExceeded", out.Err)
				require.Equal(t, "limited", e.Status)
				require.Nil(t, e.Session)
				require.Zero(t, s.Stats().BufferedBytes)
				e.Fields = map[string]any{"CALLER": "fake"}
				e.Structured = map[string]any{"fields": e.Fields}
				e.Status = "decoded"
				f, err := e.GetFields()
				require.Nil(t, f)
				rocTypedError(t, "ResourceExceeded", err)
			}
			out := s.Feed(1, time.Unix(100, 0), dnp3Link(0x44, 2, 1, []byte{0xc0, 0xc0, 129, 0, 0}))
			require.Empty(t, out.Events)
			rocTypedError(t, "FatalSessionError", out.Err)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDNP3ContextCRCAndCallbackScope(t *testing.T) {
	for _, mode := range []string{"application", "crc"} {
		t.Run(mode, func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			native := s.(*captureSession)
			callback := native.f.a.config.OnEvent
			native.f.a.config.OnEvent = func(e *ProtocolEvent) {
				// Public refusal edits cannot widen/narrow private error recovery scope.
				if e.Error != "" {
					e.Error = ""
					e.Session = map[string]any{"Error Scope": "application"}
					if mode == "application" {
						e.Session = nil
					}
				}
				callback(e)
			}
			out := s.Feed(0, time.Unix(1, 0), dnp3Link(0xc4, 1, 2, []byte{0xc0, 0xc1, 1}))
			require.Nil(t, out.Err)
			bad := dnp3Link(0x44, 2, 1, []byte{0xc0, 0xd1, 129, 0, 0})
			kind := "MalformedMessage"
			if mode == "crc" {
				bad[len(bad)-1] ^= 1
			}
			out = s.Feed(1, time.Unix(2, 0), bad)
			require.Len(t, out.Events, 1)
			rocTypedError(t, kind, out.Err)
			require.Equal(t, bad, out.Events[0].Raw)
			fields, err := out.Events[0].GetFields()
			require.Nil(t, fields)
			rocTypedError(t, kind, err)
			good := s.Feed(1, time.Unix(3, 0), dnp3Link(0x44, 2, 1, []byte{0xc0, 0xc1, 129, 0, 0}))
			if mode == "application" {
				require.Nil(t, good.Err)
				require.Len(t, good.Events, 1)
				require.Equal(t, "READ", good.Events[0].Session["In Reply To"])
			} else {
				require.Empty(t, good.Events)
				rocTypedError(t, "FatalSessionError", good.Err)
			}
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDNP3ContextMemoryAdmission(t *testing.T) {
	for _, response := range []bool{true, false} {
		t.Run(fmt.Sprint(response), func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxBufferedBytes = 350
			b.MaxMessageBytes = 64
			b.MaxFrameBytes = 64
			s, err := NewProtocolSession(b)
			require.NoError(t, err)
			raw := dnp3Link(0xc4, 1, 2, []byte{0xc0, 0xc1, 1})
			out := s.Feed(0, time.Unix(1, 0), raw)
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			require.EqualValues(t, 336, s.Stats().BufferedBytes)
			next := dnp3Link(0xc4, 1, 2, []byte{0xc0, 0xc2, 1})
			dir := 0
			if response {
				next = dnp3Link(0x44, 2, 1, []byte{0xc0, 0xc1, 129, 0, 0})
				dir = 1
			}
			out = s.Feed(dir, time.Unix(2, 0), next)
			require.Len(t, out.Events, 1)
			require.Equal(t, next, out.Events[0].Raw)
			if response {
				require.Nil(t, out.Err)
				require.Equal(t, "READ", out.Events[0].Session["In Reply To"])
			} else {
				rocTypedError(t, "ResourceExceeded", out.Err)
				require.Zero(t, s.Stats().BufferedBytes)
			}
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
