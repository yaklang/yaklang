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

type iecContextAnswer struct {
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
	} `json:"events"`
}

func iecContextRead(t *testing.T, name string) []byte {
	t.Helper()
	var b []byte
	var err error
	if strings.HasPrefix(name, "common/") {
		root, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(root, name))
	} else if root := os.Getenv("IEC104_CONTEXT_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, name))
	} else {
		b, err = trafficfixture.ReadFile("iec104-command-context/" + name)
	}
	require.NoError(t, err)
	return b
}
func iecContextCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var m struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(iecContextRead(t, "manifest.json"), &m))
	require.Equal(t, 1, m.SchemaVersion)
	require.Len(t, m.Cases, 19)
	if os.Getenv("IEC104_CONTEXT_TEST_ROOT") == "" {
		batches, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		indexed := map[string]trafficfixture.FrozenCase{}
		for _, batch := range batches {
			for _, c := range batch.Cases {
				indexed[c.ID] = c
			}
		}
		for _, c := range m.Cases {
			f, ok := indexed["iec104-command-context/"+c.ID]
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
func iecContextInput(t *testing.T, c mmsISOCase) ([]byte, iecContextAnswer) {
	t.Helper()
	raw, b := iecContextRead(t, c.Capture), iecContextRead(t, c.Answer)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
	var a iecContextAnswer
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&a))
	require.Equal(t, c.Packets, a.Packets)
	require.Len(t, a.Events, len(a.Steps))
	return raw, a
}
func iecContextCheck(t *testing.T, a iecContextAnswer, events []*ProtocolEvent, deferred bool) {
	t.Helper()
	require.Len(t, events, len(a.Events))
	for i, e := range events {
		w := a.Events[i]
		require.Equal(t, "iec104", e.Protocol)
		require.Equal(t, "iec104", e.Profile)
		require.Equal(t, "tcp", e.Transport)
		require.Equal(t, w.ID, e.ID)
		require.Equal(t, w.Dir, e.Direction)
		require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
		require.Equal(t, len(e.Raw), e.Length)
		require.Equal(t, w.ResponseTo, e.ResponseTo)
		require.Equal(t, w.TransactionID, e.TransactionID)
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
func TestIEC104ContextSealedSession(t *testing.T) {
	for _, c := range iecContextCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := iecContextInput(t, c)
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
								require.Nil(t, out.Err)
							}
							clear(part)
							all = append(all, out.Events...)
						}
					}
					all = append(all, s.Close("end")...)
					iecContextCheck(t, a, all, false)
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
func TestIEC104ContextSealedReplay(t *testing.T) {
	for _, c := range iecContextCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := iecContextInput(t, c)
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
							iecContextCheck(t, a, all, deferred)
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
func TestIEC104ContextOwnedQueries(t *testing.T) {
	for _, c := range iecContextCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := iecContextInput(t, c)
			s, err := NewProtocolSession(DefaultParserBudget())
			require.NoError(t, err)
			var events []*ProtocolEvent
			for i, step := range a.Steps {
				raw, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				out := s.Feed(step.Dir, time.Unix(int64(i+1), 0), raw)
				require.Nil(t, out.Err)
				clear(raw)
				for _, e := range out.Events {
					before, err := e.Decode()
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
			iecContextCheck(t, a, events, false)
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestIEC104ContextBudgetAndLateReply(t *testing.T) {
	for _, identity := range []string{"originator", "object-address"} {
		t.Run(identity, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxCollectionElements = 4
			s, err := NewProtocolSession(b)
			require.NoError(t, err)
			for i, c := range []byte{7, 11} {
				out := s.Feed(i, time.Unix(int64(i+1), 0), iec104U(c))
				require.Nil(t, out.Err)
				require.Len(t, out.Events, 1)
			}
			for i := 0; i < 5; i++ {
				oa := byte(1)
				ioa := uint32(i)
				if identity == "originator" {
					oa = byte(i)
					ioa = 0
				}
				raw := iec104I(uint16(i), 0, 100, uint16(oa)<<8|6, ioa, []byte{20})
				out := s.Feed(0, time.Unix(int64(i+3), 0), raw)
				require.Len(t, out.Events, 1)
				e := out.Events[0]
				require.EqualValues(t, i+3, e.ID)
				require.Equal(t, raw, e.Raw)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				if i < 4 {
					require.Nil(t, out.Err)
					require.Equal(t, "decoded", e.Status)
					fields, err := e.GetFields()
					require.NoError(t, err)
					require.NotEmpty(t, fields)
					continue
				}
				rocTypedError(t, "ResourceExceeded", out.Err)
				require.Equal(t, "limited", e.Status)
				require.Nil(t, e.Session)
				require.Zero(t, s.Stats().BufferedBytes)
				e.Fields = map[string]any{"CALLER": "fake-success"}
				e.Structured = map[string]any{"fields": e.Fields}
				e.Status = "decoded"
				fields, err := e.GetFields()
				require.Nil(t, fields)
				rocTypedError(t, "ResourceExceeded", err)
				var pe *ProtocolError
				require.True(t, errors.As(err, &pe))
				pe.Kind = ErrMalformedMessage
				pe.Message = "CALLER"
				_, again := e.GetFields()
				rocTypedError(t, "ResourceExceeded", again)
			}
			late := s.Feed(1, time.Unix(100, 0), iec104I(0, 5, 100, 7, 0, []byte{20}))
			require.Empty(t, late.Events)
			rocTypedError(t, "FatalSessionError", late.Err)
			require.Empty(t, s.Close("end"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
