package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// RFC 4340, sections 5 and 9. The fixture writer is independent of the rule.
// https://www.rfc-editor.org/rfc/rfc4340.html#section-5
func dccpTestPacket(kind byte, extended bool, options, payload []byte) []byte {
	header := make([]byte, 12)
	if extended {
		header = make([]byte, 16)
	}
	binary.BigEndian.PutUint16(header, 4000)
	binary.BigEndian.PutUint16(header[2:], 80)
	header[5], header[8] = 0xa0, kind<<1
	if extended {
		header[8] |= 1
		copy(header[10:], []byte{1, 2, 3, 4, 5, 6})
	} else {
		copy(header[9:], []byte{4, 5, 6})
	}
	if kind != 0 && kind != 2 {
		if extended {
			header = append(header, 0, 0, 6, 5, 4, 3, 2, 1)
		} else {
			header = append(header, 0, 3, 2, 1)
		}
	}
	if kind == 0 || kind == 1 {
		header = append(header, 't', 'e', 's', 't')
	}
	if kind == 7 {
		header = append(header, 5, 41, 0xab, 0xcd)
	}
	header = append(header, options...)
	for len(header)%4 != 0 {
		header = append(header, 0)
	}
	header[4] = byte(len(header) / 4)
	return append(header, payload...)
}

func dccpTestChecksum(data []byte) uint16 {
	var sum uint32
	for i := 0; i < len(data); i += 2 {
		sum += uint32(data[i]) << 8
		if i+1 < len(data) {
			sum += uint32(data[i+1])
		}
	}
	for sum > 65535 {
		sum = sum&65535 + sum>>16
	}
	return ^uint16(sum)
}

// Bit-at-a-time Castagnoli oracle, independent of hash/crc32 tables.
func dccpTestCRC32C(data []byte) uint32 {
	crc := ^uint32(0)
	for _, b := range data {
		crc ^= uint32(b)
		for bit := 0; bit < 8; bit++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0x82f63b78
			} else {
				crc >>= 1
			}
		}
	}
	return ^crc
}

func dccpTestIPv4HeaderChecksum(packet []byte) {
	packet[10], packet[11] = 0, 0
	binary.BigEndian.PutUint16(packet[10:], dccpTestChecksum(packet[:int(packet[0]&15)*4]))
}

