package engine

import (
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
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

func buildPrefilterTestTCPPacket(t *testing.T, srcIP, dstIP string, srcPort, dstPort uint16, seq, ack uint32, syn, ackFlag, psh bool, payload []byte) gopacket.Packet {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x01},
		DstMAC:       net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x02},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    net.ParseIP(srcIP).To4(),
		DstIP:    net.ParseIP(dstIP).To4(),
	}
	tcp := &layers.TCP{
		SrcPort: layers.TCPPort(srcPort),
		DstPort: layers.TCPPort(dstPort),
		Seq:     seq,
		Ack:     ack,
		SYN:     syn,
		ACK:     ackFlag,
		PSH:     psh,
		Window:  65535,
	}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	require.NoError(t, gopacket.SerializeLayers(buf, opts, eth, ip, tcp, gopacket.Payload(payload)))
	return gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}

func buildPrefilterTestDNSPacket(t *testing.T, qname string) gopacket.Packet {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x01},
		DstMAC:       net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x02},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    net.ParseIP("10.0.0.25").To4(),
		DstIP:    net.ParseIP("8.8.8.8").To4(),
	}
	udp := &layers.UDP{
		SrcPort: 54321,
		DstPort: 53,
	}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	dns := &layers.DNS{
		ID:      0x1234,
		QR:      false,
		OpCode:  layers.DNSOpCodeQuery,
		QDCount: 1,
		Questions: []layers.DNSQuestion{
			{Name: []byte(qname), Type: layers.DNSTypeA, Class: layers.DNSClassIN},
		},
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	require.NoError(t, gopacket.SerializeLayers(buf, opts, eth, ip, udp, dns))
	return gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}

func buildPrefilterTestTLSServerHello(downgrade bool) []byte {
	var body []byte
	body = append(body, 0x03, 0x03)
	random32 := make([]byte, 32)
	if downgrade {
		copy(random32[24:32], []byte{'D', 'O', 'W', 'N', 'G', 'R', 'D', 0x00})
	}
	body = append(body, random32...)
	body = append(body, 0x00, 0xc0, 0x2f, 0x00)
	hs := make([]byte, 4+len(body))
	hs[0] = 2
	hs[1] = byte(len(body) >> 16)
	hs[2] = byte(len(body) >> 8)
	hs[3] = byte(len(body))
	copy(hs[4:], body)
	rec := make([]byte, 5+len(hs))
	rec[0] = 22
	rec[1] = 0x03
	rec[2] = 0x03
	binary.BigEndian.PutUint16(rec[3:5], uint16(len(hs)))
	copy(rec[5:], hs)
	return rec
}

