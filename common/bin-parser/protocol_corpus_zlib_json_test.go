package bin_parser

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const zlibJSONTestRule = "application-layer.zlib_json"

// Independent stored-DEFLATE layout and the RFC1950 two-sum Adler recurrence,
// not the production inflater or checksum package. No fixture is transmitted.
func zlibJSONTestStored(t *testing.T, document []byte) []byte {
	t.Helper()
	require.LessOrEqual(t, len(document), 65535)
	wire := make([]byte, 4+2+5+len(document)+4)
	binary.BigEndian.PutUint32(wire[:4], uint32(len(wire)-4))
	wire[4], wire[5], wire[6] = 0x78, 1, 1
	binary.LittleEndian.PutUint16(wire[7:], uint16(len(document)))
	binary.LittleEndian.PutUint16(wire[9:], ^uint16(len(document)))
	copy(wire[11:], document)
	a, b := uint32(1), uint32(0)
	for _, c := range document {
		a = (a + uint32(c)) % 65521
		b = (b + a) % 65521
	}
	binary.BigEndian.PutUint32(wire[len(wire)-4:], b<<16|a)
	return wire
}

func zlibJSONTestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	return protocolCorpusRequireBoundedRuleParse(t, wire, zlibJSONTestRule, entry)
}

func zlibJSONTestMessage(t *testing.T, n *base.Node) *base.Node {
	t.Helper()
	field := protocolCorpusFindNode(n, "Compressed Length")
	require.NotNil(t, field)
	return field.Cfg.GetItem(base.CfgParent).(*base.Node)
}

func zlibJSONTestInfo(t *testing.T, n *base.Node) map[string]any {
	t.Helper()
	return zlibJSONTestMessage(t, n).Cfg.GetItem("additionInfo").(map[string]any)
}

func TestProtocolCorpusZlibJSONIndependentFields(t *testing.T) {
	wire := zlibJSONTestStored(t, []byte("{}"))
	require.Equal(t, "0000000d7801010200fdff7b7d017500f9", hex.EncodeToString(wire))
	for _, entry := range []string{"ZlibJSONRecord", "ZlibJSONCarrier"} {
		n := zlibJSONTestParse(t, wire, entry)
		require.Equal(t, wire, NodeToBytes(n))
		megacoTestTree(t, zlibJSONTestMessage(t, n), 0, uint64(len(wire))*8)
		latTestField(t, n, "Compressed Length", "uint32", 0, 32, uint64(13))
		latTestField(t, n, "CMF", "uint8", 32, 40, uint64(0x78))
		latTestField(t, n, "FLG", "uint8", 40, 48, uint64(1))
		latTestField(t, n, "DEFLATE Bytes", "raw", 48, 104, wire[6:13])
		latTestField(t, n, "Adler32", "uint32", 104, 136, uint64(0x017500f9))
		info := zlibJSONTestInfo(t, n)
		require.Equal(t, true, info["Adler32 Verified"])
		require.Equal(t, uint8(8), info["Compression Method"])
		require.Equal(t, uint8(7), info["Compression Info"])
		require.Equal(t, uint8(0), info["FLEVEL"])
		require.Equal(t, 32768, info["Window Bytes"])
		require.Equal(t, []byte("{}"), info["Decoded Bytes"])
		require.Equal(t, map[string]any{"Kind": "object", "Members": []any{}}, info["Decoded Document"])
		for _, key := range []string{"Decoded Values Have Wire Spans", "Application Identity Inferred", "Application Semantics Decoded", "TCP Reassembled", "Structured Generation Supported"} {
			require.Equal(t, false, info[key], key)
		}
	}
	for _, tc := range []struct {
		text, kind string
		value      any
	}{
		{`true`, "boolean", true}, {`null`, "null", nil}, {`42`, "number", json.Number("42")},
		{`9007199254740993`, "number", json.Number("9007199254740993")}, {`1e400`, "number", json.Number("1e400")},
		{`"text"`, "string", "text"}, {`"\uD83D\uDE00"`, "string", "😀"}, {`"\uDEAD"`, "string", "�"},
		{" \t\r\n-0.00e+1\n", "number", json.Number("-0.00e+1")},
	} {
		n := zlibJSONTestParse(t, zlibJSONTestStored(t, []byte(tc.text)), "ZlibJSONRecord")
		doc := zlibJSONTestInfo(t, n)["Decoded Document"].(map[string]any)
		require.Equal(t, tc.kind, doc["Kind"])
		require.Equal(t, tc.value, doc["Value"])
		require.Equal(t, []byte(tc.text), zlibJSONTestInfo(t, n)["Decoded Bytes"])
	}
	text := `{"a":1,"\u0061":2,"items":[false,null,"文",{}]}`
	n := zlibJSONTestParse(t, zlibJSONTestStored(t, []byte(text)), "ZlibJSONRecord")
	doc := zlibJSONTestInfo(t, n)["Decoded Document"].(map[string]any)
	members := doc["Members"].([]any)
	require.Len(t, members, 3)
	for i, number := range []string{"1", "2"} {
		member := members[i].(map[string]any)
		require.Equal(t, "a", member["Name"])
		require.Equal(t, json.Number(number), member["Value"].(map[string]any)["Value"])
	}
	require.Len(t, members[2].(map[string]any)["Value"].(map[string]any)["Items"], 4)
}

