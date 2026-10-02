package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const http3TestRule = "application-layer.http3"

func http3TestHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

// Independent RFC 9114/9204 wire construction: GET/https/"/" are static
// indices 17/23/1; authority is a literal with static name index zero.
func http3TestRequest(extra []byte, dynamic bool) []byte {
	block := []byte{0, 0, 0xd1, 0xd7, 0xc1, 0x50, 11}
	if dynamic {
		block[0] = 2
	}
	block = append(block, []byte("example.com")...)
	block = append(block, extra...)
	return append([]byte{1, byte(len(block))}, block...)
}

func http3TestConfig(encoder []byte, capacity uint64) map[string]any {
	return map[string]any{"http3QPACKEncoderStream": encoder, "http3QPACKMaxTableCapacity": capacity}
}

func http3TestParse(t *testing.T, wire []byte, entry string, config map[string]any) *base.Node {
	t.Helper()
	n, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), http3TestRule, config, entry)
	require.NoError(t, err)
	_, err = n.Result()
	require.NoError(t, err)
	require.Equal(t, wire, NodeToBytes(n))
	return n
}

func http3TestProtocolNode(t *testing.T, n *base.Node) *base.Node {
	t.Helper()
	for _, name := range []string{"HTTP3RequestStream", "HTTP3UnidirectionalStream"} {
		if found := protocolCorpusFindNode(n, name); found != nil {
			return found
		}
	}
	t.Fatal("missing exact HTTP3 node")
	return nil
}

// Every actual wire bit belongs to exactly one terminal. Independently read
// its numeric/raw value from the input; metadata never masquerades as wire.
func http3TestWireTree(t *testing.T, n *base.Node, wire []byte, offset uint64) {
	t.Helper()
	cursor := offset
	var visit func(*base.Node)
	visit = func(node *base.Node) {
		if protocolCorpusNodeHasResult(node) {
			r, err := node.Result()
			require.NoError(t, err)
			require.Same(t, node, r.Origin)
		}
		if stream_parser.NodeHasResult(node) {
			span := stream_parser.GetNodeResultPos(node)
			require.Equal(t, cursor, span[0], node.Name)
			require.GreaterOrEqual(t, span[1], span[0])
			require.LessOrEqual(t, span[1], offset+uint64(len(wire))*8)
			bits := span[1] - span[0]
			out := make([]byte, (bits+7)/8)
			var integer uint64
			for i := uint64(0); i < bits; i++ {
				p := span[0] - offset + i
				bit := (wire[p/8] >> (7 - p%8)) & 1
				out[i/8] |= bit << (7 - i%8)
				integer = integer<<1 | uint64(bit)
			}
			r, err := node.Result()
			require.NoError(t, err)
			switch node.Cfg.GetItem(base.CfgType) {
			case "uint8":
				require.Equal(t, uint8(integer), r.Value, node.Name)
			case "uint64":
				require.Equal(t, integer, r.Value, node.Name)
			case "raw":
				require.Equal(t, out, r.Value, node.Name)
			case "string":
				require.Equal(t, string(out), r.Value, node.Name)
			default:
				t.Fatalf("unexpected terminal type %v", node.Cfg.GetItem(base.CfgType))
			}
			cursor = span[1]
			return
		}
		for _, child := range node.Children {
			require.Same(t, node, child.Cfg.GetItem(base.CfgParent))
			require.Same(t, node.Ctx, child.Ctx)
			visit(child)
		}
	}
	visit(n)
	require.Equal(t, offset+uint64(len(wire))*8, cursor)
}

