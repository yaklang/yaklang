package bin_parser

import (
	"encoding/binary"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"testing"
)

func TestNetworkControlSharedRulesAndBoundaries(t *testing.T) {
	ike := make([]byte, 48)
	ike[7] = 1
	ike[16] = 40
	ike[17] = 0x20
	ike[18] = 34
	ike[19] = 8
	binary.BigEndian.PutUint32(ike[24:], 48)
	binary.BigEndian.PutUint16(ike[30:], 20)
	wg := make([]byte, 148)
	wg[0] = 1
	wg[4] = 3
	l2 := mustHex(t, "c802001400000000000000008008000000000001")
	rip := mustHex(t, "010200000000000000000000000000000000000000000010")
	rpc := mustHex(t, "123456780000000000000002000186a0000000020000000000000000000000000000000000000000")
	ipmi := mustHex(t, "0600ff06000011be80000000")
	ov := mustHex(t, "3801020304050607080000000000")
	for _, tc := range []struct {
		name, entry, key string
		wire             []byte
	}{{"ike", "IKEMessage", "Exchange Type", ike}, {"wireguard", "WireGuardMessage", "Sender Index", wg}, {"l2tp", "L2TPMessage", "Tunnel ID", l2}, {"rip", "RIPMessage", "Command", rip}, {"onc-rpc", "ONCRPCMessage", "XID", rpc}, {"ipmi", "IPMIMessage", "RMCP Class", ipmi}, {"openvpn", "OpenVPNMessage", "Packet ID", ov}, {"telnet", "TelnetMessage", "Option", []byte{255, 253, 24}}} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := stream_parser.DecodeNetworkControl(tc.wire, tc.name)
			require.NoError(t, err)
			require.False(t, m["Authentication Verified"].(bool))
			n := protocolCorpusRequireBoundedRuleParse(t, tc.wire, "network_control_fields", tc.entry)
			require.NotEmpty(t, n.Children)
			parseMustFail(t, tc.wire[:1], "network_control_fields", tc.entry)
			got, err := ParseStructured(tc.wire, "network_control_fields", tc.entry)
			require.NoError(t, err)
			fields := got["fields"].(map[string]any)
			require.Contains(t, fields, tc.key)
			bad := append(append([]byte(nil), tc.wire...), 0)
			if tc.name != "onc-rpc" && tc.name != "openvpn" {
				_, err = stream_parser.DecodeNetworkControl(bad, tc.name)
				require.Error(t, err)
			}
			for cut := 0; cut < min(len(tc.wire), 14); cut++ {
				_, err := stream_parser.DecodeNetworkControl(tc.wire[:cut], tc.name)
				require.Error(t, err, "cut %d", cut)
			}
		})
	}
}

func TestNetworkControlNoPanicAndNoPrefixMutation(t *testing.T) {
	for _, p := range []string{"ike", "ike-natt", "wireguard", "l2tp", "rip", "onc-rpc", "nbt-dg", "ipmi", "openvpn", "telnet"} {
		for n := 0; n < 256; n++ {
			w := make([]byte, n)
			for i := range w {
				w[i] = byte(i*73 + n)
			}
			require.NotPanics(t, func() { _, _ = stream_parser.DecodeNetworkControl(w, p) }, "%s len %d", p, n)
		}
	}
}

func TestRMCPPlusIntegrityIsNotEncryptionAndLengthExcludesTrailer(t *testing.T) {
	// Two valid IPMI checksums, followed by an opaque integrity trailer.
	body := mustHex(t, "2018c88100017e")
	header := mustHex(t, "0600ff07064001000000010000000700")
	w := append(append(header, body...), make([]byte, 15)...)
	m, err := stream_parser.DecodeNetworkControl(w, "ipmi")
	require.NoError(t, err)
	require.Equal(t, false, m["Payload Encrypted"])
	require.Equal(t, false, m["Authentication Verified"])
	require.Equal(t, "clear; integrity unverified", m["Content Visibility"])
	require.Equal(t, true, m["Payload"].(map[string]any)["Checksum Verified"])
	w[5] = 0xc0
	m, err = stream_parser.DecodeNetworkControl(w, "ipmi")
	require.NoError(t, err)
	require.Equal(t, "encrypted", m["Content Visibility"])
	require.NotContains(t, m["Payload"].(map[string]any), "Checksum Verified")
	_, err = stream_parser.DecodeNetworkControl(w[:len(w)-10], "ipmi")
	require.Error(t, err)
}

func TestWireGuardTransportPaddingCanBeClippedToMTU(t *testing.T) {
	for _, n := range []int{32, 33, 64, 1452} {
		w := make([]byte, n)
		w[0] = 4
		m, err := stream_parser.DecodeNetworkControl(w, "wireguard")
		require.NoError(t, err)
		require.Equal(t, "Transport Data", m["Packet Name"])
		require.False(t, m["Authentication Verified"].(bool))
	}
	for n := 16; n < 32; n++ {
		w := make([]byte, n)
		w[0] = 4
		_, err := stream_parser.DecodeNetworkControl(w, "wireguard")
		require.Error(t, err)
	}
}

func FuzzNetworkControlWireProfiles(f *testing.F) {
	f.Add([]byte{6, 0, 255, 6, 0, 0, 17, 190, 128, 0, 0, 0})
	f.Add([]byte{255, 250, 24, 0, 255, 255, 255, 240})
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 65507 {
			return
		}
		for _, p := range []string{"ike", "ike-natt", "wireguard", "l2tp", "rip", "onc-rpc", "nbt-dg", "ipmi", "openvpn", "telnet"} {
			_, _ = stream_parser.DecodeNetworkControl(w, p)
		}
	})
}
