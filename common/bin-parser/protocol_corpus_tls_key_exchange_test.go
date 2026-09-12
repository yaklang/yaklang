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
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

// Caller profile comes from independently observed paired ServerHello cipher,
// never from a heuristic over key-exchange bytes or the capture's label.
func tlsKXCorpusProfile(t *testing.T, cipher uint16, server bool) string {
	t.Helper()
	prefix := "ECDHE"
	switch cipher {
	case 0xc02c, 0xc02f, 0xc030, 0xcca8, 0xcca9:
	case 0x009f:
		prefix = "DHE"
	case 0x009d:
		require.False(t, server)
		prefix = "RSA"
	default:
		t.Fatalf("review unaccounted cipher 0x%04x", cipher)
	}
	if server {
		return prefix + "Server"
	}
	return prefix + "Client"
}

func tlsKXCorpusWhole(t *testing.T, frame, message []byte, start int, entry string) *base.Node {
	t.Helper()
	tail := len(frame) - start - len(message)
	require.GreaterOrEqual(t, tail, 0)
	source := fmt.Sprintf("unit: byte\nPackage:\n  Capture:\n    operator: |\n      this.ProcessSubNode(\"Envelope\")\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      if %d > 0 { this.ProcessSubNode(\"Capture Tail\") }\n    Envelope: raw,%d\n    Message: \"import:application-layer/tls_key_exchange.yaml;node:%s\"\n    Capture Tail: raw,%d\n", len(message), tail, start, entry, tail)
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

func TestProtocolCorpusTLS12KeyExchangeAllOriginalRecords(t *testing.T) {
	// Completion frames independently confirmed by TShark. Earlier physical
	// fragments are retained separately; reassembled offsets are never PCAP offsets.
	serverFrames := map[string][]int{
		"anydesk.pcapng": {16, 73, 128}, "dot.pcap": {6}, "imaps.pcap": {8},
		"wechat.pcap": {22, 102, 128, 153, 200, 240, 269, 290, 368, 391, 502, 523, 544, 560, 571, 652, 856, 900, 1061, 1081, 1142, 1168, 1232, 1270, 1438, 1486, 1523},
	}
	clientFrames := map[string][]int{
		"anydesk.pcapng": {20, 79, 87, 130}, "dot.pcap": {8}, "imaps.pcap": {11},
		"wechat.pcap": {24, 104, 130, 156, 202, 242, 271, 292, 370, 394, 504, 525, 549, 562, 575, 654, 858, 902, 972, 1063, 1083, 1144, 1170, 1234, 1272, 1440, 1488, 1525},
	}
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
	totals := map[string]int{}
	crossPackets := 0
	for _, spec := range specs {
		t.Run(spec.name, func(t *testing.T) {
			path := "testdata/protocol-corpus/captures/ndpi/ndpi-" + spec.name
			file, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, spec.sha, tlsCertificateTestSHA(file))
			records := protocolCorpusAuditPackets(t, path)
			require.Len(t, records, spec.count)
			messages := tlsCertificateTestObserve(t, records, spec.rawIP)
			ciphers := map[string]uint16{}
			for _, m := range messages {
				if m.Complete && m.Wire[0] == 2 && m.TLS12 {
					at := 39 + int(m.Wire[38])
					require.LessOrEqual(t, at+2, len(m.Wire))
					ciphers[m.pair] = binary.BigEndian.Uint16(m.Wire[at : at+2])
				}
			}
			var servers, clients []int
			for _, m := range messages {
				if m.Wire[0] != 12 && m.Wire[0] != 16 {
					continue
				}
				require.True(t, m.Complete, "new truncated key exchange must be retained as a negative")
				require.True(t, m.TLS12)
				cipher, ok := ciphers[m.pair]
				require.True(t, ok)
				server := m.Wire[0] == 12
				profile := tlsKXCorpusProfile(t, cipher, server)
				totals[profile]++
				tlsCertificateTestVerifySources(t, m, records)
				completion := 0
				for _, f := range m.Fragments {
					if !f.Duplicate {
						completion = max(completion, f.Frame)
					}
				}
				if server {
					servers = append(servers, completion)
				} else {
					clients = append(clients, completion)
				}
				t.Run(fmt.Sprintf("frame-%d-%s", completion, profile), func(t *testing.T) {
					entry := "TLS12" + profile + "KeyExchange"
					n := protocolCorpusRequireBoundedRuleParse(t, m.Wire, "application-layer.tls_key_exchange", entry)
					tlsCertificateTestTree(t, n, m.Wire, 0)
					if len(m.Fragments) == 1 {
						f := m.Fragments[0]
						require.Zero(t, f.MessageOffset)
						require.Equal(t, len(m.Wire), f.Length)
						n = tlsKXCorpusWhole(t, records[f.Frame-1], m.Wire, f.FrameOffset, entry)
					} else {
						crossPackets++
					}
					protocolCorpusRequireValue(t, n, "Handshake Type", uint64(m.Wire[0]))
					protocolCorpusRequireValue(t, n, "Handshake Length", uint64(len(m.Wire)-4))
					info := n.Cfg.GetItem("additionInfo").(map[string]any)
					require.Equal(t, "TLS 1.2 "+profile+" layout", info["Profile"])
					for _, key := range []string{"Cipher Suite Correlation Validated", "Public Value Validated", "Point Representation Decoded", "Signature Verified", "Premaster Decrypted", "Handshake Completion Validated"} {
						require.Equal(t, false, info[key])
					}
					point, group, hash, signature, sigLen := uint64(65), uint64(23), uint64(6), uint64(1), uint64(256)
					switch cipher {
					case 0xc02c:
						signature = 3
						sigLen = 71
						if completion == 128 {
							sigLen = 70
						}
					case 0xc030:
						point, group, hash = 97, 24, 4
					case 0xcca8:
						point, group, hash, signature = 32, 29, 8, 4
					case 0xcca9:
						point, group, hash, signature, sigLen = 32, 29, 4, 3, 71
					}
					switch profile {
					case "ECDHEServer":
						protocolCorpusRequireValue(t, n, "Curve Type", uint64(3))
						protocolCorpusRequireValue(t, n, "Named Group", group)
						protocolCorpusRequireValue(t, n, "Server EC Point Length", point)
					case "ECDHEClient":
						protocolCorpusRequireValue(t, n, "Client EC Point Length", point)
					case "DHEServer":
						protocolCorpusRequireValue(t, n, "DH Modulus Length", uint64(256))
						protocolCorpusRequireValue(t, n, "DH Generator Length", uint64(1))
						protocolCorpusRequireValue(t, n, "Server DH Public Value Length", uint64(256))
					case "DHEClient":
						protocolCorpusRequireValue(t, n, "Client DH Public Value Length", uint64(256))
					case "RSAClient":
						protocolCorpusRequireValue(t, n, "Encrypted Premaster Length", uint64(256))
					}
					if server {
						protocolCorpusRequireValue(t, n, "Hash Algorithm", hash)
						protocolCorpusRequireValue(t, n, "Signature Algorithm", signature)
						protocolCorpusRequireValue(t, n, "Signature Length", sigLen)
					}
					t.Logf("profile=%s cipher=%04x completion_frame=%d bytes=%d sha256=%s sources=%+v", profile, cipher, completion, len(m.Wire), tlsCertificateTestSHA(m.Wire), m.Fragments)
				})
			}
			require.Equal(t, serverFrames[spec.name], servers)
			require.Equal(t, clientFrames[spec.name], clients)
		})
	}
	require.Equal(t, map[string]int{"ECDHEServer": 31, "DHEServer": 1, "ECDHEClient": 31, "DHEClient": 1, "RSAClient": 2}, totals)
	require.Equal(t, 16, crossPackets)
	t.Logf("cross-packet key exchanges=%d", crossPackets)
}

