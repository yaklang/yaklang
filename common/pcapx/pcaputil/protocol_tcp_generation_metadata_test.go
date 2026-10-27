package pcaputil

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProtocolTCPGenerationMetadataSurvivesDistinctSessionNumbers(t *testing.T) {
	request := "GET /native-generation HTTP/1.1\r\nHost: example.test\r\n\r\n"
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("w%d/deferred%t", workers, deferred), func(t *testing.T) {
				steps := []tcpStep{
					{seq: 99, syn: true}, {seq: 499, ack: 100, syn: true, synack: true, reverse: true},
					{seq: 999, syn: true}, {seq: 1999, ack: 1000, syn: true, synack: true, reverse: true},
					{seq: 1000, ack: 2000, data: request},
				}
				wire := binTestPcap(t, steps, 80, false, true)
				var mu sync.Mutex
				var events []*ProtocolEvent
				owners := map[uint64]uint64{}
				pools := map[*TrafficPool]bool{}
				err := ReplayPcap(bytes.NewReader(wire), WithTCPReassemblyStream(64), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred),
					WithOnTrafficFlowCreated(func(f *TrafficFlow) { mu.Lock(); defer mu.Unlock(); pools[f.pool] = true }),
					WithOnTrafficFlowPacket(func(f *TrafficFlow, _ *TrafficConnection, refs []PacketReference, _ time.Time) {
						mu.Lock()
						defer mu.Unlock()
						for _, ref := range refs {
							owners[ref.Number] = f.Index
						}
					}), WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); defer mu.Unlock(); events = append(events, e) }))
				require.NoError(t, err)
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, "http", e.Protocol)
				require.NotZero(t, e.TCPFlowID)
				require.NotEqual(t, e.FlowID, e.TCPFlowID, "the old SYN-only generation allocated no protocol session")
				for _, ref := range e.SourceBytes.PacketRefs {
					require.Equal(t, owners[ref.Number], e.TCPFlowID)
				}
				require.Equal(t, []any{e.TCPFlowID}, e.DisplayFieldValues().FieldMap()["tcp.flow.id"])
				require.Contains(t, e.DumpLine(), fmt.Sprintf("tcp_flow=%d", e.TCPFlowID))
				history, err := NewProtocolInspector(8, 1<<20)
				require.NoError(t, err)
				history.OnEvent(e)
				require.Empty(t, history.TCPFlowRows(e.Domain, e.FlowID), "protocol session numbers cannot select another TCP generation")
				require.Empty(t, history.TCPFlowRows(e.Domain, 0))
				foreign := e.Domain
				foreign.Interface++
				require.Empty(t, history.TCPFlowRows(foreign, e.TCPFlowID))
				rows := history.TCPFlowRows(e.Domain, e.TCPFlowID)
				require.Len(t, rows, 1)
				require.Equal(t, e.FlowID, rows[0].FlowID)
				require.Equal(t, e.TCPFlowID, rows[0].TCPFlowID)
				require.Nil(t, rows[0].Raw)
				rows[0].SourceBytes.PacketRefs[0].Number = 999
				require.Equal(t, e.SourceBytes.PacketRefs, history.TCPFlowRows(e.Domain, e.TCPFlowID)[0].SourceBytes.PacketRefs)
				generationPoolsReleased(t, pools)
			})
		}
	}
}

func TestProtocolWithoutCapturedTCPHasNoInventedGeneration(t *testing.T) {
	e := &ProtocolEvent{ID: 1, FlowID: 7, Transport: "tcp", Protocol: "http", Status: "incomplete"}
	require.NotContains(t, e.DisplayFieldValues().FieldMap(), "tcp.flow.id")
	history, err := NewProtocolInspector(8, 4096)
	require.NoError(t, err)
	history.OnEvent(e)
	require.Len(t, history.Rows("", 7), 1)
	require.Empty(t, history.TCPFlowRows(CaptureDomain{}, 7))
}
