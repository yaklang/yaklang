package match

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"golang.org/x/exp/slices"
)

func udpIniter(c *matchContext) error {
	if !c.Must(c.Rule.ContentRuleConfig != nil) {
		return nil
	}

	// buffer provider
	provider := newUDPProvider(c.PK)
	if !c.Must(provider != nil) {
		return nil
	}

	// register buffer provider
	c.SetBufferProvider(provider)

	// fast pattern
	idx := slices.IndexFunc(c.Rule.ContentRuleConfig.ContentRules, func(rule *rule.ContentRule) bool {
		return rule.FastPattern
	})
	if idx != -1 {
		fastPatternRule := c.Rule.ContentRuleConfig.ContentRules[idx]
		c.Attach(
			newPayloadMatcher(
				fastPatternCopy(fastPatternRule),
				fastPatternRule.Modifier),
		)
	}

	// payload match
	for _, r := range c.Rule.ContentRuleConfig.ContentRules {
		c.Attach(newPayloadMatcher(r, r.Modifier))
	}

	err := c.Next()
	if err != nil {
		return err
	}

	return nil

}

func newUDPProvider(pk gopacket.Packet) func(modifier modifier.Modifier) []byte {
	udp, ok := pk.Layer(layers.LayerTypeUDP).(*layers.UDP)
	if !ok {
		return nil
	}
	return func(mdf modifier.Modifier) []byte {
		switch mdf {
		case modifier.UDPHDR:
			return udp.Contents
		case modifier.Default:
			return udp.Payload
		}
		return nil
	}
}
