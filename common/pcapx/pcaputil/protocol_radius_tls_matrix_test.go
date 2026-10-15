package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// Replace this independent fixture count only when adopting the frozen delivery.
const radiusTLSLegacyCount = 41

func radiusTLSControls(t *testing.T) []radiusEAPControl {
	var x struct{ Cases []radiusEAPControl }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-tls", "RADIUS_TLS_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 41)
	var legacy struct {
		Records map[string]map[string]any `json:"records_by_input_sha256"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-tls", "RADIUS_TLS_TEST_ROOT", "legacy-compatibility.json"), &legacy))
	for ci := range x.Cases {
		for si := range x.Cases[ci].Steps {
			w := &x.Cases[ci].Steps[si]
			w.Legacy = legacy.Records[fmt.Sprintf("%x", sha256.Sum256(radiusWire(t, w.Wire)))]
			require.NotNil(t, w.Legacy)
		}
	}
	return x.Cases
}
func TestRADIUSTLSNative(t *testing.T) { radiusNativeCases(t, radiusTLSControls(t)) }
func TestRADIUSTLSMatrix(t *testing.T) {
	radiusCaptureCases(t, radiusTLSControls(t), "radius-tls", "RADIUS_TLS_TEST_ROOT", "RADIUS_TLS_EXPORT")
}
func TestRADIUSTLSLegacy(t *testing.T) {
	radiusLegacyCases(t, radiusTLSControls(t), radiusTLSLegacyCount)
}

// Independent JSON answers describe exact public Go types, including nested
// record/handshake slices. Byte strings are hexadecimal in the portable model.
func radiusTLSWant(t *testing.T, m map[string]any) {
	for k, v := range m {
		switch x := v.(type) {
		case float64:
			switch k {
			case "Type":
				if _, ok := m["Body"]; ok {
					m[k] = byte(x)
				} else {
					m[k] = uint16(x)
				}
			case "Flags", "Reserved Bits", "Content Type", "Level", "Description", "Compression Method":
				m[k] = byte(x)
			case "Version", "Cipher Suite":
				m[k] = uint16(x)
			case "Message Length", "Reassembled Length":
				m[k] = uint32(x)
			default:
				m[k] = int(x)
			}
		case string:
			switch k {
			case "Data", "Raw", "Payload", "Body", "Random", "Session ID", "Compression Methods", "Reassembled Data":
				m[k] = radiusWire(t, x)
			}
		case map[string]any:
			radiusTLSWant(t, x)
		case []any:
			if k == "Cipher Suites" {
				a := make([]uint16, len(x))
				for i, v := range x {
					a[i] = uint16(v.(float64))
				}
				m[k] = a
			} else {
				a := make([]map[string]any, len(x))
				for i, v := range x {
					a[i] = v.(map[string]any)
					radiusTLSWant(t, a[i])
				}
				m[k] = a
			}
		}
	}
}
