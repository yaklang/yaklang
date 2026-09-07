package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sort"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const tlsControlTestRule = "application-layer.tls_control_handshake"

func tlsControlTestEntry(typ byte) string {
	if typ == 14 {
		return "TLS12ServerHelloDone"
	}
	return "TLS12NewSessionTicket"
}

func tlsControlTestFields(t *testing.T, n *base.Node, wire []byte, offset uint64) {
	t.Helper()
	latTestField(t, n, "Handshake Type", "uint8", offset, offset+8, uint64(wire[0]))
	latTestField(t, n, "Handshake Length", "uint32", offset+8, offset+32, uint64(len(wire)-4))
	if wire[0] == 4 {
		latTestField(t, n, "Ticket Lifetime Hint", "uint32", offset+32, offset+64, uint64(binary.BigEndian.Uint32(wire[4:8])))
		latTestField(t, n, "Ticket Length", "uint16", offset+64, offset+80, uint64(len(wire)-10))
		field := protocolCorpusFindNode(n, "Ticket")
		require.NotNil(t, field)
		require.Equal(t, "raw", field.Cfg.GetString(base.CfgType))
		require.Equal(t, [2]uint64{offset + 80, offset + uint64(len(wire))*8}, stream_parser.GetNodeResultPos(field))
		value, err := field.Result()
		require.NoError(t, err)
		// Original ticket bytes are compared without including their contents in
		// assertion diagnostics. They are deliberately not decoded or published.
		require.True(t, bytes.Equal(wire[10:], bytesVal(t, value)), "opaque ticket Result mismatch")
		require.True(t, bytes.Equal(wire[10:], stream_parser.GetBytesByNode(field)), "opaque ticket span mismatch")
	}
	// Imported definitions legitimately own another context; require identity
	// throughout the native subtree, not across the import boundary itself.
	native := protocolCorpusFindNode(n, "Handshake Type").Cfg.GetItem(base.CfgParent).(*base.Node)
	latTestTree(t, native, offset, offset+uint64(len(wire))*8)
}

func TestProtocolCorpusTLS12ControlIndependentFields(t *testing.T) {
	for _, literal := range []string{"0e000000", "0400000a010203040004deadbeef", "04000006000000000000", "04000006ffffffff0000"} {
		wire := h225TestHex(t, literal)
		for _, entry := range []string{tlsControlTestEntry(wire[0]), tlsControlTestEntry(wire[0]) + "Carrier"} {
			n := protocolCorpusRequireBoundedRuleParse(t, wire, tlsControlTestRule, entry)
			tlsControlTestFields(t, n, wire, 0)
			require.Equal(t, wire, NodeToBytes(n))
			if entry[len(entry)-7:] == "Carrier" {
				n = protocolCorpusFindNode(n, tlsControlTestEntry(wire[0]))
			}
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			for _, name := range []string{"Protocol Version Inferred", "Direction Validated", "Handshake Phase Validated", "Handshake Completion Validated", "TCP Reassembly Performed", "Structured Generation Supported"} {
				require.Equal(t, false, info[name], name)
			}
			if wire[0] == 4 {
				require.Equal(t, binary.BigEndian.Uint32(wire[4:8]) == 0, info["Lifetime Hint Unspecified"])
				for _, name := range []string{"Lifetime Hint Is Verified Expiry", "Ticket Contents Decoded", "Ticket Validated", "Session Resumption Validated"} {
					require.Equal(t, false, info[name], name)
				}
			}
		}
	}
}

