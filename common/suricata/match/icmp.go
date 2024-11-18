package match

import (
	"fmt"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

// match icmp
func icmpParser(c *matchContext) error {
	if !c.Must(c.Rule.ContentRuleConfig != nil) {
		return nil
	}

	// icmp6 not supported
	icmp4 := c.PK.Layer(layers.LayerTypeICMPv4)
	if !c.Must(icmp4 != nil) {
		return nil
	}

	// register buffer provider
	c.SetBufferProvider(func(mdf modifier.Modifier) []byte {
		switch mdf {
		case modifier.Default:
			return icmp4.LayerPayload()
		case modifier.ICMPV4HDR:
			return icmp4.LayerContents()
		}
		return nil
	})
	return nil
}

func icmpCfgMatch(c *matchContext) error {
	if c.Rule.ContentRuleConfig.IcmpConfig == nil {
		return nil
	}
	icmpConfig := c.Rule.ContentRuleConfig.IcmpConfig

	icmp4 := c.PK.Layer(layers.LayerTypeICMPv4).(*layers.ICMPv4)
	if icmp4 == nil {
		return fmt.Errorf("icmp layer not found")
	}

	if icmpConfig.ICMPId != nil {
		if !c.Must(*icmpConfig.ICMPId == int(icmp4.Id)) {
			return nil
		}
	}

	if icmpConfig.ICMPSeq != nil {
		if !c.Must(*icmpConfig.ICMPSeq == int(icmp4.Seq)) {
			return nil
		}
	}

	if icmpConfig.IType != nil {
		if !c.Must(icmpConfig.IType.Match(int(icmp4.TypeCode >> 8))) {
			return nil
		}
	}

	if icmpConfig.ICode != nil {
		if !c.Must(icmpConfig.ICode.Match(int(icmp4.TypeCode & 0xff))) {
			return nil
		}
	}
	return nil
}