func dccpTestIP(wire []byte, v6 bool) []byte {
	wire = append([]byte(nil), wire...)
	wire[6], wire[7] = 0, 0
	var ip, pseudo []byte
	if v6 {
		ip = make([]byte, 40)
		ip[0], ip[6], ip[7] = 0x60, 33, 64
		binary.BigEndian.PutUint16(ip[4:], uint16(len(wire)))
		copy(ip[8:24], []byte{0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
		copy(ip[24:40], ip[8:24])
		ip[39] = 2
		pseudo = append(append([]byte(nil), ip[8:40]...), 0, 0, byte(len(wire)>>8), byte(len(wire)), 0, 0, 0, 33)
	} else {
		ip = make([]byte, 20)
		ip[0], ip[8], ip[9] = 0x45, 64, 33
		binary.BigEndian.PutUint16(ip[2:], uint16(20+len(wire)))
		copy(ip[12:20], []byte{192, 0, 2, 1, 192, 0, 2, 2})
		binary.BigEndian.PutUint16(ip[10:], dccpTestChecksum(ip))
		pseudo = append(append([]byte(nil), ip[12:20]...), 0, 33, byte(len(wire)>>8), byte(len(wire)))
	}
	covered := len(wire)
	if c := int(wire[5] & 15); c > 0 {
		covered = int(wire[4])*4 + (c-1)*4
	}
	binary.BigEndian.PutUint16(wire[6:], dccpTestChecksum(append(pseudo, wire[:covered]...)))
	return append(ip, wire...)
}

func dccpTestInfo(t *testing.T, node *base.Node, key string) any {
	t.Helper()
	values, ok := node.Cfg.GetItem("additionInfo").(map[string]any)
	require.True(t, ok)
	return values[key]
}

func TestProtocolCorpusDCCPAllBasicTypes(t *testing.T) {
	for kind := byte(0); kind <= 9; kind++ {
		for _, extended := range []bool{false, true} {
			if !extended && (kind < 2 || kind > 4) {
				continue
			}
			t.Run(fmt.Sprintf("type-%d/extended-%v", kind, extended), func(t *testing.T) {
				wire := dccpTestPacket(kind, extended, nil, []byte{0xab, 0xcd, 0xef})
				for _, version := range []int{0, 4, 6} {
					input, rule, entry := wire, "dccp", "DCCP"
					if version == 4 {
						input, rule, entry = dccpTestIP(wire, false), "internet_protocol", "Internet Protocol"
					}
					if version == 6 {
						input, rule, entry = dccpTestIP(wire, true), "internet_protocol_version_6", "Internet Protocol Version 6"
					}
					node := protocolCorpusRequireBoundedRuleParse(t, input, rule, entry)
					dccp := protocolCorpusFindNode(node, "DCCP")
					require.NotNil(t, dccp)
					for field, value := range map[string]uint64{"Source Port": 4000, "Destination Port": 80, "Packet Type": uint64(kind), "CCVal": 10, "Checksum Coverage": 0, "Data Offset": uint64(wire[4])} {
						protocolCorpusRequireValue(t, dccp, field, value)
					}
					seq, ack := uint64(0x040506), uint64(0x030201)
					if extended {
						seq, ack = 0x010203040506, 0x060504030201
					}
					protocolCorpusRequireValue(t, dccp, "Sequence Number", seq)
					if kind != 0 && kind != 2 {
						protocolCorpusRequireValue(t, dccp, "Acknowledgement Number", ack)
					} else {
						require.Nil(t, protocolCorpusFindNode(dccp, "Acknowledgement Number"))
					}
					if kind == 0 || kind == 1 {
						protocolCorpusRequireValue(t, dccp, "Service Code", uint64(0x74657374))
					}
					if kind == 7 {
						for field, value := range map[string]uint64{"Reset Code": 5, "Reset Data 1": 41, "Reset Data 2": 0xab, "Reset Data 3": 0xcd} {
							protocolCorpusRequireValue(t, dccp, field, value)
						}
					}
					protocolCorpusRequireValue(t, dccp, "Application Data", []byte{0xab, 0xcd, 0xef})
					require.Equal(t, version != 0, dccpTestInfo(t, dccp, "Checksum Validated"))
					require.Equal(t, false, dccpTestInfo(t, dccp, "Connection State Validated"))
				}
				// Header-sized datagrams cannot accept any strict prefix.
				wire = dccpTestPacket(kind, extended, []byte{41, 6, 1, 2, 3, 4}, nil)
				for end := 0; end < len(wire); end++ {
					_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:end]), "dccp", "DCCP")
					require.Error(t, err, "prefix %d of %d", end, len(wire))
				}
			})
		}
	}
}

func TestProtocolCorpusDCCPOptionsAndBounds(t *testing.T) {
	options := []byte{0, 1, 32, 4, 1, 2, 33, 3, 1, 34, 4, 2, 3, 35, 4, 2, 3, 36, 4, 0xaa, 0xbb, 37, 5, 1, 2, 3, 38, 5, 0x00, 0x43, 0xc2, 39, 3, 0x80, 40, 3, 0x01, 41, 6, 1, 2, 3, 4, 42, 10, 5, 6, 7, 8, 9, 10, 11, 12, 43, 4, 1, 2, 44, 6, 0x12, 0x34, 0x56, 0x78, 255, 4, 0xaa, 0xbb}
	crc := dccpTestCRC32C([]byte("payload"))
	binary.BigEndian.PutUint32(options[bytes.Index(options, []byte{44, 6})+2:], crc)
	wire := dccpTestPacket(4, true, options, []byte("payload"))
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "dccp", "DCCP")
	for field, value := range map[string]uint64{"Feature Number": 1, "NDP Count": 0x010203, "State": 0, "Run Length": 0, "Timestamp": 0x01020304, "Timestamp Echo": 0x05060708, "Elapsed Time": 0x090a0b0c, "Data Checksum": uint64(crc)} {
		protocolCorpusRequireValue(t, node, field, value)
	}
	protocolCorpusRequireValue(t, node, "Feature Values", []byte{2})
	protocolCorpusRequireValue(t, node, "Application Data", []byte("payload"))
	// RFC 4340 5.8 ignores an invalid option and all remaining option bytes.
	for _, optionBytes := range [][]byte{{0, 0, 0, 255}, {255, 0, 41, 6}, {255, 1, 41, 6}, {255, 255, 41, 6}} {
		wire = dccpTestPacket(4, true, optionBytes, []byte("payload"))
		node = protocolCorpusRequireBoundedRuleParse(t, wire, "dccp", "DCCP")
		opts := protocolCorpusFindNode(node, "Options")
		last := opts.Children[len(opts.Children)-1]
		require.Equal(t, true, dccpTestInfo(t, last, "Malformed Length"))
		protocolCorpusRequireValue(t, node, "Application Data", []byte("payload"))
		require.Nil(t, protocolCorpusFindNode(node, "Timestamp"))
	}
	for _, optionBytes := range [][]byte{{0, 0, 0, 1}, {1, 1, 0, 0}} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(dccpTestPacket(3, true, optionBytes, nil)), "dccp", "DCCP")
		require.ErrorContains(t, err, "Mandatory")
		protocolCorpusRequireBoundedRuleParse(t, dccpTestPacket(2, true, optionBytes, nil), "dccp", "DCCP")
	}
	// Maximum Data Offset leaves exactly 1008 one-byte options at X=0.
	wire = dccpTestPacket(2, false, make([]byte, 1008), nil)
	node = protocolCorpusRequireBoundedRuleParse(t, wire, "dccp", "DCCP")
	require.Len(t, protocolCorpusFindNode(node, "Options").Children, 1008)
	for _, mutate := range []func([]byte){func(b []byte) { b[4] = 2 }, func(b []byte) { b[4] = 255 }, func(b []byte) { b[8] = 20 }, func(b []byte) { b[8] = 0 }, func(b []byte) { b[5] = 15 }} {
		bad := dccpTestPacket(2, false, nil, nil)
		mutate(bad)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "dccp", "DCCP")
		require.Error(t, err)
	}
	for _, size := range []int{65535, 65536} {
		wire = dccpTestPacket(2, false, nil, make([]byte, size-12))
		if size == 65535 {
			protocolCorpusRequireBoundedRuleParse(t, wire, "dccp", "DCCP")
		} else {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "dccp", "DCCP")
			require.ErrorContains(t, err, "outside bounds")
		}
	}
}

