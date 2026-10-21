package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcSetListBlocksControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	var out []hdlcOpCase
	for _, v := range []struct {
		name, sha string
		count     int
	}{
		{"controls.json", "82a1308e7fa87479cf36238d59be0cd46e297cd880558337cde0f4f8521836ca", 20},
		{"next-gap-supplement.json", "774fa27d1c8c3917fa34e6d63c243df7bccd5e45f4315fb7a929ddbb101149c6", 1},
		{"foreign-terminal-supplement.json", "55175a07bbd18faa02139099943bc8d043d2e85045c3a25aa80d58b22a0c4643", 1},
	} {
		raw := finsReadBatch(t, "dlms-hdlc-set-list-blocks", "DLMS_HDLC_SET_LIST_BLOCKS_TEST_ROOT", v.name)
		require.Equal(t, v.sha, fmt.Sprintf("%x", sha256.Sum256(raw)))
		var x struct{ Cases []hdlcOpCase }
		require.NoError(t, json.Unmarshal(raw, &x))
		require.Len(t, x.Cases, v.count)
		out = append(out, x.Cases...)
	}
	require.Len(t, out, 22)
	return out
}
func TestDLMSHDLCSetListBlocksNative(t *testing.T) {
	hdlcOpNative(t, hdlcSetListBlocksControls(t), 41000, 4059)
}
func TestDLMSHDLCSetListBlocksOffPort(t *testing.T) {
	c := hdlcSetListBlocksControls(t)
	hdlcOpNative(t, []hdlcOpCase{c[0], c[1], c[3]}, 45000, 45001)
}
func TestDLMSHDLCSetListBlocksMatrix(t *testing.T) {
	hdlcListSegmentsMatrixBatch(t, hdlcSetListBlocksControls(t), "dlms-hdlc-set-list-blocks", "DLMS_HDLC_SET_LIST_BLOCKS_TEST_ROOT")
}
