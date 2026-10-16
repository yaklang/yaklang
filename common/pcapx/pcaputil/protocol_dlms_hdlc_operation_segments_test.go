package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func hdlcOperationSegmentsControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-operation-segments", "DLMS_HDLC_OPERATION_SEGMENTS_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 18)
	return x.Cases
}
func TestDLMSHDLCOperationSegmentsNative(t *testing.T) {
	hdlcOpNative(t, hdlcOperationSegmentsControls(t), 41000, 4059)
}
func TestDLMSHDLCOperationSegmentsOffPort(t *testing.T) {
	hdlcOpNative(t, hdlcOperationSegmentsControls(t)[:2], 45000, 45001)
}
func TestDLMSHDLCOperationSegmentsMatrix(t *testing.T) {
	hdlcOperationSegmentsMatrix(t, hdlcOperationSegmentsControls(t))
}
func hdlcOperationSegmentsMatrix(t *testing.T, cases []hdlcOpCase) {
	for _, c := range cases {
		for _, transport := range []string{"udp", "tcp"} {
			t.Run(c.Name+"/"+transport, func(t *testing.T) {
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-operation-segments", "DLMS_HDLC_OPERATION_SEGMENTS_TEST_ROOT", "reuse-inputs.json"), &reuse))
				name := c.Name + "." + transport + ".pcap"
				raw := []byte(nil)
				if path, ok := reuse[name]; ok {
					parts := strings.SplitN(path, "/", 2)
					require.Len(t, parts, 2)
					raw = finsReadBatch(t, parts[0], "DLMS_HDLC_OPERATION_SEGMENTS_TEST_ROOT", parts[1])
				} else {
					raw = finsReadBatch(t, "dlms-hdlc-operation-segments", "DLMS_HDLC_OPERATION_SEGMENTS_TEST_ROOT", name)
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

func hdlcOperationSegmentsRefusalControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-operation-segments", "DLMS_HDLC_OPERATION_SEGMENTS_TEST_ROOT", "refusal-supplement.json"), &x))
	require.Len(t, x.Cases, 2)
	return x.Cases
}
func TestDLMSHDLCOperationSegmentsRefusalNative(t *testing.T) {
	hdlcOpNative(t, hdlcOperationSegmentsRefusalControls(t), 41000, 4059)
}
func TestDLMSHDLCOperationSegmentsRefusalMatrix(t *testing.T) {
	hdlcOperationSegmentsMatrix(t, hdlcOperationSegmentsRefusalControls(t))
}

// One capability upgrade; captured bytes and all prior answers stay archived.
func hdlcOperationSegmentHistoricalUpgrade(t *testing.T, raw []byte, cases []hdlcOpCase) []hdlcOpCase {
	t.Helper()
	require.Equal(t, "8960817ecc3ed410dd700045fccb30a903d1d198e3ad26c885e0673ed8333be8", fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Len(t, cases, 47)
	require.Equal(t, "segmented-Set-after-known-carrier-unimplemented", cases[33].Name)
	patch := finsReadBatch(t, "dlms-hdlc-operation-segments", "DLMS_HDLC_OPERATION_SEGMENTS_TEST_ROOT", "historical-upgrade.json")
	require.Equal(t, "a74ef94bffcf48ab44950a1e99e6123917419ae6e994d1b2f03194001ee3d33c", fmt.Sprintf("%x", sha256.Sum256(patch)))
	var update struct{ Case hdlcOpCase }
	require.NoError(t, json.Unmarshal(patch, &update))
	require.Equal(t, cases[33].Name, update.Case.Name)
	var original struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(raw, &original))
	var replacement struct{ Case map[string]any }
	require.NoError(t, json.Unmarshal(patch, &replacement))
	before, after := original.Cases[33], replacement.Case
	require.Len(t, after, len(before))
	for key, value := range before {
		if key != "steps" && key != "Close" {
			require.Equal(t, value, after[key], key)
		}
	}
	a, b := before["steps"].([]any), after["steps"].([]any)
	require.Len(t, a, 3)
	require.Len(t, b, 3)
	require.Equal(t, a[:2], b[:2])
	allowed := map[string]bool{}
	for _, key := range []string{"profile", "status", "completeness", "error", "fields", "session", "query_error", "GetFields", "Decode"} {
		allowed[key] = true
	}
	oldStep, newStep := a[2].(map[string]any), b[2].(map[string]any)
	require.Len(t, newStep, len(oldStep))
	for key, value := range oldStep {
		if !allowed[key] {
			require.Equal(t, value, newStep[key], key)
		}
	}
	require.Equal(t, "eaecc09c7885c97e5428107c7be256eb430cb7c3c9927c050dd49b00948d797c", update.Case.Steps[2].SHA)
	require.Empty(t, update.Case.Steps[2].Error)
	require.Zero(t, update.Case.Steps[2].Response)
	require.Zero(t, update.Case.Steps[2].Tx)
	require.Nil(t, update.Case.Steps[2].Fields["Set"])
	require.Equal(t, false, update.Case.Steps[2].Fields["HDLC Segmentation"].(map[string]any)["complete"])
	require.Len(t, update.Case.Close.Events, 1)
	require.Equal(t, map[string]any{"Outstanding": float64(1), "ObservedFrames": float64(1), "InformationBytes": float64(21)}, update.Case.Close.Events[0].Session)
	result := append([]hdlcOpCase(nil), cases...)
	result[33] = update.Case
	return result
}
func TestDLMSHDLCOperationSegmentsHistoricalUpgradeBinding(t *testing.T) {
	raw := finsReadBatch(t, "dlms-hdlc-operations", "DLMS_HDLC_OPERATIONS_TEST_ROOT", "controls.json")
	// Independent unmarshals prevent a shallow alias from making invariants pass.
	var untouched, parsed struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &untouched))
	require.NoError(t, json.Unmarshal(raw, &parsed))
	upgraded := hdlcOperationSegmentHistoricalUpgrade(t, raw, parsed.Cases)
	for i := range untouched.Cases {
		if i != 33 {
			require.Equal(t, untouched.Cases[i], upgraded[i])
		}
	}
	require.Equal(t, untouched.Cases[33].Steps[:2], upgraded[33].Steps[:2])
	for i := range untouched.Cases[33].Steps {
		a, b := untouched.Cases[33].Steps[i], upgraded[33].Steps[i]
		require.Equal(t, a.Wire, b.Wire)
		require.Equal(t, a.SHA, b.SHA)
		require.Equal(t, a.Direction, b.Direction)
		require.Equal(t, a.At, b.At)
		require.Equal(t, a.OffsetTCP, b.OffsetTCP)
		require.Equal(t, a.OffsetUDP, b.OffsetUDP)
		require.Equal(t, a.Raw, b.Raw)
		require.Equal(t, a.Length, b.Length)
		require.Equal(t, a.FlowID, b.FlowID)
		require.Equal(t, a.Tx, b.Tx)
		require.Equal(t, a.Response, b.Response)
	}
}
