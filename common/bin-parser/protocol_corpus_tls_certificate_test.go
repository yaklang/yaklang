package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

type tlsCertificateSourceSegment struct {
	Frame, FrameOffset int
	Seq                uint32
	Payload            []byte
}
type tlsCertificateSourceFlow struct {
	SYN      bool
	Initial  uint32
	Pair     string
	Segments []tlsCertificateSourceSegment
}
type tlsCertificateSourceFragment struct {
	Frame, FrameOffset, StreamOffset, RecordOffset, MessageOffset, Length int
	Duplicate                                                             bool // Identical source bytes also appeared in an earlier frame.
	SHA256                                                                string
}
type tlsCertificateObservedHandshake struct {
	Wire               []byte
	Complete, TLS12    bool
	Fragments          []tlsCertificateSourceFragment
	RecordMissingBytes int
	pair               string
}
type tlsCertificateByteOrigin struct{ StreamOffset, RecordStart int }

func tlsCertificateTestSHA(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
func tlsCertificateTestU24(b []byte) int       { return int(b[0])<<16 | int(b[1])<<8 | int(b[2]) }

// Independent, test-only framing oracle. Begin only at an observed SYN+1,
// retain/check all overlapping source segments, never bridge unavailable bytes,
// walk TLS record/handshake lengths and stop at CCS or application data. This
// does not make protected type-22 bytes look like a plaintext handshake.
func tlsCertificateTestObserve(t *testing.T, records [][]byte, rawIP bool) []tlsCertificateObservedHandshake {
	t.Helper()
	flows := map[string]*tlsCertificateSourceFlow{}
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
		network := packet.NetworkLayer().NetworkFlow()
		src := network.Src().String() + fmt.Sprintf(":%d", tcp.SrcPort)
		dst := network.Dst().String() + fmt.Sprintf(":%d", tcp.DstPort)
		key := src + " -> " + dst
		pair := key
		if src > dst {
			pair = dst + " -> " + src
		}
		flow := flows[key]
		if flow == nil {
			flow = &tlsCertificateSourceFlow{Pair: pair}
			flows[key] = flow
		}
		if tcp.SYN {
			if flow.SYN {
				require.Equal(t, flow.Initial, tcp.Seq+1, "SYN tuple reuse needs separate connection provenance")
			}
			flow.SYN = true
			flow.Initial = tcp.Seq + 1
		}
		if len(tcp.Payload) > 0 {
			// Link-layer padding can follow the IP packet (e.g. AnyDesk frame
			// 18). Derive the payload origin from decoded physical headers,
			// not len(frame)-len(payload), which would silently shift its span.
			offset := 0
			for _, layer := range packet.Layers() {
				offset += len(layer.LayerContents())
				if layer.LayerType() == layers.LayerTypeTCP {
					break
				}
			}
			require.LessOrEqual(t, offset+len(tcp.Payload), len(record))
			require.True(t, bytes.Equal(record[offset:offset+len(tcp.Payload)], tcp.Payload), "frame %d payload offset", i+1)
			flow.Segments = append(flow.Segments, tlsCertificateSourceSegment{Frame: i + 1, FrameOffset: offset, Seq: tcp.Seq, Payload: tcp.Payload})
		}
	}
	var messages []tlsCertificateObservedHandshake
	tls12Pairs := map[string]bool{}
	for _, flow := range flows {
		if !flow.SYN {
			continue
		}
		sort.SliceStable(flow.Segments, func(i, j int) bool {
			return uint32(flow.Segments[i].Seq-flow.Initial) < uint32(flow.Segments[j].Seq-flow.Initial)
		})
		var stream []byte
		var earliestFrame []int
		for _, segment := range flow.Segments {
			offset := int(uint32(segment.Seq - flow.Initial))
			if offset > len(stream) {
				break
			}
			for j, b := range segment.Payload {
				at := offset + j
				if at < len(stream) {
					require.True(t, stream[at] == b, "frame %d inconsistent repeated byte at stream offset %d", segment.Frame, at)
					if segment.Frame < earliestFrame[at] {
						earliestFrame[at] = segment.Frame
					}
					continue
				}
				stream = append(stream, b)
				earliestFrame = append(earliestFrame, segment.Frame)
			}
		}
		observed := tlsCertificateTestFrameStream(t, stream)
		for _, message := range observed {
			m := tlsCertificateObservedHandshake{Wire: message.wire, Complete: message.complete, RecordMissingBytes: message.missing, pair: flow.Pair}
			// Match every continuous logical piece against every original segment.
			// This preserves duplicate observations as separate source fragments.
			for at := 0; at < len(message.origins); {
				end := at + 1
				for end < len(message.origins) && message.origins[end].RecordStart == message.origins[at].RecordStart && message.origins[end].StreamOffset == message.origins[end-1].StreamOffset+1 {
					end++
				}
				from, to := message.origins[at].StreamOffset, message.origins[end-1].StreamOffset+1
				for _, segment := range flow.Segments {
					segStart := int(uint32(segment.Seq - flow.Initial))
					segEnd := segStart + len(segment.Payload)
					lo, hi := max(from, segStart), min(to, segEnd)
					for lo < hi {
						duplicate := earliestFrame[lo] < segment.Frame
						stop := lo + 1
						for stop < hi && (earliestFrame[stop] < segment.Frame) == duplicate {
							stop++
						}
						position := segment.FrameOffset + lo - segStart
						m.Fragments = append(m.Fragments, tlsCertificateSourceFragment{
							Frame: segment.Frame, FrameOffset: position, StreamOffset: lo, RecordOffset: lo - message.origins[at].RecordStart,
							MessageOffset: at + lo - from, Length: stop - lo, Duplicate: duplicate, SHA256: tlsCertificateTestSHA(records[segment.Frame-1][position : position+stop-lo]),
						})
						lo = stop
					}
				}
				at = end
			}
			sort.Slice(m.Fragments, func(i, j int) bool {
				a, b := m.Fragments[i], m.Fragments[j]
				if a.MessageOffset != b.MessageOffset {
					return a.MessageOffset < b.MessageOffset
				}
				return a.Frame < b.Frame
			})
			if m.Complete && m.Wire[0] == 2 {
				tls12Pairs[flow.Pair] = tlsCertificateTestIsTLS12Hello(m.Wire)
			}
			messages = append(messages, m)
		}
	}
	for i := range messages {
		messages[i].TLS12 = tls12Pairs[messages[i].pair]
	}
	sort.Slice(messages, func(i, j int) bool {
		a, b := messages[i].Fragments[0], messages[j].Fragments[0]
		if a.Frame != b.Frame {
			return a.Frame < b.Frame
		}
		return a.FrameOffset < b.FrameOffset
	})
	return messages
}

