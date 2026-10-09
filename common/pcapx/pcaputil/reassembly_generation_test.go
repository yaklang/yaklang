package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"sync"
	"sync/atomic"
	"testing"
)

type generationTCP struct {
	Dir                  int
	Seq, Ack             uint32
	SYN, RST, FIN, NoACK bool
	Data                 string
}

func generationInput(t *testing.T, name string, steps []generationTCP) []byte {
	t.Helper()
	raw, ab := industrialInput(t, "tcp-"+name)
	var answer struct{ Steps []generationTCP }
	require.NoError(t, json.Unmarshal(ab, &answer))
	require.Equal(t, steps, answer.Steps)
	return raw
}
func generationPoolsReleased(t *testing.T, pools map[*TrafficPool]bool) {
	t.Helper()
	for p := range pools {
		require.True(t, p.closed)
		require.Zero(t, p.pendingBytes)
		require.Zero(t, p.pendingSegments)
		if p.parallel != nil {
			p.parallel.budget.mu.Lock()
			require.Zero(t, p.parallel.budget.bytes)
			require.Zero(t, p.parallel.budget.segments)
			require.Zero(t, p.parallel.budget.flows)
			p.parallel.budget.mu.Unlock()
		}
		p.Close()
	}
}

func TestTCPGenerationRejectLateReverseACK(t *testing.T) {
	for _, kind := range []string{"control", "late-old-rst", "late-old-data", "late-old-fin", "late-old-synack", "ack-equals-new-iss", "noack-old-rst", "noack-old-data", "noack-old-fin"} {
		steps := []generationTCP{{Dir: 0, Seq: 99, SYN: true}, {Dir: 1, Seq: 499, Ack: 100, SYN: true}, {Dir: 0, Seq: 100, Ack: 500, RST: true}, {Dir: 0, Seq: 999, SYN: true}}
		if kind != "control" {
			s := generationTCP{Dir: 1, Seq: 500, Ack: 100}
			switch kind {
			case "late-old-rst", "noack-old-rst":
				s.RST = true
			case "late-old-data", "noack-old-data":
				s.Data = "OLD"
			case "late-old-fin", "noack-old-fin":
				s.FIN = true
			case "late-old-synack":
				s.SYN = true
			case "ack-equals-new-iss":
				s.RST = true
				s.Ack = 999
			}
			s.NoACK = kind == "noack-old-rst" || kind == "noack-old-data" || kind == "noack-old-fin"
			steps = append(steps, s)
		}
		steps = append(steps, generationTCP{Dir: 1, Seq: 1999, Ack: 1000, SYN: true}, generationTCP{Dir: 0, Seq: 1000, Ack: 2000, Data: "NEW"}, generationTCP{Dir: 1, Seq: 2000, Ack: 1003, Data: "FRESH"})
		for _, workers := range []int{1, 2, 4} {
			for _, observe := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/w%d/o%t", kind, workers, observe), func(t *testing.T) {
					wire := generationInput(t, kind, steps)
					var mu sync.Mutex
					flows := map[*TrafficFlow]int{}
					pools := map[*TrafficPool]bool{}
					frames := map[int][2]string{}
					var stats TCPReassemblyStats
					var seen atomic.Int64
					opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithTCPReassemblyStream(64), WithOnTrafficFlowCreated(func(f *TrafficFlow) { mu.Lock(); defer mu.Unlock(); flows[f] = len(flows) + 1; pools[f.pool] = true }), WithOnTrafficFlowOnDataFrameReassembled(func(f *TrafficFlow, c *TrafficConnection, fr *TrafficFrame) {
						mu.Lock()
						defer mu.Unlock()
						d := 1
						if c == f.ClientConn {
							d = 0
						}
						n := flows[f]
						v := frames[n]
						v[d] += string(fr.Payload)
						frames[n] = v
					}), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s })}
					if observe {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
					}
					require.NoError(t, ReplayPcap(bytes.NewReader(wire), opts...))
					clear(wire)
					generationPoolsReleased(t, pools)
					require.Len(t, flows, 2)
					require.Equal(t, [2]string{"NEW", "FRESH"}, frames[2])
					require.Equal(t, [2]string{}, frames[1])
					require.Zero(t, stats.UnreassembledBytes)
					require.Zero(t, stats.CallbackPanics)
					require.Zero(t, stats.UnreassembledSegments)
					require.Zero(t, stats.InvalidSegments)
					require.EqualValues(t, len(steps), stats.CapturedPackets)
					if observe {
						require.EqualValues(t, len(steps), seen.Load())
					}
				})
			}
		}
	}
}
func TestTCPGenerationAdjacentPassiveBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		steps []generationTCP
		want  [2]string
	}{
		{"current-syn-rst", []generationTCP{{Seq: 999, SYN: true}, {Dir: 1, Seq: 0, Ack: 1000, RST: true}, {Seq: 1000, Data: "ignored"}}, [2]string{}},
		{"wrap-ack-zero", []generationTCP{{Seq: 0xffffffff, SYN: true}, {Dir: 1, Seq: 499, Ack: 0, SYN: true}, {Seq: 0, Ack: 500, Data: "NEW"}, {Dir: 1, Seq: 500, Ack: 3, Data: "FRESH"}}, [2]string{"NEW", "FRESH"}},
		{"missing-synack", []generationTCP{{Seq: 999, SYN: true}, {Dir: 1, Seq: 2000, Ack: 1000, Data: "FRESH"}, {Seq: 1000, Ack: 2005, Data: "NEW"}}, [2]string{"NEW", "FRESH"}},
		{"outgoing-capture-gap", []generationTCP{{Seq: 999, SYN: true}, {Dir: 1, Seq: 2000, Ack: 1003, Data: "FRESH"}}, [2]string{"", "FRESH"}},
		{"syn-with-data", []generationTCP{{Seq: 999, SYN: true, Data: "NEW"}, {Dir: 1, Seq: 1999, Ack: 1003, SYN: true}, {Dir: 1, Seq: 2000, Ack: 1003, Data: "FRESH"}}, [2]string{"NEW", "FRESH"}},
		{"simultaneous-open", []generationTCP{{Seq: 999, SYN: true}, {Dir: 1, Seq: 1999, SYN: true, NoACK: true}, {Seq: 1000, Ack: 2000, Data: "NEW"}, {Dir: 1, Seq: 2000, Ack: 1003, Data: "FRESH"}}, [2]string{"NEW", "FRESH"}},
		{"midstream-old-looking-ack", []generationTCP{{Seq: 1000, Ack: 100, Data: "NEW"}, {Dir: 1, Seq: 2000, Ack: 100, Data: "FRESH"}}, [2]string{"NEW", "FRESH"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got [2]string
			var flow *TrafficFlow
			pools := map[*TrafficPool]bool{}
			var stats TCPReassemblyStats
			require.NoError(t, ReplayPcap(bytes.NewReader(generationInput(t, tc.name, tc.steps)), WithTCPReassemblyStream(64), WithOnTrafficFlowCreated(func(f *TrafficFlow) { require.Nil(t, flow); flow = f; pools[f.pool] = true }), WithOnTrafficFlowOnDataFrameReassembled(func(f *TrafficFlow, c *TrafficConnection, fr *TrafficFrame) {
				d := 1
				if c == f.ClientConn {
					d = 0
				}
				got[d] += string(fr.Payload)
			}), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s })))
			generationPoolsReleased(t, pools)
			require.Equal(t, tc.want, got)
			require.Zero(t, stats.UnreassembledBytes)
			require.Zero(t, stats.UnreassembledSegments)
			require.Zero(t, stats.InvalidSegments)
		})
	}
}

