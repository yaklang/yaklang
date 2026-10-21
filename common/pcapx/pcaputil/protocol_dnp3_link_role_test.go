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

type dnpLinkRoleAnswer = dnpDirectAnswer

func dnpLinkRoleRead(t *testing.T, name string) []byte {
	t.Helper()
	var b []byte
	var err error
	if strings.HasPrefix(name, "common/") {
		root, e := trafficfixture.RepositoryRoot()
		require.NoError(t, e)
		b, err = trafficfixture.ReadFile(filepath.Join(root, name))
	} else if root := os.Getenv("DNP3_LINK_ROLE_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, name))
	} else {
		b, err = trafficfixture.ReadFile("dnp3-link-role/" + name)
	}
	require.NoError(t, err)
	return b
}
func dnpLinkRoleCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var m struct {
		SchemaVersion int `json:"schema_version"`
		Cases         []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(dnpLinkRoleRead(t, "manifest.json"), &m))
	require.Equal(t, 1, m.SchemaVersion)
	require.Len(t, m.Cases, 8)
	if os.Getenv("DNP3_LINK_ROLE_TEST_ROOT") == "" {
		batches, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		indexed := map[string]trafficfixture.FrozenCase{}
		for _, batch := range batches {
			for _, c := range batch.Cases {
				indexed[c.ID] = c
			}
		}
		for _, c := range m.Cases {
			f, ok := indexed["dnp3-link-role/"+c.ID]
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
func dnpLinkRoleInput(t *testing.T, c mmsISOCase) ([]byte, dnpLinkRoleAnswer) {
	t.Helper()
	raw, b := dnpLinkRoleRead(t, c.Capture), dnpLinkRoleRead(t, c.Answer)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
	var a dnpLinkRoleAnswer
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&a))
	require.Equal(t, c.Packets, a.Packets)
	require.Len(t, a.Events, len(a.Steps))
	return raw, a
}
func dnpLinkRoleCheck(t *testing.T, a dnpLinkRoleAnswer, events []*ProtocolEvent, deferred bool) {
	dnpDirectCheck(t, a, events, deferred, "dnp3-counter-events")
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
func TestDNP3LinkRoleSealedSession(t *testing.T) {
	for _, c := range dnpLinkRoleCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := dnpLinkRoleInput(t, c)
			for _, chunk := range []int{1, 7, 100000} {
				t.Run(fmt.Sprint(chunk), func(t *testing.T) {
					options := []ProtocolSessionOption{WithSessionDNP3CounterEvents(true)}
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
					dnpLinkRoleCheck(t, a, all, false)
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
func TestDNP3LinkRoleSealedReplay(t *testing.T) {
	for _, c := range dnpLinkRoleCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := dnpLinkRoleInput(t, c)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d-d%t-o%t", worker, deferred, observe), func(t *testing.T) {
							var all []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithDNP3CounterEvents(true), WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { all = append(all, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if c.ID == "transport-composition" {
								opts = append(opts, WithDNP3TransportFragments(true))
							}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							dnpLinkRoleCheck(t, a, all, deferred)
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
func TestDNP3LinkRoleOwnedQueries(t *testing.T) {
	for _, c := range dnpLinkRoleCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			_, a := dnpLinkRoleInput(t, c)
			options := []ProtocolSessionOption{WithSessionDNP3CounterEvents(true)}
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
			dnpLinkRoleCheck(t, a, events, false)
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
