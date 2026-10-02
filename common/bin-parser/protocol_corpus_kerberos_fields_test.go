package bin_parser

import (
	"bytes"
	"encoding/asn1"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const kerberosFieldsTestRule = "application-layer.kerberos_fields"

type kerberosFieldsTestDER struct {
	value               asn1.RawValue
	start, content, end int
}

// Independent standard-library DER envelope/primitive oracle. An OCTET STRING
// is traversed only if it contains exactly one complete constructed DER value.
// This is fixture observation, not production cipher/PA classification.
func kerberosFieldsTestDERValues(t *testing.T, w []byte, start int) map[[2]int]kerberosFieldsTestDER {
	t.Helper()
	values := map[[2]int]kerberosFieldsTestDER{}
	var walk func([]byte, int, int)
	walk = func(wire []byte, at, depth int) {
		require.Less(t, depth, 33)
		for len(wire) > 0 {
			var v asn1.RawValue
			rest, err := asn1.Unmarshal(wire, &v)
			require.NoError(t, err)
			content := at + len(v.FullBytes) - len(v.Bytes)
			values[[2]int{content, at + len(v.FullBytes)}] = kerberosFieldsTestDER{v, at, content, at + len(v.FullBytes)}
			if v.IsCompound {
				walk(v.Bytes, content, depth+1)
			} else if v.Class == 0 && v.Tag == 4 && len(v.Bytes) > 0 {
				var embedded asn1.RawValue
				remain, err := asn1.Unmarshal(v.Bytes, &embedded)
				if err == nil && len(remain) == 0 && embedded.IsCompound {
					walk(v.Bytes, content, depth+1)
				}
			}
			at += len(v.FullBytes)
			wire = rest
		}
	}
	walk(w[start:], start, 0)
	return values
}

func kerberosFieldsTestValues(t *testing.T, n *base.Node, w []byte, tcp bool) {
	t.Helper()
	info := n.Cfg.GetItem("additionInfo").(map[string]any)
	at := 0
	if tcp {
		at = 4
	}
	oracle := kerberosFieldsTestDERValues(t, w, at)
	seen := map[[2]int]bool{}
	for _, decoded := range info["Decoded Fields"].([]map[string]any) {
		span := decoded["Relative Byte Range"].([2]int)
		require.False(t, seen[span])
		seen[span] = true
		v, ok := oracle[span]
		require.True(t, ok, "%v", decoded)
		require.Zero(t, v.value.Class)
		switch decoded["Kind"] {
		case "Integer":
			require.Equal(t, 2, v.value.Tag)
			var want int64
			rest, err := asn1.Unmarshal(v.value.FullBytes, &want)
			require.NoError(t, err)
			require.Empty(t, rest)
			require.Equal(t, want, decoded["Value"])
		case "KerberosString":
			require.Equal(t, 27, v.value.Tag)
			require.Equal(t, string(v.value.Bytes), decoded["Value"])
		case "KerberosTime":
			require.Equal(t, 24, v.value.Tag)
			var want time.Time
			rest, err := asn1.Unmarshal(v.value.FullBytes, &want)
			require.NoError(t, err)
			require.Empty(t, rest)
			require.Equal(t, want.UTC().Format(time.RFC3339), decoded["Value"])
		case "Flags":
			require.Equal(t, 3, v.value.Tag)
			var want asn1.BitString
			rest, err := asn1.Unmarshal(v.value.FullBytes, &want)
			require.NoError(t, err)
			require.Empty(t, rest)
			require.Equal(t, map[string]any{"First 32 Bits": uint64(binary.BigEndian.Uint32(want.Bytes)), "Meaningful Bit Count": want.BitLength}, decoded["Value"])
		default:
			t.Fatalf("unknown decoded kind %v", decoded)
		}
	}
	// No original integer/string/time/flag leaf may remain merely a tag envelope.
	for span, v := range oracle {
		if v.value.Class == 0 && (v.value.Tag == 2 || v.value.Tag == 27 || v.value.Tag == 24 || v.value.Tag == 3) {
			require.True(t, seen[span], "missing decoded original primitive at %v", span)
		}
	}
}

func kerberosFieldsTestWhole(t *testing.T, record []byte, offset int, entry string) *base.Node {
	t.Helper()
	root := latTestInline(t, fmt.Sprintf("endian: little\nunit: byte\nPackage:\n  Capture:\n    operator: |\n      this.ProcessSubNode(\"Envelope\")\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n    Envelope: raw,%d\n    Message: \"import:application-layer/kerberos_fields.yaml;node:%s\"\n", len(record)-offset, offset, entry))
	root.Cfg.SetItem(base.CfgLength, uint64(len(record))*8)
	r := base.NewBitReader(bytes.NewReader(record))
	require.NoError(t, r.Backup())
	require.NoError(t, root.ParseSubNode(r, "Capture"))
	require.Equal(t, record, NodeToBytes(base.GetNodeByPath(root, "@Capture")))
	n := protocolCorpusFindNode(root, "Message")
	tlsCertificateTestTree(t, n, record[offset:], uint64(offset)*8)
	require.NoError(t, r.Recovery())
	got, err := r.ReadBits(uint64(len(record)) * 8)
	require.NoError(t, err)
	require.Equal(t, record, got)
	require.ErrorContains(t, r.PopBackup(), "no backup")
	return n
}

func kerberosFieldsTestNamedValues(info map[string]any, name string) []any {
	var values []any
	for _, field := range info["Decoded Fields"].([]map[string]any) {
		if field["Field"] == name {
			values = append(values, field["Value"])
		}
	}
	return values
}

func TestProtocolCorpusKerberosFieldsAllOriginalRecords(t *testing.T) {
	messageCounts := map[int64]int{}
	paCounts := map[int64]int{}
	seenTCP := map[string][]byte{}
	unique, physical, controls, duplicates, tickets, ciphers := 0, 0, 0, 0, 0, 0
	uniqueTickets, uniqueCiphers := 0, 0
	for _, spec := range []struct {
		name, sha string
		records   int
	}{
		{"error", "a7cf677e50ade6ec40a9fb71fdaef9af259e5dc1ce6ecbac4e981bc106e4c4d7", 2},
		{"login", "bccc9bf683c262c7dacc455c73133980db6233066f8f24305213da85103496c2", 39},
	} {
		path := "testdata/protocol-corpus/captures/ndpi/ndpi-kerberos-" + spec.name + ".pcap"
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, spec.sha, tlsCertificateTestSHA(raw))
		records := protocolCorpusAuditPackets(t, path)
		require.Len(t, records, spec.records)
		for i, record := range records {
			p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
			require.Nil(t, p.ErrorLayer())
			transport := p.TransportLayer()
			require.NotNil(t, transport)
			w := transport.LayerPayload()
			if len(w) == 0 {
				controls++
				continue
			}
			offset := 0
			for _, layer := range p.Layers() {
				offset += len(layer.LayerContents())
				if layer.LayerType() == transport.LayerType() {
					break
				}
			}
			require.Equal(t, w, record[offset:])
			entry, tcp := "KerberosMessageFields", false
			isDuplicate := false
			if tr, ok := transport.(*layers.TCP); ok {
				tcp = true
				entry = "KerberosTCPFields"
				key := fmt.Sprint(p.NetworkLayer().NetworkFlow(), transport.TransportFlow(), tr.Seq)
				if previous, ok := seenTCP[key]; ok {
					require.Equal(t, previous, w)
					duplicates++
					isDuplicate = true
				} else {
					seenTCP[key] = bytes.Clone(w)
				}
				require.Equal(t, len(w)-4, int(binary.BigEndian.Uint32(w)))
			}
			physical++
			if !isDuplicate {
				unique++
			}
			t.Run(fmt.Sprintf("%s/frame-%d", spec.name, i+1), func(t *testing.T) {
				n := protocolCorpusRequireBoundedRuleParse(t, w, kerberosFieldsTestRule, entry)
				tlsCertificateTestTree(t, n, w, 0)
				kerberosFieldsTestValues(t, n, w, tcp)
				full := kerberosFieldsTestWhole(t, record, offset, entry)
				require.Equal(t, n.Cfg.GetItem("additionInfo"), full.Cfg.GetItem("additionInfo"))
				info := n.Cfg.GetItem("additionInfo").(map[string]any)
				messageCounts[info["Message Type"].(int64)]++
				tickets += info["Ticket Count"].(int)
				ciphers += info["Encrypted Part Count"].(int)
				if !isDuplicate {
					uniqueTickets += info["Ticket Count"].(int)
					uniqueCiphers += info["Encrypted Part Count"].(int)
				}
				// Pin field roles, not merely correct DER primitive decoding.
				expect := func(name string, values ...any) {
					require.Equal(t, values, kerberosFieldsTestNamedValues(info, name), name)
				}
				if spec.name == "error" {
					expect("Realm Encoding", "LINUX.SHELL.COM")
					if i == 0 {
						expect("Nonce Encoding", int64(1201797785))
						expect("Till Time Encoding", "2022-02-23T07:46:03Z")
						expect("Name Components Item 0", "host", "krbtgt")
					} else {
						expect("Error Code Encoding", int64(52))
						expect("Server Microseconds Encoding", int64(967277))
						expect("Server Time Encoding", "2022-02-22T07:46:04Z")
					}
				} else if i == 0 {
					expect("Nonce Encoding", int64(197296424))
					expect("Ticket Realm Encoding", "DENYDC.COM")
					expect("Requested Encryption Types Item 1", int64(-133))
					expect("Requested Encryption Types Item 2", int64(-128))
					expect("Requested Encryption Types Item 6", int64(-135))
				} else if tcp {
					expect("Ticket Realm Encoding", "TESTBED1.CA")
					if info["Message Type"] == int64(12) {
						expect("Nonce Encoding", int64(1499342448))
						expect("Checksum Type Encoding", int64(-138))
						expect("Realm Encoding", "TESTBED1.CA")
					} else {
						expect("Client Realm Encoding", "TESTBED1.CA")
						expect("Name Components Item 0", "UBUNTU64A$", "ldap")
					}
				}
				for _, pa := range info["PA-DATA Details"].([]map[string]any) {
					paCounts[pa["Type"].(int64)]++
					require.Equal(t, true, pa["Payload Layout Decoded"])
				}
				for _, key := range []string{"Decryption Performed", "Checksums Verified", "Peer Identity Validated", "Message Exchange Validated", "TCP Reassembly Performed", "Structured Generation Supported"} {
					require.Equal(t, false, info[key])
				}
				// All original packet prefixes, including the TCP record header,
				// remain negative at a strict exact-message boundary.
				for cut := 0; cut < len(w); cut++ {
					_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w[:cut]), kerberosFieldsTestRule, entry)
					require.Error(t, err, "prefix %d", cut)
				}
			})
		}
	}
	require.Equal(t, 30, physical)
	require.Equal(t, 28, unique)
	require.Equal(t, 11, controls)
	require.Equal(t, 2, duplicates)
	require.Equal(t, map[int64]int{10: 1, 12: 14, 13: 14, 30: 1}, messageCounts)
	require.Equal(t, map[int64]int{1: 14, 2: 1, 136: 4, 149: 1}, paCounts)
	require.Equal(t, 28, tickets)
	require.Equal(t, 61, ciphers)
	require.Equal(t, 26, uniqueTickets)
	require.Equal(t, 55, uniqueCiphers)
}

