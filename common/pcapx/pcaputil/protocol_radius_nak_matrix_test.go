package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// Replace this independent fixture count only when adopting the frozen delivery.
const radiusNAKLegacyCount = 55

func radiusNAKControls(t *testing.T) []radiusEAPControl {
	var x struct{ Cases []radiusEAPControl }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-nak", "RADIUS_NAK_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 56)
	var legacy struct {
		Records map[string]map[string]any `json:"records_by_input_sha256"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-nak", "RADIUS_NAK_TEST_ROOT", "legacy-compatibility.json"), &legacy))
	for ci := range x.Cases {
		for si := range x.Cases[ci].Steps {
			w := &x.Cases[ci].Steps[si]
			w.Legacy = legacy.Records[fmt.Sprintf("%x", sha256.Sum256(radiusWire(t, w.Wire)))]
			require.NotNil(t, w.Legacy)
		}
	}
	return x.Cases
}
func TestRADIUSNAKNative(t *testing.T) { radiusNativeCases(t, radiusNAKControls(t)) }
func TestRADIUSNAKMatrix(t *testing.T) {
	radiusCaptureCases(t, radiusNAKControls(t), "radius-nak", "RADIUS_NAK_TEST_ROOT", "RADIUS_NAK_EXPORT")
}
func TestRADIUSNAKLegacy(t *testing.T) {
	radiusLegacyCases(t, radiusNAKControls(t), radiusNAKLegacyCount)
}
