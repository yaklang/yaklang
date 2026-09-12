package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// RFC 6325 six-byte header followed by an 18-byte C-tagged inner header.
// The first record carries an independently checksummed IPv4/ICMP request;
// the second exposes one RFC 7179 flags word and experimental payload bytes.
var trillCorpusHex = []string{
	"0025123456780200000000200200000000108100a12308004500001c0001000040017cde7f0000017f0000010800f7ff00000000",
	"0849234567890000000001005e00000102000000001081005abc88b51020304050",
}

func trillCorpusBody(t *testing.T, index int) []byte {
	t.Helper()
	body, err := hex.DecodeString(trillCorpusHex[index])
	require.NoError(t, err)
	return body
}

func trillCorpusFrame(body []byte, tagged bool) []byte {
	header := []byte{2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 0x22, 0xf3}
	if tagged {
		header = append(header[:12], 0x81, 0, 0x60, 0x07, 0x22, 0xf3)
	}
	return append(header, body...)
}

func trillCorpusRequireFields(t *testing.T, node *base.Node, body []byte, offset int) {
	t.Helper()
	first := binary.BigEndian.Uint16(body)
	words := int(first >> 6 & 31)
	inner := 6 + words*4
	for _, field := range []struct {
		name       string
		start, end int
		value      any
	}{
		{"TRILL Version", 0, 2, uint64(first >> 14)},
		{"TRILL Reserved", 2, 4, uint64(first >> 12 & 3)},
		{"Multi Destination", 4, 5, uint64(first >> 11 & 1)},
		{"Option Length", 5, 10, uint64(words)},
		{"TRILL Hop Count", 10, 16, uint64(first & 63)},
		{"Egress Nickname", 16, 32, uint64(binary.BigEndian.Uint16(body[2:]))},
		{"Ingress Nickname", 32, 48, uint64(binary.BigEndian.Uint16(body[4:]))},
		{"Inner Destination", inner * 8, (inner + 6) * 8, body[inner : inner+6]},
		{"Inner Source", (inner + 6) * 8, (inner + 12) * 8, body[inner+6 : inner+12]},
		{"Inner Tag Type", (inner + 12) * 8, (inner + 14) * 8, uint64(0x8100)},
		{"Inner Priority", (inner + 14) * 8, (inner+14)*8 + 3, uint64(body[inner+14] >> 5)},
		{"Inner C", (inner+14)*8 + 3, (inner+14)*8 + 4, uint64(body[inner+14] >> 4 & 1)},
		{"Inner VLAN ID", (inner+14)*8 + 4, (inner + 16) * 8, uint64(binary.BigEndian.Uint16(body[inner+14:]) & 0xfff)},
		{"Inner EtherType", (inner + 16) * 8, (inner + 18) * 8, uint64(binary.BigEndian.Uint16(body[inner+16:]))},
	} {
		protocolCorpusRequireValue(t, node, field.name, field.value)
		require.Equal(t, [2]uint64{uint64(offset*8 + field.start), uint64(offset*8 + field.end)}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, field.name)), field.name)
	}
	if words > 0 {
		flags := binary.BigEndian.Uint32(body[6:])
		bit := 0
		for _, field := range []struct {
			name string
			bits int
		}{
			{"CHbHS", 1}, {"CItES", 1}, {"CRSVS", 1}, {"Critical Hop by Hop Flags", 5},
			{"Noncritical Hop by Hop Flags", 6}, {"Critical Reserved Flags", 3}, {"Noncritical Reserved Flags", 4},
			{"Critical Ingress to Egress Flags", 6}, {"Noncritical Ingress to Egress Flags", 5},
		} {
			value := uint64(flags >> uint(32-bit-field.bits) & ((1 << field.bits) - 1))
			protocolCorpusRequireValue(t, node, field.name, value)
			require.Equal(t, [2]uint64{uint64((offset+6)*8 + bit), uint64((offset+6)*8 + bit + field.bits)}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, field.name)))
			bit += field.bits
		}
		if words > 1 {
			protocolCorpusRequireValue(t, node, "Additional Extensions", body[10:inner])
		}
	}
}

