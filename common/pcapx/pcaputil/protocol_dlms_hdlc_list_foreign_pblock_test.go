package pcaputil

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcListForeignPblockControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-list-foreign-pblock", "DLMS_HDLC_FOREIGN_PBLOCK_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 2)
	return x.Cases
}
func TestDLMSHDLCListForeignPblockNative(t *testing.T) {
	hdlcOpNative(t, hdlcListForeignPblockControls(t), 41000, 4059)
}
func TestDLMSHDLCListForeignPblockOffPort(t *testing.T) {
	hdlcOpNative(t, hdlcListForeignPblockControls(t), 45000, 45001)
}
func TestDLMSHDLCListForeignPblockMatrix(t *testing.T) {
	hdlcListSegmentsMatrixBatch(t, hdlcListForeignPblockControls(t), "dlms-hdlc-list-foreign-pblock", "DLMS_HDLC_FOREIGN_PBLOCK_TEST_ROOT")
}
