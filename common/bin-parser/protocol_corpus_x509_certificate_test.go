package bin_parser

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const x509TestRule = "application-layer.x509_certificate"
const x509TestEntry = "X509CertificateDER"

func x509TestOracle(t *testing.T, n *base.Node, wire []byte, offset uint64) {
	t.Helper()
	tlsCertificateTestTree(t, n, wire, offset)
	cert, err := x509.ParseCertificate(wire)
	require.NoError(t, err)
	info := n.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, cert.Version, info["Certificate Version"])
	require.Equal(t, true, info["Signature Algorithm Encodings Match"])
	require.Equal(t, cert.SerialNumber.String(), info["Serial Number Decimal"])
	require.Equal(t, cert.NotBefore.UTC().Format(time.RFC3339), info["Not Before UTC"])
	require.Equal(t, cert.NotAfter.UTC().Format(time.RFC3339), info["Not After UTC"])
	require.Equal(t, len(cert.Extensions), info["Extension Count"])
	var wantOIDs []string
	for _, ext := range cert.Extensions {
		wantOIDs = append(wantOIDs, ext.Id.String())
	}
	require.Equal(t, wantOIDs, info["Extension OIDs"])
	var extensions []*base.Node
	var visit func(*base.Node)
	visit = func(node *base.Node) {
		if node.Name == "Extension" {
			extensions = append(extensions, node)
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(n)
	require.Len(t, extensions, len(cert.Extensions))
	for i, ext := range extensions {
		envelope := protocolCorpusFindNode(ext, "Extension Value")
		require.Len(t, envelope.Children, 3) // tag, length, exact raw or expanded body
		value := envelope.Children[2]
		require.Equal(t, tlsCertificateTestSHA(cert.Extensions[i].Value), tlsCertificateTestSHA(stream_parser.GetBytesByNode(value)))
		critical := protocolCorpusFindNode(ext, "Critical")
		require.Equal(t, cert.Extensions[i].Critical, critical != nil)
		if critical != nil {
			flag := protocolCorpusFindNode(critical, "Value")
			require.Equal(t, []byte{255}, stream_parser.GetBytesByNode(flag))
		}
	}
	for name, want := range map[string][]byte{
		"Certificate": cert.Raw, "TBSCertificate": cert.RawTBSCertificate,
		"Issuer": cert.RawIssuer, "Subject": cert.RawSubject,
		"Subject Public Key Info": cert.RawSubjectPublicKeyInfo,
	} {
		field := protocolCorpusFindNode(n, name)
		require.NotNil(t, field, name)
		require.Equal(t, tlsCertificateTestSHA(want), tlsCertificateTestSHA(stream_parser.GetBytesByNode(field)), name)
	}
	// The oracle decodes signature and public-key bits separately; the parser
	// exposes the on-wire bit string without asserting algorithm validity.
	sig := protocolCorpusFindNode(protocolCorpusFindNode(n, "Signature Value"), "Bit String Bytes")
	require.Equal(t, tlsCertificateTestSHA(cert.Signature), tlsCertificateTestSHA(stream_parser.GetBytesByNode(sig)))
	var envelope struct {
		TBS       asn1.RawValue
		Algorithm struct {
			OID        asn1.ObjectIdentifier
			Parameters asn1.RawValue `asn1:"optional"`
		}
		Signature asn1.BitString
	}
	rest, err := asn1.Unmarshal(wire, &envelope)
	require.NoError(t, err)
	require.Empty(t, rest)
	require.Equal(t, envelope.Algorithm.OID.String(), info["Signature Algorithm OID"])
	var spki struct {
		Algorithm struct {
			OID        asn1.ObjectIdentifier
			Parameters asn1.RawValue `asn1:"optional"`
		}
		Key asn1.BitString
	}
	rest, err = asn1.Unmarshal(cert.RawSubjectPublicKeyInfo, &spki)
	require.NoError(t, err)
	require.Empty(t, rest)
	require.Equal(t, spki.Algorithm.OID.String(), info["Public Key Algorithm OID"])
	key := protocolCorpusFindNode(protocolCorpusFindNode(n, "Subject Public Key"), "Bit String Bytes")
	require.Equal(t, tlsCertificateTestSHA(spki.Key.Bytes), tlsCertificateTestSHA(stream_parser.GetBytesByNode(key)))
	if info["Profile"] == "X.509 Certificate DER layout" {
		require.Equal(t, false, info["Extension Contents Decoded"])
	}
	for _, key := range []string{"All DER Semantics Validated", "Public Key Validated", "Signature Verified", "Certificate Chain Validated", "Certificate Trust Validated", "Peer Identity Validated", "Current Validity Checked", "TCP Reassembly Performed", "Structured Generation Supported"} {
		require.Equal(t, false, info[key], key)
	}
	_, err = n.Result()
	require.NoError(t, err)
	require.NotNil(t, NodeToMap(n))
}

func TestProtocolCorpusX509CertificateAllOriginalRecords(t *testing.T) {
	totalRecords, complete, partial, inTruncatedHandshake := 0, 0, 0, 0
	extensionCounts, versionCounts, keyCounts := map[string]int{}, map[int]int{}, map[string]int{}
	for _, spec := range tlsCertificateTestSpecs() {
		t.Run(spec.name, func(t *testing.T) {
			path := "testdata/protocol-corpus/captures/ndpi/ndpi-" + spec.name
			file, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, spec.sha, tlsCertificateTestSHA(file))
			records := protocolCorpusAuditPackets(t, path)
			require.Len(t, records, spec.records)
			totalRecords += len(records)
			for _, message := range tlsCertificateTestObserve(t, records, spec.rawIP) {
				if message.Wire[0] != 11 {
					continue
				}
				require.True(t, message.TLS12)
				tlsCertificateTestVerifySources(t, message, records)
				for at := 7; at < len(message.Wire); {
					require.GreaterOrEqual(t, len(message.Wire)-at, 3)
					length := tlsCertificateTestU24(message.Wire[at : at+3])
					at += 3
					if length > len(message.Wire)-at {
						require.False(t, message.Complete)
						partial++
						wire := message.Wire[at:]
						_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), x509TestRule, x509TestEntry)
						require.Error(t, err)
						n := protocolCorpusRequireBoundedRuleParse(t, wire, x509TestRule, x509TestEntry+"Carrier")
						require.Nil(t, protocolCorpusFindNode(n, "TBSCertificate"))
						require.Equal(t, tlsCertificateTestSHA(wire), tlsCertificateTestSHA(NodeToBytes(n)))
						break
					}
					wire := message.Wire[at : at+length]
					n := protocolCorpusRequireBoundedRuleParse(t, wire, x509TestRule, x509TestEntry)
					x509TestOracle(t, n, wire, 0)
					info := n.Cfg.GetItem("additionInfo").(map[string]any)
					versionCounts[info["Certificate Version"].(int)]++
					keyCounts[info["Public Key Algorithm OID"].(string)]++
					for _, oid := range info["Extension OIDs"].([]string) {
						extensionCounts[oid]++
					}
					require.Equal(t, tlsCertificateTestSHA(wire), tlsCertificateTestSHA(NodeToBytes(n)))
					// Embed the DER in its original, possibly reassembled handshake
					// buffer. These are logical offsets, never claimed as packet offsets.
					tail := len(message.Wire) - at - length
					root := latTestInline(t, fmt.Sprintf("unit: byte\nPackage:\n  Handshake:\n    operator: |\n      this.ProcessSubNode(\"Prefix\")\n      this.GetSubNode(\"DER\").SetMaxLength(%d)\n      this.ProcessSubNode(\"DER\")\n      if %d > 0 { this.ProcessSubNode(\"Tail\") }\n    Prefix: raw,%d\n    DER: \"import:application-layer/x509_certificate.yaml;node:X509CertificateDER\"\n    Tail: raw,%d\n", length, tail, at, tail))
					root.Cfg.SetItem(base.CfgLength, uint64(len(message.Wire))*8)
					require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(message.Wire)), "Handshake"))
					whole := base.GetNodeByPath(root, "@Handshake")
					require.Equal(t, tlsCertificateTestSHA(message.Wire), tlsCertificateTestSHA(NodeToBytes(whole)))
					x509TestOracle(t, protocolCorpusFindNode(whole, "DER"), wire, uint64(at)*8)
					if !message.Complete {
						inTruncatedHandshake++
					}
					complete++
					at += length
				}
			}
		})
	}
	t.Logf("records=%d complete_der=%d partial_der=%d complete_der_in_truncated_handshake=%d", totalRecords, complete, partial, inTruncatedHandshake)
	t.Logf("versions=%v public_key_algorithms=%v extension_oid_counts=%v", versionCounts, keyCounts, extensionCounts)
	require.Equal(t, 2080, totalRecords)
	require.Equal(t, 74, complete)
	require.Equal(t, 1, partial)
	require.Equal(t, 1, inTruncatedHandshake)
	require.Equal(t, map[int]int{1: 8, 3: 66}, versionCounts)
	require.Equal(t, map[string]int{"1.2.840.10045.2.1": 3, "1.2.840.113549.1.1.1": 71}, keyCounts)
	require.Equal(t, map[string]int{
		"1.2.840.113533.7.65.0": 1, "1.3.6.1.4.1.11129.2.4.2": 4,
		"1.3.6.1.5.5.7.1.1": 61, "2.5.29.14": 39, "2.5.29.15": 64,
		"2.5.29.16": 1, "2.5.29.17": 58, "2.5.29.19": 66,
		"2.5.29.31": 63, "2.5.29.32": 63, "2.5.29.35": 66, "2.5.29.37": 32,
	}, extensionCounts)
}