func TestProtocolCorpusHTTP3IndependentFields(t *testing.T) {
	encoder := []byte{2, 0x3f, 0x61, 0x41, 'x', 1, 'y'} // capacity128, literal x:y
	for _, fixture := range []struct {
		name, entry, literal string
		wire, encoder        []byte
		capacity             uint64
	}{
		{name: "static request", entry: "HTTP3RequestStream", wire: http3TestRequest(nil, false)},
		{name: "dynamic request", entry: "HTTP3RequestStream", wire: http3TestRequest([]byte{0x80}, true), encoder: encoder, capacity: 128},
		{name: "empty settings", entry: "HTTP3UnidirectionalStream", literal: "000400"},
		{name: "settings and extension", entry: "HTTP3UnidirectionalStream", literal: "000407014080070108012102aabb"},
		{name: "encoder", entry: "HTTP3UnidirectionalStream", wire: encoder, capacity: 128},
		{name: "empty encoder", entry: "HTTP3UnidirectionalStream", literal: "02"},
		{name: "decoder", entry: "HTTP3UnidirectionalStream", literal: "03804401"},
		{name: "unknown stream", entry: "HTTP3UnidirectionalStream", literal: "21aabb"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			wire := fixture.wire
			if wire == nil {
				wire = http3TestHex(t, fixture.literal)
			}
			for _, entry := range []string{fixture.entry, fixture.entry + "Carrier"} {
				n := http3TestParse(t, wire, entry, http3TestConfig(fixture.encoder, fixture.capacity))
				n = http3TestProtocolNode(t, n)
				http3TestWireTree(t, n, wire, 0)
				info := alljoynTestInfo(t, n)
				for _, key := range []string{"QUIC Decryption Performed", "QUIC Reassembly Performed", "Connection State Validated", "Endpoint Identity Proven", "Transport FIN Observed"} {
					require.Equal(t, false, info[key])
				}
				if fixture.entry == "HTTP3RequestStream" {
					require.Equal(t, "GET", info[":method"])
					require.Equal(t, "https", info[":scheme"])
					require.Equal(t, "example.com", info[":authority"])
					require.Equal(t, "/", info[":path"])
					headers := info["Request Headers"].([]map[string]any)
					require.Len(t, headers, 4+len(fixture.encoder)/7)
					if len(fixture.encoder) > 0 {
						require.Equal(t, "x", headers[4]["Name"])
						require.Equal(t, "y", headers[4]["Value"])
						require.Equal(t, "qpack-encoder-stream-relative-bits", headers[4]["Value Source"].(map[string]any)["Coordinate System"])
					}
				}
			}
		})
	}
}

type http3TestDerived struct {
	Source        string `json:"source_capture"`
	SourceSHA     string `json:"source_capture_sha256"`
	FragmentCount int    `json:"fragment_count"`
	StreamCount   int    `json:"stream_count"`
	Streams       []struct {
		ID        int    `json:"stream_id"`
		Length    int    `json:"length"`
		SHA       string `json:"sha256"`
		Hex       string `json:"hex"`
		FIN       bool   `json:"fin_observed"`
		Fragments []struct {
			Frame          int    `json:"frame"`
			Offset         int    `json:"offset"`
			Length         int    `json:"length"`
			Hex            string `json:"hex"`
			SHA            string `json:"sha256"`
			FIN            bool   `json:"fin"`
			Retransmission bool   `json:"retransmission"`
		} `json:"fragments"`
	} `json:"streams"`
}

