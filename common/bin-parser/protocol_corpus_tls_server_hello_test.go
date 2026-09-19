package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

type tlsSHTestSegment struct {
	frame   int
	seq     uint32
	payload []byte
}
type tlsSHTestFlow struct {
	syn      bool
	initial  uint32
	segments []tlsSHTestSegment
}
type tlsSHTestObserved struct {
	frame, offset     int
	record, handshake []byte
}

// Independent corpus oracle: only begin at an observed SYN+1, rebuild each
// direction by sequence number, retain/check retransmission overlaps and stop
// at a gap. Walk TLS lengths (never search for magic). Stop at CCS/application
// data: type 22 after CCS can be protected and is not plaintext evidence.
func tlsSHTestObserve(t *testing.T, records [][]byte, rawIP bool) []tlsSHTestObserved {
	t.Helper()
	flows := map[string]*tlsSHTestFlow{}
	for i, record := range records {
		first := layers.LayerTypeEthernet
		if rawIP {
			first = layers.LayerTypeIPv4
		}
		packet := gopacket.NewPacket(record, first, gopacket.Default)
		tcp, ok := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if !ok || packet.NetworkLayer() == nil {
			continue
		}
		key := packet.NetworkLayer().NetworkFlow().String() + "/" + tcp.TransportFlow().String()
		flow := flows[key]
		if flow == nil {
			flow = &tlsSHTestFlow{}
			flows[key] = flow
		}
		if tcp.SYN {
			if flow.syn {
				require.Equal(t, flow.initial, tcp.Seq+1, "reused SYN tuple")
			}
			flow.syn = true
			flow.initial = tcp.Seq + 1
		}
		if len(tcp.Payload) > 0 {
			flow.segments = append(flow.segments, tlsSHTestSegment{i + 1, tcp.Seq, append([]byte(nil), tcp.Payload...)})
		}
	}
	var observed []tlsSHTestObserved
	for _, flow := range flows {
		if !flow.syn {
			continue
		}
		sort.SliceStable(flow.segments, func(i, j int) bool {
			return uint32(flow.segments[i].seq-flow.initial) < uint32(flow.segments[j].seq-flow.initial)
		})
		var stream []byte
		var provenance []int
		for _, s := range flow.segments {
			offset := int(uint32(s.seq - flow.initial))
			if offset > len(stream) {
				break
			} // No inference over unavailable bytes.
			for j, b := range s.payload {
				at := offset + j
				if at < len(stream) {
					require.Equal(t, stream[at], b, "frame %d conflicting retransmission", s.frame)
					continue
				}
				stream = append(stream, b)
				provenance = append(provenance, s.frame)
			}
		}
		var pending []byte
		var origins []int
		for at := 0; len(stream)-at >= 5; {
			typ := stream[at]
			if typ == 20 || typ == 23 {
				break
			}
			if typ != 22 || stream[at+1] != 3 || stream[at+2] < 1 || stream[at+2] > 3 {
				break
			}
			n := int(binary.BigEndian.Uint16(stream[at+3 : at+5]))
			if n == 0 || n > 16384 || n > len(stream)-at-5 {
				break
			}
			end := at + 5 + n
			pending = append(pending, stream[at+5:end]...)
			for p := at + 5; p < end; p++ {
				origins = append(origins, p)
			}
			for len(pending) >= 4 {
				hlen := int(pending[1])<<16 | int(pending[2])<<8 | int(pending[3])
				if hlen > len(pending)-4 {
					break
				}
				if pending[0] == 2 {
					// Every observed hello in these captures is also a complete
					// first handshake in one record; assert, rather than assume it.
					require.Equal(t, at+5, origins[0])
					require.Equal(t, hlen+4, n)
					frame := provenance[at]
					for p := at; p < end; p++ {
						require.Equal(t, frame, provenance[p], "multi-frame record must use explicit reassembled entry")
					}
					observed = append(observed, tlsSHTestObserved{frame: frame, offset: at, record: append([]byte(nil), stream[at:end]...), handshake: append([]byte(nil), pending[:hlen+4]...)})
				}
				pending = pending[hlen+4:]
				origins = origins[hlen+4:]
			}
			at = end
		}
	}
	sort.Slice(observed, func(i, j int) bool { return observed[i].frame < observed[j].frame })
	return observed
}

