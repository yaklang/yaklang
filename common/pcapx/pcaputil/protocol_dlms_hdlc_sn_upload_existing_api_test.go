package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// Public session/event APIs with source-authored complete field/session answers.
// The original isolated copy proves the old missing behavior; no new runtime API.
func TestDLMSHDLCSNUploadExistingAPI(t *testing.T) {
	raw := finsReadBatch(t, "dlms-hdlc-sn-upload-source", "DLMS_HDLC_SN_UPLOAD_TEST_ROOT", "baseline-controls.json")
	require.Equal(t, "9801737c465ffc8f867d902db4ee41533b06c9b85607a2ae7c40eb6fa32d5830", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Cases []hdlcOpCase }
	var meta struct {
		Cases []struct{ Steps []map[string]any }
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.NoError(t, json.Unmarshal(raw, &meta))
	require.Len(t, doc.Cases, 3)
	require.Len(t, meta.Cases, 3)
	cases := make([]hdlcSNReadCase, len(doc.Cases))
	for i, c := range doc.Cases {
		require.Len(t, meta.Cases[i].Steps, len(c.Steps))
		cases[i] = hdlcSNReadCase{hdlcOpCase: c, Metadata: meta.Cases[i].Steps}
	}
	// Full fields, Session, typed/private queries, zero SN ownership, raw/time/
	// offsets/endpoints, input/output/query poisoning, Close0 and release through
	// existing-API native helper. Priming GET response still points to q1.
	hdlcSNReadNative(t, cases, 41000, 4059)
}
