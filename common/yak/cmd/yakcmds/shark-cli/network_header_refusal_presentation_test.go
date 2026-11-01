package sharkcli

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
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

func refusalPresentationWire(t *testing.T, version int, next layers.IPProtocol, payload []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv6}
	var ip gopacket.SerializableLayer = &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: next, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
	if version == 4 {
		eth.EthernetType = layers.EthernetTypeIPv4
		ip = &layers.IPv4{Version: 4, TTL: 64, Protocol: next, SrcIP: net.ParseIP("198.51.100.1").To4(), DstIP: net.ParseIP("198.51.100.2").To4()}
	}
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gopacket.Payload(payload)))
	return bytes.Clone(buffer.Bytes())
}

func refusalPresentationEvent(code string, names []string) *pcaputil.ProtocolEvent {
	return &pcaputil.ProtocolEvent{ID: 1, Protocol: "ip", Transport: "network", Profile: "network-header-refusal", Status: "malformed", Completeness: "malformed", ExpertCode: code, Error: "child header exceeds its parent boundary", Summary: "child header exceeds its parent boundary; child identity not admitted", Source: "unproved child source", Destination: "unproved child destination", Session: map[string]any{"Observed Header Layers": names, "Header Observation Scope": "validated network prefix before refused child header", "Child Header Identity": "unconfirmed", "Parent Header Refusal": code}}
}

func TestNativeMalformedHeaderRefusalAgreesAcrossPacketWorkerAndStream(t *testing.T) {
	w := []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 1, 1, 2, 3, 4, 5, 6, 7, 8}
	badOption := append([]byte{50, 0, 1, 7, 0, 0, 0, 0}, w...)
	badAH := append([]byte{50, 1, 0, 0, 0, 0, 0, 7, 0, 0, 0, 8}, w...)
	zeroTotal := refusalPresentationWire(t, 4, layers.IPProtocolESP, w)
	binary.BigEndian.PutUint16(zeroTotal[16:18], 0)
	for _, c := range []struct {
		code, label string
		wire        []byte
	}{
		{"IPv6OptionsHeaderBounds", "IPv6", refusalPresentationWire(t, 6, layers.IPProtocolIPv6HopByHop, badOption)},
		{"AHHeaderBounds", "IPv6", refusalPresentationWire(t, 6, layers.IPProtocolAH, badAH)},
		{"ESPParentLengthInvalid", "IPv4", zeroTotal},
	} {
		t.Run(c.code, func(t *testing.T) {
			ci := gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), Length: len(c.wire), CaptureLength: len(c.wire)}
			var input bytes.Buffer
			writer := pcapgo.NewWriter(&input)
			require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
			require.NoError(t, writer.WritePacket(ci, c.wire))
			var events []*pcaputil.ProtocolEvent
			require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true}), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) })))
			require.Len(t, events, 1)
			e := events[0]
			require.Equal(t, c.code, e.ExpertCode)
			require.Equal(t, "network-header-refusal", e.Profile)
			names, label, ok := networkHeaderRefusalPresentation(e)
			require.True(t, ok)
			require.Equal(t, c.label, label)
			raw := &capturedPacket{number: 1, data: c.wire, link: layers.LinkTypeEthernet, ci: ci, events: events, presentation: presentEvents(events)}
			row := summarize(raw)
			require.Equal(t, c.label, row.Protocol)
			require.Equal(t, names, row.ProtocolStack)
			require.Equal(t, "malformed-network-header", row.Visibility)
			require.Equal(t, "malformed", row.ProtocolStatus)
			require.Contains(t, row.Info, "child identity not admitted")
			require.Empty(t, row.ConnectionProtocol)
			require.Zero(t, row.StreamID)
			require.Zero(t, row.SourcePort)
			require.Zero(t, row.DestinationPort)
			if c.label == "IPv6" {
				require.Equal(t, "2001:db8::1", row.Source)
				require.Equal(t, "2001:db8::2", row.Destination)
				require.Equal(t, row.Source, row.PhysicalSource)
			} else {
				require.Empty(t, row.Source, "IPv4 total length zero cannot validate its address-shaped tail")
				require.Empty(t, row.PhysicalSource)
			}
			for _, got := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw)} {
				require.Equal(t, c.label, got.protocol)
				text := detailLines(got.fields)
				require.Contains(t, text, "Parent Header Refusal: "+c.code)
				require.NotContains(t, text, "YAML dissector")
				require.Empty(t, got.sources, "no refused child PDU byte source is manufactured")
				found := false
				for _, f := range got.fields {
					if f.text == "Source: 2001:db8::1" {
						found = true
						require.Zero(t, f.source)
						require.True(t, f.hasRange)
						require.Equal(t, 22, f.start)
						require.Equal(t, 38, f.end)
					}
				}
				require.Equal(t, c.label == "IPv6", found)
			}
			got := workerMetadataDetail(t, events, true)
			require.Equal(t, c.label, got.protocol)
			require.Contains(t, detailLines(got.fields), "Parent Header Refusal: "+c.code)
			require.NotContains(t, detailLines(got.fields), "Captured network prefix")
			for _, f := range got.fields {
				require.False(t, f.hasRange, "a stream-only diagnostic has no captured frame bytes")
			}
		})
	}
}

