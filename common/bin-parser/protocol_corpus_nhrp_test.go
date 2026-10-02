package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Independent RFC 2332 wire encoding; this does not use the YAML generator.
func nhrpTestClient() []byte {
	return []byte{0, 32, 0, 0, 5, 220, 1, 44, 4, 0, 4, 7, 198, 51, 100, 2, 192, 0, 2, 2}
}

func nhrpTestSeal(wire []byte) []byte {
	binary.BigEndian.PutUint16(wire[10:12], uint16(len(wire)))
	wire[12], wire[13] = 0, 0
	binary.BigEndian.PutUint16(wire[12:14], dccpTestChecksum(wire))
	return wire
}

func nhrpTestMessage(kind byte, extensions []byte) []byte {
	header := []byte{0, 1, 8, 0, 0, 0, 0, 0, 0, 16, 0, 0, 0, 0, 0, 0, 1, kind, 4, 0}
	mandatory := []byte{4, 4, 0, 0, 0x12, 0x34, 0x56, 0x78, 198, 51, 100, 1, 192, 0, 2, 1, 192, 0, 2, 2}
	if kind == 1 {
		mandatory[2] = 8 // Stable binding permits a nonzero holding time.
	}
	if kind == 7 {
		copy(mandatory[4:8], []byte{0, 7, 0, 10})
		mandatory = append(mandatory, make([]byte, 20)...)
	} else {
		client := nhrpTestClient()
		if kind == 5 || kind == 6 {
			client[4], client[5], client[6], client[7], client[11] = 0, 0, 0, 0, 0
		}
		mandatory = append(mandatory, client...)
	}
	wire := append(header, mandatory...)
	if len(extensions) > 0 {
		binary.BigEndian.PutUint16(wire[14:16], uint16(len(wire)))
		wire = append(wire, extensions...)
	}
	return nhrpTestSeal(wire)
}

func nhrpTestEthernet(wire []byte) []byte {
	ip := []byte{0x45, 0, 0, 0, 0x12, 0x34, 0, 0, 64, 54, 0, 0, 192, 0, 2, 1, 192, 0, 2, 2}
	binary.BigEndian.PutUint16(ip[2:4], uint16(len(ip)+len(wire)))
	binary.BigEndian.PutUint16(ip[10:12], dccpTestChecksum(ip))
	frame := []byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 8, 0}
	return append(append(frame, ip...), wire...)
}

func nhrpTestRequireFields(t *testing.T, node *base.Node, wire []byte, offset int) {
	t.Helper()
	address := protocolCorpusFindNode(node, "Address Family")
	require.NotNil(t, address)
	node = address.Cfg.GetItem(base.CfgParent).(*base.Node)
	for _, f := range []struct {
		name       string
		start, end int
		raw        bool
	}{
		{"Address Family", 0, 2, false}, {"Protocol Type", 2, 4, false}, {"Protocol SNAP", 4, 9, true},
		{"Hop Count", 9, 10, false}, {"Packet Length", 10, 12, false}, {"Checksum", 12, 14, false},
		{"Extension Offset", 14, 16, false}, {"Version", 16, 17, false}, {"Message Type", 17, 18, false},
		{"Source NBMA Type Length", 18, 19, false}, {"Source Subaddress Type Length", 19, 20, false},
		{"Source Protocol Length", 20, 21, false}, {"Destination Protocol Length", 21, 22, false},
		{"Source NBMA Address", 28, 32, true}, {"Source Protocol Address", 32, 36, true}, {"Destination Protocol Address", 36, 40, true},
	} {
		var want any
		if f.raw {
			want = wire[f.start:f.end]
		} else if f.end-f.start == 2 {
			want = uint64(binary.BigEndian.Uint16(wire[f.start:f.end]))
		} else {
			want = uint64(wire[f.start])
		}
		protocolCorpusRequireValue(t, node, f.name, want)
		require.Equal(t, [2]uint64{uint64(offset+f.start) * 8, uint64(offset+f.end) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, f.name)))
	}
	if wire[17] == 7 {
		protocolCorpusRequireValue(t, node, "Unused", uint64(binary.BigEndian.Uint16(wire[22:24])))
		protocolCorpusRequireValue(t, node, "Error Code", uint64(7))
		protocolCorpusRequireValue(t, node, "Error Offset", uint64(10))
		protocolCorpusRequireValue(t, node, "Packet In Error", wire[40:])
		for i, name := range []string{"Unused", "Error Code", "Error Offset"} {
			require.Equal(t, [2]uint64{uint64(offset+22+2*i) * 8, uint64(offset+24+2*i) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, name)))
		}
		require.Equal(t, [2]uint64{uint64(offset+40) * 8, uint64(offset+len(wire)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Packet In Error")))
	} else {
		protocolCorpusRequireValue(t, node, "Flags", uint64(binary.BigEndian.Uint16(wire[22:24])))
		protocolCorpusRequireValue(t, node, "Request ID", uint64(0x12345678))
		clients := protocolCorpusFindNode(node, "Clients")
		for _, f := range []struct {
			name       string
			start, end int
			raw        bool
		}{
			{"Code", 0, 1, false}, {"Prefix Length", 1, 2, false}, {"Unused", 2, 4, false}, {"MTU", 4, 6, false}, {"Holding Time", 6, 8, false},
			{"NBMA Type Length", 8, 9, false}, {"Subaddress Type Length", 9, 10, false}, {"Protocol Length", 10, 11, false}, {"Preference", 11, 12, false},
			{"NBMA Address", 12, 16, true}, {"Protocol Address", 16, 20, true},
		} {
			data := wire[40+f.start : 40+f.end]
			var want any
			if f.raw {
				want = data
			} else if len(data) == 2 {
				want = uint64(binary.BigEndian.Uint16(data))
			} else {
				want = uint64(data[0])
			}
			protocolCorpusRequireValue(t, clients, f.name, want)
			require.Equal(t, [2]uint64{uint64(offset+40+f.start) * 8, uint64(offset+40+f.end) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(clients, f.name)))
		}
	}
}

func TestProtocolCorpusNHRPMessageTypesAndFields(t *testing.T) {
	for kind := byte(1); kind <= 7; kind++ {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			wire := nhrpTestMessage(kind, nil)
			for _, imported := range []bool{false, true} {
				input, rule, entry, offset := wire, "nhrp", "NHRP", 0
				if imported {
					input, rule, entry, offset = nhrpTestEthernet(wire), "ethernet", "Ethernet", 34
				}
				node := protocolCorpusRequireBoundedRuleParse(t, input, rule, entry)
				nhrpTestRequireFields(t, node, wire, offset)
				require.Nil(t, protocolCorpusFindNode(node, "NHRP Payload"))
			}
		})
	}
}

