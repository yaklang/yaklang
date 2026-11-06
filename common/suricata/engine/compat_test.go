package engine

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"strings"
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

func TestTCPPacketAndTargetMetadata(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert tcp-pkt any any -> any any (msg:"tcp packet"; flow:established,to_server; content:"hello"; target:src_ip; sid:9700201; rev:1;)`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 22, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700012000, 0)
	feedTCP(t, analyzer, base, 41001, 21, 100, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 21, 41001, 900, 101, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41001, 21, 101, 901, "ack", nil)
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41001, 21, 101, 901, "ack,psh", []byte("hello"))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1)
	require.Equal(t, 9700201, alerts[0].Rule.Sid)
	require.Equal(t, ViewNormalizedPacket, alerts[0].View)
	require.Equal(t, "src_ip", alerts[0].Target)
	require.Equal(t, 0, alerts[0].Direction)
}

func TestTCPStreamSizeRules(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert tcp any any -> any any (msg:"client size"; flow:established,to_server; stream_size:client,=,5; content:"hello"; sid:9700301; rev:1;)
alert tcp any any -> any any (msg:"server size"; flow:established,to_client; stream_size:server,=,5; content:"world"; sid:9700302; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 25, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700012300, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41004, 80, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41004, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41004, 80, clientSeq+1, serverSeq+1, "ack", nil)
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41004, 80, clientSeq+1, serverSeq+1, "ack,psh", []byte("hello"))
	feedTCP(t, analyzer, base.Add(4*time.Millisecond), 80, 41004, serverSeq+1, clientSeq+6, "ack,psh", []byte("world"))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 2)
	seen := map[int]bool{}
	for _, alert := range alerts {
		seen[alert.Rule.Sid] = true
	}
	require.True(t, seen[9700301])
	require.True(t, seen[9700302])
}

func TestTCPPacketStreamSize(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert tcp-pkt any any -> any any (flow:established,to_server; stream_size:client,=,5; content:"hello"; sid:9700303; rev:1;)`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 26, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700012350, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41006, 80, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41006, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41006, 80, clientSeq+1, serverSeq+1, "ack", nil)
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41006, 80, clientSeq+1, serverSeq+1, "ack,psh", []byte("hello"))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1)
	require.Equal(t, 9700303, alerts[0].Rule.Sid)
}

func TestHTTPFileDataRequestAndResponse(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert http any any -> any any (msg:"request file"; flow:established,to_server; file_data; content:"upload-marker"; fast_pattern; sid:9700401; rev:1;)
alert http any any -> any any (msg:"response file"; flow:established,to_client; file_data; content:"malicious-body"; sid:9700402; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 27, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700012400, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41005, 80, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41005, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41005, 80, clientSeq+1, serverSeq+1, "ack", nil)
	requestBody := []byte("upload-marker")
	request := append([]byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: 13\r\n\r\n"), requestBody...)
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41005, 80, clientSeq+1, serverSeq+1, "ack,psh", request)
	responseBody := []byte("malicious-body")
	response := append([]byte("HTTP/1.1 200 OK\r\nContent-Length: 14\r\n\r\n"), responseBody...)
	feedTCP(t, analyzer, base.Add(4*time.Millisecond), 80, 41005, serverSeq+1, clientSeq+1+uint32(len(request)), "ack,psh", response)
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 2)
	seen := map[int]bool{}
	for _, alert := range alerts {
		seen[alert.Rule.Sid] = true
		require.Equal(t, ViewStream, alert.View)
	}
	require.True(t, seen[9700401])
	require.True(t, seen[9700402])
}

