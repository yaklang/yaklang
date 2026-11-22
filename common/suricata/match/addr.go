package match

import (
	"net/netip"

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
	var srcAddr, dstAddr netip.Addr
	switch network := nw.(type) {
	case *layers.IPv4:
		c.SetBuffer(modifier.IPv4HDR, network.LayerContents())
		if s := network.SrcIP.To4(); len(s) == 4 {
			srcAddr = netip.AddrFrom4([4]byte{s[0], s[1], s[2], s[3]})
		}
		if d := network.DstIP.To4(); len(d) == 4 {
			dstAddr = netip.AddrFrom4([4]byte{d[0], d[1], d[2], d[3]})
		}
	case *layers.IPv6:
		c.SetBuffer(modifier.IPv6HDR, network.LayerContents())
		if s := network.SrcIP.To16(); len(s) == 16 {
			srcAddr = netip.AddrFrom16([16]byte(s)).Unmap()
		}
		if d := network.DstIP.To16(); len(d) == 16 {
			dstAddr = netip.AddrFrom16([16]byte(d)).Unmap()
		}
	}
	directions := uint8(0)
	if srcAddr.IsValid() && dstAddr.IsValid() {
		if c.Rule.SourceAddress.MatchAddr(srcAddr) && c.Rule.DestinationAddress.MatchAddr(dstAddr) {
			directions |= headerDirectionForward
		}
		if c.Rule.Direction == "<>" && c.Rule.SourceAddress.MatchAddr(dstAddr) && c.Rule.DestinationAddress.MatchAddr(srcAddr) {
			directions |= headerDirectionReverse
		}
	} else {
		flow := nw.NetworkFlow()
		srcStr, dstStr := flow.Src().String(), flow.Dst().String()
		if c.Rule.SourceAddress.Match(srcStr) && c.Rule.DestinationAddress.Match(dstStr) {
			directions |= headerDirectionForward
		}
		if c.Rule.Direction == "<>" && c.Rule.SourceAddress.Match(dstStr) && c.Rule.DestinationAddress.Match(srcStr) {
			directions |= headerDirectionReverse
		}
	}
	c.Value["headerDirections"] = directions
	c.Must(directions != 0)
	return nil
}
