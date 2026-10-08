package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestDNSNativeEDNSVariations(t *testing.T) {
	root := "../../bin-parser/testdata/protocol-native/dns-edns"
	raw, err := trafficfixture.ReadFile(filepath.Join(root, "manifest.json"))
	require.NoError(t, err)
	var manifest struct {
		Evidence  string `json:"evidence_kind"`
		Generator string `json:"generator_sha256"`
		Files     []struct{ File, SHA256 string }
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, "independent-native-capture", manifest.Evidence)
	generator, err := trafficfixture.ReadFile("../../../scripts/protocol-tests/generate-dns-edns-native/generate.py")
	require.NoError(t, err)
	require.Equal(t, manifest.Generator, fmt.Sprintf("%x", sha256.Sum256(generator)))
	for _, file := range manifest.Files {
		data, err := trafficfixture.ReadFile(filepath.Join(root, file.File))
		require.NoError(t, err)
		require.Equal(t, file.SHA256, fmt.Sprintf("%x", sha256.Sum256(data)))
	}
	raw, err = trafficfixture.ReadFile(filepath.Join(root, "client-oracle.json"))
	require.NoError(t, err)
	type message struct {
		ID, Flags, RCode              uint16
		EDNS                          int
		EDNSFlags                     uint32 `json:"edns_flags"`
		Question                      []nativeDNSQuestion
		Answer, Authority, Additional []nativeDNSAnswer
	}
	var exchanges []struct {
		Transport       string
		Query, Response message
	}
	require.NoError(t, json.Unmarshal(raw, &exchanges))
	require.Len(t, exchanges, 6)
	expected := map[string]message{}
	for _, x := range exchanges {
		for _, m := range []message{x.Query, x.Response} {
			expected[fmt.Sprintf("%s/%d/%v", x.Transport, m.ID, m.Flags&0x8000 != 0)] = m
		}
	}
	capture, err := trafficfixture.ReadFile(filepath.Join(root, "native.pcapng"))
	require.NoError(t, err)
	for _, workers := range []int{1, 4} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers%d/deferred%v", workers, deferred), func(t *testing.T) {
				events, stats, err := binReplay(t, capture, workers, WithProtocolDecodeAs("udp", 19555, "dns"), WithProtocolDeferred(deferred))
				require.NoError(t, err)
				require.Len(t, events, 12)
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.Malformed)
				seen := map[string]bool{}
				badvers, additional := 0, 0
				for _, e := range events {
					require.Equal(t, "dns", e.Protocol)
					require.Empty(t, e.Error)
					dns := e.Session["DNS"].(map[string]any)
					got := nativeDNSCanonicalize(t, dns)
					key := fmt.Sprintf("%s/%d/%v", e.Transport, got.ID, got.QR)
					want, ok := expected[key]
					require.True(t, ok, key)
					require.False(t, seen[key], key)
					seen[key] = true
					require.Equal(t, want.Flags, dns["Flags"])
					require.Equal(t, want.RCode, got.RCode)
					require.Equal(t, want.Question, got.Question)
					require.Equal(t, want.Answer, got.Answer)
					require.Equal(t, want.Authority, nativeDNSRecords(t, dns["Authority"].([]map[string]any)))
					require.Equal(t, want.Additional, got.Additional)
					require.Equal(t, want.EDNS, got.EDNS)
					require.Equal(t, uint16(want.EDNSFlags), got.EDNSFlags)
					if got.QR {
						require.NotZero(t, e.ResponseTo)
						if got.RCode == 16 {
							badvers++
						}
						if len(got.Additional) > 0 {
							additional++
						}
					}
					if e.Transport == "tcp" {
						require.Equal(t, "dns-tcp", e.Profile)
						require.NotContains(t, e.Summary, "DoT")
					}
					_, err = e.Decode()
					require.NoError(t, err)
				}
				require.Len(t, seen, len(expected))
				require.Equal(t, 2, badvers)
				require.Equal(t, 4, additional)
			})
		}
	}
}
