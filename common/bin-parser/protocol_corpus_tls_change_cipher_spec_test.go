package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const tlsCCSTestRule = "application-layer.tls_change_cipher_spec"
const tlsCCSTestEntry = "TLSChangeCipherSpecRecord"

func tlsCCSTestFields(t *testing.T, n *base.Node, wire []byte, offset uint64) {
	t.Helper()
	latTestField(t, n, "Content Type", "uint8", offset, offset+8, uint64(20))
	latTestField(t, n, "Legacy Record Version", "uint16", offset+8, offset+24, uint64(binary.BigEndian.Uint16(wire[1:3])))
	latTestField(t, n, "Record Length", "uint16", offset+24, offset+40, uint64(1))
	latTestField(t, n, "ChangeCipherSpec Value", "uint8", offset+40, offset+48, uint64(1))
	native := protocolCorpusFindNode(n, "Content Type").Cfg.GetItem(base.CfgParent).(*base.Node)
	latTestTree(t, native, offset, offset+48)
	info := native.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, "TLS plaintext ChangeCipherSpec record layout", info["Profile"])
	require.Equal(t, true, info["Plaintext Context Supplied By Caller"])
	for _, key := range []string{"Protocol Version Inferred", "Handshake Phase Validated", "Cipher State Transition Validated", "Handshake Completion Validated", "TCP Reassembly Performed", "Structured Generation Supported"} {
		require.Equal(t, false, info[key], key)
	}
}

func TestProtocolCorpusTLSChangeCipherSpecBoundaries(t *testing.T) {
	for _, version := range []byte{1, 2, 3} {
		wire := []byte{20, 3, version, 0, 1, 1}
		for _, entry := range []string{tlsCCSTestEntry, tlsCCSTestEntry + "Carrier"} {
			n := protocolCorpusRequireBoundedRuleParse(t, wire, tlsCCSTestRule, entry)
			tlsCCSTestFields(t, n, wire, 0)
			require.Equal(t, wire, NodeToBytes(n))
			_, err := parser.ParseBinary(bytes.NewReader(wire), tlsCCSTestRule, entry)
			require.ErrorContains(t, err, "explicit")
			_, err = parser.GenerateBinary(map[string]any{}, tlsCCSTestRule, entry)
			require.Error(t, err)
		}
		var invalid [][]byte
		for cut := 0; cut < len(wire); cut++ {
			invalid = append(invalid, bytes.Clone(wire[:cut]))
		}
		invalid = append(invalid, append(bytes.Clone(wire), 0), append(bytes.Clone(wire), wire...))
		// Every byte is checked, not merely a record type/magic prefix.
		for i := range wire {
			bad := bytes.Clone(wire)
			bad[i] = 0xff
			invalid = append(invalid, bad)
		}
		invalid = append(invalid, []byte{20, 3, 0, 0, 1, 1}, []byte{20, 3, 4, 0, 1, 1}, []byte{20, 3, 3, 0, 1, 0})
		for _, bad := range invalid {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), tlsCCSTestRule, tlsCCSTestEntry)
			require.Error(t, err)
			if len(bad) == 0 {
				continue
			}
			n := protocolCorpusRequireBoundedRuleParse(t, bad, tlsCCSTestRule, tlsCCSTestEntry+"Carrier")
			latTestField(t, n, "Unparsed TLS ChangeCipherSpec Record", "raw", 0, uint64(len(bad))*8, bad)
			require.Nil(t, protocolCorpusFindNode(n, "Content Type"))
			require.Equal(t, bad, NodeToBytes(n))
		}
	}
	for _, entry := range []string{tlsCCSTestEntry, tlsCCSTestEntry + "Carrier"} {
		for _, bits := range []uint64{0, 1, 7, 9, 47, 49, 16390 * 8} {
			r := &tlsSHTestHeldReader{bits: bits}
			_, err := parser.ParseBinary(r, tlsCCSTestRule, entry)
			require.Error(t, err)
			require.Zero(t, r.reads, "%s boundary %d", entry, bits)
		}
	}
	maximum := make([]byte, 16389)
	n := protocolCorpusRequireBoundedRuleParse(t, maximum, tlsCCSTestRule, tlsCCSTestEntry+"Carrier")
	latTestField(t, n, "Unparsed TLS ChangeCipherSpec Record", "raw", 0, uint64(len(maximum))*8, maximum)
}