func TestHTTPXBitsAcrossFlowsAndExpiry(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert http any any -> any any (msg:"xbit set"; flow:established,to_server; xbits:set,ET.test,track ip_src,expire 1; noalert; http.uri; content:"/set"; sid:9700501; rev:1;)
alert http any any -> any any (msg:"xbit check"; flow:established,to_server; xbits:isset,ET.test,track ip_src; http.uri; content:"/check"; sid:9700502; rev:1;)
alert http any any -> any any (msg:"xbit expired"; flow:established,to_server; xbits:isnotset,ET.test,track ip_src; http.uri; content:"/after"; sid:9700503; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 28, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700012500, 0)
	feed := func(port uint16, at time.Time, path string) {
		clientSeq, serverSeq := uint32(100), uint32(900)
		feedTCP(t, analyzer, at, port, 80, clientSeq, 0, "syn", nil)
		feedTCP(t, analyzer, at.Add(time.Millisecond), 80, port, serverSeq, clientSeq+1, "syn,ack", nil)
		feedTCP(t, analyzer, at.Add(2*time.Millisecond), port, 80, clientSeq+1, serverSeq+1, "ack", nil)
		request := []byte("GET " + path + " HTTP/1.1\r\nHost: example.com\r\n\r\n")
		feedTCP(t, analyzer, at.Add(3*time.Millisecond), port, 80, clientSeq+1, serverSeq+1, "ack,psh", request)
	}
	feed(41007, base, "/set")
	feed(41008, base.Add(100*time.Millisecond), "/check")
	feed(41009, base.Add(2*time.Second), "/after")
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 2)
	seen := map[int]bool{}
	for _, alert := range alerts {
		seen[alert.Rule.Sid] = true
	}
	require.True(t, seen[9700502], "isset must observe the xbit from another flow")
	require.True(t, seen[9700503], "isnotset must observe the expired xbit")
	require.False(t, seen[9700501])
}

