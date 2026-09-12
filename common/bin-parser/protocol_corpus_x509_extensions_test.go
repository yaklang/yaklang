package bin_parser

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const x509ExtensionsTestEntry = "X509CertificateDERWithExtensions"

func x509ExtensionsTestOracle(t *testing.T, n *base.Node, wire []byte) int {
	t.Helper()
	x509TestOracle(t, n, wire, 0)
	cert, err := x509.ParseCertificate(wire)
	require.NoError(t, err)
	info := n.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, len(cert.Extensions), info["Decoded Extension Count"])
	require.Equal(t, 0, info["Opaque Extension Count"])
	require.Equal(t, len(cert.Extensions) > 0, info["Extension Contents Decoded"])
	require.Equal(t, false, info["Extension Semantics Validated"])
	details := info["Extension Details"].([]map[string]any)
	require.Len(t, details, len(cert.Extensions))
	scts := 0
	for i, ext := range cert.Extensions {
		d := details[i]
		require.Equal(t, ext.Id.String(), d["OID"])
		require.Equal(t, true, d["Decoded"])
		switch ext.Id.String() {
		case "2.5.29.16":
			var period asn1.RawValue
			rest, e := asn1.Unmarshal(ext.Value, &period)
			require.NoError(t, e)
			require.Empty(t, rest)
			// Context-tagged GeneralizedTime needs its explicit 4-digit-year
			// layout; Go's implicit time.Time path chooses UTCTime instead.
			for remaining := period.Bytes; len(remaining) > 0; {
				var field asn1.RawValue
				remaining, e = asn1.Unmarshal(remaining, &field)
				require.NoError(t, e)
				require.Equal(t, 2, field.Class)
				require.Contains(t, []int{0, 1}, field.Tag)
				date, e := time.Parse("20060102150405Z", string(field.Bytes))
				require.NoError(t, e)
				name := map[int]string{0: "Not Before UTC", 1: "Not After UTC"}[field.Tag]
				require.Equal(t, date.UTC().Format(time.RFC3339), d[name])
			}
		case "1.2.840.113533.7.65.0":
			var outer asn1.RawValue
			rest, e := asn1.Unmarshal(ext.Value, &outer)
			require.NoError(t, e)
			require.Empty(t, rest)
			var version asn1.RawValue
			rest, e = asn1.Unmarshal(outer.Bytes, &version)
			require.NoError(t, e)
			require.Equal(t, 27, version.Tag)
			require.Equal(t, version.Bytes, d["Version Bytes"])
			require.Equal(t, len(rest) > 0, d["Flags Present"])
			if len(rest) > 0 {
				var flags asn1.BitString
				rest, e = asn1.Unmarshal(rest, &flags)
				require.NoError(t, e)
				require.Empty(t, rest)
				var set []int
				for i := 0; i < flags.BitLength; i++ {
					if flags.At(i) == 1 {
						set = append(set, i)
					}
				}
				require.Equal(t, set, d["Set Flag Bits"])
			}
		case "2.5.29.14":
			require.Equal(t, cert.SubjectKeyId, d["Key Identifier"])
		case "2.5.29.35":
			if len(cert.AuthorityKeyId) > 0 {
				require.Equal(t, cert.AuthorityKeyId, d["Key Identifier"])
			}
		case "2.5.29.15":
			mask := 0
			for _, bit := range d["Set Bits"].([]int) {
				mask |= 1 << uint(bit)
			}
			require.Equal(t, int(cert.KeyUsage), mask)
		case "2.5.29.19":
			require.Equal(t, cert.IsCA, d["CA"])
			present := cert.MaxPathLen > 0 || cert.MaxPathLenZero
			require.Equal(t, present, d["Path Length Present"])
			if present {
				require.Equal(t, strconv.Itoa(cert.MaxPathLen), d["Path Length Decimal"])
			}
		case "2.5.29.17":
			var dns, email, ips, uris []string
			for _, name := range d["Names"].([]map[string]any) {
				switch name["Tag"].(byte) {
				case 0x82:
					dns = append(dns, name["Value"].(string))
				case 0x81:
					email = append(email, name["Value"].(string))
				case 0x87:
					ips = append(ips, string(name["Value"].([]byte)))
				case 0x86:
					uris = append(uris, name["Value"].(string))
				}
			}
			require.Equal(t, cert.DNSNames, dns)
			require.Equal(t, cert.EmailAddresses, email)
			var wantIPs, wantURIs []string
			for _, ip := range cert.IPAddresses {
				wantIPs = append(wantIPs, string(ip))
			}
			for _, uri := range cert.URIs {
				wantURIs = append(wantURIs, uri.String())
			}
			require.Equal(t, wantIPs, ips)
			require.Equal(t, wantURIs, uris)
		case "2.5.29.37":
			var oids []asn1.ObjectIdentifier
			rest, e := asn1.Unmarshal(ext.Value, &oids)
			require.NoError(t, e)
			require.Empty(t, rest)
			var want []string
			for _, oid := range oids {
				want = append(want, oid.String())
			}
			require.Equal(t, want, d["Key Purpose OIDs"])
		case "2.5.29.32":
			var want []string
			for _, oid := range cert.PolicyIdentifiers {
				want = append(want, oid.String())
			}
			require.Equal(t, want, d["Policy OIDs"])
		case "1.3.6.1.5.5.7.1.1":
			var ocsp, issuer []string
			for _, ad := range d["Access Descriptions"].([]map[string]any) {
				loc := ad["Location"].(map[string]any)
				if loc["Tag"] != byte(0x86) {
					continue
				}
				switch ad["Method OID"] {
				case "1.3.6.1.5.5.7.48.1":
					ocsp = append(ocsp, loc["Value"].(string))
				case "1.3.6.1.5.5.7.48.2":
					issuer = append(issuer, loc["Value"].(string))
				}
			}
			require.Equal(t, cert.OCSPServer, ocsp)
			require.Equal(t, cert.IssuingCertificateURL, issuer)
		case "2.5.29.31":
			var uris []string
			for _, dp := range d["Distribution Points"].([]map[string]any) {
				names, _ := dp["Names"].([]map[string]any)
				for _, name := range names {
					if name["Tag"] == byte(0x86) {
						uris = append(uris, name["Value"].(string))
					}
				}
			}
			require.Equal(t, cert.CRLDistributionPoints, uris)
		case "1.3.6.1.4.1.11129.2.4.2":
			var vector []byte
			rest, e := asn1.Unmarshal(ext.Value, &vector)
			require.NoError(t, e)
			require.Empty(t, rest)
			require.Equal(t, len(vector)-2, int(binary.BigEndian.Uint16(vector)))
			list := protocolCorpusFindNode(n, "SCT Entries")
			require.NotNil(t, list)
			count := 0
			for at := 2; at < len(vector); {
				length := int(binary.BigEndian.Uint16(vector[at:]))
				body := vector[at+2 : at+2+length]
				require.Zero(t, body[0])
				item := list.Children[count]
				require.Equal(t, body[1:33], stream_parser.GetBytesByNode(protocolCorpusFindNode(item, "SCT Log ID")))
				protocolCorpusRequireValue(t, item, "SCT Timestamp Milliseconds", binary.BigEndian.Uint64(body[33:41]))
				extLen := int(binary.BigEndian.Uint16(body[41:43]))
				pos := 43 + extLen
				require.Equal(t, body[43:pos], stream_parser.GetBytesByNode(protocolCorpusFindNode(item, "SCT Extensions")))
				protocolCorpusRequireValue(t, item, "SCT Hash Algorithm", uint64(body[pos]))
				protocolCorpusRequireValue(t, item, "SCT Signature Algorithm", uint64(body[pos+1]))
				sigLen := int(binary.BigEndian.Uint16(body[pos+2 : pos+4]))
				require.Equal(t, len(body)-pos-4, sigLen)
				require.Equal(t, body[pos+4:], stream_parser.GetBytesByNode(protocolCorpusFindNode(item, "SCT Signature")))
				count++
				at += 2 + length
			}
			require.Len(t, list.Children, count)
			require.Equal(t, count, d["SCT V1 Count"])
			require.Equal(t, 0, d["Opaque SCT Count"])
			scts += count
		}
	}
	return scts
}

