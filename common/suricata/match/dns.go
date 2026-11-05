package match

import (
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

// match dns
func dnsParser(c *matchContext) error {
	layer := c.PK.Layer(layers.LayerTypeDNS)
	if !c.Must(layer != nil) {
		return nil
	}
	dns, ok := layer.(*layers.DNS)
	if !c.Must(ok) {
		return nil
	}

	c.SetBufferProvider(func(mdf modifier.Modifier) []byte {
		switch mdf {
		case modifier.DNSQuery:
			if len(dns.Questions) == 0 {
				return nil
			}
			return dns.Questions[0].Name
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

	layer := c.PK.Layer(layers.LayerTypeDNS)
	if !c.Must(layer != nil) {
		return nil
	}
	dns, ok := layer.(*layers.DNS)
	if !c.Must(ok) {
		return nil
	}
	config := c.Rule.ContentRuleConfig.DNS
	if config == nil {
		return nil
	}
	if config.OpcodeSet && !c.Must(negIf(config.OpcodeNegative, int(dns.OpCode) == config.Opcode)) {
		return nil
	}
	if config.RCode != nil && !c.Must(config.RCode.Match(int(dns.ResponseCode))) {
		return nil
	}
	if config.RRType != nil {
		matched := false
		for _, question := range dns.Questions {
			matched = matched || config.RRType.Match(int(question.Type))
		}
		for _, records := range [][]layers.DNSResourceRecord{dns.Answers, dns.Authorities, dns.Additionals} {
			for _, record := range records {
				matched = matched || config.RRType.Match(int(record.Type))
			}
		}
		if !c.Must(matched) {
			return nil
		}
	}
	return nil
}
