package match

import (
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"golang.org/x/exp/slices"
)

// match dns
func dnsIniter(c *matchContext) error {
	if !c.Must(c.Rule.ContentRuleConfig != nil) {
		return nil
	}

	dns := c.PK.Layer(layers.LayerTypeDNS)
	if !c.Must(dns != nil) {
		return nil
	}
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

	// dns match
	if c.Rule.ContentRuleConfig.DNS != nil {
		if !c.Must(negIf(c.Rule.ContentRuleConfig.DNS.OpcodeNegative,
			codec.Atoi(string(dns.(*layers.DNS).OpCode)) == c.Rule.ContentRuleConfig.DNS.Opcode,
		)) {
			return nil
		}
	}

	// register buffer provider
	c.SetBufferProvider(func(mdf modifier.Modifier) []byte {
		switch mdf {
		case modifier.DNSQuery:
			return dns.(*layers.DNS).Questions[0].Name
		case modifier.Default:
			return dns.LayerContents()
		}
		return nil
	})

	for _, r := range c.Rule.ContentRuleConfig.ContentRules {
		c.Attach(newPayloadMatcher(r, r.Modifier))
	}
	return nil
}
