package bin_parser

import (
	"bytes"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/rules"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func TestProtocolCatalogRuleFilesExist(t *testing.T) {
	for _, item := range ProtocolCatalog {
		if item.RuleFile == "" {
			continue
		}
		_, err := rules.RuleFS.ReadFile(item.RuleFile)
		require.NoError(t, err, "missing rule for %s: %s", item.Name, item.RuleFile)
	}
}

func TestProtocolRoadmapIntegrity(t *testing.T) {
	seen := map[string]string{}
	var dupes []string
	for _, item := range ProtocolRoadmap {
		require.NotEmpty(t, item.Name)
		require.NotEmpty(t, item.Family)
		require.NotEmpty(t, item.Status)
		require.NotEmpty(t, item.Priority)
		require.NotEmpty(t, item.Sources)
		if prev, ok := seen[item.Name]; ok {
			dupes = append(dupes, item.Name+" also in "+prev+" and "+item.Family)
		}
		seen[item.Name] = item.Family
	}
	require.Empty(t, dupes, "duplicate roadmap names: %v", dupes)

	for _, item := range ProtocolCatalog {
		_, ok := seen[item.Name]
		if !ok {
			// catalog uses a few aliases (Ethernet vs Ethernet II, MSRdp vs RDP)
			switch item.Name {
			case "Ethernet", "MSRdp", "IIOP":
				continue
			}
			t.Errorf("catalog protocol %q missing from ProtocolRoadmap", item.Name)
		}
	}

	total, done, partial, todo := RoadmapStats()
	family := map[string]int{}
	priority := map[string]int{}
	source := map[string]int{}
	for _, item := range ProtocolRoadmap {
		family[item.Family]++
		priority[item.Priority]++
		for _, s := range item.Sources {
			source[s]++
		}
	}
	t.Logf("protocol roadmap: total=%d done=%d partial=%d todo=%d", total, done, partial, todo)
	t.Logf("by source: %#v", source)
	t.Logf("by priority: %#v", priority)
	t.Logf("by family: %#v", family)
	require.Greater(t, total, 400)
	require.Greater(t, todo, 300)
	require.Greater(t, done+partial, 10)
	require.Greater(t, source[srcColasoft], 200)
	require.Greater(t, source[srcPrivate], 150)
}

func parseRule(t *testing.T, data []byte, rule string, keys ...string) *base.NodeValue {
	t.Helper()
	node, err := parser.ParseBinary(bytes.NewReader(data), rule, keys...)
	require.NoError(t, err)
	val, err := node.Result()
	require.NoError(t, err)
	require.NotNil(t, val)
	return val
}

func parseEthernet(t *testing.T, frame []byte) *base.NodeValue {
	t.Helper()
	return parseRule(t, frame, "ethernet").Child("Ethernet")
}

func mustChild(t *testing.T, v *base.NodeValue, names ...string) *base.NodeValue {
	t.Helper()
	require.NotNil(t, v)
	cur := v
	path := cur.Name
	for _, name := range names {
		cur = cur.Child(name)
		path += "/" + name
		require.NotNil(t, cur, "missing node %s", path)
	}
	return cur
}

func uintVal(t *testing.T, v *base.NodeValue) uint64 {
	t.Helper()
	require.NotNil(t, v)
	switch n := v.Value.(type) {
	case uint8:
		return uint64(n)
	case uint16:
		return uint64(n)
	case uint32:
		return uint64(n)
	case uint64:
		return n
	case int:
		return uint64(n)
	case int8:
		return uint64(uint8(n))
	default:
		t.Fatalf("node %s: unexpected numeric type %T (%v)", v.Name, v.Value, v.Value)
		return 0
	}
}

func intVal(t *testing.T, v *base.NodeValue) int64 {
	t.Helper()
	require.NotNil(t, v)
	switch n := v.Value.(type) {
	case int8:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case uint8:
		return int64(n)
	default:
		t.Fatalf("node %s: unexpected signed type %T (%v)", v.Name, v.Value, v.Value)
		return 0
	}
}

