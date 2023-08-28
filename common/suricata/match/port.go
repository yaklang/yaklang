package match

import "github.com/yaklang/yaklang/common/yak/yaklib/codec"

// match port
func portMatcher(c *matchContext) error {
	flow := c.PK.TransportLayer().TransportFlow()
	if !c.Must(c.Rule.SourcePort.Match(codec.Atoi(flow.Src().String()))) {
		return nil

	}
	if !c.Must(c.Rule.DestinationPort.Match(codec.Atoi(flow.Dst().String()))) {
		return nil
	}
	return nil
}
