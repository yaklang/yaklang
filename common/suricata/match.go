package suricata

import (
	"bytes"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
	"strings"
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

// matcher ip
func ipMatcher(c *matchContext) error {
	flow := c.PK.NetworkLayer().NetworkFlow()
	if !c.Must(c.Rule.SourceAddress.Match(flow.Src().String())) {
		return nil
	}
	if !c.Must(c.Rule.DestinationAddress.Match(flow.Dst().String())) {
		return nil
	}
	return nil
}

// match port
func portMatcher(c *matchContext) error {
	flow := c.PK.TransportLayer().TransportFlow()
	if !c.Must(c.Rule.SourcePort.Match(utils.Atoi(flow.Src().String()))) {
		return nil

	}
	if !c.Must(c.Rule.DestinationPort.Match(utils.Atoi(flow.Src().String()))) {
		return nil
	}
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
		if !c.Must(utils.Atoi(string(dns.(*layers.DNS).OpCode)) != c.Rule.ContentRuleConfig.DNS.Opcode) {
			return nil
		}
	} else {
		if !c.Must(utils.Atoi(string(dns.(*layers.DNS).OpCode)) == c.Rule.ContentRuleConfig.DNS.Opcode) {
			return nil
		}
	}

	for _, r := range c.Rule.ContentRuleConfig.ContentRules {
		switch r.Modifier {
		case DNSQuery:
			c.Attach(newPayloadMatcher(r, dns.(*layers.DNS).Questions[0].Name))
		case Default:
			c.Attach(newPayloadMatcher(r, dns.LayerPayload()))
		}
	}
	return nil
}

func newPayloadMatcher(r *ContentRule, content []byte) func(c *matchContext) error {
	return func(c *matchContext) error {
		var buffer []byte
		copy(buffer, content)
		if r.Nocase {
			r.Content = bytes.ToLower(r.Content)
			buffer = bytes.ToLower(buffer)
		}

		// pcre not implement yet, temporarily skip
		if len(content) == 0 {
			return nil
		}

		// match all
		indexes := bytesIndexAll(content, r.Content)
		if !c.Must(len(indexes) > 0) {
			return nil
		}

		// special options startswith
		if r.StartsWith {
			if !c.Must(indexes[0].pos == 0) {
				return nil
			}
			c.Value["prevMatch"] = []matched{indexes[0]}
			return nil
		}

		// special options endswith
		if r.EndsWith {
			targetPos := len(content) - len(r.Content)
			// depth is valid in endswith
			if r.Depth != nil {
				targetPos = *r.Depth - len(r.Content)
			}

			if !c.Must(indexes[binarySearch(indexes, func(m matched) int {
				return m.pos - targetPos
			})].pos == targetPos) {
				c.Value["prevMatch"] = []matched{indexes[0]}
			}

			return nil
		}

		// depth & offset
		// [le,ri]
		le := 0
		ri := len(content)

		if r.Offset != nil {
			le = *r.Offset
		}

		if r.Depth != nil {
			ri = le + *r.Depth - len(r.Content)
		}

		// [lp,rp)
		lp := binarySearch(indexes, func(m matched) int {
			return m.pos - le
		})

		rp := binarySearch(indexes, func(m matched) int {
			return m.pos - ri
		})

		indexes = indexes[lp:rp]
		if !c.Must(len(indexes) != 0) {
			return nil
		}

		// load prev matches for rel checker
		var prevMatch []matched
		loadIfMapEz(c.Value, &prevMatch, "prevMatch")

		// distance
		if r.Distance != nil {
			indexes = sliceFilter(indexes, func(m matched) bool {
				for _, pm := range prevMatch {
					if m.pos == pm.pos+pm.len+*r.Distance {
						return true
					}
				}
				return false
			})
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

		// within
		if r.Within != nil {
			indexes = sliceFilter(indexes, func(m matched) bool {
				for _, pm := range prevMatch {
					if m.pos+m.len <= pm.pos+pm.len+*r.Within {
						return true
					}
				}
				return false
			})
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

		// isdataat
		if r.IsDataAt != "" {
			strpos := strings.Split(r.IsDataAt, ",")
			var neg bool
			var strnum string
			if len(strpos[0]) > 1 && strpos[0][0] == '!' {
				neg = true
				strnum = strpos[0][1:]
			} else {
				strnum = strpos[0]
			}
			pos, err := strconv.Atoi(strnum)
			if err != nil {
				return errors.Wrap(err, "isdataat format error")
			}
			if len(strpos) == 1 {
				// no relative
				indexes = sliceFilter(indexes, func(m matched) bool {
					return negIf(neg, m.pos+m.len+pos <= len(content))
				})
			} else {
				// with reletive
				if !c.Must(len(strpos) == 2 && strpos[1] == "relative") {
					return errors.New("isdataat format error")
				}
				indexes = sliceFilter(indexes, func(m matched) bool {
					return negIf(neg, pos < len(content))
				})
			}
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

		// todo:bsize dsize
		return nil
	}
}
