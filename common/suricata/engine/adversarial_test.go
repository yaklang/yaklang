package engine

import (
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/suricata/state"
)

func TestHalfOpenFlowDoesNotMatchEstablished(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert tcp any any -> any any (flow:to_server,established,only_stream; content:"attack"; sid:1301;)`)
	require.NoError(t, err)
	engine, err := NewStatefulEngine(rules, Config{State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()
	adapter, err := NewPcapAdapter(64, func(event InspectionEvent) { engine.Handle(event) })
	require.NoError(t, err)
	analyzer, err := pcaputil.NewPacketAnalyzer(adapter.Options()...)
	require.NoError(t, err)
	base := time.Unix(1700005000, 0)
	feedTCP(t, analyzer, base, 45000, 80, 10, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 45000, 80, 11, 0, "ack,psh", []byte("attack"))
	require.NoError(t, analyzer.Close())
	require.Zero(t, engine.Stats().Alerts)
}

func TestTupleReuseDoesNotInheritFlowbits(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert tcp any any -> any any (flow:to_server,established,only_stream; content:"setbit"; flowbits:set,old; noalert; sid:1401;)
alert tcp any any -> any any (flow:to_client,established,only_stream; content:"checkbit"; flowbits:isset,old; sid:1402;)
`)
	require.NoError(t, err)
	engine, err := NewStatefulEngine(rules, Config{State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()
	adapter, err := NewPcapAdapter(128, func(event InspectionEvent) { engine.Handle(event) })
	require.NoError(t, err)
	analyzer, err := pcaputil.NewPacketAnalyzer(adapter.Options()...)
	require.NoError(t, err)
	base := time.Unix(1700006000, 0)
	// First generation sets the bit and closes.
	feedTCP(t, analyzer, base, 46000, 80, 100, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 46000, 200, 101, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 46000, 80, 101, 201, "ack", nil)
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 46000, 80, 101, 201, "ack,psh", []byte("setbit"))
	feedTCP(t, analyzer, base.Add(4*time.Millisecond), 46000, 80, 107, 201, "rst,ack", nil)
	// Reuse exactly the same tuple; response must not observe old state.
	feedTCP(t, analyzer, base.Add(time.Second), 46000, 80, 1000, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Second+time.Millisecond), 80, 46000, 2000, 1001, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(time.Second+2*time.Millisecond), 46000, 80, 1001, 2001, "ack", nil)
	feedTCP(t, analyzer, base.Add(time.Second+3*time.Millisecond), 80, 46000, 2001, 1001, "ack,psh", []byte("checkbit"))
	require.NoError(t, analyzer.Close())
	require.Zero(t, engine.Stats().Alerts)
}

func TestStreamResourceLimitFailsClosed(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert tcp any any -> any any (flow:to_server,established,only_stream; content:"late"; flowbits:set,poison; sid:1501;)`)
	require.NoError(t, err)
	var results []Result
	engine, err := NewStatefulEngine(rules, Config{MaxStreamBytes: 8, State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()
	flow := FlowSnapshot{Key: FlowKey{Generation: 1}, First: Endpoint{}, Second: Endpoint{}}
	results = append(results, engine.Handle(InspectionEvent{View: ViewFlowCreated, Flow: flow}))
	results = append(results, engine.Handle(InspectionEvent{View: ViewStream, Flow: flow, Direction: 0, Payload: []byte("0123456789late")}))
	require.NotEmpty(t, results[1].Diagnostics)
	require.Equal(t, "stream-limit", results[1].Diagnostics[0].Kind)
	require.Zero(t, engine.Stats().Alerts)
}

func TestPacketReferenceJoinCapacityIsBounded(t *testing.T) {
	adapter, err := NewPcapAdapter(2, func(InspectionEvent) {})
	require.NoError(t, err)
	for number := uint64(1); number <= 4; number++ {
		adapter.remember(pcaputil.PacketReference{Number: number}, packetSnapshot{data: []byte{byte(number)}})
	}
	require.Equal(t, uint64(2), adapter.Stats().JoinEvictions)
	adapter.joinMu.Lock()
	require.Len(t, adapter.join, 2)
	adapter.joinMu.Unlock()
}

func BenchmarkStatefulEngineStream(b *testing.B) {
	rules, err := rule.ParseStrictRule(`alert tcp any any -> any any (flow:established,only_stream; content:"benchmark"; sid:1601;)`)
	require.NoError(b, err)
	engine, err := NewStatefulEngine(rules, Config{State: state.DefaultConfig()})
	require.NoError(b, err)
	defer engine.Close()
	packet := buildTCPPacketForBenchmark(b)
	flow := FlowSnapshot{Key: FlowKey{Generation: 1}}
	engine.flow(flow.Key, true).established = true
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.Handle(InspectionEvent{View: ViewStream, Timestamp: time.Unix(1, int64(i)), Flow: flow, Direction: 0, Packet: packet, Payload: []byte("benchmark")})
	}
}

func buildTCPPacketForBenchmark(tb testing.TB) gopacket.Packet {
	tb.Helper()
	raw := serializeTCP(tb, 47000, 80, 1, 1, "ack,psh", []byte("benchmark"))
	return gopacket.NewPacket(raw, layers.LayerTypeEthernet, gopacket.Default)
}
