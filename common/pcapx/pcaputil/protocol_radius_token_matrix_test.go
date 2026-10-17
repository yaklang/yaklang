package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// Replace this independent fixture count only when adopting the frozen delivery.
const radiusTokenLegacyCount = 45

func radiusTokenControls(t *testing.T) []radiusEAPControl {
	var x struct{ Cases []radiusEAPControl }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-token", "RADIUS_TOKEN_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 45)
	var legacy struct {
		Records map[string]map[string]any `json:"records_by_input_sha256"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-token", "RADIUS_TOKEN_TEST_ROOT", "legacy-compatibility.json"), &legacy))
	for ci := range x.Cases {
		for si := range x.Cases[ci].Steps {
			w := &x.Cases[ci].Steps[si]
			w.Legacy = legacy.Records[fmt.Sprintf("%x", sha256.Sum256(radiusWire(t, w.Wire)))]
			require.NotNil(t, w.Legacy)
		}
	}
	return x.Cases
}
func TestRADIUSTokenNative(t *testing.T) { radiusNativeCases(t, radiusTokenControls(t)) }
func TestRADIUSTokenMatrix(t *testing.T) {
	radiusCaptureCases(t, radiusTokenControls(t), "radius-token", "RADIUS_TOKEN_TEST_ROOT", "RADIUS_TOKEN_EXPORT")
}
func TestRADIUSTokenLegacy(t *testing.T) {
	radiusLegacyCases(t, radiusTokenControls(t), radiusTokenLegacyCount)
}
