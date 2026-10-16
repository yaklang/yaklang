package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcActionDownloadResponseCases(t *testing.T, name string, n int, sha string) []hdlcOpCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-action-download-response-segments", "DLMS_HDLC_ACTION_DOWNLOAD_RESPONSE_SEGMENTS_TEST_ROOT", name)
	require.Equal(t, sha, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, n)
	return d.Cases
}
func TestDLMSHDLCActionDownloadResponseSegmentsExistingAPI(t *testing.T) {
	hdlcOpNative(t, hdlcActionDownloadResponseCases(t, "baseline-controls.json", 3, "1ba09d52b63911dd979ea00b77d2971793f93a2dad4af72a511a74c620f2acf9"), 41000, 4059)
}
