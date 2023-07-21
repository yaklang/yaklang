package suricata

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Match flow with rule
func (r *Rule) Match(flow []byte) bool {
	pk := gopacket.NewPacket(flow, layers.LayerTypeEthernet, gopacket.Default)
	matcher := newMatchCtx(pk, r, matchMutex)
	err := matcher.Loop()
	if err != nil {
		log.Errorf("match flow failed: %s", err.Error())
		return false
	}
	return !matcher.rejected
}

type matchHandler func(*matchContext) error

type matchContext struct {
	rejected bool
	pos      int

	Value map[string]any

	PK   gopacket.Packet
	Rule *Rule

	workflow []matchHandler
}

func (c *matchContext) Reject() {
	c.rejected = true
}

func (c *matchContext) Attach(handler ...matchHandler) {
	c.workflow = append(c.workflow, handler...)
}

func (c *matchContext) Loop() error {
	if c.rejected || c.pos >= len(c.workflow) {
		return nil
	}
	if err := c.workflow[c.pos](c); err != nil {
		return err
	}
	c.pos++
	return c.Loop()
}

func (c *matchContext) Must(ok bool) {
	if !ok {
		c.Reject()
	}
}

func newMatchCtx(pk gopacket.Packet, r *Rule, hs ...matchHandler) *matchContext {
	return &matchContext{
		Value:    make(map[string]any),
		PK:       pk,
		Rule:     r,
		workflow: hs,
	}
}

func matchMutex(c *matchContext) error {
	switch c.Rule.Protocol {
	case DNS:
		c.Attach(ipMatcher, portMatcher, dnsMatcher)
	default:
		return fmt.Errorf("unsupported protocol: %s", c.Rule.Protocol)
	}
	return nil
}

// matcher ip
func ipMatcher(c *matchContext) error {
	flow := c.PK.NetworkLayer().NetworkFlow()
	c.Must(c.Rule.SourceAddress.Match(flow.Src().String()))
	c.Must(c.Rule.DestinationAddress.Match(flow.Dst().String()))
	return nil
}

// match port
func portMatcher(c *matchContext) error {
	flow := c.PK.TransportLayer().TransportFlow()
	c.Must(c.Rule.SourcePort.Match(utils.Atoi(flow.Src().String())))
	c.Must(c.Rule.DestinationPort.Match(utils.Atoi(flow.Src().String())))
	return nil
}

// match dns
func dnsMatcher(c *matchContext) error {
	if c.Rule.ContentRuleConfig == nil || c.Rule.ContentRuleConfig.DNS == nil {
		return nil
	}
	dns := c.PK.Layer(layers.LayerTypeDNS)
	if dns == nil {
		return fmt.Errorf("dns layer not found")
	}
	if c.Rule.ContentRuleConfig.DNS.OpcodeNegative {
		c.Must(utils.Atoi(string(dns.(*layers.DNS).OpCode)) != c.Rule.ContentRuleConfig.DNS.Opcode)
	} else {
		c.Must(utils.Atoi(string(dns.(*layers.DNS).OpCode)) == c.Rule.ContentRuleConfig.DNS.Opcode)
	}
	if c.Rule.ContentRuleConfig.DNS.DNSQuery {
	}
	return nil
}
