package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snUploadControls(t *testing.T, native, baseline bool) []snWriteControl {
	t.Helper()
	name, pin, count := "controls.json", "cfc8111f70640c337cbfc7a8f74dca1bf77c693fe7302efbf71e057abf859df5", 20
	if baseline {
		name, pin, count = "baseline-controls.json", "aff8eb648b54b01d2ed5df71cdec0642a615419d1ec8ae635f8813186ec444d1", 3
	}
	return snUploadLoadControls(t, native, name, pin, count)
}

func snUploadLoadControls(t *testing.T, native bool, name, pin string, count int) []snWriteControl {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-read-upload", "DLMS_SN_UPLOAD_TEST_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []snReadControl }
	var h struct {
		Cases []struct {
			UDP        string          `json:"sha256"`
			TCP        string          `json:"tcp_sha256"`
			UDPClosing snReadCloseSpec `json:"native_udp_Close"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &d))
	require.NoError(t, json.Unmarshal(raw, &h))
	require.Len(t, d.Cases, count)
	require.Len(t, h.Cases, count)
	out := make([]snWriteControl, count)
	for i, c := range d.Cases {
		if native {
			c.Close = h.Cases[i].UDPClosing
		}
		out[i] = snWriteControl{c, h.Cases[i].UDP, h.Cases[i].TCP, "dlms-sn-upload-captures"}
	}
	return out
}

// All helpers/session/event APIs predate this feature. Immutable independent
// full field/session/lifecycle gold, not an absent decoder API, proves old failure.
func TestDLMSWrapperSNUploadExistingAPI(t *testing.T) {
	snWriteNative(t, snUploadControls(t, true, true))
}
func TestDLMSWrapperSNUploadNative(t *testing.T) { snWriteNative(t, snUploadControls(t, true, false)) }
func TestDLMSWrapperSNUploadMatrix(t *testing.T) { snWriteMatrix(t, snUploadControls(t, false, false)) }

func TestDLMSWrapperSNUploadContextNative(t *testing.T) {
	snWriteNative(t, snUploadLoadControls(t, true, "context-supplement.json", "5fd002d5b5fa30acdbff08977c49e3d36e00d057178d3e0ec297c8c49aec1ad7", 2))
}
func TestDLMSWrapperSNUploadContextMatrix(t *testing.T) {
	snWriteMatrix(t, snUploadLoadControls(t, false, "context-supplement.json", "5fd002d5b5fa30acdbff08977c49e3d36e00d057178d3e0ec297c8c49aec1ad7", 2))
}

func TestDLMSWrapperSNCompletedEchoNative(t *testing.T) {
	snWriteNative(t, snUploadLoadControls(t, true, "completed-echo-supplement.json", "0b8bc780a0f2a68bafe4817b1a2ee3b5e61c780a3d4af5c914861a30dbd33d6f", 2))
}
func TestDLMSWrapperSNCompletedEchoMatrix(t *testing.T) {
	snWriteMatrix(t, snUploadLoadControls(t, false, "completed-echo-supplement.json", "0b8bc780a0f2a68bafe4817b1a2ee3b5e61c780a3d4af5c914861a30dbd33d6f", 2))
}
