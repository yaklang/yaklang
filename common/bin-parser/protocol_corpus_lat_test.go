package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// Independent literals from AA-NL26A-TE (June 1989), figures 4-2..4-10 and A-6.
// These are exact LAT PDUs, with no inferred Ethernet padding or FCS. Run slot
// data/status is intentionally not interpreted as class 1 without session state.
var latTestFixtures = []struct{ name, literal string }{
	{"run-empty", "010034127856feff"},
	{"start-master", "0600000078560100ee050501100208143412110105534c415645064d4153544552008002dead00"},
	{"start-host", "040034127856010040020501100200003412030105534c415645064d41535445520000"},
	{"stop", "08003412000009070203627965"},
	{"run-six-slots", "0106341278560908" +
		"01021e93011f7f03545459044e4f444501020180020234120403505f318002dead00" +
		"01020305616263e1" + "010201a299e2" + "010201b081e3" + "010001c244e4" + "010001d355e5"},
}

func latTestBytes(t *testing.T, literal string) []byte {
	t.Helper()
	b, err := hex.DecodeString(literal)
	require.NoError(t, err)
	return b
}

func latTestInline(t *testing.T, source string) *base.Node {
	t.Helper()
	var document yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &document))
	root, err := base.NewNodeTree(document)
	require.NoError(t, err)
	return root
}

func latTestNode(t *testing.T, n *base.Node) *base.Node {
	t.Helper()
	var find func(*base.Node) *base.Node
	find = func(n *base.Node) *base.Node {
		if info, ok := n.Cfg.GetItem("additionInfo").(map[string]any); ok && info["Profile"] == "LAT virtual-circuit structural fields" {
			return n
		}
		for _, child := range n.Children {
			if result := find(child); result != nil {
				return result
			}
		}
		return nil
	}
	result := find(n)
	require.NotNil(t, result, "missing decoded LAT message")
	return result
}

func latTestEthernet(t *testing.T, wire []byte) []byte {
	t.Helper()
	return append(latTestBytes(t, "0200000000020200000000016004"), wire...)
}

func latTestField(t *testing.T, n *base.Node, name, typ string, begin, end uint64, expected any) {
	t.Helper()
	field := protocolCorpusFindNode(n, name)
	require.NotNil(t, field, name)
	require.Equal(t, typ, field.Cfg.GetItem(base.CfgType), name)
	require.Equal(t, [2]uint64{begin, end}, stream_parser.GetNodeResultPos(field), name)
	protocolCorpusRequireValue(t, n, name, expected)
}

func latTestTree(t *testing.T, n *base.Node, start, end uint64) {
	t.Helper()
	if n.Cfg.GetBool(stream_parser.CfgIsList) && len(n.Children) == 0 {
		_, err := n.Result()
		require.ErrorContains(t, err, "no result")
		require.Equal(t, start, end)
		return
	}
	value, err := n.Result()
	require.NoError(t, err, n.Name)
	require.Same(t, n, value.Origin, n.Name)
	if stream_parser.NodeHasResult(n) {
		require.Equal(t, [2]uint64{start, end}, stream_parser.GetNodeResultPos(n), n.Name)
		return
	}
	position := start
	for index, child := range n.Children {
		require.Same(t, n, child.Cfg.GetItem(base.CfgParent), child.Name)
		require.Same(t, n.Ctx, child.Ctx, child.Name)
		if n.Cfg.GetBool(stream_parser.CfgIsList) {
			require.Equal(t, index, child.Cfg.GetItem(stream_parser.CfgElementIndex))
		}
		length := stream_parser.CalcNodeConsumedLength(child)
		latTestTree(t, child, position, position+length)
		position += length
	}
	require.Equal(t, end, position, n.Name)
}

