package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type droneCANStatsAnswer struct {
	Events []struct {
		Wire, Error string
		Refs        []uint64
		Fields      map[string]any
		Close       bool
	}
}

func droneCANTransportStatsSealed(t *testing.T) {
	raw, err := trafficfixture.ReadFile("dronecan-transport-stats/manifest.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []struct {
			ID, Capture, Answer, SHA256 string
			AnswerSHA                   string `json:"answer_sha256"`
			Packets                     int
		}
	}
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Equal(t, "dronecan-transport-stats/v1", m.Schema)
	require.Len(t, m.Cases, 26)
	frozen, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	bound := map[string]trafficfixture.FrozenCase{}
	for _, b := range frozen {
		for _, v := range b.Cases {
			bound[v.ID] = v
		}
	}
	for _, c := range m.Cases {
		t.Run(c.ID, func(t *testing.T) {
			v, ok := bound["dronecan-transport-stats/"+c.ID]
			require.True(t, ok)
			require.Equal(t, c.SHA256, v.Input.SHA256)
			require.Equal(t, c.Capture, v.Input.OriginalPath)
			require.Equal(t, c.Packets, v.Facts.PacketCount)
			require.Len(t, v.Expectations, 1)
			var binding struct {
				Answer  string `json:"answer_file"`
				SHA     string `json:"answer_sha256"`
				Packets int    `json:"packet_count"`
			}
			require.NoError(t, json.Unmarshal(v.Expectations[0].PayloadConstraints, &binding))
			require.Equal(t, c.Answer, binding.Answer)
			require.Equal(t, c.AnswerSHA, binding.SHA)
			require.Equal(t, c.Packets, binding.Packets)
			capture, err := trafficfixture.ReadFile(strings.TrimPrefix(c.Capture, "common/pcapx/pcaputil/"))
			require.NoError(t, err)
			answer, err := trafficfixture.ReadFile(strings.TrimPrefix(c.Answer, "common/pcapx/pcaputil/"))
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(capture)))
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
			var a droneCANStatsAnswer
			require.NoError(t, json.Unmarshal(answer, &a))
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var ps ProtocolStats
							var rs TCPReassemblyStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithCANDecodeAs(0, "dronecan-stats"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { ps = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { rs = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(p gopacket.Packet) {
									w, err := hex.DecodeString(a.Events[seen].Wire)
									require.NoError(t, err)
									require.Equal(t, w, p.Data())
									seen++
								}))
							}
							input := bytes.Clone(capture)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							require.Len(t, events, len(a.Events))
							var decoded, incomplete, context, malformed int
							for i, want := range a.Events {
								e := events[i]
								require.EqualValues(t, i+1, e.ID)
								require.Equal(t, "can", e.Transport)
								require.Equal(t, "dronecan", e.Protocol)
								require.Equal(t, "dronecan-v0-transport-stats", e.Profile)
								require.Zero(t, e.FlowID)
								require.Zero(t, e.ResponseTo)
								require.Zero(t, e.TransactionID)
								require.Empty(t, e.Source)
								require.Empty(t, e.Destination)
								w, err := hex.DecodeString(want.Wire)
								require.NoError(t, err)
								if !want.Close {
									require.Equal(t, w, e.Raw)
								} else {
									require.Empty(t, e.Raw)
								}
								require.Len(t, e.SourceBytes.PacketRefs, len(want.Refs))
								for j, n := range want.Refs {
									require.Equal(t, n, e.SourceBytes.PacketRefs[j].Number)
									require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[j].Domain)
								}
								if want.Close || len(want.Refs) > 1 {
									require.Equal(t, "reassembled", e.SourceBytes.Kind)
								} else {
									require.Equal(t, "captured", e.SourceBytes.Kind)
								}
								if want.Error != "" {
									rocTypedError(t, want.Error, e.sessionError)
									require.Nil(t, e.Session)
									require.Nil(t, e.Structured)
									require.NotEmpty(t, e.Error)
									switch want.Error {
									case "NeedMore":
										incomplete++
									case "ContextRequired", "UnsupportedFeature":
										context++
									case "MalformedMessage":
										malformed++
									}
									e.Protocol, e.Status, e.Error = "http", "decoded", ""
									e.Session = map[string]any{"forged": true}
									for _, q := range []func() (map[string]any, error){e.GetFields, e.Decode} {
										f, err := q()
										require.Nil(t, f)
										rocTypedError(t, want.Error, err)
									}
								} else {
									decoded++
									require.Empty(t, e.Error)
									rocEqualFields(t, want.Fields, e.Session)
									status := "decoded"
									if deferred {
										status = "deferred"
									}
									require.Equal(t, status, e.Status)
									for _, q := range []func() (map[string]any, error){e.GetFields, e.Decode} {
										f, err := q()
										require.NoError(t, err)
										if _, ok := f["fields"]; ok {
											rocEqualFields(t, want.Fields, protocolFields(f))
											rocEqualFields(t, want.Fields, f["session"].(map[string]any))
										} else {
											rocEqualFields(t, want.Fields, f)
										}
										droneCANStatsMutateFields(f)
									}
									e.Session = map[string]any{"forged": true}
									e.Structured = nil
									e.Protocol, e.Transport = "http", "tcp"
									clear(e.Raw)
									f, err := e.Decode()
									require.NoError(t, err)
									rocEqualFields(t, want.Fields, protocolFields(f))
									rocEqualFields(t, want.Fields, f["session"].(map[string]any))
								}
							}
							require.EqualValues(t, incomplete, ps.Incomplete)
							require.EqualValues(t, context, ps.ContextRequired)
							require.EqualValues(t, malformed, ps.Malformed)
							if deferred {
								require.EqualValues(t, decoded, ps.Deferred)
							} else {
								require.EqualValues(t, decoded, ps.Decoded)
							}
							if decoded > 0 {
								require.Greater(t, ps.PeakBufferedBytes, int64(0))
							}
							require.Zero(t, ps.BufferedBytes)
							require.Zero(t, rs.UnreassembledBytes)
							require.Zero(t, rs.UnreassembledSegments)
							require.Zero(t, rs.DecodeErrors)
							require.EqualValues(t, c.Packets, rs.CapturedPackets)
							if observe {
								require.Equal(t, c.Packets, seen)
							} else {
								require.Zero(t, seen)
							}
						})
					}
				}
			}
		})
	}
}

