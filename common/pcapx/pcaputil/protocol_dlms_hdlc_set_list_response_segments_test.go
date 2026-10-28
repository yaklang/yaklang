package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcSetListResponseSegmentsControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-set-list-response-segments", "DLMS_HDLC_SET_LIST_RESPONSE_SEGMENTS_TEST_ROOT", "controls.json")
	require.Equal(t, "750c518991502495586ec41eb974a45280473596e3ca68415c2ea92b4755bacd", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &x))
	require.Len(t, x.Cases, 22)
	return x.Cases
}
func TestDLMSHDLCSetListResponseSegmentsNative(t *testing.T) {
	hdlcOpNative(t, hdlcSetListResponseSegmentsControls(t), 41000, 4059)
}
func TestDLMSHDLCSetListResponseSegmentsOffPort(t *testing.T) {
	c := hdlcSetListResponseSegmentsControls(t)
	hdlcOpNative(t, c[:3], 45000, 45001)
}
func TestDLMSHDLCSetListResponseSegmentsMatrix(t *testing.T) {
	hdlcListSegmentsMatrixBatch(t, hdlcSetListResponseSegmentsControls(t), "dlms-hdlc-set-list-response-segments", "DLMS_HDLC_SET_LIST_RESPONSE_SEGMENTS_TEST_ROOT")
}