func TestIPThresholdWithoutTransportFlow(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert ip any any -> any any (msg:"ip threshold"; threshold:type limit,track by_src,count 1,seconds 60; sid:9700601; rev:1;)`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 29, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700012600, 0)
	raw := serializeICMP(t, "192.0.2.1", "192.0.2.2", []byte("ping"))
	require.NoError(t, analyzer.Feed(raw, gopacket.CaptureInfo{Timestamp: base, CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
	require.NoError(t, analyzer.Feed(raw, gopacket.CaptureInfo{Timestamp: base.Add(time.Second), CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1)
	require.Equal(t, 9700601, alerts[0].Rule.Sid)
	require.EqualValues(t, 1, engine.Stats().Suppressed)
}

func serializeICMP(t testing.TB, src, dst string, payload []byte) []byte {
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
		Protocol: layers.IPProtocolICMPv4,
		SrcIP:    net.ParseIP(src).To4(),
		DstIP:    net.ParseIP(dst).To4(),
	}
	icmp := &layers.ICMPv4{TypeCode: layers.CreateICMPv4TypeCode(8, 0)}
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, icmp, gopacket.Payload(payload)))
	return append([]byte(nil), buffer.Bytes()...)
}

func TestICMPThresholdWithoutTransportFlow(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert icmp any any -> any any (msg:"icmp threshold"; threshold:type limit,track by_src,count 1,seconds 60; sid:9700604; rev:1;)`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 36, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700013300, 0)
	raw := serializeICMP(t, "192.0.2.1", "192.0.2.2", []byte("ping"))
	require.NoError(t, analyzer.Feed(raw, gopacket.CaptureInfo{Timestamp: base, CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
	require.NoError(t, analyzer.Feed(raw, gopacket.CaptureInfo{Timestamp: base.Add(time.Second), CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1)
	require.Equal(t, 9700604, alerts[0].Rule.Sid)
	require.EqualValues(t, 1, engine.Stats().Suppressed)
}

func TestICMPv6PayloadAndThreshold(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert icmpv6 any any -> any any (msg:"icmpv6"; content:"|19|"; offset:12; depth:1; dsize:>10; threshold:type limit,track by_src,count 1,seconds 60; sid:9700605; rev:1;)`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 37, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	payload := make([]byte, 20)
	payload[12] = 0x19
	raw := serializeICMPv6(t, "2001:db8::1", "2001:db8::2", payload)
	base := time.Unix(1700013400, 0)
	require.NoError(t, analyzer.Feed(raw, gopacket.CaptureInfo{Timestamp: base, CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
	require.NoError(t, analyzer.Feed(raw, gopacket.CaptureInfo{Timestamp: base.Add(time.Second), CaptureLength: len(raw), Length: len(raw)}, layers.LinkTypeEthernet))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1)
	require.Equal(t, 9700605, alerts[0].Rule.Sid)
	require.EqualValues(t, 1, engine.Stats().Suppressed)
}

func serializeICMPv6(t testing.TB, src, dst string, payload []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       []byte{0, 1, 2, 3, 4, 5},
		DstMAC:       []byte{6, 7, 8, 9, 10, 11},
		EthernetType: layers.EthernetTypeIPv6,
	}
	ip := &layers.IPv6{
		Version:    6,
		NextHeader: layers.IPProtocolICMPv6,
		HopLimit:   64,
		SrcIP:      net.ParseIP(src),
		DstIP:      net.ParseIP(dst),
	}
	icmp := &layers.ICMPv6{TypeCode: layers.CreateICMPv6TypeCode(134, 0)}
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true}, eth, ip, icmp, gopacket.Payload(payload)))
	return append([]byte(nil), buffer.Bytes()...)
}

func TestIPRuleFlowbitsOnTCPFlow(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert ip [192.0.2.1] any -> any any (msg:"evil ip"; flowbits:set,ET.Evil; noalert; sid:9700602; rev:1;)
alert tcp any any -> any any (msg:"evil tcp"; flowbits:isset,ET.Evil; content:"hello"; sid:9700603; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 30, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700012700, 0)
	feedTCP(t, analyzer, base, 41010, 80, 100, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41010, 900, 101, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41010, 80, 101, 901, "ack", nil)
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41010, 80, 101, 901, "ack,psh", []byte("hello"))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1)
	require.Equal(t, 9700603, alerts[0].Rule.Sid)
}

func TestTCPApplicationProtocolRules(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert smtp any any -> any any (msg:"smtp"; flow:established,to_server; content:"SMTP-MARKER"; sid:9700701; rev:1;)
alert ftp any any -> any any (msg:"ftp"; flow:established,to_server; content:"FTP-MARKER"; sid:9700702; rev:1;)
alert smb any any -> any any (msg:"smb"; flow:established,to_server; content:"SMB-MARKER"; sid:9700703; rev:1;)
alert ssh any any -> any any (msg:"ssh"; flow:established,to_server; ssh_proto; content:"PUTTY"; sid:9700704; rev:1;)
alert tcp any any -> any any (msg:"ftp bounce"; flow:established,to_server; content:"PORT"; nocase; ftpbounce; sid:9700705; rev:1;)
alert tcp any any -> any any (msg:"asn1"; flow:established,to_server; content:"AA"; asn1:relative_offset 0, bitstring_overflow; sid:9700706; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 31, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700012800, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41012, 25, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 25, 41012, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41012, 25, clientSeq+1, serverSeq+1, "ack", nil)
	payload := []byte("SMTP-MARKER FTP-MARKER SMB-MARKER\nSSH-2.0-PUTTY\r\nPORT 10,0,0,1,4,2\r\nAA\x03\x01\xff")
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41012, 25, clientSeq+1, serverSeq+1, "ack,psh", payload)
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 6)
	seen := map[int]bool{}
	for _, alert := range alerts {
		seen[alert.Rule.Sid] = true
	}
	require.True(t, seen[9700701])
	require.True(t, seen[9700702])
	require.True(t, seen[9700703])
	require.True(t, seen[9700704])
	require.True(t, seen[9700705])
	require.True(t, seen[9700706])
}

func TestTCPRuleWithHTTPBuffers(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert tcp any any -> any any (msg:"tcp http"; flow:established,to_server; http.method; content:"POST"; http.uri; content:"/upload"; sid:9700801; rev:1;)
alert tcp any any -> any any (msg:"tcp file data"; flow:established,to_server; http.method; content:"POST"; file_data; content:"marker"; sid:9700802; rev:1;)
alert http any any -> any any (msg:"bad transfer encoding"; flow:established,to_server; app-layer-event:http.invalid_transfer_encoding_value_in_request; http.method; content:"POST"; sid:9700803; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 32, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700012900, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41013, 80, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41013, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41013, 80, clientSeq+1, serverSeq+1, "ack", nil)
	request := []byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: bogus\r\nContent-Length: 6\r\n\r\nmarker")
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41013, 80, clientSeq+1, serverSeq+1, "ack,psh", request)
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 3)
	seen := map[int]bool{}
	for _, alert := range alerts {
		seen[alert.Rule.Sid] = true
		require.Equal(t, ViewStream, alert.View)
	}
	require.True(t, seen[9700801])
	require.True(t, seen[9700802])
	require.True(t, seen[9700803])
}

func TestHTTPFlowIntPredicate(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert http any any -> any any (msg:"flowint set"; flow:established,to_server; http.uri; content:"/inc"; flowint:counter,+,1; noalert; sid:9700901; rev:1;)
alert http any any -> any any (msg:"flowint check"; flow:established,to_server; http.uri; content:"/check"; flowint:counter,==,1; sid:9700902; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 33, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700013000, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41014, 80, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41014, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41014, 80, clientSeq+1, serverSeq+1, "ack", nil)
	first := []byte("GET /inc HTTP/1.1\r\nHost: example.com\r\n\r\n")
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41014, 80, clientSeq+1, serverSeq+1, "ack,psh", first)
	second := []byte("GET /check HTTP/1.1\r\nHost: example.com\r\n\r\n")
	feedTCP(t, analyzer, base.Add(4*time.Millisecond), 41014, 80, clientSeq+1+uint32(len(first)), serverSeq+1, "ack,psh", second)
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 1)
	require.Equal(t, 9700902, alerts[0].Rule.Sid)
}

func TestSessionTagPackets(t *testing.T) {
	rules, err := rule.ParseStrictRule(`alert tcp any any -> any any (msg:"tagged"; flow:established,to_server; content:"TRIGGER"; tag:session,2,packets; sid:9701001; rev:1;)`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 34, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700013100, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41015, 80, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 80, 41015, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41015, 80, clientSeq+1, serverSeq+1, "ack", nil)
	trigger := []byte("TRIGGER")
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41015, 80, clientSeq+1, serverSeq+1, "ack,psh", trigger)
	seq := clientSeq + 1 + uint32(len(trigger))
	feedTCP(t, analyzer, base.Add(4*time.Millisecond), 41015, 80, seq, serverSeq+1, "ack,psh", []byte("one"))
	feedTCP(t, analyzer, base.Add(5*time.Millisecond), 41015, 80, seq+3, serverSeq+1, "ack,psh", []byte("two"))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 3)
	require.False(t, alerts[0].Tagged)
	require.Equal(t, ViewStream, alerts[0].View)
	require.True(t, alerts[1].Tagged)
	require.True(t, alerts[2].Tagged)
	require.Equal(t, ViewNormalizedPacket, alerts[1].View)
	require.Equal(t, ViewNormalizedPacket, alerts[2].View)
}

func TestTCPAndSMTPFileData(t *testing.T) {
	rules, err := rule.ParseStrictRule(`
alert smtp any any -> any any (msg:"smtp data"; flow:established,to_server; file_data; content:"legal"; sid:9701101; rev:1;)
alert tcp any any -> any any (msg:"tcp payload"; flow:established,to_server; file_data; content:"legal"; sid:9701102; rev:1;)
`)
	require.NoError(t, err)
	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 35, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700013200, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41016, 25, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 25, 41016, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41016, 25, clientSeq+1, serverSeq+1, "ack", nil)
	mail := []byte("EHLO example.com\r\nMAIL FROM:<a@example.com>\r\nDATA\r\nlegal\r\n.\r\n")
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41016, 25, clientSeq+1, serverSeq+1, "ack,psh", mail)
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 2)
	seen := map[int]bool{}
	for _, alert := range alerts {
		seen[alert.Rule.Sid] = true
	}
	require.True(t, seen[9701101])
	require.True(t, seen[9701102])
}

func TestTLSFlowSNIAndJA3Alias(t *testing.T) {
	const (
		sni = "example.com"
		// Synthetic ClientHello with TLS 1.2, one cipher suite (0x1301), and
		// only the server_name extension.
		ja3String = "771,4865,0,,"
	)
	ja3Hash := fmt.Sprintf("%x", md5.Sum([]byte(ja3String)))
	rules, err := rule.ParseStrictRule(fmt.Sprintf(`
alert tls any any -> any any (msg:"tls sni"; flow:established,to_server; tls_sni; content:"%s"; endswith; target:dest_ip; sid:9700202; rev:1;)
alert tls any any -> any any (msg:"tls ja3"; flow:established,to_server; ja3_hash; content:"%s"; sid:9700203; rev:1;)
`, sni, ja3Hash))
	require.NoError(t, err)
	require.Len(t, rules, 2)

	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 23, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700012100, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41002, 443, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 443, 41002, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41002, 443, clientSeq+1, serverSeq+1, "ack", nil)
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 41002, 443, clientSeq+1, serverSeq+1, "ack,psh", buildClientHello(sni))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 2)
	bySID := map[int]Alert{}
	for _, alert := range alerts {
		bySID[alert.Rule.Sid] = alert
		require.Equal(t, ViewStream, alert.View)
		require.Equal(t, 0, alert.Direction)
	}
	sniAlert, ok := bySID[9700202]
	require.True(t, ok, "missing tls.sni alert")
	require.Equal(t, "dest_ip", sniAlert.Target)
	_, ok = bySID[9700203]
	require.True(t, ok, "missing ja3_hash alias alert")
}

func TestTLSCertificateFieldExtraction(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial := big.NewInt(0x80)
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Country: []string{"US"}, Organization: []string{"Yaklang"}, CommonName: "test.example"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	fingerprint := sha1.Sum(der)
	fingerprintText := colonHex(fingerprint[:])
	ja3sHash := fmt.Sprintf("%x", md5.Sum([]byte("771,4865,")))

	rules, err := rule.ParseStrictRule(fmt.Sprintf(`
alert tls any any -> any any (msg:"cert subject"; flow:established,to_client; tls.cert_subject; content:"CN=test.example"; endswith; sid:9700211; rev:1;)
alert tls any any -> any any (msg:"cert issuer"; flow:established,to_client; tls.cert_issuer; content:"O=Yaklang"; sid:9700212; rev:1;)
alert tls any any -> any any (msg:"cert serial"; flow:established,to_client; tls.cert_serial; content:"00:80"; sid:9700213; rev:1;)
alert tls any any -> any any (msg:"cert fingerprint"; flow:established,to_client; tls.cert_fingerprint; content:"%s"; sid:9700214; rev:1;)
alert tls any any -> any any (msg:"cert raw"; flow:established,to_client; tls.certs; content:"test.example"; sid:9700215; rev:1;)
alert tls any any -> any any (msg:"tls version"; flow:established,to_client; tls.version:1.2; tls.cert_subject; content:"CN=test.example"; endswith; sid:9700216; rev:1;)
alert tls any any -> any any (msg:"ja3s"; flow:established,to_client; ja3s.hash; content:"%s"; sid:9700217; rev:1;)
alert tcp any any -> any any (msg:"tcp tls cert"; flow:established,to_client; tls.cert_subject; content:"CN=test.example"; endswith; sid:9700218; rev:1;)
`, fingerprintText, ja3sHash))
	require.NoError(t, err)
	require.Len(t, rules, 8)

	var mu sync.Mutex
	var alerts []Alert
	engine, err := NewStatefulEngine(rules, Config{Epoch: 24, State: state.DefaultConfig(), OnAlert: func(alert Alert) {
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

	base := time.Unix(1700012200, 0)
	clientSeq, serverSeq := uint32(100), uint32(900)
	feedTCP(t, analyzer, base, 41003, 443, clientSeq, 0, "syn", nil)
	feedTCP(t, analyzer, base.Add(time.Millisecond), 443, 41003, serverSeq, clientSeq+1, "syn,ack", nil)
	feedTCP(t, analyzer, base.Add(2*time.Millisecond), 41003, 443, clientSeq+1, serverSeq+1, "ack", nil)
	serverHello := buildServerHello()
	certificate := buildCertificateRecord(der)
	serverPayload := append(serverHello, certificate...)
	feedTCP(t, analyzer, base.Add(3*time.Millisecond), 443, 41003, serverSeq+1, clientSeq+1, "ack,psh", serverPayload)
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, alerts, 8)
	seen := map[int]bool{}
	for _, alert := range alerts {
		seen[alert.Rule.Sid] = true
		require.Equal(t, ViewStream, alert.View)
		require.Equal(t, 1, alert.Direction)
	}
	for _, sid := range []int{9700211, 9700212, 9700213, 9700214, 9700215, 9700216, 9700217, 9700218} {
		require.Truef(t, seen[sid], "missing alert for sid %d", sid)
	}
}

func buildServerHello() []byte {
	body := []byte{0x03, 0x03}
	body = append(body, make([]byte, 32)...)
	body = append(body, 0)          // session ID length
	body = append(body, 0x13, 0x01) // cipher suite
	body = append(body, 0)          // compression
	body = append(body, 0, 0)       // extensions length
	handshake := []byte{2, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	handshake = append(handshake, body...)
	record := []byte{0x16, 0x03, 0x03, byte(len(handshake) >> 8), byte(len(handshake))}
	return append(record, handshake...)
}

func buildCertificateRecord(der []byte) []byte {
	listLength := 3 + len(der)
	body := []byte{byte(listLength >> 16), byte(listLength >> 8), byte(listLength)}
	body = append(body, byte(len(der)>>16), byte(len(der)>>8), byte(len(der)))
	body = append(body, der...)
	handshake := []byte{11, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	handshake = append(handshake, body...)
	record := []byte{0x16, 0x03, 0x03, byte(len(handshake) >> 8), byte(len(handshake))}
	return append(record, handshake...)
}

func colonHex(data []byte) string {
	encoded := hex.EncodeToString(data)
	var parts []string
	for index := 0; index+2 <= len(encoded); index += 2 {
		parts = append(parts, encoded[index:index+2])
	}
	return strings.Join(parts, ":")
}

func buildClientHello(host string) []byte {
	hostBytes := []byte(host)
	nameListLength := 1 + 2 + len(hostBytes)
	extensionLength := 5 + len(hostBytes)
	extension := make([]byte, 0, 4+extensionLength)
	extension = append(extension,
		0, 0, // server_name extension type
		byte(extensionLength>>8), byte(extensionLength),
		byte(nameListLength>>8), byte(nameListLength),
		0, // host_name name type
		byte(len(hostBytes)>>8), byte(len(hostBytes)),
	)
	extension = append(extension, hostBytes...)

	body := make([]byte, 0, 41+len(extension))
	body = append(body, 0x03, 0x03) // TLS 1.2
	body = append(body, make([]byte, 32)...)
	body = append(body, 0) // empty session ID
	body = append(body, 0, 2, 0x13, 0x01)
	body = append(body, 1, 0) // null compression
	body = append(body, byte(len(extension)>>8), byte(len(extension)))
	body = append(body, extension...)

	handshake := []byte{
		1,
		byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body)),
	}
	handshake = append(handshake, body...)
	record := []byte{0x16, 0x03, 0x03, byte(len(handshake) >> 8), byte(len(handshake))}
	record = append(record, handshake...)
	return record
}
