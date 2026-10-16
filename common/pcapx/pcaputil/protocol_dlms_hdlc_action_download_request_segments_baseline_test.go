package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcActionDownloadRequestCases(t *testing.T, name string, n int, sha string) []hdlcOpCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-action-download-request-segments", "DLMS_HDLC_ACTION_DOWNLOAD_REQUEST_SEGMENTS_TEST_ROOT", name)
	require.Equal(t, sha, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, n)
	return d.Cases
}
func TestDLMSHDLCActionDownloadRequestSegmentsExistingAPI(t *testing.T) {
	hdlcOpNative(t, hdlcActionDownloadRequestCases(t, "baseline-controls.json", 3, "4bcdaf99f9a0a9fdd466e339ad14fcc869c548da7ae149eabdf527161d3abb1b"), 41000, 4059)
}
