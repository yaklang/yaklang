package bin_parser

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const ldapFieldsTestRule = "application-layer.ldap_fields"
const ldapFieldsTestEntry = "LDAPBindRequestFields"

func ldapFieldsPublicFixtures(t *testing.T) [][]byte {
	return [][]byte{
		mustHex(t, "300c020101600702010304008000"),
		mustHex(t, "302b02017f60180201030404636e3d78a30d04074558414d504c45040200ffa00c300a0403312e320101ff0400"),
		mustHex(t, "3082000f028101016081080201030400808100"),
	}
}

func TestProtocolCorpusLDAPFieldsOriginalRecords(t *testing.T) {
	baseDir := "testdata/protocol-corpus/captures/"
	want := ldapFieldsPublicFixtures(t)[0]
	controls, payloads := 0, 0
	for path, hash := range map[string]string{
		"generated-local/gen-ldap.pcap":         "c17f0738743e3087d78a6c34fb89e231805046afa8ff7f47775ce287ee4e02da",
		"generated-pr5023/pr5023-gen-ldap.pcap": "6475faf95acacc7beb2fb2fe7d6a0885fbd06119c807ccbf012abd0cdf0db8d1",
	} {
		raw, err := os.ReadFile(baseDir + path)
		require.NoError(t, err)
		require.Equal(t, hash, tlsCertificateTestSHA(raw))
		records := protocolCorpusAuditPackets(t, baseDir+path)
		require.Len(t, records, 4)
		for index, record := range records {
			p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
			require.Nil(t, p.ErrorLayer())
			ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
			tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
			require.Equal(t, len(record), 14+int(ip.Length))
			require.Equal(t, uint8(5), ip.IHL)
			require.Equal(t, uint8(5), tcp.DataOffset)
			if index < 3 {
				controls++
				require.Empty(t, tcp.Payload)
				require.Equal(t, index < 2, tcp.SYN)
				require.Equal(t, []uint32{1, 1000, 2}[index], tcp.Seq)
				require.Equal(t, []uint32{0, 2, 1001}[index], tcp.Ack)
				continue
			}
			payloads++
			require.Equal(t, uint32(2), tcp.Seq)
			require.Equal(t, uint32(1001), tcp.Ack)
			require.Equal(t, layers.TCPPort(389), tcp.DstPort)
			require.Equal(t, want, tcp.Payload)
			require.Equal(t, want, record[54:])
			n := smtpFieldsTestWhole(t, record, 54, 14, "application-layer/ldap_fields.yaml", ldapFieldsTestEntry)
			protocolCorpusRequireValue(t, n, "Message ID", uint64(1))
			protocolCorpusRequireValue(t, n, "Version", uint64(3))
			protocolCorpusRequireValue(t, n, "Directory Name", "")
			protocolCorpusRequireValue(t, n, "Simple Octets", []byte{})
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, "simple", info["Choice"])
			require.Equal(t, false, info["Session State Validated"])
			require.Equal(t, [2]int{12, 12}, info["Directory Name"].(map[string]any)["Relative Byte Range"])
			require.Equal(t, [2]int{14, 14}, info["Simple Octets"].(map[string]any)["Relative Byte Range"])
		}
	}
	require.Equal(t, 6, controls)
	require.Equal(t, 2, payloads)
	// Keep both wrong-transport generations negative under the CLDAP entry.
	for path, hash := range map[string]string{
		"generated-local/gen-cldap.pcap":           "d117870d10dd1769e57bdcad54d9aecf003aebd01bcdb6118e1d1e58f21dbacb",
		"generated-pr5023/pr5023-gen-cldap.pcap":   "b209c594a317f3ff829d188b94d172009390ecc9491d71e4eef7969308feeea1",
		"generated-validated/gen-cldap-valid.pcap": "dd6e7f6f8ec0cf3c46311813def511d8950284e2b8ba88167bd4d10deecb1417",
	} {
		raw, err := os.ReadFile(baseDir + path)
		require.NoError(t, err)
		require.Equal(t, hash, tlsCertificateTestSHA(raw))
		records := protocolCorpusAuditPackets(t, baseDir+path)
		require.Len(t, records, 1)
		p := gopacket.NewPacket(records[0], layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, p.ErrorLayer())
		ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
		udp := p.Layer(layers.LayerTypeUDP).(*layers.UDP)
		require.Equal(t, len(records[0]), 14+int(ip.Length))
		require.Equal(t, 8+len(udp.Payload), int(udp.Length))
		require.Equal(t, udp.Payload, records[0][42:])
		_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(udp.Payload), "application-layer.cldap", "CLDAP")
		if path == "generated-validated/gen-cldap-valid.pcap" {
			require.NoError(t, err)
			_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(udp.Payload), ldapFieldsTestRule, ldapFieldsTestEntry)
			require.Error(t, err) // search is not a BindRequest
		} else {
			require.Equal(t, want, udp.Payload)
			require.Error(t, err)
		}
	}
}

