package match

import (
	"fmt"
	"sync"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/data/protocol"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

type Matcher struct {
	matcher *matchContext
}

func CompileRule(r *rule.Rule) (*Matcher, error) {
	ctx, err := compile(r)
	if err != nil {
		return nil, err
	}
	return &Matcher{
		matcher: ctx,
	}, nil
}
func New(r *rule.Rule) *Matcher {
	ctx, err := compile(r)
	if err != nil {
		log.Error(err)
	}
	return &Matcher{
		matcher: ctx,
	}
}

func (m *Matcher) Match(flow []byte) bool {
	if len(flow) == 0 {
		return false
	}
	pk := gopacket.NewPacket(flow, layers.LayerTypeEthernet, gopacket.NoCopy)
	return m.MatchPackage(pk)
}

func (m *Matcher) MatchHTTPFlow(flow *HttpFlow) bool {
	if flow == nil {
		return false
	}
	for _, packet := range flow.ToRequestPacket() {
		if m.MatchPackage(packet) {
			return true
		}
	}
	return false
}

func (m *Matcher) MatchPackage(pk gopacket.Packet) bool {
	if pk == nil {
		return false
	}

	err, ok := m.matcher.Match(pk)
	if err != nil {
		return false
	}
	return ok
}

type matchHandler func(*matchContext) error

type bufferProvider func(modifier modifier.Modifier) []byte

type matchContext struct {
	// matcher itself not designed for multi threads, you'd best clone it before use.
	// lock is used to protect the matcher from being used by multiple goroutines.
	lock sync.Mutex

	rejected bool
	pos      int

	provider bufferProvider
	buffer   map[modifier.Modifier][]byte

	Value map[string]any

	prevMatched  []data.Matched
	prevModifier modifier.Modifier

	PK   gopacket.Packet
	Rule *rule.Rule

	workflow []matchHandler
}

func (c *matchContext) Clone() *matchContext {
	return &matchContext{
		Value:    make(map[string]any),
		pos:      -1,
		buffer:   make(map[modifier.Modifier][]byte),
		Rule:     c.Rule,
		workflow: c.workflow,
	}
}

func (c *matchContext) Reject() {
	c.rejected = true
}

func (c *matchContext) Recover() {
	c.rejected = false
}

func (c *matchContext) IsRejected() bool {
	return c.rejected
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
	c.pos++
	defer func() {
		c.pos--
	}()
	if c.rejected || c.pos >= len(c.workflow) {
		return nil
	}
	if err := c.workflow[c.pos](c); err != nil {
		return err
	}
	return c.Next()
}

func (c *matchContext) Must(ok bool) bool {
	if !ok {
		c.Reject()
	}
	return ok
}

func (c *matchContext) Tidy() {
	c.Value = make(map[string]any)
	c.PK = nil
	c.provider = nil
	c.buffer = make(map[modifier.Modifier][]byte)
	c.pos = -1
	c.rejected = false
	c.prevMatched = nil
	c.prevModifier = modifier.Default
}

func compile(r *rule.Rule) (*matchContext, error) {
	c := &matchContext{
		Value:  make(map[string]any),
		Rule:   r,
		pos:    -1,
		buffer: make(map[modifier.Modifier][]byte),
	}

	if err := matchMutex(c); err != nil {
		return c, fmt.Errorf("match mutex failed: %s", err.Error())
	}

	return c, nil
}

func (c *matchContext) match() (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("match flow failed: %v", e)
		}
	}()
	err = c.Next()
	if err != nil {
		return err
	}
	return nil
}
func (c *matchContext) Match(pk gopacket.Packet) (error, bool) {
	c.lock.Lock()
	c.Tidy()
	defer c.lock.Unlock()
	c.PK = pk
	err := c.match()
	if err != nil {
		return fmt.Errorf("match failed: %v", err), false
	}
	return nil, !c.rejected
}

func matchMutex(c *matchContext) error {
	switch c.Rule.Protocol {
	case protocol.DNS:
		c.Attach(ipMatcher, portMatcher, dnsParser)
		attachFastPattern(c)
		c.Attach(dnsMatcher)
		attachPayloadMatcher(c)
	case protocol.HTTP:
		c.Attach(ipMatcher, portMatcher, httpParser)
		attachFastPattern(c)
		attachHTTPMatcher(c)
		attachPayloadMatcher(c)
	case protocol.TCP:
		c.Attach(ipMatcher, portMatcher, tcpParser)
		attachFastPattern(c)
		c.Attach(tcpCfgMatch)
		attachPayloadMatcher(c)
	case protocol.UDP:
		c.Attach(ipMatcher, portMatcher, udpParser)
		attachFastPattern(c)
		attachPayloadMatcher(c)
	case protocol.ICMP:
		c.Attach(ipMatcher, icmpParser)
		attachFastPattern(c)
		c.Attach(icmpCfgMatch)
		attachPayloadMatcher(c)
	case protocol.TLS:
		c.Attach(ipMatcher, portMatcher, tlsParser)
		attachFastPattern(c)
		// c.Attach(tlsMatcher)
		attachPayloadMatcher(c)
	default:
		return fmt.Errorf("unsupported protocol: %s", c.Rule.Protocol)
	}
	return nil
}

func (c *matchContext) SetBufferProvider(p func(modifier modifier.Modifier) []byte) {
	c.provider = p
}

func (c *matchContext) GetBuffer(modifier modifier.Modifier) []byte {
	if _, ok := c.buffer[modifier]; !ok {
		c.buffer[modifier] = c.provider(modifier)
	}
	return c.buffer[modifier]
}

func (c *matchContext) SetBuffer(modifier modifier.Modifier, buf []byte) {
	c.buffer[modifier] = buf
}

// GetPrevMatched return true if the previous motch for current modifier existed.
func (c *matchContext) GetPrevMatched(mdf modifier.Modifier) ([]data.Matched, bool) {
	if c.prevModifier == mdf {
		return c.prevMatched, true
	}
	return nil, false
}

func (c *matchContext) SetPrevMatched(mdf modifier.Modifier, matched []data.Matched) {
	c.prevModifier = mdf
	c.prevMatched = matched
}
