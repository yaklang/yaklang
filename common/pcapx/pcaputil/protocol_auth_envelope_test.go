package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestAuthenticationEnvelopeNativeSMBOracle(t *testing.T) {
	wire, err := trafficfixture.ReadFile("testdata/protocol-sessions/first-batch-oracles/smb2-samba-live.pcap")
	require.NoError(t, err)
	events, stats, err := binReplay(t, wire, 1)
	require.NoError(t, err)
	require.Zero(t, stats.BufferedBytes)
	types := map[uint32]bool{}
	guest := false
	for _, e := range events {
		if e.Protocol != "smb2" {
			continue
		}
		if e.Session["Guest Session"] == true {
			guest = true
		}
		auth, ok := e.Session["Authentication"].(map[string]any)
		if !ok {
			continue
		}
		require.Equal(t, "decoded", auth["Status"], "%+v", auth)
		require.Equal(t, "SPNEGO", auth["Protocol"])
		require.Equal(t, false, auth["MIC Verified"])
		require.Equal(t, false, auth["Authentication Verified"])
		if ntlm, ok := auth["Mechanism"].(map[string]any); ok {
			types[ntlm["Message Type"].(uint32)] = true
			require.Equal(t, false, ntlm["Authentication Verified"])
			if ntlm["Message Type"] == uint32(2) {
				require.Equal(t, "SMBLDAP", ntlm["Target Name"])
				require.NotEmpty(t, ntlm["Server Challenge"])
			}
			if ntlm["Message Type"] == uint32(3) {
				require.Equal(t, "root", ntlm["User Name"])
				require.Equal(t, "WORKGROUP", ntlm["Domain"])
			}
		} else {
			require.NotContains(t, auth, "Mechanism Error", "%+v", auth)
		}
	}
	require.Equal(t, map[uint32]bool{1: true, 2: true, 3: true}, types)
	require.True(t, guest, "server success was explicitly guest mapped, not proof of client identity")
}

func TestAuthenticationEnvelopeTruncationAndFalsePrefixes(t *testing.T) {
	valid := make([]byte, 32)
	copy(valid, []byte("NTLMSSP\x00"))
	binary.LittleEndian.PutUint32(valid[8:], 1)
	binary.LittleEndian.PutUint32(valid[12:], 1)
	for cut := 0; cut < len(valid); cut++ {
		_, err := decodeNTLMEnvelope(valid[:cut])
		require.Error(t, err, "cut %d", cut)
	}
	info, err := decodeNTLMEnvelope(valid)
	require.NoError(t, err)
	require.Equal(t, uint32(1), info["Message Type"])
	for _, offset := range []uint32{0, 16, 31, 32, 0xffffffff} {
		bad := bytes.Clone(valid)
		binary.LittleEndian.PutUint32(bad[12:], 0x1001)
		binary.LittleEndian.PutUint16(bad[16:], 10)
		binary.LittleEndian.PutUint32(bad[20:], offset)
		_, err := decodeNTLMEnvelope(bad)
		require.Error(t, err)
	}
	for _, w := range [][]byte{[]byte("NTLMSSP\x00"), append([]byte("prefix"), valid...), {0x60, 1, 0}, {0xa1, 2, 0x30, 1}} {
		got := decodeAuthenticationEnvelope(w)
		require.NotEqual(t, "decoded", got["Status"], "partial/embedded signatures must not become identity")
	}
}

func FuzzAuthenticationEnvelope(f *testing.F) {
	f.Add([]byte("NTLMSSP\x00"))
	f.Add([]byte{0xa1, 2, 0x30, 0})
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 65536 {
			return
		}
		decodeAuthenticationEnvelope(w)
	})
}
