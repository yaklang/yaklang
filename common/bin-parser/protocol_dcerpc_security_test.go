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

func dcerpcSecurityRuleTestWire() []byte {
	wire := make([]byte, 32+8+6)
	wire[0], wire[2], wire[3], wire[4] = 5, 11, 3, 0x10
	binary.LittleEndian.PutUint16(wire[8:10], uint16(len(wire)))
	binary.LittleEndian.PutUint16(wire[10:12], 6)
	binary.LittleEndian.PutUint32(wire[12:16], 2)
	copy(wire[16:32], bytes.Repeat([]byte{0xa5}, 16))
	wire[32], wire[33], wire[34], wire[35] = 10, 6, 3, 0x7f // reserved is ignored on read
	binary.LittleEndian.PutUint32(wire[36:40], 0x10203040)
	copy(wire[40:], []byte{0, 0xff, 0x80, 1, 2, 3})
	return wire
}

func TestDCERPCSecurityRuleWireBoundariesAndOpaqueBody(t *testing.T) {
	wire := dcerpcSecurityRuleTestWire()
	for offset := uint64(0); offset < 8; offset++ {
		var packed bytes.Buffer
		writer := base.NewBitWriter(&packed)
		if offset > 0 {
			require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
		}
		require.NoError(t, writer.WriteBits(wire, uint64(len(wire))*8))
		require.NoError(t, writer.WriteBits([]byte{0xd3}, 8))
		if offset > 0 {
			require.NoError(t, writer.WriteBits([]byte{0}, 8-offset))
		}
		root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/dcerpc.yaml;node:DCERPC\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, 8-offset))
		root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
		require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(packed.Bytes())), "Wrapped"))
		node := protocolCorpusFindNode(root, "Message")
		tlsCertificateTestTree(t, node, wire, offset)
		protocolCorpusRequireValue(t, root, "Sentinel", uint64(0xd3))
		protocolCorpusRequireValue(t, root, "Auth Context ID", uint64(0x10203040))
		protocolCorpusRequireValue(t, root, "Auth Reserved", uint64(0x7f))
		protocolCorpusRequireValue(t, root, "Authentication Token", wire[40:])
		protocolCorpusRequireValue(t, root, "Opaque Body And Padding", wire[16:32])
		require.Nil(t, protocolCorpusFindNode(root, "Bind"), "authenticated body bytes cannot become unverified bind contexts")
	}
	for cut := 0; cut < len(wire); cut++ {
		_, err := parser.ParseBinary(bytes.NewReader(wire[:cut]), "application-layer.dcerpc", "DCERPC")
		require.Error(t, err, "truncated complete-fragment declaration %d", cut)
	}
	for _, mutate := range []func([]byte){
		func(w []byte) { binary.LittleEndian.PutUint16(w[10:12], uint16(len(w))) },
		func(w []byte) { w[34] = 17 },
		func(w []byte) { w[1] = 1 },
		func(w []byte) { w[4] = 0 },
	} {
		bad := bytes.Clone(wire)
		mutate(bad)
		_, err := parser.ParseBinary(bytes.NewReader(bad), "application-layer.dcerpc", "DCERPC")
		require.Error(t, err)
	}
}

func TestDCERPCSecurityRuleDoesNotAssumeCommonHeaderAlignmentOrigin(t *testing.T) {
	wire := make([]byte, 76)
	wire[0], wire[2], wire[3], wire[4] = 5, 15, 7, 0x10
	binary.LittleEndian.PutUint16(wire[8:10], uint16(len(wire)))
	binary.LittleEndian.PutUint16(wire[10:12], 12)
	wire[56], wire[57], wire[60] = 68, 6, 1
	root, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.dcerpc", "DCERPC")
	require.NoError(t, err)
	protocolCorpusRequireValue(t, root, "Auth Type", uint64(68))
	protocolCorpusRequireValue(t, root, "Auth Context ID", uint64(1))
	protocolCorpusRequireValue(t, root, "Authentication Token", wire[64:])
	require.Nil(t, protocolCorpusFindNode(root, "BindAck"))
}

func TestDCERPCSecurityRuleInvalidBoundsPrecedeUnsupportedProfile(t *testing.T) {
	wire := dcerpcSecurityRuleTestWire()
	wire[32], wire[33], wire[34] = 0xeb, 0xeb, 0xeb
	_, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.dcerpc", "DCERPC")
	require.ErrorContains(t, err, "authentication padding overlaps")
	wire[34] = 0
	_, err = parser.ParseBinary(bytes.NewReader(wire), "application-layer.dcerpc", "DCERPC")
	require.ErrorContains(t, err, "unsupported authentication type or level")
}

func requireP0DCERPCWireEnvelopeScope(t *testing.T) {
	t.Helper()
	TestDCERPCSecurityRuleWireBoundariesAndOpaqueBody(t)
	TestDCERPCSecurityRuleDoesNotAssumeCommonHeaderAlignmentOrigin(t)
	TestDCERPCSecurityRuleInvalidBoundsPrecedeUnsupportedProfile(t)
	catalog, roadmap, aliasCatalog, aliasRoadmap := false, false, false, false
	for _, entry := range ProtocolCatalog {
		if entry.Name == "DCE/RPC" {
			catalog = true
			require.Equal(t, statusPartial, entry.Status)
			require.Contains(t, entry.Notes, "opaque")
			require.Contains(t, entry.Notes, "not validated")
		}
		if entry.Name == "MSRPC" {
			aliasCatalog = true
			require.Equal(t, statusPartial, entry.Status)
			require.Equal(t, "application-layer/dcerpc.yaml", entry.RuleFile)
			require.Contains(t, entry.Notes, "bounded")
		}
	}
	for _, entry := range ProtocolRoadmap {
		if entry.Name == "DCE/RPC" {
			roadmap = true
			require.Equal(t, stPartial, entry.Status)
			require.Equal(t, priP0, entry.Priority)
		}
		if entry.Name == "MSRPC" {
			aliasRoadmap = true
			require.Equal(t, stPartial, entry.Status)
			require.Equal(t, priP0, entry.Priority)
		}
	}
	require.True(t, catalog)
	require.True(t, roadmap)
	require.True(t, aliasCatalog)
	require.True(t, aliasRoadmap)
}
