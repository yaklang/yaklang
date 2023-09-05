package match

// matcher ip
func ipMatcher(c *matchContext) error {
	nw := c.PK.NetworkLayer()
	if !c.Must(nw != nil) {
		return nil
	}
	flow := nw.NetworkFlow()
	if !c.Must(c.Rule.SourceAddress.Match(flow.Src().String())) {
		return nil
	}
	if !c.Must(c.Rule.DestinationAddress.Match(flow.Dst().String())) {
		return nil
	}
	return nil
}
