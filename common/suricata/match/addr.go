package match

import (
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

const (
	headerDirectionForward uint8 = 1 << iota
	headerDirectionReverse
)

// matcher ip
func ipMatcher(c *matchContext) error {
	nw := c.PK.NetworkLayer()
	if !c.Must(nw != nil) {
		return nil
	}
	switch network := nw.(type) {
	case *layers.IPv4:
		c.SetBuffer(modifier.IPv4HDR, network.LayerContents())
	case *layers.IPv6:
		c.SetBuffer(modifier.IPv6HDR, network.LayerContents())
	}
	flow := nw.NetworkFlow()
	directions := uint8(0)
	if c.Rule.SourceAddress.Match(flow.Src().String()) && c.Rule.DestinationAddress.Match(flow.Dst().String()) {
		directions |= headerDirectionForward
	}
	if c.Rule.Direction == "<>" && c.Rule.SourceAddress.Match(flow.Dst().String()) && c.Rule.DestinationAddress.Match(flow.Src().String()) {
		directions |= headerDirectionReverse
	}
	c.Value["headerDirections"] = directions
	c.Must(directions != 0)
	return nil
}