func tlsSHTestValues(t *testing.T, n *base.Node) {
	t.Helper()
	if stream_parser.NodeHasResult(n) {
		typ := n.Cfg.GetString(base.CfgType)
		if strings.HasPrefix(typ, "uint") {
			var want uint64
			for i, b := range stream_parser.GetBytesByNode(n) {
				if n.Cfg.GetString(base.CfgEndian) == "little" {
					want |= uint64(b) << (8 * i)
				} else {
					want = want<<8 | uint64(b)
				}
			}
			result, err := n.Result()
			require.NoError(t, err)
			require.Equal(t, want, uintVal(t, result), n.Name)
		}
	}
	for _, child := range n.Children {
		tlsSHTestValues(t, child)
	}
}

func tlsSHTestWhole(t *testing.T, frame []byte, start, size int) *base.Node {
	t.Helper()
	tail := len(frame) - start - size
	source := fmt.Sprintf("unit: byte\nPackage:\n  Capture:\n    operator: |\n      this.ProcessSubNode(\"Envelope\")\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      if %d > 0 { this.ProcessSubNode(\"Capture Tail\") }\n    Envelope: raw,%d\n    Message: \"import:application-layer/tls_server_hello.yaml;node:TLSServerHelloRecord\"\n    Capture Tail: raw,%d\n", size, tail, start, tail)
	var doc yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
	root, err := base.NewNodeTree(doc)
	require.NoError(t, err)
	root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
	reader := base.NewBitReader(bytes.NewReader(frame))
	require.NoError(t, root.ParseSubNode(reader, "Capture"))
	n := base.GetNodeByPath(root, "@Capture")
	require.Equal(t, frame, NodeToBytes(n))
	h225TestTree(t, n, frame, 0)
	require.ErrorContains(t, reader.Recovery(), "no backup")
	return protocolCorpusFindNode(n, "Message")
}

