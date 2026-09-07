package bin_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func TestProtocolCorpusRARPRequestAndReply(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-rarp-valid.pcap")
	require.Len(t, frames, 2)
	for index, frame := range frames {
		// gopacket does not dispatch EtherType 0x8035. Check that discriminator
		// independently, then use its shared ARP address decoder for the body.
		require.Equal(t, uint16(0x8035), binary.BigEndian.Uint16(frame[12:14]))
		packet := gopacket.NewPacket(frame[14:], layers.LayerTypeARP, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		arp := packet.Layer(layers.LayerTypeARP).(*layers.ARP)
		node := protocolCorpusRequireBoundedRuleParse(t, frame, "rarp", "RARPFrame")
		protocolCorpusRequireValue(t, node, "Destination", frame[:6])
		protocolCorpusRequireValue(t, node, "Source", frame[6:12])
		protocolCorpusRequireValue(t, node, "EtherType", uint64(0x8035))
		for field, want := range map[string]uint64{"Hardware type": uint64(arp.AddrType), "Protocol type": uint64(arp.Protocol), "Hardware size": uint64(arp.HwAddressSize), "Protocol size": uint64(arp.ProtAddressSize), "Opcode": uint64(3 + index)} {
			protocolCorpusRequireValue(t, node, field, want)
		}
		for field, want := range map[string][]byte{"Sender MAC address": arp.SourceHwAddress, "Sender IP address": arp.SourceProtAddress, "Target MAC address": arp.DstHwAddress, "Target IP address": arp.DstProtAddress} {
			protocolCorpusRequireValue(t, node, field, want)
		}
		protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		for _, cut := range []int{0, 13, 14, 21, len(frame) - 1} {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(frame[:cut]), "rarp", "RARPFrame")
			require.Error(t, err)
		}
	}
}

func TestProtocolCorpus6to4And6in4AreDistinctEvidence(t *testing.T) {
	for _, tc := range []struct {
		path   string
		sixto4 bool
	}{{"generated-local/gen-6to4.pcap", false}, {"generated-validated/gen-6to4-valid.pcap", true}} {
		frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/"+tc.path)
		require.Len(t, frames, 1)
		data := frames[0][14:]
		node := protocolCorpusRequireBoundedRuleParse(t, data, "internet_protocol", "Internet Protocol")
		protocolCorpusRequireValue(t, node, "Protocol", uint64(41))
		inner := protocolCorpusFindNode(node, "IPv6")
		require.NotNil(t, inner)
		protocolCorpusIPv6Header(t, inner, data[20:])
		matchesPrefix := bytes.Equal(data[28:30], []byte{0x20, 2}) && bytes.Equal(data[44:46], []byte{0x20, 2})
		require.Equal(t, tc.sixto4, matchesPrefix)
		if tc.sixto4 {
			require.Equal(t, data[12:16], data[30:34], "embedded source IPv4")
			require.Equal(t, data[16:20], data[46:50], "embedded destination IPv4")
		}
		protocolCorpusRequireValue(t, protocolCorpusFindNode(inner, "ICMPv6"), "Type", uint64(128))
	}
}

func TestProtocolCorpusAoEEveryQueryConfigField(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-aoe-valid.pcap")
	require.Len(t, frames, 2)
	for _, frame := range frames {
		b := frame[14:]
		node := protocolCorpusRequireBoundedRuleParse(t, b, "aoe", "AoE")
		for field, value := range map[string]uint64{"Version": uint64(b[0] >> 4), "Flags": uint64(b[0] & 15), "Error": uint64(b[1]), "Major": uint64(binary.BigEndian.Uint16(b[2:4])), "Minor": uint64(b[4]), "Command": uint64(b[5]), "Tag": uint64(binary.BigEndian.Uint32(b[6:10])), "Buffer Count": uint64(binary.BigEndian.Uint16(b[10:12])), "Firmware": uint64(binary.BigEndian.Uint16(b[12:14])), "Sector Count": uint64(b[14]), "Config Version": uint64(b[15] >> 4), "Config Command": uint64(b[15] & 15), "Config Length": uint64(binary.BigEndian.Uint16(b[16:18]))} {
			protocolCorpusRequireValue(t, node, field, value)
		}
		if len(b) > 18 {
			protocolCorpusRequireValue(t, node, "Config String", string(b[18:]))
		}
		protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		for cut := 0; cut < len(b); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(b[:cut]), "aoe", "AoE")
			require.Error(t, err, "AoE truncation at %d", cut)
		}
	}
}

func TestProtocolCorpusCLDAPSearchEveryField(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-cldap-valid.pcap")
	require.Len(t, frames, 1)
	packet := gopacket.NewPacket(frames[0], layers.LayerTypeEthernet, gopacket.Default)
	require.Nil(t, packet.ErrorLayer())
	input := packet.Layer(layers.LayerTypeUDP).(*layers.UDP).Payload
	outer := protocolCorpusBERFields(t, input)
	require.Len(t, outer, 1)
	message := protocolCorpusBERFields(t, outer[0].value)
	require.Len(t, message, 2)
	encode := func(tag byte, b []byte) []byte {
		if len(b) < 128 {
			return append([]byte{tag, byte(len(b))}, b...)
		}
		require.Less(t, len(b), 256)
		return append([]byte{tag, 0x81, byte(len(b))}, b...)
	}
	user := bytes.Repeat([]byte("sample"), 22)
	rfcBody := append(encode(2, message[0].value), encode(4, user)...)
	rfcBody = append(rfcBody, encode(0x63, message[1].value)...)
	for _, data := range [][]byte{input, encode(0x30, rfcBody)} {
		node := protocolCorpusRequireBoundedRuleParse(t, data, "application-layer.cldap", "CLDAP")
		protocolCorpusRequireValue(t, node, "Sequence Tag", uint64(0x30))
		protocolCorpusRequireValue(t, protocolCorpusFindNode(node, "Message ID"), "Value", uint64(1))
		if !bytes.Equal(data, input) {
			protocolCorpusRequireValue(t, node, "User", string(user))
		}
		fields := protocolCorpusBERFields(t, message[1].value)
		require.Len(t, fields, 8)
		base := protocolCorpusFindNode(node, "Base Object")
		protocolCorpusRequireValue(t, base, "Length", uint64(len(fields[0].value)))
		if len(fields[0].value) != 0 {
			protocolCorpusRequireValue(t, base, "Value", string(fields[0].value))
		}
		for i, name := range []string{"Scope", "Alias Mode", "Size Limit", "Time Limit", "Types Only"} {
			field := protocolCorpusFindNode(node, name)
			protocolCorpusRequireValue(t, field, "Tag", uint64(fields[i+1].tag))
			protocolCorpusRequireValue(t, field, "Value", protocolCorpusBERUnsigned(t, fields[i+1].value))
		}
		protocolCorpusRequireValue(t, node, "Present Attribute", string(fields[6].value))
		attributes := protocolCorpusBERFields(t, fields[7].value)
		parsed := protocolCorpusNodesNamed(node, "Attribute")
		require.Len(t, parsed, len(attributes))
		for i, attribute := range parsed {
			protocolCorpusRequireValue(t, attribute, "Value", string(attributes[i].value))
		}
		for _, cut := range []int{0, 1, 4, len(data) / 2, len(data) - 1} {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(data[:cut]), "application-layer.cldap", "CLDAP")
			require.Error(t, err)
		}
	}
}
