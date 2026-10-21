package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcSNGBTSplitHeaderLoad(t *testing.T, name, pin string, count int) ([]hdlcOpCase, map[string]hdlcSNGBTInformationMeta) {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-gbt-split-header-source", "DLMS_HDLC_SN_GBT_SPLIT_HEADER_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, count)
	var extra struct{ Cases []hdlcSNGBTInformationMeta }
	require.NoError(t, json.Unmarshal(raw, &extra))
	require.Len(t, extra.Cases, len(d.Cases))
	m := map[string]hdlcSNGBTInformationMeta{}
	for i, c := range d.Cases {
		require.Equal(t, c.Name, extra.Cases[i].Name)
		require.Len(t, extra.Cases[i].Steps, len(c.Steps))
		require.Len(t, extra.Cases[i].Close.Events, len(c.Close.Events))
		m[c.Name] = extra.Cases[i]
	}
	return d.Cases, m
}
func TestDLMSHDLCSNGBTSplitHeaderExistingAPI(t *testing.T) {
	c, m := hdlcSNGBTSplitHeaderLoad(t, "baseline-controls.json", "8e8fa3f260b3649bc4e282642cf428a331de26b5056bc06dbbb282f8d01fb9d7", 3)
	hdlcSNGBTInformationNative(t, c, m)
}
func TestDLMSHDLCSNGBTSplitHeaderNative(t *testing.T) {
	c, m := hdlcSNGBTSplitHeaderLoad(t, "full-controls.json", "efa38f4980158a783d97f7a06a8b36686ae567fe9d575aa6059907112d795912", 22)
	hdlcSNGBTInformationNative(t, c, m)
}
