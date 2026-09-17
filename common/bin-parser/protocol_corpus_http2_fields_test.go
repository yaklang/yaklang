package bin_parser

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"golang.org/x/net/http2"
)

const http2FieldsTestRule = "application-layer.http2_fields"

// Byte/bit oracle independent of the production field planner. Decoded HPACK
// strings are metadata; every actual tree leaf still refers to wire bits.
func http2FieldsTestTree(t *testing.T, n *base.Node, wire []byte, offset uint64) {
	t.Helper()
	// A committed trial import deliberately owns an isolated context. Check
	// strict field-tree context identity inside that boundary, not across it.
	var strict func(*base.Node) *base.Node
	strict = func(node *base.Node) *base.Node {
		if info, ok := node.Cfg.GetItem("additionInfo").(map[string]any); ok {
			if profile, ok := info["Profile"].(string); ok && strings.HasPrefix(profile, "HTTP/2 ") {
				return node
			}
		}
		for _, child := range node.Children {
			if found := strict(child); found != nil {
				return found
			}
		}
		return nil
	}
	fieldRoot := strict(n)
	if fieldRoot == nil {
		fieldRoot = n
	}
	latTestTree(t, fieldRoot, offset, offset+uint64(len(wire))*8)
	require.Equal(t, wire, stream_parser.GetBytesByNode(n))
	var walk func(*base.Node)
	walk = func(n *base.Node) {
		if stream_parser.NodeHasResult(n) {
			span := stream_parser.GetNodeResultPos(n)
			start, end := span[0]-offset, span[1]-offset
			typ := n.Cfg.GetString(base.CfgType)
			result, err := n.Result()
			require.NoError(t, err)
			if strings.HasPrefix(typ, "uint") {
				var want uint64
				for bit := start; bit < end; bit++ {
					want = want<<1 | uint64((wire[bit/8]>>(7-bit%8))&1)
				}
				require.Equal(t, want, uintVal(t, result), n.Name)
			} else {
				require.Zero(t, start%8)
				require.Zero(t, end%8)
				require.Equal(t, tlsCertificateTestSHA(wire[start/8:end/8]), tlsCertificateTestSHA(stream_parser.GetBytesByNode(n)), n.Name)
			}
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(n)
}

func http2FieldsTestWhole(t *testing.T, frame []byte, start, size int, entry string) *base.Node {
	t.Helper()
	tail := len(frame) - start - size
	source := fmt.Sprintf("endian: little\nunit: byte\nPackage:\n  Capture:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Envelope\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      if %d > 0 { this.ProcessSubNode(\"Tail\") }\n    Envelope: raw,%d\n    Message: \"import:application-layer/http2_fields.yaml;node:%s\"\n    Tail: raw,%d\n", start, size, tail, start, entry, tail)
	root := latTestInline(t, source)
	root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
	r := base.NewBitReader(bytes.NewReader(frame))
	require.NoError(t, r.Backup())
	require.NoError(t, root.ParseSubNode(r, "Capture"))
	require.Equal(t, frame, NodeToBytes(base.GetNodeByPath(root, "@Capture")))
	m := protocolCorpusFindNode(root, "Message")
	http2FieldsTestTree(t, m, frame[start:start+size], uint64(start)*8)
	require.NoError(t, r.Recovery())
	got, err := r.ReadBits(uint64(len(frame)) * 8)
	require.NoError(t, err)
	require.Equal(t, frame, got)
	require.ErrorContains(t, r.PopBackup(), "no backup")
	return m
}

func http2FieldsTestFrameOracle(t *testing.T, n *base.Node, w []byte, offset uint64) {
	t.Helper()
	f, err := http2.NewFramer(nil, bytes.NewReader(w)).ReadFrame()
	require.NoError(t, err)
	h := f.Header()
	latTestField(t, n, "Length", "uint32", offset, offset+24, uint64(h.Length))
	latTestField(t, n, "Type", "uint8", offset+24, offset+32, uint64(h.Type))
	latTestField(t, n, "Flags", "uint8", offset+32, offset+40, uint64(h.Flags))
	latTestField(t, n, "Reserved", "uint8", offset+40, offset+41, uint64(w[5]>>7))
	latTestField(t, n, "Stream Identifier", "uint32", offset+41, offset+72, uint64(h.StreamID))
	switch f := f.(type) {
	case *http2.DataFrame:
		protocolCorpusRequireValue(t, n, "Data", f.Data())
	case *http2.HeadersFrame:
		protocolCorpusRequireValue(t, n, "Field Block Fragment", f.HeaderBlockFragment())
	case *http2.SettingsFrame:
		for i := 0; i < f.NumSettings(); i++ {
			s := f.Setting(i)
			setting := protocolCorpusFindNode(n, fmt.Sprintf("Setting %d", i))
			require.NotNil(t, setting)
			latTestField(t, setting, "Identifier", "uint16", offset+uint64(9+i*6)*8, offset+uint64(11+i*6)*8, uint64(s.ID))
			protocolCorpusRequireValue(t, setting, "Value", uint64(s.Val))
		}
	case *http2.WindowUpdateFrame:
		protocolCorpusRequireValue(t, n, "Window Size Increment", uint64(f.Increment))
	case *http2.GoAwayFrame:
		protocolCorpusRequireValue(t, n, "Last Stream Identifier", uint64(f.LastStreamID))
		protocolCorpusRequireValue(t, n, "Error Code", uint64(f.ErrCode))
	default:
		t.Fatalf("unaccounted original frame type %T", f)
	}
}

func TestProtocolCorpusHTTP2FieldsAllOriginalRecords(t *testing.T) {
	const path = "testdata/protocol-corpus/captures/ndpi/ndpi-http2.pcapng"
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "8ca9722db9527618db1f1a35841d01e46d0d82b2e3bde022e5d24e28cff0e598", tlsCertificateTestSHA(raw))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 10)
	streams := map[uint16][]byte{}
	initial := map[uint16]uint32{37824: 474296310, 29518: 2726225307}
	types := map[byte]int{}
	bodyBytes := 0
	for i, record := range records {
		p := gopacket.NewPacket(record, layers.LayerTypeLinuxSLL, gopacket.Default)
		require.Nil(t, p.ErrorLayer())
		tcp, ok := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		require.True(t, ok)
		require.False(t, tcp.SYN)
		require.False(t, tcp.FIN)
		require.Equal(t, "127.0.0.1", p.NetworkLayer().NetworkFlow().Src().String())
		require.Equal(t, "127.0.0.1", p.NetworkLayer().NetworkFlow().Dst().String())
		port := uint16(tcp.SrcPort)
		require.Contains(t, initial, port)
		require.Equal(t, initial[port]+uint32(len(streams[port])), tcp.Seq, "fixture must be contiguous, no guessed gaps or overlaps")
		require.Equal(t, tcp.Payload, record[68:])
		streams[port] = append(streams[port], tcp.Payload...)
		at := 0
		if i == 0 {
			require.Equal(t, http2.ClientPreface, string(tcp.Payload[:24]))
			at = 24
		}
		for at < len(tcp.Payload) {
			size := int(tcp.Payload[at])<<16 | int(tcp.Payload[at+1])<<8 | int(tcp.Payload[at+2])
			size += 9
			require.LessOrEqual(t, at+size, len(tcp.Payload))
			w := tcp.Payload[at : at+size]
			types[w[3]]++
			t.Run(fmt.Sprintf("record-%d-byte-%d", i+1, at+68), func(t *testing.T) {
				n := protocolCorpusRequireBoundedRuleParse(t, w, http2FieldsTestRule, "HTTP2FrameFields")
				http2FieldsTestTree(t, n, w, 0)
				http2FieldsTestFrameOracle(t, n, w, 0)
				physical := http2FieldsTestWhole(t, record, at+68, size, "HTTP2FrameFields")
				http2FieldsTestFrameOracle(t, physical, w, uint64(at+68)*8)
				for cut := 0; cut < size; cut++ {
					_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w[:cut]), http2FieldsTestRule, "HTTP2FrameFields")
					require.Error(t, err, "prefix %d", cut)
				}
			})
			if w[3] == 0 {
				bodyBytes += size - 9
				want := `{"amfStatusUri":"","guamiList":[{"plmnId":{"mcc":"208","mnc":"93"},"amfId":"cafe00"}]}`
				if port == 37824 {
					want = `{"amfStatusUri":"http://127.0.0.1:29507/npcf-callback/v1/amfstatus","guamiList":[{"plmnId":{"mcc":"208","mnc":"93"},"amfId":"cafe00"}]}`
				}
				require.Equal(t, want, string(w[9:])) // original body bytes; no endpoint contact
			}
			at += size
		}
	}
	require.Equal(t, map[byte]int{0: 2, 1: 2, 4: 4, 7: 1, 8: 3}, types)
	require.Equal(t, 221, bodyBytes)
	for _, port := range []uint16{37824, 29518} {
		entry, size, count := "HTTP2InitialClientStream", 319, 5
		want := [][2]string{{":authority", "127.0.0.1:29518"}, {":method", "POST"}, {":path", "/namf-comm/v1/subscriptions"}, {":scheme", "http"}, {"content-type", "application/json"}, {"accept", "application/json"}, {"user-agent", "OpenAPI-Generator/1.0.0/go"}, {"content-length", "135"}, {"accept-encoding", "gzip"}}
		if port == 29518 {
			entry, size, count = "HTTP2InitialServerStream", 272, 7
			want = [][2]string{{":status", "201"}, {"content-type", "application/json"}, {"location", "http://127.0.0.1:29507/npcf-callback/v1/amfstatus/1"}, {"content-length", "86"}, {"date", "Thu, 11 Jun 2020 08:17:40 GMT"}}
		}
		w := streams[port]
		require.Len(t, w, size)
		n := protocolCorpusRequireBoundedRuleParse(t, w, http2FieldsTestRule, entry)
		http2FieldsTestTree(t, n, w, 0)
		info := n.Cfg.GetItem("additionInfo").(map[string]any)
		require.Equal(t, count, info["Frame Count"])
		require.Equal(t, len(want), info["Header Count"])
		for _, key := range []string{"Peer Settings Applied", "Peer Frame Limit Validated", "HTTP Semantics Validated", "Flow Control Validated", "Connection Lifecycle Validated", "TCP Reassembly Performed", "Structured Generation Supported"} {
			require.Equal(t, false, info[key], key)
		}
		blocks := info["Header Blocks"].([]map[string]any)
		require.Len(t, blocks, 1)
		require.Equal(t, uint64(3), blocks[0]["Stream Identifier"])
		headers := blocks[0]["Headers"].([]map[string]any)
		require.Len(t, headers, len(want))
		for i, pair := range want {
			require.Equal(t, pair[0], headers[i]["Name"])
			require.Equal(t, pair[1], headers[i]["Value"])
		}
		// Compressed evidence points into the caller's ordered direction. Locate
		// it back in its original physical HEADERS record without invented spans.
		spans := blocks[0]["Relative Byte Ranges"].([][2]int)
		require.Len(t, spans, 1)
		record := records[1]
		if port == 29518 {
			record = records[6]
		}
		require.Equal(t, record[77:], w[spans[0][0]:spans[0][1]])
		require.Equal(t, false, blocks[0]["Decoded Strings Have Direct Wire Spans"])
	}
}

