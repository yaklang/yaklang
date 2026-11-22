package match

import (
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// match port
func portMatcher(c *matchContext) error {
	transLayer := c.PK.TransportLayer()
	if !c.Must(transLayer != nil) {
		return nil
	}
	var srcPort, dstPort int
	switch tl := transLayer.(type) {
	case *layers.TCP:
		srcPort = int(tl.SrcPort)
		dstPort = int(tl.DstPort)
	case *layers.UDP:
		srcPort = int(tl.SrcPort)
		dstPort = int(tl.DstPort)
	default:
		flow := transLayer.TransportFlow()
		srcPort = codec.Atoi(flow.Src().String())
		dstPort = codec.Atoi(flow.Dst().String())
	}
	directions, _ := c.Value["headerDirections"].(uint8)
	if directions&headerDirectionForward != 0 &&
		!(c.Rule.SourcePort.Match(srcPort) && c.Rule.DestinationPort.Match(dstPort)) {
		directions &^= headerDirectionForward
	}
	if directions&headerDirectionReverse != 0 &&
		!(c.Rule.SourcePort.Match(dstPort) && c.Rule.DestinationPort.Match(srcPort)) {
		directions &^= headerDirectionReverse
	}
	c.Value["headerDirections"] = directions
	c.Must(directions != 0)
	return nil
}