func TestTCPHistoricalCarrierRefusalAndCorrection(t *testing.T) {
	raw, err := trafficfixture.ReadFile("incremental-mvp/manifest.json")
	require.NoError(t, err)
	var m struct{ Cases []incrementalMVPCase }
	require.NoError(t, json.Unmarshal(raw, &m))
	count := 0
	for _, c := range m.Cases {
		if c.Group != "tunnel" && c.Group != "atg" && c.Group != "doip" && c.Group != "ssh-s7" {
			continue
		}
		original, err := trafficfixture.ReadFile("incremental-mvp/" + c.Capture)
		require.NoError(t, err)
		answer, err := trafficfixture.ReadFile("incremental-mvp/" + c.Answer)
		require.NoError(t, err)
		corrected := correctedIncrementalCarrier(t, c, original, answer)
		if bytes.Equal(original, corrected) {
			continue
		}
		count++
		t.Run(c.Group+"/"+c.ID, func(t *testing.T) {
			pools := map[*TrafficPool]bool{}
			var reverse bytes.Buffer
			var stats TCPReassemblyStats
			require.NoError(t, ReplayPcap(bytes.NewReader(original), WithTCPReassemblyStream(64), WithOnTrafficFlowCreated(func(f *TrafficFlow) { pools[f.pool] = true }), WithOnTrafficFlowOnDataFrameReassembled(func(f *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame) {
				if conn == f.ServerConn {
					reverse.Write(frame.Payload)
				}
			}), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s })))
			// The client may still produce useful observations. The invalid SYN-ACK
			// must never supply a reverse cursor or manufacture a matched response.
			require.Empty(t, reverse.Bytes())
			require.Zero(t, stats.InvalidSegments)
			require.Zero(t, stats.CallbackPanics)
			require.EqualValues(t, c.Packets, stats.CapturedPackets)
			generationPoolsReleased(t, pools)
		})
	}
	require.Equal(t, 38, count)
}