func TestProtocolCorpusHTTP2FieldsBoundariesAndIsolation(t *testing.T) {
	frame := latTestBytes(t, "00000408000000000080000001")
	settings := latTestBytes(t, "000000040000000000")
	for _, entry := range []string{"HTTP2FrameFields", "HTTP2InitialClientStream", "HTTP2InitialServerStream"} {
		wire := bytes.Clone(frame)
		if entry != "HTTP2FrameFields" {
			wire = append(bytes.Clone(settings), wire...)
		}
		if entry == "HTTP2InitialClientStream" {
			wire = append([]byte(http2.ClientPreface), wire...)
		}
		for _, name := range []string{entry, entry + "Carrier"} {
			_, err := parser.ParseBinary(bytes.NewReader(wire), http2FieldsTestRule, name)
			require.ErrorContains(t, err, "explicit")
			_, err = parser.GenerateBinary(map[string]any{}, http2FieldsTestRule, name)
			require.Error(t, err)
			for _, bits := range []uint64{0, 1, 7, (1<<20)*8 + 1, ((1 << 20) + 1) * 8} {
				r := &tlsSHTestHeldReader{bits: bits}
				_, err := parser.ParseBinary(r, http2FieldsTestRule, name)
				require.Error(t, err)
				require.Zero(t, r.reads)
			}
		}
		for offset := uint64(0); offset < 8; offset++ {
			for _, good := range []bool{true, false} {
				w := bytes.Clone(wire)
				if !good {
					w[len(w)-1] = 0
				}
				var packed bytes.Buffer
				writer := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, writer.WriteBits(w, uint64(len(w))*8))
				require.NoError(t, writer.WriteBits([]byte{0xd3}, 8))
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0}, 8-offset))
				}
				root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/http2_fields.yaml;node:%sCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(w), offset, offset, entry, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("marker", 123)
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Wrapped"))
				m := protocolCorpusFindNode(root, "Message")
				http2FieldsTestTree(t, m, w, offset)
				if good {
					protocolCorpusRequireValue(t, m, "Window Size Increment", uint64(1))
				} else {
					protocolCorpusRequireValue(t, m, "Unparsed HTTP2 Wire", w)
					require.Nil(t, protocolCorpusFindNode(m, "Frame"))
				}
				protocolCorpusRequireValue(t, root, "Sentinel", uint64(0xd3))
				require.Equal(t, 123, root.Ctx.GetItem("marker"))
				require.Equal(t, packed.Bytes(), NodeToBytes(base.GetNodeByPath(root, "@Wrapped")))
				require.NoError(t, r.Recovery())
				got, err := r.ReadBits(uint64(packed.Len()) * 8)
				require.NoError(t, err)
				require.Equal(t, packed.Bytes(), got)
				require.ErrorContains(t, r.PopBackup(), "no backup")
				_, err = r.ReadBits(8)
				require.ErrorIs(t, err, io.EOF)
			}
		}
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("isolated-%d", worker), func(t *testing.T) {
			t.Parallel()
			// A local dynamic entry must not leak through the shared rule cache.
			block := []byte{0x40, 1, 'x', 1, byte('a' + worker), 0xbe}
			w := append(bytes.Clone(settings), []byte{0, 0, byte(len(block)), 1, 4, 0, 0, 0, 1}...)
			w = append(w, block...)
			cfg := map[string]any{"marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, w, http2FieldsTestRule, "HTTP2InitialServerStream", cfg)
			h := n.Cfg.GetItem("additionInfo").(map[string]any)["Header Blocks"].([]map[string]any)[0]["Headers"].([]map[string]any)
			require.Len(t, h, 2)
			require.Equal(t, string(byte('a'+worker)), h[0]["Value"])
			require.Equal(t, h[0]["Value"], h[1]["Value"])
			require.Equal(t, map[string]any{"marker": worker}, cfg)
			bad := append(bytes.Clone(settings), []byte{0, 0, 1, 1, 4, 0, 0, 0, 1, 0xbe}...)
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), http2FieldsTestRule, "HTTP2InitialServerStream")
			require.Error(t, err)
		})
	}
}