func TestProtocolCorpusNHRPRejectionsAndCarrierTransactions(t *testing.T) {
	original := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-nhrp.pcap")[0]
	require.Len(t, original, 54)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(original[34:]), "nhrp", "NHRP")
	require.ErrorContains(t, err, "nhrp: packet size outside 28..65535 bytes")
	valid := nhrpTestMessage(2, nil)
	invalid := [][]byte{original[34:]}
	for cut := 1; cut < len(valid); cut++ {
		invalid = append(invalid, valid[:cut])
	}
	for _, mutation := range []struct {
		offset int
		value  byte
		seal   bool
	}{
		{11, 0, false}, {16, 0, true}, {17, 0, true}, {17, 8, true}, {18, 0x84, true}, {19, 0x80, true}, {15, 20, true}, {15, 59, true},
		{48, 0x84, true}, {49, 0x80, true}, {50, 255, true}, {28, 123, false},
	} {
		wire := bytes.Clone(valid)
		wire[mutation.offset] = mutation.value
		if mutation.seal {
			nhrpTestSeal(wire)
		}
		invalid = append(invalid, wire)
	}
	for index, wire := range invalid {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "nhrp", "NHRP")
		require.Error(t, err, "invalid %d", index)
		frame := nhrpTestEthernet(wire)
		node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		protocolCorpusRequireValue(t, node, "NHRP Payload", wire)
		require.Nil(t, protocolCorpusFindNode(node, "Address Family"))
		require.Equal(t, [2]uint64{34 * 8, uint64(len(frame)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "NHRP Payload")))
	}
	for _, wire := range [][]byte{valid, original[34:], valid[:1], invalid[len(invalid)-1]} {
		frame := nhrpTestEthernet(wire)
		root, err := base.ParseRule("ethernet.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
		reader := bytes.NewReader(frame)
		bits := base.NewBitReader(reader)
		require.NoError(t, root.ParseSubNode(bits, "Ethernet"))
		require.Zero(t, reader.Len())
		require.ErrorContains(t, bits.Recovery(), "no backup")
		require.ErrorContains(t, bits.PopBackup(), "no backup")
		_, err = bits.ReadBits(8)
		require.ErrorIs(t, err, io.EOF)
	}
}

func nhrpTestExtension(kind uint16, value []byte) []byte {
	data := make([]byte, 4, len(value)+4)
	binary.BigEndian.PutUint16(data, kind)
	binary.BigEndian.PutUint16(data[2:], uint16(len(value)))
	return append(data, value...)
}

func TestProtocolCorpusNHRPCompanionEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-nhrp-valid.pcap")
	require.Len(t, frames, 7)
	for i, frame := range frames {
		t.Run(fmt.Sprint(i+1), func(t *testing.T) {
			var extensions []byte
			if i == 1 {
				responder := nhrpTestClient()
				responder[1] = 0 // Responder prefix is ignored on receipt, zero on transmit.
				extensions = append(nhrpTestExtension(0x8003, responder), 0x80, 0, 0, 0)
			}
			expected := nhrpTestMessage(byte(i+1), extensions)
			require.Equal(t, expected, frame[34:])
			for _, imported := range []bool{false, true} {
				wire, rule, entry, offset := expected, "nhrp", "NHRP", 0
				if imported {
					wire, rule, entry, offset = frame, "ethernet", "Ethernet", 34
				}
				node := protocolCorpusRequireBoundedRuleParse(t, wire, rule, entry)
				nhrpTestRequireFields(t, node, expected, offset)
				if i == 1 {
					ext := protocolCorpusFindNode(node, "Extensions")
					protocolCorpusRequireValue(t, ext, "Compulsory", uint64(1))
					protocolCorpusRequireValue(t, ext, "Type", uint64(3))
					protocolCorpusRequireValue(t, ext, "Length", uint64(20))
					protocolCorpusRequireValue(t, ext, "NBMA Address", []byte{198, 51, 100, 2})
					require.Equal(t, [2]uint64{uint64(offset+76) * 8, uint64(offset+80) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(ext, "NBMA Address")))
				}
			}
		})
	}
}

func TestProtocolCorpusNHRPExtensionsAndAddressLengths(t *testing.T) {
	end := []byte{0x80, 0, 0, 0}
	var exts []byte
	for _, kind := range []uint16{3, 4, 5} {
		exts = append(exts, nhrpTestExtension(0x8000|kind, nhrpTestClient())...)
	}
	exts = append(exts, nhrpTestExtension(0x3800, []byte{1, 0xfe, 3})...)
	exts = append(exts, end...)
	wire := nhrpTestMessage(2, exts)
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "nhrp", "NHRP")
	protocolCorpusRequireValue(t, node, "Opaque Value", []byte{1, 0xfe, 3})
	// Undefined optional/compulsory extensions are retained, never treated as
	// interpreted data or used to authorize a routing-state transition.
	for _, kind := range []uint16{0x3800, 0xb800} {
		x := append(nhrpTestExtension(kind, []byte{0xaa, 0xbb}), end...)
		protocolCorpusRequireValue(t, protocolCorpusRequireBoundedRuleParse(t, nhrpTestMessage(2, x), "nhrp", "NHRP"), "Opaque Value", []byte{0xaa, 0xbb})
	}
	for _, bad := range [][]byte{
		{0x80, 0, 0, 1, 0}, {0x80, 0, 0}, append(bytes.Clone(end), 0),
		nhrpTestExtension(0x8003, nhrpTestClient()),
		append(append(nhrpTestExtension(0x8004, nil), nhrpTestExtension(0x8004, nil)...), end...),
		append(nhrpTestExtension(0x8003, append(nhrpTestClient(), nhrpTestClient()...)), end...),
		append(nhrpTestExtension(0x8003, nhrpTestClient()[:19]), end...),
		{0x38, 1, 0xff, 0xff, 0x80, 0, 0, 0},
	} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(nhrpTestMessage(2, bad)), "nhrp", "NHRP")
		require.Error(t, err)
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(nhrpTestMessage(7, end)), "nhrp", "NHRP")
	require.ErrorContains(t, err, "error indication cannot contain extensions")
	// Exercise every length-bearing address with nonuniform bytes, including
	// empty addresses and the 6-bit/8-bit field maxima. Unused/SNAP bits must
	// be retained and ignored on receipt, not normalized or invented.
	for _, length := range []int{0, 1, 4, 63} {
		header := bytes.Clone(nhrpTestMessage(1, nil)[:20])
		header[18], header[19] = byte(length), byte(length)
		copy(header[4:9], []byte{1, 2, 3, 4, 5})
		mandatory := []byte{255, 255, 0xa5, 0x5a, 1, 2, 3, 4}
		values := [][]byte{bytes.Repeat([]byte{0xa1}, length), bytes.Repeat([]byte{0xb2}, length), bytes.Repeat([]byte{0xc3}, 255), bytes.Repeat([]byte{0xd4}, 255)}
		for _, value := range values {
			mandatory = append(mandatory, value...)
		}
		wire := nhrpTestSeal(append(header, mandatory...))
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "nhrp", "NHRP")
		protocolCorpusRequireValue(t, node, "Flags", uint64(0xa55a))
		protocolCorpusRequireValue(t, node, "Protocol SNAP", []byte{1, 2, 3, 4, 5})
		for i, name := range []string{"Source NBMA Address", "Source NBMA Subaddress", "Source Protocol Address", "Destination Protocol Address"} {
			if len(values[i]) > 0 {
				protocolCorpusRequireValue(t, node, name, values[i])
			}
		}
	}
}

