package pcaputil

import (
	"bytes"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCaptureTransportLayersSurviveFragmentedTunnelNormalization(t *testing.T) {
	for _, tc := range []struct {
		profile string
		want    []string
	}{
		{"ipip", []string{"Ethernet", "IPv4", "IPv4", "UDP"}},
		{"6in4", []string{"Ethernet", "IPv4", "IPv6", "UDP"}},
		{"geneve", []string{"Ethernet", "IPv4", "UDP", "Geneve", "Ethernet", "IPv4", "UDP"}},
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(tc.profile+map[bool]string{false: "/ordered", true: "/reversed"}[reverse], func(t *testing.T) {
				frame := tunnelAdmissionWire(t, tc.profile, 5013, "198.51.100.1", false, 1, false)
				pieces := icmpFragmentFrames(t, frame, 32, false)
				if reverse {
					pieces[0], pieces[1] = pieces[1], pieces[0]
				}
				wire := icmpFragmentCapture(t, pieces)
				calls := 0
				var stats ProtocolStats
				require.NoError(t, ReplayPcap(bytes.NewReader(wire), WithOnProtocolMessage(func(*ProtocolEvent) {}), WithBeforeTransportPacket(func(p gopacket.Packet) {
					n, tr := CaptureTransport(p)
					require.NotNil(t, n)
					require.IsType(t, &layers.UDP{}, tr)
					got, complete := CaptureTransportLayers(p)
					require.True(t, complete)
					require.Equal(t, tc.want, got)
					require.Len(t, CapturePacketReferences(p), len(pieces))
					got[0] = "caller edited"
					again, _ := CaptureTransportLayers(p)
					require.Equal(t, tc.want, again)
					calls++
				}), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
				require.Equal(t, 1, calls)
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
			})
		}
	}
}

func TestCaptureTransportLayerProjectionLimitAndCopyOnWrite(t *testing.T) {
	got, complete := CaptureTransportLayers(nil)
	require.Nil(t, got)
	require.False(t, complete)
	got, complete = CaptureTransportLayers(gopacket.NewPacket([]byte{1, 2}, layers.LayerTypeIPv4, gopacket.Default))
	require.Nil(t, got)
	require.False(t, complete, "a missing transport cannot produce a complete header path")
	p := gopacket.NewPacket(tunnelAdmissionWire(t, "ipip", 1, "198.51.100.1", false, 1, false), CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	first := captureEvidence{networkPrefix: make([]gopacket.LayerType, 1, 8)}
	first.networkPrefix[0] = layers.LayerTypeEthernet
	copyEvidence := first
	copyEvidence.preserveNetworkPrefix(p, p.NetworkLayer())
	require.Len(t, first.networkPrefix, 1)
	require.Equal(t, gopacket.LayerType(0), first.networkPrefix[:cap(first.networkPrefix)][1], "extending a later observation cannot alter retained metadata")
	e := captureEvidence{networkPrefix: make([]gopacket.LayerType, maxCaptureTransportLayers)}
	for i := range e.networkPrefix {
		e.networkPrefix[i] = layers.LayerTypeDot1Q
	}
	p.Metadata().CaptureInfo = withEvidence(p.Metadata().CaptureInfo, e)
	got, complete = CaptureTransportLayers(p)
	require.False(t, complete)
	require.Len(t, got, maxCaptureTransportLayers)
	// A display projection limit does not rewrite the packet or its domains.
	require.NotNil(t, p.NetworkLayer())
	require.NotNil(t, p.TransportLayer())
}
