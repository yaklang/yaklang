package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// The official eMule download page links the community-maintained irwir/eMule
// implementation. BaseClient.cpp ProcessHelloTypePacket and packets.cpp CTag
// define these layouts. The expected values below were independently checked
// against the original nDPI capture with Wireshark, not computed from the rule.
// This covers client Hello/HelloAnswer, not server login, transfers or UDP.
const edonkeyTestRule = "application-layer.edonkey"

func edonkeyTestParse(t *testing.T, wire []byte) *base.Node {
	t.Helper()
	return protocolCorpusRequireBoundedRuleParse(t, wire, edonkeyTestRule, "EDonkey")
}

func edonkeyTestInfo(t *testing.T, node *base.Node) map[string]any {
	t.Helper()
	info, ok := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.True(t, ok, "missing additional fields on %s", node.Name)
	return info
}

func edonkeyTestMessages(t *testing.T, node *base.Node) []*base.Node {
	t.Helper()
	list := protocolCorpusFindNode(node, "Messages")
	require.NotNil(t, list)
	return list.Children
}

func edonkeyTestWire(answer bool, tags ...[]byte) []byte {
	wire := []byte{0xe3, 0, 0, 0, 0, 1}
	if answer {
		wire[5] = 0x4c
	} else {
		wire = append(wire, 16)
	}
	wire = append(wire, []byte("0123456789abcdef")...)
	wire = binary.LittleEndian.AppendUint32(wire, 0x12345678)
	wire = binary.LittleEndian.AppendUint16(wire, 4662)
	wire = binary.LittleEndian.AppendUint32(wire, uint32(len(tags)))
	for _, tag := range tags {
		wire = append(wire, tag...)
	}
	wire = append(wire, 192, 0, 2, 9)
	wire = binary.LittleEndian.AppendUint16(wire, 4242)
	binary.LittleEndian.PutUint32(wire[1:], uint32(len(wire)-5))
	return wire
}

func edonkeyTestTag(kind, id byte, value []byte, compact bool) []byte {
	tag := []byte{kind, 1, 0, id}
	if compact {
		tag = []byte{kind | 0x80, id}
	}
	return append(tag, value...)
}

