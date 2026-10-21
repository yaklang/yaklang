package pcaputil

import (
	"testing"
)

func hdlcActionResponseRoundtripControls(t *testing.T) []hdlcOpCase {
	return hdlcActionDownloadResponseCases(t, "roundtrip-supplement.json", 1, "cb3a9be98c72f80a0866a30b6505242daf142f9d9322006ebd879913b60a1cfa")
}
func TestDLMSHDLCActionResponseRoundtripNative(t *testing.T) {
	hdlcOpNative(t, hdlcActionResponseRoundtripControls(t), 41000, 4059)
}
func TestDLMSHDLCActionResponseRoundtripMatrix(t *testing.T) {
	hdlcListSegmentsMatrixBatch(t, hdlcActionResponseRoundtripControls(t), "dlms-hdlc-action-download-response-segments", "DLMS_HDLC_ACTION_DOWNLOAD_RESPONSE_SEGMENTS_TEST_ROOT")
}
