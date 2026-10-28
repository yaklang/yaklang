package pcaputil

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// DecodeMLDPacketFields reads the exact captured ICMPv6 envelope. Selection
// means that this wire header is MLD, not that its parent or membership state
// is valid. The native receiver applies parent admission before this function.
func DecodeMLDPacketFields(p gopacket.Packet, maxBytes, maxElements int) (result stream_parser.MLDWireResult, offset int, selected bool, err error) {
	icmp, ok := p.Layer(layers.LayerTypeICMPv6).(*layers.ICMPv6)
	if !ok || len(icmp.LayerContents()) != 4 {
		return
	}
	typ := icmp.TypeCode.Type()
	if typ != 130 && typ != 131 && typ != 132 && typ != 143 {
		return
	}
	offset, ok = nativeNetworkLayerOffset(p, icmp)
	if !ok {
		return
	}
	available := 4 + len(icmp.LayerPayload())
	if available > len(p.Data())-offset {
		return
	}
	var network gopacket.NetworkLayer
	for _, layer := range p.Layers() {
		if layer == icmp {
			break
		}
		if n, isNetwork := layer.(gopacket.NetworkLayer); isNetwork {
			network = n
		}
	}
	context := stream_parser.MLDWireContext{DeclaredBytes: -1}
	if network != nil {
		at, located := nativeNetworkLayerOffset(p, network)
		if located {
			switch ip := network.(type) {
			case *layers.IPv4:
				if ip.Version == 4 && int(ip.Length) >= offset-at {
					context.DeclaredBytes = int(ip.Length) - (offset - at)
				}
			case *layers.IPv6:
				if ip.Version == 6 && ip.Length > 0 && 40+int(ip.Length) >= offset-at {
					context.DeclaredBytes = 40 + int(ip.Length) - (offset - at)
				}
			}
		}
	}
	selected = true
	result, err = stream_parser.DecodeMLDWire(p.Data()[offset:offset+available], maxBytes, maxElements, context)
	shiftMLDWireRanges(result.Fields, offset)
	return
}

func shiftMLDWireRanges(value any, offset int) {
	switch v := value.(type) {
	case map[string]any:
		for name, child := range v {
			if name == "Wire Field Ranges" {
				if ranges, ok := child.(map[string]any); ok {
					for _, raw := range ranges {
						if span, valid := raw.([]int); valid && len(span) == 2 {
							span[0], span[1] = span[0]+offset, span[1]+offset
						}
					}
				}
			} else {
				shiftMLDWireRanges(child, offset)
			}
		}
	case []map[string]any:
		for _, child := range v {
			shiftMLDWireRanges(child, offset)
		}
	}
}
