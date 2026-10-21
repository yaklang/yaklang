package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDLMSHDLCGetCloseMatrix(t *testing.T) {
	c := hdlcGetCloseCases(t)
	raw := finsReadBatch(t, "dlms-hdlc-get-close", "DLMS_HDLC_SN_READ_TEST_ROOT", "controls.json")
	require.Equal(t, "362ecb5135c58085d40846abcf501d670160f8f2fc003bf369f8e6b5e96c2dfe", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var gold struct {
		Cases []struct {
			Name  string
			Steps []map[string]any
		}
	}
	require.NoError(t, json.Unmarshal(raw, &gold))
	require.Len(t, gold.Cases, len(c))
	raw = finsReadBatch(t, "dlms-hdlc-get-close", "DLMS_HDLC_SN_READ_TEST_ROOT", "capture-index.json")
	require.Equal(t, "b987b44206a51556d0a22d2cd4e7938caf228fce7a78fad1754509b264cd659f", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var captures []struct{ Case, Transport, File, SHA256 string }
	require.NoError(t, json.Unmarshal(raw, &captures))
	require.Len(t, captures, 12)
	var cases []hdlcSNReadCase
	for i, v := range c {
		require.Equal(t, v.Name, gold.Cases[i].Name)
		meta := gold.Cases[i].Steps
		for j, m := range meta {
			require.Equal(t, v.Steps[j].Wire, m["input_hex"])
			require.Equal(t, true, m["native_udp_flow_nonzero"])
			m["captured_udp_flow_nonzero"] = true
			m["capture_udp_direction"] = m["caller_direction"]
			m["PacketRef_udp"] = float64(j + 1)
			m["PacketRef_tcp"] = float64(j + 4)
		}
		entry := hdlcSNReadCase{hdlcOpCase: v, Metadata: meta}
		found := 0
		for _, cap := range captures {
			if cap.Case == v.Name {
				require.Equal(t, v.Name+"."+cap.Transport+".pcap", cap.File)
				require.Len(t, cap.SHA256, 64)
				if cap.Transport == "udp" {
					entry.UDPHash = cap.SHA256
				} else {
					require.Equal(t, "tcp", cap.Transport)
					entry.TCPHash = cap.SHA256
				}
				found++
			}
		}
		require.Equal(t, 2, found)
		cases = append(cases, entry)
	}
	hdlcSNReadMatrix(t, cases, "dlms-hdlc-get-close")
}
