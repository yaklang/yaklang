package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// Existing public session APIs and strict previously published helper only.
// No new runtime function is referenced by the missing-capability witness.
func TestDLMSHDLCSNServiceGBTExistingAPI(t *testing.T) {
	raw := finsReadBatch(t, "dlms-hdlc-sn-service-gbt-source", "DLMS_HDLC_SN_SERVICE_GBT_ROOT", "baseline-controls.json")
	require.Equal(t, "98bab43c3bb949eb41f2767da91432b87b58afe77e08ae8a2a20769f84053261", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 3)
	hdlcSNGBTPublicAPINative(t, d.Cases)
}
