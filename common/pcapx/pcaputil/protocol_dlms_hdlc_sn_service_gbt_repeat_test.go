package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// Independent full-frame exact-repeat policy; public API only, normal neighbors
// remain in ExistingAPI. A repeated selected E0 must not erase its SN prefix.
func TestDLMSHDLCSNServiceGBTExactSelectedRepeatNative(t *testing.T) {
	raw := finsReadBatch(t, "dlms-hdlc-sn-service-gbt-source", "DLMS_HDLC_SN_SERVICE_GBT_ROOT", "exact-selected-carrier-repeat-supplement.json")
	require.Equal(t, "c008792858c11d514999cfe0b1de1027e0056fa0f8e2d6dde483a648be975a96", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 1)
	hdlcSNGBTPublicAPINativeWithChecks(t, d.Cases, hdlcSNServiceGBTTypes, hdlcSNServiceGBTErrorOwnership, hdlcSNServiceGBTCloseCheck)
}

func TestDLMSHDLCSNServiceGBTRepeatGateNegativeNative(t *testing.T) {
	raw := finsReadBatch(t, "dlms-hdlc-sn-service-gbt-source", "DLMS_HDLC_SN_SERVICE_GBT_ROOT", "repeat-gate-negative-supplement.json")
	require.Equal(t, "4f1dac44050b0ddfc2b31c35c0d6a0529a6d546d7c5091262654a930a2bd3aa1", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 2)
	hdlcSNGBTPublicAPINativeWithChecks(t, d.Cases, hdlcSNServiceGBTTypes, hdlcSNServiceGBTErrorOwnership, hdlcSNServiceGBTCloseCheck)
}