// The fixed captures have a complete ordinary ServerHello before each TLS 1.2
// certificate. Inspect its version/extension lengths independently, without
// treating an application's name, port or dissector label as a version oracle.
func tlsCertificateTestIsTLS12Hello(wire []byte) bool {
	if len(wire) < 42 || wire[0] != 2 || wire[4] != 3 || wire[5] != 3 {
		return false
	}
	at := 39 + int(wire[38]) + 3
	if at > len(wire) {
		return false
	}
	if at == len(wire) {
		return true
	}
	if len(wire)-at < 2 || int(binary.BigEndian.Uint16(wire[at:at+2])) != len(wire)-at-2 {
		return false
	}
	at += 2
	for at < len(wire) {
		if len(wire)-at < 4 {
			return false
		}
		typ := binary.BigEndian.Uint16(wire[at : at+2])
		length := int(binary.BigEndian.Uint16(wire[at+2 : at+4]))
		if length > len(wire)-at-4 || typ == 43 {
			return false
		}
		at += 4 + length
	}
	return true
}

type tlsCertificateFramedHandshake struct {
	wire     []byte
	origins  []tlsCertificateByteOrigin
	complete bool
	missing  int
}

// Pure record-boundary oracle also exercised with synthetic split-record
// vectors below. Original captures' complete Certificate messages span TCP
// packets, but none of their 38 messages span multiple TLS records.
func tlsCertificateTestFrameStream(t *testing.T, stream []byte) []tlsCertificateFramedHandshake {
	t.Helper()
	var messages []tlsCertificateFramedHandshake
	var pending []byte
	var origins []tlsCertificateByteOrigin
	missing := 0
	for at := 0; len(stream)-at >= 5; {
		if stream[at] != 22 || stream[at+1] != 3 || stream[at+2] < 1 || stream[at+2] > 3 {
			break
		}
		size := int(binary.BigEndian.Uint16(stream[at+3 : at+5]))
		if size == 0 || size > 16384 {
			break
		}
		available := min(size, len(stream)-at-5)
		pending = append(pending, stream[at+5:at+5+available]...)
		for offset := at + 5; offset < at+5+available; offset++ {
			origins = append(origins, tlsCertificateByteOrigin{StreamOffset: offset, RecordStart: at})
		}
		for len(pending) >= 4 {
			n := tlsCertificateTestU24(pending[1:4]) + 4
			if n > len(pending) {
				break
			}
			messages = append(messages, tlsCertificateFramedHandshake{wire: append([]byte(nil), pending[:n]...), origins: append([]tlsCertificateByteOrigin(nil), origins[:n]...), complete: true})
			pending = pending[n:]
			origins = origins[n:]
		}
		if available < size {
			missing = size - available
			break
		}
		at += 5 + size
	}
	if len(pending) > 0 {
		messages = append(messages, tlsCertificateFramedHandshake{wire: append([]byte(nil), pending...), origins: origins, missing: missing})
	}
	return messages
}

