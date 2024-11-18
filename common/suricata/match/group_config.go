package match

import (
	"github.com/gopacket/gopacket"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

type GroupOption func(group *Group)

func WithGroupOnMatchedCallback(cb func(packet gopacket.Packet, match *rule.Rule)) GroupOption {
	return func(c *Group) {
		c.onMatchedCallback = cb
	}
}