func TestStatefulEnginePrefilterEquivalence(t *testing.T) {
	bodyHash := sha1.Sum([]byte("payload=secret"))
	hexHash := "|"
	for i, b := range bodyHash {
		if i > 0 {
			hexHash += " "
		}
		hexHash += fmt.Sprintf("%02x", b)
	}
	hexHash += "|"

	ruleText := fmt.Sprintf(`
alert http any any -> $HOME_NET any (msg:"http lower uri"; flow:established,to_server; http.uri; to_lowercase; content:"/admin/"; flowbits:set,seen.admin; noalert; sid:9800001; rev:1;)
alert http any any -> $HOME_NET any (msg:"http flowbit follow"; flow:established,to_server; flowbits:isset,seen.admin; http.request_header; header_lowercase; content:"x-exploit: "; startswith; sid:9800002; rev:1;)
alert http1 any any -> $HOME_NET any (msg:"http1 urldecode"; flow:established,to_server; http.uri.raw; url_decode; to_lowercase; content:"/admin/panel"; sid:9800003; rev:1;)
alert http any any -> $HOME_NET any (msg:"http sha1 body"; flow:established,to_server; http.request_body; to_sha1; content:"%s"; sid:9800004; rev:1;)
alert http any any -> $HOME_NET any (msg:"http strip ws"; flow:established,to_server; http.user_agent; strip_whitespace; content:"BadAgent99"; sid:9800005; rev:1;)
alert http any any -> $HOME_NET any (msg:"http pcre only"; flow:established,to_server; http.uri; pcre:"/\/admin\/%%50[a-z]+/i"; sid:9800006; rev:1;)
alert http any any -> $HOME_NET any (msg:"http neg only"; flow:established,to_server; http.method; content:!"DELETE"; urilen:>5; sid:9800007; rev:1;)
alert http $HOME_NET any -> any any (msg:"http resp hdr"; flow:established,to_client; http.response_header; to_lowercase; content:"x-debug: leaked"; bsize:15; sid:9800008; rev:1;)
alert rdp any any -> $HOME_NET 3389 (msg:"rdp banner"; flow:established,to_server; content:"Cookie: mstshash=admin"; sid:9800009; rev:1;)
alert tcp any any -> $HOME_NET any (msg:"tcp binary only"; flow:established,to_server; content:"|de ad be ef|"; sid:9800010; rev:1;)
alert tls $HOME_NET any -> any any (msg:"tls downgrade"; flow:established,to_client; tls.random; content:"DOWNGRD|00|"; offset:24; depth:8; sid:9800011; rev:1;)
alert dns any any -> any 53 (msg:"dns dotprefix lower"; dns.query; dotprefix; to_lowercase; content:".evil-c2.example"; endswith; sid:9800012; rev:1;)
alert ip [198.51.100.0/24,203.0.113.0/24] any -> $HOME_NET any (msg:"ip blocklist"; sid:9800013; rev:1;)
alert tcp-pkt any any -> $HOME_NET 80 (msg:"tcp syn"; flags:S,12; sid:9800014; rev:1;)
`, hexHash)

	rules, err := rule.ParseStrictRule(ruleText)
	require.NoError(t, err)
	require.Len(t, rules, 14)

	runAll := func(disablePrefilter bool) []int {
		var sids []int
		eng, err := NewStatefulEngine(rules, Config{
			Epoch:            55,
			State:            state.DefaultConfig(),
			DisablePrefilter: disablePrefilter,
			OnAlert: func(a Alert) {
				sids = append(sids, a.Rule.Sid)
			},
		})
		require.NoError(t, err)
		defer eng.Close()

		now := time.Unix(1700020000, 0)

		// 1. Established HTTP request + response flow.
		fcHTTP := FlowSnapshot{
			Key:        FlowKey{Generation: 1},
			First:      Endpoint{Address: netip.MustParseAddr("198.51.100.42"), Port: 45001},
			Second:     Endpoint{Address: netip.MustParseAddr("10.0.0.80"), Port: 80},
			IPv4:       true,
			IPProtocol: uint8(layers.IPProtocolTCP),
		}
		eng.Handle(InspectionEvent{View: ViewFlowCreated, Timestamp: now, Flow: fcHTTP})
		eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fcHTTP, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.42", "10.0.0.80", 45001, 80, 100, 0, true, false, false, nil)})
		eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fcHTTP, Direction: 1, Packet: buildPrefilterTestTCPPacket(t, "10.0.0.80", "198.51.100.42", 80, 45001, 900, 101, true, true, false, nil)})
		eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fcHTTP, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.42", "10.0.0.80", 45001, 80, 101, 901, false, true, false, nil)})

		req := []byte("POST /Admin/%50anel HTTP/1.1\r\nHost: target.local\r\nX-Exploit: yes\r\nUser-Agent: Bad   Agent\t99\r\nContent-Length: 14\r\n\r\npayload=secret")
		eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now, Flow: fcHTTP, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.42", "10.0.0.80", 45001, 80, 101, 901, false, true, true, req), Payload: req})

		resp := []byte("HTTP/1.1 200 OK\r\nX-Debug: Leaked\r\nContent-Length: 2\r\n\r\nOK")
		eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now, Flow: fcHTTP, Direction: 1, Packet: buildPrefilterTestTCPPacket(t, "10.0.0.80", "198.51.100.42", 80, 45001, 901, 101+uint32(len(req)), false, true, true, resp), Payload: resp})
		eng.Handle(InspectionEvent{View: ViewFlowClosed, Timestamp: now, Flow: fcHTTP})

		// 2. Established RDP + binary TCP flow.
		fcRDP := FlowSnapshot{
			Key:        FlowKey{Generation: 2},
			First:      Endpoint{Address: netip.MustParseAddr("198.51.100.43"), Port: 45002},
			Second:     Endpoint{Address: netip.MustParseAddr("10.0.0.10"), Port: 3389},
			IPv4:       true,
			IPProtocol: uint8(layers.IPProtocolTCP),
		}
		eng.Handle(InspectionEvent{View: ViewFlowCreated, Timestamp: now, Flow: fcRDP})
		eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fcRDP, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.43", "10.0.0.10", 45002, 3389, 10, 0, true, false, false, nil)})
		eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fcRDP, Direction: 1, Packet: buildPrefilterTestTCPPacket(t, "10.0.0.10", "198.51.100.43", 3389, 45002, 20, 11, true, true, false, nil)})
		eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fcRDP, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.43", "10.0.0.10", 45002, 3389, 11, 21, false, true, false, nil)})
		rdpData := []byte("\x03\x00\x00\x20\xde\xad\xbe\xefCookie: mstshash=admin\r\n")
		eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now, Flow: fcRDP, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.43", "10.0.0.10", 45002, 3389, 11, 21, false, true, true, rdpData), Payload: rdpData})
		eng.Handle(InspectionEvent{View: ViewFlowClosed, Timestamp: now, Flow: fcRDP})

		// 3. Established TLS ServerHello flow.
		fcTLS := FlowSnapshot{
			Key:        FlowKey{Generation: 3},
			First:      Endpoint{Address: netip.MustParseAddr("198.51.100.44"), Port: 45003},
			Second:     Endpoint{Address: netip.MustParseAddr("10.0.0.80"), Port: 443},
			IPv4:       true,
			IPProtocol: uint8(layers.IPProtocolTCP),
		}
		eng.Handle(InspectionEvent{View: ViewFlowCreated, Timestamp: now, Flow: fcTLS})
		eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fcTLS, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.44", "10.0.0.80", 45003, 443, 10, 0, true, false, false, nil)})
		eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fcTLS, Direction: 1, Packet: buildPrefilterTestTCPPacket(t, "10.0.0.80", "198.51.100.44", 443, 45003, 20, 11, true, true, false, nil)})
		eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fcTLS, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.44", "10.0.0.80", 45003, 443, 11, 21, false, true, false, nil)})
		sh := buildPrefilterTestTLSServerHello(true)
		eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now, Flow: fcTLS, Direction: 1, Packet: buildPrefilterTestTCPPacket(t, "10.0.0.80", "198.51.100.44", 443, 45003, 21, 11, false, true, true, sh), Payload: sh})
		eng.Handle(InspectionEvent{View: ViewFlowClosed, Timestamp: now, Flow: fcTLS})

		// 4. DNS query packet.
		dnsPkt := buildPrefilterTestDNSPacket(t, "Sub.EVIL-C2.Example")
		eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Packet: dnsPkt})

		return sids
	}

	fastSIDs := runAll(false)
	linearSIDs := runAll(true)
	require.Equal(t, linearSIDs, fastSIDs, "prefiltered engine must produce exact same alerts in exact same order as linear evaluation")

	// Sanity check: all 13 alerting rules (sid 9800001 has noalert) fired.
	seen := make(map[int]bool, len(fastSIDs))
	for _, sid := range fastSIDs {
		seen[sid] = true
	}
	for sid := 9800002; sid <= 9800014; sid++ {
		require.True(t, seen[sid], "expected rule sid=%d to fire", sid)
	}
	require.False(t, seen[9800001], "noalert rule sid=9800001 must not emit an alert")
}

