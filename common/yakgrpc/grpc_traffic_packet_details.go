package yakgrpc

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
)

// The node shape is intentionally identical to the original ParseTraffic tree.
// Scope is an optional bit interval [start,end) in this tree's own byte source.
type trafficField struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
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

func trafficLeaf(name string, value any, scope ...int) trafficField {
	verbose := fmt.Sprint(value)
	if b, ok := value.([]byte); ok {
		verbose = "0x" + hex.EncodeToString(b)
	}
	return trafficField{Name: name, Value: trafficFieldValue{Leaf: true, Verbose: verbose, Scope: scope}}
}

// Saved application fields have no reliable physical-frame offsets. Do not
// invent offsets by reparsing an HTTP message or assigning them to a TCP frame.
func trafficSavedFields(fields map[string]any) []trafficField {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make([]trafficField, 0, len(keys))
	for _, key := range keys {
		result = append(result, trafficSavedField(key, fields[key]))
	}
	return result
}

func trafficSavedField(name string, value any) trafficField {
	switch v := value.(type) {
	case map[string]any:
		return trafficField{Name: name, Value: trafficSavedFields(v)}
	case []any:
		children := make([]trafficField, 0, len(v))
		for i, x := range v {
			children = append(children, trafficSavedField(fmt.Sprintf("[%d]", i), x))
		}
		return trafficField{Name: name, Value: children}
	default:
		return trafficLeaf(name, value)
	}
}

