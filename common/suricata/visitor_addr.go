package suricata

import (
	"yaklang/common/suricata/parser"
)

type AddressRule struct {
	// 这两个是修饰词
	Any      bool
	Negative bool

	// 这几个是终结词
	IPv4CIDR string
	IPv6     string
	SubRules []*AddressRule
	Env      string
}

func (v *RuleSyntaxVisitor) VisitSrcAddress(i *parser.Src_addressContext) *AddressRule {
	return v.VisitAddress(i.Address().(*parser.AddressContext))
}

func (v *RuleSyntaxVisitor) VisitDstAddress(i *parser.Dest_addressContext) *AddressRule {
	return v.VisitAddress(i.Address().(*parser.AddressContext))
}

func (v *RuleSyntaxVisitor) VisitAddress(i *parser.AddressContext) *AddressRule {
	if i == nil {
		return nil
	}
	addr := &AddressRule{}
	if i.Any() != nil {
		addr.Any = true
		return addr
	}
	if i.Negative() != nil {
		addr.Negative = true
		addr.SubRules = append(addr.SubRules, v.VisitAddress(i.Address(0).(*parser.AddressContext)))
		return addr
	}
	if i.Ipv4() != nil {
		addr.IPv4CIDR = trim(i.Ipv4().GetText())
		return addr
	}
	if i.Ipv6() != nil {
		addr.IPv6 = trim(i.Ipv6().GetText())
		return addr
	}
	if i.Environment_var() != nil {
		addr.Env = trim(i.Environment_var().GetText())
		return addr
	}

	var subs []*AddressRule
	for _, r := range i.AllAddress() {
		if r == nil {
			continue
		}
		subs = append(subs, v.VisitAddress(r.(*parser.AddressContext)))
	}
	addr.SubRules = subs
	return addr
}