// Exact frames are read from the sealed independent codec witness. Direct
// delivery exposes capture domains and budgets unavailable in classic PCAP.
func droneCANStatsFrames(t *testing.T) [][]byte {
	b, err := trafficfixture.ReadFile("dronecan-transport-stats/answers/no-interfaces.json")
	require.NoError(t, err)
	var a droneCANStatsAnswer
	require.NoError(t, json.Unmarshal(b, &a))
	var out [][]byte
	for _, e := range a.Events {
		w, err := hex.DecodeString(e.Wire)
		require.NoError(t, err)
		out = append(out, w)
	}
	return out
}
func droneCANStatsParser(events *[]*ProtocolEvent, b ParserBudget) *binParser {
	return &binParser{budget: b, canDecodeAs: map[int]string{0: "dronecan-stats", 1: "dronecan-stats"}, config: BinParserConfig{MaxBufferedBytes: b.MaxBufferedBytes, OnEvent: func(e *ProtocolEvent) { *events = append(*events, e) }}}
}
func droneCANStatsDeliver(a *binParser, w []byte, n uint64, domain CaptureDomain, at time.Time) {
	a.decodeCANRecord(w, captureEvidence{Ref: PacketReference{Number: n, Domain: domain}}, gopacket.CaptureInfo{Timestamp: at, CaptureLength: len(w), Length: len(w), InterfaceIndex: domain.Interface})
}
func TestDroneCANStatsIsolationAndResources(t *testing.T) {
	frames := droneCANStatsFrames(t)
	now := time.Unix(1700001000, 0)
	t.Run("CaptureDomainsAndOwnership", func(t *testing.T) {
		var events []*ProtocolEvent
		a := droneCANStatsParser(&events, DefaultParserBudget())
		domains := []CaptureDomain{{}, {Interface: 1}, {Section: 1}, {Encapsulation: "vpn/1"}}
		for i, w := range frames {
			for j, d := range domains {
				input := bytes.Clone(w)
				droneCANStatsDeliver(a, input, uint64(i*4+j+1), d, now.Add(time.Duration(i)*time.Millisecond))
				clear(input)
			}
		}
		require.Len(t, events, 12)
		for j, d := range domains {
			e := events[8+j]
			f, err := e.GetFields()
			require.NoError(t, err)
			require.EqualValues(t, 1, f["DroneCAN"].(map[string]any)["Transfers TX"])
			require.Equal(t, d, e.Domain)
			require.Len(t, e.SourceBytes.PacketRefs, 3)
			for i, r := range e.SourceBytes.PacketRefs {
				require.Equal(t, d, r.Domain)
				require.EqualValues(t, i*4+j+1, r.Number)
			}
		}
		a.closeDroneCANStats()
		a.closeDroneCANStats()
		require.Len(t, events, 12)
		require.Zero(t, a.stats().BufferedBytes)
	})
	t.Run("OuterRefusalRetiresPartial", func(t *testing.T) {
		for _, truncated := range []bool{false, true} {
			var events []*ProtocolEvent
			b := DefaultParserBudget()
			a := droneCANStatsParser(&events, b)
			droneCANStatsDeliver(a, frames[0], 1, CaptureDomain{}, now)
			if truncated {
				a.decodeCANRecord(frames[1][:15], captureEvidence{Ref: PacketReference{Number: 2}}, gopacket.CaptureInfo{CaptureLength: 15, Length: 16, Timestamp: now})
			} else {
				a.budget.MaxFrameBytes = 15
				droneCANStatsDeliver(a, frames[1], 2, CaptureDomain{}, now)
				a.budget.MaxFrameBytes = b.MaxFrameBytes
			}
			droneCANStatsDeliver(a, frames[2], 3, CaptureDomain{}, now)
			rocTypedError(t, "ResourceExceeded", events[2].sessionError)
			require.Nil(t, events[2].Session)
			a.closeDroneCANStats()
			require.Len(t, events, 3)
			require.Zero(t, a.stats().BufferedBytes)
		}
	})
	t.Run("ElementAndByteBudgetFailClosed", func(t *testing.T) {
		for _, elements := range []bool{false, true} {
			var events []*ProtocolEvent
			b := DefaultParserBudget()
			if elements {
				b.MaxCollectionElements = 1
			} else {
				b.MaxBufferedBytes = 1
			}
			a := droneCANStatsParser(&events, b)
			for i, w := range frames {
				droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now)
			}
			if elements {
				rocTypedError(t, "NeedMore", events[0].sessionError)
				rocTypedError(t, "ResourceExceeded", events[1].sessionError)
				rocTypedError(t, "ContextRequired", events[2].sessionError)
			} else {
				for _, e := range events {
					rocTypedError(t, "ResourceExceeded", e.sessionError)
				}
			}
			a.closeDroneCANStats()
			require.Len(t, events, 3)
			require.Zero(t, a.stats().BufferedBytes)
		}
	})
	t.Run("TimeoutAndBackdated", func(t *testing.T) {
		for _, backward := range []bool{false, true} {
			var events []*ProtocolEvent
			a := droneCANStatsParser(&events, DefaultParserBudget())
			droneCANStatsDeliver(a, frames[0], 1, CaptureDomain{}, now)
			at := now.Add(3 * time.Second)
			if backward {
				at = now.Add(-time.Second)
			}
			droneCANStatsDeliver(a, frames[1], 2, CaptureDomain{}, at)
			rocTypedError(t, "ContextRequired", events[1].sessionError)
			droneCANStatsDeliver(a, frames[2], 3, CaptureDomain{}, now.Add(4*time.Second))
			rocTypedError(t, "ContextRequired", events[2].sessionError)
			a.closeDroneCANStats()
			require.Len(t, events, 3)
			require.Zero(t, a.stats().BufferedBytes)
		}
	})
	t.Run("BackdatedCannotAdvanceCompletedHistory", func(t *testing.T) {
		var events []*ProtocolEvent
		a := droneCANStatsParser(&events, DefaultParserBudget())
		for i, w := range frames {
			droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now.Add(time.Duration(i)*time.Millisecond))
		}
		droneCANStatsDeliver(a, frames[1], 4, CaptureDomain{}, now.Add(-time.Second))
		rocTypedError(t, "ContextRequired", events[3].sessionError)
		for i, w := range frames {
			droneCANStatsDeliver(a, w, uint64(i+5), CaptureDomain{}, now.Add(time.Second))
			rocTypedError(t, "ContextRequired", events[i+4].sessionError)
		}
		a.closeDroneCANStats()
		require.Len(t, events, 7)
		require.Zero(t, a.stats().BufferedBytes)
	})
	t.Run("UntrackedDescriptorExhaustion", func(t *testing.T) {
		var events []*ProtocolEvent
		b := DefaultParserBudget()
		b.MaxCollectionElements = 1
		a := droneCANStatsParser(&events, b)
		droneCANStatsDeliver(a, frames[0], 1, CaptureDomain{}, now)
		droneCANStatsDeliver(a, frames[0], 2, CaptureDomain{Interface: 1}, now)
		rocTypedError(t, "ResourceExceeded", events[1].sessionError)
		for i, w := range frames[1:] {
			droneCANStatsDeliver(a, w, uint64(i+3), CaptureDomain{}, now)
			rocTypedError(t, "ResourceExceeded", events[i+2].sessionError)
		}
		a.closeDroneCANStats()
		a.closeDroneCANStats()
		require.Len(t, events, 4)
		require.Zero(t, a.stats().BufferedBytes)
	})
	t.Run("PartialCloseIsIdempotent", func(t *testing.T) {
		var events []*ProtocolEvent
		a := droneCANStatsParser(&events, DefaultParserBudget())
		droneCANStatsDeliver(a, frames[0], 9, CaptureDomain{}, now)
		a.closeDroneCANStats()
		a.closeDroneCANStats()
		require.Len(t, events, 2)
		rocTypedError(t, "NeedMore", events[1].sessionError)
		require.Empty(t, events[1].Raw)
		require.Nil(t, events[1].Session)
		require.Equal(t, []PacketReference{{Number: 9}}, events[1].SourceBytes.PacketRefs)
		require.Zero(t, a.stats().BufferedBytes)
	})
	t.Run("MessageBodyBudget", func(t *testing.T) {
		var events []*ProtocolEvent
		b := DefaultParserBudget()
		b.MaxMessageBytes = 16
		a := droneCANStatsParser(&events, b)
		for i, w := range frames {
			droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now)
		}
		rocTypedError(t, "NeedMore", events[0].sessionError)
		rocTypedError(t, "NeedMore", events[1].sessionError)
		rocTypedError(t, "ResourceExceeded", events[2].sessionError)
		a.closeDroneCANStats()
		require.Len(t, events, 3)
		require.Zero(t, a.stats().BufferedBytes)
	})

	t.Run("RepeatedStartCannotConcatenate", func(t *testing.T) {
		var events []*ProtocolEvent
		a := droneCANStatsParser(&events, DefaultParserBudget())
		input := [][]byte{frames[0], frames[1], frames[0], frames[2]}
		for i, w := range input {
			droneCANStatsDeliver(a, w, uint64(i+1), CaptureDomain{}, now)
		}
		require.Len(t, events, 4)
		rocTypedError(t, "ContextRequired", events[2].sessionError)
		f, err := events[3].GetFields()
		require.NoError(t, err)
		require.EqualValues(t, 1, f["DroneCAN"].(map[string]any)["Transfers TX"])
		require.Equal(t, []PacketReference{{Number: 1}, {Number: 2}, {Number: 4}}, events[3].SourceBytes.PacketRefs)
		a.closeDroneCANStats()
		require.Zero(t, a.stats().BufferedBytes)
	})

	t.Run("ConcurrentCloseReleasesReceiver", func(t *testing.T) {
		a := droneCANStatsParser(new([]*ProtocolEvent), DefaultParserBudget())
		a.config.OnEvent = nil
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for _, w := range frames {
					droneCANStatsDeliver(a, w, 1, CaptureDomain{}, now)
				}
			}()
		}
		wg.Add(1)
		go func() { defer wg.Done(); a.closeDroneCANStats(); a.closeDroneCANStats() }()
		wg.Wait()
		a.closeDroneCANStats()
		require.Zero(t, a.stats().BufferedBytes)
		require.Nil(t, a.droneCANStats)
	})

}

func droneCANStatsMutateFields(v any) {
	switch x := v.(type) {
	case map[string]any:
		for _, v := range x {
			droneCANStatsMutateFields(v)
		}
		clear(x)
	case []any:
		for _, v := range x {
			droneCANStatsMutateFields(v)
		}
		clear(x)
	case []byte:
		clear(x)
	}
}