func bytesVal(t *testing.T, v *base.NodeValue) []byte {
	t.Helper()
	require.NotNil(t, v)
	switch n := v.Value.(type) {
	case []byte:
		return n
	case string:
		return []byte(n)
	default:
		t.Fatalf("node %s: unexpected bytes type %T", v.Name, v.Value)
		return nil
	}
}

func strVal(t *testing.T, v *base.NodeValue) string {
	t.Helper()
	require.NotNil(t, v)
	switch n := v.Value.(type) {
	case string:
		return n
	case []byte:
		return string(n)
	default:
		t.Fatalf("node %s: unexpected string type %T", v.Name, v.Value)
		return ""
	}
}

func serializeLayers(t *testing.T, ls ...gopacket.SerializableLayer) []byte {
	t.Helper()
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	require.NoError(t, gopacket.SerializeLayers(buf, opts, ls...))
	return buf.Bytes()
}

func ipv4UDPFrame(t *testing.T, srcPort, dstPort layers.UDPPort, payload gopacket.SerializableLayer) []byte {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		DstMAC:       net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    net.IP{10, 0, 0, 1},
		DstIP:    net.IP{10, 0, 0, 2},
	}
	udp := &layers.UDP{SrcPort: srcPort, DstPort: dstPort}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	return serializeLayers(t, eth, ip, udp, payload)
}

func ipv4TCPFrame(t *testing.T, srcPort, dstPort layers.TCPPort, payload []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		DstMAC:       net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    net.IP{10, 0, 0, 1},
		DstIP:    net.IP{10, 0, 0, 2},
	}
	tcp := &layers.TCP{SrcPort: srcPort, DstPort: dstPort, Seq: 1, Ack: 1, Window: 8192, PSH: true, ACK: true}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	return serializeLayers(t, eth, ip, tcp, gopacket.Payload(payload))
}

func TestNTPWiresharkSample(t *testing.T) {
	// First NTP packet from Wireshark SampleCaptures NTP_sync.pcap,
	// as embedded in gopacket layers/ntp_test.go.
	frame := []byte{
		0x00, 0x0c, 0x41, 0x82, 0xb2, 0x53, 0x00, 0xd0,
		0x59, 0x6c, 0x40, 0x4e, 0x08, 0x00, 0x45, 0x00,
		0x00, 0x4c, 0x0a, 0x42, 0x00, 0x00, 0x80, 0x11,
		0xb5, 0xfa, 0xc0, 0xa8, 0x32, 0x32, 0x43, 0x81,
		0x44, 0x09, 0x00, 0x7b, 0x00, 0x7b, 0x00, 0x38,
		0xf8, 0xd2, 0xd9, 0x00, 0x0a, 0xfa, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x01, 0x02, 0x90, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xc5, 0x02, 0x04, 0xec, 0xec, 0x42,
		0xee, 0x92,
	}

	pkt := gopacket.NewPacket(frame, layers.LinkTypeEthernet, gopacket.Default)
	require.Nil(t, pkt.ErrorLayer())
	gopacketNTP, ok := pkt.ApplicationLayer().(*layers.NTP)
	require.True(t, ok, "gopacket should dissect Wireshark NTP sample")

	eth := parseEthernet(t, frame)
	ntp := mustChild(t, eth, "IP", "UDP", "NTP")
	require.Equal(t, uint64(gopacketNTP.LeapIndicator), uintVal(t, ntp.Child("Leap Indicator")))
	require.Equal(t, uint64(gopacketNTP.Version), uintVal(t, ntp.Child("Version")))
	require.Equal(t, uint64(gopacketNTP.Mode), uintVal(t, ntp.Child("Mode")))
	require.Equal(t, uint64(gopacketNTP.Stratum), uintVal(t, ntp.Child("Stratum")))
	require.Equal(t, int64(gopacketNTP.Poll), intVal(t, ntp.Child("Poll")))
	require.Equal(t, int64(gopacketNTP.Precision), intVal(t, ntp.Child("Precision")))
	require.Equal(t, uint64(gopacketNTP.RootDispersion), uintVal(t, ntp.Child("Root Dispersion")))
	require.Equal(t, []byte{0xc5, 0x02, 0x04, 0xec, 0xec, 0x42, 0xee, 0x92}, bytesVal(t, ntp.Child("Transmit Timestamp")))
}

