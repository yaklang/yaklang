package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func wepTestFields(t *testing.T, node *base.Node, wire []byte, offset uint64) {
	t.Helper()
	miopTestField(t, node, "Initialization Vector", wire[:3], offset, offset+24)
	miopTestField(t, node, "Key Index", uint8(wire[3]>>6), offset+24, offset+26)
	miopTestField(t, node, "Extended IV", uint8(0), offset+26, offset+27)
	miopTestField(t, node, "Reserved", uint8(wire[3]&31), offset+27, offset+32)
	end := offset + uint64(len(wire))*8
	if len(wire) > 8 {
		miopTestField(t, node, "Encrypted Data", wire[4:len(wire)-4], offset+32, end-32)
	} else {
		require.Nil(t, protocolCorpusFindNode(node, "Encrypted Data"))
	}
	miopTestField(t, node, "Encrypted ICV", wire[len(wire)-4:], end-32, end)
	body := protocolCorpusFindNode(node, "Initialization Vector").Cfg.GetItem(base.CfgParent).(*base.Node)
	info := alljoynTestInfo(t, body)
	require.Equal(t, true, info["Cipher Selection Explicit"])
	for _, name := range []string{"Cipher Context Validated", "Data Decrypted", "ICV Verified", "Frame FCS Verified", "Application Semantics Decoded", "Reserved Bits Validated"} {
		require.Equal(t, false, info[name], name)
	}
	require.EqualValues(t, len(wire)-8, info["Encrypted Data Bytes"])
}

func TestProtocolCorpusWEPOriginalEveryRecord(t *testing.T) {
	const path = "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-wep.pcap"
	encoded, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "398559abad1a948444bebd78c40869ea15d65c8b0cf41a3e600b3b4254a8766e", fmt.Sprintf("%x", sha256.Sum256(encoded)))
	// The manifest independently pins the complete original capture digest.
	var manifest protocolCorpusManifest
	require.NoError(t, json.Unmarshal(readProtocolCorpusFile(t, "testdata/protocol-corpus", "manifest.json"), &manifest))
	found := false
	for _, capture := range manifest.Captures {
		if capture.ID == "pr5023-gen-wep" {
			require.Equal(t, capture.SHA256, fmt.Sprintf("%x", sha256.Sum256(encoded)))
			found = true
		}
	}
	require.True(t, found)
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 1)
	for _, frame := range frames {
		require.Len(t, frame, 60)
		require.Equal(t, []byte{0, 0, 8, 0, 0, 0, 0, 0}, frame[:8])
		macOffset := int(binary.LittleEndian.Uint16(frame[2:4]))
		fc := binary.LittleEndian.Uint16(frame[macOffset:])
		require.Equal(t, uint16(0x4008), fc)
		// Non-QoS, three-address Data with no MAC FCS in this capture. Check
		// the complete carrier headers, not an arbitrary cropped suffix.
		headerLength := 24
		require.Equal(t, []byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 2, 0, 0, 0, 0, 1}, frame[macOffset+4:macOffset+22])
		bodyOffset := macOffset + headerLength
		wire := frame[bodyOffset:]
		require.Equal(t, append([]byte{0, 0, 1, 0}, make([]byte, 24)...), wire)
		for _, entry := range []string{"WEP", "WEPCarrier"} {
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "wep", entry)
			wepTestFields(t, node, wire, 0)
			require.Equal(t, wire, NodeToBytes(node))
			root := fcoeTestInline(t, fmt.Sprintf("Package:\n  Envelope:\n    Headers: raw,%d\n    Body:\n      import: wep.yaml\n      node: %s\n      length: %d\n", bodyOffset, entry, len(wire)*8))
			root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
			require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(frame)), "Envelope"))
			envelope := base.GetNodeByPath(root, "@Envelope")
			require.Equal(t, frame, NodeToBytes(envelope))
			wepTestFields(t, envelope, wire, uint64(bodyOffset)*8)
		}
	}
}