func TestNetworkHeaderRefusalPreservesFullValidatedPrefixAndIgnoresEventAddresses(t *testing.T) {
	w := []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 1}
	body := append([]byte{51, 0, 0, 0, 0, 0, 0, 0, 50, 1, 0, 0, 0, 0, 0, 7, 0, 0, 0, 8}, w...)
	inner := refusalPresentationWire(t, 6, layers.IPProtocolIPv6Destination, body)
	wire := endpointVXLANWire(t, "203.0.113.1", "203.0.113.2", 8, 42, inner)
	names := []string{"Ethernet", "IPv4", "UDP", "VXLAN", "Ethernet", "IPv6", "IPv6Destination"}
	var input bytes.Buffer
	writer := pcapgo.NewWriter(&input)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), Length: len(wire), CaptureLength: len(wire)}, wire))
	var actual []*pcaputil.ProtocolEvent
	require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true}), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { actual = append(actual, e) })))
	require.Len(t, actual, 1)
	require.Equal(t, names, actual[0].Session["Observed Header Layers"])
	require.Equal(t, "2001:db8::1", actual[0].Source)
	actualRaw := &capturedPacket{number: 1, data: wire, ci: gopacket.CaptureInfo{Length: len(wire), CaptureLength: len(wire)}, link: layers.LinkTypeEthernet, events: actual, presentation: presentEvents(actual)}
	actualRow := summarize(actualRaw)
	require.Equal(t, "2001:db8::1", actualRow.Source)
	require.Equal(t, "203.0.113.1", actualRow.PhysicalSource)
	for _, got := range []packetDetail{detail(actualRaw), workerMetadataDetailForPacket(t, actualRaw)} {
		found := false
		for _, field := range got.fields {
			if field.text == "Source: 2001:db8::1" && field.hasRange {
				found = true
				require.Zero(t, field.source)
				require.Equal(t, 72, field.start)
				require.Equal(t, 88, field.end)
			}
		}
		require.True(t, found)
	}
	e := refusalPresentationEvent("AHHeaderBounds", names)
	child := &pcaputil.ProtocolEvent{ID: 2, Protocol: "dns", Status: "decoded", Session: map[string]any{"QNAME": "unproved.invalid"}}
	for _, events := range [][]*pcaputil.ProtocolEvent{{e, child}, {child, e}} {
		before, err := json.Marshal(events)
		require.NoError(t, err)
		raw := &capturedPacket{number: 1, data: wire, ci: gopacket.CaptureInfo{Length: len(wire), CaptureLength: len(wire)}, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
		row := summarize(raw)
		require.Equal(t, "IPv6", row.Protocol)
		require.Equal(t, names, row.ProtocolStack)
		require.Equal(t, "2001:db8::1", row.Source)
		require.Equal(t, "2001:db8::2", row.Destination)
		require.Equal(t, "203.0.113.1", row.PhysicalSource)
		require.Equal(t, "203.0.113.2", row.PhysicalDestination)
		for _, got := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw), workerMetadataDetail(t, events, true)} {
			require.Equal(t, "IPv6", got.protocol)
			text := detailLines(got.fields)
			require.NotContains(t, text, "unproved.invalid")
		}
		for _, got := range []packetDetail{detail(raw), workerMetadataDetailForPacket(t, raw)} {
			found := false
			for _, f := range got.fields {
				if f.text == "Source: 2001:db8::1" && f.hasRange {
					found = true
					require.Zero(t, f.source)
					require.Equal(t, 72, f.start)
					require.Equal(t, 88, f.end)
				}
			}
			require.True(t, found)
		}
		after, err := json.Marshal(events)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
}