func TestProtocolCorpusZlibJSONOriginalRecords(t *testing.T) {
	const path = "testdata/protocol-corpus/captures/ndpi/ndpi-tencent-games.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "d13fedee16feb1b676a73c77df9676cd84b2c136f7ae9e809efe5cf537a6d9d1", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 32)
	empty, other, decodedCount := 0, 0, 0
	for i, frame := range frames {
		// This original PCAP's link type is Raw IP, not Ethernet.
		require.GreaterOrEqual(t, len(frame), 40)
		require.Equal(t, byte(4), frame[0]>>4)
		require.Equal(t, byte(6), frame[9])
		ihl, total := int(frame[0]&15)*4, int(binary.BigEndian.Uint16(frame[2:]))
		require.GreaterOrEqual(t, ihl, 20)
		require.LessOrEqual(t, total, len(frame))
		require.Zero(t, binary.BigEndian.Uint16(frame[6:])&0x3fff)
		require.GreaterOrEqual(t, total-ihl, 20)
		offset := ihl + int(frame[ihl+12]>>4)*4
		require.GreaterOrEqual(t, offset-ihl, 20)
		require.LessOrEqual(t, offset, total)
		wire := frame[offset:total]
		if len(wire) == 0 {
			empty++
			continue
		}
		if i+1 != 20 && i+1 != 22 {
			other++
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), zlibJSONTestRule, "ZlibJSONRecord")
			require.Error(t, err, "unrelated frame %d", i+1)
			continue
		}
		decodedCount++
		n := zlibJSONTestParse(t, wire, "ZlibJSONRecord")
		require.True(t, bytes.Equal(wire, NodeToBytes(n)), "original record bytes changed")
		megacoTestTree(t, zlibJSONTestMessage(t, n), 0, uint64(len(wire))*8)
		info := zlibJSONTestInfo(t, n)
		length, compressed, hash := 509, 334, "10d488868cba9cae4d6aadd651dd70aceaaa283e05f6723d988f7dc719163e86"
		if i+1 == 22 {
			length, compressed, hash = 736, 429, "5ad9c06607c62b7fb28a6b0c1c5aacf9eef9fcffdd797273e4cc5cb9e5db9f67"
		}
		require.Equal(t, length, info["Decoded Byte Length"])
		require.Equal(t, hash, info["Decoded SHA256"])
		require.Equal(t, hash, fmt.Sprintf("%x", sha256.Sum256(info["Decoded Bytes"].([]byte))))
		latTestField(t, n, "Compressed Length", "uint32", 0, 32, uint64(compressed))
		latTestField(t, n, "CMF", "uint8", 32, 40, uint64(0x78))
		latTestField(t, n, "FLG", "uint8", 40, 48, uint64(1))
		doc := info["Decoded Document"].(map[string]any)
		require.Equal(t, "object", doc["Kind"])
		require.Len(t, doc["Members"], 13) // no original token values in diagnostics
		root := latTestInline(t, fmt.Sprintf("Package:\n  Envelope:\n    operator: |\n      this.ProcessSubNode(\"Prefix\")\n      this.GetSubNode(\"Record\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Record\")\n    Prefix: raw,%d\n    Record: \"import:application-layer/zlib_json.yaml;node:ZlibJSONRecord\"\n", len(wire), offset))
		root.Cfg.SetItem(base.CfgLength, uint64(total)*8)
		require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(frame[:total])), "Envelope"))
		container := base.GetNodeByPath(root, "@Envelope")
		require.True(t, bytes.Equal(frame[:total], NodeToBytes(container)))
		megacoTestTree(t, zlibJSONTestMessage(t, container), uint64(offset)*8, uint64(total)*8)
		t.Logf("frame=%d input=%d compressed=%d decoded=%d Adler=verified exact-member=true", i+1, len(wire), compressed, length)
	}
	require.Equal(t, []int{22, 8, 2}, []int{empty, other, decodedCount})
}

