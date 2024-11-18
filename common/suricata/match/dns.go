package match

import (
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// match dns
func dnsParser(c *matchContext) error {
	dns := c.PK.Layer(layers.LayerTypeDNS)
	if !c.Must(dns != nil) {
		return nil
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

	return nil
}

func dnsMatcher(c *matchContext) error {
	if !c.Must(c.Rule.ContentRuleConfig != nil) {
		return nil
	}

	// dns match
	dns := c.PK.Layer(layers.LayerTypeDNS)
	if !c.Must(dns != nil) {
		return nil
	}
	if c.Rule.ContentRuleConfig.DNS != nil {
		if !c.Must(negIf(c.Rule.ContentRuleConfig.DNS.OpcodeNegative,
			codec.Atoi(string(dns.(*layers.DNS).OpCode)) == c.Rule.ContentRuleConfig.DNS.Opcode,
		)) {
			return nil
		}
	}
	return nil
}
