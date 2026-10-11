package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// Answers describe complete stateless transfers, not a verified invocation or
// actual node restart. The sealed independent codec/DSDL witness owns this scope.
func droneCANRestartNodeSealed(t *testing.T) {
	raw, err := trafficfixture.ReadFile("dronecan-restart-node/manifest.json")
	require.NoError(t, err)
	var manifest struct {
		Schema string
		Cases  []rocControl
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, "dronecan-restart-node/v1", manifest.Schema)
	require.Len(t, manifest.Cases, 29)
	frozen, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	bound := map[string]trafficfixture.FrozenCase{}
	for _, batch := range frozen {
		for _, entry := range batch.Cases {
			bound[entry.ID] = entry
		}
	}
	t.Run("ResourceAndInterface", droneCANRestartNodeResources)
	for _, c := range manifest.Cases {
		t.Run(c.ID, func(t *testing.T) {
			v, ok := bound["dronecan-restart-node/"+c.ID]
			require.True(t, ok)
			require.Equal(t, c.SHA256, v.Input.SHA256)
			require.Equal(t, c.Capture, v.Input.OriginalPath)
			require.Equal(t, 1, v.Facts.PacketCount)
			require.Len(t, v.Expectations, 1)
			var binding struct {
				AnswerFile string `json:"answer_file"`
				AnswerSHA  string `json:"answer_sha256"`
			}
			require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &binding))
			require.Equal(t, c.Answer, binding.AnswerFile)
			require.Equal(t, c.AnswerSHA, binding.AnswerSHA)

			capture, err := trafficfixture.ReadFile(strings.TrimPrefix(c.Capture, "common/pcapx/pcaputil/"))
			require.NoError(t, err)
			answer, err := trafficfixture.ReadFile(strings.TrimPrefix(c.Answer, "common/pcapx/pcaputil/"))
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(capture)))
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
			var a struct {
				Wire, Error, Profile string
				BaseFields           map[string]any `json:"base_fields"`
				Fields               map[string]any
			}
			require.NoError(t, json.Unmarshal(answer, &a))
			wire, err := hex.DecodeString(a.Wire)
			require.NoError(t, err)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						for _, selected := range []bool{false, true} {
							t.Run(fmt.Sprintf("w%d/d%t/o%t/s%t", workers, deferred, observe, selected), func(t *testing.T) {
								var events []*ProtocolEvent
								var ps ProtocolStats
								var rs TCPReassemblyStats
								seen := 0
								opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { ps = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { rs = s })}
								if selected {
									opts = append(opts, WithCANDecodeAs(0, "dronecan"))
								}
								if observe {
									opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.Equal(t, wire, p.Data()); seen++ }))
								}
								input := bytes.Clone(capture)
								require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
								clear(input)
								require.Len(t, events, 1)
								e := events[0]
								require.Equal(t, "can", e.Transport)
								require.Zero(t, e.FlowID)
								require.Zero(t, e.ResponseTo)
								require.Zero(t, e.TransactionID)
								require.Empty(t, e.Source)
								require.Empty(t, e.Destination)
								require.Equal(t, wire, e.Raw)
								require.Len(t, e.SourceBytes.PacketRefs, 1)
								require.EqualValues(t, 1, e.SourceBytes.PacketRefs[0].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
								require.Zero(t, e.Domain.Section)
								require.Zero(t, e.Domain.Interface)
								want, kind, protocol, profile := a.BaseFields, "", "socketcan", "socketcan-controller-record"
								if selected {
									want, kind, protocol, profile = a.Fields, a.Error, "dronecan", a.Profile
								}
								require.Equal(t, protocol, e.Protocol)
								if kind != "" {
									rocTypedError(t, kind, e.sessionError)
									require.Nil(t, e.Session)
									require.Nil(t, e.Structured)
									require.NotEmpty(t, e.Error)
									e.Session = map[string]any{"forged": true}
									e.Status = "decoded"
									e.Error = ""
									e.Protocol = "http"
									for _, query := range []func() (map[string]any, error){e.Decode, e.GetFields} {
										got, err := query()
										require.Nil(t, got)
										rocTypedError(t, kind, err)
									}
								} else {
									require.Empty(t, e.Error)
									status := "decoded"
									if deferred {
										status = "deferred"
									}
									require.Equal(t, status, e.Status)
									rocEqualFields(t, want, e.Session)
									for _, query := range []func() (map[string]any, error){e.GetFields, e.Decode} {
										got, err := query()
										require.NoError(t, err)
										if _, ok := got["fields"]; ok {
											rocEqualFields(t, want, protocolFields(got))
											rocEqualFields(t, want, got["session"].(map[string]any))
										} else {
											rocEqualFields(t, want, got)
										}
										clear(got)
									}
									e.Protocol = "http"
									e.Transport = "tcp"
									e.Session = map[string]any{"forged": true}
									e.Structured = map[string]any{"fields": map[string]any{"forged": true}}
									clear(e.Raw)
									got, err := e.Decode()
									require.NoError(t, err)
									rocEqualFields(t, want, protocolFields(got))
									rocEqualFields(t, want, got["session"].(map[string]any))
								}
								require.Equal(t, profile, e.Profile)
								require.Zero(t, ps.BufferedBytes)
								require.Zero(t, rs.UnreassembledBytes)
								require.Zero(t, rs.UnreassembledSegments)
								require.Zero(t, rs.DecodeErrors)
								require.EqualValues(t, 1, rs.CapturedPackets)
								if observe {
									require.Equal(t, 1, seen)
								} else {
									require.Zero(t, seen)
								}
							})
						}
					}
				}
			}
		})
	}
}