func TestProtocolCorpusTLSServerHelloAllOriginalRecords(t *testing.T) {
	specs := []struct {
		name, sha string
		records   int
		rawIP     bool
		frames    []int
	}{
		{"anydesk.pcapng", "21082a0bf1af60d6acbd2f736e4686938050065d2b48f37053d9a64791b1c09a", 174, false, []int{14, 72, 85, 126}},
		{"dingtalk.pcap", "238ee8af258426a8f51bbf7a6222b0b9437d2822fac91ab0391f670c89960d5f", 16, true, []int{11}},
		{"doh.pcap", "b1459348b4a72c24e5646fdc05131ded69afb2fc543dc5568d004807939554c4", 142, false, []int{6}},
		{"dot.pcap", "8f6125abecbf0e28ab824246581ba5fa3b42dcdeb8626d191f25e63699b5fb78", 24, false, []int{6}},
		{"imaps.pcap", "b53f4d242f620db57ad82f298552aec59b60dc8fc9f77d16a5d2d057ecfe7c59", 28, false, []int{6, 27}},
		{"wechat.pcap", "2d82f575a8b9addfc9e66582e34911f79b5388984e54299293a28393c6d7c24e", 1672, false, []int{18, 98, 124, 151, 196, 238, 265, 286, 366, 389, 500, 521, 544, 558, 567, 648, 852, 896, 968, 1057, 1077, 1138, 1166, 1228, 1268, 1434, 1484, 1521}},
		{"smtps.pcapng", "8d68f3726c5ea2b8527cf52a28b8e132b2c332633562f24280f82de05832f169", 4, false, nil},
		{"netease-games.pcapng", "f662aff63082da1da601841bdb3d88f4ca557be2bdb8528ee746f910bd28d38a", 20, false, []int{10}},
	}
	total := 0
	for _, spec := range specs {
		t.Run(spec.name, func(t *testing.T) {
			path := "testdata/protocol-corpus/captures/ndpi/ndpi-" + spec.name
			file, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, spec.sha, fmt.Sprintf("%x", sha256.Sum256(file)))
			records := protocolCorpusAuditPackets(t, path)
			require.Len(t, records, spec.records)
			observed := tlsSHTestObserve(t, records, spec.rawIP)
			var frames []int
			for _, hello := range observed {
				frames = append(frames, hello.frame)
			}
			require.Equal(t, spec.frames, frames)
			total += len(observed)
			for _, hello := range observed {
				t.Run(fmt.Sprintf("frame-%d", hello.frame), func(t *testing.T) {
					frame := records[hello.frame-1]
					first := layers.LayerTypeEthernet
					if spec.rawIP {
						first = layers.LayerTypeIPv4
					}
					packet := gopacket.NewPacket(frame, first, gopacket.Default)
					tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
					require.Zero(t, hello.offset, "each hello is the initial server-direction record in this fixed corpus")
					require.Equal(t, hello.record, tcp.Payload[:len(hello.record)])
					start := len(frame) - len(tcp.Payload)
					require.Equal(t, tcp.Payload, frame[start:])
					n := tlsSHTestWhole(t, frame, start, len(hello.record))
					tlsSHTestValues(t, n)
					protocolCorpusRequireValue(t, n, "Content Type", uint64(22))
					protocolCorpusRequireValue(t, n, "Record Version", uint64(binary.BigEndian.Uint16(hello.record[1:3])))
					protocolCorpusRequireValue(t, n, "Record Length", uint64(len(hello.handshake)))
					protocolCorpusRequireValue(t, n, "Handshake Type", uint64(2))
					protocolCorpusRequireValue(t, n, "Handshake Length", uint64(len(hello.handshake)-4))
					protocolCorpusRequireValue(t, n, "Hello Version", uint64(0x0303))
					protocolCorpusRequireValue(t, n, "Random", hello.handshake[6:38])
					sid := int(hello.handshake[38])
					require.LessOrEqual(t, sid, 32)
					protocolCorpusRequireValue(t, n, "Session ID Length", uint64(sid))
					protocolCorpusRequireValue(t, n, "Session ID", hello.handshake[39:39+sid])
					at := 39 + sid
					protocolCorpusRequireValue(t, n, "Cipher Suite", uint64(binary.BigEndian.Uint16(hello.handshake[at:at+2])))
					protocolCorpusRequireValue(t, n, "Compression Method", uint64(0))
					at += 3
					extSize := int(binary.BigEndian.Uint16(hello.handshake[at : at+2]))
					at += 2
					require.Equal(t, len(hello.handshake)-at, extSize)
					list := protocolCorpusFindNode(n, "Extensions")
					require.NotNil(t, list)
					require.True(t, list.Cfg.GetBool(stream_parser.CfgIsList))
					var types []uint16
					for index := 0; at < len(hello.handshake); index++ {
						typ := binary.BigEndian.Uint16(hello.handshake[at : at+2])
						size := int(binary.BigEndian.Uint16(hello.handshake[at+2 : at+4]))
						types = append(types, typ)
						require.LessOrEqual(t, at+4+size, len(hello.handshake))
						protocolCorpusRequireValue(t, list.Children[index], "Extension Type", uint64(typ))
						protocolCorpusRequireValue(t, list.Children[index], "Extension Length", uint64(size))
						require.Equal(t, index, list.Children[index].Cfg.GetItem(stream_parser.CfgElementIndex))
						at += 4 + size
					}
					require.Len(t, list.Children, len(types))
					info := n.Cfg.GetItem("additionInfo").(map[string]any)
					require.Equal(t, true, info["ParsedFirstHandshakeOnly"])
					require.Equal(t, 0, info["Following Handshake Byte Count"])
					for _, key := range []string{"Negotiation Validated", "Handshake Completion Validated", "Peer Identity Validated", "Key Exchange Validated", "Certificate Trust Validated", "TCP Reassembly Performed"} {
						require.Equal(t, false, info[key], key)
					}
					if spec.name == "doh.pcap" || spec.name == "dingtalk.pcap" {
						require.ElementsMatch(t, []uint16{43, 51}, types)
						protocolCorpusRequireValue(t, n, "Supported Version", uint64(0x0304))
						protocolCorpusRequireValue(t, n, "Key Share Group", uint64(29))
						protocolCorpusRequireValue(t, n, "Key Exchange Length", uint64(32))
					}
					if spec.name == "wechat.pcap" && hello.frame == 18 {
						protocolCorpusRequireValue(t, n, "ALPN Protocol", []byte("h2"))
						scts := protocolCorpusFindNode(n, "Signed Certificate Timestamps")
						require.Len(t, scts.Children, 2)
						for i, length := range []uint64{119, 117} {
							protocolCorpusRequireValue(t, scts.Children[i], "SCT Length", length)
							protocolCorpusRequireValue(t, scts.Children[i], "SCT Version", uint64(0))
							protocolCorpusRequireValue(t, scts.Children[i], "SCT Hash Algorithm", uint64(4))
							protocolCorpusRequireValue(t, scts.Children[i], "SCT Signature Algorithm", uint64(3))
						}
					}
					if spec.name == "wechat.pcap" && hello.frame == 968 {
						protocolCorpusRequireValue(t, n, "ALPN Protocol", []byte("http/1.1"))
					}
					h := protocolCorpusRequireBoundedRuleParse(t, hello.handshake, "application-layer.tls_server_hello", "TLSHandshakeServerHello")
					h225TestTree(t, h, hello.handshake, 0)
					tlsSHTestValues(t, h)
					require.Equal(t, false, h.Cfg.GetItem("additionInfo").(map[string]any)["Record Wrapped"])
				})
			}
		})
	}
	require.Equal(t, 38, total)
}