func TestProtocolCorpusTLSChangeCipherSpecIsolation(t *testing.T) {
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			wire := []byte{20, 3, byte(1 + worker%3), 0, 1, 1}
			cfg := map[string]any{"caller-ccs": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, tlsCCSTestRule, tlsCCSTestEntry, cfg)
			tlsCCSTestFields(t, n, wire, 0)
			n.Cfg.GetItem("additionInfo").(map[string]any)["Cipher State Transition Validated"] = true
			again := protocolCorpusRequireBoundedRuleParse(t, wire, tlsCCSTestRule, tlsCCSTestEntry)
			tlsCCSTestFields(t, again, wire, 0)
			require.Equal(t, map[string]any{"caller-ccs": worker}, cfg)
		})
	}
}

func TestProtocolCorpusTLSChangeCipherSpecOffsetsAndRollback(t *testing.T) {
	for offset := uint64(0); offset < 8; offset++ {
		for _, valid := range []bool{true, false} {
			wire := []byte{20, 3, 3, 0, 1, 1}
			if !valid {
				wire[5] = 0
			}
			var packed bytes.Buffer
			w := base.NewBitWriter(&packed)
			if offset > 0 {
				require.NoError(t, w.WriteBits([]byte{0x55}, offset))
			}
			require.NoError(t, w.WriteBits(wire, 48))
			require.NoError(t, w.WriteBits([]byte{0xd3}, 8))
			if offset > 0 {
				require.NoError(t, w.WriteBits([]byte{0}, 8-offset))
			}
			root := latTestInline(t, fmt.Sprintf(`endian: little
Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Message").SetMaxLength(6)
      this.ProcessSubNode("Message")
      this.ProcessSubNode("Sentinel")
      if %d > 0 { this.ProcessSubNode("Padding") }
    Prefix: uint8,%dbit
    Message: "import:application-layer/tls_change_cipher_spec.yaml;node:TLSChangeCipherSpecRecordCarrier"
    Sentinel: uint8
    Padding: uint8,%dbit
`, offset, offset, offset, 8-offset))
			root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
			root.Ctx.SetItem("caller-ccs", "preserved")
			r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
			require.NoError(t, r.Backup())
			require.NoError(t, root.ParseSubNode(r, "Envelope"))
			n := base.GetNodeByPath(root, "@Envelope")
			if valid {
				tlsCCSTestFields(t, protocolCorpusFindNode(n, "Message"), wire, offset)
			} else {
				latTestField(t, n, "Unparsed TLS ChangeCipherSpec Record", "raw", offset, offset+48, wire)
				require.Nil(t, protocolCorpusFindNode(n, "Content Type"))
			}
			protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
			require.Equal(t, "preserved", root.Ctx.GetItem("caller-ccs"))
			require.Equal(t, packed.Bytes(), NodeToBytes(n))
			require.NoError(t, r.Recovery())
			got, err := r.ReadBits(uint64(packed.Len()) * 8)
			require.NoError(t, err)
			require.Equal(t, packed.Bytes(), got)
			require.ErrorContains(t, r.PopBackup(), "no backup")
		}
	}
}

