package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type udpBudgetControl struct {
	Name, Protocol, File, SHA256 string
	Port                         uint16
	Limit                        int `json:"max_message_bytes"`
	Packets                      int
	Steps                        []struct {
		Wire                  string `json:"wire_hex"`
		Fields                map[string]any
		Direction, Interface  int
		Timestamp             int64
		Expected, Association string
		FrameExpected         string `json:"frame_expected"`
		Response              int    `json:"response_to_ref"`
	}
}

func udpBudgetControls(t *testing.T) []udpBudgetControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("udp-terminal-budget/controls.json")
	require.NoError(t, err)
	var doc struct{ Cases []udpBudgetControl }
	var rows struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(b, &doc))
	require.NoError(t, json.Unmarshal(b, &rows))
	require.Len(t, doc.Cases, 11)
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for i, c := range doc.Cases {
		answer := "answers/" + c.Name + ".json"
		raw, err := trafficfixture.ReadFile("udp-terminal-budget/" + answer)
		require.NoError(t, err)
		require.JSONEq(t, string(rows.Cases[i]), string(raw))
		bound := 0
		for _, inv := range inventories {
			for _, row := range inv.Cases {
				if row.Input.OriginalPath != "common/pcapx/pcaputil/udp-terminal-budget/"+c.File {
					continue
				}
				require.Equal(t, c.SHA256, row.Input.SHA256)
				for _, e := range row.Expectations {
					var link struct {
						Answer  string `json:"answer_file"`
						SHA     string `json:"answer_sha256"`
						Packets int    `json:"packet_count"`
					}
					require.NoError(t, json.Unmarshal(e.PayloadConstraints, &link))
					if link.Answer == answer {
						bound++
						require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(raw)), link.SHA)
						require.Equal(t, c.Packets, link.Packets)
					}
				}
			}
		}
		require.Equal(t, 1, bound, c.Name)
	}
	return doc.Cases
}
func checkUDPBudgetEvents(t *testing.T, c udpBudgetControl, events []*ProtocolEvent, deferred bool) {
	t.Helper()
	require.Len(t, events, len(c.Steps))
	for i, e := range events {
		want := c.Steps[i]
		require.Equal(t, "udp", e.Transport)
		if want.Expected == "generic-limit" || want.Expected == "unrecognized" {
			status := "limited"
			if want.Expected == "unrecognized" {
				status = "unrecognized"
			}
			require.Equal(t, status, e.Status)
			require.Empty(t, e.Protocol)
			require.Zero(t, e.ResponseTo)
			continue
		}
		require.Equal(t, c.Protocol, e.Protocol)
		if want.Response == 0 {
			require.Zero(t, e.ResponseTo)
		} else {
			require.Equal(t, events[want.Response-1].ID, e.ResponseTo)
			require.Equal(t, events[want.Response-1].Domain, e.Domain)
			require.Equal(t, events[want.Response-1].FlowID, e.FlowID)
		}
		fields, err := e.GetFields()
		if want.Expected != "decoded" {
			rocTypedError(t, want.Expected, err)
			require.Nil(t, fields)
			require.Nil(t, e.Session)
			if want.Expected == "ResourceExceeded" {
				require.Nil(t, e.Raw)
				require.Equal(t, "limited", e.Status)
			}
			continue
		}
		require.NoError(t, err)
		rocEqualFields(t, want.Fields, fields)
		require.Equal(t, wrapperWire(t, want.Wire), e.Raw)
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		require.Equal(t, status, e.Status)
		require.Equal(t, want.Association, e.Session["Association"])
		fields["caller changed"] = true
		clear(e.Raw)
		e.Session["Association"] = "changed"
		fields, err = e.GetFields()
		require.NoError(t, err)
		rocEqualFields(t, want.Fields, fields)
	}
}
func TestUDPTerminalBudgetSealedCaptureMatrix(t *testing.T) {
	for _, c := range udpBudgetControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile("udp-terminal-budget/" + c.File)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
				var events []*ProtocolEvent
				var stats ProtocolStats
				var assembly TCPReassemblyStats
				var seen atomic.Int64
				opts := []CaptureOption{WithBinParserConfig(BinParserConfig{MaxMessageBytes: c.Limit, Deferred: d, OnEvent: func(e *ProtocolEvent) { events = append(events, e) }, OnStats: func(s ProtocolStats) { stats = s }}), WithTCPReassemblyWorkers(w), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s })}
				// Automatic admission is essential here: an unrelated unknown oversize
				// header must not quarantine an already observed native conversation.
				if o {
					opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
				}
				b := bytes.Clone(raw)
				require.NoError(t, ReplayPcap(bytes.NewReader(b), opts...))
				clear(b)
				require.EqualValues(t, c.Packets, assembly.CapturedPackets)
				require.Zero(t, assembly.DecodeErrors)
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
				if o {
					require.EqualValues(t, c.Packets, seen.Load())
				}
				for i, e := range events {
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, c.Steps[i].Interface, e.Domain.Interface)
				}
				checkUDPBudgetEvents(t, c, events, d)
				var refused uint64
				for _, s := range c.Steps {
					if s.Expected == "ResourceExceeded" || s.Expected == "generic-limit" {
						refused += uint64(len(s.Wire) / 2)
					}
				}
				require.Equal(t, refused, stats.LimitedBytes)
			})
		})
	}
}
func TestUDPTerminalBudgetSealedSessionAndFrameLimits(t *testing.T) {
	for _, c := range udpBudgetControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			unknown := false
			for _, st := range c.Steps {
				unknown = unknown || (st.Expected == "generic-limit" && st.FrameExpected == "")
			}
			for _, deferred := range []bool{false, true} {
				for _, frameOnly := range []bool{false, true} {
					// An unknown payload has no selected native frame guard.
					// Its global message-only refusal remains in both feed
					// modes and all 12 capture configurations.
					if frameOnly && unknown {
						continue
					}
					// Feed has no per-capture interface/section selection. Byte/chunk ownership
					// is checked on complete UDP datagrams; splitting UDP would be another input.
					scenario := c
					scenario.Steps = append(c.Steps[:0:0], c.Steps...)
					if frameOnly {
						for i := range scenario.Steps {
							if scenario.Steps[i].FrameExpected != "" {
								scenario.Steps[i].Expected = scenario.Steps[i].FrameExpected
							}
						}
					}
					budget := DefaultParserBudget()
					budget.MaxMessageBytes = c.Limit
					budget.MaxFrameBytes = c.Limit
					if frameOnly {
						budget.MaxMessageBytes = 2048
					}
					// Feed observes one fixed capture domain. Callers route different
					// interfaces to independent sessions; the capture matrix above
					// verifies the actual InterfaceIndex routing inside ReplayPcap.
					sessions := map[int]ProtocolSession{}
					fed := map[int]int{}
					getSession := func(iface int) ProtocolSession {
						if s := sessions[iface]; s != nil {
							return s
						}
						s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("udp"), WithSessionPorts(39000, c.Port))
						require.NoError(t, err)
						s.(*captureSession).f.a.config.Deferred = deferred
						sessions[iface] = s
						return s
					}
					var events []*ProtocolEvent
					for _, st := range scenario.Steps {
						s := getSession(st.Interface)
						wire := wrapperWire(t, st.Wire)
						out := s.Feed(st.Direction, time.Unix(st.Timestamp, 0), wire)
						clear(wire)
						require.Len(t, out.Events, 1)
						events = append(events, out.Events...)
						if st.Expected == "ResourceExceeded" {
							rocTypedError(t, "ResourceExceeded", out.Err)
							if fed[st.Interface] > 0 && st.Timestamp < 130 {
								retained := int64(512)
								if c.Protocol == "dlms-wrapper" {
									retained = 768
								}
								require.Equal(t, retained, s.Stats().BufferedBytes)
							}
						}
						fed[st.Interface]++
					}
					checkUDPBudgetEvents(t, scenario, events, deferred)
					var refused uint64
					for _, st := range scenario.Steps {
						if st.Expected == "ResourceExceeded" || st.Expected == "generic-limit" {
							refused += uint64(len(st.Wire) / 2)
						}
					}
					var actualRefused uint64
					for _, s := range sessions {
						actualRefused += s.Stats().LimitedBytes
						s.Close("sealed terminal budget cleanup")
						s.Close("idempotent")
						require.Zero(t, s.Stats().BufferedBytes)
					}
					require.Equal(t, refused, actualRefused)
				}
			}
		})
	}
}
