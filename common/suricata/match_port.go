package suricata

import "github.com/yaklang/yaklang/common/utils"

// match port
func portMatcher(c *matchContext) error {
	flow := c.PK.TransportLayer().TransportFlow()
	if !c.Must(c.Rule.SourcePort.Match(utils.Atoi(flow.Src().String()))) {
		return nil

	}
	if !c.Must(c.Rule.DestinationPort.Match(utils.Atoi(flow.Dst().String()))) {
		return nil
	}
	return nil
}
