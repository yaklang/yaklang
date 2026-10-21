package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcActionUploadForeignGetOwnerControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-action-upload-foreign-get-owner", "DLMS_HDLC_ACTION_UPLOAD_FOREIGN_GET_OWNER_TEST_ROOT", "controls.json")
	require.Equal(t, "b6a390ee725a42f13a4c448541ffb03467444d2d98958f9cb9a4665bbd154928", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &x))
	require.Len(t, x.Cases, 3)
	return x.Cases
}
func TestDLMSHDLCActionUploadForeignGetOwnerNative(t *testing.T) {
	hdlcOpNative(t, hdlcActionUploadForeignGetOwnerControls(t), 41000, 4059)
}
func TestDLMSHDLCActionUploadForeignGetOwnerOffPort(t *testing.T) {
	c := hdlcActionUploadForeignGetOwnerControls(t)
	hdlcOpNative(t, c, 45000, 45001)
}
func TestDLMSHDLCActionUploadForeignGetOwnerMatrix(t *testing.T) {
	hdlcListSegmentsMatrixBatch(t, hdlcActionUploadForeignGetOwnerControls(t), "dlms-hdlc-action-upload-foreign-get-owner", "DLMS_HDLC_ACTION_UPLOAD_FOREIGN_GET_OWNER_TEST_ROOT")
}
