package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func snWriteUploadControls(t *testing.T, native, baseline bool) []snWriteControl {
	t.Helper()
	name, pin, count := "full-controls.json", "e1f3e7ad9e8088c8769c6153fa614778f29ddb768c750041e3b31f9a20448df8", 24
	if baseline {
		name, pin, count = "full-baseline-controls.json", "c9f3b776b768eb94b4ab6229e83ce9afac03f44b17b567a6a8dbad832c45a5a1", 3
	}
	return snWriteUploadLoadControls(t, native, name, pin, count)
}

func snWriteUploadLoadControls(t *testing.T, native bool, name, pin string, count int) []snWriteControl {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-write-upload", "DLMS_SN_WRITE_UPLOAD_TEST_ROOT", name)
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
		out[i] = snWriteControl{c, h.Cases[i].UDP, h.Cases[i].TCP, "dlms-sn-write-upload-captures"}
	}
	return out
}

// Complete immutable independent fields, Session, IDs, errors, ownership and
// lifecycle are asserted through session/capture APIs that predate this feature.
func TestDLMSWrapperSNWriteUploadExistingAPI(t *testing.T) {
	snWriteNative(t, snWriteUploadControls(t, true, true))
}
func TestDLMSWrapperSNWriteUploadNative(t *testing.T) {
	snWriteNative(t, snWriteUploadControls(t, true, false))
}
func TestDLMSWrapperSNWriteUploadMatrix(t *testing.T) {
	snWriteMatrix(t, snWriteUploadControls(t, false, false))
}
