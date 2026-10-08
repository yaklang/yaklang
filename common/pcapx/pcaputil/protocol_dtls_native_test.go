package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"

	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestDTLSNativeCaptureOracle(t *testing.T) {
	root := "../../bin-parser/testdata/protocol-native/dtls"
	raw, err := trafficfixture.ReadFile(filepath.Join(root, "manifest.json"))
	require.NoError(t, err)
	var manifest struct {
		Evidence  string `json:"evidence_kind"`
		Generator string `json:"generator_sha256"`
		Files     []struct{ File, SHA256 string }
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, "independent-native-capture", manifest.Evidence)
	generator, err := trafficfixture.ReadFile("../../../scripts/protocol-tests/generate-dtls-native/generate.py")
	require.NoError(t, err)
	require.Equal(t, manifest.Generator, fmt.Sprintf("%x", sha256.Sum256(generator)))
	cert, err := trafficfixture.ReadFile(filepath.Join(root, "server.pem"))
	require.NoError(t, err)
	certificate, _ := pem.Decode(cert)
	require.NotNil(t, certificate)
	certificateHash := fmt.Sprintf("%x", sha256.Sum256(certificate.Bytes))
	for _, file := range manifest.Files {
		b, err := trafficfixture.ReadFile(filepath.Join(root, file.File))
		require.NoError(t, err)
		require.Equal(t, file.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
		require.NotContains(t, string(b), "PRIVATE KEY")
	}
	for _, name := range []string{"normal", "fragmented", "legacy"} {
		t.Run(name, func(t *testing.T) {
			version, suite := uint16(0xfefd), uint16(0xc02f)
			if name == "legacy" {
				version, suite = 0xfeff, 0x2f
			}
			capture, err := trafficfixture.ReadFile(filepath.Join(root, name+".pcapng"))
			require.NoError(t, err)
			oracle, err := trafficfixture.ReadFile(filepath.Join(root, name+"-tshark.tsv"))
			require.NoError(t, err)
			lines := strings.Split(strings.TrimSuffix(string(oracle), "\n"), "\n")
			for _, workers := range []int{1, 4} {
				for _, deferred := range []bool{false, true} {
					events, stats, err := binReplay(t, capture, workers, WithProtocolDeferred(deferred))
					require.NoError(t, err)
					require.Len(t, events, len(lines))
					require.Zero(t, stats.BufferedBytes)
					require.Zero(t, stats.Malformed)
					protected, hellos, certificates, fragmented := 0, 0, 0, 0
					for i, e := range events {
						fields := strings.Split(lines[i], "\t")
						require.Len(t, fields, 18)
						wire, err := hex.DecodeString(fields[5])
						require.NoError(t, err)
						require.Equal(t, wire, e.Raw)
						require.Equal(t, "dtls", e.Protocol)
						require.Empty(t, e.Error)
						want := "decoded"
						if deferred {
							want = "deferred"
						}
						require.Equal(t, want, e.Status)
						decoded, err := e.Decode()
						require.NoError(t, err)
						semantic := decoded["fields"].(map[string]any)
						require.Equal(t, e.Session, semantic)
						require.Equal(t, false, semantic["Authentication Verified"])
						require.Equal(t, false, semantic["Payload Decrypted"])
						rows := semantic["Records"].([]map[string]any)
						for col, key := range map[int]string{6: "Content Type", 7: "Version", 8: "Epoch", 9: "Sequence", 10: "Length"} {
							expected := strings.Split(fields[col], ",")
							require.Len(t, rows, len(expected))
							for j, text := range expected {
								value, err := strconv.ParseUint(text, 0, 64)
								require.NoError(t, err)
								require.EqualValues(t, value, rows[j][key])
							}
						}
						observedTypes, observedSeqs, observedOffsets, observedLengths := []string{}, []string{}, []string{}, []string{}
						for _, row := range rows {
							if row["Protected"] == true {
								protected++
								require.Equal(t, false, row["Content Decoded"])
								require.NotContains(t, row, "Handshakes")
								continue
							}
							for _, m := range dtlsTestMessages(row) {
								observedTypes = append(observedTypes, fmt.Sprint(m["Type"]))
								observedSeqs = append(observedSeqs, fmt.Sprint(m["Message Seq"]))
								observedOffsets = append(observedOffsets, fmt.Sprint(m["Fragment Offset"]))
								observedLengths = append(observedLengths, fmt.Sprint(m["Fragment Length"]))
								if m["Type"] == byte(1) && m["Complete"] == true {
									hellos++
									require.Equal(t, "dtls.native.test", m["SNI"])
									require.Contains(t, fields[15], m["Random"].(string))
									require.Contains(t, m["Cipher Suites"], suite)
								}
								if m["Type"] == byte(2) {
									require.Equal(t, version, m["Hello Version"])
									require.Equal(t, suite, m["Cipher Suite"])
								}
								if m["Type"] == byte(11) && m["Complete"] == true {
									certificates++
									require.Len(t, m["Certificates"], 1)
									leaf := m["Certificates"].([]map[string]any)[0]
									require.Equal(t, certificateHash, leaf["SHA256"])
									require.Equal(t, []string{"dtls.native.test"}, leaf["DNS Names"])
									require.Equal(t, false, m["Certificate Verified"])
									if len(m["Packet Refs"].([]PacketReference)) > 1 {
										fragmented++
									}
								}
							}
						}
						require.Equal(t, fields[11], strings.Join(observedTypes, ","))
						require.Equal(t, fields[12], strings.Join(observedSeqs, ","))
						require.Equal(t, fields[13], strings.Join(observedOffsets, ","))
						require.Equal(t, fields[14], strings.Join(observedLengths, ","))
					}
					require.Greater(t, protected, 0)
					require.Equal(t, 2, hellos)
					require.Equal(t, 1, certificates)
					if name == "fragmented" {
						require.Equal(t, 1, fragmented)
					}
				}
			}
		})
	}
}
func dtlsTestMessages(row map[string]any) []map[string]any {
	if v, ok := row["Handshakes"].([]map[string]any); ok {
		return v
	}
	return nil
}

// No baseline throughput comparison: the previous revision did not decode
// these native DTLS captures. Each timed replay still checks the complete
// datagram accounting and final release, rather than timing partial success.
func BenchmarkDTLSNativeReplay(b *testing.B) {
	for _, name := range []string{"normal", "fragmented", "legacy"} {
		wire, err := trafficfixture.ReadFile("../../bin-parser/testdata/protocol-native/dtls/" + name + ".pcapng")
		require.NoError(b, err)
		oracle, err := trafficfixture.ReadFile("../../bin-parser/testdata/protocol-native/dtls/" + name + "-tshark.tsv")
		require.NoError(b, err)
		messages := strings.Count(string(oracle), "\n")
		for _, deferred := range []bool{false, true} {
			b.Run(fmt.Sprintf("%s/deferred%v", name, deferred), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(wire)))
				for i := 0; i < b.N; i++ {
					events, stats, err := binReplay(b, wire, 1, WithProtocolDeferred(deferred))
					require.NoError(b, err)
					require.Len(b, events, messages)
					require.Zero(b, stats.BufferedBytes)
					for _, e := range events {
						require.Equal(b, "dtls", e.Protocol)
						require.Empty(b, e.Error)
						_, err = e.Decode()
						require.NoError(b, err)
					}
				}
				b.ReportMetric(float64(messages), "datagrams/batch")
			})
		}
	}
}
