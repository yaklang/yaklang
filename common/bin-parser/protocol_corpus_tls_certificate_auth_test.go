package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func tlsAuthCorpusEntry(kind byte) string {
	if kind == 13 {
		return "TLS12CertificateRequest"
	}
	return "TLS12CertificateVerify"
}

func tlsAuthCorpusWhole(t *testing.T, frame, message []byte, start int, entry string) *base.Node {
	t.Helper()
	tail := len(frame) - start - len(message)
	require.GreaterOrEqual(t, tail, 0)
	source := fmt.Sprintf("unit: byte\nPackage:\n  Capture:\n    operator: |\n      this.ProcessSubNode(\"Envelope\")\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      if %d > 0 { this.ProcessSubNode(\"Capture Tail\") }\n    Envelope: raw,%d\n    Message: \"import:application-layer/tls_certificate_auth.yaml;node:%s\"\n    Capture Tail: raw,%d\n", len(message), tail, start, entry, tail)
	var doc yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
	root, err := base.NewNodeTree(doc)
	require.NoError(t, err)
	root.Cfg.SetItem(base.CfgLength, uint64(len(frame))*8)
	reader := base.NewBitReader(bytes.NewReader(frame))
	require.NoError(t, root.ParseSubNode(reader, "Capture"))
	n := base.GetNodeByPath(root, "@Capture")
	require.Equal(t, tlsCertificateTestSHA(frame), tlsCertificateTestSHA(NodeToBytes(n)))
	messageNode := protocolCorpusFindNode(n, "Message")
	tlsCertificateTestTree(t, messageNode, message, uint64(start)*8)
	require.ErrorContains(t, reader.Recovery(), "no backup")
	return messageNode
}

// RFC 5246 4.7/7.4.4/7.4.8. These are unchanged capture messages, extracted
// from sequence-aware record boundaries; no TLS-label or body-magic filter.
func TestProtocolCorpusTLS12CertificateAuthAllOriginalRecords(t *testing.T) {
	specs := []struct {
		name, sha string
		count     int
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
	var requests, verifies []int
	for _, spec := range specs {
		t.Run(spec.name, func(t *testing.T) {
			path := "testdata/protocol-corpus/captures/ndpi/ndpi-" + spec.name
			file, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, spec.sha, tlsCertificateTestSHA(file))
			records := protocolCorpusAuditPackets(t, path)
			require.Len(t, records, spec.count)
			for _, message := range tlsCertificateTestObserve(t, records, spec.rawIP) {
				if message.Wire[0] != 13 && message.Wire[0] != 15 {
					continue
				}
				require.Equal(t, "anydesk.pcapng", spec.name, "review a newly observed source")
				require.True(t, message.Complete)
				require.True(t, message.TLS12)
				tlsCertificateTestVerifySources(t, message, records)
				// All eight observed messages are physically contiguous; future
				// split messages must retain explicit reassembly provenance.
				require.Len(t, message.Fragments, 1)
				fragment := message.Fragments[0]
				require.Zero(t, fragment.MessageOffset)
				require.Equal(t, len(message.Wire), fragment.Length)
				t.Run(fmt.Sprintf("frame-%d-type-%d", fragment.Frame, message.Wire[0]), func(t *testing.T) {
					entry := tlsAuthCorpusEntry(message.Wire[0])
					direct := protocolCorpusRequireBoundedRuleParse(t, message.Wire, "application-layer.tls_certificate_auth", entry)
					tlsCertificateTestTree(t, direct, message.Wire, 0)
					n := tlsAuthCorpusWhole(t, records[fragment.Frame-1], message.Wire, fragment.FrameOffset, entry)
					protocolCorpusRequireValue(t, n, "Handshake Type", uint64(message.Wire[0]))
					protocolCorpusRequireValue(t, n, "Handshake Length", uint64(len(message.Wire)-4))
					info := n.Cfg.GetItem("additionInfo").(map[string]any)
					require.Equal(t, true, info["Version Is Caller Supplied"])
					for _, key := range []string{"Signature Verified", "Handshake Transcript Validated", "Request Correlation Validated", "Distinguished Names DER Parsed", "Algorithm Usage Validated", "Handshake Completion Validated"} {
						require.Equal(t, false, info[key], key)
					}
					if message.Wire[0] == 15 {
						verifies = append(verifies, fragment.Frame)
						protocolCorpusRequireValue(t, n, "Hash Algorithm", uint64(6))
						protocolCorpusRequireValue(t, n, "Signature Algorithm", uint64(1))
						protocolCorpusRequireValue(t, n, "Signature Length", uint64(256))
						signature := protocolCorpusFindNode(n, "Signature")
						require.Equal(t, tlsCertificateTestSHA(message.Wire[8:]), tlsCertificateTestSHA(stream_parser.GetBytesByNode(signature)))
					} else {
						requests = append(requests, fragment.Frame)
						wantTypes := map[int][]byte{16: {64, 1, 2}, 73: {3, 4, 1, 2, 64}, 85: {1, 2, 64}, 128: {64, 1, 2}}[fragment.Frame]
						require.NotEmpty(t, wantTypes)
						types := protocolCorpusFindNode(n, "Certificate Types")
						require.Len(t, types.Children, len(wantTypes))
						protocolCorpusRequireValue(t, n, "Certificate Types Length", uint64(len(wantTypes)))
						for i, value := range wantTypes {
							protocolCorpusRequireValue(t, types.Children[i], "Certificate Type", uint64(value))
							require.Equal(t, i, types.Children[i].Cfg.GetItem(stream_parser.CfgElementIndex))
						}
						at := 5 + len(wantTypes)
						sigSize := int(binary.BigEndian.Uint16(message.Wire[at : at+2]))
						wantSigSize := map[int]int{16: 22, 73: 30, 85: 30, 128: 22}[fragment.Frame]
						require.Equal(t, wantSigSize, sigSize)
						protocolCorpusRequireValue(t, n, "Signature Algorithms Length", uint64(sigSize))
						algorithms := protocolCorpusFindNode(n, "Signature Algorithms")
						require.Len(t, algorithms.Children, sigSize/2)
						at += 2
						for i, pair := range algorithms.Children {
							protocolCorpusRequireValue(t, pair, "Hash Algorithm", uint64(message.Wire[at]))
							protocolCorpusRequireValue(t, pair, "Signature Algorithm", uint64(message.Wire[at+1]))
							require.Equal(t, i, pair.Cfg.GetItem(stream_parser.CfgElementIndex))
							at += 2
						}
						nameSize, nameCount := 0, 0
						if fragment.Frame == 16 {
							nameSize, nameCount = 76, 1
						}
						protocolCorpusRequireValue(t, n, "Distinguished Names Length", uint64(nameSize))
						names := protocolCorpusFindNode(n, "Distinguished Names")
						require.NotNil(t, names)
						require.Len(t, names.Children, nameCount)
						require.True(t, names.Cfg.GetBool(stream_parser.CfgIsList))
						nameResult, err := names.Result()
						require.NoError(t, err)
						require.True(t, nameResult.IsList())
						require.Len(t, nameResult.Children(), nameCount)
						if nameCount == 0 {
							require.Equal(t, []any{}, NodeToMap(names))
							end := uint64(fragment.FrameOffset+len(message.Wire)) * 8
							require.Equal(t, [2]uint64{end, end}, stream_parser.GetNodeResultPos(names))
						}
						if nameCount == 1 {
							protocolCorpusRequireValue(t, names, "Distinguished Name Length", uint64(74))
						}
						require.Equal(t, len(wantTypes), info["Certificate Type Count"])
						require.Equal(t, sigSize/2, info["Signature Algorithm Count"])
						require.Equal(t, nameCount, info["Distinguished Name Count"])
					}
					t.Logf("frame=%d offset=%d handshake_bytes=%d sha256=%s", fragment.Frame, fragment.FrameOffset, fragment.Length, tlsCertificateTestSHA(message.Wire))
				})
			}
		})
	}
	require.Equal(t, []int{16, 73, 85, 128}, requests)
	require.Equal(t, []int{20, 79, 87, 130}, verifies)
}