func http3TestDerivedStreams(t *testing.T) (http3TestDerived, map[int][]byte) {
	t.Helper()
	var d http3TestDerived
	b, err := os.ReadFile("testdata/protocol-corpus/derived/wireshark-http3-qpack.streams.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &d))
	require.Equal(t, 4, d.StreamCount)
	require.Equal(t, 6, d.FragmentCount)
	require.Len(t, d.Streams, 4)
	wires := map[int][]byte{}
	count := 0
	for _, s := range d.Streams {
		wire := http3TestHex(t, s.Hex)
		require.Len(t, wire, s.Length)
		require.Equal(t, s.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
		var joined []byte
		for _, f := range s.Fragments {
			count++
			require.False(t, f.Retransmission)
			require.Equal(t, len(joined), f.Offset)
			part := http3TestHex(t, f.Hex)
			require.Len(t, part, f.Length)
			require.Equal(t, f.SHA, fmt.Sprintf("%x", sha256.Sum256(part)))
			require.Contains(t, []int{2, 4, 7, 8}, f.Frame)
			joined = append(joined, part...)
		}
		require.Equal(t, wire, joined)
		require.Equal(t, s.ID == 0, s.FIN)
		wires[s.ID] = wire
	}
	require.Equal(t, 6, count)
	return d, wires
}

func TestProtocolCorpusHTTP3AllDerivedAndOriginalRecords(t *testing.T) {
	d, wires := http3TestDerivedStreams(t)
	require.Equal(t, "captures/wireshark-tests/wireshark-http3-qpack.pcapng", d.Source)
	require.Equal(t, "09aa88ee755cae2c441af94c7da2a2577de8d761d19b4521c753d10bcdd1d051", d.SourceSHA)
	path := "testdata/protocol-corpus/" + d.Source
	original, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, d.SourceSHA, fmt.Sprintf("%x", sha256.Sum256(original)))
	// This source is LINKTYPE_RAW: its first byte is IPv4, with no Ethernet
	// header. Original packet coordinates and decrypted stream coordinates
	// are separate; no synthetic link header is introduced into either.
	capture, err := pcapgo.NewNgReader(bytes.NewReader(original), pcapgo.DefaultNgReaderOptions)
	require.NoError(t, err)
	require.Equal(t, layers.LinkTypeRaw, capture.LinkType())
	for index, size := range []int{1278, 105, 1278, 95, 106, 59, 1274, 398, 143, 53} {
		wire, ci, err := capture.ReadPacketData()
		require.NoError(t, err)
		require.Len(t, wire, size)
		require.Equal(t, size, ci.CaptureLength)
		require.Equal(t, size, ci.Length)
		n := protocolCorpusRequireBoundedRuleParse(t, wire, "internet_protocol", "Internet Protocol")
		require.Equal(t, wire, NodeToBytes(n))
		require.Nil(t, protocolCorpusFindNode(n, "HTTP3RequestStream"))
		miopTestField(t, n, "Version", uint8(4), 0, 4)
		miopTestField(t, n, "Header Length", uint8(5), 4, 8)
		miopTestField(t, n, "Total Length", uint16(size), 16, 32)
		miopTestField(t, n, "Protocol", uint8(17), 72, 80)
		src, dst := []byte{10, 0, 2, 15}, []byte{127, 0, 0, 1}
		if index == 2 || index == 3 || index == 8 || index == 9 {
			src, dst = dst, src
		}
		miopTestField(t, n, "Source", src, 96, 128)
		miopTestField(t, n, "Destination", dst, 128, 160)
	}
	_, _, err = capture.ReadPacketData()
	require.ErrorIs(t, err, io.EOF)
	for _, id := range []int{2, 3, 10, 0} {
		entry := "HTTP3UnidirectionalStream"
		config := http3TestConfig(nil, 65536)
		if id == 0 {
			entry = "HTTP3RequestStream"
			config = http3TestConfig(wires[10], 65536)
		}
		for _, e := range []string{entry, entry + "Carrier"} {
			n := http3TestParse(t, wires[id], e, config)
			n = http3TestProtocolNode(t, n)
			http3TestWireTree(t, n, wires[id], 0)
			info := alljoynTestInfo(t, n)
			switch id {
			case 2, 3:
				settings := info["Settings"].(map[uint64]uint64)
				require.Equal(t, uint64(65536), settings[1])
				require.Equal(t, uint64(100), settings[7])
				if id == 2 {
					require.Equal(t, uint64(262144), settings[6])
					require.Equal(t, 3, info["Frame Count"])
					protocolCorpusRequireValue(t, n, "Priority Field Value", "u=1, i")
				} else {
					require.Equal(t, uint64(65536), settings[6])
					require.Equal(t, uint64(1), settings[8])
					require.Equal(t, 2, info["Frame Count"])
				}
			case 10:
				require.Equal(t, uint64(21), info["QPACK Insert Count"])
				require.Equal(t, uint64(65536), info["QPACK Current Capacity"])
				require.Len(t, n.Children, 24)
			case 0:
				require.Equal(t, 2, info["Frame Count"])
				require.Equal(t, uint64(2), info["Data Bytes"])
				require.Equal(t, uint64(2), info["Content Length"])
				headers := info["Request Headers"].([]map[string]any)
				require.Len(t, headers, 25)
				expected := [][2]string{{":method", "POST"}, {":authority", "rr2---sn-q4fzen7l.googlevideo.com"}, {":scheme", "https"}, {":path", ""}, {"content-length", "2"}, {"sec-ch-ua", `"Chromium";v="119", "Not?A_Brand";v="24"`}, {"sec-ch-ua-mobile", "?0"}, {"user-agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36"}, {"sec-ch-ua-arch", `"x86"`}, {"sec-ch-ua-full-version", `"119.0.6045.123"`}, {"sec-ch-ua-platform-version", `"6.6.1"`}, {"sec-ch-ua-full-version-list", `"Chromium";v="119.0.6045.123", "Not?A_Brand";v="24.0.0.0"`}, {"sec-ch-ua-bitness", `"64"`}, {"sec-ch-ua-model", `""`}, {"sec-ch-ua-wow64", "?0"}, {"sec-ch-ua-platform", `"Linux"`}, {"accept", "*/*"}, {"origin", "https://www.youtube.com"}, {"x-client-data", "CKCGywE="}, {"sec-fetch-site", "cross-site"}, {"sec-fetch-mode", "cors"}, {"sec-fetch-dest", "empty"}, {"referer", "https://www.youtube.com/"}, {"accept-encoding", "gzip, deflate, br"}, {"accept-language", "en-US,en;q=0.9"}}
				for i, h := range headers {
					require.Equal(t, expected[i][0], h["Name"])
					if i == 3 {
						require.Len(t, h["Value"], 1293)
						require.Equal(t, "ee6b1430158cc318ddcf1e91b6d558fa3b182c76225dacb46cf7a24ec5d5a2e1", fmt.Sprintf("%x", sha256.Sum256([]byte(h["Value"].(string)))))
					} else {
						require.Equal(t, expected[i][1], h["Value"])
					}
					require.Equal(t, [2]uint64{uint64(4+i) * 8, uint64(5+i) * 8}, h["Representation Bit Span"])
				}
				protocolCorpusRequireValue(t, n, "Data", []byte{0x78, 0})
			}
		}
	}
	_, err = parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wires[0]), http3TestRule, http3TestConfig(nil, 65536), "HTTP3RequestStream")
	require.ErrorContains(t, err, "blocked")
	_, err = parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wires[10][:1202]), http3TestRule, http3TestConfig(nil, 65536), "HTTP3UnidirectionalStream")
	require.Error(t, err)
	for cut := 0; cut < len(wires[0]); cut++ {
		http3TestReject(t, wires[0][:cut], "HTTP3RequestStream", http3TestConfig(wires[10], 65536))
	}
}

