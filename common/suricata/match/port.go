package match

import "github.com/yaklang/yaklang/common/yak/yaklib/codec"

// match port
func portMatcher(c *matchContext) error {
	transLayer := c.PK.TransportLayer()
	if !c.Must(transLayer != nil) {
		return nil
	}
	flow := transLayer.TransportFlow()
	directions, _ := c.Value["headerDirections"].(uint8)
	srcPort := codec.Atoi(flow.Src().String())
	dstPort := codec.Atoi(flow.Dst().String())
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
