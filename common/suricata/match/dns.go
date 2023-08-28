package match

import (
	"fmt"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// match dns
func dnsIniter(c *matchContext) error {
	if c.Rule.ContentRuleConfig == nil {
		return nil
	}

	dns := c.PK.Layer(layers.LayerTypeDNS)
	if dns == nil {
		return fmt.Errorf("dns layer not found")
	}

	if c.Rule.ContentRuleConfig.DNS != nil {
		if !c.Must(negIf(c.Rule.ContentRuleConfig.DNS.OpcodeNegative,
			codec.Atoi(string(dns.(*layers.DNS).OpCode)) == c.Rule.ContentRuleConfig.DNS.Opcode,
		)) {
			return nil
		}
	}

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