func tlsCertificateTestVerifySources(t *testing.T, message tlsCertificateObservedHandshake, records [][]byte) {
	t.Helper()
	seen := make([]bool, len(message.Wire))
	rebuilt := make([]byte, len(message.Wire))
	for _, fragment := range message.Fragments {
		require.Greater(t, fragment.Length, 0)
		require.GreaterOrEqual(t, fragment.RecordOffset, 5)
		record := records[fragment.Frame-1]
		require.LessOrEqual(t, fragment.FrameOffset+fragment.Length, len(record))
		raw := record[fragment.FrameOffset : fragment.FrameOffset+fragment.Length]
		require.Equal(t, fragment.SHA256, tlsCertificateTestSHA(raw))
		for i, b := range raw {
			at := fragment.MessageOffset + i
			require.Less(t, at, len(message.Wire))
			if seen[at] {
				require.True(t, rebuilt[at] == b, "conflicting source byte")
			}
			rebuilt[at] = b
			seen[at] = true
		}
	}
	for _, covered := range seen {
		require.True(t, covered, "missing source byte")
	}
	require.Equal(t, tlsCertificateTestSHA(message.Wire), tlsCertificateTestSHA(rebuilt))
}

// Hash comparisons avoid printing certificate contents even on a test failure.
// For reassembled inputs these are logical input spans, never physical PCAP spans.
func tlsCertificateTestTree(t *testing.T, n *base.Node, wire []byte, offset uint64) {
	t.Helper()
	var walk func(*base.Node, uint64) uint64
	walk = func(n *base.Node, at uint64) uint64 {
		if stream_parser.NodeHasResult(n) {
			span := stream_parser.GetNodeResultPos(n)
			require.Equal(t, at, span[0], n.Name)
			require.LessOrEqual(t, span[1], offset+uint64(len(wire))*8, n.Name)
			require.Zero(t, (span[0]-offset)%8)
			require.Zero(t, (span[1]-offset)%8)
			expected := wire[(span[0]-offset)/8 : (span[1]-offset)/8]
			require.Equal(t, tlsCertificateTestSHA(expected), tlsCertificateTestSHA(stream_parser.GetBytesByNode(n)), n.Name)
			result, err := n.Result()
			require.NoError(t, err)
			require.Same(t, n, result.Origin)
			return span[1]
		}
		for _, child := range n.Children {
			at = walk(child, at)
		}
		return at
	}
	require.Equal(t, offset+uint64(len(wire))*8, walk(n, offset))
	require.Equal(t, uint64(len(wire))*8, stream_parser.CalcNodeConsumedLength(n))
	tlsSHTestValues(t, n)
}

type tlsCertificateTestCaptureSpec struct {
	name, sha   string
	records     int
	rawIP       bool
	firstFrames []int
}

