package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func hdlcSNBlockControls(t *testing.T) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-read-block", "DLMS_HDLC_SN_BLOCK_TEST_ROOT", "controls.json")
	require.Equal(t, "2f4e6fdcfdf2ec090f356bb11bc66f09a7555f423e4041bf12623bdb1d36f3ea", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var typed struct{ Cases []hdlcOpCase }
	var meta struct {
		Cases []struct {
			Steps []map[string]any
			UDP   string `json:"sha256"`
			TCP   string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &typed))
	require.NoError(t, json.Unmarshal(raw, &meta))
	require.Len(t, typed.Cases, 23)
	require.Len(t, meta.Cases, 23)
	out := make([]hdlcSNReadCase, len(typed.Cases))
	for i, c := range typed.Cases {
		require.Len(t, c.Steps, len(meta.Cases[i].Steps))
		out[i] = hdlcSNReadCase{hdlcOpCase: c, Metadata: meta.Cases[i].Steps, UDPHash: meta.Cases[i].UDP, TCPHash: meta.Cases[i].TCP}
	}
	return out
}
func TestDLMSHDLCSNBlocksNative(t *testing.T) {
	hdlcSNReadNative(t, hdlcSNBlockControls(t), 41000, 4059)
}
func TestDLMSHDLCSNBlocksMatrix(t *testing.T) {
	hdlcSNReadMatrix(t, hdlcSNBlockControls(t), "dlms-hdlc-sn-read-block")
}
