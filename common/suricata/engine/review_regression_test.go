package engine

import (
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/suricata/state"
)

func TestLateEventCannotReviveClosedGeneration(t *testing.T) {
	engine, err := NewStatefulEngine(nil, Config{State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()
	flow := testFlowSnapshot(1)
	engine.Handle(InspectionEvent{View: ViewFlowCreated, Flow: flow})
	engine.Handle(InspectionEvent{View: ViewFlowClosed, Flow: flow})
	result := engine.Handle(InspectionEvent{View: ViewStream, Flow: flow, Direction: 0, Payload: []byte("late")})
	require.Len(t, result.Diagnostics, 1)
	require.Equal(t, "flow-unavailable", result.Diagnostics[0].Kind)
	require.Zero(t, engine.Stats().ActiveFlows)
}

func TestHandshakeRejectsWrongACKAndSurvivesSYNRetransmit(t *testing.T) {
	flow := &flowRuntime{streamCursor: make(map[ruleDirection]int)}
	flow.observeTCP(0, tcpLayer(100, 0, true, false, false))
	flow.observeTCP(1, tcpLayer(200, 999, true, true, false))
	flow.observeTCP(0, tcpLayer(101, 201, false, true, false))
	require.False(t, flow.established)
	flow.observeTCP(1, tcpLayer(200, 101, true, true, false))
	flow.observeTCP(0, tcpLayer(101, 201, false, true, false))
	require.True(t, flow.established)
	flow.observeTCP(0, tcpLayer(100, 0, true, false, false))
	require.True(t, flow.established)
}

func TestStreamEvidenceCursorDoesNotReplayOldMultiContent(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert tcp any any -> any any (flow:established,only_stream; content:"A"; content:"B"; distance:0; sid:1701;)`)
	require.NoError(t, err)
	engine, err := NewStatefulEngine(rules, Config{State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()
	flow := testFlowSnapshot(2)
	runtime := engine.flow(flow.Key, true)
	runtime.established = true
	packet := buildTCPPacketForBenchmark(t)
	first := engine.Handle(InspectionEvent{View: ViewStream, Timestamp: time.Unix(1, 0), Flow: flow, Direction: 0, Packet: packet, Payload: []byte("AB")})
	require.Len(t, first.Alerts, 1)
	second := engine.Handle(InspectionEvent{View: ViewStream, Timestamp: time.Unix(2, 0), Flow: flow, Direction: 0, Packet: packet, Payload: []byte("A")})
	require.Empty(t, second.Alerts)
}

func TestGlobalStreamBudgetAndFlowCapacity(t *testing.T) {
	engine, err := NewStatefulEngine(nil, Config{MaxFlows: 1, MaxStreamBytes: 16, MaxTotalStreamBytes: 4, State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()
	flow1, flow2 := testFlowSnapshot(3), testFlowSnapshot(4)
	engine.Handle(InspectionEvent{View: ViewFlowCreated, Flow: flow1})
	result := engine.Handle(InspectionEvent{View: ViewStream, Flow: flow1, Direction: 0, Payload: []byte("12345")})
	require.Len(t, result.Diagnostics, 1)
	require.Equal(t, "stream-global-limit", result.Diagnostics[0].Kind)
	engine.Handle(InspectionEvent{View: ViewFlowCreated, Flow: flow2})
	result = engine.Handle(InspectionEvent{View: ViewStream, Flow: flow2, Direction: 0, Payload: []byte("x")})
	require.Len(t, result.Diagnostics, 1)
	require.Equal(t, "flow-unavailable", result.Diagnostics[0].Kind)
}

func TestAdapterJoinOrderCompactsAfterSuccessfulJoins(t *testing.T) {
	adapter, err := NewPcapAdapter(4, func(InspectionEvent) {})
	require.NoError(t, err)
	for number := uint64(1); number <= 100; number++ {
		ref := packetRef(number)
		adapter.remember(ref, packetSnapshot{data: []byte{1}})
		_, ok := adapter.take(ref)
		require.True(t, ok)
	}
	adapter.joinMu.Lock()
	defer adapter.joinMu.Unlock()
	require.LessOrEqual(t, len(adapter.joinOrder), adapter.maxJoin*2)
}

func TestDuplicateStatefulRuleIdentityRejected(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert tcp any any -> any any (sid:1801;)
alert tcp any any -> any any (sid:1801;)
`)
	require.NoError(t, err)
	_, err = NewStatefulEngine(rules, Config{State: state.DefaultConfig()})
	require.ErrorContains(t, err, "duplicate")
}

func TestConcurrentCloseAndStreamDoesNotDeadlock(t *testing.T) {
	engine, err := NewStatefulEngine(nil, Config{State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()
	flow := testFlowSnapshot(5)
	engine.Handle(InspectionEvent{View: ViewFlowCreated, Flow: flow})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			engine.Handle(InspectionEvent{View: ViewStream, Flow: flow, Direction: 0, Payload: []byte("x")})
		}
	}()
	go func() {
		defer wg.Done()
		engine.Handle(InspectionEvent{View: ViewFlowClosed, Flow: flow})
	}()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent close and stream deadlocked")
	}
}

func testFlowSnapshot(generation uint64) FlowSnapshot {
	return FlowSnapshot{Key: FlowKey{Generation: generation}}
}

func packetRef(number uint64) pcaputil.PacketReference {
	return pcaputil.PacketReference{Number: number}
}

func tcpLayer(seq, ack uint32, syn, ackFlag, rst bool) *layers.TCP {
	return &layers.TCP{Seq: seq, Ack: ack, SYN: syn, ACK: ackFlag, RST: rst}
}