func TestDHCPGopacketDiscover(t *testing.T) {
	dhcp := &layers.DHCPv4{
		Operation:    layers.DHCPOpRequest,
		HardwareType: layers.LinkTypeEthernet,
		Xid:          0x12345678,
		ClientIP:     net.IPv4zero,
		YourClientIP: net.IPv4zero,
		NextServerIP: net.IPv4zero,
		RelayAgentIP: net.IPv4zero,
		ClientHWAddr: net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		ServerName:   make([]byte, 64),
		File:         make([]byte, 128),
		Options: []layers.DHCPOption{
			layers.NewDHCPOption(layers.DHCPOptMessageType, []byte{byte(layers.DHCPMsgTypeDiscover)}),
			layers.NewDHCPOption(layers.DHCPOptHostname, []byte("example.com")),
			layers.NewDHCPOption(layers.DHCPOptEnd, nil),
		},
	}
	frame := ipv4UDPFrame(t, 68, 67, dhcp)
	eth := parseEthernet(t, frame)
	parsed := mustChild(t, eth, "IP", "UDP", "DHCP")
	require.Equal(t, uint64(1), uintVal(t, parsed.Child("Operation")))
	require.Equal(t, uint64(0x12345678), uintVal(t, parsed.Child("Xid")))
	require.Equal(t, uint64(0x63825363), uintVal(t, parsed.Child("Magic Cookie")))
	require.Equal(t, []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}, bytesVal(t, parsed.Child("Client MAC")))

	opts := parsed.Child("Options")
	require.NotNil(t, opts)
	require.True(t, opts.IsList())
	require.GreaterOrEqual(t, len(opts.Children()), 2)
	require.Equal(t, uint64(53), uintVal(t, opts.Children()[0].Child("Code")))
	require.Equal(t, []byte{1}, bytesVal(t, opts.Children()[0].Child("Data")))
}

