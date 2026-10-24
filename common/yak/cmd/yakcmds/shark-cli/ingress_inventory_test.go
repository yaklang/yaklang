package sharkcli

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/rules"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"strings"
	"testing"
)

func TestCommonPortHintsDeclareNativeOrCatalogOnlyCoverage(t *testing.T) {
	profiles := map[string]bool{}
	for _, p := range pcaputil.NativeProtocolProfiles() {
		require.False(t, profiles[p.Protocol], "duplicate native profile %s", p.Protocol)
		profiles[p.Protocol] = true
	}
	aliases := map[string]string{"TLS": "tls", "SMB": "smb2", "SOCKS": "socks5", "VNC": "rfb", "RTCP": "rtp", "STUN": "stun/turn", "NBT-DG": "nbt-dg", "ONC-RPC": "onc-rpc", "MDNS": "mdns", "FTP-DATA": "ftp-data"}
	// Public-corpus replay exposed these false native capability declarations.
	// Keep the legacy rules available, but do not turn a port hint into a claim
	// that native capture/session dispatch exists. Remove an exception only when
	// its real ingress and message/connection tests have been implemented.
	catalogOnly := map[string]string{"ajp": "application-layer/ajp.yaml", "bgp": "bgp.yaml", "rtmp": "rtmp.yaml"}
	seenCatalogOnly := map[string]bool{}
	for _, ports := range []map[uint16]string{tcpPortProtocols, udpPortProtocols} {
		for port, label := range ports {
			name := strings.ToLower(label)
			if a := aliases[label]; a != "" {
				name = a
			}
			if rule := catalogOnly[name]; rule != "" {
				require.False(t, profiles[name], "catalog-only protocol %s must not advertise native ingress", name)
				data, err := rules.RuleFS.ReadFile(rule)
				require.NoError(t, err)
				require.NotEmpty(t, data)
				seenCatalogOnly[name] = true
				continue
			}
			require.True(t, profiles[name], "port %d hint %s has no declared native scope", port, label)
		}
	}
	require.Len(t, seenCatalogOnly, len(catalogOnly))
}

func TestExplicitUDPDecodeAsValidatesBeforeCaptureAndStillRejectsGarbage(t *testing.T) {
	for _, value := range []string{"tcp:55001:ike", "udp:0:ike", "udp:65536:ike", "udp:55001:unknown"} {
		var out bytes.Buffer
		require.Error(t, invoke(&out, "--decode-as", value, "-r", "unused.pcap", "--json"))
	}
	w := []byte{6, 0, 255, 6, 0, 0, 17, 190, 128, 0, 0, 0}
	path := fixture(t, "decode-as.pcap", udpPacket(t, 55001, w), udpPacket(t, 55001, []byte("garbage")))
	var out bytes.Buffer
	require.NoError(t, invoke(&out, "-r", path, "--decode-as", "udp:55001:ipmi", "--json"))
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 2)
	var good, bad packetSummary
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &good))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &bad))
	require.Equal(t, "IPMI", good.Protocol)
	require.Equal(t, "UDP", bad.Protocol)
}

func TestControlWireFieldByteSourcesAndProgression(t *testing.T) {
	w := make([]byte, 148)
	w[0] = 1
	w[4] = 3
	e := &pcaputil.ProtocolEvent{Protocol: "wireguard", Status: "decoded", Raw: w, Session: map[string]any{"Packet Name": "Handshake Initiation", "Sender Index": uint64(3), "Content Visibility": "encrypted", "Authentication Verified": false, "Wire Field Ranges": map[string]any{"Sender Index": []any{uint64(4), uint64(8)}}}}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"Loopback", "IPv4", "UDP"}})
	require.Equal(t, "WireGuard", row.Protocol)
	require.Equal(t, []string{"Loopback", "IPv4", "UDP", "WireGuard"}, row.ProtocolStack)
	require.Equal(t, "encrypted", row.Visibility)
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	found := false
	for _, f := range fields {
		require.NotContains(t, f.text, "Wire Field Ranges")
		if strings.HasPrefix(f.text, "Sender Index:") {
			require.True(t, f.hasRange)
			require.Equal(t, []byte{3, 0, 0, 0}, sources[f.source-1].Data[f.start:f.end])
			found = true
		}
	}
	require.True(t, found)
}

func TestControlProfileLabelsAndNBSSProgression(t *testing.T) {
	for _, tc := range []struct{ protocol, label, transport string }{
		{"ike", "IKEv2", "UDP"}, {"wireguard", "WireGuard", "UDP"},
		{"rip", "RIP", "UDP"}, {"nbt-dg", "NBT-DG", "UDP"},
		{"onc-rpc", "ONC-RPC", "UDP"}, {"l2tp", "L2TP", "UDP"},
		{"openvpn", "OpenVPN", "UDP"}, {"ipmi", "IPMI", "UDP"},
		{"telnet", "Telnet", "TCP"}, {"ssh", "SSH", "TCP"}, {"nbss", "NBSS", "TCP"},
	} {
		e := &pcaputil.ProtocolEvent{Protocol: tc.protocol, Status: "decoded", Session: map[string]any{"Packet Name": "Observed control", "Version": uint64(32)}}
		base := packetSummary{Protocol: tc.transport, Layers: []string{"Ethernet", "IPv4", tc.transport}}
		row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(base)
		require.Equal(t, tc.label, row.Protocol, tc.protocol)
		require.Equal(t, []string{"Ethernet", "IPv4", tc.transport, tc.label}, row.ProtocolStack, tc.protocol)
	}
	e := &pcaputil.ProtocolEvent{Protocol: "smb2", Status: "decoded", Session: map[string]any{"Packet Name": "NEGOTIATE", "Carrier": "NetBIOS Session Service"}}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TCP", Layers: []string{"Ethernet", "IPv4", "TCP"}})
	require.Equal(t, []string{"Ethernet", "IPv4", "TCP", "NBSS", "SMB2"}, row.ProtocolStack)
}
