package match

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
)

func udpParser(c *matchContext) error {
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

	return nil
}

// tcpOrUDPParser handles dual-transport protocols such as SIP.
func tcpOrUDPParser(c *matchContext) error {
	if !c.Must(c.Rule.ContentRuleConfig != nil) {
		return nil
	}
	if provider := newTCPProvider(c.PK, c.overridePayload); provider != nil {
		c.SetBufferProvider(provider)
		return nil
	}
	if provider := newUDPProvider(c.PK); provider != nil {
		c.SetBufferProvider(provider)
		return nil
	}
	c.Reject()
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
