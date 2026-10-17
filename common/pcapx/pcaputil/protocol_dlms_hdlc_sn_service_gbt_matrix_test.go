package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestDLMSHDLCSNServiceGBTMatrix(t *testing.T) {
	raw := finsReadBatch(t, "dlms-hdlc-sn-service-gbt", "DLMS_HDLC_SN_SERVICE_GBT_ROOT", "captured-controls.json")
	require.Equal(t, "92519d6ca3ab9ad2da9c33ef298bd973e22b8d602a2accc6675283f2d1c64bf3", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 21)
	var captures struct {
		Cases []struct {
			UDP string `json:"sha256"`
			TCP string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &captures))
	require.Len(t, captures.Cases, 21)
	var reuse map[string]string
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-sn-service-gbt-captures", "DLMS_HDLC_SN_SERVICE_GBT_ROOT", "reuse-inputs.json"), &reuse))
	for ci, original := range d.Cases {
		for _, tr := range []string{"udp", "tcp"} {
			t.Run(original.Name+"/"+tr, func(t *testing.T) {
				name := original.Name + "." + tr + ".pcap"
				var input []byte
				if alias, ok := reuse[name]; ok {
					const prefix = "common/pcapx/pcaputil/"
					require.True(t, strings.HasPrefix(alias, prefix))
					if strings.HasPrefix(alias, prefix+"dlms-hdlc-sn-service-gbt-captures/") {
						n := strings.TrimPrefix(alias, prefix+"dlms-hdlc-sn-service-gbt-captures/")
						require.NotContains(t, n, "/")
						input = finsReadBatch(t, "dlms-hdlc-sn-service-gbt-captures", "DLMS_HDLC_SN_SERVICE_GBT_ROOT", n)
					} else {
						input = wrapperStructuredInput(t, alias)
					}
				} else {
					input = finsReadBatch(t, "dlms-hdlc-sn-service-gbt-captures", "DLMS_HDLC_SN_SERVICE_GBT_ROOT", name)
				}
				want := captures.Cases[ci].UDP
				if tr == "tcp" {
					want = captures.Cases[ci].TCP
				}
				require.Equal(t, want, fmt.Sprintf("%x", sha256.Sum256(input)))
				discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
					encoded, err := json.Marshal(original)
					require.NoError(t, err)
					var c hdlcOpCase
					require.NoError(t, json.Unmarshal(encoded, &c))
					es, stats := hdlcOpReplay(t, c, input, workers, deferred, observe, tr)
					require.Len(t, es, len(c.Steps)+len(c.Close.Events))
					require.NotZero(t, es[0].FlowID)
					ids := make([]uint64, len(c.Steps))
					for i := range c.Steps {
						ids[i] = es[i].ID
					}
					for i := range c.Steps {
						c.Steps[i] = hdlcSNGBTBind(c.Steps[i], ids)
					}
					// Queries are checked before any public graph is poisoned by hdlcOpCheck.
					for i, e := range es[:len(c.Steps)] {
						require.Equal(t, es[0].FlowID, e.FlowID)
						require.Equal(t, CaptureDomain{}, e.Domain)
						if c.Steps[i].Error != "" {
							hdlcSNServiceGBTErrorOwnership(t, e, c.Steps[i].Error)
						}
						if c.Steps[i].Error == "" {
							f, err := e.GetFields()
							require.NoError(t, err)
							hdlcSNServiceGBTTypes(t, f)
							hdlcSNServiceGBTTypes(t, e.Session)
							decoded, err := e.Decode()
							require.NoError(t, err)
							for _, key := range []string{"fields", "session"} {
								q, ok := decoded[key].(map[string]any)
								require.True(t, ok)
								hdlcSNServiceGBTTypes(t, q)
							}
						}
					}
					hdlcOpCheck(t, c, es[:len(c.Steps)], tr, true, deferred)
					closed := es[len(c.Steps):]
					for i, e := range closed {
						require.Equal(t, es[0].FlowID, e.FlowID)
						for _, id := range ids {
							require.NotEqual(t, id, e.ID)
						}
						for j := 0; j < i; j++ {
							require.NotEqual(t, closed[j].ID, e.ID)
						}
					}
					hdlcSNServiceGBTCloseCheck(t, c, closed, tr, "ctx-canceled")
					require.EqualValues(t, len(c.Steps), stats.Messages)
					require.EqualValues(t, len(c.Close.Events), stats.Incomplete)
				})
			})
		}
	}
}
