package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func vxlanRuleHeader(flags, second byte, policy uint16, vni uint32, last byte) []byte {
	return []byte{flags, second, byte(policy >> 8), byte(policy), byte(vni >> 16), byte(vni >> 8), byte(vni), last}
}

func vxlanRuleARP() []byte {
	// Authored Ethernet/ARP request, including complete address fields. No
	// captured bytes or parse-success oracle are used to generate this shape.
	return []byte{255, 255, 255, 255, 255, 255, 2, 0, 0, 0, 0, 1, 8, 6, 0, 1, 8, 0, 6, 4, 0, 1, 2, 0, 0, 0, 0, 1, 192, 0, 2, 1, 0, 0, 0, 0, 0, 0, 192, 0, 2, 2}
}

func vxlanRuleUDP(payload []byte, source, destination uint16) []byte {
	wire := make([]byte, 8, 8+len(payload))
	binary.BigEndian.PutUint16(wire, source)
	binary.BigEndian.PutUint16(wire[2:], destination)
	binary.BigEndian.PutUint16(wire[4:], uint16(8+len(payload)))
	return append(wire, payload...)
}

func vxlanRuleParse(t *testing.T, wire []byte, rule, entry string) *base.Node {
	t.Helper()
	reader := newTransportTailExactReader(wire)
	node, err := parser.ParseBinary(reader, rule, entry)
	require.NoError(t, err)
	require.Zero(t, reader.Len())
	require.Equal(t, wire, NodeToBytes(node), "all original bytes retain exact field/raw ownership")
	return node
}

func TestVXLANRuleReservedReceiveAndConditionalGBPFieldRanges(t *testing.T) {
	for _, flags := range []byte{8, 9, 10, 12, 24, 40, 72, 136, 255} {
		for _, second := range []byte{8, 0x40, 0x48, 0x80, 0xff} {
			t.Run(fmt.Sprintf("flags%02x/second%02x", flags, second), func(t *testing.T) {
				wire := append(vxlanRuleHeader(flags, second, 0x1234, 42, 0xff), vxlanRuleARP()...)
				node := vxlanRuleParse(t, wire, "vxlan", "VXLAN")
				value, err := node.Result()
				require.NoError(t, err)
				require.EqualValues(t, 1, uintVal(t, value.Child("I")))
				require.EqualValues(t, 42, uintVal(t, value.Child("VNI")))
				require.EqualValues(t, 1, uintVal(t, mustChild(t, value, "Inner", "ARP", "Opcode")))
				if flags&128 == 0 {
					require.Nil(t, value.Child("GBP"))
					require.Equal(t, []byte{second, 0x12, 0x34}, value.Child("Reserved").Value)
				} else {
					gbp := mustChild(t, value, "GBP")
					require.EqualValues(t, (second>>3)&1, uintVal(t, gbp.Child("A")))
					require.EqualValues(t, (second>>6)&1, uintVal(t, gbp.Child("D")))
					require.EqualValues(t, 0x1234, uintVal(t, gbp.Child("Group Policy ID")))
					for path, span := range map[string][2]uint64{"@VXLAN.G": {0, 1}, "@VXLAN.I": {4, 5}, "@VXLAN.GBP.D": {9, 10}, "@VXLAN.GBP.A": {12, 13}, "@VXLAN.GBP.Group Policy ID": {16, 32}, "@VXLAN.VNI": {32, 56}} {
						field := base.GetNodeByPath(node, path)
						require.NotNil(t, field)
						require.Equal(t, span, stream_parser.GetNodeResultPos(field))
					}
				}
			})
		}
	}
}

func TestVXLANRuleActualDestinationPrecedesSourceHints(t *testing.T) {
	payload := append(vxlanRuleHeader(8, 0, 0, 42, 0), vxlanRuleARP()...)
	for _, source := range []uint16{53, 4789, 55000} {
		node := vxlanRuleParse(t, vxlanRuleUDP(payload, source, 4789), "user_datagram_protocol", "UDP")
		require.NotNil(t, base.GetNodeByPath(node, "@UDP.Payload.VXLAN.Inner.ARP"))
		require.Nil(t, base.GetNodeByPath(node, "@UDP.Payload.DNS"))
	}
	wire := vxlanRuleUDP(payload, 4789, 55001)
	node := vxlanRuleParse(t, wire, "user_datagram_protocol", "UDP")
	require.Nil(t, base.GetNodeByPath(node, "@UDP.Payload.VXLAN"))
	tail := base.GetNodeByPath(node, "@UDP.Payload.Remaining Payload")
	require.NotNil(t, tail)
	value, err := tail.Result()
	require.NoError(t, err)
	require.Equal(t, payload, value.Value)
}