func TestDHCPOfferYourIP(t *testing.T) {
	dhcp := &layers.DHCPv4{
		Operation:    layers.DHCPOpReply,
		HardwareType: layers.LinkTypeEthernet,
		Xid:          0xaabbccdd,
		ClientIP:     net.IPv4zero,
		YourClientIP: net.IP{192, 168, 0, 123},
		NextServerIP: net.IP{192, 168, 0, 1},
		RelayAgentIP: net.IPv4zero,
		ClientHWAddr: net.HardwareAddr{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		ServerName:   make([]byte, 64),
		File:         make([]byte, 128),
		Options: []layers.DHCPOption{
			layers.NewDHCPOption(layers.DHCPOptMessageType, []byte{byte(layers.DHCPMsgTypeOffer)}),
			layers.NewDHCPOption(layers.DHCPOptServerID, []byte{192, 168, 0, 1}),
			layers.NewDHCPOption(layers.DHCPOptEnd, nil),
		},
	}
	frame := ipv4UDPFrame(t, 67, 68, dhcp)
	eth := parseEthernet(t, frame)
	parsed := mustChild(t, eth, "IP", "UDP", "DHCP")
	require.Equal(t, uint64(2), uintVal(t, parsed.Child("Operation")))
	require.Equal(t, []byte{192, 168, 0, 123}, bytesVal(t, parsed.Child("Your IP")))
	require.Equal(t, []byte{192, 168, 0, 1}, bytesVal(t, parsed.Child("Server IP")))
}

func TestSNMPGetRequest(t *testing.T) {
	// SNMPv1 GetRequest, community "public", oid 1.3.6.1.2.1.1.1.0
	raw, err := codec.DecodeHex("302602010004067075626c6963a019020101020100020100300e300c06082b060102010101000500")
	require.NoError(t, err)
	snmp := parseRule(t, raw, "application-layer.snmp", "SNMP")
	require.Equal(t, uint64(0x30), uintVal(t, snmp.Child("Identifier")))
	require.Equal(t, []byte{0}, bytesVal(t, snmp.Child("Version")))
	require.Equal(t, []byte("public"), bytesVal(t, snmp.Child("Community")))
	require.Equal(t, uint64(0xa0), uintVal(t, snmp.Child("PDU Tag")))
	require.Equal(t, []byte{1}, bytesVal(t, mustChild(t, snmp, "PDU Body", "Request ID")))

	frame := ipv4UDPFrame(t, 40000, 161, gopacket.Payload(raw))
	eth := parseEthernet(t, frame)
	wired := mustChild(t, eth, "IP", "UDP", "SNMP")
	require.Equal(t, []byte("public"), bytesVal(t, wired.Child("Community")))
}

func TestVLANIPv4ARPInner(t *testing.T) {
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		DstMAC:       net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		EthernetType: layers.EthernetTypeDot1Q,
	}
	vlan := &layers.Dot1Q{Priority: 3, VLANIdentifier: 100, Type: layers.EthernetTypeARP}
	arp := &layers.ARP{
		AddrType:          layers.LinkTypeEthernet,
		Protocol:          layers.EthernetTypeIPv4,
		HwAddressSize:     6,
		ProtAddressSize:   4,
		Operation:         layers.ARPRequest,
		SourceHwAddress:   []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		SourceProtAddress: []byte{10, 0, 0, 1},
		DstHwAddress:      []byte{0, 0, 0, 0, 0, 0},
		DstProtAddress:    []byte{10, 0, 0, 2},
	}
	frame := serializeLayers(t, eth, vlan, arp)
	parsed := parseEthernet(t, frame)
	require.Equal(t, uint64(0x8100), uintVal(t, parsed.Child("Type")))
	q := mustChild(t, parsed, "VLAN")
	tci := uintVal(t, q.Child("TCI"))
	require.Equal(t, uint64(3), tci>>13)
	require.Equal(t, uint64(100), tci&0x0fff)
	require.Equal(t, uint64(0x0806), uintVal(t, q.Child("Type")))
	require.NotNil(t, q.Child("ARP"))
}

func TestSSHIdentification(t *testing.T) {
	payload := []byte("SSH-2.0-OpenSSH_8.9\r\n")
	ssh := parseRule(t, payload, "application-layer.ssh", "SSH")
	require.Equal(t, "SSH-2.0-OpenSSH_8.9", strVal(t, ssh.Child("Identification")))

	eth := parseEthernet(t, ipv4TCPFrame(t, 4000, 22, payload))
	require.Equal(t, "SSH-2.0-OpenSSH_8.9", strVal(t, mustChild(t, eth, "IP", "TCP", "SSH", "Identification")))
}

func TestFTPReply(t *testing.T) {
	payload := []byte("220 FTP server ready.\r\n")
	ftp := parseRule(t, payload, "application-layer.ftp", "FTP")
	require.Equal(t, "220", strVal(t, ftp.Child("Code")))
	require.Equal(t, "FTP server ready.", strVal(t, ftp.Child("Message")))

	eth := parseEthernet(t, ipv4TCPFrame(t, 21, 50000, payload))
	require.Equal(t, "220", strVal(t, mustChild(t, eth, "IP", "TCP", "FTP", "Code")))
}

func TestSMTPReply(t *testing.T) {
	payload := []byte("220 mail.example.com ESMTP\r\n")
	smtp := parseRule(t, payload, "application-layer.smtp", "SMTP")
	require.Equal(t, "220", strVal(t, smtp.Child("Code")))
	require.Equal(t, "mail.example.com ESMTP", strVal(t, smtp.Child("Message")))

	eth := parseEthernet(t, ipv4TCPFrame(t, 25, 50000, payload))
	require.Equal(t, "220", strVal(t, mustChild(t, eth, "IP", "TCP", "SMTP", "Code")))
}