func edonkeyTestCapturedFields(t *testing.T, node *base.Node, answer bool, offset int) {
	t.Helper()
	messages := edonkeyTestMessages(t, node)
	require.Len(t, messages, 1)
	m := messages[0]
	hash, size, id, port, server, serverPort := "1b42cd5b940ed779c20ab755fe256f97", 119, uint64(0xe367497d), uint64(19850), []byte{85, 17, 172, 34}, uint64(4321)
	ids := []byte{1, 0x11, 0x55, 0xee, 0xf9, 0xfa, 0xfb, 0xfe}
	values := []any{"[CHN][VeryCD]xf", uint64(60), "xl build61", uint64(0x1489e90c), uint64(0x4ba24ba2), uint64(0x34134214), uint64(0xc000), uint64(0x94)}
	opcode := uint64(1)
	if answer {
		hash, size, id, port, server, serverPort = "3a3544a8310e281d51ed5182f4cf6fd4", 127, 0xcff4718c, 7551, []byte{212, 63, 206, 35}, 4242
		ids = []byte{1, 0x11, 0xf9, 0xfa, 0xfe, 0xfb, 0x55, 0xee}
		values = []any{"[CHN][VeryCD]yourname", uint64(60), uint64(0x1d891d89), uint64(0x3413421b), uint64(0x1b4), uint64(0xc000), "VeryCD 090304", uint64(0x1489e90c)}
		opcode = 0x4c
		require.Nil(t, protocolCorpusFindNode(m, "User Hash Length"))
	} else {
		protocolCorpusRequireValue(t, m, "User Hash Length", uint64(16))
	}
	decodedHash, err := hex.DecodeString(hash)
	require.NoError(t, err)
	for name, value := range map[string]any{"Protocol": uint64(0xe3), "Message Length": uint64(size), "Opcode": opcode, "User Hash": decodedHash, "Client ID": id, "TCP Port": port, "Tag Count": uint64(8), "Server Address": server, "Server Port": serverPort} {
		protocolCorpusRequireValue(t, m, name, value)
	}
	_, first, last := protocolCorpusConsumedRange(m, uint64((offset+size+5)*8))
	require.Equal(t, uint64(offset*8), first)
	require.Equal(t, uint64((offset+size+5)*8), last)
	tags := protocolCorpusFindNode(m, "Tags")
	require.NotNil(t, tags)
	require.Len(t, tags.Children, 8)
	for i, tag := range tags.Children {
		protocolCorpusRequireValue(t, tag, "Name Length", uint64(1))
		protocolCorpusRequireValue(t, tag, "Name ID", uint64(ids[i]))
		switch value := values[i].(type) {
		case string:
			protocolCorpusRequireValue(t, tag, "Tag Type", uint64(2))
			protocolCorpusRequireValue(t, tag, "String Length", uint64(len(value)))
			protocolCorpusRequireValue(t, tag, "String Value", value)
		case uint64:
			protocolCorpusRequireValue(t, tag, "Tag Type", uint64(3))
			protocolCorpusRequireValue(t, tag, "Unsigned Value", value)
		}
		var fields map[string]uint64
		switch ids[i] {
		case 0xf9:
			ports := uint64(19362)
			if answer {
				ports = 7561
			}
			fields = map[string]uint64{"Kad Port": ports, "UDP Port": ports}
		case 0xfa:
			fields = map[string]uint64{"AICH Version": 1, "Unicode": 1, "UDP Version": 4, "Compression Version": 1, "Identification Version": 3, "Source Exchange Version": 4, "Extended Requests Version": 2, "Comments Version": 1, "Peer Cache": 0, "No View Shared Files": 1, "Multipacket": 0, "Preview": 0}
			if answer {
				fields["Peer Cache"], fields["No View Shared Files"], fields["Multipacket"], fields["Preview"] = 1, 0, 1, 1
			}
		case 0xfb:
			fields = map[string]uint64{"Compatible Client ID": 0, "Major Version": 0, "Minor Version": 48, "Update Version": 0, "Build Version": 0}
		case 0xfe:
			fields = map[string]uint64{"Reserved High": 0, "File Identifiers": 0, "Direct UDP Callback": 0, "Chat Captcha": 0, "Source Exchange 2": 0, "Transport Obfuscation Required": 0, "Transport Obfuscation Requested": 0, "Transport Obfuscation Supported": 1, "Reserved Mod Bit": 0, "Extended Multipacket": 0, "Large Files": 1, "Kad Version": 4}
			if answer {
				fields["Transport Obfuscation Requested"], fields["Extended Multipacket"] = 1, 1
			}
		}
		if fields != nil {
			edonkeyTestRequireFields(t, tag, fields)
		}
	}
	require.Equal(t, true, edonkeyTestInfo(t, m)["Extension Decoded"])
	require.Nil(t, protocolCorpusFindNode(m, "Extension Data"))
}

func edonkeyTestRequireFields(t *testing.T, tag *base.Node, expected map[string]uint64) {
	t.Helper()
	fields := reflect.ValueOf(edonkeyTestInfo(t, tag)["Fields"])
	require.Equal(t, reflect.Map, fields.Kind())
	require.Equal(t, len(expected), fields.Len())
	for name, value := range expected {
		actual := fields.MapIndex(reflect.ValueOf(name))
		require.True(t, actual.IsValid(), "missing advertised field %s", name)
		require.EqualValues(t, value, actual.Interface(), "field %s", name)
	}
}

func TestProtocolCorpusEDonkeyEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-edonkey.pcap")
	require.Len(t, frames, 17)
	counts := map[byte]int{}
	unique := map[byte][]byte{}
	controls := 0
	expectedOpcode := map[int]byte{4: 1, 5: 0x4c, 6: 0x4c, 7: 0x4c, 8: 1, 10: 0x4c, 11: 0x4c, 13: 0x4c}
	for i, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		public := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		if len(tcp.Payload) == 0 {
			controls++
			require.Zero(t, expectedOpcode[i+1])
			require.Nil(t, protocolCorpusFindNode(public, "EDonkey"), "record %d is transport-only", i+1)
			continue
		}
		require.Equal(t, uint8(5), tcp.DataOffset)
		require.Equal(t, frame[54:], tcp.Payload)
		require.Equal(t, expectedOpcode[i+1], tcp.Payload[5])
		if tcp.Payload[5] == 1 {
			require.Equal(t, uint32(3792028599), tcp.Seq)
			require.Equal(t, uint32(3541588102), tcp.Ack)
		} else {
			require.Equal(t, uint32(3541588102), tcp.Seq)
			require.Equal(t, uint32(3792028723), tcp.Ack)
		}
		counts[tcp.Payload[5]]++
		if previous, ok := unique[tcp.Payload[5]]; ok {
			require.Equal(t, previous, tcp.Payload, "record %d retransmission", i+1)
		} else {
			unique[tcp.Payload[5]] = bytes.Clone(tcp.Payload)
		}
		n := edonkeyTestParse(t, tcp.Payload)
		edonkeyTestCapturedFields(t, n, tcp.Payload[5] == 0x4c, 0)
		edonkeyTestCapturedFields(t, public, tcp.Payload[5] == 0x4c, 54)
	}
	require.Equal(t, 9, controls)
	require.Equal(t, map[byte]int{1: 2, 0x4c: 6}, counts)
	require.Len(t, unique, 2)
}

func edonkeyTestFrame(t *testing.T, payload []byte, port layers.TCPPort) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2)}
	tcp := &layers.TCP{SrcPort: 1754, DstPort: port, Seq: 1, ACK: true, PSH: true, Window: 4096}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(payload)))
	return buffer.Bytes()
}

func edonkeyTestReject(t *testing.T, wire []byte) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), edonkeyTestRule, "EDonkey")
	require.Error(t, err)
}

