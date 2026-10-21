package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// Existing public API and existing helpers only: no new production names.
func TestDLMSHDLCSNGBTInformationFullExistingAPI(t *testing.T) {
	raw := finsReadBatch(t, "dlms-hdlc-sn-gbt-information-source", "DLMS_HDLC_SN_GBT_INFORMATION_ROOT", "baseline-controls.json")
	require.Equal(t, "210aee339d958f6461f988d500d0a08c02940f2f5f5d83435911eb871f724068", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 3)
	hdlcSNGBTPublicAPINative(t, d.Cases)
}
