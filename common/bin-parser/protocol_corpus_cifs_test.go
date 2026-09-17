package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// Independent MS-CIFS 2.2.4.52.1 request construction, not parser round-tripping.
func cifsTestRequest(names ...string) []byte {
	wire := make([]byte, 35)
	copy(wire, []byte{0xff, 'S', 'M', 'B', 0x72})
	wire[9] = 0x18
	binary.LittleEndian.PutUint16(wire[10:12], 0xc853)
	binary.LittleEndian.PutUint16(wire[26:28], 0x1234)
	binary.LittleEndian.PutUint16(wire[30:32], 7)
	for _, name := range names {
		wire = append(wire, 2)
		wire = append(wire, name...)
		wire = append(wire, 0)
	}
	binary.LittleEndian.PutUint16(wire[33:35], uint16(len(wire)-35))
	return wire
}

func cifsTestDirect(message []byte) []byte {
	wire := make([]byte, 4, len(message)+4)
	binary.BigEndian.PutUint32(wire, uint32(len(message)))
	return append(wire, message...)
}

func TestProtocolCorpusCIFSNegotiateRequestFields(t *testing.T) {
	for _, names := range [][]string{{"NT LM 0.12"}, {"PC NETWORK PROGRAM 1.0", "NT LM 0.12"}, {"", "OEM-\x80\xfe"}} {
		message := cifsTestRequest(names...)
		for _, direct := range []bool{false, true} {
			wire, entry, offset := message, "CIFSNegotiateRequest", uint64(0)
			if direct {
				wire, entry, offset = cifsTestDirect(message), "CIFSDirectTCP", 32
			}
			n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.cifs", entry)
			require.Equal(t, wire, NodeToBytes(n))
			miopTestField(t, n, "ProtocolId", uint32(0x424d53ff), offset, offset+32)
			miopTestField(t, n, "Command", uint8(0x72), offset+32, offset+40)
			miopTestField(t, n, "Flags", uint8(0x18), offset+72, offset+80)
			miopTestField(t, n, "Flags2", uint16(0xc853), offset+80, offset+96)
			miopTestField(t, n, "PID", uint16(0x1234), offset+208, offset+224)
			miopTestField(t, n, "MID", uint16(7), offset+240, offset+256)
			miopTestField(t, n, "WordCount", uint8(0), offset+256, offset+264)
			miopTestField(t, n, "ByteCount", uint16(len(message)-35), offset+264, offset+280)
			dialects := protocolCorpusFindNode(n, "Dialects")
			require.NotNil(t, dialects)
			require.Len(t, dialects.Children, len(names))
			pos := offset + 280
			for i, name := range names {
				miopTestField(t, dialects.Children[i], "BufferFormat", uint8(2), pos, pos+8)
				miopTestField(t, dialects.Children[i], "Dialect Name", name, pos+8, pos+uint64(len(name)+1)*8)
				pos += uint64(len(name)+2) * 8
			}
			require.EqualValues(t, len(names), alljoynTestInfo(t, dialects)["Dialect Count"])
		}
	}
}

func TestProtocolCorpusCIFSCompanionEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-validated/gen-cifs-valid.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "d41b7ed920693490a691d4338e16c0fd49ba38cba0cbad98c85f7e847c1647f3", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 4)
	for i, frame := range frames {
		require.Equal(t, byte(6), frame[23])
		start := 14 + int(frame[14]&15)*4
		start += int(frame[start+12]>>4) * 4
		if i != 3 {
			require.Len(t, frame, start)
			continue
		}
		wire := frame[start:]
		require.Equal(t, cifsTestDirect(cifsTestRequest("PC NETWORK PROGRAM 1.0", "NT LM 0.12")), wire)
		n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.cifs", "CIFSDirectTCP")
		require.Equal(t, wire, NodeToBytes(n))
		protocolCorpusRequireValue(t, n, "Direct TCP Length", uint64(len(wire)-4))
		protocolCorpusRequireValue(t, n, "ByteCount", uint64(36))
		protocolCorpusRequireValue(t, n, "Dialect Name", "PC NETWORK PROGRAM 1.0")
	}
}