func tlsCertificateTestSpecs() []tlsCertificateTestCaptureSpec {
	return []tlsCertificateTestCaptureSpec{
		{"anydesk.pcapng", "21082a0bf1af60d6acbd2f736e4686938050065d2b48f37053d9a64791b1c09a", 174, false, []int{14, 20, 72, 79, 85, 87, 126, 130}},
		{"dingtalk.pcap", "238ee8af258426a8f51bbf7a6222b0b9437d2822fac91ab0391f670c89960d5f", 16, true, nil},
		{"doh.pcap", "b1459348b4a72c24e5646fdc05131ded69afb2fc543dc5568d004807939554c4", 142, false, nil},
		{"dot.pcap", "8f6125abecbf0e28ab824246581ba5fa3b42dcdeb8626d191f25e63699b5fb78", 24, false, []int{6}},
		{"imaps.pcap", "b53f4d242f620db57ad82f298552aec59b60dc8fc9f77d16a5d2d057ecfe7c59", 28, false, []int{6}},
		{"wechat.pcap", "2d82f575a8b9addfc9e66582e34911f79b5388984e54299293a28393c6d7c24e", 1672, false, []int{18, 98, 124, 151, 196, 238, 265, 286, 366, 389, 500, 521, 544, 558, 567, 648, 852, 896, 968, 1057, 1077, 1138, 1166, 1228, 1268, 1434, 1484, 1521}},
		{"smtps.pcapng", "8d68f3726c5ea2b8527cf52a28b8e132b2c332633562f24280f82de05832f169", 4, false, nil},
		{"netease-games.pcapng", "f662aff63082da1da601841bdb3d88f4ca557be2bdb8528ee746f910bd28d38a", 20, false, nil},
	}
}

