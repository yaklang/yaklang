package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestOPCUANativeRenewalAndMultiChunk(t *testing.T) {
	root := "../../bin-parser/testdata/protocol-native/opcua-lifecycle"
	raw, err := trafficfixture.ReadFile(filepath.Join(root, "manifest.json"))
	require.NoError(t, err)
	var manifest struct {
		Evidence   string `json:"evidence_kind"`
		Files      []struct{ File, SHA256 string }
		Generators map[string]string
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, "independent-native-capture", manifest.Evidence)
	for _, file := range manifest.Files {
		b, err := trafficfixture.ReadFile(filepath.Join(root, file.File))
		require.NoError(t, err)
		require.Equal(t, file.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
	}
	for file, hash := range manifest.Generators {
		b, err := trafficfixture.ReadFile(filepath.Join("../../../scripts/protocol-tests/generate-opcua-lifecycle-native", file))
		require.NoError(t, err)
		require.Equal(t, hash, fmt.Sprintf("%x", sha256.Sum256(b)))
	}
	oracle, err := trafficfixture.ReadFile(filepath.Join(root, "endpoint-oracle.json"))
	require.NoError(t, err)
	var endpoint struct {
		Policy  string `json:"security_policy"`
		Oracles []string
	}
	require.NoError(t, json.Unmarshal(oracle, &endpoint))
	require.Equal(t, "None", endpoint.Policy)
	require.Equal(t, []string{"ORACLE round=1 results=2048 all=Running", "ORACLE round=2 results=2048 all=Running"}, endpoint.Oracles)
	rows, err := trafficfixture.ReadFile(filepath.Join(root, "tshark.tsv"))
	require.NoError(t, err)
	type message struct {
		kind, chunk    string
		token, service uint32
	}
	expected := []message{}
	for _, line := range strings.Split(strings.TrimSuffix(string(rows), "\n"), "\n") {
		cells := strings.Split(line, "\t")
		require.Len(t, cells, 8)
		kinds, chunks := strings.Split(cells[1], ","), strings.Split(cells[2], ",")
		require.Len(t, chunks, len(kinds))
		// This independent original capture has one UA chunk per dissector row.
		require.Len(t, kinds, 1)
		m := message{kind: kinds[0], chunk: chunks[0]}
		for col, target := range map[int]*uint32{4: &m.token, 7: &m.service} {
			if cells[col] != "" {
				n, err := strconv.ParseUint(cells[col], 10, 32)
				require.NoError(t, err)
				*target = uint32(n)
			}
		}
		expected = append(expected, m)
	}
	capture, err := trafficfixture.ReadFile(filepath.Join(root, "native.pcap"))
	require.NoError(t, err)
	for _, workers := range []int{1, 4} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers%d/deferred%v", workers, deferred), func(t *testing.T) {
				events, stats, err := binReplay(t, capture, workers, WithProtocolDeferred(deferred))
				require.NoError(t, err)
				require.Len(t, events, len(expected))
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.Malformed)
				opnReq, opnResp, readReq, readResp, reassembled, continuations := 0, 0, 0, 0, 0, 0
				chunkReq, chunkResp := 0, 0
				tokens := map[uint32]bool{}
				for i, e := range events {
					require.Equal(t, "opcua", e.Protocol)
					require.Empty(t, e.Error)
					_, err = e.Decode()
					require.NoError(t, err)
					want := expected[i]
					require.Equal(t, want.kind, e.Session["Message Type"])
					require.Equal(t, want.chunk, e.Session["Chunk Type"])
					if want.token != 0 {
						require.Equal(t, want.token, e.Session["Token ID"])
						tokens[want.token] = true
					}
					if want.service != 0 {
						require.Equal(t, want.service, e.Session["Service Type ID"])
					}
					switch e.Session["Service Type ID"] {
					case uint32(446):
						opnReq++
					case uint32(449):
						opnResp++
						require.Equal(t, true, e.Session["Matched"])
					case uint32(631):
						readReq++
						if e.Session["Reassembled"] == true {
							chunkReq++
						}
					case uint32(634):
						readResp++
						if e.Session["Reassembled"] == true {
							chunkResp++
						}
						require.Equal(t, true, e.Session["Matched"])
					}
					if want.chunk == "C" {
						continuations++
						require.NotContains(t, e.Session, "Service Type ID")
						require.Positive(t, e.Session["Buffered Bytes"])
					}
					if e.Session["Reassembled"] == true {
						reassembled++
					}
				}
				require.GreaterOrEqual(t, opnReq, 3)
				require.Equal(t, opnReq, opnResp)
				require.Equal(t, 3, readReq)
				require.Equal(t, 2, chunkReq)
				require.Equal(t, 3, readResp)
				require.Equal(t, 2, chunkResp)
				require.GreaterOrEqual(t, reassembled, 4)
				require.Greater(t, continuations, 4)
				// Renewal token 2 has no application message in this capture; the
				// independent dissector observes token 1 before and 3 after renewal.
				require.Equal(t, map[uint32]bool{1: true, 3: true}, tokens)
			})
		}
	}
}