func TestProtocolCorpusHTTP2FieldsInitialBoundaries(t *testing.T) {
	// Complete frame boundaries are valid initial snapshots. Partial frames,
	// wrong prefaces and a following incomplete field block are not.
	settings := latTestBytes(t, "000000040000000000")
	for i := range []byte(http2.ClientPreface) {
		w := append([]byte(http2.ClientPreface), settings...)
		w[i] ^= 1
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), http2FieldsTestRule, "HTTP2InitialClientStream")
		require.Error(t, err)
	}
	for cut := 0; cut < 33; cut++ {
		w := append([]byte(http2.ClientPreface), settings...)[:cut]
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), http2FieldsTestRule, "HTTP2InitialClientStream")
		require.Error(t, err)
	}
	for _, entry := range []string{"HTTP2InitialClientStream", "HTTP2InitialServerStream"} {
		w := bytes.Clone(settings)
		if entry == "HTTP2InitialClientStream" {
			w = append([]byte(http2.ClientPreface), w...)
		}
		protocolCorpusRequireBoundedRuleParse(t, w, http2FieldsTestRule, entry)
		for _, tail := range [][]byte{{0}, {0, 0, 1, 1, 0, 0, 0, 0, 1, 0x82}, {0, 0, 1, 1, 4, 0, 0, 0, 1, 0x40}} {
			bad := append(bytes.Clone(w), tail...)
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), http2FieldsTestRule, entry)
			require.Error(t, err)
			n := protocolCorpusRequireBoundedRuleParse(t, bad, http2FieldsTestRule, entry+"Carrier")
			protocolCorpusRequireValue(t, n, "Unparsed HTTP2 Wire", bad)
			require.Nil(t, protocolCorpusFindNode(n, "Frame"))
		}
	}
	// A bounded frame never consumes the following complete frame.
	w := append(bytes.Clone(settings), settings...)
	n := http2FieldsTestWhole(t, w, 0, len(settings), "HTTP2FrameFields")
	protocolCorpusRequireValue(t, n, "Type", uint64(4))
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), http2FieldsTestRule, "HTTP2FrameFields")
	require.Error(t, err)
}