func TestNetworkHeaderRefusalRequiresCompleteCoherentContract(t *testing.T) {
	mutations := []func(*pcaputil.ProtocolEvent){
		func(e *pcaputil.ProtocolEvent) { e.Protocol = "esp" },
		func(e *pcaputil.ProtocolEvent) { e.Transport = "udp" },
		func(e *pcaputil.ProtocolEvent) { e.Profile = "" },
		func(e *pcaputil.ProtocolEvent) { e.Status = "limited" },
		func(e *pcaputil.ProtocolEvent) { e.Completeness = "headers" },
		func(e *pcaputil.ProtocolEvent) { e.ExpertCode = "ProtocolNotRecognized" },
		func(e *pcaputil.ProtocolEvent) { e.Session["Parent Header Refusal"] = "AHHeaderBounds" },
		func(e *pcaputil.ProtocolEvent) {
			e.Session["Parent Header Refusal"] = []byte("IPv6OptionsHeaderBounds")
		},
		func(e *pcaputil.ProtocolEvent) { delete(e.Session, "Header Observation Scope") },
		func(e *pcaputil.ProtocolEvent) { e.Session["Header Observation Scope"] = "complete decrypted message" },
		func(e *pcaputil.ProtocolEvent) { e.Session["Child Header Identity"] = "confirmed" },
		func(e *pcaputil.ProtocolEvent) { e.Session["Observed Header Layers"] = []string{"Ethernet", "IP"} },
		func(e *pcaputil.ProtocolEvent) {
			e.Session["Observed Header Layers"] = []string{"Ethernet", "IPv6", ""}
		},
		func(e *pcaputil.ProtocolEvent) { e.Session["Observed Header Layers"] = []any{"Ethernet", 6} },
		func(e *pcaputil.ProtocolEvent) {
			e.Session["Observed Header Layers"] = []string{"Ethernet", "IPv6", "IPSecESP"}
		},
		func(e *pcaputil.ProtocolEvent) {
			e.Session["Observed Header Layers"] = []string{"Ethernet", "IPv4", "ICMPv4", "IPv6"}
		},
		func(e *pcaputil.ProtocolEvent) {
			e.Session["Observed Header Layers"] = []string{"Ethernet", "IPv6", strings.Repeat("界", 43)}
		},
		func(e *pcaputil.ProtocolEvent) {
			e.Session["Observed Header Layers"] = []string{"Ethernet", "IPv6", "bad\x1bname"}
		},
		func(e *pcaputil.ProtocolEvent) { e.Session = nil },
	}
	for i, mutate := range mutations {
		e := refusalPresentationEvent("IPv6OptionsHeaderBounds", []string{"Ethernet", "IPv6"})
		mutate(e)
		_, _, ok := networkHeaderRefusalPresentation(e)
		require.False(t, ok, "contract control %d", i)
		require.False(t, presentEvents([]*pcaputil.ProtocolEvent{e}).networkRefused, "code alone is not native prefix evidence, control %d", i)
		child := &pcaputil.ProtocolEvent{Protocol: "dns", Status: "decoded"}
		got := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e, child}, true)
		require.Equal(t, "DNS", got.protocol, "other stream behavior remains unchanged, control %d", i)
	}
	_, _, ok := networkHeaderRefusalPresentation(nil)
	require.False(t, ok)
}

func TestNetworkHeaderRefusalDoesNotInventEndpointsFromPartialOrInvalidFixedIP(t *testing.T) {
	wire := refusalPresentationWire(t, 6, layers.IPProtocolAH, make([]byte, 28))
	for _, mutate := range []func([]byte) []byte{
		func(w []byte) []byte { return w[:14+39] },
		func(w []byte) []byte { w[14] = 0x40; return w },
	} {
		w := mutate(bytes.Clone(wire))
		e := refusalPresentationEvent("AHHeaderBounds", []string{"Ethernet", "IPv6"})
		raw := &capturedPacket{number: 1, data: w, ci: gopacket.CaptureInfo{Length: len(w), CaptureLength: len(w)}, link: layers.LinkTypeEthernet, events: []*pcaputil.ProtocolEvent{e}, presentation: presentEvents([]*pcaputil.ProtocolEvent{e})}
		row := summarize(raw)
		require.Empty(t, row.Source)
		require.Empty(t, row.Destination)
		require.Empty(t, row.PhysicalSource)
		for _, got := range []packetDetail{detail(raw), workerMetadataDetailForPacket(t, raw)} {
			require.NotContains(t, detailLines(got.fields), "Captured network prefix")
			require.NotContains(t, detailLines(got.fields), "Source: 2001:db8::1")
		}
	}
}

func TestNetworkHeaderOrderRefusalRequiresTheCompleteOwnedPrefixContract(t *testing.T) {
	e := refusalPresentationEvent("IPv6OptionsHeaderOrder", []string{"Ethernet", "IPv4"})
	names, label, ok := networkHeaderRefusalPresentation(e)
	require.True(t, ok)
	require.Equal(t, []string{"Ethernet", "IPv4"}, names)
	require.Equal(t, "IPv4", label)
	p := presentEvents([]*pcaputil.ProtocolEvent{e})
	require.True(t, p.networkRefused)
	require.Equal(t, "malformed-network-header", p.visibility)
	delete(e.Session, "Header Observation Scope")
	_, _, ok = networkHeaderRefusalPresentation(e)
	require.False(t, ok, "a header-order expert code alone cannot declare a validated parent")
	require.False(t, presentEvents([]*pcaputil.ProtocolEvent{e}).networkRefused)
}