func latTestCheck(t *testing.T, node *base.Node, wire []byte, offset uint64) {
	t.Helper()
	n := latTestNode(t, node)
	latTestTree(t, n, offset, offset+uint64(len(wire))*8)
	latTestField(t, n, "Message Type", "uint8", offset, offset+6, uint64(wire[0]>>2))
	latTestField(t, n, "Master", "uint8", offset+6, offset+7, uint64((wire[0]>>1)&1))
	latTestField(t, n, "Response Required", "uint8", offset+7, offset+8, uint64(wire[0]&1))
	latTestField(t, n, "Slot Count", "uint8", offset+8, offset+16, uint64(wire[1]))
	latTestField(t, n, "Destination Circuit ID", "uint16", offset+16, offset+32, uint64(wire[2])|uint64(wire[3])<<8)
	latTestField(t, n, "Source Circuit ID", "uint16", offset+32, offset+48, uint64(wire[4])|uint64(wire[5])<<8)
	latTestField(t, n, "Message Sequence Number", "uint8", offset+48, offset+56, uint64(wire[6]))
	latTestField(t, n, "Message Acknowledgment Number", "uint8", offset+56, offset+64, uint64(wire[7]))
	info := n.Cfg.GetItem("additionInfo").(map[string]any)
	for _, key := range []string{"Session State Validated", "Negotiated Limits Validated", "Service Data Semantics Decoded", "Parameter Data Semantics Decoded", "Discovery Exchange Decoded", "Ethernet Padding Inferred"} {
		require.Equal(t, false, info[key], key)
	}
	require.Equal(t, 1500, info["Maximum Message Bytes"])
	role := "host"
	if wire[0]&2 != 0 {
		role = "terminal-server"
	}
	require.Equal(t, role, info["Sender Role"])
	if wire[0]>>2 == 1 {
		latTestField(t, n, "Protocol Version", "uint8", offset+80, offset+88, uint64(5))
		latTestField(t, n, "Protocol ECO", "uint8", offset+88, offset+96, uint64(1))
		latTestField(t, n, "Facility Number", "uint16", offset+128, offset+144, uint64(0x1234))
		latTestField(t, n, "Slave Node Name", "string", offset+168, offset+208, "SLAVE")
		latTestField(t, n, "Master Node Name", "string", offset+216, offset+264, "MASTER")
	} else if wire[0]>>2 == 2 {
		latTestField(t, n, "Circuit Disconnect Reason", "uint8", offset+64, offset+72, uint64(2))
		latTestField(t, n, "Reason Text", "string", offset+80, offset+104, "bye")
	} else if wire[1] == 6 {
		slots := protocolCorpusFindNode(n, "Slots")
		require.Len(t, slots.Children, 6)
		for index, typ := range []uint64{9, 0, 10, 11, 12, 13} {
			protocolCorpusRequireValue(t, slots.Children[index], "Slot Type", typ)
		}
		latTestField(t, slots.Children[3], "Must Be Zero", "uint8", offset+476, offset+480, uint64(0))
		latTestField(t, slots.Children[4], "Slot Reason", "uint8", offset+524, offset+528, uint64(2))
		latTestField(t, slots.Children[5], "Slot Reason", "uint8", offset+572, offset+576, uint64(3))
		latTestField(t, slots.Children[0], "Service Class", "uint8", offset+96, offset+104, uint64(1))
		latTestField(t, slots.Children[0], "Object Service Name", "string", offset+128, offset+152, "TTY")
		latTestField(t, slots.Children[0], "Subject Description", "string", offset+160, offset+192, "NODE")
		parameters := protocolCorpusFindNode(slots.Children[0], "Parameters")
		require.Len(t, parameters.Children, 5)
		for index, value := range [][]byte{{1, 128}, {0x34, 0x12}, []byte("P_1"), {0xde, 0xad}} {
			protocolCorpusRequireValue(t, parameters.Children[index], "Parameter Data", value)
		}
		for index, value := range [][]byte{[]byte("abc"), {0x99}, {0x81}, {0x44}, {0x55}} {
			data := "Slot Data"
			if index >= 3 {
				data = "Slot Status"
			}
			protocolCorpusRequireValue(t, slots.Children[index+1], data, value)
			protocolCorpusRequireValue(t, slots.Children[index+1], "Slot Padding", []byte{0xe1 + byte(index)})
		}
	}
}

func TestProtocolCorpusLATFields(t *testing.T) {
	for _, fixture := range latTestFixtures {
		for _, entry := range []string{"LAT", "LATCarrier"} {
			t.Run(fixture.name+"/"+entry, func(t *testing.T) {
				wire := latTestBytes(t, fixture.literal)
				node := protocolCorpusRequireBoundedRuleParse(t, wire, "lat", entry)
				require.Equal(t, wire, NodeToBytes(node))
				latTestCheck(t, node, wire, 0)
			})
		}
	}
}

