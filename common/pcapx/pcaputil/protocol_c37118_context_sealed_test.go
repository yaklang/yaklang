package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestC37118ContextSealedReplay(t *testing.T) {
	read := func(name string) []byte {
		t.Helper()
		b, err := trafficfixture.ReadFile("c37118-context/" + name)
		require.NoError(t, err)
		return b
	}
	var controls struct {
		Schema string
		Cases  []struct {
			ID, Capture, Answer, SHA256, Carrier string
			AnswerSHA                            string `json:"answer_sha256"`
			Packets                              int
			Limit                                int `json:"collection_limit"`
		}
	}
	require.NoError(t, json.Unmarshal(read("controls.json"), &controls))
	require.Equal(t, "c37118-context/v1", controls.Schema)
	require.Len(t, controls.Cases, 7)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	bound := map[string]trafficfixture.FrozenCase{}
	for _, batch := range all {
		for _, row := range batch.Cases {
			bound[row.ID] = row
		}
	}
	for _, c := range controls.Cases {
		row, ok := bound["c37118-context/"+c.ID]
		require.True(t, ok)
		require.Equal(t, c.Capture, row.Input.File)
		require.Equal(t, c.SHA256, row.Input.SHA256)
		require.Equal(t, c.Packets, row.Facts.PacketCount)
		require.Len(t, row.Expectations, 1)
		var binding struct {
			File  string `json:"answer_file"`
			SHA   string `json:"answer_sha256"`
			Limit int    `json:"collection_limit"`
		}
		require.NoError(t, json.Unmarshal(row.Expectations[0].PayloadConstraints, &binding))
		require.Equal(t, c.Answer, binding.File)
		require.Equal(t, c.AnswerSHA, binding.SHA)
		require.Equal(t, c.Limit, binding.Limit)
	}

	for _, c := range controls.Cases {
		t.Run(c.ID, func(t *testing.T) {
			raw, ab := read(c.Capture), read(c.Answer)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ab)))
			var answer struct {
				Packets int
				Events  []struct {
					Raw, Error                 string
					Fields                     map[string]any
					Direction, Interface, Peer int
					Second                     int64
					FlowGroup                  int `json:"flow_group"`
				}
			}
			require.NoError(t, json.Unmarshal(ab, &answer))
			require.Equal(t, c.Packets, answer.Packets)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var assembly TCPReassemblyStats
							var seen atomic.Int64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s })}
							if c.Carrier == "udp" {
								opts = append(opts, WithProtocolDecodeAs("udp", 28400, "c37118"))
							}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.NotEmpty(t, p.Data()); seen.Add(1) }))
							}
							owned := bytes.Clone(raw)
							analyzer, err := NewPacketAnalyzer(opts...)
							require.NoError(t, err)
							defer analyzer.Close()
							// Capture options expose byte budgets, while the existing public
							// standalone session accepts a full ParserBudget (see the regression).
							// Set only the test collection ceiling before any worker receives input.
							analyzer.conf.binParser.budget.MaxCollectionElements = c.Limit
							reader, err := NewCaptureReader(bytes.NewReader(owned))
							require.NoError(t, err)
							for {
								packet, ci, err := reader.ReadPacketData()
								if err == io.EOF {
									break
								}
								require.NoError(t, err)
								require.NoError(t, analyzer.Feed(packet, ci, reader.LinkType()))
							}
							require.NoError(t, analyzer.Close())
							require.NoError(t, analyzer.Close())
							clear(owned)
							require.Len(t, events, len(answer.Events))
							require.EqualValues(t, len(events), stats.Messages)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							require.Zero(t, assembly.UnreassembledBytes)
							require.EqualValues(t, c.Packets, assembly.CapturedPackets)
							if observe {
								require.EqualValues(t, c.Packets, seen.Load())
							}

							positives, contexts, limited := 0, 0, 0
							for _, w := range answer.Events {
								switch w.Error {
								case "":
									positives++
								case "ContextRequired":
									contexts++
								case "ResourceExceeded":
									limited += len(w.Raw) / 2
								default:
									t.Fatalf("unrecognized independently specified refusal %q", w.Error)
								}
							}
							require.EqualValues(t, contexts, stats.ContextRequired)
							require.EqualValues(t, limited, stats.LimitedBytes)
							require.Zero(t, stats.Malformed)
							require.Zero(t, stats.Incomplete)
							require.Zero(t, stats.Unknown)
							if deferred {
								require.EqualValues(t, positives, stats.Deferred)
								require.Zero(t, stats.Decoded)
							} else {
								require.EqualValues(t, positives, stats.Decoded)
								require.Zero(t, stats.Deferred)
							}
							groups := map[int]uint64{}
							for i, e := range events {
								w := answer.Events[i]
								require.EqualValues(t, i+1, e.ID)
								require.Equal(t, "c37118", e.Protocol)
								require.Equal(t, "c37118-v1-v2-cfg2-data", e.Profile)
								require.Equal(t, c.Carrier, e.Transport)
								require.True(t, time.Unix(w.Second, 0).Equal(e.Timestamp), "exact capture instant, irrespective of reader timezone location")
								require.Equal(t, w.Direction, e.Direction)
								require.Equal(t, w.Interface, e.Domain.Interface)
								require.Zero(t, e.ResponseTo)
								require.Zero(t, e.TransactionID)
								src, dst := "192.0.2.10:39300", fmt.Sprintf("192.0.2.%d:28400", w.Peer)
								if w.Direction == 1 {
									src, dst = dst, src
								}
								require.Equal(t, src, e.Source)
								require.Equal(t, dst, e.Destination)
								require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
								require.Len(t, e.SourceBytes.PacketRefs, 1)
								n := i + 1
								if c.Carrier == "tcp" {
									n += 3
								}
								require.EqualValues(t, n, e.SourceBytes.PacketRefs[0].Number)
								require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
								if w.FlowGroup == 0 {
									require.Zero(t, e.FlowID)
								} else {
									require.NotZero(t, e.FlowID)
									if id, ok := groups[w.FlowGroup]; ok {
										require.Equal(t, id, e.FlowID)
									} else {
										for _, id := range groups {
											require.NotEqual(t, id, e.FlowID)
										}
										groups[w.FlowGroup] = e.FlowID
									}
								}
								if w.Error != "" {
									rocTypedError(t, w.Error, e.sessionError)
									require.NotEmpty(t, e.Error)
									if w.Error == "ResourceExceeded" {
										require.Equal(t, "limited", e.Status)
									} else {
										require.Equal(t, "context-required", e.Status)
									}
									_, ferr := e.GetFields()
									rocTypedError(t, w.Error, ferr)
									if w.Fields == nil {
										require.Nil(t, e.Session)
									} else {
										rocEqualFields(t, w.Fields, e.Session)
									}
									_, err := e.Decode()
									rocTypedError(t, w.Error, err)
									continue
								}
								status := "decoded"
								if deferred {
									status = "deferred"
								}
								require.Equal(t, status, e.Status)
								require.Empty(t, e.Error)
								require.Nil(t, e.sessionError)
								rocEqualFields(t, w.Fields, e.Session)
								if cfg, ok := e.Session["Configuration"].(map[string]any); ok {
									cfg["Time Base"] = 999999
								}
								if data, ok := e.Session["Data"].([]map[string]any); ok {
									data[0]["Frequency"].(map[string]any)["Value"] = 999999
								}
								for _, replacement := range []map[string]any{e.Session, {"forged": true}, nil} {
									e.Session = replacement
									f, err := e.GetFields()
									require.NoError(t, err)
									rocEqualFields(t, w.Fields, f)
									d, err := e.Decode()
									require.NoError(t, err)
									rocEqualFields(t, w.Fields, protocolFields(d))
									q, ok := d["session"].(map[string]any)
									require.True(t, ok)
									rocEqualFields(t, w.Fields, q)
									clear(q)
									again, err := e.Decode()
									require.NoError(t, err)
									rocEqualFields(t, w.Fields, again["session"].(map[string]any))
								}
							}
						})
					}
				}
			}
		})
	}
}
