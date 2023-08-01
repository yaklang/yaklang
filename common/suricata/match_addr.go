package suricata

// matcher ip
func ipMatcher(c *matchContext) error {
	flow := c.PK.NetworkLayer().NetworkFlow()
	if !c.Must(c.Rule.SourceAddress.Match(flow.Src().String())) {
		return nil
	}
	if !c.Must(c.Rule.DestinationAddress.Match(flow.Dst().String())) {
		return nil
	}
	return nil
}