func latTestRaw(t *testing.T, wire []byte) {
	t.Helper()
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "lat", "LATCarrier")
	require.Equal(t, wire, NodeToBytes(node))
	require.Len(t, node.Children, 1)
	latTestField(t, node, "Unparsed LAT Payload", "raw", 0, uint64(len(wire))*8, wire)
	require.Nil(t, protocolCorpusFindNode(node, "Message Type"))
	require.False(t, node.Cfg.Has("additionInfo"))
}

func TestProtocolCorpusLATAllOriginalRecords(t *testing.T) {
	const path = "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-lat.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "9d8eae2891109a462e5a15ce2d6ddce98f86bceefa94bdbd485e3d4bd53b77d1", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 1)
	for _, frame := range frames {
		require.Equal(t, latTestBytes(t, "020000000002020000000001600401000000000000000000000000000000"), frame)
		wire := frame[14:]
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "lat", "LAT")
		require.ErrorContains(t, err, "Run circuit identifiers must be nonzero")
		latTestRaw(t, wire)
		node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		latTestField(t, node, "Unparsed LAT Payload", "raw", 112, uint64(len(frame))*8, wire)
		require.Nil(t, protocolCorpusFindNode(node, "Message Type"))
		require.Equal(t, frame, NodeToBytes(node))
		// Fixing only the zero IDs exposes the second defect: zero slots
		// describes exactly the eight-byte header, not the extra eight bytes.
		fixedIDs := bytes.Clone(wire)
		fixedIDs[2], fixedIDs[4] = 1, 2
		_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(fixedIDs), "lat", "LAT")
		require.ErrorContains(t, err, "unexpected trailing message bytes")
		latTestRaw(t, fixedIDs)
	}
}

func TestProtocolCorpusLATEthernetBoundaries(t *testing.T) {
	for _, fixture := range latTestFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			wire := latTestBytes(t, fixture.literal)
			frame := latTestEthernet(t, wire)
			node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			latTestCheck(t, node, wire, 112)
			require.Equal(t, frame, NodeToBytes(node))
			// An unrelated EtherType must never select LAT based on its bytes.
			other := bytes.Clone(frame)
			other[12], other[13] = 0x88, 0xb5
			node = protocolCorpusRequireBoundedRuleParse(t, other, "ethernet", "Ethernet")
			require.Nil(t, protocolCorpusFindNode(node, "Message Type"))
			require.Nil(t, protocolCorpusFindNode(node, "Unparsed LAT Payload"))
			require.Equal(t, other, NodeToBytes(node))
			for _, tail := range [][]byte{{0}, {0xde, 0xad, 0xbe, 0xef}, bytes.Repeat([]byte{0}, 46)} {
				padded := append(bytes.Clone(frame), tail...)
				node = protocolCorpusRequireBoundedRuleParse(t, padded, "ethernet", "Ethernet")
				if wire[0]>>2 == 1 {
					// This is LAT's specified Start tail, not inferred Ethernet pad.
					latTestField(t, node, "Unpredictable Tail", "raw", uint64(len(frame))*8, uint64(len(padded))*8, tail)
				} else {
					latTestField(t, node, "Unparsed LAT Payload", "raw", 112, uint64(len(padded))*8, padded[14:])
					require.Nil(t, protocolCorpusFindNode(node, "Message Type"))
				}
				require.Nil(t, protocolCorpusFindNode(node, "Frame Trailer"))
				require.Equal(t, padded, NodeToBytes(node))
			}
		})
	}
}

func TestProtocolCorpusLATShortPrefixes(t *testing.T) {
	for _, fixture := range latTestFixtures {
		wire := latTestBytes(t, fixture.literal)
		t.Run(fixture.name, func(t *testing.T) {
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "lat", "LAT")
				require.Error(t, err, "prefix %d", cut)
				if cut > 0 {
					latTestRaw(t, wire[:cut])
				}
			}
		})
	}
}