func TestProtocolCorpusEDonkeyTypedTags(t *testing.T) {
	// CTag's BooleanArray intentionally uses bitCount/8+1, including an extra
	// octet for exact multiples of eight. This is not a generic ceil division.
	tests := []struct {
		name, field string
		kind        byte
		wire        []byte
		value       any
	}{
		{"hash", "Hash Value", 1, []byte("fedcba9876543210"), []byte("fedcba9876543210")},
		{"empty-string", "", 2, []byte{0, 0}, ""},
		{"legacy-string", "String Value", 2, []byte{3, 0, 0xff, 0, 'a'}, string([]byte{0xff, 0, 'a'})},
		{"u32", "Unsigned Value", 3, []byte{255, 255, 255, 255}, uint64(0xffffffff)},
		{"float-one", "Float32 Bits", 4, []byte{0, 0, 0x80, 0x3f}, uint64(0x3f800000)},
		{"float-negative", "Float32 Bits", 4, []byte{0, 0, 0xc0, 0xbf}, uint64(0xbfc00000)},
		{"float-nan", "Float32 Bits", 4, []byte{1, 0, 0xc0, 0x7f}, uint64(0x7fc00001)},
		{"bool-zero", "Boolean Byte", 5, []byte{0}, uint64(0)},
		{"bool-one", "Boolean Byte", 5, []byte{1}, uint64(1)},
		{"bool-reserved", "Boolean Byte", 5, []byte{255}, uint64(255)},
		{"boolarray-zero", "Byte Value", 6, []byte{0, 0, 0xa5}, []byte{0xa5}},
		{"boolarray-seven", "Byte Value", 6, []byte{7, 0, 0xa5}, []byte{0xa5}},
		{"boolarray-eight", "Byte Value", 6, []byte{8, 0, 0xa5, 0x5a}, []byte{0xa5, 0x5a}},
		{"boolarray-nine", "Byte Value", 6, []byte{9, 0, 0xa5, 0x5a}, []byte{0xa5, 0x5a}},
		{"blob-empty", "", 7, []byte{0, 0, 0, 0}, []byte{}},
		{"blob", "Byte Value", 7, []byte{3, 0, 0, 0, 0x80, 0, 0xff}, []byte{0x80, 0, 0xff}},
		{"u16", "Unsigned Value", 8, []byte{255, 255}, uint64(65535)},
		{"u8", "Unsigned Value", 9, []byte{255}, uint64(255)},
		{"u64", "Unsigned Value", 11, bytes.Repeat([]byte{255}, 8), uint64(0xffffffffffffffff)},
	}
	for size := 1; size <= 16; size++ {
		value := bytes.Repeat([]byte{byte(64 + size)}, size)
		tests = append(tests, struct {
			name, field string
			kind        byte
			wire        []byte
			value       any
		}{fmt.Sprintf("short-string-%d", size), "String Value", byte(16 + size), value, string(value)})
	}
	for _, tc := range tests {
		for _, compact := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/compact-%t", tc.name, compact), func(t *testing.T) {
				wire := edonkeyTestWire(false, edonkeyTestTag(tc.kind, 0x79, tc.wire, compact))
				n := edonkeyTestParse(t, wire)
				tag := protocolCorpusFindNode(n, "Tags").Children[0]
				wireType := tc.kind
				if compact {
					wireType |= 0x80
					require.Nil(t, protocolCorpusFindNode(tag, "Name Length"))
				} else {
					protocolCorpusRequireValue(t, tag, "Name Length", uint64(1))
				}
				protocolCorpusRequireValue(t, tag, "Tag Type", uint64(wireType))
				protocolCorpusRequireValue(t, tag, "Name ID", uint64(0x79))
				if tc.field != "" {
					protocolCorpusRequireValue(t, tag, tc.field, tc.value)
				}
				require.EqualValues(t, tc.value, edonkeyTestInfo(t, tag)["Value"])
				// Every missing value byte must be rejected even when the outer
				// length is made self-consistent and the server tail is present.
				for cut := 0; cut < len(tc.wire); cut++ {
					edonkeyTestReject(t, edonkeyTestWire(false, edonkeyTestTag(tc.kind, 0x79, tc.wire[:cut], compact)))
				}
			})
		}
	}
	for _, name := range []string{"", "a", "arbitrary-name"} {
		tag := binary.LittleEndian.AppendUint16([]byte{9}, uint16(len(name)))
		tag = append(tag, name...)
		tag = append(tag, 37)
		n := edonkeyTestParse(t, edonkeyTestWire(true, tag, tag))
		tags := protocolCorpusFindNode(n, "Tags")
		require.Len(t, tags.Children, 2, "duplicate names are preserved")
		for _, item := range tags.Children {
			protocolCorpusRequireValue(t, item, "Unsigned Value", uint64(37))
			protocolCorpusRequireValue(t, item, "Name Length", uint64(len(name)))
			if len(name) > 1 {
				protocolCorpusRequireValue(t, item, "Name", name)
			} else if len(name) == 1 {
				protocolCorpusRequireValue(t, item, "Name ID", uint64(name[0]))
			} else {
				require.Nil(t, protocolCorpusFindNode(item, "Name ID"))
			}
		}
	}
}