// Test-only stream traversal starts at observed SYN sequence + 1, checks all
// overlaps and stops at a gap. Only record envelopes are traversed after the
// first CCS; protected body bytes are never reinterpreted as handshake fields.
func tlsCCSTestObserve(t *testing.T, records [][]byte, rawIP bool) (ccs, alerts []tlsControlTestObserved) {
	t.Helper()
	for _, flow := range tlsControlTestFlows(t, records, rawIP) {
		if !flow.syn {
			continue
		}
		sort.SliceStable(flow.segments, func(i, j int) bool {
			return uint32(flow.segments[i].seq-flow.initial) < uint32(flow.segments[j].seq-flow.initial)
		})
		var stream []byte
		var origins []tlsControlTestOrigin
		for _, segment := range flow.segments {
			offset := uint32(segment.seq - flow.initial)
			if uint64(offset) > uint64(len(stream)) {
				break
			}
			for i, b := range segment.data {
				at := int(offset) + i
				if at < len(stream) {
					require.Equal(t, stream[at], b, "conflicting TCP overlap")
					continue
				}
				stream = append(stream, b)
				origins = append(origins, tlsControlTestOrigin{segment.origin.frame, segment.origin.offset + i})
			}
		}
		changed := false
		for at := 0; len(stream)-at >= 5; {
			typ := stream[at]
			if typ < 20 || typ > 23 || stream[at+1] != 3 || stream[at+2] < 1 || stream[at+2] > 3 {
				break
			}
			length := int(binary.BigEndian.Uint16(stream[at+3 : at+5]))
			if length > 18432 || length > len(stream)-at-5 {
				break
			}
			end := at + 5 + length
			item := tlsControlTestObserved{bytes.Clone(stream[at:end]), append([]tlsControlTestOrigin(nil), origins[at:end]...)}
			if typ == 20 && !changed {
				ccs = append(ccs, item)
				changed = true
			} else if typ == 21 && changed {
				alerts = append(alerts, item)
			} else if !changed && typ != 22 {
				break
			}
			at = end
		}
	}
	for _, items := range [][]tlsControlTestObserved{ccs, alerts} {
		sort.Slice(items, func(i, j int) bool {
			a, b := items[i].origins[0], items[j].origins[0]
			if a.frame == b.frame {
				return a.offset < b.offset
			}
			return a.frame < b.frame
		})
	}
	return
}