func TestProtocolCorpusX509ExtensionsOffsetsAndRollback(t *testing.T) {
	x509TestExpandedOffsetsAndRollback(t, x509ExtensionsTestEntry)
}

func x509TestExpandedOffsetsAndRollback(t *testing.T, entry string) {
	spec := tlsCertificateTestSpecs()[3]
	path := "testdata/protocol-corpus/captures/ndpi/ndpi-" + spec.name
	file, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, spec.sha, tlsCertificateTestSHA(file))
	var original []byte
	for _, message := range tlsCertificateTestObserve(t, protocolCorpusAuditPackets(t, path), spec.rawIP) {
		if message.Wire[0] == 11 && message.Complete {
			length := tlsCertificateTestU24(message.Wire[7:10])
			original = bytes.Clone(message.Wire[10 : 10+length])
			break
		}
	}
	require.NotEmpty(t, original)
	cert, err := x509.ParseCertificate(original)
	require.NoError(t, err)
	require.NotEmpty(t, cert.Extensions)
	body := cert.Extensions[0].Value
	require.Equal(t, 1, bytes.Count(original, body))
	position := bytes.Index(original, body)
	for offset := uint64(0); offset < 8; offset++ {
		for _, valid := range []bool{true, false} {
			wire := bytes.Clone(original)
			if !valid {
				wire[position] = 5
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
    Message: "import:application-layer/x509_certificate.yaml;node:%sCarrier"
    Sentinel: uint8
    Padding: uint8,%dbit
`, offset, len(wire), offset, offset, entry, 8-offset))
			root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
			r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
			require.NoError(t, r.Backup())
			require.NoError(t, root.ParseSubNode(r, "Envelope"))
			n := base.GetNodeByPath(root, "@Envelope")
			if valid {
				x509TestOracle(t, protocolCorpusFindNode(n, entry), wire, offset)
			} else {
				rawName := "Unparsed X509 Certificate With Extensions"
				if entry == x509PublicKeyTestEntry {
					rawName = "Unparsed X509 Certificate With Public Key"
				}
				latTestField(t, n, rawName, "raw", offset, offset+uint64(len(wire))*8, wire)
				require.Nil(t, protocolCorpusFindNode(n, "TBSCertificate"))
				// The unchanged base profile still accepts an opaque extension body.
				protocolCorpusRequireBoundedRuleParse(t, wire, x509TestRule, x509TestEntry)
			}
			protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
			require.Equal(t, packed.Bytes(), NodeToBytes(n))
			require.NoError(t, r.Recovery())
			got, err := r.ReadBits(uint64(packed.Len()) * 8)
			require.NoError(t, err)
			require.Equal(t, packed.Bytes(), got)
			require.ErrorContains(t, r.PopBackup(), "no backup")
		}
	}
	for _, entry := range []string{entry, entry + "Carrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(original), x509TestRule, entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, x509TestRule, entry)
		require.Error(t, err)
		for _, bits := range []uint64{0, 1, 7, 9, ((1 << 20) + 1) * 8} {
			r := &tlsSHTestHeldReader{bits: bits}
			_, err := parser.ParseBinary(r, x509TestRule, entry)
			require.Error(t, err)
			require.Zero(t, r.reads)
		}
	}
}

func TestProtocolCorpusX509ExtensionsAllOriginalRecords(t *testing.T) {
	x509TestExpandedAllOriginalRecords(t, x509ExtensionsTestEntry)
}

func x509TestExpandedAllOriginalRecords(t *testing.T, entry string) {
	recordsTotal, complete, partial, extensions, scts := 0, 0, 0, 0, 0
	oids := map[string]int{}
	keys := map[string]int{}
	signatures := map[string]int{}
	frameLogged := false
	for _, spec := range tlsCertificateTestSpecs() {
		t.Run(spec.name, func(t *testing.T) {
			path := "testdata/protocol-corpus/captures/ndpi/ndpi-" + spec.name
			file, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, spec.sha, tlsCertificateTestSHA(file))
			records := protocolCorpusAuditPackets(t, path)
			require.Len(t, records, spec.records)
			recordsTotal += len(records)
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
						_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), x509TestRule, entry)
						require.Error(t, err)
						n := protocolCorpusRequireBoundedRuleParse(t, wire, x509TestRule, entry+"Carrier")
						require.Nil(t, protocolCorpusFindNode(n, "TBSCertificate"))
						require.Equal(t, wire, NodeToBytes(n))
						break
					}
					wire := message.Wire[at : at+length]
					n := protocolCorpusRequireBoundedRuleParse(t, wire, x509TestRule, entry)
					scts += x509ExtensionsTestOracle(t, n, wire)
					if entry == x509PublicKeyTestEntry {
						keys[x509PublicKeyTestOracle(t, n, wire)]++
					}
					info := n.Cfg.GetItem("additionInfo").(map[string]any)
					signatures[info["Signature Algorithm OID"].(string)]++
					if !frameLogged && info["Extension Count"].(int) > 0 {
						for frame, record := range records {
							if at := bytes.Index(record, wire); at >= 0 {
								t.Logf("contiguous extended certificate: capture=%s frame=%d offset=%d length=%d tail=%d extensions=%v", spec.name, frame+1, at, len(wire), len(record)-at-len(wire), info["Extension Count"])
								frameLogged = true
								break
							}
						}
					}
					extensions += info["Extension Count"].(int)
					for _, oid := range info["Extension OIDs"].([]string) {
						oids[oid]++
					}
					complete++
					at += length
				}
			}
		})
	}
	require.Equal(t, 2080, recordsTotal)
	require.Equal(t, 74, complete)
	require.Equal(t, 1, partial)
	require.Equal(t, 518, extensions)
	require.Len(t, oids, 12)
	require.Equal(t, 10, scts)
	if entry == x509PublicKeyTestEntry {
		t.Logf("public key layouts=%v", keys)
		t.Logf("certificate signature algorithm OIDs=%v", signatures)
		require.Equal(t, map[string]int{"RSA": 71, "EC/P-256": 3}, keys)
		require.Equal(t, map[string]int{"1.2.840.113549.1.1.11": 69, "1.2.840.113549.1.1.5": 5}, signatures)
	}
	t.Logf("records=%d certificates=%d partial=%d extensions=%d OIDs=%d SCTs=%d", recordsTotal, complete, partial, extensions, len(oids), scts)
}