func pcapTrafficDetails(handle *pcapdb.DatabaseHandle, packet pcapdb.Packet, raw []byte) (*trafficPacketDetails, error) {
	meta, err := handle.Metadata()
	if err != nil {
		return nil, err
	}
	result := &trafficPacketDetails{Result: trafficFrameFields(packet, raw), Messages: make([]trafficMessageDetails, 0), ProtocolsIndexed: meta.ProtocolsIndexed, DecodeError: packet.DecodeError}
	messages, err := handle.PacketMessages(packet.ID, 1<<20)
	if err != nil {
		return nil, err
	}
	for _, m := range messages {
		source, err := pcapdb.MessageByteSource(m)
		if err != nil {
			return nil, err
		}
		view := trafficMessageDetails{MessageId: m.ID, Protocol: strings.ToUpper(m.Protocol), Info: m.Summary, Status: m.Status, Completeness: m.Completeness, DataLength: m.DataLength, Kind: source.Kind, RAW: m.Data,
			PacketIds: make([]uint64, 0, len(source.PacketRefs)), Result: []trafficField{{Name: strings.ToUpper(m.Protocol), Value: trafficSavedFields(m.Fields)}}, DecodeError: m.DecodeError}
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

func trafficFrameFields(packet pcapdb.Packet, raw []byte) []trafficField {
	frame := []trafficField{trafficLeaf("Number", packet.ID), trafficLeaf("Captured length", packet.CapturedLength), trafficLeaf("Original length", packet.OriginalLength), trafficLeaf("Link type", packet.LinkType), trafficLeaf("Section", packet.Section), trafficLeaf("Interface", packet.Interface)}
	if packet.TimestampNS != nil {
		frame = append(frame, trafficLeaf("Timestamp (ns)", *packet.TimestampNS))
	}
	result := []trafficField{{Name: fmt.Sprintf("Frame %d: %d bytes on wire, %d bytes captured", packet.ID, packet.OriginalLength, packet.CapturedLength), Value: frame}}
	decoded := gopacket.NewPacket(raw, layers.LinkType(packet.LinkType), gopacket.Default)
	offset := 0
	for _, layer := range decoded.Layers() {
		if layer.LayerType() == gopacket.LayerTypePayload || layer.LayerType() == gopacket.LayerTypeDecodeFailure {
			continue
		}
		header := layer.LayerContents()
		if len(header) == 0 {
			continue
		}
		start := offset * 8
		leaf := func(name string, value any, first, last int) trafficField {
			return trafficLeaf(name, value, start+first*8, start+last*8)
		}
		var fields []trafficField
		switch v := layer.(type) {
		case *layers.Ethernet:
			fields = []trafficField{leaf("Destination", v.DstMAC, 0, 6), leaf("Source", v.SrcMAC, 6, 12), leaf("Type", v.EthernetType, 12, 14)}
		case *layers.Dot1Q:
			fields = []trafficField{leaf("Priority", v.Priority, 0, 2), leaf("Drop eligible", v.DropEligible, 0, 2), leaf("VLAN ID", v.VLANIdentifier, 0, 2), leaf("Type", v.Type, 2, 4)}
		case *layers.IPv4:
			fields = []trafficField{leaf("Version", v.Version, 0, 1), leaf("Header length", int(v.IHL)*4, 0, 1), leaf("Type of service", v.TOS, 1, 2), leaf("Total length", v.Length, 2, 4), leaf("Identification", v.Id, 4, 6), leaf("Flags", v.Flags, 6, 8), leaf("Fragment offset", v.FragOffset, 6, 8), leaf("Time to live", v.TTL, 8, 9), leaf("Protocol", v.Protocol, 9, 10), leaf("Checksum", fmt.Sprintf("0x%04x", v.Checksum), 10, 12), leaf("Source address", v.SrcIP, 12, 16), leaf("Destination address", v.DstIP, 16, 20)}
			if len(header) > 20 {
				fields = append(fields, leaf("Options", header[20:], 20, len(header)))
			}
		case *layers.IPv6:
			fields = []trafficField{leaf("Version", v.Version, 0, 1), leaf("Traffic class", v.TrafficClass, 0, 2), leaf("Flow label", v.FlowLabel, 1, 4), leaf("Payload length", v.Length, 4, 6), leaf("Next header", v.NextHeader, 6, 7), leaf("Hop limit", v.HopLimit, 7, 8), leaf("Source address", v.SrcIP, 8, 24), leaf("Destination address", v.DstIP, 24, 40)}
		case *layers.TCP:
			flags := []string{}
			for _, f := range []struct {
				set  bool
				name string
			}{{v.FIN, "FIN"}, {v.SYN, "SYN"}, {v.RST, "RST"}, {v.PSH, "PSH"}, {v.ACK, "ACK"}, {v.URG, "URG"}, {v.ECE, "ECE"}, {v.CWR, "CWR"}, {v.NS, "NS"}} {
				if f.set {
					flags = append(flags, f.name)
				}
			}
			fields = []trafficField{leaf("Source port", uint16(v.SrcPort), 0, 2), leaf("Destination port", uint16(v.DstPort), 2, 4), leaf("Sequence number", v.Seq, 4, 8), leaf("Acknowledgment number", v.Ack, 8, 12), leaf("Header length", int(v.DataOffset)*4, 12, 13), leaf("Flags", strings.Join(flags, ", "), 12, 14), leaf("Window", v.Window, 14, 16), leaf("Checksum", fmt.Sprintf("0x%04x", v.Checksum), 16, 18), leaf("Urgent pointer", v.Urgent, 18, 20)}
			if len(header) > 20 {
				fields = append(fields, leaf("Options", header[20:], 20, len(header)))
			}
		case *layers.UDP:
			fields = []trafficField{leaf("Source port", uint16(v.SrcPort), 0, 2), leaf("Destination port", uint16(v.DstPort), 2, 4), leaf("Length", v.Length, 4, 6), leaf("Checksum", fmt.Sprintf("0x%04x", v.Checksum), 6, 8)}
		default:
			// All other decoded link/network headers remain inspectable. Their
			// field offsets are unknown; only expose the verified header range.
			value := reflect.ValueOf(layer)
			if value.Kind() == reflect.Pointer {
				value = value.Elem()
			}
			if value.Kind() == reflect.Struct {
				typ := value.Type()
				for i := 0; i < value.NumField(); i++ {
					field := typ.Field(i)
					if field.PkgPath != "" || field.Anonymous || !value.Field(i).CanInterface() {
						continue
					}
					fields = append(fields, trafficLeaf(field.Name, value.Field(i).Interface()))
				}
			}
		}
		fields = append(fields, leaf("Header bytes", header, 0, len(header)))
		result = append(result, trafficField{Name: layer.LayerType().String(), Value: fields})
		offset += len(header)
	}
	if offset < len(raw) {
		result = append(result, trafficLeaf("Payload", raw[offset:], offset*8, len(raw)*8))
	}
	return result
}
