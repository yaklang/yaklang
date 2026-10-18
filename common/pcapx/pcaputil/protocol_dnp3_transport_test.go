package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

type dnpTransportAnswer struct {
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

func dnpTransportRead(t *testing.T, name string) []byte {
	t.Helper()
	var b []byte
	var err error
	if strings.HasPrefix(name, "common/") {
		root, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(root, name))
	} else if root := os.Getenv("DNP3_TRANSPORT_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, name))
	} else {
		b, err = trafficfixture.ReadFile("dnp3-transport/" + name)
	}
	require.NoError(t, err)
	return b
}
func dnpTransportCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var m struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(dnpTransportRead(t, "manifest.json"), &m))
	require.Equal(t, 1, m.SchemaVersion)
	require.Len(t, m.Cases, 20)
	if os.Getenv("DNP3_TRANSPORT_TEST_ROOT") == "" {
		batches, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		indexed := map[string]trafficfixture.FrozenCase{}
		for _, batch := range batches {
			for _, c := range batch.Cases {
				indexed[c.ID] = c
			}
		}
		for _, c := range m.Cases {
			f, ok := indexed["dnp3-transport/"+c.ID]
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
func dnpTransportInput(t *testing.T, c mmsISOCase) ([]byte, dnpTransportAnswer) {
	t.Helper()
	raw, b := dnpTransportRead(t, c.Capture), dnpTransportRead(t, c.Answer)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
	var a dnpTransportAnswer
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&a))
	require.Equal(t, c.Packets, a.Packets)
	require.GreaterOrEqual(t, len(a.Events), len(a.Steps))
	return raw, a
}
func dnpTransportCheck(t *testing.T, a dnpTransportAnswer, events []*ProtocolEvent, deferred bool) {
	t.Helper()
	require.Len(t, events, len(a.Events))
	for i, e := range events {
		w := a.Events[i]
		require.Equal(t, "dnp3", e.Protocol)
		require.Equal(t, "dnp3-read-points-transport", e.Profile)
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
			if w.Error == "NeedMore" {
				status = "incomplete"
			}
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
func TestDNP3TransportSealedSession(t *testing.T) {
	for _, c := range dnpTransportCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := dnpTransportInput(t, c)
			for _, chunk := range []int{1, 7, 100000} {
				t.Run(fmt.Sprint(chunk), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionDNP3ReadPoints(true), WithSessionDNP3TransportFragments(true))
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
					dnpTransportCheck(t, a, all, false)
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
func TestDNP3TransportSealedReplay(t *testing.T) {
	for _, c := range dnpTransportCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := dnpTransportInput(t, c)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d-d%t-o%t", worker, deferred, observe), func(t *testing.T) {
							var all []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithDNP3ReadPoints(true), WithDNP3TransportFragments(true), WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { all = append(all, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							dnpTransportCheck(t, a, all, deferred)
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
func TestDNP3TransportOwnedQueries(t *testing.T) {
	for _, c := range dnpTransportCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := dnpTransportInput(t, c)
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionDNP3ReadPoints(true), WithSessionDNP3TransportFragments(true))
			require.NoError(t, err)
			var all []*ProtocolEvent
			for i, step := range a.Steps {
				raw, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
				clear(raw)
				all = append(all, out.Events...)
			}
			all = append(all, s.Close("end")...)
			dnpTransportCheck(t, a, all, false)
			for i, e := range all {
				before, err := e.Decode()
				if a.Events[i].Error != "" {
					rocTypedError(t, a.Events[i].Error, err)
					require.Nil(t, before)
					e.Fields = map[string]any{"CALLER": true}
					e.Structured = map[string]any{"fields": e.Fields}
					e.Status = "decoded"
					fields, err := e.GetFields()
					require.Nil(t, fields)
					rocTypedError(t, a.Events[i].Error, err)
					continue
				}
				require.NoError(t, err)
				fields := before["fields"].(map[string]any)
				fields["Transport Bytes"] = -1
				if b, ok := fields["Application Data"].([]byte); ok {
					clear(b)
				}
				before["session"].(map[string]any)["Transport State"] = "CALLER"
				e.Fields = fields
				e.Structured = before
				e.Session = map[string]any{"CALLER": true}
				fresh, err := e.Decode()
				require.NoError(t, err)
				rocEqualFields(t, a.Events[i].Fields, protocolFields(fresh))
				rocEqualFields(t, a.Events[i].Session, fresh["session"].(map[string]any))
			}
			require.Zero(t, s.Stats().BufferedBytes)
			require.Empty(t, s.Close("again"))
		})
	}
}
func TestDNP3TransportRequiresTCP(t *testing.T) {
	for _, opts := range [][]ProtocolSessionOption{{WithSessionTransport("udp"), WithSessionDNP3TransportFragments(true)}, {WithSessionDNP3TransportFragments(true), WithSessionTransport("udp")}} {
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.ErrorContains(t, err, "DNP3 transport assembly requires TCP")
		require.Nil(t, s)
	}
}
func TestDNP3TransportBudgets(t *testing.T) {
	var m struct {
		Cases []struct {
			ID     string
			Budget ParserBudget
			Steps  []struct {
				Dir int
				Hex string
			}
			FailedStep       int `json:"failed_step"`
			Error, LateError string
			Status           string
		}
	}
	require.NoError(t, json.Unmarshal(dnpTransportRead(t, "guard-vectors.json"), &m))
	require.Len(t, m.Cases, 4)
	for _, c := range m.Cases {
		t.Run(c.ID, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(c.Budget, WithSessionDNP3TransportFragments(true))
			require.NoError(t, err)
			for i, step := range c.Steps {
				raw, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
				clear(raw)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				require.Equal(t, uint64(i+1), e.ID)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				if i+1 < c.FailedStep {
					require.Nil(t, out.Err)
					require.Empty(t, e.Error)
					require.Equal(t, "buffering", e.Session["Transport State"])
				} else {
					rocTypedError(t, "ResourceExceeded", out.Err)
					require.Equal(t, c.Status, e.Status)
					require.Zero(t, s.Stats().BufferedBytes)
					e.Fields = map[string]any{"CALLER": true}
					e.Structured = map[string]any{"fields": e.Fields}
					e.Status = "decoded"
					fields, err := e.GetFields()
					require.Nil(t, fields)
					rocTypedError(t, "ResourceExceeded", err)
				}
			}
			late := s.Feed(1, time.Unix(99, 0), []byte{5, 100})
			require.Empty(t, late.Events)
			rocTypedError(t, "FatalSessionError", late.Err)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestDNP3TransportCRCAndClose(t *testing.T) {
	var c mmsISOCase
	for _, x := range dnpTransportCases(t) {
		if x.ID == "source-response-wrap" {
			c = x
		}
	}
	_, a := dnpTransportInput(t, c)
	for _, bad := range []bool{false, true} {
		t.Run(fmt.Sprint(bad), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionDNP3ReadPoints(true), WithSessionDNP3TransportFragments(true))
			require.NoError(t, err)
			for i, step := range a.Steps {
				raw, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				if bad && i == 2 {
					raw[len(raw)-1] ^= 1
				}
				out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
				clear(raw)
				require.Len(t, out.Events, 1)
				if bad && i == 2 {
					rocTypedError(t, "MalformedMessage", out.Err)
					require.Contains(t, out.Events[0].Error, "user data CRC mismatch")
					require.Zero(t, s.Stats().BufferedBytes)
				} else {
					require.Nil(t, out.Err)
				}
			}
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestDNP3TransportDefaultCompatibility(t *testing.T) {
	var c mmsISOCase
	for _, x := range dnpTransportCases(t) {
		if x.ID == "source-response-wrap" {
			c = x
		}
	}
	_, a := dnpTransportInput(t, c)
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionDNP3ReadPoints(true))
	require.NoError(t, err)
	for i, step := range a.Steps {
		raw, err := hex.DecodeString(step.Hex)
		require.NoError(t, err)
		out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
		require.Len(t, out.Events, 1)
		e := out.Events[0]
		require.Equal(t, "dnp3-read-points", e.Profile)
		if i == 0 {
			require.Nil(t, out.Err)
			require.Equal(t, "READ", e.Session["Function"])
		} else {
			rocTypedError(t, "ContextRequired", out.Err)
			require.Contains(t, e.Error, "transport fragment requires reassembly")
			require.NotContains(t, e.Session, "Objects")
			require.NotContains(t, e.Session, "Application Data")
		}
	}
	s.Close("end")
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestDNP3TransportBudgetAdjacent(t *testing.T) {
	var m struct {
		Cases []struct {
			ID     string
			Budget ParserBudget
			Steps  []struct {
				Dir int
				Hex string
			}
		}
	}
	require.NoError(t, json.Unmarshal(dnpTransportRead(t, "guard-vectors.json"), &m))
	for _, c := range m.Cases {
		t.Run(c.ID, func(t *testing.T) {
			b := c.Budget
			switch c.ID {
			case "application-length":
				b.MaxMessageBytes = 96
			case "segment-count":
				b.MaxCollectionElements = 3
			case "assembly-count":
				b.MaxCollectionElements = 2
			case "memory-admission":
				b.MaxBufferedBytes = 600
			}
			s, err := NewProtocolSessionWithOptions(b, WithSessionDNP3TransportFragments(true))
			require.NoError(t, err)
			for i, step := range c.Steps {
				raw, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
				require.Nil(t, out.Err)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				require.Empty(t, e.Error)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				if c.ID == "application-length" || c.ID == "segment-count" {
					if i == len(c.Steps)-1 {
						require.Equal(t, "complete", e.Session["Transport State"])
						require.Equal(t, "WRITE", e.Session["Function"])
						app := []byte{0xc8, 2}
						app = append(app, make([]byte, 88)...)
						require.Equal(t, app, e.Session["Application Data"])
						require.Equal(t, 3, e.Session["Transport Segments"])
					} else {
						require.Equal(t, "buffering", e.Session["Transport State"])
					}
				}
			}
			closed := s.Close("end")
			if c.ID == "assembly-count" {
				require.Len(t, closed, 2)
			} else if c.ID == "memory-admission" {
				require.Len(t, closed, 1)
			} else {
				require.Empty(t, closed)
			}
			for _, e := range closed {
				require.Equal(t, "incomplete", e.Status)
				rocTypedError(t, "NeedMore", e.sessionError)
			}
			require.Zero(t, s.Stats().BufferedBytes)
			require.Empty(t, s.Close("again"))
		})
	}
}
