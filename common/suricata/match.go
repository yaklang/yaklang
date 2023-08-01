package suricata

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
)

// Match flow with rule
func (r *Rule) Match(flow []byte) bool {
	pk := gopacket.NewPacket(flow, layers.LayerTypeEthernet, gopacket.NoCopy)
	matcher := newMatchCtx(pk, r, matchMutex)
	err := matcher.Next()
	if err != nil {
		log.Errorf("match flow failed: %s", err.Error())
		return false
	}
	return !matcher.rejected
}

type matched struct {
	pos int
	len int
}

type matchHandler func(*matchContext) error

type matchContext struct {
	rejected  bool
	recovered bool
	pos       int

	Value             map[string]any
	ContentMatchCache []matched

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

func (c *matchContext) Insert(handler ...matchHandler) {
	if c.pos+1 < len(c.workflow) {
		c.workflow = append(c.workflow[:c.pos+1], append(handler, c.workflow[c.pos+1:]...)...)
	} else {
		c.workflow = append(c.workflow, handler...)
	}
}

func (c *matchContext) Next() error {
	if c.rejected || c.pos >= len(c.workflow) {
		return nil
	}
	if err := c.workflow[c.pos](c); err != nil {
		return err
	}
	c.pos++
	return c.Next()
}

func (c *matchContext) Must(ok bool) bool {
	if !ok {
		c.Reject()
	}
	return ok
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
