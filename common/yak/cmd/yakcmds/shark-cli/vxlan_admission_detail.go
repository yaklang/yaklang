package sharkcli

import (
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// The legacy observer deliberately exposes speculative layers. A packet may
// reach an inspector without its native event (including a bounded-history or
// IPC projection marker), so those layers cannot supply fallback identity.
// Keep this check independent of the speculative VXLAN/child decode: use the
// selected UDP destination, its bounded payload and the literal fixed header.
type vxlanInspectionBoundary struct {
	prefix               []gopacket.Layer
	network              gopacket.NetworkLayer
	reason               string
	payloadStart, length int
	trustedUDP           bool
}

func needsVXLANInspectionFallback(raw *capturedPacket, events []*pcaputil.ProtocolEvent) bool {
	if raw.transportView != nil {
		return false // Native network reassembly owns this proven logical header.
	}
	for _, event := range events {
		if event != nil && event.ExpertCode != string(pcaputil.ErrResourceExceeded) {
			return false // Preserve the native profile/admission decision when retained.
		}
	}
	return true
}

func inspectionVXLANBoundary(p gopacket.Packet) *vxlanInspectionBoundary {
	var network gopacket.NetworkLayer
	offset := 0
	decoded := p.Layers()
	for i, layer := range decoded {
		if layer.LayerType() == layers.LayerTypeICMPv4 || layer.LayerType() == layers.LayerTypeICMPv6 {
			return nil // A quoted original datagram is not a captured tunnel.
		}
		if n, ok := layer.(gopacket.NetworkLayer); ok {
			network = n
		}
		offset += len(layer.LayerContents())
		udp, ok := layer.(*layers.UDP)
		if !ok || udp.DstPort.LayerType() != layers.LayerTypeVXLAN {
			continue // Source-only hints and the separate GPE profile do not select Ethernet.
		}
		reason, trusted := "", true
		if len(udp.LayerContents()) != 8 || udp.Length < 8 || int(udp.Length) != 8+len(udp.Payload) {
			reason, trusted = "UDP carrier bounds unavailable; VXLAN child identity unconfirmed", false
		} else {
			switch {
			case len(udp.Payload) < 8:
				reason = fmt.Sprintf("VXLAN header: incomplete fixed header: %d of 8 bytes available; inner identity unconfirmed", len(udp.Payload))
			case udp.Payload[0]&8 == 0:
				reason = "VXLAN header: I bit is clear; VNI is not valid; inner identity unconfirmed"
			case len(udp.Payload)-8 < 14:
				reason = fmt.Sprintf("VXLAN header: Ethernet child prefix incomplete: %d of 14 bytes available; inner identity unconfirmed", len(udp.Payload)-8)
			default:
				continue
			}
		}
		return &vxlanInspectionBoundary{prefix: decoded[:i+1], network: network, payloadStart: offset, length: len(udp.Payload), reason: reason, trustedUDP: trusted}
	}
	return nil
}

func (b *vxlanInspectionBoundary) apply(row packetSummary) packetSummary {
	row.Protocol, row.Info = "UDP", b.reason
	row.Layers = nil
	for i, layer := range b.prefix {
		if i == len(b.prefix)-1 {
			// Only the terminal UDP payload failed admission. Earlier UDP
			// headers may carry already admitted outer VXLAN envelopes.
			break
		}
		row.Layers = append(row.Layers, layer.LayerType().String())
		row.Protocol = layer.LayerType().String()
	}
	if b.network != nil {
		row.Source, row.Destination = b.network.NetworkFlow().Src().String(), b.network.NetworkFlow().Dst().String()
		for _, layer := range b.prefix {
			if n, ok := layer.(gopacket.NetworkLayer); ok {
				if n != b.network {
					row.PhysicalSource, row.PhysicalDestination = n.NetworkFlow().Src().String(), n.NetworkFlow().Dst().String()
				}
				break
			}
		}
	}
	row.ProtocolStack = append([]string(nil), row.Layers...)
	row.Transport, row.ProtocolStatus = "network", "limited"
	row.ProtocolScope, row.Visibility = "unselected VXLAN payload", "headers"
	row.TCPFlowID, row.TCPFlowDomain, row.TCPFlowState = 0, nil, ""
	row.MessageSource, row.MessageDestination, row.MessageTransport, row.MessageDomain = "", "", "", nil
	row.ConnectionProtocol, row.ConnectionStack, row.ConnectionEvidence = "", nil, ""
	row.StreamID, row.ConversationID = 0, 0
	row.packetProtocol, row.packetInfo = row.Protocol, row.Info
	return row
}

func (b *vxlanInspectionBoundary) appendFields(d *packetDetail, raw *capturedPacket) {
	offset := 0
	for _, layer := range b.prefix {
		start := offset
		end := offset + len(layer.LayerContents())
		d.fields = append(d.fields, field{text: layer.LayerType().String(), branch: true, start: offset, end: end, hasRange: end > offset && end <= len(raw.data)})
		offset = end
		if n, ok := layer.(gopacket.NetworkLayer); ok {
			d.fields = append(d.fields, field{depth: 1, text: "Source: " + n.NetworkFlow().Src().String()}, field{depth: 1, text: "Destination: " + n.NetworkFlow().Dst().String()})
		}
		if udp, ok := layer.(*layers.UDP); ok {
			d.fields = append(d.fields, field{depth: 1, text: fmt.Sprintf("Source port: %d; destination port: %d; length: %d", udp.SrcPort, udp.DstPort, udp.Length)})
		}
		if v, ok := layer.(*layers.VXLAN); ok && v.ValidIDFlag && end-start == 8 && end <= len(raw.data) {
			d.fields = append(d.fields, field{depth: 1, text: fmt.Sprintf("Admitted outer VNI: %d", v.VNI), start: start + 4, end: start + 7, hasRange: true})
		}
	}
	d.fields = append(d.fields, field{text: b.reason})
	if b.payloadStart >= 0 && b.length > 0 && b.payloadStart <= len(raw.data)-b.length {
		d.fields = append(d.fields, field{text: "Unselected VXLAN payload; no admitted child fields", start: b.payloadStart, end: b.payloadStart + b.length, hasRange: true})
		if b.trustedUDP {
			d.fields = append(d.fields, field{depth: 1, text: fmt.Sprintf("Observed flags: 0x%02x; I: %t", raw.data[b.payloadStart], raw.data[b.payloadStart]&8 != 0), start: b.payloadStart, end: b.payloadStart + 1, hasRange: true})
		}
	}
}
