package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// Budget variants reuse exact canonical bytes; semantic expectations remain the
// independent native gold, including the genuinely pending owner at refusal.
func snWriteResourceCaptureHashes(t *testing.T, carrier, kind string) (string, string) {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-write-resources", "DLMS_HDLC_SN_WRITE_TEST_ROOT", "capture-index.json")
	require.Equal(t, "70922793e1428969cf1ec483fc8e7615fab61739803d79903217e84d7621a97a", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var index struct {
		Captures []struct{ Carrier, Kind, Transport, File, SHA256 string }
	}
	require.NoError(t, json.Unmarshal(raw, &index))
	require.Len(t, index.Captures, 8)
	seen := map[string]bool{}
	var udp, tcp string
	for _, c := range index.Captures {
		require.Contains(t, []string{"wrapper", "hdlc"}, c.Carrier)
		require.Contains(t, []string{"resource", "normal"}, c.Kind)
		require.Contains(t, []string{"udp", "tcp"}, c.Transport)
		key := c.Carrier + "-" + c.Kind + "." + c.Transport + ".pcap"
		require.Equal(t, key, c.File)
		require.False(t, seen[key])
		seen[key] = true
		require.Len(t, c.SHA256, 64)
		if c.Carrier == carrier && c.Kind == kind {
			if c.Transport == "udp" {
				udp = c.SHA256
			} else {
				tcp = c.SHA256
			}
		}
	}
	require.NotEmpty(t, udp)
	require.NotEmpty(t, tcp)
	return udp, tcp
}
func TestDLMSWrapperSNWriteResourcePendingOwnerMatrix(t *testing.T) {
	for i, c := range snWriteResourceWrapperControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			kind := "resource"
			if i%2 == 1 {
				kind = "normal"
			}
			c.Name = "wrapper-" + kind
			c.Folder = "dlms-hdlc-sn-write-resources"
			c.UDPHash, c.TCPHash = snWriteResourceCaptureHashes(t, "wrapper", kind)
			snWriteMatrix(t, []snWriteControl{c})
		})
	}
}
func TestDLMSHDLCSNWriteResourcePendingOwnerMatrix(t *testing.T) {
	for i, c := range snWriteResourceHDLCControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			kind := "resource"
			if i%2 == 1 {
				kind = "normal"
			}
			c.Name = "hdlc-" + kind
			c.UDPHash, c.TCPHash = snWriteResourceCaptureHashes(t, "hdlc", kind)
			hdlcSNReadMatrix(t, []hdlcSNReadCase{c}, "dlms-hdlc-sn-write-resources")
		})
	}
}