func TestProtocolCorpusTLSCertificateAllOriginalRecords(t *testing.T) {
	specs := tlsCertificateTestSpecs()
	total, certificates, crossPackets, crossRecords, truncated := 0, 0, 0, 0, 0
	for _, spec := range specs {
		t.Run(spec.name, func(t *testing.T) {
			path := "testdata/protocol-corpus/captures/ndpi/ndpi-" + spec.name
			file, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, spec.sha, tlsCertificateTestSHA(file))
			records := protocolCorpusAuditPackets(t, path)
			require.Len(t, records, spec.records)
			var firstFrames []int
			for _, message := range tlsCertificateTestObserve(t, records, spec.rawIP) {
				if message.Wire[0] != 11 {
					continue
				}
				require.True(t, message.TLS12, "type11 requires independently observed TLS1.2 ServerHello")
				tlsCertificateTestVerifySources(t, message, records)
				first := message.Fragments[0].Frame
				t.Run(fmt.Sprintf("certificate-first-frame-%d", first), func(t *testing.T) {
					if !message.Complete {
						truncated++
						require.Equal(t, "imaps.pcap", spec.name)
						require.Equal(t, 27, first)
						require.Len(t, message.Wire, 2673)
						require.Equal(t, 3250, tlsCertificateTestU24(message.Wire[1:4])+4)
						require.Equal(t, 577, message.RecordMissingBytes)
						_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(message.Wire), "application-layer.tls_certificate", "TLSHandshakeCertificate")
						require.Error(t, err)
						n := protocolCorpusRequireBoundedRuleParse(t, message.Wire, "application-layer.tls_certificate", "TLSHandshakeCertificateCarrier")
						tlsCertificateTestTree(t, n, message.Wire, 0)
						require.Nil(t, protocolCorpusFindNode(n, "Certificates"))
						raw := protocolCorpusFindNode(n, "Unparsed TLS Certificate Handshake")
						require.NotNil(t, raw)
						require.Equal(t, tlsCertificateTestSHA(message.Wire), tlsCertificateTestSHA(stream_parser.GetBytesByNode(raw)))
						return
					}
					firstFrames = append(firstFrames, first)
					total++
					frames, recordStarts := map[int]bool{}, map[int]bool{}
					for _, fragment := range message.Fragments {
						frames[fragment.Frame] = true
						recordStarts[fragment.StreamOffset-fragment.RecordOffset] = true
					}
					if len(frames) > 1 {
						crossPackets++
					}
					if len(recordStarts) > 1 {
						crossRecords++
					}
					n := protocolCorpusRequireBoundedRuleParse(t, message.Wire, "application-layer.tls_certificate", "TLSHandshakeCertificate")
					tlsCertificateTestTree(t, n, message.Wire, 0)
					require.Equal(t, tlsCertificateTestSHA(message.Wire), tlsCertificateTestSHA(NodeToBytes(n)))
					protocolCorpusRequireValue(t, n, "Handshake Type", uint64(11))
					protocolCorpusRequireValue(t, n, "Handshake Length", uint64(len(message.Wire)-4))
					protocolCorpusRequireValue(t, n, "Certificate List Length", uint64(len(message.Wire)-7))
					list := protocolCorpusFindNode(n, "Certificates")
					require.NotNil(t, list)
					require.True(t, list.Cfg.GetBool(stream_parser.CfgIsList))
					var lengths []int
					for at, index := 7, 0; at < len(message.Wire); index++ {
						length := tlsCertificateTestU24(message.Wire[at : at+3])
						lengths = append(lengths, length)
						protocolCorpusRequireValue(t, list.Children[index], "Certificate Length", uint64(length))
						require.Equal(t, index, list.Children[index].Cfg.GetItem(stream_parser.CfgElementIndex))
						der := protocolCorpusFindNode(list.Children[index], "Certificate DER")
						require.Equal(t, "raw", der.Cfg.GetString(base.CfgType))
						require.Equal(t, tlsCertificateTestSHA(message.Wire[at+3:at+3+length]), tlsCertificateTestSHA(stream_parser.GetBytesByNode(der)))
						at += 3 + length
					}
					require.Len(t, list.Children, len(lengths))
					certificates += len(lengths)
					info := n.Cfg.GetItem("additionInfo").(map[string]any)
					require.Equal(t, len(lengths), info["Certificate Count"])
					require.Equal(t, true, info["Version Is Caller Supplied"])
					for _, key := range []string{"DER Parsed", "Sender Role Validated", "Certificate Trust Validated", "Peer Identity Validated", "Handshake Completion Validated", "TCP Reassembly Performed", "Record Reassembly Performed"} {
						require.Equal(t, false, info[key])
					}
					// Reproducible provenance without logging DER or certificate names.
					t.Logf("source=%s first_frame=%d handshake_bytes=%d sha256=%s certificate_lengths=%v source_fragments=%+v", spec.name, first, len(message.Wire), tlsCertificateTestSHA(message.Wire), lengths, message.Fragments)
				})
			}
			require.Equal(t, spec.firstFrames, firstFrames)
		})
	}
	require.Equal(t, 38, total)
	require.Equal(t, 73, certificates)
	require.Equal(t, 30, crossPackets)
	require.Zero(t, crossRecords)
	require.Equal(t, 1, truncated)
}