func http3TestReject(t *testing.T, wire []byte, entry string, config map[string]any) {
	t.Helper()
	_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), http3TestRule, config, entry)
	require.Error(t, err)
	if len(wire) == 0 {
		return
	}
	n := http3TestParse(t, wire, entry+"Carrier", config)
	name := "Unparsed HTTP3 Request Stream"
	if entry == "HTTP3UnidirectionalStream" {
		name = "Unparsed HTTP3 Unidirectional Stream"
	}
	protocolCorpusRequireValue(t, n, name, wire)
	require.Nil(t, protocolCorpusFindNode(n, "Frame 0"))
	require.Nil(t, protocolCorpusFindNode(n, "Stream Type"))
}

func TestProtocolCorpusHTTP3PrefixesAndMalformedBoundaries(t *testing.T) {
	request := http3TestRequest(nil, false)
	for cut := 0; cut < len(request); cut++ {
		http3TestReject(t, request[:cut], "HTTP3RequestStream", nil)
	}
	for _, literal := range []string{"00", "0004", "000401", "0004020100ff", "00040401000100", "0004020200", "0004020802", "0004000400", "0004000200", "000000", "000400800f07000101", "000400800f070002001f", "0300", "02ff", "023f61c063", "0220", "0241610162", "01"} {
		http3TestReject(t, http3TestHex(t, literal), "HTTP3UnidirectionalStream", nil)
	}
	for _, extra := range [][]byte{{0}, {2, 0}, {4, 0}, {1, 2, 0, 0, 0, 0}, {0, 1}, {0, 1, 0, 1, 2, 0, 0, 0, 0}} {
		http3TestReject(t, append(append([]byte{}, request...), extra...), "HTTP3RequestStream", nil)
	}
	// Nonminimal QUIC integers are permitted, not silently canonicalized.
	wide := append([]byte{0x40, 1, 0x40, byte(len(request) - 2)}, request[2:]...)
	n := http3TestParse(t, wide, "HTTP3RequestStream", nil)
	http3TestWireTree(t, n, wide, 0)
	// Prefixes ending on complete encoder/decoder instructions are valid
	// observed prefixes. Mid-instruction cuts are rejected, not all cuts.
	encoder := []byte{2, 0x3f, 0x61, 0x41, 'x', 1, 'y'}
	for cut := 0; cut < len(encoder); cut++ {
		if cut == 1 || cut == 3 {
			http3TestParse(t, encoder[:cut], "HTTP3UnidirectionalStream", http3TestConfig(nil, 128))
		} else {
			http3TestReject(t, encoder[:cut], "HTTP3UnidirectionalStream", http3TestConfig(nil, 128))
		}
	}
}