func TestProtocolCorpusCIFSExplicitBoundsAndOffsets(t *testing.T) {
	message := cifsTestRequest("NT LM 0.12")
	for _, entry := range []string{"CIFSNegotiateRequest", "CIFSDirectTCP", "CIFSDirectTCPCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(message), "application-layer.cifs", entry)
		require.ErrorContains(t, err, "explicit byte boundary required")
	}
	for _, count := range []int{1024, 1025} {
		names := make([]string, count)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(cifsTestRequest(names...)), "application-layer.cifs", "CIFSNegotiateRequest")
		if count == 1024 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "dialect count exceeds implementation profile")
		}
	}
	// The declared byte count permits one large OEM name; no guessed codepage.
	large := cifsTestRequest(string(bytes.Repeat([]byte{0x81}, 65533)))
	n := protocolCorpusRequireBoundedRuleParse(t, large, "application-layer.cifs", "CIFSNegotiateRequest")
	require.Equal(t, large, NodeToBytes(n))
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(large, 0)), "application-layer.cifs", "CIFSNegotiateRequest")
	require.ErrorContains(t, err, "exceeds implementation profile")
	for offset := uint64(0); offset < 8; offset++ {
		var packed bytes.Buffer
		writer := base.NewBitWriter(&packed)
		if offset > 0 {
			require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
		}
		wire := cifsTestDirect(message)
		require.NoError(t, writer.WriteBits(wire, uint64(len(wire))*8))
		require.NoError(t, writer.WriteBits([]byte{0xa5}, 8))
		require.NoError(t, writer.WriteBits([]byte{0}, 8-offset))
		var doc yaml.MapSlice
		source := fmt.Sprintf("Package:\n  Frame:\n    Prefix: raw,%dbit\n    Record:\n      import: application-layer/cifs.yaml\n      node: CIFSDirectTCP\n      length: %d\n    Suffix: uint8\n", offset, len(wire)*8)
		if offset == 0 {
			source = fmt.Sprintf("Package:\n  Frame:\n    Record:\n      import: application-layer/cifs.yaml\n      node: CIFSDirectTCP\n      length: %d\n    Suffix: uint8\n", len(wire)*8)
		}
		require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
		root, err := base.NewNodeTree(doc)
		require.NoError(t, err)
		reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
		require.NoError(t, root.ParseSubNode(reader, "Frame"))
		node := base.GetNodeByPath(root, "@Frame")
		miopTestField(t, node, "Command", uint8(0x72), offset+64, offset+72)
		miopTestField(t, node, "Dialect Name", "NT LM 0.12", offset+320, offset+400)
		protocolCorpusRequireValue(t, node, "Suffix", uint64(0xa5))
		require.Equal(t, offset+uint64(len(wire))*8+8, stream_parser.CalcNodeConsumedLength(node))
		require.ErrorContains(t, reader.Recovery(), "no backup")
	}
}

func TestProtocolCorpusCIFSOriginalAndMalformedRequests(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-cifs.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "26a7d42824ea4e5987030caff39abddad361d29e2b43e5e3aeb2bde9d16528bc", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 4)
	for i, frame := range frames {
		require.Equal(t, byte(6), frame[23])
		start := 14 + int(frame[14]&15)*4
		start += int(frame[start+12]>>4) * 4
		if i != 3 {
			require.Len(t, frame, start)
			continue
		}
		wire := frame[start:]
		require.EqualValues(t, len(wire)-4, binary.BigEndian.Uint32(wire[:4]))
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.cifs", "CIFSDirectTCP")
		require.ErrorContains(t, err, "cifs: dialect ByteCount does not equal bounded data")
		n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.cifs", "CIFSDirectTCPCarrier")
		protocolCorpusRequireValue(t, n, "Unparsed CIFS Direct TCP Record", wire)
		require.Equal(t, wire, NodeToBytes(n))
		require.Nil(t, protocolCorpusFindNode(n, "Command"))
	}
	valid := cifsTestDirect(cifsTestRequest("NT LM 0.12"))
	invalid := [][]byte{append(bytes.Clone(valid), 0), cifsTestDirect(cifsTestRequest())}
	for _, change := range []struct {
		offset int
		value  byte
	}{{0, 1}, {3, 1}, {4, 0}, {8, 0x73}, {13, 0x98}, {36, 1}, {37, 1}, {39, 3}, {len(valid) - 1, 1}} {
		wire := bytes.Clone(valid)
		wire[change.offset] = change.value
		invalid = append(invalid, wire)
	}
	for cut := 0; cut < len(valid); cut++ {
		invalid = append(invalid, valid[:cut])
	}
	for i, wire := range invalid {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.cifs", "CIFSDirectTCP")
		require.Error(t, err, "case %d", i)
		if len(wire) > 0 {
			n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.cifs", "CIFSDirectTCPCarrier")
			protocolCorpusRequireValue(t, n, "Unparsed CIFS Direct TCP Record", wire)
			require.Nil(t, protocolCorpusFindNode(n, "ProtocolId"))
			require.Equal(t, wire, NodeToBytes(n))
		}
	}
	// Preserve a parent's unread suffix on both direct success and raw fallback.
	for _, wire := range [][]byte{valid, invalid[0]} {
		var doc yaml.MapSlice
		require.NoError(t, yaml.Unmarshal([]byte(fmt.Sprintf("Package:\n  Record:\n    import: application-layer/cifs.yaml\n    node: CIFSDirectTCPCarrier\n    length: %d\n", len(wire)*8)), &doc))
		root, err := base.NewNodeTree(doc)
		require.NoError(t, err)
		reader := base.NewBitReader(bytes.NewReader(append(bytes.Clone(wire), 0x7e, 0x51)))
		require.NoError(t, root.ParseSubNode(reader, "Record"))
		n := base.GetNodeByPath(root, "@Record")
		require.Equal(t, wire, NodeToBytes(n))
		suffix, err := reader.ReadBits(16)
		require.NoError(t, err)
		require.Equal(t, []byte{0x7e, 0x51}, suffix)
		require.ErrorContains(t, reader.Recovery(), "no backup")
	}
}
