package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestDLMSWrapperSNMultiResultMatrix(t *testing.T) {
	raw := finsReadBatch(t, "dlms-sn-multiresult-source", "DLMS_SN_MULTIRESULT_ROOT", "controls.json")
	require.Equal(t, "6cbd4623ac896cfa2ec23212d3ab39c4af2178c1e4fece7fda2a0b7a8fc0e519", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []snReadControl }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 18)
	var h struct {
		Cases []struct {
			UDP string `json:"sha256"`
			TCP string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &h))
	require.Len(t, h.Cases, 18)
	cases := make([]snWriteControl, len(d.Cases))
	for i, c := range d.Cases {
		cases[i] = snWriteControl{c, h.Cases[i].UDP, h.Cases[i].TCP, "dlms-sn-multiresult-captures"}
	}
	snMultiResultMatrix(t, cases)
}
func TestDLMSWrapperSNMultiResultThreeModesCapturedMatrix(t *testing.T) {
	raw := finsReadBatch(t, "dlms-sn-multiresult-source", "DLMS_SN_MULTIRESULT_ROOT", "three-mode-captured-controls.json")
	require.Equal(t, "8a6e615d81d1a7b6b176577cbdcb7b6a1aa15e20a6a66ddd690bf662f9d287f1", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []snReadControl }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 1)
	var h struct {
		Cases []struct {
			UDP string `json:"sha256"`
			TCP string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &h))
	snMultiResultMatrix(t, []snWriteControl{{d.Cases[0], h.Cases[0].UDP, h.Cases[0].TCP, "dlms-sn-multiresult-captures"}})
}
func snMultiResultMatrix(t *testing.T, cases []snWriteControl) {
	for _, c := range cases {
		for _, tr := range []string{"tcp", "udp"} {
			t.Run(c.Name+"/"+tr, func(t *testing.T) {
				reuseFolder, reuseEnv := c.Folder, "DLMS_SN_MULTIRESULT_ROOT"
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, reuseFolder, reuseEnv, "reuse-inputs.json"), &reuse))
				name := c.Name + "." + tr + ".pcap"
				var raw []byte
				if p, ok := reuse[name]; ok {
					require.True(t, strings.HasPrefix(p, "common/pcapx/pcaputil/"))
					const prefix = "common/pcapx/pcaputil/"
					if strings.HasPrefix(p, prefix+c.Folder+"/") {
						name := strings.TrimPrefix(p, prefix+c.Folder+"/")
						require.NotContains(t, name, "/")
						raw = finsReadBatch(t, c.Folder, reuseEnv, name)
					} else {
						raw = wrapperStructuredInput(t, p)
					}
				} else {
					raw = finsReadBatch(t, c.Folder, reuseEnv, name)
				}
				want := c.UDPHash
				if tr == "tcp" {
					want = c.TCPHash
				}
				require.NotEmpty(t, want)
				require.Equal(t, want, fmt.Sprintf("%x", sha256.Sum256(raw)))
				discoveryMatrix(t, func(t *testing.T, workers int, d, o bool) {
					cfg := hdlcOpCase{}
					cfg.Steps = make([]hdlcOpStep, len(c.Steps))
					cfg.Budget.Collection = c.Budget.Collection
					cfg.Budget.Depth = c.Budget.Depth
					es, stats := selectedReplay(t, cfg, raw, workers, d, o, tr, "dlms-wrapper")
					indices := make([]int, 0, len(c.Steps))
					for i := range c.Steps {
						skip := false
						if tr == "tcp" {
							for _, v := range c.Overrides {
								if v.Step == i+1 {
									skip = true
								}
							}
						}
						if !skip {
							indices = append(indices, i)
						}
					}
					close := c.Close
					if tr == "tcp" {
						close = c.TCPClose
					}
					require.Len(t, es, len(indices)+len(close.Events))
					ids := make([]uint64, len(c.Steps))
					for j, i := range indices {
						ids[i] = es[j].ID
					}
					var flow uint64
					for j, i := range indices {
						e := es[j]
						w := snReadStep(t, c.Steps[i], tr)
						if tr == "udp" {
							w["direction"] = w["capture_udp_direction"]
						}
						expectedFlow := snReadFlow(t, e, w, tr, true, &flow)
						require.NotZero(t, e.ID)
						require.Equal(t, tr, e.Transport)
						require.Equal(t, CaptureDomain{}, e.Domain)
						for j := 0; j < i; j++ {
							if ids[j] != 0 {
								require.NotEqual(t, ids[j], e.ID)
							}
						}
						snReadCheckEvent(t, e, w, ids, d, expectedFlow)
						if w["error"] == nil {
							snWriteTypes(t, e.Session)
						}
						snWriteQueries(t, e, w, ids)
						if kind, ok := w["query_error"].(string); ok {
							snMultiOwnedDiagnosis(t, e, kind, w["error_message"].(string))
						}
						require.Equal(t, CaptureDomain{}, e.Domain)
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						shift := 1
						if tr == "tcp" {
							shift = 4
						}
						require.EqualValues(t, i+shift, e.SourceBytes.PacketRefs[0].Number)
						require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
						wrapperClosePoison(e)
						snWriteQueries(t, e, w, ids)
						if kind, ok := w["query_error"].(string); ok {
							snMultiOwnedDiagnosis(t, e, kind, w["error_message"].(string))
						}
					}
					snWriteCloseIdentity(t, es[len(indices):], ids)
					snReadCheckClose(t, c.snReadControl, es[len(indices):], tr, flow, true)
					require.EqualValues(t, len(indices), stats.Messages)
					require.EqualValues(t, len(close.Events), stats.Incomplete)
				})
			})
		}
	}
}