func redisRoot(t *testing.T, v *base.NodeValue) *base.NodeValue {
	t.Helper()
	if v.Child("RESP") != nil {
		return v.Child("RESP")
	}
	return v
}

func TestRedisPING(t *testing.T) {
	payload := []byte("*1\r\n$4\r\nPING\r\n")
	redis := redisRoot(t, parseRule(t, payload, "application-layer.redis", "Redis"))
	require.Equal(t, uint64('*'), uintVal(t, redis.Child("Prefix")))
	require.Equal(t, "1", strVal(t, mustChild(t, redis, "Array", "Count")))
	elements := mustChild(t, redis, "Array", "Elements")
	require.True(t, elements.IsList())
	require.Len(t, elements.Children(), 1)
	bulk := elements.Children()[0]
	require.Equal(t, uint64('$'), uintVal(t, bulk.Child("Prefix")))
	require.Equal(t, []byte("PING"), bytesVal(t, mustChild(t, bulk, "Bulk", "Data")))

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 6379, payload))
	wired := redisRoot(t, mustChild(t, eth, "IP", "TCP", "Redis"))
	elem := mustChild(t, wired, "Array", "Elements").Children()[0]
	require.Equal(t, []byte("PING"), bytesVal(t, mustChild(t, elem, "Bulk", "Data")))
}

func TestMQTTConnectAndConnack(t *testing.T) {
	connect, err := codec.DecodeHex("101000044d5154540402003c000474657374")
	require.NoError(t, err)
	mqtt := parseRule(t, connect, "application-layer.mqtt", "MQTT")
	require.Equal(t, uint64(1), uintVal(t, mqtt.Child("Packet Type")))
	require.Equal(t, "MQTT", strVal(t, mustChild(t, mqtt, "Payload", "Connect", "Protocol Name")))
	require.Equal(t, "test", strVal(t, mustChild(t, mqtt, "Payload", "Connect", "Client ID")))
	require.Equal(t, uint64(60), uintVal(t, mustChild(t, mqtt, "Payload", "Connect", "Keep Alive")))

	eth := parseEthernet(t, ipv4TCPFrame(t, 50000, 1883, connect))
	require.Equal(t, uint64(1), uintVal(t, mustChild(t, eth, "IP", "TCP", "MQTT", "Packet Type")))

	connack, err := codec.DecodeHex("20020000")
	require.NoError(t, err)
	ack := parseRule(t, connack, "application-layer.mqtt", "MQTT")
	require.Equal(t, uint64(2), uintVal(t, ack.Child("Packet Type")))
	require.Equal(t, uint64(0), uintVal(t, mustChild(t, ack, "Payload", "Connack", "Return Code")))
}

func TestICMPDestinationUnreachable(t *testing.T) {
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		DstMAC:       net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolICMPv4,
		SrcIP:    net.IP{10, 0, 0, 1},
		DstIP:    net.IP{10, 0, 0, 2},
	}
	icmp := &layers.ICMPv4{TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeDestinationUnreachable, 1)}
	frame := serializeLayers(t, eth, ip, icmp, gopacket.Payload([]byte{0, 1, 2, 3, 4, 5, 6, 7}))
	parsed := parseEthernet(t, frame)
	icmpNode := mustChild(t, parsed, "IP", "ICMP")
	require.Equal(t, uint64(3), uintVal(t, icmpNode.Child("Type")))
	require.NotNil(t, icmpNode.Child("ICMP Destination Unreachable"))
}

func TestNTPZeroPacketParsesHeader(t *testing.T) {
	zero := bytes.Repeat([]byte{0}, 48)
	ntp := parseRule(t, zero, "application-layer.ntp", "NTP")
	require.Equal(t, uint64(0), uintVal(t, ntp.Child("Version")))
	require.Equal(t, uint64(0), uintVal(t, ntp.Child("Mode")))
}