func TestProtocolCorpusTLS12ControlStrictBoundaries(t *testing.T) {
	for _, literal := range []string{"0e000000", "0400000a010203040004deadbeef"} {
		wire := h225TestHex(t, literal)
		entry := tlsControlTestEntry(wire[0])
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), tlsControlTestRule, entry)
			require.Error(t, err, "%s prefix %d", entry, cut)
		}
		for _, bad := range [][]byte{append(bytes.Clone(wire), 0), append(bytes.Clone(wire), wire...), {0xde}, {14, 0, 0, 1, 0}, {4, 0, 0, 6, 0, 0, 0, 0, 0, 1}, {4, 0, 0, 7, 0, 0, 0, 0, 0, 0, 0}} {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), tlsControlTestRule, entry)
			require.Error(t, err)
			n := protocolCorpusRequireBoundedRuleParse(t, bad, tlsControlTestRule, entry+"Carrier")
			latTestField(t, n, "Unparsed TLS Control Handshake", "raw", 0, uint64(len(bad))*8, bad)
			require.Nil(t, protocolCorpusFindNode(n, "Handshake Type"))
			require.Equal(t, bad, NodeToBytes(n))
		}
		other := h225TestHex(t, "0e000000")
		if wire[0] == 14 {
			other = h225TestHex(t, "04000006000000000000")
		}
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(other), tlsControlTestRule, entry)
		require.ErrorContains(t, err, "unexpected handshake type")
		for _, name := range []string{entry, entry + "Carrier"} {
			_, err := parser.ParseBinary(bytes.NewReader(wire), tlsControlTestRule, name)
			require.ErrorContains(t, err, "explicit")
			_, err = parser.GenerateBinary(map[string]any{}, tlsControlTestRule, name)
			require.Error(t, err)
		}
	}
	// The 16-bit ticket vector has an inclusive maximum and permits empty data.
	wire := make([]byte, 65545)
	copy(wire, []byte{4, 1, 0, 5, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	n := protocolCorpusRequireBoundedRuleParse(t, wire, tlsControlTestRule, "TLS12NewSessionTicket")
	tlsControlTestFields(t, n, wire, 0)
	for _, entry := range []string{"TLS12ServerHelloDone", "TLS12NewSessionTicket", "TLS12ServerHelloDoneCarrier", "TLS12NewSessionTicketCarrier"} {
		for _, bits := range []uint64{0, 1, 2, 3, 4, 5, 6, 7, 65546 * 8, 65545*8 + 1} {
			r := &tlsSHTestHeldReader{bits: bits}
			_, err := parser.ParseBinary(r, tlsControlTestRule, entry)
			require.Error(t, err)
			require.Zero(t, r.reads, "%s bits=%d", entry, bits)
			root := latTestInline(t, fmt.Sprintf("Package:\n  Message: \"import:application-layer/tls_control_handshake.yaml;node:%s\"\n", entry))
			root.Cfg.SetItem(base.CfgLength, bits)
			reader := bytes.NewReader([]byte{0xff})
			require.Error(t, root.ParseSubNode(base.NewBitReader(reader), "Message"))
			require.Equal(t, 1, reader.Len())
		}
	}
}

func TestProtocolCorpusTLS12ControlImportedHeldReader(t *testing.T) {
	for _, literal := range []string{"0e000000", "0400000a010203040004deadbeef"} {
		for offset := uint64(0); offset < 8; offset++ {
			for _, valid := range []bool{true, false} {
				wire := h225TestHex(t, literal)
				entry := tlsControlTestEntry(wire[0])
				if !valid {
					wire[3]++
					entry += "Carrier"
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
				root := latTestInline(t, fmt.Sprintf(`endian: little
Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Message").SetMaxLength(%d)
      this.ProcessSubNode("Message")
      this.ProcessSubNode("Sentinel")
      if %d > 0 { this.ProcessSubNode("Padding") }
    Prefix: uint8,%dbit
    Message: "import:application-layer/tls_control_handshake.yaml;node:%s"
    Sentinel: uint8
    Padding: uint8,%dbit
`, offset, len(wire), offset, offset, entry, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("caller-control", "preserved")
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Envelope"))
				n := base.GetNodeByPath(root, "@Envelope")
				message := protocolCorpusFindNode(n, "Message")
				if valid {
					tlsControlTestFields(t, message, wire, offset)
				} else {
					latTestField(t, n, "Unparsed TLS Control Handshake", "raw", offset, offset+uint64(len(wire))*8, wire)
					require.Nil(t, protocolCorpusFindNode(n, "Handshake Type"))
				}
				protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
				require.Equal(t, "preserved", root.Ctx.GetItem("caller-control"))
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

func TestProtocolCorpusTLS12ControlConcurrentIsolation(t *testing.T) {
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			wire := h225TestHex(t, "04000007010203040001ff")
			wire[7] = byte(worker)
			cfg := map[string]any{"caller-control": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, tlsControlTestRule, "TLS12NewSessionTicket", cfg)
			tlsControlTestFields(t, n, wire, 0)
			n.Cfg.GetItem("additionInfo").(map[string]any)["Ticket Validated"] = true
			again := protocolCorpusRequireBoundedRuleParse(t, wire, tlsControlTestRule, "TLS12NewSessionTicket")
			require.Equal(t, false, again.Cfg.GetItem("additionInfo").(map[string]any)["Ticket Validated"])
			require.Equal(t, map[string]any{"caller-control": worker}, cfg)
		})
	}
}

type tlsControlTestOrigin struct{ frame, offset int }
type tlsControlTestSegment struct {
	seq    uint32
	origin tlsControlTestOrigin
	data   []byte
}
type tlsControlTestFlow struct {
	syn, server bool
	initial     uint32
	segments    []tlsControlTestSegment
}
type tlsControlTestObserved struct {
	wire    []byte
	origins []tlsControlTestOrigin
}

// Independent version evidence, not just the record's legacy version. TLS 1.3
// ordinary ServerHello has 0303 too, but selects its version in extension 43.
func tlsControlTestVersion(t *testing.T, h []byte) uint16 {
	t.Helper()
	require.GreaterOrEqual(t, len(h), 42)
	version := binary.BigEndian.Uint16(h[4:6])
	at := 42 + int(h[38])
	require.LessOrEqual(t, at, len(h))
	if at == len(h) {
		return version
	}
	require.LessOrEqual(t, at+2, len(h))
	length := int(binary.BigEndian.Uint16(h[at : at+2]))
	at += 2
	require.Equal(t, len(h)-at, length)
	for at < len(h) {
		require.LessOrEqual(t, at+4, len(h))
		typ, n := binary.BigEndian.Uint16(h[at:at+2]), int(binary.BigEndian.Uint16(h[at+2:at+4]))
		at += 4
		require.LessOrEqual(t, at+n, len(h))
		if typ == 43 {
			require.Equal(t, 2, n)
			version = binary.BigEndian.Uint16(h[at : at+2])
		}
		at += n
	}
	return version
}

// All TCP ports, including AnyDesk's TCP 80: no dissector labels, magic search,
// missing-prefix inference or assumptions about one TCP segment/record/message.
// Reconstruct from observed SYN+1, check overlaps, stop at gaps. Every selected
// byte retains its original physical frame/offset; CCS ends plaintext evidence.
func tlsControlTestFlows(t *testing.T, records [][]byte, rawIP bool) map[string]*tlsControlTestFlow {
	t.Helper()
	flows := map[string]*tlsControlTestFlow{}
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
			flow = &tlsControlTestFlow{}
			flows[key] = flow
		}
		if tcp.SYN {
			if flow.syn {
				require.Equal(t, flow.initial, tcp.Seq+1, "reused SYN tuple")
			}
			flow.syn, flow.server, flow.initial = true, tcp.ACK, tcp.Seq+1
		}
		if len(tcp.Payload) != 0 {
			// Locate by decoded headers rather than assuming no Ethernet padding.
			offset := 0
			for _, layer := range packet.Layers() {
				offset += len(layer.LayerContents())
				if layer.LayerType() == layers.LayerTypeTCP {
					break
				}
			}
			require.LessOrEqual(t, offset+len(tcp.Payload), len(record))
			require.True(t, bytes.Equal(tcp.Payload, record[offset:offset+len(tcp.Payload)]))
			seq := tcp.Seq
			if tcp.SYN {
				seq++
			}
			flow.segments = append(flow.segments, tlsControlTestSegment{seq, tlsControlTestOrigin{i + 1, offset}, bytes.Clone(tcp.Payload)})
		}
	}
	return flows
}

func tlsControlTestObserve(t *testing.T, records [][]byte, rawIP bool) []tlsControlTestObserved {
	t.Helper()
	flows := tlsControlTestFlows(t, records, rawIP)
	var observed []tlsControlTestObserved
	for _, flow := range flows {
		if !flow.syn {
			continue
		}
		sort.SliceStable(flow.segments, func(i, j int) bool {
			return uint32(flow.segments[i].seq-flow.initial) < uint32(flow.segments[j].seq-flow.initial)
		})
		var stream []byte
		var provenance []tlsControlTestOrigin
		for _, s := range flow.segments {
			offset := uint32(s.seq - flow.initial)
			if uint64(offset) > uint64(len(stream)) {
				break
			}
			for j, b := range s.data {
				at := int(offset) + j
				if at < len(stream) {
					require.Equal(t, stream[at], b, "frame %d conflicting overlap", s.origin.frame)
					continue
				}
				stream = append(stream, b)
				provenance = append(provenance, tlsControlTestOrigin{s.origin.frame, s.origin.offset + j})
			}
		}
		var pending []byte
		var origins []tlsControlTestOrigin
		var version uint16
		for at := 0; len(stream)-at >= 5; {
			if stream[at] == 20 || stream[at] == 23 {
				break
			}
			if stream[at] != 22 || stream[at+1] != 3 || stream[at+2] < 1 || stream[at+2] > 3 {
				break
			}
			n := int(binary.BigEndian.Uint16(stream[at+3 : at+5]))
			if n == 0 || n > 16384 || n > len(stream)-at-5 {
				break
			}
			end := at + 5 + n
			pending = append(pending, stream[at+5:end]...)
			origins = append(origins, provenance[at+5:end]...)
			for len(pending) >= 4 {
				size := 4 + (int(pending[1]) << 16) + (int(pending[2]) << 8) + int(pending[3])
				if size > len(pending) {
					break
				}
				if pending[0] == 2 {
					version = tlsControlTestVersion(t, pending[:size])
				}
				if pending[0] == 4 || pending[0] == 14 {
					require.True(t, flow.server, "control message must have observed server direction")
					require.Equal(t, uint16(0x0303), version, "explicit TLS 1.2 evidence required")
					observed = append(observed, tlsControlTestObserved{bytes.Clone(pending[:size]), append([]tlsControlTestOrigin(nil), origins[:size]...)})
				}
				pending, origins = pending[size:], origins[size:]
			}
			at = end
		}
	}
	sort.Slice(observed, func(i, j int) bool {
		a, b := observed[i].origins[0], observed[j].origins[0]
		if a.frame == b.frame {
			return a.offset < b.offset
		}
		return a.frame < b.frame
	})
	return observed
}

func TestProtocolCorpusTLS12ControlAllOriginalRecords(t *testing.T) {
	// Independent frame/offset expectations, not derived from the native parser.
	// All original tickets remain in the unchanged capture; only their digest,
	// length and public lifetime hint are recorded here, never their contents.
	type ticketExpectation struct {
		frame, offset, length int
		lifetime              uint32
		sha                   string
	}
	donePositions := map[string][][2]int{
		"anydesk.pcapng": {{18, 55}, {73, 193}, {85, 863}, {128, 1142}},
		"dot.pcap":       {{6, 3131}},
		"imaps.pcap":     {{8, 221}},
		"wechat.pcap":    {{22, 1438}, {102, 322}, {128, 322}, {153, 1750}, {200, 322}, {240, 1750}, {269, 322}, {290, 322}, {368, 1750}, {391, 1750}, {502, 1750}, {523, 1750}, {544, 3178}, {560, 1750}, {571, 322}, {652, 322}, {856, 322}, {900, 322}, {970, 3480}, {1061, 322}, {1081, 322}, {1142, 322}, {1168, 1750}, {1232, 322}, {1270, 1750}, {1438, 322}, {1486, 1750}, {1523, 1750}},
	}
	ticketPositions := map[string][]ticketExpectation{
		"anydesk.pcapng": {
			{81, 59, 858, 7200, "f016f7327c4d4f528e93a84d3c7f4422a611c96153471ce0035835aaa42acbba"},
			{94, 59, 858, 7200, "c0c3a05107bb1fe747f104bb31ab04183c66903667c5f98bb2dd2c70c18c7f3a"},
		},
		"dot.pcap": {{11, 71, 228, 100800, "a22afc636ef6df6c3d8f89143eb231efcaa7e2c5bb613dba3d2745a9f617a127"}},
		"wechat.pcap": {
			{27, 71, 222, 100799, "d1cba2a7b17c7ee7abca0c0afe9bd31aa6e16e821844a6cf19460f5afaaca982"},
			{992, 59, 202, 6000, "9c8193b08cf9da7a5a68a506cf2aec158ba3deffeb78c11c4b1d70e50d6dea95"},
		},
	}
	specs := []struct {
		name, sha string
		records   int
		rawIP     bool
	}{
		{"anydesk.pcapng", "21082a0bf1af60d6acbd2f736e4686938050065d2b48f37053d9a64791b1c09a", 174, false},
		{"dingtalk.pcap", "238ee8af258426a8f51bbf7a6222b0b9437d2822fac91ab0391f670c89960d5f", 16, true},
		{"doh.pcap", "b1459348b4a72c24e5646fdc05131ded69afb2fc543dc5568d004807939554c4", 142, false},
		{"dot.pcap", "8f6125abecbf0e28ab824246581ba5fa3b42dcdeb8626d191f25e63699b5fb78", 24, false},
		{"imaps.pcap", "b53f4d242f620db57ad82f298552aec59b60dc8fc9f77d16a5d2d057ecfe7c59", 28, false},
		{"wechat.pcap", "2d82f575a8b9addfc9e66582e34911f79b5388984e54299293a28393c6d7c24e", 1672, false},
		{"smtps.pcapng", "8d68f3726c5ea2b8527cf52a28b8e132b2c332633562f24280f82de05832f169", 4, false},
		{"netease-games.pcapng", "f662aff63082da1da601841bdb3d88f4ca557be2bdb8528ee746f910bd28d38a", 20, false},
	}
	totalRecords, totalDone, totalTickets := 0, 0, 0
	for _, spec := range specs {
		t.Run(spec.name, func(t *testing.T) {
			path := "testdata/protocol-corpus/captures/ndpi/ndpi-" + spec.name
			capture, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, spec.sha, fmt.Sprintf("%x", sha256.Sum256(capture)))
			records := protocolCorpusAuditPackets(t, path)
			require.Len(t, records, spec.records)
			totalRecords += len(records)
			observed := tlsControlTestObserve(t, records, spec.rawIP)
			require.Len(t, observed, len(donePositions[spec.name])+len(ticketPositions[spec.name]))
			doneIndex, ticketIndex := 0, 0
			for _, item := range observed {
				n := protocolCorpusRequireBoundedRuleParse(t, item.wire, tlsControlTestRule, tlsControlTestEntry(item.wire[0]))
				tlsControlTestFields(t, n, item.wire, 0)
				require.True(t, bytes.Equal(item.wire, NodeToBytes(n)), "handshake bytes preserved")
				for i, origin := range item.origins {
					require.Equal(t, item.wire[i], records[origin.frame-1][origin.offset], "physical provenance")
				}
				first := item.origins[0]
				if item.wire[0] == 14 {
					require.Less(t, doneIndex, len(donePositions[spec.name]))
					require.Equal(t, donePositions[spec.name][doneIndex], [2]int{first.frame, first.offset})
					require.Equal(t, []byte{14, 0, 0, 0}, item.wire)
					doneIndex++
					totalDone++
				} else {
					require.Less(t, ticketIndex, len(ticketPositions[spec.name]))
					want := ticketPositions[spec.name][ticketIndex]
					require.Equal(t, [3]int{want.frame, want.offset, want.length}, [3]int{first.frame, first.offset, len(item.wire)})
					require.Equal(t, want.lifetime, binary.BigEndian.Uint32(item.wire[4:8]))
					require.Equal(t, want.sha, fmt.Sprintf("%x", sha256.Sum256(item.wire)))
					ticketIndex++
					totalTickets++
				}
				// Only a physically contiguous message is imported with physical
				// spans. Reassembled bytes above use their own relative spans.
				contiguous := true
				for i, origin := range item.origins {
					if origin.frame != first.frame || origin.offset != first.offset+i {
						contiguous = false
						break
					}
				}
				require.True(t, contiguous, "these 39 original controls have contiguous physical provenance")
				if contiguous {
					frame := records[first.frame-1]
					tail := len(frame) - first.offset - len(item.wire)
					root := latTestInline(t, fmt.Sprintf("unit: byte\nPackage:\n  Capture:\n    operator: |\n      this.ProcessSubNode(\"Envelope\")\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      if %d > 0 { this.ProcessSubNode(\"Tail\") }\n    Envelope: raw,%d\n    Message: \"import:application-layer/tls_control_handshake.yaml;node:%s\"\n    Tail: raw,%d\n", len(item.wire), tail, first.offset, tlsControlTestEntry(item.wire[0]), tail))
					root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
					require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(frame)), "Capture"))
					whole := base.GetNodeByPath(root, "@Capture")
					require.True(t, bytes.Equal(frame, NodeToBytes(whole)))
					tlsControlTestFields(t, protocolCorpusFindNode(whole, "Message"), item.wire, uint64(first.offset)*8)
				}
			}
			require.Equal(t, len(donePositions[spec.name]), doneIndex)
			require.Equal(t, len(ticketPositions[spec.name]), ticketIndex)
		})
	}
	require.Equal(t, 2080, totalRecords)
	require.Equal(t, 34, totalDone)
	require.Equal(t, 5, totalTickets)
}