func TestProtocolCorpusEDonkeyAdvertisedBitFields(t *testing.T) {
	expected := map[byte]map[string]uint64{
		0xf9: {"Kad Port": 65535, "UDP Port": 65535},
		0xfa: {"AICH Version": 7, "Unicode": 1, "UDP Version": 15, "Compression Version": 15, "Identification Version": 15, "Source Exchange Version": 15, "Extended Requests Version": 15, "Comments Version": 15, "Peer Cache": 1, "No View Shared Files": 1, "Multipacket": 1, "Preview": 1},
		0xfb: {"Compatible Client ID": 255, "Major Version": 127, "Minor Version": 127, "Update Version": 7, "Build Version": 127},
		0xfe: {"Reserved High": 262143, "File Identifiers": 1, "Direct UDP Callback": 1, "Chat Captcha": 1, "Source Exchange 2": 1, "Transport Obfuscation Required": 1, "Transport Obfuscation Requested": 1, "Transport Obfuscation Supported": 1, "Reserved Mod Bit": 1, "Extended Multipacket": 1, "Large Files": 1, "Kad Version": 15},
	}
	for id, values := range expected {
		n := edonkeyTestParse(t, edonkeyTestWire(true, edonkeyTestTag(3, id, bytes.Repeat([]byte{255}, 4), true)))
		edonkeyTestRequireFields(t, protocolCorpusFindNode(n, "Tags").Children[0], values)
	}
	n := edonkeyTestParse(t, edonkeyTestWire(true, edonkeyTestTag(3, 0xfb, []byte{0xd4, 0xc3, 0xb2, 0xa1}, false)))
	edonkeyTestRequireFields(t, protocolCorpusFindNode(n, "Tags").Children[0], map[string]uint64{"Compatible Client ID": 161, "Major Version": 89, "Minor Version": 48, "Update Version": 7, "Build Version": 84})
	// A UINT64 with a known numeric name is not CTag::IsInt(), so it must
	// remain a typed value rather than inheriting the UINT32 interpretation.
	n = edonkeyTestParse(t, edonkeyTestWire(true, edonkeyTestTag(11, 0xfb, bytes.Repeat([]byte{255}, 8), true)))
	require.NotContains(t, edonkeyTestInfo(t, protocolCorpusFindNode(n, "Tags").Children[0]), "Fields")
}