func TestStatefulEnginePrefilterConcurrentAndClose(t *testing.T) {
	var ruleLines string
	for i := 0; i < 64; i++ {
		ruleLines += fmt.Sprintf(
			`alert http any any -> any any (msg:"r%d"; flow:established,to_server; http.uri; to_lowercase; content:"/needle-%03d"; sid:%d; rev:1;)`+"\n",
			i, i, 9900000+i,
		)
	}
	rules, err := rule.ParseStrictRule(ruleLines)
	require.NoError(t, err)

	eng, err := NewStatefulEngine(rules, Config{Epoch: 77, State: state.DefaultConfig()})
	require.NoError(t, err)

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 16; i++ {
				gen := uint64(worker*100 + i + 1)
				fc := FlowSnapshot{
					Key:        FlowKey{Generation: gen},
					First:      Endpoint{Address: netip.MustParseAddr("198.51.100.10"), Port: uint16(30000 + worker*100 + i)},
					Second:     Endpoint{Address: netip.MustParseAddr("10.0.0.80"), Port: 80},
					IPv4:       true,
					IPProtocol: uint8(layers.IPProtocolTCP),
				}
				now := time.Now()
				eng.Handle(InspectionEvent{View: ViewFlowCreated, Timestamp: now, Flow: fc})
				eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fc, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.10", "10.0.0.80", fc.First.Port, 80, 10, 0, true, false, false, nil)})
				eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fc, Direction: 1, Packet: buildPrefilterTestTCPPacket(t, "10.0.0.80", "198.51.100.10", 80, fc.First.Port, 20, 11, true, true, false, nil)})
				eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fc, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.10", "10.0.0.80", fc.First.Port, 80, 11, 21, false, true, false, nil)})
				req := []byte(fmt.Sprintf("GET /NEEDLE-%03d HTTP/1.1\r\nHost: demo.local\r\n\r\n", i))
				res := eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now, Flow: fc, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.10", "10.0.0.80", fc.First.Port, 80, 11, 21, false, true, true, req), Payload: req})
				require.Len(t, res.Alerts, 1)
				require.Equal(t, 9900000+i, res.Alerts[0].Rule.Sid)
				eng.Handle(InspectionEvent{View: ViewFlowClosed, Timestamp: now, Flow: fc})
			}
		}(g)
	}
	wg.Wait()
	require.NoError(t, eng.Close())
}

