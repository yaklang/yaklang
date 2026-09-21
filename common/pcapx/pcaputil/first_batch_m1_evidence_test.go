package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirstBatchT06(t *testing.T) {
	const base = "testdata/protocol-sessions/first-batch-m1"
	b, err := os.ReadFile(filepath.Join(base, "manifest.json"))
	require.NoError(t, err)
	var doc struct {
		Samples []struct {
			ID             string
			Records        int `json:"record_count"`
			Representation string
			Native         bool `json:"native_carrier"`
			Files          []struct {
				File, SHA256 string
				Bytes        int
			}
		}
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Len(t, doc.Samples, 3)
	for _, sample := range doc.Samples {
		t.Run(sample.ID, func(t *testing.T) {
			require.True(t, sample.Native)
			require.Equal(t, "native_capture", sample.Representation)
			for _, file := range sample.Files {
				raw, err := os.ReadFile(filepath.Join(base, file.File))
				require.NoError(t, err)
				hash := sha256.Sum256(raw)
				require.Equal(t, file.SHA256, hex.EncodeToString(hash[:]))
				require.Len(t, raw, file.Bytes)
				if strings.HasSuffix(file.File, ".pcap") {
					r, err := NewCaptureReader(bytes.NewReader(raw))
					require.NoError(t, err)
					n := 0
					for {
						_, _, err := r.ReadPacketData()
						if err == io.EOF {
							break
						}
						require.NoError(t, err)
						n++
					}
					require.Equal(t, sample.Records, n)
				}
				if strings.HasSuffix(file.File, ".tsv") {
					lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
					require.Len(t, lines, sample.Records)
					found := 0
					for _, line := range lines {
						f := strings.Split(line, "\t")
						switch sample.ID {
						case "tls-h2-bidi":
							require.Len(t, f, 7)
							if f[4] == "5" {
								found++
							}
						case "http-ws-native":
							require.Len(t, f, 5)
							if f[3] == "1" && f[4] == "True" {
								found++
							}
						case "dhcpv6-native":
							require.Len(t, f, 4)
							if f[1] != "" {
								found++
							}
						}
					}
					require.Equal(t, map[string]int{"tls-h2-bidi": 12, "http-ws-native": 4, "dhcpv6-native": 6}[sample.ID], found)
				}
			}
		})
	}
}
