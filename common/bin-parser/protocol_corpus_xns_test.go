package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math/bits"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Independently encode Xerox XSIS 028112 section 3.4. Each word contributes
// its rotation by the distance to the end, summed using one's complement;
// the YAML computes the equivalent iterative add-and-rotate recurrence.
func xnsTestChecksum(wire []byte) uint16 {
	var sum uint32
	for offset := 2; offset < len(wire); offset += 2 {
		word := binary.BigEndian.Uint16(wire[offset : offset+2])
		sum += uint32(bits.RotateLeft16(word, (len(wire)-offset)/2))
		sum = (sum & 0xffff) + (sum >> 16)
	}
	if sum == 0xffff {
		return 0
	}
	return uint16(sum)
}

func xnsTestDatagram(kind byte, payload []byte, checksummed bool) []byte {
	wire := []byte{
		0xff, 0xff, 0, 0, 0x0b, kind,
		0x12, 0x34, 0x56, 0x78, 2, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 4, 0x51,
		0x90, 0xab, 0xcd, 0xef, 2, 0x13, 0x24, 0x35, 0x46, 0x57, 0x8a, 0xce,
	}
	wire = append(wire, payload...)
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)))
	if len(wire)%2 != 0 {
		wire = append(wire, 0xa7) // Real wire octet, deliberately not zero.
	}
	if checksummed {
		binary.BigEndian.PutUint16(wire, xnsTestChecksum(wire))
	}
	return wire
}

func xnsTestEthernet(wire []byte) []byte {
	return append([]byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 6, 0}, wire...)
}

