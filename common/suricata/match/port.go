package match

import "github.com/yaklang/yaklang/common/utils"

// match port
func portMatcher(c *matchContext) error {
	transLayer := c.PK.TransportLayer()
	if !c.Must(transLayer != nil) {
		return nil
	}
	flow := transLayer.TransportFlow()
	if !c.Must(c.Rule.SourcePort.Match(utils.Atoi(flow.Src().String()))) {
		return nil

	}
	if !c.Must(c.Rule.DestinationPort.Match(utils.Atoi(flow.Dst().String()))) {
		return nil
	}
	return nil
}