func TestProtocolCorpusLDAPFieldsBoundariesAndIsolation(t *testing.T) {
	fixtures := ldapFieldsPublicFixtures(t)
	for _, entry := range []string{ldapFieldsTestEntry, ldapFieldsTestEntry + "Carrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(fixtures[0]), ldapFieldsTestRule, entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, ldapFieldsTestRule, entry)
		require.Error(t, err)
		for _, bits := range []uint64{0, 1, 7, (1<<20)*8 + 1, (1<<20 + 1) * 8} {
			r := &tlsSHTestHeldReader{bits: bits}
			_, err := parser.ParseBinary(r, ldapFieldsTestRule, entry)
			require.Error(t, err)
			require.Zero(t, r.reads)
		}
	}
	for _, wire := range fixtures {
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire[:cut]), ldapFieldsTestRule, ldapFieldsTestEntry)
			require.Error(t, err)
		}
		for offset := uint64(0); offset < 8; offset++ {
			for _, good := range []bool{true, false} {
				w := bytes.Clone(wire)
				if !good {
					if len(w) == 45 {
						w[len(w)-3] = 0
					} else {
						w[len(w)-2] = 0xa0
					}
				}
				var packed bytes.Buffer
				bw := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, bw.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, bw.WriteBits(w, uint64(len(w))*8))
				require.NoError(t, bw.WriteBits([]byte{0xd3}, 8))
				if offset > 0 {
					require.NoError(t, bw.WriteBits([]byte{0}, 8-offset))
				}
				root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/ldap_fields.yaml;node:LDAPBindRequestFieldsCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(w), offset, offset, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("marker", 123)
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Wrapped"))
				n := protocolCorpusFindNode(root, "Message")
				tlsCertificateTestTree(t, n, w, offset)
				if good {
					require.Nil(t, protocolCorpusFindNode(n, "Unparsed LDAP Wire"))
					protocolCorpusRequireValue(t, n, "Version", uint64(3))
				} else {
					protocolCorpusRequireValue(t, n, "Unparsed LDAP Wire", w)
					for _, name := range []string{"Message ID", "BindRequest", "Controls"} {
						require.Nil(t, protocolCorpusFindNode(n, name))
					}
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
		w := append(bytes.Clone(wire), wire...)
		smtpFieldsTestWhole(t, w, 0, len(wire), "application-layer/ldap_fields.yaml", ldapFieldsTestEntry)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), ldapFieldsTestRule, ldapFieldsTestEntry)
		require.Error(t, err)
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprintf("cache-%d", worker), func(t *testing.T) {
			t.Parallel()
			wire := fixtures[1]
			hash := tlsCertificateTestSHA(wire)
			cfg := map[string]any{"marker": worker, "Layout Context": "response"}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, ldapFieldsTestRule, ldapFieldsTestEntry, cfg)
			info := n.Cfg.GetItem("additionInfo").(map[string]any)
			info["Directory Name"].(map[string]any)["Value"].([]byte)[0] = '!'
			info["Controls"].([]map[string]any)[0]["OID"].(map[string]any)["Value"].([]byte)[0] = '!'
			n = protocolCorpusRequireBoundedRuleParse(t, wire, ldapFieldsTestRule, ldapFieldsTestEntry)
			info = n.Cfg.GetItem("additionInfo").(map[string]any)
			require.Equal(t, []byte("cn=x"), info["Directory Name"].(map[string]any)["Value"])
			require.Equal(t, []byte("1.2"), info["Controls"].([]map[string]any)[0]["OID"].(map[string]any)["Value"])
			require.Equal(t, []byte{0, 255}, info["SASL Octets"].(map[string]any)["Value"])
			require.Equal(t, true, info["SASL Octets Present"])
			require.Equal(t, hash, tlsCertificateTestSHA(wire))
			require.Equal(t, map[string]any{"marker": worker, "Layout Context": "response"}, cfg)
		})
	}
}

func TestProtocolCorpusLDAPFieldsIntegerWidths(t *testing.T) {
	for encoded, want := range map[string]uint64{
		"300e0203123456600702010304008000":   0x123456,
		"300f02047fffffff600702010304008000": 0x7fffffff,
		"300d02020080600702010304008000":     128,
	} {
		w := mustHex(t, encoded)
		n := smtpFieldsTestWhole(t, append([]byte{0x78}, w...), 1, len(w), "application-layer/ldap_fields.yaml", ldapFieldsTestEntry)
		protocolCorpusRequireValue(t, n, "Message ID", want)
		require.Equal(t, want, n.Cfg.GetItem("additionInfo").(map[string]any)["Message ID"])
	}
}
