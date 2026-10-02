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
)

// Independently serialized historical draft-ipsec-swipe-01 field layouts.
// Ciphertext and authenticator bytes are placeholders, never validated data.
var swipeTestLiterals = []string{
	"0001000145000017010200003f110000c000020ac6336414646174",
	"0103123401020304deadbeef45000017010200003f110000c000020ac63364146461740000",
	"0202123411223344aabbccdd",
	"030312341122334455667788aabbccddeeff0011",
	"10010001010203",
	"3f021234aabbccdd",
}

func swipeTestFields(t *testing.T, n *base.Node, wire []byte, offset uint64) {
	t.Helper()
	field := func(name string, want any, start, end int) {
		miopTestField(t, n, name, want, offset+uint64(start)*8, offset+uint64(end)*8)
	}
	kind, header := wire[0], int(wire[1])*4
	field("Packet Type", kind, 0, 1)
	field("Header Length Words", wire[1], 1, 2)
	field("Policy Identifier", binary.BigEndian.Uint16(wire[2:4]), 2, 4)
	message := protocolCorpusFindNode(n, "Packet Type").Cfg.GetItem(base.CfgParent).(*base.Node)
	info := alljoynTestInfo(t, message)
	require.Equal(t, "draft-ipsec-swipe-01 historical datagram", info["Profile"])
	require.EqualValues(t, header, info["Header Bytes"])
	require.Equal(t, kind <= 1, info["Clear Inner IPv4 Fields Decoded"])
	for _, key := range []string{"Policy Context Validated", "Data Decrypted", "Authenticator Verified", "Sequence State Validated", "Padding Validated", "Control Semantics Decoded"} {
		require.Equal(t, false, info[key], key)
	}
	if kind == 1 {
		field("Packet Sequence Number", binary.BigEndian.Uint32(wire[4:8]), 4, 8)
		if header > 8 {
			field("Authenticator", wire[8:header], 8, header)
		}
	} else {
		require.Nil(t, protocolCorpusFindNode(n, "Packet Sequence Number"))
		if kind == 2 || kind == 3 {
			field("Encrypted Header Fields", wire[4:header], 4, header)
			field("Encrypted Packet and Padding", wire[header:], header, len(wire))
		} else if kind >= 16 {
			if header > 4 {
				field("Control Header Extension", wire[4:header], 4, header)
			}
			if len(wire) > header {
				field("Control Payload", wire[header:], header, len(wire))
			}
		}
	}
	if kind <= 1 {
		ip := wire[header:]
		miopTestField(t, n, "Inner IP Version", uint8(4), offset+uint64(header)*8, offset+uint64(header)*8+4)
		miopTestField(t, n, "Inner Header Length Words", ip[0]&15, offset+uint64(header)*8+4, offset+uint64(header+1)*8)
		field("Inner Total Length", binary.BigEndian.Uint16(ip[2:4]), header+2, header+4)
		field("Inner Source", ip[12:16], header+12, header+16)
		field("Inner Destination", ip[16:20], header+16, header+20)
		size := int(binary.BigEndian.Uint16(ip[2:4]))
		iheader := int(ip[0]&15) * 4
		if iheader > 20 {
			field("Inner Header Options", ip[20:iheader], header+20, header+iheader)
		}
		if size > iheader {
			field("Inner Payload", ip[iheader:size], header+iheader, header+size)
		}
		if len(ip) > size {
			field("Unverified Padding", ip[size:], header+size, len(wire))
		}
	} else {
		require.Nil(t, protocolCorpusFindNode(n, "Inner IP Version"))
	}
	// NodeToBytes returns the complete parse-context buffer, including any
	// enclosing prefix. Read this message's independently known bit extent.
	// An unfinished non-byte-aligned context keeps its last bits in the writer,
	// outside Buffer.Bytes. Those imports are checked field-by-field above and
	// against the held reader below; do not flush/mutate the caller's writer.
	if offset%8 == 0 {
		reader := base.NewBitReader(bytes.NewReader(NodeToBytes(message)))
		if offset > 0 {
			_, err := reader.ReadBits(offset)
			require.NoError(t, err)
		}
		actual, err := reader.ReadBits(uint64(len(wire)) * 8)
		require.NoError(t, err)
		require.Equal(t, wire, actual)
	}
}