func TestProtocolCorpusLATNegativeBoundaries(t *testing.T) {
	run, start, slots := latTestBytes(t, latTestFixtures[0].literal), latTestBytes(t, latTestFixtures[1].literal), latTestBytes(t, latTestFixtures[4].literal)
	for _, tc := range []struct {
		name   string
		wire   []byte
		index  int
		value  byte
		reason string
	}{
		{"unsupported-announcement", run, 0, 40, "unsupported message type"},
		{"master-response-required", run, 0, 3, "response-required flag"},
		{"start-response-required", start, 0, 7, "response-required flag"},
		{"run-zero-destination", run, 2, 0, ""},
		{"start-slots", start, 1, 1, "invalid Start slot count"},
		{"start-wrong-destination", start, 2, 1, "directional circuit"},
		{"start-version", start, 10, 6, "unsupported Start version"},
		{"start-eco", start, 11, 0, "unsupported Start version"},
		{"start-timer", start, 14, 0, "circuit timer"},
		{"start-empty-slave", start, 20, 0, "must be nonempty"},
		{"start-invalid-name", start, 21, 0x20, "invalid character"},
		{"too-many-slots", slots, 1, 7, "truncated"},
		{"unknown-slot", slots, 11, 0x83, "unsupported slot type"},
		{"slot-length", slots, 10, 255, "slot length"},
		{"service-class", slots, 12, 2, "unsupported Start slot service class"},
		{"zero-data-size", slots, 14, 0, "minimum data slot size"},
		{"parameter-size", slots, 25, 1, "class 1 parameter length"},
		{"attention-mbz", slots, 59, 0xb1, "slot flags"},
		{"stop-slot-source", slots, 69, 2, "slot identifiers"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := bytes.Clone(tc.wire)
			wire[tc.index] = tc.value
			if tc.name == "run-zero-destination" {
				wire[3] = 0
			}
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "lat", "LAT")
			require.Error(t, err)
			if tc.reason != "" {
				require.ErrorContains(t, err, tc.reason)
			}
			latTestRaw(t, wire)
		})
	}
	for _, index := range []int{0, 3, 4} {
		wire := append(latTestBytes(t, latTestFixtures[index].literal), 0xaa)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "lat", "LAT")
		require.ErrorContains(t, err, "unexpected trailing message bytes")
		latTestRaw(t, wire)
	}
}

func TestProtocolCorpusLATRunSlotSourceIDs(t *testing.T) {
	for _, command := range []byte{0, 2} {
		for _, typ := range []byte{0, 10, 11} {
			wire := latTestBytes(t, "000134127856090801000000")
			wire[0], wire[11] = command, typ<<4
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "lat", "LAT")
			require.ErrorContains(t, err, "Run slot source identifier must be nonzero")
			latTestRaw(t, wire)
			wire[9] = 2
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "lat", "LAT")
			require.Equal(t, wire, NodeToBytes(node))
		}
	}
	// The separate Stop rule requires zero; Reject is not a Run slot and
	// must not inherit this source-ID constraint.
	for _, typ := range []byte{12, 13} {
		wire := latTestBytes(t, "000134127856090801000000")
		wire[11] = typ<<4 | 1
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "lat", "LAT")
		require.Equal(t, wire, NodeToBytes(node))
	}
}

func TestProtocolCorpusLATStartSlotTermination(t *testing.T) {
	valid := latTestBytes(t, "010134127856090801020690010001000000")
	node := protocolCorpusRequireBoundedRuleParse(t, valid, "lat", "LAT")
	require.Equal(t, valid, NodeToBytes(node))
	for _, literal := range []string{
		"010134127856090801020890010001000000aabb",
		"010134127856090801020790010001000000aae1",
	} {
		wire := latTestBytes(t, literal)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "lat", "LAT")
		require.ErrorContains(t, err, "trailing class 1 Start slot status")
		latTestRaw(t, wire)
	}
	// A seven-byte status with a one-byte counted description is valid;
	// its required eighth alignment byte is outside the declared status.
	odd := latTestBytes(t, "01013412785609080102079001000100014e00e1")
	node = protocolCorpusRequireBoundedRuleParse(t, odd, "lat", "LAT")
	latTestField(t, node, "Slot Padding", "raw", 152, 160, []byte{0xe1})
	require.Equal(t, odd, NodeToBytes(node))
}

