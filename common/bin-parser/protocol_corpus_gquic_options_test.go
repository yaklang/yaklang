package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func gquic35TestOptionsOracle(t *testing.T, n *base.Node, wire []byte, offset uint64) int {
	t.Helper()
	require.Equal(t, 1, bytes.Count(wire, []byte("CHLO")))
	start := bytes.Index(wire, []byte("CHLO"))
	count := int(binary.LittleEndian.Uint16(wire[start+4:]))
	dataStart := start + 8 + count*8
	previous, typed := 0, 0
	for i := 0; i < count; i++ {
		at := start + 8 + i*8
		tag := string(wire[at : at+4])
		end := int(binary.LittleEndian.Uint32(wire[at+4:]))
		raw := wire[dataStart+previous : dataStart+end]
		position := offset + uint64(dataStart+previous)*8
		name := strings.TrimRight(tag, "\x00")
		node := protocolCorpusFindNode(n, "CHLO "+name)
		require.NotNil(t, node)
		require.Equal(t, raw, stream_parser.GetBytesByNode(node), tag)
		info := node.Cfg.GetItem("additionInfo").(map[string]any)
		require.Equal(t, false, info["Value Semantics Validated"])
		selected := true
		switch tag {
		case "SMHL":
			latTestField(t, n, "CHLO SMHL", "uint32", position, position+32, uint64(binary.LittleEndian.Uint32(raw)))
		case "CTIM", "XLCT":
			latTestField(t, n, "CHLO "+name, "uint64", position, position+64, binary.LittleEndian.Uint64(raw))
		case "CCS\x00", "CCRT":
			require.Equal(t, len(raw)/8, info["Hash Count"])
			require.Equal(t, false, info["Certificate Hash Matched"])
			for j := 0; j < len(raw); j += 8 {
				latTestField(t, n, fmt.Sprintf("%s Hash %d", name, j/8), "uint64", position+uint64(j)*8, position+uint64(j+8)*8, binary.LittleEndian.Uint64(raw[j:]))
			}
		case "NONC":
			require.Len(t, raw, 32)
			latTestField(t, n, "Nonce Timestamp Seconds", "uint32", position, position+32, uint64(binary.BigEndian.Uint32(raw)))
			latTestField(t, n, "Nonce Orbit", "raw", position+32, position+96, raw[4:12])
			latTestField(t, n, "Nonce Random Bytes", "raw", position+96, position+256, raw[12:])
			require.Equal(t, false, info["Orbit Correlated"])
			require.Equal(t, false, info["Clock Validated"])
			require.Equal(t, false, info["Randomness Validated"])
		case "NONP":
			require.Len(t, raw, 32)
			require.Empty(t, node.Children)
			require.Equal(t, false, info["Randomness Validated"])
		case "CSCT":
			require.Empty(t, raw)
			require.Equal(t, true, info["Empty SCT Request Marker"])
		case "PUBS":
			// CHLO SetStringPiece is one public value, not SCFG's u24 vector.
			require.Empty(t, node.Children)
			require.Equal(t, false, info["Value Layout Decoded"])
			selected = false
		default:
			selected = false
		}
		if selected {
			typed++
			require.Equal(t, true, info["Value Layout Decoded"], tag)
		}
		previous = end
	}
	return typed
}

func gquic35TestOptionsPacket(t *testing.T, options map[string][]byte) []byte {
	t.Helper()
	var tags []string
	valueBytes := 0
	for tag, data := range options {
		require.Len(t, tag, 4)
		require.NotEqual(t, "PAD\x00", tag)
		tags = append(tags, tag)
		valueBytes += len(data)
	}
	tags = append(tags, "PAD\x00")
	sort.Slice(tags, func(i, j int) bool {
		return binary.LittleEndian.Uint32([]byte(tags[i])) < binary.LittleEndian.Uint32([]byte(tags[j]))
	})
	table := make([]byte, 8+len(tags)*8)
	copy(table, "CHLO")
	binary.LittleEndian.PutUint16(table[4:], uint16(len(tags)))
	pad := 1024 - len(table) - valueBytes
	require.GreaterOrEqual(t, pad, 0)
	var values []byte
	for i, tag := range tags {
		data := options[tag]
		if tag == "PAD\x00" {
			data = bytes.Repeat([]byte{0x5a}, pad)
		}
		values = append(values, data...)
		copy(table[8+i*8:], tag)
		binary.LittleEndian.PutUint32(table[12+i*8:], uint32(len(values)))
	}
	chlo := append(table, values...)
	require.Len(t, chlo, 1024)
	header, _ := hex.DecodeString("0d01020304050607085130333501")
	body := append([]byte{0xa0, 1, 0, 4}, chlo...)
	return append(append(bytes.Clone(header), gquic35TestHash(header, body)...), body...)
}

