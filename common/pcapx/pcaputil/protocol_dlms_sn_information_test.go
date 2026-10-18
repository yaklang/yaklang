package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snInformationCases(t *testing.T, name, pin string, count int) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-information-segments", "DLMS_SN_SEGMENTS_TEST_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct{ Cases []hdlcOpCase }
	var meta struct {
		Cases []struct {
			Steps []map[string]any
			UDP   string `json:"sha256"`
			TCP   string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &gold))
	require.NoError(t, json.Unmarshal(raw, &meta))
	require.Len(t, gold.Cases, count)
	require.Len(t, meta.Cases, count)
	out := make([]hdlcSNReadCase, count)
	for i, c := range gold.Cases {
		out[i] = hdlcSNReadCase{c, meta.Cases[i].Steps, meta.Cases[i].UDP, meta.Cases[i].TCP}
	}
	return out
}
func TestDLMSSNInformationSegmentsExistingAPI(t *testing.T) {
	hdlcSNReadNative(t, snInformationCases(t, "baseline-controls.json", "203bc38549f57d2579b70621b39b1654913149eaed491dc9e2a05f11b7d48f53", 2), 41000, 4059)
}

func TestDLMSSNInformationSegmentsNative(t *testing.T) {
	hdlcSNReadNative(t, snInformationCases(t, "controls.json", "45b769772414808e774269de75f9825a5af3ac47260f753d397d2de64d417617", 9), 41000, 4059)
}

func TestDLMSSNInformationSegmentsMatrix(t *testing.T) {
	hdlcSNReadMatrix(t, snInformationCases(t, "controls.json", "45b769772414808e774269de75f9825a5af3ac47260f753d397d2de64d417617", 9), "dlms-sn-information-segments")
}
func TestDLMSSNInformationSegmentsOffPort(t *testing.T) {
	c := snInformationCases(t, "controls.json", "45b769772414808e774269de75f9825a5af3ac47260f753d397d2de64d417617", 9)
	hdlcSNReadNative(t, []hdlcSNReadCase{c[0], c[2]}, 45000, 45001)
}

func TestDLMSSNInformationGETPayloadLookalike(t *testing.T) {
	hdlcSNReadNative(t, snInformationCases(t, "continuation-lookalike.json", "fe848148e3f00adb2904797620fed4761032d387fd66d93c62c9785c3721e383", 2), 41000, 4059)
}

func TestDLMSSNInformationHardBoundariesNative(t *testing.T) {
	hdlcSNReadNative(t, snInformationCases(t, "hard-boundaries.json", "3cabdf29e72d070ddf499cb88343bc7e412e7dd022e1cb6d4f48e6bad52735aa", 4), 41000, 4059)
}
func TestDLMSSNInformationHardBoundariesMatrix(t *testing.T) {
	hdlcSNReadMatrix(t, snInformationCases(t, "hard-boundaries.json", "3cabdf29e72d070ddf499cb88343bc7e412e7dd022e1cb6d4f48e6bad52735aa", 4), "dlms-sn-information-segments-hard")
}
func TestDLMSSNInformationGETPayloadLookalikeMatrix(t *testing.T) {
	hdlcSNReadMatrix(t, snInformationCases(t, "continuation-lookalike.json", "fe848148e3f00adb2904797620fed4761032d387fd66d93c62c9785c3721e383", 2), "dlms-sn-information-segments-lookalike")
}