func xnsTestRequireFields(t *testing.T, node *base.Node, wire []byte, offset int) {
	t.Helper()
	checksum := protocolCorpusFindNode(node, "Checksum")
	require.NotNil(t, checksum)
	node = checksum.Cfg.GetItem(base.CfgParent).(*base.Node)
	for _, f := range []struct {
		name       string
		start, end int
		raw        bool
	}{
		{"Checksum", 0, 2, false}, {"Datagram Length", 2, 4, false}, {"Packet Type", 5, 6, false},
		{"Destination Network", 6, 10, false}, {"Destination Host", 10, 16, true}, {"Destination Socket", 16, 18, false},
		{"Source Network", 18, 22, false}, {"Source Host", 22, 28, true}, {"Source Socket", 28, 30, false},
	} {
		var want any
		value := wire[f.start:f.end]
		if f.raw {
			want = value
		} else {
			var number uint64
			for _, b := range value {
				number = number<<8 | uint64(b)
			}
			want = number
		}
		protocolCorpusRequireValue(t, node, f.name, want)
		require.Equal(t, [2]uint64{uint64(offset+f.start) * 8, uint64(offset+f.end) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, f.name)))
	}
	protocolCorpusRequireValue(t, node, "Reserved", uint64(wire[4]>>4))
	protocolCorpusRequireValue(t, node, "Hop Count", uint64(wire[4]&15))
	require.Equal(t, [2]uint64{uint64(offset)*8 + 32, uint64(offset)*8 + 36}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Reserved")))
	require.Equal(t, [2]uint64{uint64(offset)*8 + 36, uint64(offset)*8 + 40}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Hop Count")))
	length := int(binary.BigEndian.Uint16(wire[2:4]))
	if wire[5] == 2 {
		protocolCorpusRequireValue(t, node, "Echo Operation", uint64(binary.BigEndian.Uint16(wire[30:32])))
		require.Equal(t, [2]uint64{uint64(offset+30) * 8, uint64(offset+32) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Echo Operation")))
		if length > 32 {
			protocolCorpusRequireValue(t, node, "Echo Data", wire[32:length])
			require.Equal(t, [2]uint64{uint64(offset+32) * 8, uint64(offset+length) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Echo Data")))
		}
	} else if length > 30 {
		protocolCorpusRequireValue(t, node, "IDP Data", wire[30:length])
		require.Equal(t, [2]uint64{uint64(offset+30) * 8, uint64(offset+length) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "IDP Data")))
	}
	garbage := protocolCorpusFindNode(node, "Garbage Byte")
	if length%2 != 0 {
		protocolCorpusRequireValue(t, node, "Garbage Byte", uint64(wire[length]))
		require.Equal(t, [2]uint64{uint64(offset+length) * 8, uint64(offset+length+1) * 8}, stream_parser.GetNodeResultPos(garbage))
	} else if garbage != nil {
		require.False(t, stream_parser.NodeHasResult(garbage))
	}
	info := node.Cfg.GetItem("additionInfo").(map[string]any)
	checked := binary.BigEndian.Uint16(wire[:2]) != 0xffff
	require.Equal(t, checked, info["Checksum Present"])
	require.Equal(t, checked, info["Checksum Validated"])
	require.Equal(t, wire[5] == 2, info["Echo Decoded"])
	require.Equal(t, uint64(len(wire))*8, stream_parser.CalcNodeConsumedLength(node))
}

func TestProtocolCorpusXNSFieldsAndChecksum(t *testing.T) {
	for _, kind := range []byte{0, 1, 2, 3, 4, 5, 16, 31, 255} {
		for _, checked := range []bool{false, true} {
			for _, odd := range []bool{false, true} {
				t.Run(fmt.Sprintf("type-%d/checksum-%t/odd-%t", kind, checked, odd), func(t *testing.T) {
					payload := []byte{0, 1, 0xde, 0xad, 0x17, 0x80}
					if odd {
						payload = append(payload, 0x39)
					}
					wire := xnsTestDatagram(kind, payload, checked)
					for _, imported := range []bool{false, true} {
						input, rule, entry, offset := wire, "xns", "XNS", 0
						if imported {
							input, rule, entry, offset = xnsTestEthernet(wire), "ethernet", "Ethernet", 14
						}
						node := protocolCorpusRequireBoundedRuleParse(t, input, rule, entry)
						xnsTestRequireFields(t, node, wire, offset)
						require.Nil(t, protocolCorpusFindNode(node, "XNS Payload"))
					}
				})
			}
		}
	}
	// Sender-reserved bits are retained; neither they nor socket values imply
	// a receiving-host policy. A hop count of fifteen remains representable.
	wire := xnsTestDatagram(2, []byte{0, 2}, true)
	wire[4] = 0xdf
	binary.BigEndian.PutUint16(wire, xnsTestChecksum(wire))
	xnsTestRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, wire, "xns", "XNS"), wire, 0)
	// A fixed independently calculated nonuniform vector includes a nonzero
	// garbage octet; this catches zero-padding and ordinary-IP-checksum bugs.
	vector, err := hex.DecodeString("541100250b021234567802aabbccddee045190abcdef0213243546578ace0001dead178039a7")
	require.NoError(t, err)
	require.Equal(t, uint16(0x5411), xnsTestChecksum(vector))
	require.Equal(t, vector, xnsTestDatagram(2, []byte{0, 1, 0xde, 0xad, 0x17, 0x80, 0x39}, true))
	xnsTestRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, vector, "xns", "XNS"), vector, 0)
	// The computed one's-complement minus zero is transmitted as 0000;
	// unlike ffff, it remains a present checksum and must be validated.
	zero := xnsTestDatagram(255, []byte{0, 0}, false)
	binary.BigEndian.PutUint16(zero[30:32], bits.RotateLeft16(^xnsTestChecksum(zero), -1))
	require.Zero(t, xnsTestChecksum(zero))
	binary.BigEndian.PutUint16(zero, 0)
	xnsTestRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, zero, "xns", "XNS"), zero, 0)
	zero[31] ^= 1
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(zero), "xns", "XNS")
	require.ErrorContains(t, err, "xns: checksum mismatch")
}