func swipeTestReject(t *testing.T, wire []byte, message string) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "swipe", "SwIPe")
	require.Error(t, err)
	if message != "" {
		require.ErrorContains(t, err, message)
	}
	if len(wire) > 0 {
		n := protocolCorpusRequireBoundedRuleParse(t, wire, "swipe", "SwIPeCarrier")
		require.Nil(t, protocolCorpusFindNode(n, "Packet Type"))
		miopTestField(t, n, "Unparsed swIPe Payload", wire, 0, uint64(len(wire))*8)
		require.Equal(t, wire, NodeToBytes(n))
	}
}

func TestProtocolCorpusSwIPeFieldsAndBoundaries(t *testing.T) {
	for _, literal := range swipeTestLiterals {
		wire := alljoynTestHex(t, literal)
		for _, entry := range []string{"SwIPe", "SwIPeCarrier"} {
			n := protocolCorpusRequireBoundedRuleParse(t, wire, "swipe", entry)
			swipeTestFields(t, n, wire, 0)
		}
	}
	for _, literal := range []string{"", "000100", "00000000", "0002000100000000", "00010002", "01010001", "0202000111223344", "40010001", "ff010001", "0103123401020304", "0001000165000014010200003f110000c000020ac6336414"} {
		swipeTestReject(t, alljoynTestHex(t, literal), "")
	}
	// All control types are common-header-only, with no guessed sequence fields.
	for kind := 4; kind < 256; kind++ {
		wire := []byte{byte(kind), 1, 0x12, 0x34}
		if kind < 16 || kind >= 64 {
			swipeTestReject(t, wire, "unused or reserved packet type")
		} else {
			n := protocolCorpusRequireBoundedRuleParse(t, wire, "swipe", "SwIPe")
			swipeTestFields(t, n, wire, 0)
		}
	}
	// Header length units are words. Every legal encoded length is bounded.
	for words := 2; words <= 255; words++ {
		wire := append([]byte{3, byte(words), 0x12, 0x34}, bytes.Repeat([]byte{0xa5}, words*4-4+1)...)
		n := protocolCorpusRequireBoundedRuleParse(t, wire, "swipe", "SwIPe")
		swipeTestFields(t, n, wire, 0)
		swipeTestReject(t, wire[:words*4-1], "invalid header word length")
	}
	plain := alljoynTestHex(t, swipeTestLiterals[0])
	for end := 0; end < len(plain); end++ {
		swipeTestReject(t, plain[:end], "")
	}
	for _, invalidSize := range []uint16{0, 19, 24, 65535} {
		wire := append([]byte(nil), plain...)
		binary.BigEndian.PutUint16(wire[6:8], invalidSize)
		swipeTestReject(t, wire, "inner IPv4 length")
	}
	// Options and fragment bytes are retained, not reassembled or interpreted.
	options := alljoynTestHex(t, "000100014600001b010220013f110000c000020ac633641401010000646174")
	n := protocolCorpusRequireBoundedRuleParse(t, options, "swipe", "SwIPe")
	swipeTestFields(t, n, options, 0)
}

