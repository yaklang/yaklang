package yakgrpc

import (
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// The node shape is intentionally identical to the original ParseTraffic tree.
// Scope is an optional bit interval [start,end) in this tree's own byte source.
type trafficField struct {
	Name         string `json:"name"`
	DisplayValue string `json:"display_value,omitempty"`
	Value        any    `json:"value"`
}

type trafficFieldValue struct {
	Leaf    bool   `json:"leaf"`
	Verbose string `json:"verbose"`
	Scope   []int  `json:"scope,omitempty"`
}

type trafficPacketDetails struct {
	Result           []trafficField
	Messages         []trafficMessageDetails
	ProtocolsIndexed bool
	DecodeError      string `json:",omitempty"`
}

type trafficMessageDetails struct {
	MessageId           uint
	Protocol            string
	Info                string
	Status              string
	Completeness        string
	DataLength          int
	PacketIds           []uint64
	CompletedInPacketId uint64
	Kind                string
	RAW                 []byte
	Result              []trafficField
	DecodeError         string `json:",omitempty"`
}

func pcapTrafficDetails(handle *pcapdb.DatabaseHandle, packet pcapdb.Packet, raw []byte) (*trafficPacketDetails, error) {
	meta, err := handle.Metadata()
	if err != nil {
		return nil, err
	}
	result := &trafficPacketDetails{
		Result: trafficTreeFromDisplayNodes(pcaputil.BuildFrameDisplayNodes(pcaputil.FrameDisplayInput{
			Raw: raw, LinkType: packet.LinkType, PacketID: uint64(packet.ID),
			CapturedLength: packet.CapturedLength, OriginalLength: packet.OriginalLength,
			Section: packet.Section, Interface: packet.Interface, TimestampNS: packet.TimestampNS,
		})),
		Messages:         make([]trafficMessageDetails, 0),
		ProtocolsIndexed: meta.ProtocolsIndexed,
		DecodeError:      packet.DecodeError,
	}
	messages, err := handle.PacketMessages(packet.ID, 1<<20)
	if err != nil {
		return nil, err
	}
	for _, m := range messages {
		source, err := pcapdb.MessageByteSource(m)
		if err != nil {
			return nil, err
		}
		protocol := strings.ToUpper(m.Protocol)
		view := trafficMessageDetails{MessageId: m.ID, Protocol: protocol, Info: m.Summary, Status: m.Status, Completeness: m.Completeness, DataLength: m.DataLength, Kind: source.Kind, RAW: m.Data,
			PacketIds: make([]uint64, 0, len(source.PacketRefs)), Result: trafficTreeFromDisplayNodes([]pcaputil.DisplayNode{{
				Name: protocol, DisplayValue: m.Summary, Children: pcaputil.BuildProtocolFieldDisplayNodes(m.Fields),
			}}), DecodeError: m.DecodeError}
		if view.DecodeError == "" {
			view.DecodeError = m.Error
		}
		for _, ref := range source.PacketRefs {
			view.PacketIds = append(view.PacketIds, ref.Number)
		}
		if m.CompletedInPacketID > 0 {
			view.CompletedInPacketId = uint64(m.CompletedInPacketID)
		} else if source.CompletedIn != nil {
			view.CompletedInPacketId = source.CompletedIn.Number
		}
		result.Messages = append(result.Messages, view)
	}
	return result, nil
}

func trafficTreeFromDisplayNodes(nodes []pcaputil.DisplayNode) []trafficField {
	result := make([]trafficField, 0, len(nodes))
	for _, node := range nodes {
		field := trafficField{Name: node.Name, DisplayValue: node.DisplayValue}
		if len(node.Children) > 0 {
			field.Value = trafficTreeFromDisplayNodes(node.Children)
		} else {
			scope := make([]int, 0, 2)
			if len(node.Ranges) > 0 && node.Ranges[0].BitOffset >= 0 && node.Ranges[0].BitLength > 0 {
				scope = append(scope, int(node.Ranges[0].BitOffset), int(node.Ranges[0].BitOffset+node.Ranges[0].BitLength))
			}
			field.Value = trafficFieldValue{Leaf: true, Verbose: node.DisplayValue, Scope: scope}
		}
		result = append(result, field)
	}
	return result
}
