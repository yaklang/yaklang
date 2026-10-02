package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// RFC 3320 header layouts and RFC 4077 NACK v1. These vectors verify framing,
// not successful state lookup, uploaded program execution or decompression.
var sigcompTestLiterals = []string{
	"f9010203040506aabb",
	"fe7f010203040506070809cc",
	"ff8210200102030405060708090a0b0cdd",
	"f8001223ee",
	"f8000101000000000102030405060708090a0b0c0d0e0f10111213010203040506",
}

func sigcompTestFields(t *testing.T, node *base.Node, wire []byte, offset uint64) {
	t.Helper()
	field := func(name string, want any, start, end int) {
		miopTestField(t, node, name, want, offset+uint64(start), offset+uint64(end))
	}
	field("Signature", uint8(31), 0, 5)
	field("Feedback Present", uint8((wire[0]>>2)&1), 5, 6)
	selector := wire[0] & 3
	field("State ID Length Selector", selector, 6, 8)
	pos := 1
	if wire[0]&4 != 0 {
		field("Feedback Length Form", wire[pos]>>7, pos*8, pos*8+1)
		field("Feedback Value or Length", wire[pos]&127, pos*8+1, pos*8+8)
		n := 0
		if wire[pos]&128 != 0 {
			n = int(wire[pos] & 127)
		}
		pos++
		if n > 0 {
			field("Feedback Data", wire[pos:pos+n], pos*8, (pos+n)*8)
		}
		pos += n
	}
	nack := false
	if selector != 0 {
		n := 3 + 3*int(selector)
		field("Partial State Identifier", wire[pos:pos+n], pos*8, (pos+n)*8)
		pos += n
	} else {
		length := int(wire[pos])<<4 | int(wire[pos+1]>>4)
		field("Code Length", uint16(length), pos*8, pos*8+12)
		field("Destination or NACK Version", wire[pos+1]&15, pos*8+12, pos*8+16)
		pos += 2
		if length != 0 {
			field("Uploaded UDVM Bytecode", wire[pos:pos+length], pos*8, (pos+length)*8)
			pos += length
		} else {
			nack = true
			field("NACK Reason Code", wire[pos], pos*8, pos*8+8)
			field("Failed Opcode", wire[pos+1], pos*8+8, pos*8+16)
			field("Failed Program Counter", uint16(wire[pos+2])<<8|uint16(wire[pos+3]), pos*8+16, pos*8+32)
			pos += 4
			field("Failed Message SHA1", wire[pos:pos+20], pos*8, pos*8+160)
			pos += 20
			if pos < len(wire) {
				field("NACK Error Details", wire[pos:], pos*8, len(wire)*8)
			}
			pos = len(wire)
		}
	}
	if pos < len(wire) {
		field("Compressed Data", wire[pos:], pos*8, len(wire)*8)
	}
	message := protocolCorpusFindNode(node, "Header").Cfg.GetItem(base.CfgParent).(*base.Node)
	info := alljoynTestInfo(t, message)
	require.Equal(t, nack, info["NACK Fields Decoded"])
	for _, name := range []string{"State Resolved", "UDVM Executed", "Application Decompressed", "NACK Details Semantics Decoded", "Failed Message Hash Verified", "Stream Record Unquoting Performed"} {
		require.Equal(t, false, info[name], name)
	}
}

func TestProtocolCorpusSIGCOMPFieldsAndBoundaryFailures(t *testing.T) {
	for _, literal := range sigcompTestLiterals {
		wire := alljoynTestHex(t, literal)
		for _, entry := range []string{"SIGCOMP", "SIGCOMPCarrier"} {
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.sigcomp", entry)
			require.Equal(t, wire, NodeToBytes(node))
			sigcompTestFields(t, node, wire, 0)
		}
	}
	for _, invalid := range []string{"00", "f800", "f0001123", "ff7f00", "ff8210", "ff800001", "f80010ff", "f8002100", "f80001", "f80002"} {
		wire := alljoynTestHex(t, invalid)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.sigcomp", "SIGCOMP")
		require.Error(t, err, invalid)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.sigcomp", "SIGCOMPCarrier")
		require.Equal(t, wire, NodeToBytes(node))
		require.Nil(t, protocolCorpusFindNode(node, "Signature"))
		miopTestField(t, node, "Unparsed SIGCOMP Payload", wire, 0, uint64(len(wire))*8)
	}
	// Every feedback length and partial-ID selector is independently delimited.
	for length := 0; length < 128; length++ {
		for selector := 1; selector <= 3; selector++ {
			wire := append([]byte{0xfc | byte(selector), 0x80 | byte(length)}, bytes.Repeat([]byte{0xa1}, length)...)
			wire = append(wire, bytes.Repeat([]byte{0xb2}, 3+3*selector)...)
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.sigcomp", "SIGCOMP")
			sigcompTestFields(t, node, wire, 0)
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:len(wire)-1]), "application-layer.sigcomp", "SIGCOMP")
			require.Error(t, err)
		}
	}
	for _, size := range []int{1, 15, 16, 255, 256, 4095} {
		for destination := 1; destination <= 15; destination++ {
			wire := append([]byte{0xf8, byte(size >> 4), byte(size<<4) | byte(destination)}, bytes.Repeat([]byte{0}, size)...)
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.sigcomp", "SIGCOMP")
			sigcompTestFields(t, node, wire, 0)
			message := protocolCorpusFindNode(node, "Header").Cfg.GetItem(base.CfgParent).(*base.Node)
			require.EqualValues(t, (destination+1)*64, alljoynTestInfo(t, message)["Bytecode Destination Address"])
		}
	}
}

func TestProtocolCorpusSIGCOMPOriginalAndCompanionEveryRecord(t *testing.T) {
	const original = "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-sigcomp.pcap"
	data, err := os.ReadFile(original)
	require.NoError(t, err)
	require.Equal(t, "0395d5e4950c47c0c09cdc6f1dfdc6722237200685f0e3b2c0d1d7f212a95b9e", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, original)
	require.Len(t, frames, 1)
	require.Equal(t, append([]byte{0xff, 0xff}, make([]byte, 8)...), frames[0][42:])
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(frames[0][42:]), "application-layer.sigcomp", "SIGCOMP")
	require.ErrorContains(t, err, "sigcomp: feedback exceeds message boundary")
	frames = protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-sigcomp-valid.pcap")
	require.Len(t, frames, len(sigcompTestLiterals))
	for index, frame := range frames {
		wire := alljoynTestHex(t, sigcompTestLiterals[index])
		require.Equal(t, wire, frame[42:])
		root := fcoeTestInline(t, fmt.Sprintf("Package:\n  Envelope:\n    Headers: raw,42\n    Body:\n      import: application-layer/sigcomp.yaml\n      node: SIGCOMP\n      length: %d\n", len(wire)*8))
		root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
		require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(frame)), "Envelope"))
		node := base.GetNodeByPath(root, "@Envelope")
		require.Equal(t, frame, NodeToBytes(node))
		sigcompTestFields(t, node, wire, 42*8)
	}
}
