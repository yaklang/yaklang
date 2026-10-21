package pcaputil

import (
	"testing"
)

func hdlcActionDownloadResponsePrefixControls(t *testing.T) []hdlcOpCase {
	return hdlcActionDownloadResponseCases(t, "prefix-close.json", 1, "345d32a8b5f0bdf9aff543f1e2eb510c61c2156979cd44600f5fbc4021b51858")
}
func TestDLMSHDLCActionDownloadResponseSegmentsPrefixCloseNative(t *testing.T) {
	hdlcOpNative(t, hdlcActionDownloadResponsePrefixControls(t), 41000, 4059)
}
func TestDLMSHDLCActionDownloadResponseSegmentsPrefixCloseMatrix(t *testing.T) {
	hdlcListSegmentsMatrixBatch(t, hdlcActionDownloadResponsePrefixControls(t), "dlms-hdlc-action-download-response-segments", "DLMS_HDLC_ACTION_DOWNLOAD_RESPONSE_SEGMENTS_TEST_ROOT")
}
