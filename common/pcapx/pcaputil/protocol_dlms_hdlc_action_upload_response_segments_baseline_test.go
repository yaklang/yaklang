package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcActionResponseCases(t *testing.T, name string, n int, sha string) []hdlcOpCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-action-upload-response-segments", "DLMS_HDLC_ACTION_UPLOAD_RESPONSE_SEGMENTS_TEST_ROOT", name)
	require.Equal(t, sha, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, n)
	return d.Cases
}
func TestDLMSHDLCActionUploadResponseSegmentsExistingAPI(t *testing.T) {
	hdlcOpNative(t, hdlcActionResponseCases(t, "baseline-controls.json", 3, "311083dd24e1af9d8ae40b9c976def6e61e9e7e4c8725ad5977649e8ed70bc63"), 41000, 4059)
}