func TestProtocolCorpusTRILLFieldsAndCarriers(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-validated/gen-trill-valid.pcap")
	require.Len(t, frames, len(trillCorpusHex))
	for index := range trillCorpusHex {
		body := trillCorpusBody(t, index)
		require.Equal(t, trillCorpusFrame(body, false), frames[index])
		for _, entry := range []struct {
			rule, name string
			wire       []byte
			offset     int
		}{
			{"trill", "TRILL", body, 0}, {"trill", "TRILLCarrier", body, 0},
			{"ethernet", "Ethernet", trillCorpusFrame(body, false), 14},
			{"ethernet", "Ethernet", trillCorpusFrame(body, true), 18},
		} {
			node := protocolCorpusRequireBoundedRuleParse(t, entry.wire, entry.rule, entry.name)
			trillCorpusRequireFields(t, node, body, entry.offset)
			require.Nil(t, protocolCorpusFindNode(node, "Unparsed TRILL Payload"))
			if index == 0 {
				protocolCorpusRequireValue(t, node, "Total Length", uint64(28))
				protocolCorpusRequireValue(t, node, "Protocol", uint64(1))
				require.Nil(t, protocolCorpusFindNode(node, "Inner Opaque Payload"))
			} else {
				protocolCorpusRequireValue(t, node, "Inner Opaque Payload", []byte{0x10, 0x20, 0x30, 0x40, 0x50})
			}
		}
	}
}

func trillCorpusRequireFallback(t *testing.T, body []byte) {
	t.Helper()
	wire := trillCorpusFrame(body, false)
	if len(body) == 0 {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "ethernet", "Ethernet")
		require.ErrorContains(t, err, "trill: empty carrier")
		return
	}
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "ethernet", "Ethernet")
	protocolCorpusRequireValue(t, node, "Unparsed TRILL Payload", body)
	require.Nil(t, protocolCorpusFindNode(node, "TRILL Version"))
	require.Equal(t, [2]uint64{112, uint64(len(wire) * 8)}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Unparsed TRILL Payload")))
}

func TestProtocolCorpusTRILLBoundsAndRollback(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-trill.pcap")
	require.Len(t, frames, 1)
	require.Len(t, frames[0], 64)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(frames[0][14:]), "trill", "TRILL")
	require.ErrorContains(t, err, "trill: unsupported inner tag; C-tag required")
	trillCorpusRequireFallback(t, frames[0][14:])
	body := trillCorpusBody(t, 1)
	// Only prefixes shorter than the declared headers are necessarily invalid;
	// TRILL has no payload length, so a shorter opaque body is observable data.
	for end := 0; end < 28; end++ {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(body[:end]), "trill", "TRILL")
		require.ErrorContains(t, err, "trill:", "prefix %d", end)
		trillCorpusRequireFallback(t, body[:end])
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { b[0] |= 0x40 }, func(b []byte) { b[1] = 0xc9 },
		func(b []byte) { b[22] = 0x22; b[23] = 0xe7 },
	} {
		wire := bytes.Clone(body)
		mutate(wire)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "trill", "TRILL")
		require.ErrorContains(t, err, "trill:")
		trillCorpusRequireFallback(t, wire)
	}
	for _, entry := range []string{"TRILL", "TRILLCarrier"} {
		_, err := parser.ParseBinary(bytes.NewBuffer(body), "trill", entry)
		require.ErrorContains(t, err, "explicit")
	}
	wire := append(bytes.Clone(body), make([]byte, 65536-len(body))...)
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "trill", "TRILL")
	require.ErrorContains(t, err, "65535-byte")
	trillCorpusRequireFallback(t, wire)
	wire = wire[:65535]
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "trill", "TRILL")
	protocolCorpusRequireValue(t, node, "Inner Opaque Payload", wire[28:])
}