func TestProtocolCorpusWEPSelectorsAndOpaqueBoundaries(t *testing.T) {
	for key := 0; key < 4; key++ {
		for _, reserved := range []byte{0, 1, 31} {
			for _, length := range []int{0, 1, 20, 2304} {
				wire := append([]byte{0xa1, 0xb2, 0xc3, byte(key<<6) | reserved}, bytes.Repeat([]byte{0x5a}, length)...)
				wire = append(wire, 0x12, 0x34, 0x56, 0x78)
				for _, entry := range []string{"WEP", "WEPCarrier"} {
					node := protocolCorpusRequireBoundedRuleParse(t, wire, "wep", entry)
					wepTestFields(t, node, wire, 0)
					require.Equal(t, wire, NodeToBytes(node))
				}
			}
		}
	}
	// WEP has no clear body-length field. Prefixes >=8 bytes can only be
	// structurally delimited, never declared authentic or complete plaintext.
	wire := append([]byte{1, 2, 3, 0}, bytes.Repeat([]byte{0xff}, 24)...)
	for length := 1; length <= len(wire); length++ {
		prefix := wire[:length]
		if length < 8 {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(prefix), "wep", "WEP")
			require.ErrorContains(t, err, "wep: body outside")
			node := protocolCorpusRequireBoundedRuleParse(t, prefix, "wep", "WEPCarrier")
			miopTestField(t, node, "Unparsed WEP Payload", prefix, 0, uint64(length)*8)
			require.Nil(t, protocolCorpusFindNode(node, "Initialization Vector"))
		} else {
			wepTestFields(t, protocolCorpusRequireBoundedRuleParse(t, prefix, "wep", "WEP"), prefix, 0)
		}
	}
	for selector := 0; selector < 256; selector++ {
		if selector&32 == 0 {
			continue
		}
		wire[3] = byte(selector)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "wep", "WEP")
		require.ErrorContains(t, err, "wep: extended IV is not a WEP layout")
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "wep", "WEPCarrier")
		require.Equal(t, wire, NodeToBytes(node))
		require.Nil(t, protocolCorpusFindNode(node, "Initialization Vector"))
	}
}

func TestProtocolCorpusWEPImportsAndResourceBoundary(t *testing.T) {
	wire := []byte{1, 2, 3, 0xc1, 0xff, 0x12, 0x34, 0x56, 0x78}
	for bit := 0; bit < 8; bit++ {
		for _, valid := range []bool{true, false} {
			body := bytes.Clone(wire)
			if !valid {
				body[3] |= 32
			}
			root := fcoeTestInline(t, fmt.Sprintf("Package:\n  Envelope:\n    Prefix: uint8,%dbit\n    Body:\n      import: wep.yaml\n      node: WEPCarrier\n      length: %d\n", bit+8, len(body)*8))
			var packed bytes.Buffer
			writer := base.NewBitWriter(&packed)
			require.NoError(t, writer.WriteBits([]byte{0, 0}, uint64(bit+8)))
			require.NoError(t, writer.WriteBits(body, uint64(len(body))*8))
			require.NoError(t, writer.WriteBits([]byte{0xa5}, 8))
			if bit != 0 {
				require.NoError(t, writer.WriteBits([]byte{0}, uint64(8-bit)))
			}
			reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
			require.NoError(t, root.ParseSubNode(reader, "Envelope"))
			node := base.GetNodeByPath(root, "@Envelope")
			if valid {
				wepTestFields(t, node, body, uint64(bit+8))
			} else {
				miopTestField(t, node, "Unparsed WEP Payload", body, uint64(bit+8), uint64(bit+8+len(body)*8))
				require.Nil(t, protocolCorpusFindNode(node, "Initialization Vector"))
			}
			held, err := reader.ReadBits(8)
			require.NoError(t, err)
			require.Equal(t, []byte{0xa5}, held)
			require.ErrorContains(t, reader.Recovery(), "no backup")
		}
	}
	maximum := make([]byte, 65535)
	node := protocolCorpusRequireBoundedRuleParse(t, maximum, "wep", "WEP")
	wepTestFields(t, node, maximum, 0)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(maximum, 0)), "wep", "WEP")
	require.ErrorContains(t, err, "wep: body outside")
	for _, entry := range []string{"WEP", "WEPCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(wire), "wep", entry)
		require.ErrorContains(t, err, "wep: explicit")
		root, err := base.ParseRule("wep.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8-1)
		require.ErrorContains(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(wire)), entry), "wep: boundary must be byte-aligned")
	}
}