func TestProtocolCorpusXNSOriginalAndCarrierTransactions(t *testing.T) {
	original := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-xns.pcap")[0]
	require.Equal(t, xnsTestEthernet(make([]byte, 16)), original)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(original[14:]), "xns", "XNS")
	require.ErrorContains(t, err, "xns: datagram wire size must be 30 through 65536 bytes")
	valid := xnsTestDatagram(2, []byte{0, 1, 0xde, 0xad, 0x17, 0x80, 0x39}, true)
	invalid := [][]byte{original[14:]}
	for cut := 1; cut < len(valid); cut++ {
		invalid = append(invalid, bytes.Clone(valid[:cut]))
	}
	for _, mutation := range []struct {
		offset int
		value  byte
	}{
		{0, 0}, {2, 0xff}, {3, 29}, {3, 38}, {31, 3}, {32, 0}, {37, 0},
	} {
		wire := bytes.Clone(valid)
		wire[mutation.offset] = mutation.value
		invalid = append(invalid, wire)
	}
	invalid = append(invalid, xnsTestDatagram(2, nil, false), xnsTestDatagram(2, []byte{0}, false), xnsTestDatagram(2, []byte{0, 3}, false))
	for i, wire := range invalid {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "xns", "XNS")
		require.Error(t, err, "invalid %d", i)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "xns", "XNSCarrier")
		protocolCorpusRequireValue(t, node, "XNS Payload", wire)
		require.Nil(t, protocolCorpusFindNode(node, "Destination Network"))
		require.Equal(t, [2]uint64{0, uint64(len(wire)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "XNS Payload")))
		frame := xnsTestEthernet(wire)
		imported := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		protocolCorpusRequireValue(t, imported, "XNS Payload", wire)
		require.Nil(t, protocolCorpusFindNode(imported, "Destination Network"))
		require.Equal(t, [2]uint64{14 * 8, uint64(len(frame)) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(imported, "XNS Payload")))
	}
	for _, wire := range [][]byte{valid, original[14:], valid[:1], invalid[len(invalid)-1], append(bytes.Clone(valid), 0x61, 0x93)} {
		root, err := base.ParseRule("xns.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
		reader := bytes.NewReader(wire)
		readerBits := base.NewBitReader(reader)
		require.NoError(t, root.ParseSubNode(readerBits, "XNSCarrier"))
		require.Zero(t, reader.Len())
		require.ErrorContains(t, readerBits.Recovery(), "no backup")
		require.ErrorContains(t, readerBits.PopBackup(), "no backup")
		_, err = readerBits.ReadBits(8)
		require.ErrorIs(t, err, io.EOF)
	}
	trailer := []byte{0x61, 0x93}
	node := protocolCorpusRequireBoundedRuleParse(t, append(bytes.Clone(valid), trailer...), "xns", "XNSCarrier")
	protocolCorpusRequireValue(t, node, "XNS Link Trailer", trailer)
	xnsTestRequireFields(t, node, valid, 0)
}

func TestProtocolCorpusXNSResourceBounds(t *testing.T) {
	for _, length := range []int{30, 31, 32, 576, 577, 65534, 65535} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			payload := make([]byte, length-30)
			for i := range payload {
				payload[i] = byte(i*29 + 7)
			}
			wire := xnsTestDatagram(255, payload, length != 65534)
			xnsTestRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, wire, "xns", "XNS"), wire, 0)
		})
	}
	for _, wire := range [][]byte{nil, make([]byte, 29), make([]byte, 65537)} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "xns", "XNS")
		require.ErrorContains(t, err, "xns: datagram wire size must be 30 through 65536 bytes")
	}
	for _, entry := range []string{"XNS", "XNSCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(xnsTestDatagram(0, nil, false)), "xns", entry)
		require.ErrorContains(t, err, "explicit")
	}
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(nil), "xns", "XNSCarrier")
	require.ErrorContains(t, err, "xns: empty carrier has no datagram")
}

func TestProtocolCorpusXNSCompanionEveryRecord(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-xns-valid.pcap")
	require.Len(t, frames, 1)
	expected, err := hex.DecodeString("0200000000020200000000010600541100250b021234567802aabbccddee045190abcdef0213243546578ace0001dead178039a7")
	require.NoError(t, err)
	require.Equal(t, expected, frames[0])
	for _, imported := range []bool{false, true} {
		input, rule, entry, offset := frames[0][14:], "xns", "XNS", 0
		if imported {
			input, rule, entry, offset = frames[0], "ethernet", "Ethernet", 14
		}
		node := protocolCorpusRequireBoundedRuleParse(t, input, rule, entry)
		xnsTestRequireFields(t, node, expected[14:], offset)
		require.Nil(t, protocolCorpusFindNode(node, "XNS Payload"))
	}
}

func TestProtocolCorpusXNSParallelIsolation(t *testing.T) {
	for i := 0; i < 12; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			payload := []byte{0, byte(1 + i%2), byte(0x91 + i), 0x7e, 0x03}
			for repeat := 0; repeat < 2; repeat++ {
				wire := xnsTestDatagram(2, payload, i%2 == 0)
				xnsTestRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, wire, "xns", "XNSCarrier"), wire, 0)
			}
		})
	}
}