func TestProtocolCorpusTRILLExtensionBitsAndOpaqueLimits(t *testing.T) {
	body := trillCorpusBody(t, 1)
	for bit := 0; bit < 32; bit++ {
		wire := bytes.Clone(body)
		binary.BigEndian.PutUint32(wire[6:], 1<<bit)
		trillCorpusRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, wire, "trill", "TRILL"), wire, 0)
	}
	for words := 1; words <= 31; words++ {
		wire := append(bytes.Clone(body[:10]), bytes.Repeat([]byte{0x17, 0xa5, 0x3c, 0xe9}, words-1)...)
		wire = append(wire, body[10:]...)
		binary.BigEndian.PutUint16(wire, 0x3800|uint16(words<<6)|63)
		// Reserved base bits, C bit and exhausted hops are retained on reception.
		wire[6+words*4+14] |= 0x10
		trillCorpusRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, wire, "trill", "TRILL"), wire, 0)
	}
	// Nested TRILL is explicit opaque data, never recursive dispatch.
	zeroHop := bytes.Clone(body)
	zeroHop[1] &= 0xc0
	trillCorpusRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, zeroHop, "trill", "TRILL"), zeroHop, 0)
	wire := bytes.Clone(body)
	wire[26] = 0x22
	wire[27] = 0xf3
	wire = append(wire[:28], bytes.Repeat(body, 100)...)
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "trill", "TRILL")
	protocolCorpusRequireValue(t, node, "Inner Opaque Payload", wire[28:])
	// A failed inner IP trial keeps all bytes but retains successful TRILL fields.
	wire = trillCorpusBody(t, 0)
	wire = wire[:25]
	node = protocolCorpusRequireBoundedRuleParse(t, wire, "trill", "TRILL")
	protocolCorpusRequireValue(t, node, "Inner Opaque Payload", wire[24:])
	require.Nil(t, protocolCorpusFindNode(node, "Total Length"))
}

func TestProtocolCorpusTRILLInnerIPv4Boundary(t *testing.T) {
	withOptions, err := hex.DecodeString("0025123456780200000000200200000000108100a12308004600002000010000400179d97f0000017f000001010101000800f7ff00000000")
	require.NoError(t, err)
	variants := [][]byte{withOptions}
	for _, mutate := range []func([]byte){
		func(b []byte) { b[24] = 0x55 },
		func(b []byte) { b[30] = 0x20 },
		func(b []byte) { b[31] = 1 },
		func(b []byte) { b[27] = 19 },
		func(b []byte) { b[27] = 29 },
		func(b []byte) { b[33] = 4 },
		func(b []byte) { b[33] = 41 },
		func(b []byte) { b[33] = 17 },
		func(b []byte) { b[33] = 6 },
		func(b []byte) { b[44] = 1 }, // Unknown ICMP type: no partial success.
	} {
		wire := trillCorpusBody(t, 0)
		mutate(wire)
		variants = append(variants, wire)
	}
	for _, wire := range variants {
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "trill", "TRILL")
		trillCorpusRequireFields(t, node, wire, 0)
		protocolCorpusRequireValue(t, node, "Inner Opaque Payload", wire[24:])
		require.Equal(t, [2]uint64{192, uint64(len(wire) * 8)}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Inner Opaque Payload")))
		require.Nil(t, protocolCorpusFindNode(node, "Total Length"))
		require.Nil(t, protocolCorpusFindNode(node, "Inner Link Trailer"))
	}
}