func TestProtocolCorpusNHRPResourceBounds(t *testing.T) {
	for _, count := range []int{4096, 4097} {
		t.Run(fmt.Sprintf("clients-%d", count), func(t *testing.T) {
			wire := nhrpTestMessage(2, nil)[:40]
			wire = append(wire, bytes.Repeat([]byte{0, 32, 0, 0, 5, 220, 1, 44, 0, 0, 0, 7}, count)...)
			nhrpTestSeal(wire)
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "nhrp", "NHRP")
			if count > 4096 {
				require.ErrorContains(t, err, "too many client entries")
				return
			}
			require.NoError(t, err)
			require.Equal(t, wire, NodeToBytes(node))
			clients := protocolCorpusFindNode(node, "Clients")
			value, err := clients.Result()
			require.NoError(t, err)
			require.Len(t, value.Children(), count)
		})
	}
	for _, count := range []int{1024, 1025} {
		t.Run(fmt.Sprintf("extensions-%d", count), func(t *testing.T) {
			var ext []byte
			for i := 0; i < count-1; i++ {
				ext = append(ext, nhrpTestExtension(uint16(0x3800+i), nil)...)
			}
			ext = append(ext, 0x80, 0, 0, 0)
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(nhrpTestMessage(2, ext)), "nhrp", "NHRP")
			if count > 1024 {
				require.ErrorContains(t, err, "too many extensions")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestProtocolCorpusNHRPRequestClientCounts(t *testing.T) {
	for _, tc := range []struct {
		kind      byte
		count     int
		flags     uint16
		prefix    byte
		wantError string
	}{
		{1, 0, 0, 0, ""}, {1, 1, 0x0800, 32, ""}, {1, 2, 0, 32, "at most one client"},
		{2, 0, 0, 32, "at least one client"}, {4, 0, 0, 32, "at least one client"}, {6, 0, 0, 32, "at least one client"},
		{2, 1, 0x1000, 32, ""}, {2, 2, 0x1000, 32, "unique reply requires one client"},
		{3, 0, 0, 32, "at least one client"}, {3, 2, 0, 32, ""},
		{5, 0, 0, 32, "at least one client"}, {5, 2, 0, 32, ""},
		{1, 1, 0x1800, 255, ""}, {1, 1, 0x1800, 32, "host prefix"},
		{3, 1, 0x8000, 255, ""}, {3, 1, 0x8000, 32, "host prefix"}, {3, 2, 0x8000, 255, "one client"},
	} {
		t.Run(fmt.Sprintf("%d-%d-%x-%d", tc.kind, tc.count, tc.flags, tc.prefix), func(t *testing.T) {
			wire := nhrpTestMessage(tc.kind, nil)[:40]
			binary.BigEndian.PutUint16(wire[22:24], tc.flags)
			client := nhrpTestClient()
			client[1] = tc.prefix
			wire = nhrpTestSeal(append(wire, bytes.Repeat(client, tc.count)...))
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "nhrp", "NHRP")
			if tc.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantError)
			}
		})
	}
}

func TestProtocolCorpusNHRPBoundariesAndParallel(t *testing.T) {
	for _, entry := range []string{"NHRP", "NHRPCarrier"} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(nil), "nhrp", entry)
		require.Error(t, err)
		_, err = parser.ParseBinary(bytes.NewReader(nhrpTestMessage(2, nil)), "nhrp", entry)
		require.ErrorContains(t, err, "explicit")
	}
	for i := 0; i < 12; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			for round := 0; round < 3; round++ {
				wire := nhrpTestMessage(byte(1+(i+round)%7), nil)
				wire[9] = byte(i + round + 1)
				nhrpTestSeal(wire)
				node := protocolCorpusRequireBoundedRuleParse(t, nhrpTestEthernet(wire), "ethernet", "Ethernet")
				nhrpTestRequireFields(t, node, wire, 34)
			}
		})
	}
}

func TestProtocolCorpusNHRPPacketSizeLimit(t *testing.T) {
	// Odd-length full packet exercises the RFC checksum's final zero octet.
	wire := nhrpTestMessage(7, nil)[:40]
	wire = append(wire, bytes.Repeat([]byte{0xa5}, 65535-len(wire))...)
	copy(wire[22:24], []byte{0x5a, 0xc3}) // Retain receiver-ignored unused bits.
	nhrpTestSeal(wire)
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "nhrp", "NHRP")
	nhrpTestRequireFields(t, node, wire, 0)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(bytes.Clone(wire), 0)), "nhrp", "NHRP")
	require.ErrorContains(t, err, "nhrp: packet size outside 28..65535 bytes")
	wire[len(wire)-1] ^= 1
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "nhrp", "NHRP")
	require.ErrorContains(t, err, "checksum mismatch")
}
