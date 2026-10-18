package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcActionRequestCases(t *testing.T, name string, n int, sha string) []hdlcOpCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-action-upload-request-segments", "DLMS_HDLC_ACTION_UPLOAD_REQUEST_SEGMENTS_TEST_ROOT", name)
	require.Equal(t, sha, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, n)
	return d.Cases
}
func TestDLMSHDLCActionUploadRequestSegmentsExistingAPI(t *testing.T) {
	hdlcOpNative(t, hdlcActionRequestCases(t, "baseline-controls.json", 3, "e91466488a392414b513656311364b905ed7c158b246949cf14d0eaf843f272d"), 41000, 4059)
}