func TestProtocolCorpusTRILLInnerIPv6AndARP(t *testing.T) {
	// An IPv6 tunnel must not bypass the IPv4 options guard recursively.
	tunnel, err := hex.DecodeString("0025123456780200000000200200000000108100a12386dd600000000020042520010000000000000000000000000001200100000000000000000000000000024600002000010000400179d97f0000017f000001010101000800f7ff00000000")
	require.NoError(t, err)
	node := protocolCorpusRequireBoundedRuleParse(t, tunnel, "trill", "TRILL")
	protocolCorpusRequireValue(t, node, "Inner Opaque Payload", tunnel[24:])
	require.Nil(t, protocolCorpusFindNode(node, "Total Length"))
	require.Nil(t, protocolCorpusFindNode(node, "Inner Link Trailer"))
	// Direct, nonrecursive ICMPv6 echo with eight bytes of ICMPv6 data.
	v6 := bytes.Clone(tunnel[:64])
	v6[29], v6[30] = 8, 58
	v6 = append(v6, 128, 0, 0x3f, 0xb8, 0, 0, 0, 0)
	node = protocolCorpusRequireBoundedRuleParse(t, v6, "trill", "TRILL")
	protocolCorpusRequireValue(t, node, "Payload Length", uint64(8))
	require.Equal(t, [2]uint64{224, 240}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Payload Length")))
	require.Nil(t, protocolCorpusFindNode(node, "Inner Opaque Payload"))
	for _, mutate := range []func([]byte){
		func(b []byte) { b[24] = 0x50 },
		func(b []byte) { b[29] = 9 },
		func(b []byte) { b[29] = 0 },
		func(b []byte) { b[30] = 44 },
		func(b []byte) { b[30] = 0 },
		func(b []byte) { b[30] = 17 },
	} {
		wire := bytes.Clone(v6)
		mutate(wire)
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "trill", "TRILL")
		protocolCorpusRequireValue(t, node, "Inner Opaque Payload", wire[24:])
		require.Nil(t, protocolCorpusFindNode(node, "Payload Length"))
	}
	arp, err := hex.DecodeString("0001080006040001020000000010c0000201000000000000c0000202")
	require.NoError(t, err)
	wire := append(trillCorpusBody(t, 0)[:24:24], arp...)
	wire[22], wire[23] = 8, 6
	node = protocolCorpusRequireBoundedRuleParse(t, wire, "trill", "TRILL")
	require.Nil(t, protocolCorpusFindNode(node, "Inner Opaque Payload"))
	protocolCorpusRequireValue(t, node, "Sender MAC address", []byte{2, 0, 0, 0, 0, 0x10})
	require.Equal(t, [2]uint64{256, 304}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(node, "Sender MAC address")))
	wire[28] = 5 // Unsupported hardware-address layout is kept intact.
	node = protocolCorpusRequireBoundedRuleParse(t, wire, "trill", "TRILL")
	protocolCorpusRequireValue(t, node, "Inner Opaque Payload", wire[24:])
}

func TestProtocolCorpusTRILLCarrierTransactions(t *testing.T) {
	valid := trillCorpusBody(t, 0)
	withTail := append(bytes.Clone(valid), 0x17, 0xa5, 0xfe)
	badTag := trillCorpusBody(t, 1)
	badTag[22] = 0x22
	for _, body := range [][]byte{valid, withTail, badTag, valid[:25], valid[:23]} {
		for _, tagged := range []bool{false, true} {
			wire := trillCorpusFrame(body, tagged)
			root, err := base.ParseRule("ethernet.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			reader := bytes.NewReader(wire)
			bitReader := base.NewBitReader(reader)
			require.NoError(t, root.ParseSubNode(bitReader, "Ethernet"))
			node := base.GetNodeByPath(root, "@Ethernet")
			require.Equal(t, wire, NodeToBytes(node))
			if bytes.Equal(body, withTail) {
				protocolCorpusRequireValue(t, node, "Inner Link Trailer", []byte{0x17, 0xa5, 0xfe})
			}
			require.Zero(t, reader.Len())
			require.ErrorContains(t, bitReader.Recovery(), "no backup")
			require.ErrorContains(t, bitReader.PopBackup(), "no backup")
			_, err = bitReader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		}
	}
}

func TestProtocolCorpusTRILLParallel(t *testing.T) {
	for worker := 0; worker < 8; worker++ {
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			for index := range trillCorpusHex {
				body := trillCorpusBody(t, index)
				trillCorpusRequireFields(t, protocolCorpusRequireBoundedRuleParse(t, trillCorpusFrame(body, false), "ethernet", "Ethernet"), body, 14)
			}
		})
	}
}
