package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func espNATTPresentationFrame(t *testing.T, version int, wire []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	udp := &layers.UDP{SrcPort: 45000, DstPort: 4500}
	ip4 := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}
	var ip gopacket.SerializableLayer = ip4
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip4))
	if version == 6 {
		eth.EthernetType = layers.EthernetTypeIPv6
		ip6 := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolUDP, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
		ip = ip6
		require.NoError(t, udp.SetNetworkLayerForChecksum(ip6))
	}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(wire)))
	return bytes.Clone(b.Bytes())
}

func TestESPNATTProgressionAndByteSourcesSurviveWindowsWorkerIPC(t *testing.T) {
	for _, version := range []int{4, 6} {
		t.Run(fmt.Sprintf("IPv%d", version), func(t *testing.T) {
			w := bytes.Clone(espPresentationEvent().Raw)
			frame := espNATTPresentationFrame(t, version, w)
			ci := gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), Length: len(frame), CaptureLength: len(frame)}
			var input bytes.Buffer
			writer := pcapgo.NewWriter(&input)
			require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
			require.NoError(t, writer.WritePacket(ci, frame))
			var events []*pcaputil.ProtocolEvent
			require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) })))
			require.Len(t, events, 1)
			e := events[0]
			require.True(t, validESPHeaderObservation(e))
			before, err := json.Marshal(e)
			require.NoError(t, err)
			raw := &capturedPacket{number: 1, data: frame, ci: ci, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
			row := summarize(raw)
			require.Equal(t, "ESP", row.Protocol)
			require.Equal(t, "udp", row.Transport)
			require.Equal(t, []string{"Ethernet", fmt.Sprintf("IPv%d", version), "UDP", "ESP"}, row.ProtocolStack)
			require.Equal(t, "observed clear ESP header and opaque remainder", row.ProtocolScope)
			require.Equal(t, "SA context unavailable; authentication, decryption and replay not verified", row.ProtocolEvidence)
			require.Equal(t, "headers", row.Visibility)
			require.Equal(t, "limited", row.ProtocolStatus)
			require.Equal(t, e.Summary, row.Info)
			require.Contains(t, row.Info, "UDP encapsulation")
			require.Empty(t, row.ConnectionProtocol)
			require.Zero(t, row.StreamID)
			for _, got := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw), workerMetadataDetail(t, events, true)} {
				require.Equal(t, "ESP", got.protocol)
				require.NotEmpty(t, got.sources)
				text := detailLines(got.fields)
				require.Contains(t, text, "Authentication Verified: false")
				require.Contains(t, text, "Algorithm Context Available: false")
				seen := map[string]bool{}
				for _, f := range got.fields {
					for key, span := range map[string][2]int{"SPI:": {0, 4}, "Sequence:": {4, 8}, "Opaque Body:": {8, len(w)}} {
						if strings.HasPrefix(f.text, key) {
							seen[key] = true
							require.True(t, f.hasRange)
							require.Positive(t, f.source, "ESP ranges belong to the UDP payload source, not the physical packet prefix")
							require.Equal(t, span[0], f.start)
							require.Equal(t, span[1], f.end)
							require.Equal(t, w[span[0]:span[1]], got.sources[f.source-1].Data[f.start:f.end])
						}
					}
				}
				require.Len(t, seen, 3)
			}
			after, err := json.Marshal(e)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestESPNATTCarrierEvidenceCannotBeForgedByPortOrProfileLabels(t *testing.T) {
	valid := func() *pcaputil.ProtocolEvent {
		e := espPresentationEvent()
		e.Transport, e.Profile, e.Admission = "udp", "esp-natt-observed-header", "observed-UDP4500-nonzero-SPI"
		e.Rule, e.Source, e.Destination = "ipsec_esp", "192.0.2.1:45000", "192.0.2.2:4500"
		return e
	}
	mutations := []func(*pcaputil.ProtocolEvent){
		func(e *pcaputil.ProtocolEvent) { e.Transport = "network" },
		func(e *pcaputil.ProtocolEvent) { e.Profile = "esp-observed-header" },
		func(e *pcaputil.ProtocolEvent) { e.Admission = "port-hint" },
		func(e *pcaputil.ProtocolEvent) { e.Rule = "" },
		func(e *pcaputil.ProtocolEvent) { e.Entry = "" },
		func(e *pcaputil.ProtocolEvent) { e.Destination = "192.0.2.2:500" },
		func(e *pcaputil.ProtocolEvent) { e.Source = "unknown:4500" },
		func(e *pcaputil.ProtocolEvent) { e.Source = "192.0.2.1:0" },
		func(e *pcaputil.ProtocolEvent) { e.Destination = "[2001:db8::2]:65536" },
		func(e *pcaputil.ProtocolEvent) { e.Session["Authentication Verified"] = true },
		func(e *pcaputil.ProtocolEvent) { e.Raw = e.Raw[:11] },
		func(e *pcaputil.ProtocolEvent) { e.Raw[0] = 0 },
		func(e *pcaputil.ProtocolEvent) { e.SourceBytes.SHA256 = "" },
	}
	for i, mutate := range mutations {
		e := valid()
		mutate(e)
		require.False(t, validESPHeaderObservation(e), "control %d", i)
		require.Empty(t, eventProtocols([]*pcaputil.ProtocolEvent{e}), "control %d", i)
		require.NotEqual(t, "headers", presentEvents([]*pcaputil.ProtocolEvent{e}).visibility)
	}
	e := valid()
	e.Admission, e.Destination = "explicit-decode-as", "[2001:db8::2]:55001"
	require.True(t, validESPHeaderObservation(e))
	e.Raw = e.Raw[:8]
	e.Session["Opaque Body"] = []byte{}
	e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(e.Raw))
	require.False(t, validESPHeaderObservation(e), "NAT-T does not inherit the direct-IP eight-byte minimum")
}
