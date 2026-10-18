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

type dnpPointsAnswer struct {
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

func dnpPointsRead(t *testing.T, name string) []byte {
	t.Helper()
	var b []byte
	var err error
	if strings.HasPrefix(name, "common/") {
		root, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(root, name))
	} else if root := os.Getenv("DNP3_POINTS_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, name))
	} else {
		b, err = trafficfixture.ReadFile("dnp3-read-points/" + name)
	}
	require.NoError(t, err)
	return b
}
func dnpPointsCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var m struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(dnpPointsRead(t, "manifest.json"), &m))
	require.Equal(t, 1, m.SchemaVersion)
	require.Len(t, m.Cases, 21)
	if os.Getenv("DNP3_POINTS_TEST_ROOT") == "" {
		batches, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		indexed := map[string]trafficfixture.FrozenCase{}
		for _, batch := range batches {
			for _, c := range batch.Cases {
				indexed[c.ID] = c
			}
		}
		for _, c := range m.Cases {
			f, ok := indexed["dnp3-read-points/"+c.ID]
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
func dnpPointsInput(t *testing.T, c mmsISOCase) ([]byte, dnpPointsAnswer) {
	t.Helper()
	raw, b := dnpPointsRead(t, c.Capture), dnpPointsRead(t, c.Answer)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
	var a dnpPointsAnswer
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&a))
	require.Equal(t, c.Packets, a.Packets)
	require.Len(t, a.Events, len(a.Steps))
	return raw, a
}
func dnpPointsCheck(t *testing.T, a dnpPointsAnswer, events []*ProtocolEvent, deferred bool) {
	t.Helper()
	require.Len(t, events, len(a.Events))
	for i, e := range events {
		w := a.Events[i]
		require.Equal(t, "dnp3", e.Protocol)
		require.Equal(t, "dnp3-read-points", e.Profile)
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
			if w.Error == "ContextRequired" || w.Error == "UnsupportedFeature" {
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
func TestDNP3ReadPointsSealedSession(t *testing.T) {
	for _, c := range dnpPointsCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := dnpPointsInput(t, c)
			for _, chunk := range []int{1, 7, 100000} {
				t.Run(fmt.Sprint(chunk), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionDNP3ReadPoints(true))
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
					dnpPointsCheck(t, a, all, false)
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
func TestDNP3ReadPointsSealedReplay(t *testing.T) {
	for _, c := range dnpPointsCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := dnpPointsInput(t, c)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d-d%t-o%t", worker, deferred, observe), func(t *testing.T) {
							var all []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithDNP3ReadPoints(true), WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { all = append(all, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							dnpPointsCheck(t, a, all, deferred)
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
func TestDNP3ReadPointsOwnedQueries(t *testing.T) {
	for _, c := range dnpPointsCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := dnpPointsInput(t, c)
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionDNP3ReadPoints(true))
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
			dnpPointsCheck(t, a, events, false)
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDNP3ReadPointsBudgetAndOwnership(t *testing.T) {
	_, a := dnpPointsInput(t, dnpPointsCases(t)[0])
	for _, mode := range []string{"collection34", "collection33", "depth2"} {
		t.Run(mode, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxCollectionElements = 34
			if mode == "collection33" {
				b.MaxCollectionElements = 33
			}
			if mode == "depth2" {
				b.MaxRecursionDepth = 2
			}
			s, err := NewProtocolSessionWithOptions(b, WithSessionDNP3ReadPoints(true))
			require.NoError(t, err)
			for i, step := range a.Steps {
				raw, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				require.Equal(t, raw, e.Raw)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				if i == 0 || mode == "collection34" {
					require.Nil(t, out.Err)
					_, err := e.GetFields()
					require.NoError(t, err)
					continue
				}
				rocTypedError(t, "ResourceExceeded", out.Err)
				require.Equal(t, "limited", e.Status)
				require.Nil(t, e.Session)
				require.Zero(t, s.Stats().BufferedBytes)
				e.Fields = map[string]any{"Objects": "CALLER"}
				e.Structured = map[string]any{"fields": e.Fields}
				e.Status = "decoded"
				fields, err := e.GetFields()
				require.Nil(t, fields)
				rocTypedError(t, "ResourceExceeded", err)
				var pe *ProtocolError
				require.True(t, errors.As(err, &pe))
				pe.Kind = ErrMalformedMessage
				pe.Message = "CALLER"
				_, err = e.GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				out = s.Feed(step.Dir, time.Unix(99, 0), raw)
				require.Empty(t, out.Events)
				rocTypedError(t, "FatalSessionError", out.Err)
			}
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDNP3UserDataLinkControlBoundary(t *testing.T) {
	// Each exact input and independent typed refusal is sealed with the batch.
	var data struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []map[string]json.RawMessage
	}
	require.NoError(t, json.Unmarshal(dnpPointsRead(t, "guard-vectors.json"), &data))
	require.Equal(t, 1, data.SchemaVersion)
	require.Len(t, data.Cases, 2)
	for _, c := range data.Cases {
		number := func(key string) uint64 { var v uint64; require.NoError(t, json.Unmarshal(c[key], &v)); return v }
		text := func(key string) string { var v string; require.NoError(t, json.Unmarshal(c[key], &v)); return v }
		wire := func(key string) []byte { b, err := hex.DecodeString(text(key)); require.NoError(t, err); return b }
		t.Run(fmt.Sprint(number("control")), func(t *testing.T) {
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			out := s.Feed(0, time.Unix(1, 0), wire("good"))
			require.Nil(t, out.Err)
			require.Equal(t, "READ", out.Events[0].Session["Function"])
			bad := wire("bad")
			before := s.Stats()
			require.NotEqual(t, "dnp3", s.Probe(bad).Protocol)
			require.Equal(t, before, s.Stats())
			out = s.Feed(0, time.Unix(2, 0), bad)
			require.Len(t, out.Events, 1)
			e := out.Events[0]
			rocTypedError(t, text("error"), out.Err)
			require.Contains(t, e.Error, text("message"))
			require.Equal(t, bad, e.Raw)
			require.Nil(t, e.Session)
			require.Equal(t, number("bad_event_id"), e.ID)
			require.Equal(t, number("response_to"), e.ResponseTo)
			require.Equal(t, number("transaction_id"), e.TransactionID)
			e.Fields = map[string]any{"CALLER": "fake"}
			fields, err := e.GetFields()
			require.Nil(t, fields)
			rocTypedError(t, text("error"), err)
			require.EqualValues(t, number("buffered_after_refusal"), s.Stats().BufferedBytes)
			out = s.Feed(1, time.Unix(3, 0), wire("late"))
			require.Empty(t, out.Events)
			rocTypedError(t, text("late_error"), out.Err)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDNP3ReadPointsRequiresTCP(t *testing.T) {
	for _, opts := range [][]ProtocolSessionOption{{WithSessionTransport("udp"), WithSessionDNP3ReadPoints(true)}, {WithSessionDNP3ReadPoints(true), WithSessionTransport("udp")}} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.Nil(t, s)
		require.ErrorContains(t, err, "require TCP")
	}
}