func TestProtocolCorpusZlibJSONNegatives(t *testing.T) {
	valid := zlibJSONTestStored(t, []byte(`{"value":"fixture"}`))
	for cut := 0; cut < len(valid); cut++ {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(valid[:cut]), zlibJSONTestRule, "ZlibJSONRecord")
		require.Error(t, err, "prefix %d", cut)
	}
	bad := [][]byte{}
	for _, text := range []string{"", " ", "{}{}", "{}x", "[1,]", `{"x":1,}`, `{"x" 1}`, "NaN", "01", "1.", "1e", "+1", `"\x00"`, "\xef\xbb\xbf{}", "\"\xff\"", "\"\n\"", "[", `{"x":`, "true false"} {
		bad = append(bad, zlibJSONTestStored(t, []byte(text)))
	}
	for _, index := range []int{0, 3, 4, 5, 7, len(valid) - 1} {
		wire := bytes.Clone(valid)
		wire[index] ^= 0x80
		bad = append(bad, wire)
	}
	for cut := 6; cut < len(valid); cut++ {
		wire := bytes.Clone(valid[:cut])
		binary.BigEndian.PutUint32(wire, uint32(len(wire)-4))
		bad = append(bad, wire)
	}
	for _, tail := range [][]byte{{0}, valid[4:]} {
		wire := append(bytes.Clone(valid), tail...)
		binary.BigEndian.PutUint32(wire, uint32(len(wire)-4))
		bad = append(bad, wire)
	}
	// Empty dictionary's DICTID is 1; Go can accept it with nil dictionary.
	// This explicit no-dictionary profile must still reject the FDICT bit.
	dictionary := append([]byte{0, 0, 0, 0, 0x78, 0x20, 0, 0, 0, 1}, valid[6:]...)
	binary.BigEndian.PutUint32(dictionary, uint32(len(dictionary)-4))
	bad = append(bad, dictionary)
	for cinfo := byte(0); cinfo <= 7; cinfo++ {
		wire := bytes.Clone(valid)
		wire[4], wire[5] = cinfo<<4|8, 0
		wire[5] = byte((31 - binary.BigEndian.Uint16(wire[4:6])%31) % 31)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), zlibJSONTestRule, "ZlibJSONRecord")
		if cinfo == 7 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "unsupported smaller-window")
			bad = append(bad, wire)
		}
	}
	for i, wire := range bad {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), zlibJSONTestRule, "ZlibJSONRecord")
		require.Error(t, err, "negative %d", i)
		n := zlibJSONTestParse(t, wire, "ZlibJSONCarrier")
		require.Equal(t, wire, NodeToBytes(n))
		latTestField(t, n, "Unparsed Zlib JSON Record", "raw", 0, uint64(len(wire))*8, wire)
		require.Nil(t, protocolCorpusFindNode(n, "Zlib Member"))
	}
}

