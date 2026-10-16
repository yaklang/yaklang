package pcaputil

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func hdlcSNWriteCases(t *testing.T, name, pin string, count int) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-write", "DLMS_HDLC_SN_WRITE_TEST_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	var meta struct {
		Cases []struct {
			Steps []map[string]any
			UDP   string `json:"sha256"`
			TCP   string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &d))
	require.NoError(t, json.Unmarshal(raw, &meta))
	require.Len(t, d.Cases, count)
	out := make([]hdlcSNReadCase, len(d.Cases))
	for i, c := range d.Cases {
		for j := range c.Steps {
			for _, f := range []map[string]any{c.Steps[j].Fields, c.Steps[j].Session} {
				if f == nil {
					continue
				}
				if text, ok := f["LLC"].(string); ok {
					b, err := base64.StdEncoding.DecodeString(text)
					require.NoError(t, err)
					require.Len(t, b, 3)
					f["LLC"] = map[string]any{"hexbytes": fmt.Sprintf("%x", b)}
				}
			}
		}
		out[i] = hdlcSNReadCase{c, meta.Cases[i].Steps, meta.Cases[i].UDP, meta.Cases[i].TCP}
	}
	return out
}
func hdlcSNWriteControls(t *testing.T) []hdlcSNReadCase {
	return hdlcSNWriteCases(t, "controls.json", "2089396fca41e028327b03a0f762552ef8fb9f313d1d47ca81869dd7d806ed4c", 16)
}
func TestDLMSHDLCSNWriteNative(t *testing.T) {
	hdlcSNReadNative(t, hdlcSNWriteControls(t), 41000, 4059)
}
func TestDLMSHDLCSNWriteMatrix(t *testing.T) {
	hdlcSNReadMatrix(t, hdlcSNWriteControls(t), "dlms-hdlc-sn-write")
}
func TestDLMSHDLCSNWriteOffPort(t *testing.T) {
	c := hdlcSNWriteControls(t)
	hdlcSNReadNative(t, []hdlcSNReadCase{c[0], c[1], c[3]}, 45000, 45001)
}
