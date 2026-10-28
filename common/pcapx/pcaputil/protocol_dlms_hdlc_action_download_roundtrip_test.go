package pcaputil

import (
	"testing"
)

func hdlcActionDownloadRoundtripControls(t *testing.T) []hdlcOpCase {
	return hdlcActionDownloadRequestCases(t, "roundtrip-supplement.json", 1, "bf126127a2c49e23bd998fb9acfa5925212225446a24510c540c4e8fecef1c3c")
}
func TestDLMSHDLCActionDownloadRoundtripNative(t *testing.T) {
	hdlcOpNative(t, hdlcActionDownloadRoundtripControls(t), 41000, 4059)
}
func TestDLMSHDLCActionDownloadRoundtripMatrix(t *testing.T) {
	hdlcListSegmentsMatrixBatch(t, hdlcActionDownloadRoundtripControls(t), "dlms-hdlc-action-download-request-segments", "DLMS_HDLC_ACTION_DOWNLOAD_REQUEST_SEGMENTS_TEST_ROOT")
}