func TestProtocolCorpusDCCPChecksumsAndReserved(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		rule, entry, iplen := "internet_protocol", "Internet Protocol", 20
		if v6 {
			rule, entry, iplen = "internet_protocol_version_6", "Internet Protocol Version 6", 40
		}
		for coverage := byte(0); coverage <= 15; coverage++ {
			wire := dccpTestPacket(4, true, []byte{41, 6, 1, 2, 3, 4}, bytes.Repeat([]byte{0xab}, 57))
			wire[5] = 0x90 | coverage
			// Reserved bits are retained, not rejected (RFC 4340 3.1).
			wire[8] |= 0xe0
			wire[9] = 0xff
			wire[16], wire[17] = 0xfe, 0xfd
			ip := dccpTestIP(wire, v6)
			node := protocolCorpusRequireBoundedRuleParse(t, ip, rule, entry)
			protocolCorpusRequireValue(t, node, "Reserved", uint64(7))
			protocolCorpusRequireValue(t, node, "Sequence Reserved", uint64(255))
			protocolCorpusRequireValue(t, node, "Acknowledgement Reserved", uint64(0xfefd))
			bad := append([]byte(nil), ip...)
			bad[iplen+10] ^= 1
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), rule, entry)
			require.ErrorContains(t, err, "checksum mismatch")
			bad = append([]byte(nil), ip...)
			bad[len(bad)-1] ^= 1
			if coverage == 0 {
				_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(bad), rule, entry)
				require.ErrorContains(t, err, "checksum mismatch")
			} else {
				protocolCorpusRequireBoundedRuleParse(t, bad, rule, entry)
			}
		}
	}
}

func TestProtocolCorpusDCCPOriginalRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-dccp.pcap")
	require.Len(t, frames, 1)
	require.Len(t, frames[0], 50)
	require.Equal(t, byte(33), frames[0][23])
	require.Equal(t, byte(32), frames[0][38])
	for _, target := range []struct {
		wire        []byte
		rule, entry string
	}{{frames[0][34:], "dccp", "DCCP"}, {frames[0][14:], "internet_protocol", "Internet Protocol"}, {frames[0], "ethernet", "Ethernet"}} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(target.wire), target.rule, target.entry)
		require.ErrorContains(t, err, "requires extended sequence numbers")
	}
}