func TestProtocolCorpusSwIPeOriginalAndCompanionEveryRecord(t *testing.T) {
	const original = "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-swipe.pcap"
	data, err := os.ReadFile(original)
	require.NoError(t, err)
	require.Equal(t, "7cb5760fe6b9441fa11d1898670b36dac47b2ca0dfccb6fc1a1dc187df20dc8b", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, original)
	require.Len(t, frames, 1)
	require.Len(t, frames[0], 50)
	require.Equal(t, uint16(0x800), binary.BigEndian.Uint16(frames[0][12:14]))
	require.Equal(t, byte(0x45), frames[0][14])
	require.Equal(t, byte(53), frames[0][23])
	require.Equal(t, make([]byte, 16), frames[0][34:])
	swipeTestReject(t, frames[0][34:], "invalid header word length")
	frames = protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-swipe-valid.pcap")
	require.Len(t, frames, len(swipeTestLiterals))
	for i, frame := range frames {
		wire := alljoynTestHex(t, swipeTestLiterals[i])
		require.Equal(t, uint16(0x800), binary.BigEndian.Uint16(frame[12:14]))
		require.Equal(t, byte(0x45), frame[14])
		require.Equal(t, byte(53), frame[23])
		require.Equal(t, uint16(len(frame)-14), binary.BigEndian.Uint16(frame[16:18]))
		require.Equal(t, wire, frame[34:])
		root := fcoeTestInline(t, fmt.Sprintf("Package:\n  Envelope:\n    Headers: raw,34\n    Body:\n      import: swipe.yaml\n      node: SwIPe\n      length: %d\n", len(wire)*8))
		root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
		require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(frame)), "Envelope"))
		n := base.GetNodeByPath(root, "@Envelope")
		require.Equal(t, frame, NodeToBytes(n))
		swipeTestFields(t, n, wire, 34*8)
	}
}

func TestProtocolCorpusSwIPeImportedAndResourceBoundaries(t *testing.T) {
	for bit := 0; bit < 8; bit++ {
		for _, literal := range append(append([]string(nil), swipeTestLiterals...), "00000000") {
			wire := alljoynTestHex(t, literal)
			root := fcoeTestInline(t, fmt.Sprintf("Package:\n  Envelope:\n    Prefix: uint16,%dbit\n    Body:\n      import: swipe.yaml\n      node: SwIPeCarrier\n      length: %d\n", bit+8, len(wire)*8))
			var packed bytes.Buffer
			writer := base.NewBitWriter(&packed)
			require.NoError(t, writer.WriteBits([]byte{0, 0}, uint64(bit+8)))
			require.NoError(t, writer.WriteBits(wire, uint64(len(wire))*8))
			require.NoError(t, writer.WriteBits([]byte{0xa5}, 8))
			if bit != 0 {
				require.NoError(t, writer.WriteBits([]byte{0}, uint64(8-bit)))
			}
			reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
			require.NoError(t, root.ParseSubNode(reader, "Envelope"))
			node := base.GetNodeByPath(root, "@Envelope")
			if wire[1] != 0 {
				swipeTestFields(t, node, wire, uint64(bit+8))
			} else {
				miopTestField(t, node, "Unparsed swIPe Payload", wire, uint64(bit+8), uint64(bit+8+len(wire)*8))
				require.Nil(t, protocolCorpusFindNode(node, "Packet Type"))
			}
			held, err := reader.ReadBits(8)
			require.NoError(t, err)
			require.Equal(t, []byte{0xa5}, held)
			require.ErrorContains(t, reader.Recovery(), "no backup")
		}
	}
	maximum := make([]byte, 65515)
	copy(maximum, []byte{16, 1, 0, 1})
	n := protocolCorpusRequireBoundedRuleParse(t, maximum, "swipe", "SwIPe")
	swipeTestFields(t, n, maximum, 0)
	swipeTestReject(t, append(maximum, 0), "implementation boundary")
	for _, entry := range []string{"SwIPe", "SwIPeCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(maximum), "swipe", entry)
		require.ErrorContains(t, err, "explicit datagram boundary required")
		root, err := base.ParseRule("swipe.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(maximum))*8-1)
		require.ErrorContains(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(maximum)), entry), "boundary must be byte-aligned")
	}
}