func TestProtocolCorpusHTTP3HTTPFieldsAndResourceBounds(t *testing.T) {
	for _, extra := range [][]byte{
		{0x21, 'X', 1, 'a'}, // uppercase field name
		{0x21, 'x', 1, 1}, {0x21, 'x', 1, 0x7f}, {0x21, 'x', 1, ' '},
		{0xd1},         // duplicate pseudo-header
		{0x54, 1, '1'}, // content-length one, no DATA
		{0x22, 't', 'e', 3, 'b', 'a', 'd'},
	} {
		http3TestReject(t, http3TestRequest(extra, false), "HTTP3RequestStream", nil)
	}
	extra := append([]byte{0x22, 't', 'e', 8}, []byte("Trailers")...)
	http3TestParse(t, http3TestRequest(extra, false), "HTTP3RequestStream", nil)
	scheme := bytes.Replace(http3TestRequest(nil, false), []byte{0xd7}, []byte{0x5f, 8, 5, 'H', 'T', 'T', 'P', 'S'}, 1)
	scheme[1] = byte(len(scheme) - 2)
	n := http3TestParse(t, scheme, "HTTP3RequestStream", nil)
	require.Equal(t, "HTTPS", alljoynTestInfo(t, n)[":scheme"])
	for _, authority := range []string{"example/a", "example?x", "example#x", "x@example", "[::1"} {
		wire := http3TestRequest(nil, false)
		wire = append(wire[:8], append([]byte{byte(len(authority))}, []byte(authority)...)...)
		wire[1] = byte(len(wire) - 2)
		http3TestReject(t, wire, "HTTP3RequestStream", nil)
	}
	reader := &alljoynTestBitBoundaryReader{Reader: bytes.NewReader([]byte{2}), bits: (1048576 + 1) * 8}
	_, err := parser.ParseBinary(reader, http3TestRule, "HTTP3UnidirectionalStream")
	require.Error(t, err)
	require.Equal(t, 1, reader.Len())
	for _, cfg := range []map[string]any{{"http3QPACKMaxTableCapacity": -1}, {"http3QPACKMaxTableCapacity": 1048577}, {"http3QPACKMaxTableCapacity": "128"}, {"http3QPACKEncoderStream": "02"}, {"http3QPACKEncoderStream": make([]byte, 1048577)}} {
		http3TestReject(t, []byte{2}, "HTTP3UnidirectionalStream", cfg)
	}
	for _, entry := range []string{"HTTP3RequestStream", "HTTP3UnidirectionalStream"} {
		_, err := parser.ParseBinary(bytes.NewReader([]byte{2}), http3TestRule, entry)
		require.Error(t, err)
		for _, bits := range []uint64{1, 7, 9, 15} {
			r := &alljoynTestBitBoundaryReader{Reader: bytes.NewReader([]byte{2, 0}), bits: bits}
			_, err := parser.ParseBinary(r, http3TestRule, entry)
			require.Error(t, err)
			require.Equal(t, 2, r.Len())
		}
		_, err = parser.GenerateBinary(map[string]any{}, http3TestRule, entry)
		require.Error(t, err)
	}
}

