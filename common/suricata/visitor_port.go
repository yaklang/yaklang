package suricata

import (
	"github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

type PortRule struct {
	Any      bool
	Negative bool

	// port cache
	portsMap map[int]int
	Ports    []int

	envTable map[string]string

	// rule cache
	negativeRules []*PortRule
	positiveRules []*PortRule
	Rules         []*PortRule

	Env string
}

func (p *PortRule) postHandle() {
	if len(p.Ports) > 0 {
		if p.portsMap == nil {
			p.portsMap = make(map[int]int)
		}
		for _, port := range p.Ports {
			p.portsMap[port] = port
		}
	}

	if len(p.Rules) > 0 {
		for _, r := range p.Rules {
			if r.Negative {
				p.negativeRules = append(p.negativeRules, r)
			} else {
				p.positiveRules = append(p.positiveRules, r)
			}
		}
	}
}

func (p *PortRule) _matchWithoutNegative(i int) bool {
	if p == nil {
		return false
	}
	if p.Any {
		return true
	}

	if len(p.Ports) > 0 {
		if p.portsMap != nil {
			_, ok := p.portsMap[i]
			if ok {
				return ok
			}
		} else {
			for _, pInt := range p.Ports {
				if pInt == i {
					return true
				}
			}
		}
	}

	if len(p.Rules) > 0 {
		if len(p.negativeRules) <= 0 && len(p.positiveRules) <= 0 {
			p.postHandle()
		}
		for _, nr := range p.negativeRules {
			if !nr.Match(i) {
				return false
			}
		}
		for _, pr := range p.positiveRules {
			if pr.Match(i) {
				return true
			}
		}
	}

	if p.Env != "" && p.envTable != nil {
		result, ok := p.envTable[p.Env]
		result = strings.TrimSpace(result)
		if ok && utils.Atoi(result) == i {
			return true
		}
	}

	return false
}

func (p *PortRule) Match(i int) bool {
	if p.Negative {
		return !p._matchWithoutNegative(i)
	} else {
		return p._matchWithoutNegative(i)
	}
}

func (v *RuleSyntaxVisitor) VisitSrcPort(i *parser.Src_portContext) *PortRule {
	p := v.VisitPortRule(i.Port().(*parser.PortContext))
	return p
}

func (v *RuleSyntaxVisitor) VisitDstPort(i *parser.Dest_portContext) *PortRule {
	p := v.VisitPortRule(i.Port().(*parser.PortContext))
	return p
}

func (v *RuleSyntaxVisitor) VisitPortRule(i *parser.PortContext) *PortRule {
	if i == nil {
		return nil
	}
	r := &PortRule{envTable: v.Environment}
	if i.Any() != nil {
		r.Any = true
		return r
	}

	if i.Environment_var() != nil {
		r.Env = trim(i.Environment_var().GetText())
		return r
	}

	if i.Negative() != nil {
		r.Negative = true
		r.Rules = append(r.Rules, v.VisitPortRule(i.Port(0).(*parser.PortContext)))
		return r
	}

	if i.Colon() == nil {
		if len(i.AllINT()) == 1 && i.INT(0) != nil {
			r.Ports = []int{atoi(strings.TrimSpace(i.INT(0).GetText()))}
			return r
		}
	} else {
		raw := strings.TrimSpace(i.GetText())
		inPrefix := strings.HasPrefix(raw, ":")
		inSuffix := strings.HasSuffix(raw, ":")
		if inPrefix {
			raw = "1" + raw
		}
		if inSuffix {
			raw += "65535"
		}
		raw = strings.ReplaceAll(raw, ":", "-")
		r.Ports = utils.ParseStringToPorts(raw)
		return r
	}

	var rules []*PortRule
	for _, subRule := range i.AllPort() {
		rules = append(rules, v.VisitPortRule(subRule.(*parser.PortContext)))
	}
	r.Rules = rules
	return r
}
