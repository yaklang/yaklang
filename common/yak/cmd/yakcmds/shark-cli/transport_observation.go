package sharkcli

import (
	"bytes"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// This is a header observation, independent of application admission and TCP
// sequence validation. Layers refer to the decoded network reassembly; offsets
// into the physical capture record are deliberately not fabricated.
type packetTransportView struct {
	Header          []byte                     `json:"-"`
	HeaderSHA256    string                     `json:"header_sha256"`
	Protocol        string                     `json:"protocol"`
	Source          string                     `json:"source"`
	Destination     string                     `json:"destination"`
	SourcePort      uint16                     `json:"source_port"`
	DestinationPort uint16                     `json:"destination_port"`
	Domain          pcaputil.CaptureDomain     `json:"domain"`
	PacketRefs      []pcaputil.PacketReference `json:"packet_refs"`
	Layers          []string                   `json:"layers"`
	LayersComplete  bool                       `json:"layers_complete"`
	HeaderLength    int                        `json:"header_length"`
	PayloadLength   int                        `json:"payload_length"`
	Sequence        uint32                     `json:"sequence,omitempty"`
	Acknowledgment  uint32                     `json:"acknowledgment,omitempty"`
	TCPFlags        uint16                     `json:"tcp_flags,omitempty"`
	Info            string                     `json:"info"`
	Evidence        string                     `json:"evidence"`
}

func (p *capturedPacket) observeTransportHeader(packet gopacket.Packet) {
	if packet == nil || packet.ErrorLayer() != nil {
		return
	}
	refs := pcaputil.CapturePacketReferences(packet)
	matched := false
	for _, ref := range refs {
		if ref.Number == p.reference.Number && ref.Domain.Section == p.reference.Domain.Section && ref.Domain.Interface == p.reference.Domain.Interface {
			matched = true
			break
		}
	}
	if !matched {
		return
	}
	network, transport := pcaputil.CaptureTransport(packet)
	if network == nil || transport == nil {
		return
	}
	switch ip := network.(type) {
	case *layers.IPv4:
		if ip.Version != 4 || ip.SrcIP.To4() == nil || ip.DstIP.To4() == nil || ip.FragOffset != 0 || ip.Flags&layers.IPv4MoreFragments != 0 {
			return
		}
	case *layers.IPv6:
		if ip.Version != 6 || ip.SrcIP.To16() == nil || ip.DstIP.To16() == nil {
			return
		}
	default:
		return
	}
	v := &packetTransportView{Source: network.NetworkFlow().Src().String(), Destination: network.NetworkFlow().Dst().String(), Domain: pcaputil.CapturePacketEvidence(packet).Domain, PacketRefs: refs, Evidence: "complete observed transport header; application identity and TCP sequence acceptance are separate"}
	switch t := transport.(type) {
	case *layers.TCP:
		if t.DataOffset < 5 || len(t.LayerContents()) != int(t.DataOffset)*4 {
			return
		}
		v.Protocol, v.SourcePort, v.DestinationPort = "tcp", uint16(t.SrcPort), uint16(t.DstPort)
		v.HeaderLength, v.PayloadLength = len(t.LayerContents()), len(t.Payload)
		v.Sequence, v.Acknowledgment = t.Seq, t.Ack
		v.TCPFlags = uint16(t.LayerContents()[12]&15)<<8 | uint16(t.LayerContents()[13])
		v.Info = fmt.Sprintf("%d → %d TCP header Seq=%d Ack=%d Payload=%d · sequence acceptance separate", v.SourcePort, v.DestinationPort, t.Seq, t.Ack, len(t.Payload))
	case *layers.UDP:
		if len(t.LayerContents()) != 8 || t.Length != 0 && t.Length < 8 {
			return
		}
		v.Protocol, v.SourcePort, v.DestinationPort = "udp", uint16(t.SrcPort), uint16(t.DstPort)
		v.HeaderLength, v.PayloadLength = 8, len(t.Payload)
		v.Info = fmt.Sprintf("%d → %d UDP header Payload=%d · application identity unconfirmed", v.SourcePort, v.DestinationPort, len(t.Payload))
	default:
		return
	}
	v.Layers, v.LayersComplete = pcaputil.CaptureTransportLayers(packet)
	v.Header = bytes.Clone(transport.LayerContents())
	p.transportView = v
}