func TestProtocolCorpusKerberosFieldsBoundariesAndIsolation(t *testing.T) {
	records := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-kerberos-error.pcap")
	message := records[1][46:]
	for _, tcp := range []bool{false, true} {
		entry, wire := "KerberosMessageFields", bytes.Clone(message)
		if tcp {
			entry = "KerberosTCPFields"
			wire = make([]byte, 4, len(message)+4)
			binary.BigEndian.PutUint32(wire, uint32(len(message)))
			wire = append(wire, message...)
		}
		for offset := uint64(0); offset < 8; offset++ {
			for _, good := range []bool{true, false} {
				w := bytes.Clone(wire)
				if !good {
					w[0] = 0xff
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
				root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/kerberos_fields.yaml;node:%sCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(w), offset, offset, entry, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("marker", 123)
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Wrapped"))
				m := protocolCorpusFindNode(root, "Message")
				tlsCertificateTestTree(t, m, w, offset)
				if good {
					require.NotNil(t, protocolCorpusFindNode(m, "Error Code"))
				} else {
					protocolCorpusRequireValue(t, m, "Unparsed Kerberos Wire", w)
					require.Nil(t, protocolCorpusFindNode(m, "Error Code"))
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
		// A second complete message is not part of an exact single-message
		// boundary. Bounded callers retain it unchanged for the next parse.
		both := append(bytes.Clone(wire), wire...)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(both), kerberosFieldsTestRule, entry)
		require.Error(t, err)
		root := latTestInline(t, fmt.Sprintf("Package:\n  Wrapped:\n    operator: |\n      this.GetSubNode(\"First\").SetMaxLength(%d)\n      this.ProcessSubNode(\"First\")\n      this.GetSubNode(\"Second\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Second\")\n    First: \"import:application-layer/kerberos_fields.yaml;node:%s\"\n    Second: \"import:application-layer/kerberos_fields.yaml;node:%s\"\n", len(wire), len(wire), entry, entry))
		root.Cfg.SetItem(base.CfgLength, uint64(len(both))*8)
		require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(both)), "Wrapped"))
		tlsCertificateTestTree(t, protocolCorpusFindNode(root, "Second"), wire, uint64(len(wire))*8)
		require.Equal(t, both, NodeToBytes(base.GetNodeByPath(root, "@Wrapped")))
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("isolated-%d", worker), func(t *testing.T) {
			t.Parallel()
			cfg := map[string]any{"marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, message, kerberosFieldsTestRule, "KerberosMessageFields", cfg)
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, []any{int64(52)}, kerberosFieldsTestNamedValues(info, "Error Code Encoding"))
			info["Decoded Fields"].([]map[string]any)[0]["Value"] = int64(worker)
			next := protocolCorpusRequireBoundedRuleParse(t, message, kerberosFieldsTestRule, "KerberosMessageFields")
			require.Equal(t, []any{int64(5)}, kerberosFieldsTestNamedValues(next.Cfg.GetItem("additionInfo").(map[string]any), "Protocol Version Encoding"))
			require.Equal(t, map[string]any{"marker": worker}, cfg)
		})
	}
}

func TestProtocolCorpusKerberosFieldsLimits(t *testing.T) {
	for _, entry := range []string{"KerberosMessageFields", "KerberosTCPFields", "KerberosMessageFieldsCarrier", "KerberosTCPFieldsCarrier"} {
		for _, bits := range []uint64{0, 1, 7, (1<<20)*8 + 1, ((1 << 20) + 1) * 8} {
			r := &tlsSHTestHeldReader{bits: bits}
			_, err := parser.ParseBinary(r, kerberosFieldsTestRule, entry)
			require.Error(t, err)
			require.Zero(t, r.reads)
		}
		_, err := parser.ParseBinary(bytes.NewReader([]byte{0x7e, 0}), kerberosFieldsTestRule, entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, kerberosFieldsTestRule, entry)
		require.Error(t, err)
	}
}