func TestProtocolCorpusZlibJSONResourceBounds(t *testing.T) {
	for _, size := range []int{65525, 65526} {
		wire := zlibJSONTestStored(t, []byte(`"`+strings.Repeat("x", size-2)+`"`))
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), zlibJSONTestRule, "ZlibJSONRecord")
		if size == 65525 {
			require.NoError(t, err)
			require.Len(t, wire, 65540)
		} else {
			require.ErrorContains(t, err, "boundary")
		}
	}
	for _, size := range []int{1 << 20, (1 << 20) + 1} {
		wire := tencentGamesBuildZlibRecord(t, []byte(`"`+strings.Repeat("x", size-2)+`"`))
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), zlibJSONTestRule, "ZlibJSONRecord")
		if size == 1<<20 {
			require.NoError(t, err)
			wire[len(wire)-1] ^= 1
			_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(wire), zlibJSONTestRule, "ZlibJSONRecord")
			require.ErrorContains(t, err, "Adler32 checksum mismatch")
		} else {
			require.ErrorContains(t, err, "1MiB")
		}
	}
	for _, depth := range []int{128, 129} {
		wire := zlibJSONTestStored(t, []byte(strings.Repeat("[", depth)+"0"+strings.Repeat("]", depth)))
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), zlibJSONTestRule, "ZlibJSONRecord")
		if depth == 128 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "128-container")
		}
	}
	for _, count := range []int{65535, 65536} {
		wire := tencentGamesBuildZlibRecord(t, []byte("["+strings.Repeat("0,", count-1)+"0]"))
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), zlibJSONTestRule, "ZlibJSONRecord")
		if count == 65535 { // array itself also consumes one value
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "65536-value")
		}
	}
}