func TestProtocolCorpusEDonkeyMessageBoundariesAndFallback(t *testing.T) {
	one, two := edonkeyTestWire(false), edonkeyTestWire(true)
	unknown := bytes.Clone(two)
	unknown[5] = 0xff
	combined := append(bytes.Clone(one), two...)
	require.Len(t, edonkeyTestMessages(t, edonkeyTestParse(t, combined)), 2)
	for _, port := range []layers.TCPPort{4662, 7551} {
		n := protocolCorpusRequireBoundedRuleParse(t, edonkeyTestFrame(t, combined, port), "ethernet", "Ethernet")
		messages := edonkeyTestMessages(t, n)
		require.Len(t, messages, 2)
		_, first, last := protocolCorpusConsumedRange(messages[1], uint64((54+len(combined))*8))
		require.Equal(t, uint64((54+len(one))*8), first)
		require.Equal(t, uint64((54+len(combined))*8), last)
		for _, payload := range [][]byte{one[:len(one)-1], append(bytes.Clone(one), two[:len(two)-1]...), append(bytes.Clone(one), unknown...), append(bytes.Clone(one), 0xaa)} {
			edonkeyTestReject(t, payload)
			public := protocolCorpusRequireBoundedRuleParse(t, edonkeyTestFrame(t, payload, port), "ethernet", "Ethernet")
			require.Nil(t, protocolCorpusFindNode(public, "EDonkey"))
			protocolCorpusRequireValue(t, public, "Remaining Payload", payload)
		}
		// Preserve the pre-existing default candidates on both added ports.
		http := []byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n")
		public := protocolCorpusRequireBoundedRuleParse(t, edonkeyTestFrame(t, http, port), "ethernet", "Ethernet")
		require.NotNil(t, protocolCorpusFindNode(public, "HTTP"))
		// A legal TLS Application Data record continues through the old path.
		tls := []byte{23, 3, 3, 0, 3, 1, 2, 3}
		public = protocolCorpusRequireBoundedRuleParse(t, edonkeyTestFrame(t, tls, port), "ethernet", "Ethernet")
		protocolCorpusRequireValue(t, public, "ContentType", uint64(23))
		record := protocolCorpusFindNode(public, "Record Layer")
		require.NotNil(t, record)
		protocolCorpusRequireValue(t, record, "Version", uint64(0x0303))
		protocolCorpusRequireValue(t, record, "Length", uint64(3))
		require.Nil(t, protocolCorpusFindNode(public, "EDonkeyProtocolPrefix"), "the probe must always be rolled back")
		for _, marker := range []byte{0xe3, 0xc5, 0xd4} {
			for _, payload := range [][]byte{{marker}, {marker, 0x22, 0, 0, 0}, {marker, 1, 0, 0, 0, 0xff}} {
				edonkeyTestReject(t, payload)
				public = protocolCorpusRequireBoundedRuleParse(t, edonkeyTestFrame(t, payload, port), "ethernet", "Ethernet")
				require.Nil(t, protocolCorpusFindNode(public, "EDonkey"))
				require.Nil(t, protocolCorpusFindNode(public, "EDonkeyProtocolPrefix"))
				require.Nil(t, protocolCorpusFindNode(public, "ContentType"))
				protocolCorpusRequireValue(t, public, "Remaining Payload", payload)
			}
		}
	}
	for _, extension := range [][]byte{{1}, {0x4b, 0x44, 0x4c, 0x4d}, {1, 2, 3, 4, 5}} {
		wire := append(bytes.Clone(one), extension...)
		binary.LittleEndian.PutUint32(wire[1:], uint32(len(wire)-5))
		n := edonkeyTestParse(t, wire)
		protocolCorpusRequireValue(t, n, "Extension Data", extension)
		require.Equal(t, false, edonkeyTestInfo(t, edonkeyTestMessages(t, n)[0])["Extension Decoded"])
	}
}

func TestProtocolCorpusEDonkeyInvalidLengthsAndTypes(t *testing.T) {
	valid := edonkeyTestWire(false, edonkeyTestTag(2, 1, []byte{3, 0, 'a', 'b', 'c'}, false))
	for cut := 5; cut < len(valid); cut++ {
		short := bytes.Clone(valid[:cut])
		binary.LittleEndian.PutUint32(short[1:], uint32(cut-5))
		edonkeyTestReject(t, short)
	}
	for _, size := range []uint32{0, 1, 32, 33, uint32(len(valid) - 6), uint32(len(valid) - 4), 0xffffffff} {
		bad := bytes.Clone(valid)
		binary.LittleEndian.PutUint32(bad[1:], size)
		edonkeyTestReject(t, bad)
	}
	for _, offset := range []int{1, 4} { // NameLength and StringLength within the tag.
		for _, change := range []int{-1, 1} {
			tag := edonkeyTestTag(2, 1, []byte{3, 0, 'a', 'b', 'c'}, false)
			binary.LittleEndian.PutUint16(tag[offset:], uint16(int(binary.LittleEndian.Uint16(tag[offset:]))+change))
			if offset == 4 && change == -1 {
				// A shorter string leaves one byte for the allowed opaque
				// trailer; it is not necessarily invalid wire grammar.
				n := edonkeyTestParse(t, edonkeyTestWire(false, tag))
				protocolCorpusRequireValue(t, n, "String Value", "ab")
				require.Equal(t, false, edonkeyTestInfo(t, edonkeyTestMessages(t, n)[0])["Extension Decoded"])
			} else {
				edonkeyTestReject(t, edonkeyTestWire(false, tag))
			}
		}
	}
	for _, kind := range []byte{0, 10, 12, 16, 33, 127} {
		for _, compact := range []bool{false, true} {
			edonkeyTestReject(t, edonkeyTestWire(true, edonkeyTestTag(kind, 1, nil, compact)))
		}
	}
	for _, pair := range [][2]byte{{0, 0xc5}, {0, 0xd4}, {0, 0}, {5, 0}, {5, 0x15}, {5, 0xff}, {6, 0}, {6, 15}, {6, 17}} {
		bad := bytes.Clone(valid)
		bad[pair[0]] = pair[1]
		edonkeyTestReject(t, bad)
		public := protocolCorpusRequireBoundedRuleParse(t, edonkeyTestFrame(t, bad, 4662), "ethernet", "Ethernet")
		require.Nil(t, protocolCorpusFindNode(public, "EDonkey"))
		if pair != [2]byte{0, 0} {
			protocolCorpusRequireValue(t, public, "Remaining Payload", bad)
		}
	}
	for _, count := range []uint32{2, 257, 0xffffffff} {
		bad := bytes.Clone(valid)
		binary.LittleEndian.PutUint32(bad[29:], count)
		edonkeyTestReject(t, bad)
	}
	for _, tag := range [][]byte{{2, 255, 255}, {0x82, 1, 255, 255}, {0x87, 1, 255, 255, 255, 255}, {0x86, 1, 255, 255}} {
		edonkeyTestReject(t, edonkeyTestWire(false, tag))
	}
	_, err := parser.ParseBinary(bytes.NewReader(valid), edonkeyTestRule, "EDonkey")
	require.ErrorContains(t, err, "message boundary is required")
}