func droneCANRestartNodeResources(t *testing.T) {
	raw, err := trafficfixture.ReadFile("dronecan-restart-node/captures/valid-magic.pcap")
	require.NoError(t, err)
	wire := bytes.Clone(raw[40:])
	for _, limit := range []int{15, 16} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("limit%d/deferred%t", limit, deferred), func(t *testing.T) {
				budget := DefaultParserBudget()
				budget.MaxFrameBytes = limit
				budget.MaxMessageBytes = limit
				budget.MaxBufferedBytes = 1
				budget.MaxCollectionElements = 1
				var events []*ProtocolEvent
				a := &binParser{budget: budget, canDecodeAs: map[int]string{1: "dronecan"}, config: BinParserConfig{Deferred: deferred, MaxBufferedBytes: 1, OnEvent: func(e *ProtocolEvent) { events = append(events, e) }}}
				input := bytes.Clone(wire)
				for _, iface := range []int{1, 2} {
					ci := gopacket.CaptureInfo{CaptureLength: 16, Length: 16, InterfaceIndex: iface}
					a.decodeCANRecord(input, captureEvidence{Ref: PacketReference{Number: 10, Domain: CaptureDomain{Section: 3, Interface: iface}}}, ci)
				}
				clear(input)
				require.Len(t, events, 2)
				require.NotEqual(t, events[0].Domain, events[1].Domain)
				for i, e := range events {
					require.EqualValues(t, 10, e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
					require.Zero(t, e.FlowID)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
					if limit == 15 {
						require.Empty(t, e.Raw)
						require.Nil(t, e.Session)
						rocTypedError(t, "ResourceExceeded", e.sessionError)
						continue
					}
					require.Equal(t, wire, e.Raw)
					fields, err := e.GetFields()
					require.NoError(t, err)
					if i == 0 {
						require.Equal(t, "dronecan-v0-restart-node", e.Profile)
						f := fields["DroneCAN"].(map[string]any)
						require.EqualValues(t, 0xACCE551B1E, f["Magic Number"])
						require.Equal(t, true, f["Magic Number Matches"])
						require.Equal(t, "request", f["Transfer Type"])
						require.EqualValues(t, 42, f["Source Node ID"])
						require.EqualValues(t, 20, f["Destination Node ID"])
					} else {
						require.Equal(t, "socketcan", e.Protocol)
						require.NotContains(t, fields, "DroneCAN")
					}
				}
				require.Zero(t, a.stats().BufferedBytes)
			})
		}
	}
}