func TestStatefulEngineHotReloadAndTelemetry(t *testing.T) {
	initialRules, err := rule.ParseStrictRule(`
alert http any any -> any any (msg:"v1 rule"; flow:established,to_server; http.uri; content:"/v1-only"; sid:9910001; rev:1;)
alert http any any -> any any (msg:"flowbit setter"; flow:established,to_server; http.uri; content:"/stage1"; flowbits:set,hot.reload; noalert; sid:9910002; rev:1;)
`)
	require.NoError(t, err)

	eng, err := NewStatefulEngine(initialRules, Config{Epoch: 91, State: state.DefaultConfig()})
	require.NoError(t, err)
	defer eng.Close()

	fc := FlowSnapshot{
		Key:        FlowKey{Generation: 500},
		First:      Endpoint{Address: netip.MustParseAddr("198.51.100.10"), Port: 50100},
		Second:     Endpoint{Address: netip.MustParseAddr("10.0.0.80"), Port: 80},
		IPv4:       true,
		IPProtocol: uint8(layers.IPProtocolTCP),
	}
	now := time.Unix(1700040000, 0)
	eng.Handle(InspectionEvent{View: ViewFlowCreated, Timestamp: now, Flow: fc})
	eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fc, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.10", "10.0.0.80", 50100, 80, 10, 0, true, false, false, nil)})
	eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fc, Direction: 1, Packet: buildPrefilterTestTCPPacket(t, "10.0.0.80", "198.51.100.10", 80, 50100, 20, 11, true, true, false, nil)})
	eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fc, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.10", "10.0.0.80", 50100, 80, 11, 21, false, true, false, nil)})

	// Set flowbit before reload on the live established flow.
	req1 := []byte("GET /stage1 HTTP/1.1\r\nHost: demo.local\r\n\r\n")
	res1 := eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now, Flow: fc, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.10", "10.0.0.80", 50100, 80, 11, 21, false, true, true, req1), Payload: req1})
	require.Empty(t, res1.Alerts)

	// Invalid reload (duplicate SID) must fail cleanly without disturbing active rules.
	badRules, err := rule.ParseStrictRule(`
alert http any any -> any any (msg:"dup1"; http.uri; content:"/a"; sid:9910099; rev:1;)
alert http any any -> any any (msg:"dup2"; http.uri; content:"/b"; sid:9910099; rev:1;)
`)
	require.NoError(t, err)
	require.Error(t, eng.Reload(badRules))
	require.Equal(t, uint64(0), eng.Stats().Prefilter.Reloads)

	// Valid hot reload replacing rules with v2 rule that depends on flowbit set before reload + gzip response rule.
	v2Rules, err := rule.ParseStrictRule(`
alert http any any -> any any (msg:"v2 flowbit consumer"; flow:established,to_server; flowbits:isset,hot.reload; http.uri; content:"/stage2"; sid:9910010; rev:1;)
alert http any any -> any any (msg:"v2 gzip resp"; flow:established,to_client; http.response_body; content:"GZIP_HOT_RELOAD_HIT"; sid:9910011; rev:1;)
alert tls any any -> any any (msg:"tls dummy"; flow:established,to_server; tls.sni; content:"unused.example"; sid:9910012; rev:1;)
`)
	require.NoError(t, err)
	require.NoError(t, eng.Reload(v2Rules))

	// Continue the SAME keep-alive flow after reload: flowbit must still be set and v2 rule must fire!
	req2 := []byte("GET /stage2 HTTP/1.1\r\nHost: demo.local\r\n\r\n")
	res2 := eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now.Add(time.Second), Flow: fc, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, "198.51.100.10", "10.0.0.80", 50100, 80, 11+uint32(len(req1)), 21, false, true, true, req2), Payload: req2})
	require.Len(t, res2.Alerts, 1)
	require.Equal(t, 9910010, res2.Alerts[0].Rule.Sid)

	// Send gzipped HTTP response on the same flow and verify decompression telemetry.
	var gz bytes.Buffer
	gw := gzip.NewWriter(&gz)
	_, err = gw.Write([]byte("body with GZIP_HOT_RELOAD_HIT marker"))
	require.NoError(t, err)
	require.NoError(t, gw.Close())
	resp := append([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n", gz.Len())), gz.Bytes()...)
	res3 := eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now.Add(2 * time.Second), Flow: fc, Direction: 1, Packet: buildPrefilterTestTCPPacket(t, "10.0.0.80", "198.51.100.10", 80, 50100, 21, 11+uint32(len(req1)+len(req2)), false, true, true, resp), Payload: resp})
	require.Len(t, res3.Alerts, 1)
	require.Equal(t, 9910011, res3.Alerts[0].Rule.Sid)

	st := eng.Stats().Prefilter
	require.True(t, st.Enabled)
	require.Equal(t, 3, st.ActiveRules)
	require.Equal(t, uint64(1), st.Reloads)
	require.GreaterOrEqual(t, st.DecompressedHTTPBodies, uint64(1))
	require.GreaterOrEqual(t, st.StreamFastFailTLS, uint64(1))
	require.Greater(t, st.RulesSkipped, uint64(0))
}