// Independent prefixed-integer serialization for boundary inputs only.
func http3TestPrefixed(prefix uint, high byte, n uint64) []byte {
	mask := uint64(1<<prefix) - 1
	if n < mask {
		return []byte{high | byte(n)}
	}
	b := []byte{high | byte(mask)}
	for n -= mask; n >= 128; n >>= 7 {
		b = append(b, byte(n&127)|128)
	}
	return append(b, byte(n))
}

func TestProtocolCorpusHTTP3ExactResourceLimits(t *testing.T) {
	// Maximum stream size is accepted for an explicitly unknown stream, but
	// no unknown payload semantics are claimed. One extra byte is rejected.
	wire := make([]byte, 1048576)
	wire[0] = 0x21
	n := http3TestParse(t, wire, "HTTP3UnidirectionalStream", nil)
	require.Equal(t, false, alljoynTestInfo(t, n)["Unknown Stream Semantics Decoded"])
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(append(wire, 0)), http3TestRule, "HTTP3UnidirectionalStream")
	require.ErrorContains(t, err, "1048576")
	// The SETTINGS frame counts toward the 4096 frame bound.
	frames := append([]byte{0, 4, 0}, bytes.Repeat([]byte{0x21, 0}, 4095)...)
	n = http3TestParse(t, frames, "HTTP3UnidirectionalStream", nil)
	require.Equal(t, 4096, alljoynTestInfo(t, n)["Frame Count"])
	http3TestReject(t, append(frames, 0x21, 0), "HTTP3UnidirectionalStream", nil)
	// Zero *current* capacity can be set with a nonzero observed maximum.
	encoder := append([]byte{2}, bytes.Repeat([]byte{0x20}, 4096)...)
	http3TestParse(t, encoder, "HTTP3UnidirectionalStream", http3TestConfig(nil, 128))
	http3TestReject(t, append(encoder, 0x20), "HTTP3UnidirectionalStream", http3TestConfig(nil, 128))
	// Exact 65536-octet values are decoded, not silently truncated. Repeated
	// dynamic references independently exercise decoded-field size limits.
	encoder = append([]byte{2}, http3TestPrefixed(5, 0x20, 1048576)...)
	encoder = append(encoder, 0x41, 'x')
	encoder = append(encoder, http3TestPrefixed(7, 0, 65536)...)
	encoder = append(encoder, bytes.Repeat([]byte{'a'}, 65536)...)
	n = http3TestParse(t, encoder, "HTTP3UnidirectionalStream", http3TestConfig(nil, 1048576))
	entry := protocolCorpusFindNode(n, "Encoder Instruction 1")
	require.Len(t, alljoynTestInfo(t, entry)["Value"], 65536)
	http3TestParse(t, http3TestRequest(bytes.Repeat([]byte{0x80}, 15), true), "HTTP3RequestStream", http3TestConfig(encoder, 1048576))
	http3TestReject(t, http3TestRequest(bytes.Repeat([]byte{0x80}, 16), true), "HTTP3RequestStream", http3TestConfig(encoder, 1048576))
	tooLong := append([]byte{2}, http3TestPrefixed(5, 0x20, 1048576)...)
	tooLong = append(tooLong, 0x41, 'x')
	tooLong = append(tooLong, http3TestPrefixed(7, 0, 65537)...)
	tooLong = append(tooLong, bytes.Repeat([]byte{'a'}, 65537)...)
	http3TestReject(t, tooLong, "HTTP3UnidirectionalStream", http3TestConfig(nil, 1048576))
}