const tlsSHTestLiteral = "160303003a02000036030384e2e29a8324785a9cd99535f7b95f359f325f044ccb10549e721e1216dec4c700009f00000eff0100010000230000000f000101"

type tlsSHTestHeldReader struct {
	bits  uint64
	reads int
}

func (r *tlsSHTestHeldReader) InputBitLength() uint64     { return r.bits }
func (r *tlsSHTestHeldReader) Read(p []byte) (int, error) { r.reads++; return 0, io.EOF }

func TestProtocolCorpusTLSServerHelloReadBeforeLimitGuard(t *testing.T) {
	for entry, max := range map[string]uint64{"TLSHandshakeServerHello": 65611, "TLSHandshakeServerHelloCarrier": 65611, "TLSServerHelloRecord": 16389, "TLSServerHelloRecordCarrier": 16389} {
		for _, bits := range []uint64{0, 1, 7, max*8 + 1, (max + 1) * 8} {
			reader := &tlsSHTestHeldReader{bits: bits}
			_, err := parser.ParseBinary(reader, "application-layer.tls_server_hello", entry)
			require.Error(t, err, "%s / %d", entry, bits)
			require.Zero(t, reader.reads, "%s / %d", entry, bits)
		}
	}
}

func TestProtocolCorpusTLSServerHelloBoundariesAndCompatibility(t *testing.T) {
	good := h225TestHex(t, tlsSHTestLiteral)
	for _, record := range []bool{true, false} {
		entry := "TLSHandshakeServerHello"
		wire := good[5:]
		if record {
			entry = "TLSServerHelloRecord"
			wire = good
		}
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.tls_server_hello", entry)
			require.Error(t, err, "%s cut %d", entry, cut)
		}
		for _, bad := range [][]byte{append(append([]byte(nil), wire...), 0), {0xde, 0xad}, wire[:len(wire)-1]} {
			n := protocolCorpusRequireBoundedRuleParse(t, bad, "application-layer.tls_server_hello", entry+"Carrier")
			h225TestTree(t, n, bad, 0)
			require.Nil(t, protocolCorpusFindNode(n, "ServerHello"))
			name := "Unparsed TLS ServerHello"
			if record {
				name += " Record"
			}
			protocolCorpusRequireValue(t, n, name, bad)
		}
		for _, name := range []string{entry, entry + "Carrier"} {
			_, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.tls_server_hello", name)
			require.ErrorContains(t, err, "explicit")
			_, err = parser.GenerateBinary(map[string]any{}, "application-layer.tls_server_hello", name)
			require.Error(t, err)
		}
	}
	// Preserve an additional complete handshake and an incomplete following header
	// without suggesting either was decoded; strict handshake entry rejects tails.
	tail := []byte{14, 0, 0, 0, 11, 0}
	withTail := append(append([]byte(nil), good...), tail...)
	binary.BigEndian.PutUint16(withTail[3:5], uint16(len(withTail)-5))
	n := protocolCorpusRequireBoundedRuleParse(t, withTail, "application-layer.tls_server_hello", "TLSServerHelloRecord")
	h225TestTree(t, n, withTail, 0)
	protocolCorpusRequireValue(t, n, "Following Handshake Bytes", tail)
	require.Equal(t, len(tail), n.Cfg.GetItem("additionInfo").(map[string]any)["Following Handshake Byte Count"])
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(withTail[5:]), "application-layer.tls_server_hello", "TLSHandshakeServerHello")
	require.Error(t, err)
	// Existing record API still retains its old names and raw non-ClientHello body.
	old, err := parser.ParseBinary(newProtocolCorpusBoundedReader(good), "application-layer.tls")
	require.NoError(t, err)
	protocolCorpusRequireValue(t, old, "ContentType", uint64(22))
	require.Equal(t, good, NodeToBytes(old))
}