// Keep the test-only reassembly oracle honest: records and handshake messages
// can each straddle segments, record boundaries need not be handshake
// boundaries, input arrival order differs from sequence order, and ciphertext
// after CCS must never be interpreted as another plaintext ticket.
func TestProtocolCorpusTLS12ControlProvenanceOracle(t *testing.T) {
	record := func(typ byte, payload []byte) []byte {
		header := []byte{typ, 3, 3, 0, 0}
		binary.BigEndian.PutUint16(header[3:], uint16(len(payload)))
		return append(header, payload...)
	}
	hello := make([]byte, 42)
	copy(hello, []byte{2, 0, 0, 38, 3, 3})
	hello[40] = 0x2f // TLS_RSA_WITH_AES_128_CBC_SHA, null compression.
	done := []byte{14, 0, 0, 0}
	ticket := h225TestHex(t, "04000007000000010001ff")
	stream := record(22, hello)
	stream = append(stream, record(22, done[:2])...)
	stream = append(stream, record(22, append(bytes.Clone(done[2:]), ticket...))...)
	stream = append(stream, record(20, []byte{1})...)
	stream = append(stream, record(22, ticket)...)
	packet := func(seq uint32, syn bool, payload []byte) []byte {
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2)}
		tcp := &layers.TCP{SrcPort: 80, DstPort: 50100, Seq: seq, SYN: syn, ACK: true, Window: 4096}
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
		buffer := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(payload)))
		return bytes.Clone(buffer.Bytes())
	}
	// Split the first two-byte portion of Done again across physical segments.
	records := [][]byte{packet(100, true, nil), packet(154, false, stream[53:]), packet(101, false, stream[:53]), packet(101, false, stream[:53])}
	observed := tlsControlTestObserve(t, records, false)
	require.Len(t, observed, 2)
	var gotDone, gotTicket *tlsControlTestObserved
	for i := range observed {
		if observed[i].wire[0] == 14 {
			gotDone = &observed[i]
		} else {
			gotTicket = &observed[i]
		}
	}
	require.NotNil(t, gotDone)
	require.NotNil(t, gotTicket)
	require.Equal(t, done, gotDone.wire)
	require.Equal(t, ticket, gotTicket.wire)
	require.Equal(t, []tlsControlTestOrigin{{3, 106}, {2, 54}, {2, 60}, {2, 61}}, gotDone.origins)
	for _, item := range observed {
		for i, origin := range item.origins {
			require.Equal(t, item.wire[i], records[origin.frame-1][origin.offset])
		}
		n := protocolCorpusRequireBoundedRuleParse(t, item.wire, tlsControlTestRule, tlsControlTestEntry(item.wire[0]))
		tlsControlTestFields(t, n, item.wire, 0)
	}
	require.Empty(t, tlsControlTestObserve(t, records[1:], false), "no observed SYN must not invent stream start")
	require.Empty(t, tlsControlTestObserve(t, records[:2], false), "a missing initial segment must not be skipped")
	// Independently split a CCS record inside its header, reorder its segments,
	// and retransmit the first segment. Its origins must survive deduplication.
	ccsWire := record(20, []byte{1})
	ccsStart := bytes.Index(stream, ccsWire)
	require.Positive(t, ccsStart)
	split := ccsStart + 3
	ccsRecords := [][]byte{packet(100, true, nil), packet(101+uint32(split), false, stream[split:]), packet(101, false, stream[:split]), packet(101, false, stream[:split])}
	ccs, alerts := tlsCCSTestObserve(t, ccsRecords, false)
	require.Len(t, ccs, 1)
	require.Empty(t, alerts)
	require.Equal(t, ccsWire, ccs[0].wire)
	for i, origin := range ccs[0].origins {
		want := tlsControlTestOrigin{3, 54 + ccsStart + i}
		if i >= 3 {
			want = tlsControlTestOrigin{2, 54 + i - 3}
		}
		require.Equal(t, want, origin)
		require.Equal(t, ccsWire[i], ccsRecords[origin.frame-1][origin.offset])
	}
	ccs, _ = tlsCCSTestObserve(t, ccsRecords[1:], false)
	require.Empty(t, ccs, "no SYN must not invent stream start")
	ccs, _ = tlsCCSTestObserve(t, ccsRecords[:2], false)
	require.Empty(t, ccs, "a missing first segment must not be skipped")
}
