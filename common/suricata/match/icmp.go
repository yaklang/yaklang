package match

import (
	"fmt"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"golang.org/x/exp/slices"
)

// match icmp
func icmpIniter(c *matchContext) error {
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

	// fast pattern
	idx := slices.IndexFunc(c.Rule.ContentRuleConfig.ContentRules, func(rule *rule.ContentRule) bool {
		return rule.FastPattern
	})
	if idx != -1 {
		fastPatternRule := c.Rule.ContentRuleConfig.ContentRules[idx]
		c.Attach(newPayloadMatcher(fastPatternCopy(fastPatternRule), fastPatternRule.Modifier))
		err := c.Next()
		if c.IsRejected() {
			return err
		}
	}

	// icmp match
	c.Attach(icmpCfgMatch)

	// payload match
	for _, r := range c.Rule.ContentRuleConfig.ContentRules {
		c.Attach(newPayloadMatcher(r, r.Modifier))
	}

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