func x509TestOriginalCertificate(t *testing.T) []byte {
	t.Helper()
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-anydesk.pcapng"
	file, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, tlsCertificateTestSpecs()[0].sha, tlsCertificateTestSHA(file))
	frame := protocolCorpusAuditPackets(t, path)[84]
	require.Len(t, frame, 867)
	require.Equal(t, []byte{0, 2, 172}, frame[129:132]) // 684-byte DER vector.
	return bytes.Clone(frame[132:816])
}

func TestProtocolCorpusX509CertificateBoundaries(t *testing.T) {
	wire := x509TestOriginalCertificate(t)
	for cut := 0; cut < len(wire); cut++ {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), x509TestRule, x509TestEntry)
		require.Error(t, err, "prefix %d", cut)
	}
	badTag := bytes.Clone(wire)
	badTag[0] = 0
	for _, bad := range [][]byte{badTag, wire[:len(wire)-1], append(bytes.Clone(wire), 0), append(bytes.Clone(wire), wire...)} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(bad), x509TestRule, x509TestEntry)
		require.Error(t, err)
		n := protocolCorpusRequireBoundedRuleParse(t, bad, x509TestRule, x509TestEntry+"Carrier")
		require.Nil(t, protocolCorpusFindNode(n, "TBSCertificate"))
		latTestField(t, n, "Unparsed X509 Certificate DER", "raw", 0, uint64(len(bad))*8, bad)
		require.Equal(t, tlsCertificateTestSHA(bad), tlsCertificateTestSHA(NodeToBytes(n)))
	}
	for _, entry := range []string{x509TestEntry, x509TestEntry + "Carrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(wire), x509TestRule, entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, x509TestRule, entry)
		require.Error(t, err)
		for _, bits := range []uint64{0, 1, 7, 9, (1<<20)*8 + 1, ((1 << 20) + 1) * 8} {
			r := &tlsSHTestHeldReader{bits: bits}
			_, err := parser.ParseBinary(r, x509TestRule, entry)
			require.Error(t, err)
			require.Zero(t, r.reads)
		}
	}
}

