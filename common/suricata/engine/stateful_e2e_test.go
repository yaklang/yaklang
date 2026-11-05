package engine

import (
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/suricata/state"
)

func TestStatefulEngineFlowbitsAcrossDirections(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert tcp any any -> any any (flow:to_server,established,only_stream; content:"login"; flowbits:set,authenticated; flowbits:noalert; sid:1001;)
alert tcp any any -> any any (flow:to_client,established,only_stream; content:"welcome"; flowbits:isset,authenticated; sid:1002;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 7, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
		mu.Lock()
		alerts = append(alerts, alert)
		mu.Unlock()
	}})
	require.NoError(t, err)
	defer engine.Close()
	adapter, err := NewPcapAdapter(128, func(event InspectionEvent) { engine.Handle(event) })
	require.NoError(t, err)
	analyzer, err := pcaputil.NewPacketAnalyzer(adapter.Options()...)
	require.NoError(t, err)

	base := time.Unix(1700001000, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41000, 80, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41000, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack", nil)
	feedTCP(t, analyzer, base.Add(4*time.Millisecond), 41000, 80, clientSeq+4, serverSeq+1, "ack,psh", []byte("in"))
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41000, 80, clientSeq+1, serverSeq+1, "ack,psh", []byte("log"))
	feedTCP(t, analyzer, base.Add(5*time.Millisecond), 80, 41000, serverSeq+1, clientSeq+6, "ack,psh", []byte("welcome"))
	// Retransmission must not produce a second stream alert.
	feedTCP(t, analyzer, base.Add(6*time.Millisecond), 80, 41000, serverSeq+1, clientSeq+6, "ack,psh", []byte("welcome"))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1)
	require.Equal(t, 1002, alerts[0].Rule.Sid)
	stats := engine.Stats()
	require.Equal(t, uint64(1), stats.Alerts)
	require.GreaterOrEqual(t, stats.Suppressed, uint64(1))
	require.Equal(t, 0, stats.ActiveFlows)
	require.Equal(t, 0, stats.State.Flows)
}

func TestStatefulEngineDoesNotCommitBitOnPayloadFailure(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert tcp any any -> any any (flow:to_server,established,only_stream; content:"never"; flowbits:set,poisoned; noalert; sid:1101;)
alert tcp any any -> any any (flow:to_client,established,only_stream; content:"response"; flowbits:isset,poisoned; sid:1102;)
`)
	require.NoError(t, err)
	engine, err := NewStatefulEngine(rules, Config{State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()
	var results []Result
	adapter, err := NewPcapAdapter(128, func(event InspectionEvent) { results = append(results, engine.Handle(event)) })
	require.NoError(t, err)
	analyzer, err := pcaputil.NewPacketAnalyzer(adapter.Options()...)
	require.NoError(t, err)
	base := time.Unix(1700002000, 0)
	feedTCP(t, analyzer, base, 42000, 80, 10, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 42000, 20, 11, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 42000, 80, 11, 21, "ack", nil)
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 42000, 80, 11, 21, "ack,psh", []byte("benign"))
	feedTCP(t, analyzer, base.Add(4*time.Millisecond), 80, 42000, 21, 17, "ack,psh", []byte("response"))
	require.NoError(t, analyzer.Close())
	for _, result := range results {
		require.Empty(t, result.Alerts)
	}
}

func TestStatefulEngineThresholdUsesCaptureTime(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert tcp any any -> any any (flow:to_server,established,no_stream; content:"ping"; threshold:type limit,track by_src,count 2,seconds 10; sid:1201;)`)
	require.NoError(t, err)
	engine, err := NewStatefulEngine(rules, Config{State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()
	adapter, err := NewPcapAdapter(128, func(event InspectionEvent) { engine.Handle(event) })
	require.NoError(t, err)
	analyzer, err := pcaputil.NewPacketAnalyzer(adapter.Options()...)
	require.NoError(t, err)
	base := time.Unix(1700003000, 0)
	feedTCP(t, analyzer, base, 43000, 80, 100, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 43000, 200, 101, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 43000, 80, 101, 201, "ack", nil)
	seq := uint32(101)
	for index, offset := range []time.Duration{time.Second, 2 * time.Second, 3 * time.Second, 11 * time.Second} {
		feedTCP(t, analyzer, base.Add(offset), 43000, 80, seq+uint32(index*4), 201, "ack,psh", []byte("ping"))
	}
	require.NoError(t, analyzer.Close())
	require.Equal(t, uint64(3), engine.Stats().Alerts)
	require.Equal(t, uint64(1), engine.Stats().Suppressed)
}

func feedTCP(t *testing.T, analyzer *pcaputil.PacketAnalyzer, at time.Time, srcPort, dstPort uint16, seq, ack uint32, flags string, payload []byte) {
	t.Helper()
	raw := serializeTCP(t, srcPort, dstPort, seq, ack, flags, payload)
	require.NoError(t, analyzer.Feed(raw, gopacket.CaptureInfo{Timestamp: at, CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
}
