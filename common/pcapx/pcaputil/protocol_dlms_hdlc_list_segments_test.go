package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func hdlcListSegmentsControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-list-segments", "DLMS_HDLC_LIST_SEGMENTS_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 19)
	return x.Cases
}
func TestDLMSHDLCListSegmentsNative(t *testing.T) {
	hdlcOpNative(t, hdlcListSegmentsControls(t), 41000, 4059)
}
func TestDLMSHDLCListSegmentsOffPort(t *testing.T) {
	hdlcOpNative(t, hdlcListSegmentsControls(t)[:2], 45000, 45001)
}
func TestDLMSHDLCListSegmentsMatrix(t *testing.T) {
	hdlcListSegmentsMatrix(t, hdlcListSegmentsControls(t))
}
func hdlcListSegmentsMatrix(t *testing.T, cases []hdlcOpCase) {
	hdlcListSegmentsMatrixBatch(t, cases, "dlms-hdlc-list-segments", "DLMS_HDLC_LIST_SEGMENTS_TEST_ROOT")
}
func hdlcListSegmentsMatrixBatch(t *testing.T, cases []hdlcOpCase, batch, env string) {
	for _, c := range cases {
		for _, transport := range []string{"udp", "tcp"} {
			t.Run(c.Name+"/"+transport, func(t *testing.T) {
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, batch, env, "reuse-inputs.json"), &reuse))
				name := c.Name + "." + transport + ".pcap"
				raw := []byte(nil)
				if path, ok := reuse[name]; ok {
					parts := strings.SplitN(path, "/", 2)
					require.Len(t, parts, 2)
					raw = finsReadBatch(t, parts[0], env, parts[1])
				} else {
					raw = finsReadBatch(t, batch, env, name)
				}
				discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
					es, stats := hdlcOpReplay(t, c, raw, w, d, o, transport)
					count := len(c.Steps)
					if transport == "tcp" {
						for _, s := range c.Steps {
							if s.TCPOverride != nil {
								count--
							}
						}
					}
					closeCount := len(c.Close.Events)
					if transport == "tcp" && c.TCPCloseOverride != nil {
						closeCount = 0
					}
					require.Len(t, es, count+closeCount)
					hdlcOpCheck(t, c, es[:count], transport, true, d)
					hdlcOpCloseCheck(t, c, es[count:], transport, "ctx-canceled")
					require.EqualValues(t, count, stats.Messages)
					require.EqualValues(t, closeCount, stats.Incomplete)
				})
			})
		}
	}
}

// One new capability projection; old archive, input contexts and original answers remain immutable.
func hdlcListSegmentHistoricalUpgrade(t *testing.T, raw []byte, cases []hdlcOpCase) []hdlcOpCase {
	t.Helper()
	require.Equal(t, "4afe9a2d49ba7945c26d47b374171d23750640374a55d2ff40e5a5937885808c", fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Len(t, cases, 46)
	require.Equal(t, "Set-list-segmented", cases[42].Name)
	patch := finsReadBatch(t, "dlms-hdlc-list-segments", "DLMS_HDLC_LIST_SEGMENTS_TEST_ROOT", "historical-upgrade.json")
	require.Equal(t, "0d3e8139844dcbe6127f9e8ec720294a6ae1a6a8fda7c0632d991919b5269b3f", fmt.Sprintf("%x", sha256.Sum256(patch)))
	var update struct{ Case hdlcOpCase }
	require.NoError(t, json.Unmarshal(patch, &update))
	require.Equal(t, cases[42].Name, update.Case.Name)
	var old struct{ Cases []map[string]any }
	var next struct{ Case map[string]any }
	require.NoError(t, json.Unmarshal(raw, &old))
	require.NoError(t, json.Unmarshal(patch, &next))
	before, after := old.Cases[42], next.Case
	require.Len(t, after, len(before))
	for k, v := range before {
		if k != "steps" && k != "Close" {
			require.Equal(t, v, after[k], k)
		}
	}
	a, b := before["steps"].([]any), after["steps"].([]any)
	require.Len(t, a, 4)
	require.Len(t, b, 4)
	require.Equal(t, a[:2], b[:2])
	allowed := map[string]bool{}
	for _, k := range []string{"profile", "status", "completeness", "error", "fields", "session", "query_error", "GetFields", "Decode"} {
		allowed[k] = true
	}
	for i := 2; i < 4; i++ {
		aa, bb := a[i].(map[string]any), b[i].(map[string]any)
		require.Len(t, bb, len(aa))
		for k, v := range aa {
			if !(i == 2 && allowed[k] || i == 3 && k == "profile") {
				require.Equal(t, v, bb[k], fmt.Sprintf("step%d/%s", i, k))
			}
		}
	}
	require.Equal(t, "def46687ae97055ae12b725b7673f30ee4553238ad1499cdfdaf84232ba0c259", update.Case.Steps[2].SHA)
	require.Empty(t, update.Case.Steps[2].Error)
	require.Zero(t, update.Case.Steps[2].Response)
	require.Zero(t, update.Case.Steps[2].Tx)
	require.Nil(t, update.Case.Steps[2].Fields["Set"])
	require.Equal(t, false, update.Case.Steps[2].Fields["HDLC Segmentation"].(map[string]any)["complete"])
	require.Equal(t, "ContextRequired", update.Case.Steps[3].Error)
	require.Len(t, update.Case.Close.Events, 1)
	require.Equal(t, map[string]any{"Outstanding": float64(1), "ObservedFrames": float64(1), "InformationBytes": float64(35)}, update.Case.Close.Events[0].Session)
	result := append([]hdlcOpCase(nil), cases...)
	result[42] = update.Case
	return result
}
func TestDLMSHDLCListSegmentsHistoricalUpgradeBinding(t *testing.T) {
	raw := finsReadBatch(t, "dlms-hdlc-operation-lists", "DLMS_HDLC_OPERATION_LISTS_TEST_ROOT", "controls.json")
	var untouched, parsed struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &untouched))
	require.NoError(t, json.Unmarshal(raw, &parsed))
	changed := hdlcListSegmentHistoricalUpgrade(t, raw, parsed.Cases)
	for i := range untouched.Cases {
		if i != 42 {
			require.Equal(t, untouched.Cases[i], changed[i])
		}
		require.Len(t, changed[i].Steps, len(untouched.Cases[i].Steps))
		for j, w := range untouched.Cases[i].Steps {
			v := changed[i].Steps[j]
			require.Equal(t, w.Wire, v.Wire)
			require.Equal(t, w.SHA, v.SHA)
			require.Equal(t, w.Raw, v.Raw)
			require.Equal(t, w.Direction, v.Direction)
			require.Equal(t, w.At, v.At)
			require.Equal(t, w.OffsetTCP, v.OffsetTCP)
			require.Equal(t, w.OffsetUDP, v.OffsetUDP)
			require.Equal(t, w.Response, v.Response)
			require.Equal(t, w.Tx, v.Tx)
		}
	}
}
