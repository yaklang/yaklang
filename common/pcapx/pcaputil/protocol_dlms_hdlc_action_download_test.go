package pcaputil

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func hdlcActionDownloadControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-action-download", "DLMS_HDLC_ACTION_DOWNLOAD_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 18)
	return x.Cases
}
func TestDLMSHDLCActionDownloadNative(t *testing.T) {
	hdlcOpNative(t, hdlcActionDownloadControls(t), 41000, 4059)
}
func TestDLMSHDLCActionDownloadOffPort(t *testing.T) {
	hdlcOpNative(t, hdlcActionDownloadControls(t)[:2], 45000, 45001)
}
func TestDLMSHDLCActionDownloadMatrix(t *testing.T) {
	hdlcActionDownloadMatrix(t, hdlcActionDownloadControls(t))
}
func hdlcActionDownloadMatrix(t *testing.T, cases []hdlcOpCase) {
	for _, c := range cases {
		for _, transport := range []string{"udp", "tcp"} {
			t.Run(c.Name+"/"+transport, func(t *testing.T) {
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-action-download", "DLMS_HDLC_ACTION_DOWNLOAD_TEST_ROOT", "reuse-inputs.json"), &reuse))
				name := c.Name + "." + transport + ".pcap"
				raw := []byte(nil)
				if path, ok := reuse[name]; ok {
					parts := strings.SplitN(path, "/", 2)
					require.Len(t, parts, 2)
					raw = finsReadBatch(t, parts[0], "DLMS_HDLC_ACTION_DOWNLOAD_TEST_ROOT", parts[1])
				} else {
					raw = finsReadBatch(t, "dlms-hdlc-action-download", "DLMS_HDLC_ACTION_DOWNLOAD_TEST_ROOT", name)
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

func hdlcActionDownloadForeignControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-action-download", "DLMS_HDLC_ACTION_DOWNLOAD_TEST_ROOT", "foreign-form-supplement.json"), &x))
	require.Len(t, x.Cases, 2)
	return x.Cases
}
func TestDLMSHDLCActionDownloadForeignFormNative(t *testing.T) {
	hdlcOpNative(t, hdlcActionDownloadForeignControls(t), 41000, 4059)
}
func TestDLMSHDLCActionDownloadForeignFormMatrix(t *testing.T) {
	hdlcActionDownloadMatrix(t, hdlcActionDownloadForeignControls(t))
}
