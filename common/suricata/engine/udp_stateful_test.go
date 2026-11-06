package engine

import (
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/suricata/state"
)

func TestUDPStatefulRulesOverPseudoFlow(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert udp any any -> any any (msg:"udp request"; flow:to_server; content:"hello"; threshold:type both,track by_src,count 1,seconds 60; sid:9700021; rev:1;)
alert udp any any -> any any (msg:"udp response"; flow:established,to_client; content:"world"; sid:9700022; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 12, UDPFlowTimeout: time.Minute, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700006000, 0)
	feedUDP(t, analyzer, base, "10.0.0.1", "10.0.0.2", 40000, 9999, []byte("hello"))
	feedUDP(t, analyzer, base.Add(time.Millisecond), "10.0.0.2", "10.0.0.1", 9999, 40000, []byte("world"))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 2)
	bySID := map[int]Alert{}
	for _, alert := range alerts {
		bySID[alert.Rule.Sid] = alert
	}
	requestAlert, ok := bySID[9700021]
	require.True(t, ok, "missing UDP request alert")
	require.Equal(t, 0, requestAlert.Direction)
	responseAlert, ok := bySID[9700022]
	require.True(t, ok, "missing UDP response alert")
	require.Equal(t, 1, responseAlert.Direction)
}

func TestUDPStatefulPcapReplay(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert udp any any -> any any (msg:"udp request"; flow:to_server; content:"hello"; threshold:type both,track by_src,count 1,seconds 60; sid:9700031; rev:1;)
alert udp any any -> any any (msg:"udp response"; flow:established,to_client; content:"world"; sid:9700032; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 21, UDPFlowTimeout: time.Minute, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
		mu.Lock()
		alerts = append(alerts, alert)
		mu.Unlock()
	}})
	require.NoError(t, err)
	defer engine.Close()

	base := time.Unix(1700011000, 0)
	query := serializeUDP(t, "10.0.0.1", "10.0.0.2", 40000, 9999, gopacket.Payload([]byte("hello")))
	response := serializeUDP(t, "10.0.0.2", "10.0.0.1", 9999, 40000, gopacket.Payload([]byte("world")))
	path := filepath.Join(t.TempDir(), "udp.pcap")
	file, err := os.Create(path)
	require.NoError(t, err)
	writer := pcapgo.NewWriter(file)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: base, CaptureLength: len(query), Length: len(query)}, query))
	require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: base.Add(time.Millisecond), CaptureLength: len(response), Length: len(response)}, response))
	require.NoError(t, file.Close())

	require.NoError(t, engine.OpenPcapFile(path))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 2)
	seen := map[int]bool{}
	for _, alert := range alerts {
		seen[alert.Rule.Sid] = true
	}
	require.True(t, seen[9700031])
	require.True(t, seen[9700032])
}

func TestUDPFlowbitsAcrossDirections(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert udp any any -> any any (msg:"udp set"; flow:to_server; content:"hello"; flowbits:set,udp_seen; flowbits:noalert; sid:9700025; rev:1;)
alert udp any any -> any any (msg:"udp isset"; flow:established,to_client; content:"world"; flowbits:isset,udp_seen; sid:9700026; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 18, UDPFlowTimeout: time.Minute, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700009100, 0)
	feedUDP(t, analyzer, base, "10.0.0.1", "10.0.0.2", 40000, 9999, []byte("hello"))
	feedUDP(t, analyzer, base.Add(time.Millisecond), "10.0.0.2", "10.0.0.1", 9999, 40000, []byte("world"))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1, "request must set the bit without alerting")
	require.Equal(t, 9700026, alerts[0].Rule.Sid)
	require.Equal(t, 1, alerts[0].Direction)
}

func TestUDPStatefulDNSRuleMatches(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert dns any any -> any any (msg:"dns example"; flow:to_server; dns_query; content:"example.com"; sid:9700023; rev:1;)`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 13, UDPFlowTimeout: time.Minute, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700006100, 0)
	feedDNSQuery(t, analyzer, base, "10.0.0.1", "10.0.0.2", 40001, 53, "example.com")
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1)
	require.Equal(t, 9700023, alerts[0].Rule.Sid)
	require.Equal(t, 0, alerts[0].Direction)
}

func TestUDPIdleCleanupIsolatesTupleReuse(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert udp any any -> any any (msg:"udp ttl"; flow:to_server; content:"hello"; threshold:type both,track by_src,count 1,seconds 60; sid:9700024; rev:1;)`)
	require.NoError(t, err)
	engine, err := NewStatefulEngine(rules, Config{Epoch: 14, UDPFlowTimeout: time.Second, State: state.DefaultConfig()})
	require.NoError(t, err)
	defer engine.Close()

	adapter, err := NewPcapAdapter(128, func(event InspectionEvent) { engine.Handle(event) })
	require.NoError(t, err)
	analyzer, err := pcaputil.NewPacketAnalyzer(adapter.Options()...)
	require.NoError(t, err)

	base := time.Unix(1700006200, 0)
	feedUDP(t, analyzer, base, "10.0.0.1", "10.0.0.2", 40002, 9999, []byte("hello"))
	require.NoError(t, analyzer.Close())
	require.Equal(t, 1, engine.Stats().ActiveFlows)

	engine.cleanupUDPFlows(base.Add(2 * time.Second))
	require.Zero(t, engine.Stats().ActiveFlows, "idle UDP flow must be evicted")

	adapter2, err := NewPcapAdapter(128, func(event InspectionEvent) { engine.Handle(event) })
	require.NoError(t, err)
	analyzer2, err := pcaputil.NewPacketAnalyzer(adapter2.Options()...)
	require.NoError(t, err)
	feedUDP(t, analyzer2, base.Add(3*time.Second), "10.0.0.1", "10.0.0.2", 40002, 9999, []byte("hello"))
	require.NoError(t, analyzer2.Close())
	require.EqualValues(t, 1, engine.Stats().Alerts, "new generation must alert again")
}

func feedUDP(t *testing.T, analyzer *pcaputil.PacketAnalyzer, at time.Time, src, dst string, srcPort, dstPort uint16, payload []byte) {
	t.Helper()
	raw := serializeUDP(t, src, dst, srcPort, dstPort, gopacket.Payload(payload))
	require.NoError(t, analyzer.Feed(raw, gopacket.CaptureInfo{Timestamp: at, CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
}

func feedDNSQuery(t *testing.T, analyzer *pcaputil.PacketAnalyzer, at time.Time, src, dst string, srcPort, dstPort uint16, name string) {
	t.Helper()
	dns := &layers.DNS{ID: 1, RD: true, Questions: []layers.DNSQuestion{{Name: []byte(name), Type: layers.DNSTypeA, Class: layers.DNSClassIN}}}
	raw := serializeUDP(t, src, dst, srcPort, dstPort, dns)
	require.NoError(t, analyzer.Feed(raw, gopacket.CaptureInfo{Timestamp: at, CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
}

func serializeUDP(t testing.TB, src, dst string, srcPort, dstPort uint16, payload gopacket.SerializableLayer) []byte {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       []byte{0, 1, 2, 3, 4, 5},
		DstMAC:       []byte{6, 7, 8, 9, 10, 11},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    net.ParseIP(src).To4(),
		DstIP:    net.ParseIP(dst).To4(),
	}
	udp := &layers.UDP{SrcPort: layers.UDPPort(srcPort), DstPort: layers.UDPPort(dstPort)}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, payload))
	return append([]byte(nil), buffer.Bytes()...)
}