func TestProtocolCorpusTLS12CertificateAuthBoundaries(t *testing.T) {
	for _, wire := range [][]byte{
		{13, 0, 0, 8, 1, 1, 0, 2, 4, 1, 0, 0},
		{15, 0, 0, 7, 6, 1, 0, 3, 1, 2, 3},
		{15, 0, 0, 4, 4, 1, 0, 0},
	} {
		entry := tlsAuthCorpusEntry(wire[0])
		n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.tls_certificate_auth", entry)
		tlsCertificateTestTree(t, n, wire, 0)
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.tls_certificate_auth", entry)
			require.Error(t, err)
		}
		for _, selected := range []string{entry, entry + "Carrier"} {
			_, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.tls_certificate_auth", selected)
			require.ErrorContains(t, err, "explicit")
			_, err = parser.GenerateBinary(map[string]any{}, "application-layer.tls_certificate_auth", selected)
			require.Error(t, err)
			for _, bits := range []uint64{0, 1, 7, 131333*8 + 1, 131334 * 8} {
				reader := &tlsSHTestHeldReader{bits: bits}
				_, err = parser.ParseBinary(reader, "application-layer.tls_certificate_auth", selected)
				require.Error(t, err)
				require.Zero(t, reader.reads)
			}
		}
		for _, bad := range [][]byte{wire[:len(wire)-1], append(bytes.Clone(wire), 0)} {
			n := protocolCorpusRequireBoundedRuleParse(t, bad, "application-layer.tls_certificate_auth", entry+"Carrier")
			tlsCertificateTestTree(t, n, bad, 0)
			require.Nil(t, protocolCorpusFindNode(n, "Handshake Type"))
		}
	}
}

func TestProtocolCorpusTLS12CertificateAuthOffsetsAndRollback(t *testing.T) {
	for _, good := range [][]byte{{13, 0, 0, 8, 1, 1, 0, 2, 4, 1, 0, 0}, {15, 0, 0, 5, 6, 1, 0, 1, 0xa5}} {
		for offset := uint64(0); offset < 8; offset++ {
			for _, valid := range []bool{true, false} {
				wire := bytes.Clone(good)
				if !valid {
					wire[3]++
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
				source := fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/tls_certificate_auth.yaml;node:%sCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, tlsAuthCorpusEntry(wire[0]), 8-offset)
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
					protocolCorpusRequireValue(t, message, "Handshake Type", uint64(wire[0]))
				} else {
					require.Nil(t, protocolCorpusFindNode(message, "Handshake Type"))
				}
				protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
				require.Equal(t, 123, root.Ctx.GetItem("body_length"))
				require.Equal(t, tlsCertificateTestSHA(packed.Bytes()), tlsCertificateTestSHA(NodeToBytes(n)))
				require.ErrorContains(t, reader.Recovery(), "no backup")
			}
		}
	}
}