func TestProtocolCorpusTLS12KeyExchangeIsolation(t *testing.T) {
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("worker-%d", worker), func(t *testing.T) {
			t.Parallel()
			for profile, wire := range tlsKXCorpusControls() {
				entry := "TLS12" + profile + "KeyExchange"
				cfg := map[string]any{"marker": worker}
				n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "application-layer.tls_key_exchange", entry, cfg)
				tlsCertificateTestTree(t, n, wire, 0)
				info := n.Cfg.GetItem("additionInfo").(map[string]any)
				info["Profile"] = "modified local result"
				next := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.tls_key_exchange", entry)
				require.Equal(t, "TLS 1.2 "+profile+" layout", next.Cfg.GetItem("additionInfo").(map[string]any)["Profile"])
				require.Equal(t, map[string]any{"marker": worker}, cfg)
			}
		})
	}
}

func tlsKXCorpusControls() map[string][]byte {
	return map[string][]byte{
		"ECDHEServer": {12, 0, 0, 9, 3, 0, 23, 1, 4, 6, 3, 0, 0},
		"DHEServer":   {12, 0, 0, 13, 0, 1, 2, 0, 1, 3, 0, 1, 4, 6, 1, 0, 0},
		"ECDHEClient": {16, 0, 0, 2, 1, 4},
		"DHEClient":   {16, 0, 0, 3, 0, 1, 4},
		"RSAClient":   {16, 0, 0, 2, 0, 0},
	}
}

func TestProtocolCorpusTLS12KeyExchangeBoundaries(t *testing.T) {
	for profile, wire := range tlsKXCorpusControls() {
		t.Run(profile, func(t *testing.T) {
			entry := "TLS12" + profile + "KeyExchange"
			n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.tls_key_exchange", entry)
			tlsCertificateTestTree(t, n, wire, 0)
			for cut := 0; cut < len(wire); cut++ {
				_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.tls_key_exchange", entry)
				require.Error(t, err)
			}
			for _, selected := range []string{entry, entry + "Carrier"} {
				_, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.tls_key_exchange", selected)
				require.ErrorContains(t, err, "explicit")
				_, err = parser.GenerateBinary(map[string]any{}, "application-layer.tls_key_exchange", selected)
				require.Error(t, err)
				for _, bits := range []uint64{0, 1, 7, 262154*8 + 1, 262155 * 8} {
					reader := &tlsSHTestHeldReader{bits: bits}
					_, err = parser.ParseBinary(reader, "application-layer.tls_key_exchange", selected)
					require.Error(t, err)
					require.Zero(t, reader.reads)
				}
			}
			for _, bad := range [][]byte{wire[:len(wire)-1], append(bytes.Clone(wire), 0), bytes.Repeat([]byte{0}, 262154)} {
				n := protocolCorpusRequireBoundedRuleParse(t, bad, "application-layer.tls_key_exchange", entry+"Carrier")
				tlsCertificateTestTree(t, n, bad, 0)
				require.Nil(t, protocolCorpusFindNode(n, "Handshake Type"))
				require.NotNil(t, protocolCorpusFindNode(n, "Unparsed TLS Key Exchange"))
			}
		})
	}
}

func TestProtocolCorpusTLS12KeyExchangeOffsetsAndRollback(t *testing.T) {
	for profile, good := range tlsKXCorpusControls() {
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
				source := fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/tls_key_exchange.yaml;node:TLS12%sKeyExchangeCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, profile, 8-offset)
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
