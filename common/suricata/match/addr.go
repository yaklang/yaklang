package match

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