func TestProtocolCorpusTLSChangeCipherSpecAllOriginalRecords(t *testing.T) {
	// Expected frames independently cross-checked with tshark's record tree.
	// TLS 1.3 compatibility CCS in DingTalk/DoH have the same wire bytes as
	// earlier-version CCS; the parser deliberately cannot infer that distinction.
	specs := []struct {
		name, sha string
		records   int
		rawIP     bool
		frames    []int
	}{
		{"anydesk.pcapng", "21082a0bf1af60d6acbd2f736e4686938050065d2b48f37053d9a64791b1c09a", 174, false, []int{20, 22, 79, 81, 87, 94, 130, 131}},
		{"dingtalk.pcap", "238ee8af258426a8f51bbf7a6222b0b9437d2822fac91ab0391f670c89960d5f", 16, true, []int{11, 13}},
		{"doh.pcap", "b1459348b4a72c24e5646fdc05131ded69afb2fc543dc5568d004807939554c4", 142, false, []int{6, 12}},
		{"dot.pcap", "8f6125abecbf0e28ab824246581ba5fa3b42dcdeb8626d191f25e63699b5fb78", 24, false, []int{8, 11}},
		{"imaps.pcap", "b53f4d242f620db57ad82f298552aec59b60dc8fc9f77d16a5d2d057ecfe7c59", 28, false, []int{13, 15}},
		{"wechat.pcap", "2d82f575a8b9addfc9e66582e34911f79b5388984e54299293a28393c6d7c24e", 1672, false, []int{24, 27, 104, 105, 130, 144, 156, 160, 202, 203, 242, 243, 271, 280, 292, 299, 370, 371, 394, 399, 504, 505, 525, 540, 549, 562, 564, 575, 579, 596, 654, 663, 858, 859, 902, 903, 972, 992, 1063, 1064, 1083, 1087, 1144, 1145, 1170, 1175, 1234, 1235, 1272, 1273, 1440, 1442, 1488, 1493, 1525, 1526}},
		{"smtps.pcapng", "8d68f3726c5ea2b8527cf52a28b8e132b2c332633562f24280f82de05832f169", 4, false, nil},
		{"netease-games.pcapng", "f662aff63082da1da601841bdb3d88f4ca557be2bdb8528ee746f910bd28d38a", 20, false, nil},
	}
	totalRecords, totalCCS, totalAlerts := 0, 0, 0
	for _, spec := range specs {
		t.Run(spec.name, func(t *testing.T) {
			path := "testdata/protocol-corpus/captures/ndpi/ndpi-" + spec.name
			capture, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, spec.sha, fmt.Sprintf("%x", sha256.Sum256(capture)))
			records := protocolCorpusAuditPackets(t, path)
			require.Len(t, records, spec.records)
			totalRecords += len(records)
			ccs, alerts := tlsCCSTestObserve(t, records, spec.rawIP)
			require.Len(t, ccs, len(spec.frames))
			for i, item := range ccs {
				first := item.origins[0]
				require.Equal(t, spec.frames[i], first.frame)
				require.Equal(t, []byte{20, 3, 3, 0, 1, 1}, item.wire)
				for j, origin := range item.origins {
					require.Equal(t, tlsControlTestOrigin{first.frame, first.offset + j}, origin)
					require.Equal(t, item.wire[j], records[origin.frame-1][origin.offset])
				}
				for _, entry := range []string{tlsCCSTestEntry, tlsCCSTestEntry + "Carrier"} {
					n := protocolCorpusRequireBoundedRuleParse(t, item.wire, tlsCCSTestRule, entry)
					tlsCCSTestFields(t, n, item.wire, 0)
					require.Equal(t, item.wire, NodeToBytes(n))
				}
				frame := records[first.frame-1]
				tail := len(frame) - first.offset - 6
				root := latTestInline(t, fmt.Sprintf("unit: byte\nPackage:\n  Capture:\n    operator: |\n      this.ProcessSubNode(\"Envelope\")\n      this.GetSubNode(\"Message\").SetMaxLength(6)\n      this.ProcessSubNode(\"Message\")\n      if %d > 0 { this.ProcessSubNode(\"Tail\") }\n    Envelope: raw,%d\n    Message: \"import:application-layer/tls_change_cipher_spec.yaml;node:TLSChangeCipherSpecRecord\"\n    Tail: raw,%d\n", tail, first.offset, tail))
				root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
				require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(frame)), "Capture"))
				whole := base.GetNodeByPath(root, "@Capture")
				require.True(t, bytes.Equal(frame, NodeToBytes(whole)))
				tlsCCSTestFields(t, protocolCorpusFindNode(whole, "Message"), item.wire, uint64(first.offset)*8)
				totalCCS++
			}
			if spec.name != "wechat.pcap" {
				require.Empty(t, alerts)
			} else {
				// Frame 374 has no captured SYN/handshake; it must not gain
				// invented stream provenance merely to satisfy a count.
				require.Equal(t, 1, len(alerts))
				require.Equal(t, 1114, alerts[0].origins[0].frame)
				for _, frameNumber := range []int{374, 1114} {
					frame := records[frameNumber-1]
					require.Len(t, frame, 85)
					wire := frame[54:] // Independently checked tshark PDML span.
					require.Equal(t, []byte{21, 3, 3, 0, 26}, wire[:5])
					_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), tlsCCSTestRule, tlsCCSTestEntry)
					require.Error(t, err)
					n := protocolCorpusRequireBoundedRuleParse(t, wire, tlsCCSTestRule, tlsCCSTestEntry+"Carrier")
					require.Nil(t, protocolCorpusFindNode(n, "ChangeCipherSpec Value"))
					require.True(t, bytes.Equal(wire, NodeToBytes(n)))
					totalAlerts++
				}
			}
		})
	}
	require.Equal(t, 2080, totalRecords)
	require.Equal(t, 72, totalCCS)
	require.Equal(t, 2, totalAlerts)
}