func TestProtocolCorpusLATAllCompanionRecords(t *testing.T) {
	const path = "testdata/protocol-corpus/captures/generated-validated/gen-lat-valid.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "19d397a78b62df964315f7258417fcf58349b91b707c946d447fa52514e6589c", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, len(latTestFixtures))
	for index, frame := range frames {
		wire := latTestBytes(t, latTestFixtures[index].literal)
		require.Equal(t, latTestEthernet(t, wire), frame)
		for _, entry := range []string{"LAT", "LATCarrier"} {
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "lat", entry)
			latTestCheck(t, node, wire, 0)
			require.Equal(t, wire, NodeToBytes(node))
		}
		node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		latTestCheck(t, node, wire, 112)
		require.Equal(t, frame, NodeToBytes(node))
	}
}

func TestProtocolCorpusLATUnpredictableAndResources(t *testing.T) {
	start := latTestBytes(t, latTestFixtures[1].literal)
	// Start expressly permits unpredictable bytes following parameter code 0.
	for _, n := range []int{1, 7, 1500 - len(start)} {
		wire := append(bytes.Clone(start), bytes.Repeat([]byte{0xa5}, n)...)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "lat", "LAT")
		latTestField(t, node, "Unpredictable Tail", "raw", uint64(len(start))*8, uint64(len(wire))*8, wire[len(start):])
		require.Equal(t, wire, NodeToBytes(node))
	}
	oversize := append(bytes.Clone(start), bytes.Repeat([]byte{0xa5}, 1501-len(start))...)
	reader := newProtocolCorpusBoundedReader(oversize)
	_, err := parser.ParseBinary(reader, "lat", "LAT")
	require.ErrorContains(t, err, "8..1500")
	require.Equal(t, len(oversize), reader.Len())
	latTestRaw(t, oversize)
	// All 255 encoded slot positions, including zero-length opaque data.
	run := latTestBytes(t, "0100341278560908")
	run[1] = 255
	run = append(run, bytes.Repeat([]byte{1, 2, 0, 1}, 255)...)
	node := protocolCorpusRequireBoundedRuleParse(t, run, "lat", "LAT")
	require.Len(t, protocolCorpusFindNode(node, "Slots").Children, 255)
	latTestTree(t, node, 0, uint64(len(run))*8)
	require.Equal(t, run, NodeToBytes(node))
	for _, entry := range []string{"LAT", "LATCarrier"} {
		_, err = parser.ParseBinary(bytes.NewReader(start), "lat", entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, "lat", entry)
		require.Error(t, err)
	}
}

func TestProtocolCorpusLATSlotAndParameterBounds(t *testing.T) {
	// The full uint8 body count is valid and needs an external alignment byte.
	maximum := latTestBytes(t, "01013412785609080102ff00")
	maximum = append(maximum, bytes.Repeat([]byte{0xab}, 255)...)
	maximum = append(maximum, 0xed)
	node := protocolCorpusRequireBoundedRuleParse(t, maximum, "lat", "LAT")
	latTestField(t, node, "Slot Data", "raw", 96, 267*8, maximum[12:267])
	latTestField(t, node, "Slot Padding", "raw", 267*8, 268*8, []byte{0xed})
	require.Equal(t, maximum, NodeToBytes(node))
	for _, n := range []int{0, 32, 33} {
		// Class 1's group-code mask is at most 256 bits. Other parameter
		// values are preserved, including reserved flags ignored on receive.
		status := []byte{1, 0, 1, 0, 0, 6, byte(n)}
		status = append(status, bytes.Repeat([]byte{0xff}, n)...)
		status = append(status, 0)
		wire := latTestBytes(t, "010134127856090801020090")
		wire[10] = byte(len(status))
		wire = append(wire, status...)
		if len(status)%2 != 0 {
			wire = append(wire, 0x85)
		}
		if n > 32 {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "lat", "LAT")
			require.ErrorContains(t, err, "class 1 parameter length")
			latTestRaw(t, wire)
		} else {
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "lat", "LAT")
			require.Equal(t, wire, NodeToBytes(node))
			protocolCorpusRequireValue(t, node, "Minimum Attention Slot Size", uint64(0))
		}
	}
	// The generic Start parameter envelope accepts the whole uint8 count.
	start := latTestBytes(t, latTestFixtures[2].literal)
	start = append(start[:len(start)-1], 0x80, 255)
	start = append(start, bytes.Repeat([]byte{0xa5}, 255)...)
	start = append(start, 0)
	node = protocolCorpusRequireBoundedRuleParse(t, start, "lat", "LAT")
	protocolCorpusRequireValue(t, node, "Parameter Data", bytes.Repeat([]byte{0xa5}, 255))
	require.Equal(t, start, NodeToBytes(node))
}

