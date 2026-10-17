package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcSNUploadControls(t *testing.T, boundary bool) []hdlcSNReadCase {
	t.Helper()
	name, pin, count := "controls.json", "5dde41f21a5c9c477cc43cf11c896c473de53f55d57f98dfa61faa711b6044c3", 12
	if boundary {
		name, pin, count = "information-boundary-supplement.json", "3cddf7b150c0e11afdec9fa704a0db6c7351bca554684d7c76f08ca05fa9a399", 3
	}
	raw := finsReadBatch(t, "dlms-hdlc-sn-upload", "DLMS_HDLC_SN_UPLOAD_TEST_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Cases []hdlcOpCase }
	var meta struct {
		Cases []struct {
			Steps []map[string]any
			UDP   string `json:"sha256"`
			TCP   string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.NoError(t, json.Unmarshal(raw, &meta))
	require.Len(t, doc.Cases, count)
	require.Len(t, meta.Cases, count)
	out := make([]hdlcSNReadCase, count)
	for i, c := range doc.Cases {
		require.Len(t, meta.Cases[i].Steps, len(c.Steps))
		out[i] = hdlcSNReadCase{hdlcOpCase: c, Metadata: meta.Cases[i].Steps, UDPHash: meta.Cases[i].UDP, TCPHash: meta.Cases[i].TCP}
	}
	if boundary {
		out = snWriteUploadInformationUpgrade(t, raw, out)
	}
	return out
}
func TestDLMSHDLCSNUploadNative(t *testing.T) {
	hdlcSNReadNative(t, hdlcSNUploadControls(t, false), 41000, 4059)
}
func TestDLMSHDLCSNUploadMatrix(t *testing.T) {
	hdlcSNReadMatrix(t, hdlcSNUploadControls(t, false), "dlms-hdlc-sn-upload")
}
func TestDLMSHDLCSNUploadInformationBoundaryNative(t *testing.T) {
	hdlcSNReadNative(t, hdlcSNUploadControls(t, true), 41000, 4059)
}
func TestDLMSHDLCSNUploadInformationBoundaryMatrix(t *testing.T) {
	hdlcSNReadMatrix(t, hdlcSNUploadControls(t, true), "dlms-hdlc-sn-upload")
}

func hdlcSNUploadCarrierControls(t *testing.T) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-upload", "DLMS_HDLC_SN_UPLOAD_TEST_ROOT", "carrier-boundary-controls.json")
	require.Equal(t, "e5954b43a5fe745b5791a305bb999a75e306cff5c1e6770128359b5863ceb8d6", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Cases []hdlcOpCase }
	var meta struct {
		Cases []struct {
			Steps []map[string]any
			UDP   string `json:"sha256"`
			TCP   string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.NoError(t, json.Unmarshal(raw, &meta))
	require.Len(t, doc.Cases, 3)
	require.Len(t, meta.Cases, 3)
	out := make([]hdlcSNReadCase, 3)
	for i, c := range doc.Cases {
		require.Len(t, meta.Cases[i].Steps, len(c.Steps))
		out[i] = hdlcSNReadCase{hdlcOpCase: c, Metadata: meta.Cases[i].Steps, UDPHash: meta.Cases[i].UDP, TCPHash: meta.Cases[i].TCP}
	}
	return out
}
func TestDLMSHDLCSNUploadCarrierNative(t *testing.T) {
	hdlcSNReadNative(t, hdlcSNUploadCarrierControls(t), 41000, 4059)
}
func TestDLMSHDLCSNUploadCarrierMatrix(t *testing.T) {
	hdlcSNReadMatrix(t, hdlcSNUploadCarrierControls(t), "dlms-hdlc-sn-upload")
}

func hdlcSNUploadTCPConflictControls(t *testing.T) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-upload", "DLMS_HDLC_SN_UPLOAD_TEST_ROOT", "tcp-requester-conflict-controls.json")
	require.Equal(t, "3ad33a52d9ab87f855b27ff08a5de75bd640deb72e914e33f7616d37b4a4d630", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Cases []hdlcOpCase }
	var meta struct {
		Cases []struct {
			Steps []map[string]any
			UDP   string `json:"sha256"`
			TCP   string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.NoError(t, json.Unmarshal(raw, &meta))
	require.Len(t, doc.Cases, 1)
	require.Len(t, meta.Cases, 1)
	out := make([]hdlcSNReadCase, 1)
	for i, c := range doc.Cases {
		require.Len(t, meta.Cases[i].Steps, len(c.Steps))
		out[i] = hdlcSNReadCase{hdlcOpCase: c, Metadata: meta.Cases[i].Steps, UDPHash: meta.Cases[i].UDP, TCPHash: meta.Cases[i].TCP}
	}
	return out
}

func TestDLMSHDLCSNUploadTCPRequesterConflict(t *testing.T) {
	hdlcSNReadMatrixTransports(t, hdlcSNUploadTCPConflictControls(t), "dlms-hdlc-sn-upload", []string{"tcp"})
}