func TestProtocolCorpusEDonkeyResourceLimits(t *testing.T) {
	tag := edonkeyTestTag(9, 1, []byte{7}, true)
	tags := make([][]byte, 257)
	for i := range tags {
		tags[i] = tag
	}
	edonkeyTestParse(t, edonkeyTestWire(false, tags[:256]...))
	edonkeyTestReject(t, edonkeyTestWire(false, tags...))
	edonkeyTestParse(t, append(edonkeyTestWire(false, tags[:128]...), edonkeyTestWire(true, tags[:128]...)...))
	edonkeyTestReject(t, append(edonkeyTestWire(false, tags[:129]...), edonkeyTestWire(true, tags[:128]...)...))
	empty := edonkeyTestWire(true)
	edonkeyTestParse(t, bytes.Repeat(empty, 256))
	edonkeyTestReject(t, bytes.Repeat(empty, 257))
	large := append(edonkeyTestWire(true), bytes.Repeat([]byte{0xa5}, (1<<20)-len(empty))...)
	binary.LittleEndian.PutUint32(large[1:], uint32(len(large)-5))
	edonkeyTestParse(t, large)
	edonkeyTestReject(t, append(large, 1))
}

func TestProtocolCorpusEDonkeyConcurrentIsolation(t *testing.T) {
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			for j := 0; j < 4; j++ {
				wire := edonkeyTestWire(j%2 == 0, edonkeyTestTag(9, byte(i+1), []byte{byte(j + 1)}, true))
				edonkeyTestReject(t, wire[:len(wire)-1])
				n := edonkeyTestParse(t, wire)
				protocolCorpusRequireValue(t, n, "Name ID", uint64(i+1))
				protocolCorpusRequireValue(t, n, "Unsigned Value", uint64(j+1))
			}
		})
	}
}

func TestProtocolCorpusEDonkeyShortPrefixes(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-edonkey.pcap")
	for _, index := range []int{3, 4} {
		wire := frames[index][54:]
		for cut := 0; cut < len(wire); cut++ {
			t.Run(fmt.Sprintf("record-%d/prefix-%d", index+1, cut), func(t *testing.T) {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), edonkeyTestRule, "EDonkey")
				require.Error(t, err)
			})
		}
	}
}