func TestIPv4SweepIndexZeroFNEquivalence(t *testing.T) {
	var ruleText string
	for i := 0; i < 40; i++ {
		ruleText += fmt.Sprintf(
			`alert ip [198.51.%d.0/24,203.0.113.%d] any -> $HOME_NET any (msg:"blk-%d"; sid:%d; rev:1;)`+"\n",
			i, i+1, i, 9920000+i,
		)
	}
	// Overlapping CIDR + bidirectional + negative exception + TCP flags rules.
	ruleText += `
alert ip [198.51.10.0/23,!198.51.10.5] any <> $HOME_NET any (msg:"overlap-bi"; sid:9920100; rev:1;)
alert tcp-pkt any any -> $HOME_NET 80 (msg:"syn-only"; flags:S,12; sid:9920101; rev:1;)
alert tcp-pkt any any -> $HOME_NET 80 (msg:"ack-only"; flags:A; sid:9920102; rev:1;)
`
	rules, err := rule.ParseStrictRule(ruleText)
	require.NoError(t, err)

	pkts := []gopacket.Packet{
		buildPrefilterTestTCPPacket(t, "198.51.10.5", "10.0.0.80", 40001, 80, 1, 0, true, false, false, nil),  // hits blk-10 + syn-only, excluded from overlap-bi
		buildPrefilterTestTCPPacket(t, "198.51.10.9", "10.0.0.80", 40002, 80, 1, 1, false, true, false, nil),  // hits blk-10 + overlap-bi + ack-only
		buildPrefilterTestTCPPacket(t, "10.0.0.80", "198.51.11.20", 80, 40003, 1, 1, false, true, false, nil), // reverse hits overlap-bi
		buildPrefilterTestTCPPacket(t, "203.0.113.15", "10.0.0.80", 40004, 443, 1, 0, true, false, false, nil),
		buildPrefilterTestTCPPacket(t, "8.8.8.8", "10.0.0.80", 40005, 80, 1, 1, false, true, false, nil), // benign IP, hits ack-only
	}

	run := func(disablePrefilter bool) [][]int {
		eng, err := NewStatefulEngine(rules, Config{
			Epoch:            92,
			State:            state.DefaultConfig(),
			DisablePrefilter: disablePrefilter,
		})
		require.NoError(t, err)
		defer eng.Close()

		var out [][]int
		for i, pk := range pkts {
			res := eng.Handle(InspectionEvent{
				View:      ViewNormalizedPacket,
				Timestamp: time.Unix(1700050000+int64(i), 0),
				Packet:    pk,
			})
			var sids []int
			for _, a := range res.Alerts {
				sids = append(sids, a.Rule.Sid)
			}
			out = append(out, sids)
		}
		return out
	}

	fast := run(false)
	linear := run(true)
	require.Equal(t, linear, fast)
	require.Equal(t, []int{9920010, 9920101}, fast[0])
	require.Equal(t, []int{9920010, 9920100, 9920102}, fast[1])
	require.Equal(t, []int{9920100}, fast[2])
	require.Equal(t, []int{9920014}, fast[3])
	require.Equal(t, []int{9920102}, fast[4])
}