func TestProtocolCorpusLATImportedTransactions(t *testing.T) {
	for offset := uint64(0); offset < 8; offset++ {
		for _, valid := range []bool{false, true} {
			t.Run(fmt.Sprintf("offset-%d/valid-%t", offset, valid), func(t *testing.T) {
				wire := latTestBytes(t, latTestFixtures[4].literal)
				if !valid {
					wire[len(wire)-3] = 0xff
				} // invalid final Stop slot
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
				root := latTestInline(t, fmt.Sprintf(`
endian: big
Package:
  Wrapped:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Frame").SetMaxLength(%d)
      this.ProcessSubNode("Frame")
      this.ProcessSubNode("Sentinel")
      if %d > 0 { this.ProcessSubNode("Padding") }
    Prefix: uint8,%dbit
    Frame: "import:lat.yaml;node:LATCarrier"
    Sentinel: uint8
    Padding: uint8,%dbit
`, offset, len(wire), offset, offset, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("lat-test-marker", "outer")
				reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, root.ParseSubNode(reader, "Wrapped"))
				wrapped := base.GetNodeByPath(root, "@Wrapped")
				frame := protocolCorpusFindNode(wrapped, "Frame")
				if valid {
					latTestCheck(t, frame, wire, offset)
				} else {
					latTestField(t, frame, "Unparsed LAT Payload", "raw", offset, offset+uint64(len(wire))*8, wire)
					require.Nil(t, protocolCorpusFindNode(frame, "Message Type"))
					require.False(t, frame.Cfg.Has("additionInfo"))
				}
				protocolCorpusRequireValue(t, wrapped, "Sentinel", uint64(0xd3))
				require.Equal(t, packed.Bytes(), NodeToBytes(wrapped))
				require.Equal(t, uint64(packed.Len())*8, root.Ctx.GetUint64("pointer"))
				require.Equal(t, "outer", root.Ctx.GetItem("lat-test-marker"))
				require.ErrorContains(t, reader.Recovery(), "no backup")
				require.ErrorContains(t, reader.PopBackup(), "no backup")
				_, err := reader.ReadBits(8)
				require.ErrorIs(t, err, io.EOF)
			})
		}
	}
}

func TestProtocolCorpusLATShortPhysicalAndBitBoundary(t *testing.T) {
	wire := latTestBytes(t, latTestFixtures[4].literal)
	for _, entry := range []string{"LAT", "LATCarrier"} {
		for available := 0; available < len(wire); available++ {
			root, err := base.ParseRule("lat.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			reader := base.NewBitReader(bytes.NewReader(wire[:available]))
			err = root.ParseSubNode(reader, entry)
			require.Error(t, err)
			require.Nil(t, protocolCorpusFindNode(base.GetNodeByPath(root, "@"+entry), "Message Type"))
			require.ErrorContains(t, reader.Recovery(), "no backup")
			require.ErrorContains(t, reader.PopBackup(), "no backup")
		}
		for extra := uint64(1); extra < 8; extra++ {
			reader := &giopCarrierTestBitReader{Reader: bytes.NewReader(append(bytes.Clone(wire), 0)), bits: uint64(len(wire))*8 + extra}
			_, err := parser.ParseBinary(reader, "lat", entry)
			require.ErrorContains(t, err, "byte")
			require.Equal(t, len(wire)+1, reader.Len())
		}
	}
}

func TestProtocolCorpusLATConcurrentIsolation(t *testing.T) {
	var wg sync.WaitGroup
	for worker := 0; worker < 24; worker++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			wire := latTestBytes(t, latTestFixtures[index%len(latTestFixtures)].literal)
			node := protocolCorpusRequireBoundedRuleParse(t, wire, "lat", "LATCarrier")
			latTestCheck(t, node, wire, 0)
			require.Equal(t, wire, NodeToBytes(node))
		}(worker)
	}
	wg.Wait()
}

func TestProtocolCorpusLATParseOnlyErrors(t *testing.T) {
	_, err := parser.GenerateBinary(map[string]any{}, "lat", "LAT")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "structured generation is not supported"), err.Error())
}