func gquic35TestOptionValues() map[string][]byte {
	nonce := make([]byte, 32)
	for i := range nonce {
		nonce[i] = byte(i + 1)
	}
	return map[string][]byte{"SMHL": {0x81, 2, 3, 4}, "CTIM": {1, 2, 3, 4, 5, 6, 7, 0x88}, "XLCT": {8, 7, 6, 5, 4, 3, 2, 1}, "CCS\x00": {1, 2, 3, 4, 5, 6, 7, 8, 8, 7, 6, 5, 4, 3, 2, 1}, "CCRT": {0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, "NONC": nonce, "NONP": bytes.Repeat([]byte{0x3b}, 32), "CSCT": {}}
}

func TestProtocolCorpusGQUIC35OptionFieldsAndRollback(t *testing.T) {
	wire := gquic35TestOptionsPacket(t, gquic35TestOptionValues())
	n := gquic35TestParse(t, wire, "GQUIC35ClientHello")
	require.Equal(t, 8, gquic35TestOptionsOracle(t, n, wire, 0))
	megacoTestTree(t, gquic35TestMessage(t, n), 0, uint64(len(wire))*8)
	for cut := 0; cut < len(wire); cut++ {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), gquic35TestRule, "GQUIC35ClientHello")
		require.Error(t, err, "prefix %d", cut)
	}
	for _, tag := range []string{"SMHL", "CTIM", "XLCT", "NONC", "NONP", "CCS\x00", "CCRT"} {
		for _, size := range []int{1, 3, 5, 7, 9, 31, 33} {
			// All listed lengths are invalid for the selected fixed-width/vector type.
			w := gquic35TestOptionsPacket(t, map[string][]byte{tag: bytes.Repeat([]byte{1}, size)})
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), gquic35TestRule, "GQUIC35ClientHello")
			require.Error(t, err)
			raw := gquic35TestParse(t, w, "GQUIC35ClientHelloCarrier")
			require.Nil(t, protocolCorpusFindNode(raw, "Public Flags"))
			latTestField(t, raw, "Unparsed GQUIC35 Client Hello", "raw", 0, uint64(len(w))*8, w)
			gquic35TestParse(t, w, "GQUIC35ClientPacket") // envelope profile stays opaque
		}
	}
	for _, tag := range []string{"CCS\x00", "CCRT"} {
		w := gquic35TestOptionsPacket(t, map[string][]byte{tag: {}})
		n := gquic35TestParse(t, w, "GQUIC35ClientHello")
		f := protocolCorpusFindNode(n, "CHLO "+strings.TrimRight(tag, "\x00"))
		require.Equal(t, 0, f.Cfg.GetItem("additionInfo").(map[string]any)["Hash Count"])
		v, err := f.Result()
		require.NoError(t, err)
		require.NotNil(t, v)
	}
	// Nonempty CSCT is not labeled an SCT vector, and unknown tags stay raw.
	for _, tag := range []string{"CSCT", "ZZZZ"} {
		w := gquic35TestOptionsPacket(t, map[string][]byte{tag: {1, 2, 3}})
		n := gquic35TestParse(t, w, "GQUIC35ClientHello")
		f := protocolCorpusFindNode(n, "CHLO "+tag)
		require.Equal(t, false, f.Cfg.GetItem("additionInfo").(map[string]any)["Value Layout Decoded"])
		require.Equal(t, []byte{1, 2, 3}, stream_parser.GetBytesByNode(f))
	}
}