func BenchmarkStatefulEngineHTTPPrefilterVsLinear(b *testing.B) {
	var ruleLines string
	for i := 0; i < 2000; i++ {
		ruleLines += fmt.Sprintf(
			`alert http any any -> any any (msg:"bench-%d"; flow:established,to_server; http.uri; to_lowercase; content:"/exploit/cve-2026-%05d"; sid:%d; rev:1;)`+"\n",
			i, i, 9950000+i,
		)
	}
	rules, err := rule.ParseStrictRule(ruleLines)
	require.NoError(b, err)

	req := []byte("GET /api/v1/health HTTP/1.1\r\nHost: bench.local\r\nUser-Agent: Bench/1.0\r\n\r\n")
	synPkt := buildPrefilterTestTCPPacket(&testing.T{}, "198.51.100.10", "10.0.0.80", 42000, 80, 10, 0, true, false, false, nil)
	synAckPkt := buildPrefilterTestTCPPacket(&testing.T{}, "10.0.0.80", "198.51.100.10", 80, 42000, 20, 11, true, true, false, nil)
	ackPkt := buildPrefilterTestTCPPacket(&testing.T{}, "198.51.100.10", "10.0.0.80", 42000, 80, 11, 21, false, true, false, nil)
	dataPkt := buildPrefilterTestTCPPacket(&testing.T{}, "198.51.100.10", "10.0.0.80", 42000, 80, 11, 21, false, true, true, req)

	for _, tc := range []struct {
		name             string
		disablePrefilter bool
	}{
		{"MinirehsPrefilter", false},
		{"LinearBaseline", true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			eng, err := NewStatefulEngine(rules, Config{
				Epoch:            88,
				State:            state.DefaultConfig(),
				DisablePrefilter: tc.disablePrefilter,
			})
			require.NoError(b, err)
			defer eng.Close()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				fc := FlowSnapshot{
					Key:        FlowKey{Generation: uint64(i + 1)},
					First:      Endpoint{Address: netip.MustParseAddr("198.51.100.10"), Port: 42000},
					Second:     Endpoint{Address: netip.MustParseAddr("10.0.0.80"), Port: 80},
					IPv4:       true,
					IPProtocol: uint8(layers.IPProtocolTCP),
				}
				now := time.Unix(1700030000, int64(i))
				eng.Handle(InspectionEvent{View: ViewFlowCreated, Timestamp: now, Flow: fc})
				eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fc, Direction: 0, Packet: synPkt})
				eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fc, Direction: 1, Packet: synAckPkt})
				eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fc, Direction: 0, Packet: ackPkt})
				eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now, Flow: fc, Direction: 0, Packet: dataPkt, Payload: req})
				eng.Handle(InspectionEvent{View: ViewFlowClosed, Timestamp: now, Flow: fc})
			}
		})
	}
}

func buildPrefilterTestUDPPacket(t *testing.T, srcIP, dstIP string, srcPort, dstPort uint16, payload []byte) gopacket.Packet {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x01},
		DstMAC:       net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x02},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    net.ParseIP(srcIP).To4(),
		DstIP:    net.ParseIP(dstIP).To4(),
	}
	udp := &layers.UDP{
		SrcPort: layers.UDPPort(srcPort),
		DstPort: layers.UDPPort(dstPort),
	}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	require.NoError(t, gopacket.SerializeLayers(buf, opts, eth, ip, udp, gopacket.Payload(payload)))
	return gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}