func TestProtocolCorpusDCCPDataCRCAndInvalidHeader(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("123456789"), bytes.Repeat([]byte{0xab}, 65500)} {
		option := []byte{44, 6, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(option[2:], dccpTestCRC32C(data))
		wire := dccpTestPacket(2, false, append(bytes.Clone(option), option...), data)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "dccp", "DCCP")
		require.Equal(t, true, dccpTestInfo(t, node, "Data Checksum Validated"))
		require.Equal(t, 2, dccpTestInfo(t, node, "Data Checksums Verified"))
		wire[20] ^= 1
		node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "dccp", "DCCP")
		require.ErrorContains(t, err, "CRC32c mismatch")
		require.Nil(t, node)
	}
	for _, options := range [][]byte{{255, 1, 41, 6}, {0, 0, 0, 255}, {41, 6, 0, 0, 0, 1}, {0, 0, 0, 1}} {
		ip := dccpTestIP(dccpTestPacket(4, true, options, []byte("sample")), false)
		ip[26] ^= 1 // Header checksum, not option or application bytes.
		node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(ip), "internet_protocol", "Internet Protocol")
		require.Error(t, err)
		require.Nil(t, node)
	}
}

func TestProtocolCorpusDCCPCompanionEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-dccp-valid.pcap")
	require.Len(t, frames, 13)
	seen := make(map[string]bool)
	for _, frame := range frames {
		kind, extended := (frame[42]>>1)&15, frame[42]&1 != 0
		key := fmt.Sprintf("%d/%v", kind, extended)
		require.False(t, seen[key])
		seen[key] = true
		want := dccpTestPacket(kind, extended, []byte{41, 6, 1, 2, 3, 4}, []byte("sample"))
		wantIP := dccpTestIP(want, false)
		require.Equal(t, wantIP[20:], frame[34:])
		for _, target := range []struct {
			wire        []byte
			rule, entry string
		}{{frame[34:], "dccp", "DCCP"}, {frame, "ethernet", "Ethernet"}} {
			node := protocolCorpusRequireBoundedRuleParse(t, target.wire, target.rule, target.entry)
			protocolCorpusRequireValue(t, node, "Packet Type", uint64(kind))
			protocolCorpusRequireValue(t, node, "Timestamp", uint64(0x01020304))
			protocolCorpusRequireValue(t, node, "Application Data", []byte("sample"))
		}
		for end := 14; end < len(frame); end++ {
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(frame[:end]), "ethernet", "Ethernet")
			require.Error(t, err, "%s prefix %d", key, end)
			require.Nil(t, node)
		}
	}
}

func TestProtocolCorpusDCCPIPOptionsAndFragments(t *testing.T) {
	ip := dccpTestIP(dccpTestPacket(2, false, nil, []byte("sample")), false)
	withOptions := append(bytes.Clone(ip[:20]), append([]byte{1, 1, 0, 0}, ip[20:]...)...)
	withOptions[0] = 0x46
	binary.BigEndian.PutUint16(withOptions[2:], uint16(len(withOptions)))
	withOptions[10], withOptions[11] = 0, 0
	binary.BigEndian.PutUint16(withOptions[10:], dccpTestChecksum(withOptions[:24]))
	node := protocolCorpusRequireBoundedRuleParse(t, withOptions, "internet_protocol", "Internet Protocol")
	protocolCorpusRequireValue(t, node, "IP Header Options", []byte{1, 1, 0, 0})
	protocolCorpusRequireValue(t, node, "Application Data", []byte("sample"))
	for _, fragment := range []uint16{1, 0x2000, 0x2001} {
		bad := bytes.Clone(ip)
		binary.BigEndian.PutUint16(bad[6:], fragment)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), "internet_protocol", "Internet Protocol")
		require.ErrorContains(t, err, "require caller reassembly")
	}
	ip6 := dccpTestIP(dccpTestPacket(2, false, nil, []byte("sample")), true)
	for _, protocol := range []byte{0, 60, 44} {
		extension := []byte{33, 0, 0, 0, 0, 0, 0, 0}
		withExtension := append(bytes.Clone(ip6[:40]), append(extension, ip6[40:]...)...)
		withExtension[6] = protocol
		binary.BigEndian.PutUint16(withExtension[4:], uint16(len(withExtension)-40))
		node := protocolCorpusRequireBoundedRuleParse(t, withExtension, "internet_protocol_version_6", "Internet Protocol Version 6")
		protocolCorpusRequireValue(t, node, "Application Data", []byte("sample"))
	}
}

