package suricata

import (
	"strings"
	"yaklang/common/suricata/parser"
	"yaklang/common/utils"
)

type PortRule struct {
	Any      bool
	Negative bool

	Ports []int
	Rules []*PortRule
	Env   string
}

func (v *RuleSyntaxVisitor) VisitSrcPort(i *parser.Src_portContext) *PortRule {
	return v.VisitPortRule(i.Port().(*parser.PortContext))
}

func (v *RuleSyntaxVisitor) VisitDstPort(i *parser.Dest_portContext) *PortRule {
	return v.VisitPortRule(i.Port().(*parser.PortContext))
}

func (v *RuleSyntaxVisitor) VisitPortRule(i *parser.PortContext) *PortRule {
	if i == nil {
		return nil
	}
	r := &PortRule{}
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
