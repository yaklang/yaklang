package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// The padding-only UDP control has no byte-stream carrier; the other eleven do.
const radiusTLSChainLegacyCount = 11

func radiusTLSChainControls(t *testing.T) []radiusEAPControl {
	var x struct{ Cases []radiusEAPControl }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-tls-chain", "RADIUS_TLS_CHAIN_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 8)
	var supplement struct{ Cases []radiusEAPControl }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-tls-chain", "RADIUS_TLS_CHAIN_TEST_ROOT", "supplement.json"), &supplement))
	require.Len(t, supplement.Cases, 4)
	x.Cases = append(x.Cases, supplement.Cases...)
	var legacy struct {
		Records map[string]map[string]any `json:"records_by_input_sha256"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-tls-chain", "RADIUS_TLS_CHAIN_TEST_ROOT", "legacy-compatibility.json"), &legacy))
	var extraLegacy struct {
		Records map[string]map[string]any `json:"records_by_input_sha256"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-tls-chain", "RADIUS_TLS_CHAIN_TEST_ROOT", "supplement-legacy.json"), &extraLegacy))
	for sha, f := range extraLegacy.Records {
		legacy.Records[sha] = f
	}
	for ci := range x.Cases {
		for si := range x.Cases[ci].Steps {
			w := &x.Cases[ci].Steps[si]
			w.Legacy = legacy.Records[fmt.Sprintf("%x", sha256.Sum256(radiusWire(t, w.Wire)))]
			if x.Cases[ci].Legacy {
				require.NotNil(t, w.Legacy)
			}
		}
	}
	for i := range x.Cases {
		c := &x.Cases[i]
		if c.Name == "partial-observed-round-Close" {
			c.Close.Profile = "radius-eap-observed"
			c.Close.Outstanding = 1
			c.Close.Fields = map[string]any{"Outstanding": 1, "TLS Fragment Groups": 1, "TLS Fragment Buffered Bytes": 13}
		}
	}
	return x.Cases
}
func TestRADIUSTLSChainNative(t *testing.T) { radiusNativeCases(t, radiusTLSChainControls(t)) }
func TestRADIUSTLSChainMatrix(t *testing.T) {
	radiusCaptureCases(t, radiusTLSChainControls(t), "radius-tls-chain", "RADIUS_TLS_CHAIN_TEST_ROOT", "RADIUS_TLS_CHAIN_EXPORT")
}
func TestRADIUSTLSChainLegacy(t *testing.T) {
	radiusLegacyCases(t, radiusTLSChainControls(t), radiusTLSChainLegacyCount)
}