func TestProtocolCorpusDCCPSourceRouteChecksums(t *testing.T) {
	wire := dccpTestPacket(2, false, nil, []byte("sample"))
	ip := dccpTestIP(wire, false)
	for _, kind := range []byte{131, 137} {
		option := append([]byte{kind, 7, 4}, ip[16:20]...)
		option = append(option, 0)
		routed := append(bytes.Clone(ip[:20]), append(option, ip[20:]...)...)
		routed[0], routed[19] = 0x47, 9
		binary.BigEndian.PutUint16(routed[2:], uint16(len(routed)))
		dccpTestIPv4HeaderChecksum(routed)
		require.Zero(t, dccpTestChecksum(routed[:28]))
		node := protocolCorpusRequireBoundedRuleParse(t, routed, "internet_protocol", "Internet Protocol")
		require.Equal(t, true, dccpTestInfo(t, protocolCorpusFindNode(node, "DCCP"), "Checksum Validated"))
		badRoute := bytes.Clone(routed)
		badRoute[26] ^= 1
		dccpTestIPv4HeaderChecksum(badRoute)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(badRoute), "internet_protocol", "Internet Protocol")
		require.ErrorContains(t, err, "checksum mismatch")
		// Once the pointer passes the route, base Destination is the endpoint.
		routed[22], routed[19] = 8, 2
		routed[26] = 99 // The recorded old hop is no longer the pseudo destination.
		dccpTestIPv4HeaderChecksum(routed)
		require.Zero(t, dccpTestChecksum(routed[:28]))
		protocolCorpusRequireBoundedRuleParse(t, routed, "internet_protocol", "Internet Protocol")
	}
	ip6 := dccpTestIP(wire, true)
	for _, kind := range []byte{0, 2, 99} {
		route := append([]byte{33, 2, kind, 1, 0, 0, 0, 0}, ip6[24:40]...)
		routed := append(bytes.Clone(ip6[:40]), append(route, ip6[40:]...)...)
		routed[6], routed[39] = 43, 9
		binary.BigEndian.PutUint16(routed[4:], uint16(len(routed)-40))
		node := protocolCorpusRequireBoundedRuleParse(t, routed, "internet_protocol_version_6", "Internet Protocol Version 6")
		dccp := protocolCorpusFindNode(node, "DCCP")
		require.Equal(t, kind != 99, dccpTestInfo(t, dccp, "Checksum Validated"))
		require.Equal(t, kind == 99, dccpTestInfo(t, dccp, "Checksum Context Required"))
		if kind != 99 {
			badRoute := bytes.Clone(routed)
			badRoute[63] ^= 1
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(badRoute), "internet_protocol_version_6", "Internet Protocol Version 6")
			require.ErrorContains(t, err, "checksum mismatch")
		}
	}
}

func TestProtocolCorpusDCCPOptionValueLayouts(t *testing.T) {
	for _, kind := range []byte{32, 33, 34, 35} {
		node := protocolCorpusRequireBoundedRuleParse(t, dccpTestPacket(4, true, []byte{kind, 3, 1}, nil), "dccp", "DCCP")
		option := protocolCorpusFindNode(node, "Options").Children[0]
		require.Equal(t, kind == 33 || kind == 35, dccpTestInfo(t, option, "Value Decoded"))
		if kind == 32 || kind == 34 {
			require.Nil(t, protocolCorpusFindNode(option, "Feature Number"))
		}
	}
	data := []byte{40, 6, 0, 0xa0, 3, 0xa2}
	node := protocolCorpusRequireBoundedRuleParse(t, dccpTestPacket(4, true, data, nil), "dccp", "DCCP")
	blocks := protocolCorpusFindNode(node, "Data Dropped").Children
	require.Len(t, blocks, 4)
	for i, run := range []uint64{0, 0, 3, 2} {
		protocolCorpusRequireValue(t, blocks[i], "Drop Run Length", run)
		if i%2 == 1 {
			protocolCorpusRequireValue(t, blocks[i], "Drop Code", uint64(2))
		} else {
			require.Nil(t, protocolCorpusFindNode(blocks[i], "Drop Code"))
		}
	}
	for _, kind := range []byte{37, 41, 42, 43, 44, 128, 255} {
		for size := 2; size <= 10; size++ {
			option := make([]byte, size)
			option[0], option[1] = kind, byte(size)
			node := protocolCorpusRequireBoundedRuleParse(t, dccpTestPacket(4, true, option, nil), "dccp", "DCCP")
			decoded := kind == 37 && size >= 3 && size <= 8 || kind == 41 && size == 6 || kind == 42 && (size == 6 || size == 8 || size == 10) || kind == 43 && (size == 4 || size == 6) || kind == 44 && size == 6
			require.Equal(t, decoded, dccpTestInfo(t, protocolCorpusFindNode(node, "Options").Children[0], "Value Decoded"), "kind=%d size=%d", kind, size)
		}
	}
}
