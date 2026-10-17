package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func radiusTLSCertificateControls(t *testing.T) []radiusEAPControl {
	raw := finsReadBatch(t, "radius-tls-certificate", "RADIUS_TLS_CERTIFICATE_TEST_ROOT", "controls.json")
	var x struct{ Cases []radiusEAPControl }
	require.NoError(t, json.Unmarshal(raw, &x))
	require.Len(t, x.Cases, 21)
	var legacy struct {
		Records map[string]map[string]any `json:"records_by_input_sha256"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-tls-certificate", "RADIUS_TLS_CERTIFICATE_TEST_ROOT", "legacy.json"), &legacy))
	for i := range x.Cases {
		for j := range x.Cases[i].Steps {
			w := &x.Cases[i].Steps[j]
			w.Legacy = legacy.Records[fmt.Sprintf("%x", sha256.Sum256(radiusWire(t, w.Wire)))]
			require.NotNil(t, w.Legacy)
		}
	}
	return x.Cases
}
func TestRADIUSTLSCertificateNative(t *testing.T) {
	radiusNativeCases(t, radiusTLSCertificateControls(t))
}
func TestRADIUSTLSCertificateMatrix(t *testing.T) {
	radiusCaptureCases(t, radiusTLSCertificateControls(t), "radius-tls-certificate", "RADIUS_TLS_CERTIFICATE_TEST_ROOT", "RADIUS_TLS_CERTIFICATE_EXPORT")
}
func TestRADIUSTLSCertificateLegacy(t *testing.T) {
	radiusLegacyCases(t, radiusTLSCertificateControls(t), 21)
}
func TestRADIUSTLSCertificateActiveOwnership(t *testing.T) {
	radiusTLSActiveOwnershipCases(t, radiusTLSCertificateControls(t))
}
func TestRADIUSTLSCertificateResourceRelease(t *testing.T) {
	radiusTLSResourceReleaseCases(t, radiusTLSCertificateControls(t), 0)
}
