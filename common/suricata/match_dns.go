package suricata

import (
	"fmt"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/utils"
)

type DNSRule struct {
	OpcodeNegative bool
	Opcode         int
}

// match dns
func dnsMatcher(c *matchContext) error {
	if c.Rule.ContentRuleConfig == nil {
		return nil
	}

	dns := c.PK.Layer(layers.LayerTypeDNS)
	if dns == nil {
		return fmt.Errorf("dns layer not found")
	}

	if c.Rule.ContentRuleConfig.DNS != nil {
		if !c.Must(negIf(c.Rule.ContentRuleConfig.DNS.OpcodeNegative,
			utils.Atoi(string(dns.(*layers.DNS).OpCode)) == c.Rule.ContentRuleConfig.DNS.Opcode,
		)) {
			return nil
		}
	}

	for _, r := range c.Rule.ContentRuleConfig.ContentRules {
		switch r.Modifier {
		case DNSQuery:
			c.Attach(newPayloadMatcher(r, dns.(*layers.DNS).Questions[0].Name))
		case Default:
			c.Attach(newPayloadMatcher(r, dns.LayerContents()))
		}
	}
	return nil
}