func TestVXLANRuleTruncatedAndInvalidParentRollBackWithoutChild(t *testing.T) {
	valid := append(vxlanRuleHeader(8, 0, 0, 42, 0), vxlanRuleARP()...)
	cases := [][]byte{append(vxlanRuleHeader(0, 0, 0, 42, 0), vxlanRuleARP()...), append(vxlanRuleHeader(0xf7, 0xff, 0xffff, 42, 0xff), vxlanRuleARP()...)}
	for size := 0; size < 22; size++ {
		cases = append(cases, bytes.Clone(valid[:size]))
	}
	for _, payload := range cases {
		_, err := parser.ParseBinary(newTransportTailExactReader(payload), "vxlan", "VXLAN")
		require.Error(t, err)
		node := vxlanRuleParse(t, vxlanRuleUDP(payload, 55000, 4789), "user_datagram_protocol", "UDP")
		require.Nil(t, base.GetNodeByPath(node, "@UDP.Payload.VXLAN"), "a partially decoded speculative parent cannot survive recovery")
	}
	for _, declared := range []uint16{0, 7, uint16(8 + len(valid) + 1)} {
		wire := vxlanRuleUDP(valid, 55000, 4789)
		binary.BigEndian.PutUint16(wire[4:], declared)
		_, err := parser.ParseBinary(newTransportTailExactReader(wire), "user_datagram_protocol", "UDP")
		require.Error(t, err)
	}
	// Bytes outside the declared UDP payload cannot finish an inner header.
	wire := vxlanRuleUDP(valid, 55000, 4789)
	binary.BigEndian.PutUint16(wire[4:], 16)
	reader := newTransportTailExactReader(wire)
	node, err := parser.ParseBinary(reader, "user_datagram_protocol", "UDP")
	require.NoError(t, err)
	require.Nil(t, base.GetNodeByPath(node, "@UDP.Payload.VXLAN"))
	require.Equal(t, len(vxlanRuleARP()), reader.Len())
	require.Equal(t, wire[:16], NodeToBytes(node))
}

func TestVXLANGPERuleIsCoherentHeaderOnlyWithUninterpretedPayload(t *testing.T) {
	for _, next := range []uint16{0, 1, 2, 3, 4, 127} {
		flags := byte(8)
		if next != 0 {
			flags |= 4
		}
		// Flags c0 are reserved on this profile too; they are ignored, not a
		// license to dispatch the complete Ethernet-looking payload below it.
		payload := append(vxlanRuleHeader(flags|0xc0, 0xff, 0xff00|next, 42, 0xff), vxlanRuleARP()...)
		if next == 0 {
			payload[3] = 0
		}
		node := vxlanRuleParse(t, vxlanRuleUDP(payload, 53, 4790), "user_datagram_protocol", "UDP")
		header := base.GetNodeByPath(node, "@UDP.Payload.VXLANGPEHeader")
		require.NotNil(t, header)
		require.Nil(t, base.GetNodeByPath(node, "@UDP.Payload.VXLANGPEHeader.Inner"))
		require.Nil(t, base.GetNodeByPath(node, "@UDP.Payload.VXLAN"))
		require.Nil(t, base.GetNodeByPath(node, "@UDP.Payload.DNS"))
		v, err := header.Result()
		require.NoError(t, err)
		require.EqualValues(t, next, uintVal(t, v.Child("Next Protocol")))
		tail := base.GetNodeByPath(node, "@UDP.Payload.Remaining Payload")
		require.NotNil(t, tail)
		tailValue, err := tail.Result()
		require.NoError(t, err)
		require.Equal(t, vxlanRuleARP(), tailValue.Value)
	}
	for _, header := range [][]byte{vxlanRuleHeader(4, 0, 3, 42, 0), vxlanRuleHeader(28, 0, 3, 42, 0), vxlanRuleHeader(8, 0, 3, 42, 0), vxlanRuleHeader(12, 0, 0, 42, 0), {12, 0, 0, 3, 0, 0, 42}} {
		node := vxlanRuleParse(t, vxlanRuleUDP(header, 55000, 4790), "user_datagram_protocol", "UDP")
		require.Nil(t, base.GetNodeByPath(node, "@UDP.Payload.VXLANGPEHeader"))
		require.Nil(t, base.GetNodeByPath(node, "@UDP.Payload.VXLAN"))
	}
}

func TestVXLANCatalogDoesNotClaimGPEDataOrPolicyExecution(t *testing.T) {
	baseCount, headerCount := 0, 0
	for _, info := range ProtocolCatalog {
		if info.Name == "VXLAN" {
			baseCount++
			require.Equal(t, statusPartial, info.Status)
			require.Equal(t, "VXLAN", info.EntryNode)
			require.Contains(t, info.Notes, "not policy execution")
		}
		if info.EntryNode == "VXLANGPEHeader" {
			headerCount++
			require.Equal(t, statusPartial, info.Status)
			require.Equal(t, "VXLAN", info.ProfileOf)
			require.Contains(t, info.Notes, "header only")
		}
	}
	require.Equal(t, 1, baseCount)
	require.Equal(t, 1, headerCount)
}