func TestProtocolCorpusHTTP3ImportsRollbackAndIsolation(t *testing.T) {
	wire := http3TestRequest(nil, false)
	for offset := 0; offset < 8; offset++ {
		prefix, pad := "", ""
		if offset > 0 {
			prefix = fmt.Sprintf("    Prefix: uint8,%dbit\n", offset)
			pad = fmt.Sprintf("    Padding: uint8,%dbit\n", 8-offset)
		}
		root := fcoeTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Stream").SetMaxLength(%d)
      this.ProcessSubNode("Stream")
      this.ProcessSubNode("Suffix")
      if %d > 0 { this.ProcessSubNode("Padding") }
%s    Stream: "import:application-layer/http3.yaml;node:HTTP3RequestStreamCarrier"
    Suffix: uint8
%s`, offset, len(wire), offset, prefix, pad))
		var fs []snaTestField
		if offset > 0 {
			fs = append(fs, snaBit("Prefix", uint8(1<<offset-1), uint64(offset)))
		}
		fs = append(fs, snaRaw("Stream", wire), snaU8("Suffix", 0x5a))
		if offset > 0 {
			fs = append(fs, snaBit("Padding", 0, uint64(8-offset)))
		}
		input := snaEncode(fs)
		root.Cfg.SetItem(base.CfgLength, uint64(len(input))*8)
		r := base.NewBitReader(bytes.NewReader(input))
		require.NoError(t, r.Backup())
		require.NoError(t, root.ParseSubNode(r, "Envelope"))
		n := base.GetNodeByPath(root, "@Envelope")
		http3TestWireTree(t, http3TestProtocolNode(t, n), wire, uint64(offset))
		require.Equal(t, input, NodeToBytes(n))
		miopTestField(t, n, "Suffix", uint8(0x5a), uint64(offset+len(wire)*8), uint64(offset+(len(wire)+1)*8))
		require.NoError(t, r.Recovery())
		replay, err := r.ReadBits(uint64(len(input)) * 8)
		require.NoError(t, err)
		require.Equal(t, input, replay)
	}
	for _, valid := range []bool{false, true} {
		for _, rollback := range []bool{false, true} {
			input := append([]byte{}, wire...)
			if !valid {
				input[2] = 1
			}
			r := base.NewBitReader(bytes.NewReader(append(input, 0x5a)))
			require.NoError(t, r.Backup())
			root, err := base.ParseRule("application-layer/http3.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(input))*8)
			require.NoError(t, root.ParseSubNode(r, "HTTP3RequestStreamCarrier"))
			n := base.GetNodeByPath(root, "@HTTP3RequestStreamCarrier")
			require.Equal(t, input, NodeToBytes(n))
			require.Equal(t, valid, protocolCorpusFindNode(n, "Frame 0") != nil)
			if rollback {
				require.NoError(t, r.Recovery())
				p, err := r.ReadBits(uint64(len(input)) * 8)
				require.NoError(t, err)
				require.Equal(t, input, p)
			} else {
				require.NoError(t, r.PopBackup())
			}
			suffix, err := r.ReadBits(8)
			require.NoError(t, err)
			require.Equal(t, []byte{0x5a}, suffix)
			require.ErrorContains(t, r.Recovery(), "no backup")
		}
	}
	for _, cut := range []int{1, len(wire) - 1} {
		r := base.NewBitReader(bytes.NewReader(wire[:cut]))
		require.NoError(t, r.Backup())
		root, err := base.ParseRule("application-layer/http3.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
		require.Error(t, root.ParseSubNode(r, "HTTP3RequestStreamCarrier"))
		require.NoError(t, r.Recovery())
		p, err := r.ReadBits(uint64(cut) * 8)
		require.NoError(t, err)
		require.Equal(t, wire[:cut], p)
		_, err = r.ReadBits(8)
		require.ErrorIs(t, err, io.EOF)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			encoder := []byte{2, 0x3f, 0x61, 0x41, 'x', 1, byte('a' + i)}
			input := http3TestRequest([]byte{0x80}, true)
			if i%3 == 0 {
				encoder = nil
			}
			n, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(input), http3TestRule, http3TestConfig(encoder, 128), "HTTP3RequestStreamCarrier")
			if err == nil {
				_, err = n.Result()
			}
			if err == nil {
				if !bytes.Equal(input, NodeToBytes(n)) {
					err = fmt.Errorf("wire isolation")
				}
				raw := protocolCorpusFindNode(n, "Unparsed HTTP3 Request Stream")
				if (raw != nil) != (i%3 == 0) {
					err = fmt.Errorf("QPACK state leaked")
				}
				if raw == nil {
					p := protocolCorpusFindNode(n, "HTTP3RequestStream")
					h := alljoynTestInfo(t, p)["Request Headers"].([]map[string]any)
					if h[4]["Value"] != string([]byte{byte('a' + i)}) {
						err = fmt.Errorf("dynamic value leaked")
					}
				}
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}