func TestProtocolCorpusZlibJSONConcurrentIsolation(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			wire := zlibJSONTestStored(t, []byte(fmt.Sprintf(`{"i":%d}`, i)))
			n := zlibJSONTestParse(t, wire, "ZlibJSONRecord")
			require.Equal(t, wire, NodeToBytes(n))
			info := zlibJSONTestInfo(t, n)
			members := info["Decoded Document"].(map[string]any)["Members"].([]any)
			require.Equal(t, json.Number(fmt.Sprint(i)), members[0].(map[string]any)["Value"].(map[string]any)["Value"])
			info["Decoded Bytes"].([]byte)[0] = '!'
			members[0].(map[string]any)["Name"] = "changed"
			again := zlibJSONTestParse(t, wire, "ZlibJSONRecord")
			require.Equal(t, byte('{'), zlibJSONTestInfo(t, again)["Decoded Bytes"].([]byte)[0])
			require.Equal(t, wire, NodeToBytes(again))
		}(i)
	}
	wg.Wait()
	_, err := parser.GenerateBinary(map[string]any{}, zlibJSONTestRule, "ZlibJSONRecord")
	require.ErrorContains(t, err, "structured generation is not supported")
	// Compressed fixture generation above is test-only and does not enable
	// production structured generation or alter the existing framing entry.
	var compressed bytes.Buffer
	w, err := zlib.NewWriterLevel(&compressed, zlib.BestCompression)
	require.NoError(t, err)
	_, err = w.Write([]byte("[]"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	wire := make([]byte, 4)
	binary.BigEndian.PutUint32(wire, uint32(compressed.Len()))
	n := zlibJSONTestParse(t, append(wire, compressed.Bytes()...), "ZlibJSONRecord")
	require.Equal(t, uint8(3), zlibJSONTestInfo(t, n)["FLEVEL"])
}

func TestProtocolCorpusZlibJSONImportedHeldReader(t *testing.T) {
	for _, entry := range []string{"ZlibJSONRecord", "ZlibJSONCarrier"} {
		for offset := uint64(0); offset < 8; offset++ {
			for _, valid := range []bool{true, false} {
				if !valid && entry != "ZlibJSONCarrier" {
					continue
				}
				wire := zlibJSONTestStored(t, []byte(`{"value":1}`))
				if !valid {
					wire[len(wire)-1] ^= 1
				}
				var packed bytes.Buffer
				w := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, w.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, w.WriteBits(wire, uint64(len(wire))*8))
				require.NoError(t, w.WriteBits([]byte{0xd3}, 8))
				if offset > 0 {
					require.NoError(t, w.WriteBits([]byte{0}, 8-offset))
				}
				root := latTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Message").SetMaxLength(%d)
      this.ProcessSubNode("Message")
      this.ProcessSubNode("Sentinel")
      if %d > 0 { this.ProcessSubNode("Padding") }
    Prefix: uint8,%dbit
    Message: "import:application-layer/zlib_json.yaml;node:%s"
    Sentinel: uint8
    Padding: uint8,%dbit
`, offset, len(wire), offset, offset, entry, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("zlib-json-caller", "preserved")
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Envelope"))
				n := base.GetNodeByPath(root, "@Envelope")
				if valid {
					megacoTestTree(t, zlibJSONTestMessage(t, n), offset, offset+uint64(len(wire))*8)
					latTestField(t, n, "Compressed Length", "uint32", offset, offset+32, uint64(len(wire)-4))
					info := zlibJSONTestInfo(t, n)
					require.Equal(t, []byte(`{"value":1}`), info["Decoded Bytes"])
					require.Equal(t, false, info["Decoded Values Have Wire Spans"])
					require.Nil(t, protocolCorpusFindNode(n, "value"))
				} else {
					latTestField(t, n, "Unparsed Zlib JSON Record", "raw", offset, offset+uint64(len(wire))*8, wire)
					require.Nil(t, protocolCorpusFindNode(n, "Zlib Member"))
				}
				protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
				require.Equal(t, "preserved", root.Ctx.GetItem("zlib-json-caller"))
				require.Equal(t, packed.Bytes(), NodeToBytes(n))
				require.NoError(t, r.Recovery())
				got, err := r.ReadBits(uint64(packed.Len()) * 8)
				require.NoError(t, err)
				require.Equal(t, packed.Bytes(), got)
				require.ErrorContains(t, r.PopBackup(), "no backup")
			}
		}
	}
}

func TestProtocolCorpusZlibJSONReadBoundariesAndCompatibility(t *testing.T) {
	valid := zlibJSONTestStored(t, []byte("{}"))
	for _, entry := range []string{"ZlibJSONRecord", "ZlibJSONCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(valid), zlibJSONTestRule, entry)
		require.Error(t, err)
		for _, bits := range []uint64{0, 65541 * 8, uint64(len(valid))*8 - 1, uint64(len(valid))*8 - 2,
			uint64(len(valid))*8 - 3, uint64(len(valid))*8 - 4, uint64(len(valid))*8 - 5,
			uint64(len(valid))*8 - 6, uint64(len(valid))*8 - 7} {
			r := &giopCarrierTestBitReader{Reader: bytes.NewReader(valid), bits: bits}
			_, err := parser.ParseBinary(r, zlibJSONTestRule, entry)
			require.Error(t, err)
			require.Equal(t, len(valid), r.Len(), "direct boundary %d read input", bits)
			root := latTestInline(t, fmt.Sprintf("Package:\n  Message: \"import:application-layer/zlib_json.yaml;node:%s\"\n", entry))
			root.Cfg.SetItem(base.CfgLength, bits)
			reader := bytes.NewReader(valid)
			require.Error(t, root.ParseSubNode(base.NewBitReader(reader), "Message"))
			require.Equal(t, len(valid), reader.Len(), "import boundary %d read input", bits)
		}
	}
	// Carrier fallback shares the same resource cap and may retain a short
	// nonempty input, but cannot allocate an arbitrarily large raw remainder.
	for _, size := range []int{1, 65540} {
		wire := bytes.Repeat([]byte{0x42}, size)
		n := zlibJSONTestParse(t, wire, "ZlibJSONCarrier")
		latTestField(t, n, "Unparsed Zlib JSON Record", "raw", 0, uint64(size)*8, wire)
		require.Equal(t, wire, NodeToBytes(n))
	}
	// Existing generic framing continues to accept raw member-shaped bytes;
	// opting into JSON validation must not retroactively alter that entry.
	badChecksum := bytes.Clone(valid)
	badChecksum[len(badChecksum)-1] ^= 1
	n := protocolCorpusRequireBoundedRuleParse(t, badChecksum, tencentGamesApplicationDataRule, "LengthPrefixedZlibRecord")
	require.Equal(t, badChecksum, NodeToBytes(n))
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(badChecksum), zlibJSONTestRule, "ZlibJSONRecord")
	require.ErrorContains(t, err, "Adler32 checksum mismatch")
	// Even invalid JSON diagnostic messages must not echo the input token.
	privateFixture := "inline-private-placeholder"
	wire := zlibJSONTestStored(t, []byte(`{"`+privateFixture+`":`+privateFixture+`}`))
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(wire), zlibJSONTestRule, "ZlibJSONRecord")
	require.Error(t, err)
	require.NotContains(t, err.Error(), privateFixture)
}