func TestProtocolCorpusX509CertificateOffsetsRollbackAndIsolation(t *testing.T) {
	original := x509TestOriginalCertificate(t)
	for offset := uint64(0); offset < 8; offset++ {
		for _, valid := range []bool{true, false} {
			wire := bytes.Clone(original)
			if !valid {
				wire[0] = 0
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
    Message: "import:application-layer/x509_certificate.yaml;node:X509CertificateDERCarrier"
    Sentinel: uint8
    Padding: uint8,%dbit
`, offset, len(wire), offset, offset, 8-offset))
			root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
			root.Ctx.SetItem("caller-x509", "preserved")
			r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
			require.NoError(t, r.Backup())
			require.NoError(t, root.ParseSubNode(r, "Envelope"))
			n := base.GetNodeByPath(root, "@Envelope")
			if valid {
				x509TestOracle(t, protocolCorpusFindNode(n, x509TestEntry), wire, offset)
			} else {
				latTestField(t, n, "Unparsed X509 Certificate DER", "raw", offset, offset+uint64(len(wire))*8, wire)
				require.Nil(t, protocolCorpusFindNode(n, "TBSCertificate"))
			}
			protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
			require.Equal(t, "preserved", root.Ctx.GetItem("caller-x509"))
			require.Equal(t, tlsCertificateTestSHA(packed.Bytes()), tlsCertificateTestSHA(NodeToBytes(n)))
			require.NoError(t, r.Recovery())
			got, err := r.ReadBits(uint64(packed.Len()) * 8)
			require.NoError(t, err)
			require.Equal(t, tlsCertificateTestSHA(packed.Bytes()), tlsCertificateTestSHA(got))
			require.ErrorContains(t, r.PopBackup(), "no backup")
		}
	}
	// The compact AnyDesk fixture is v1 with no extensions. Use a distinct
	// original v3 certificate when testing mutable extension-slice isolation.
	spec := tlsCertificateTestSpecs()[3]
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-" + spec.name
	file, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, spec.sha, tlsCertificateTestSHA(file))
	var withExtensions []byte
	for _, message := range tlsCertificateTestObserve(t, protocolCorpusAuditPackets(t, path), spec.rawIP) {
		if message.Wire[0] != 11 || !message.Complete {
			continue
		}
		length := tlsCertificateTestU24(message.Wire[7:10])
		withExtensions = bytes.Clone(message.Wire[10 : 10+length])
		break
	}
	require.NotEmpty(t, withExtensions)
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			cfg := map[string]any{"caller-x509": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, withExtensions, x509TestRule, x509TestEntry, cfg)
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			info["Certificate Trust Validated"] = true
			require.NotEmpty(t, info["Extension OIDs"])
			info["Extension OIDs"].([]string)[0] = "modified"
			again := protocolCorpusRequireBoundedRuleParse(t, withExtensions, x509TestRule, x509TestEntry)
			x509TestOracle(t, again, withExtensions, 0)
			require.Equal(t, map[string]any{"caller-x509": worker}, cfg)
		})
	}
}