func TestStatefulEngineProtocolExtensionsAndPrefilterEquivalence(t *testing.T) {
	ruleText := `
alert http2 any any -> any any (msg:"HTTP2 Window"; flow:established,to_server; http2.frametype:WINDOWUPDATE; http2.window:>2147483000; sid:940001; rev:1;)
alert snmp any any -> any 161 (msg:"SNMP RCE"; flow:established,to_server; snmp.version:<=2; content:"|06 0D 2B 06 01 04 01 09 09 85 10 01 04 02 01|"; pcre:"/^.{0,256}(?:\x24\x28|\x3b|\x7c|\x60)/R"; sid:940002; rev:1;)
alert tcp any any -> any 445 (msg:"DCERPC Bridged"; flow:established,to_server; dcerpc.iface:4b324fc8-1670-01d3-1278-5a47bf6ee188; dcerpc.opnum:31; dcerpc.stub_data; content:"|5C|..|5C|"; sid:940003; rev:1;)
alert tcp any any -> any any (msg:"Gzip Tar ZipSlip"; flow:established,to_client; file.magic; content:"POSIX tar archive"; file.data; content:"|00 00 00 2e 2e 2f|"; fast_pattern; sid:940004; rev:1;)
alert http any any -> any any (msg:"PigButcher Auth"; flow:established,to_server; app-layer-event:http.request_auth_unrecognized; flowint:http.anomaly.count,+,1; http.header_names; to_lowercase; content:"|0d 0a|x-channel-code"; fast_pattern; sid:940005; rev:1;)
`
	rules, err := rule.ParseStrictRule(ruleText)
	require.NoError(t, err)
	require.Len(t, rules, 5)

	runScenario := func(disablePrefilter bool) []int {
		eng, err := NewStatefulEngine(rules, Config{
			Epoch:            99,
			State:            state.DefaultConfig(),
			DisablePrefilter: disablePrefilter,
		})
		require.NoError(t, err)
		defer eng.Close()

		var sids []int
		collect := func(res Result) {
			for _, a := range res.Alerts {
				if a.Rule != nil {
					sids = append(sids, a.Rule.Sid)
				}
			}
		}

		now := time.Unix(1700050000, 0)
		establishTCP := func(gen uint64, srcIP, dstIP string, srcPort, dstPort uint16) FlowSnapshot {
			fc := FlowSnapshot{
				Key:        FlowKey{Generation: gen},
				First:      Endpoint{Address: netip.MustParseAddr(srcIP), Port: srcPort},
				Second:     Endpoint{Address: netip.MustParseAddr(dstIP), Port: dstPort},
				IPv4:       true,
				IPProtocol: uint8(layers.IPProtocolTCP),
			}
			eng.Handle(InspectionEvent{View: ViewFlowCreated, Timestamp: now, Flow: fc})
			eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fc, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, srcIP, dstIP, srcPort, dstPort, 1, 0, true, false, false, nil)})
			eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fc, Direction: 1, Packet: buildPrefilterTestTCPPacket(t, dstIP, srcIP, dstPort, srcPort, 10, 2, true, true, false, nil)})
			eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Flow: fc, Direction: 0, Packet: buildPrefilterTestTCPPacket(t, srcIP, dstIP, srcPort, dstPort, 2, 11, false, true, false, nil)})
			return fc
		}

		// 1. Multi-segment HTTP/2 stream: chunk 1 preface, chunk 2 WINDOW_UPDATE, chunk 3 PING (no duplicate replay).
		fcH2 := establishTCP(1, "198.51.100.1", "10.0.0.10", 40001, 443)
		preface := []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")
		wuFrame := []byte{0, 0, 4, 8, 0, 0, 0, 0, 0, 0x7f, 0xff, 0xff, 0xff}
		pingFrame := []byte{0, 0, 8, 6, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}
		pktH2 := buildPrefilterTestTCPPacket(t, "198.51.100.1", "10.0.0.10", 40001, 443, 2, 11, false, true, true, wuFrame)
		collect(eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now, Flow: fcH2, Direction: 0, Packet: pktH2, Payload: preface}))
		collect(eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now, Flow: fcH2, Direction: 0, Packet: pktH2, Payload: wuFrame}))
		collect(eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now, Flow: fcH2, Direction: 0, Packet: pktH2, Payload: pingFrame}))

		// 2. Established UDP SNMPv2c pseudo-flow (bidirectional packet makes UDP flow established).
		oid := []byte{0x06, 0x0d, 0x2b, 0x06, 0x01, 0x04, 0x01, 0x09, 0x09, 0x85, 0x10, 0x01, 0x04, 0x02, 0x01}
		snmpBody := append([]byte{0x02, 0x01, 0x01, 0x04, 0x06, 'p', 'u', 'b', 'l', 'i', 'c'}, oid...)
		snmpBody = append(snmpBody, []byte(";id")...)
		snmpPDU := append([]byte{0x30, byte(len(snmpBody))}, snmpBody...)
		snmpProbe := []byte{0x30, 0x0b, 0x02, 0x01, 0x01, 0x04, 0x06, 'p', 'u', 'b', 'l', 'i', 'c'}
		collect(eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Packet: buildPrefilterTestUDPPacket(t, "198.51.100.2", "10.0.0.10", 40002, 161, snmpProbe)}))
		collect(eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Packet: buildPrefilterTestUDPPacket(t, "10.0.0.10", "198.51.100.2", 161, 40002, snmpProbe)}))
		collect(eng.Handle(InspectionEvent{View: ViewNormalizedPacket, Timestamp: now, Packet: buildPrefilterTestUDPPacket(t, "198.51.100.2", "10.0.0.10", 40002, 161, snmpPDU)}))

		// 3. DCE/RPC bridged via pcaputil ProtocolEvent (Bind in ViewTransaction, Request in ViewStream).
		fcRPC := establishTCP(3, "198.51.100.3", "10.0.0.10", 40003, 445)
		eng.Handle(InspectionEvent{
			View:      ViewTransaction,
			Timestamp: now,
			Flow:      fcRPC,
			Protocol: &pcaputil.ProtocolEvent{
				Protocol: "dcerpc",
				Status:   "decoded",
				Fields: map[string]any{
					"Context ID":              uint16(0),
					"Abstract Syntax":         "c84f324b7016d30112785a47bf6ee188",
					"Interface Version Major": uint16(3),
					"Interface Version Minor": uint16(0),
				},
			},
		})
		stub := []byte("abc\\..\\..\\pwn")
		reqPDU := make([]byte, 24+len(stub))
		reqPDU[0], reqPDU[1], reqPDU[2], reqPDU[3], reqPDU[4] = 5, 0, 0, 0x03, 0x10
		binary.LittleEndian.PutUint16(reqPDU[8:10], uint16(len(reqPDU)))
		binary.LittleEndian.PutUint16(reqPDU[20:22], 0)  // ctxID 0 bound by pcaputil ProtocolEvent
		binary.LittleEndian.PutUint16(reqPDU[22:24], 31) // opnum 31
		copy(reqPDU[24:], stub)
		pktRPC := buildPrefilterTestTCPPacket(t, "198.51.100.3", "10.0.0.10", 40003, 445, 2, 11, false, true, true, reqPDU)
		collect(eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now, Flow: fcRPC, Direction: 0, Packet: pktRPC, Payload: reqPDU}))

		// 4. Gzip-compressed HTTP response carrying a POSIX tar Zip-Slip over alert tcp (zero-FN prefilter check).
		fcTar := establishTCP(4, "10.0.0.10", "198.51.100.4", 40004, 80)
		tarBuf := make([]byte, 512)
		copy(tarBuf[257:263], []byte("ustar\x00"))
		copy(tarBuf[20:26], []byte{0x00, 0x00, 0x00, '.', '.', '/'})
		var gz bytes.Buffer
		gw := gzip.NewWriter(&gz)
		_, _ = gw.Write(tarBuf)
		_ = gw.Close()
		httpResp := append([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n", gz.Len())), gz.Bytes()...)
		pktTar := buildPrefilterTestTCPPacket(t, "198.51.100.4", "10.0.0.10", 80, 40004, 11, 2, false, true, true, httpResp)
		collect(eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now, Flow: fcTar, Direction: 1, Packet: pktTar, Payload: httpResp}))

		// 5. PigButcher app-layer-event:http.request_auth_unrecognized + flowint.
		fcAuth := establishTCP(5, "10.0.0.10", "198.51.100.5", 40005, 80)
		authReq := []byte("GET /x HTTP/1.1\r\nHost: bad.example\r\nAuthorization: baretoken999\r\nX-Channel-Code: 7\r\n\r\n")
		pktAuth := buildPrefilterTestTCPPacket(t, "10.0.0.10", "198.51.100.5", 40005, 80, 2, 11, false, true, true, authReq)
		collect(eng.Handle(InspectionEvent{View: ViewStream, Timestamp: now, Flow: fcAuth, Direction: 0, Packet: pktAuth, Payload: authReq}))

		return sids
	}

	gotPrefilter := runScenario(false)
	gotLinear := runScenario(true)
	require.Equal(t, []int{940001, 940002, 940003, 940004, 940005}, gotLinear)
	require.Equal(t, gotLinear, gotPrefilter, "prefilter and linear baseline must produce identical alerts across all 5 protocol extensions")
}