func TestProtocolCorpusTLSServerHelloOffsetsRollbackAndIsolation(t *testing.T) {
	for offset := uint64(0); offset < 8; offset++ {
		for _, good := range []bool{true, false} {
			wire := h225TestHex(t, tlsSHTestLiteral)
			if !good {
				wire[8]--
			}
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
			source := fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/tls_server_hello.yaml;node:TLSServerHelloRecordCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, 8-offset)
			var doc yaml.MapSlice
			require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
			root, err := base.NewNodeTree(doc)
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
			root.Ctx.SetItem("body_length", 123)
			reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
			require.NoError(t, root.ParseSubNode(reader, "Wrapped"))
			n := base.GetNodeByPath(root, "@Wrapped")
			message := protocolCorpusFindNode(n, "Message")
			h225TestTree(t, message, wire, offset)
			tlsSHTestValues(t, message)
			if good {
				protocolCorpusRequireValue(t, message, "Cipher Suite", uint64(0x009f))
			} else {
				protocolCorpusRequireValue(t, message, "Unparsed TLS ServerHello Record", wire)
				require.Nil(t, protocolCorpusFindNode(message, "ServerHello"))
			}
			protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
			require.Equal(t, packed.Bytes(), NodeToBytes(n))
			require.Equal(t, 123, root.Ctx.GetItem("body_length"))
			require.ErrorContains(t, reader.Recovery(), "no backup")
			_, err = reader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		}
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("isolated-%d", worker), func(t *testing.T) {
			t.Parallel()
			wire := h225TestHex(t, tlsSHTestLiteral)
			wire[11] = byte(worker)
			cfg := map[string]any{"marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "application-layer.tls_server_hello", "TLSServerHelloRecord", cfg)
			protocolCorpusRequireValue(t, n, "Random", wire[11:43])
			require.Equal(t, map[string]any{"marker": worker}, cfg)
		})
	}
}
