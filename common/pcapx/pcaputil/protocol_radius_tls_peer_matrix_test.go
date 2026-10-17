package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func radiusTLSPeerControls(t *testing.T) []radiusEAPControl {
	raw := finsReadBatch(t, "radius-tls-peer", "RADIUS_TLS_PEER_TEST_ROOT", "controls.json")
	var x struct{ Cases []radiusEAPControl }
	require.NoError(t, json.Unmarshal(raw, &x))
	require.Len(t, x.Cases, 9)
	profileRaw := finsReadBatch(t, "radius-tls-peer", "RADIUS_TLS_PEER_TEST_ROOT", "profile-refusals.json")
	var profiles struct{ Cases []radiusEAPControl }
	require.NoError(t, json.Unmarshal(profileRaw, &profiles))
	require.Len(t, profiles.Cases, 3)
	x.Cases = append(x.Cases, profiles.Cases...)
	var closeFacts struct {
		Cases []struct {
			Close struct {
				Events int
				Fields map[string]any
			} `json:"close_expected"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &closeFacts))
	extraClose := closeFacts
	extraClose.Cases = nil
	require.NoError(t, json.Unmarshal(profileRaw, &extraClose))
	closeFacts.Cases = append(closeFacts.Cases, extraClose.Cases...)
	var legacy struct {
		Records map[string]map[string]any `json:"records_by_input_sha256"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-tls-peer", "RADIUS_TLS_PEER_TEST_ROOT", "legacy-compatibility.json"), &legacy))
	var extraLegacy struct {
		Records map[string]map[string]any `json:"records_by_input_sha256"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-tls-peer", "RADIUS_TLS_PEER_TEST_ROOT", "profile-legacy.json"), &extraLegacy))
	for h, f := range extraLegacy.Records {
		legacy.Records[h] = f
	}
	for i := range x.Cases {
		c := &x.Cases[i]
		for j := range c.Steps {
			w := &c.Steps[j]
			w.Legacy = legacy.Records[fmt.Sprintf("%x", sha256.Sum256(radiusWire(t, w.Wire)))]
			require.NotNil(t, w.Legacy)
		}
		if closeFacts.Cases[i].Close.Events == 1 {
			c.Close.Fields = closeFacts.Cases[i].Close.Fields
			c.Close.Outstanding = int(c.Close.Fields["Outstanding"].(float64))
		}
	}
	return x.Cases
}
func TestRADIUSTLSPeerNative(t *testing.T) { radiusNativeCases(t, radiusTLSPeerControls(t)) }
func TestRADIUSTLSPeerMatrix(t *testing.T) {
	radiusCaptureCases(t, radiusTLSPeerControls(t), "radius-tls-peer", "RADIUS_TLS_PEER_TEST_ROOT", "RADIUS_TLS_PEER_EXPORT")
}
func TestRADIUSTLSPeerLegacy(t *testing.T) { radiusLegacyCases(t, radiusTLSPeerControls(t), 12) }
func TestRADIUSTLSPeerActiveOwnership(t *testing.T) {
	radiusTLSActiveOwnershipCases(t, radiusTLSPeerControls(t))
}
func TestRADIUSTLSPeerResourceRelease(t *testing.T) {
	radiusTLSResourceReleaseCases(t, radiusTLSPeerControls(t), 192)
}