func TestProtocolCorpusTLSCertificateInlineFramingAndBoundaries(t *testing.T) {
	// Opaque bytes are not claimed to be valid X.509 certificates.
	good := []byte{11, 0, 0, 12, 0, 0, 9, 0, 0, 2, 0x30, 0, 0, 0, 1, 0xff}
	for _, wire := range [][]byte{good, {11, 0, 0, 3, 0, 0, 0}} {
		n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.tls_certificate", "TLSHandshakeCertificate")
		tlsCertificateTestTree(t, n, wire, 0)
		certificates := protocolCorpusFindNode(n, "Certificates")
		require.NotNil(t, certificates)
		require.True(t, certificates.Cfg.GetBool(stream_parser.CfgIsList))
		certificateResult, err := certificates.Result()
		require.NoError(t, err)
		require.True(t, certificateResult.IsList())
		if len(wire) == 7 {
			require.Empty(t, certificates.Children)
			require.Empty(t, certificateResult.Children())
			require.Equal(t, []any{}, NodeToMap(certificates))
			certificates.Cfg.SetItem("out", "panic(\"NodeToMap must not execute out\")")
			require.Equal(t, []any{}, NodeToMap(certificates))
			certificates.Cfg.DeleteItem("out")
			require.Equal(t, [2]uint64{56, 56}, stream_parser.GetNodeResultPos(certificates))
		}
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.tls_certificate", "TLSHandshakeCertificate")
			require.Error(t, err, "cut %d", cut)
		}
	}
	// One real wire layout split at every handshake byte across TLS records.
	// These inline vectors test the extraction oracle, not new captured evidence.
	record := func(b []byte) []byte { return append([]byte{22, 3, 3, byte(len(b) >> 8), byte(len(b))}, b...) }
	for cut := 1; cut < len(good); cut++ {
		wire := append(record(good[:cut]), record(good[cut:])...)
		messages := tlsCertificateTestFrameStream(t, wire)
		require.Len(t, messages, 1)
		require.True(t, messages[0].complete)
		require.Equal(t, tlsCertificateTestSHA(good), tlsCertificateTestSHA(messages[0].wire))
		for i, origin := range messages[0].origins {
			require.Equal(t, good[i], wire[origin.StreamOffset])
			require.GreaterOrEqual(t, origin.StreamOffset-origin.RecordStart, 5)
		}
	}
	for _, typ := range []byte{20, 23} {
		protected := append([]byte{typ, 3, 3, 0, 1, 1}, record(good)...)
		require.Empty(t, tlsCertificateTestFrameStream(t, protected), "no plaintext scan after CCS/application data")
	}
	for _, bad := range [][]byte{{11, 0, 0, 6, 0, 0, 3, 0, 0, 0}, append(append([]byte(nil), good...), 0), good[:len(good)-1]} {
		n := protocolCorpusRequireBoundedRuleParse(t, bad, "application-layer.tls_certificate", "TLSHandshakeCertificateCarrier")
		tlsCertificateTestTree(t, n, bad, 0)
		require.Nil(t, protocolCorpusFindNode(n, "Certificates"))
	}
	for _, entry := range []string{"TLSHandshakeCertificate", "TLSHandshakeCertificateCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(good), "application-layer.tls_certificate", entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, "application-layer.tls_certificate", entry)
		require.Error(t, err)
		for _, bits := range []uint64{0, 1, 7, (1<<20)*8 + 1, ((1 << 20) + 1) * 8} {
			reader := &tlsSHTestHeldReader{bits: bits}
			_, err = parser.ParseBinary(reader, "application-layer.tls_certificate", entry)
			require.Error(t, err)
			require.Zero(t, reader.reads)
		}
	}
}

func TestProtocolCorpusTLSCertificateOffsetsRollbackAndIsolation(t *testing.T) {
	for offset := uint64(0); offset < 8; offset++ {
		for _, valid := range []bool{true, false} {
			wire := []byte{11, 0, 0, 7, 0, 0, 4, 0, 0, 1, 0xa5}
			if !valid {
				wire[9] = 2
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
			source := fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/tls_certificate.yaml;node:TLSHandshakeCertificateCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, 8-offset)
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
			tlsCertificateTestTree(t, message, wire, offset)
			if valid {
				protocolCorpusRequireValue(t, message, "Certificate Length", uint64(1))
			} else {
				require.Nil(t, protocolCorpusFindNode(message, "Certificates"))
				require.NotNil(t, protocolCorpusFindNode(message, "Unparsed TLS Certificate Handshake"))
			}
			protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
			require.Equal(t, 123, root.Ctx.GetItem("body_length"))
			require.Equal(t, tlsCertificateTestSHA(packed.Bytes()), tlsCertificateTestSHA(NodeToBytes(n)))
			require.ErrorContains(t, reader.Recovery(), "no backup")
			_, err = reader.ReadBits(8)
			require.ErrorIs(t, err, io.EOF)
		}
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("isolated-%d", worker), func(t *testing.T) {
			t.Parallel()
			wire := []byte{11, 0, 0, 7, 0, 0, 4, 0, 0, 1, byte(worker)}
			cfg := map[string]any{"marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "application-layer.tls_certificate", "TLSHandshakeCertificate", cfg)
			tlsCertificateTestTree(t, n, wire, 0)
			require.Equal(t, map[string]any{"marker": worker}, cfg)
		})
	}
}
