package bin_parser

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// These are the same independent byte controls consumed by native replay. The
// embedded rule must read request values at their real spans, without the
// BlockInfo/Qualifier prefix still required by response/Set layouts.
func TestDCPIdentifySealedRuleRequestLayout(t *testing.T) {
	b, err := trafficfixture.ReadFile("../pcapx/pcaputil/dcp-identify/controls.json")
	require.NoError(t, err)
	var doc struct {
		Cases []struct {
			Name  string
			Steps []struct {
				Frame string `json:"frame_hex"`
			}
		}
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	for _, c := range doc.Cases {
		if c.Name != "identify-name-filter-odd" && c.Name != "identify-compound-fixed-filters" && c.Name != "identify-all-zero-length-compatibility" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			f, err := hex.DecodeString(c.Steps[0].Frame)
			require.NoError(t, err)
			node := protocolCorpusRequireBoundedRuleParse(t, f[14:], "profinet_dcp", "ProfinetDCP")
			if c.Name == "identify-name-filter-odd" {
				protocolCorpusRequireValue(t, node, "Station Name", []byte("abc"))
			}
			require.Empty(t, protocolCorpusNodesNamed(node, "Block Info"), "Identify request must not have response BlockInfo")
		})
	}
}
