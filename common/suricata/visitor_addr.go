package suricata

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

type AddressRule struct {
	// 这两个是修饰词
	Any      bool
	Negative bool

	// 这几个是终结词
	hostFilter *utils.HostsFilter

	IPv4CIDR string
	IPv6     string

	negativeRules []*AddressRule
	positiveRules []*AddressRule
	SubRules      []*AddressRule

	// envtable
	envtable map[string]string
	Env      string
}

func (a *AddressRule) _matchWithoutNegative(i string) bool {
	if a == nil {
		return false
	}
	if a.Any {
		return true
	}

	if a.Env != "" && a.envtable != nil {
		raw, _ := a.envtable[a.Env]
		if raw != "" && raw == i {
			return true
		}
	}

	if len(a.SubRules) > 0 {
		if len(a.negativeRules) <= 0 && len(a.positiveRules) <= 0 {
			for _, n := range a.SubRules {
				if n.Negative {
					a.negativeRules = append(a.negativeRules, n)
				} else {
					a.positiveRules = append(a.positiveRules, n)
				}
			}
		}
		for _, n := range a.negativeRules {
			if !n.Match(i) {
				return false
			}
		}
		for _, n := range a.positiveRules {
			if n.Match(i) {
				return true
			}
		}
	}

	if a.IPv4CIDR != "" {
		if a.hostFilter == nil {
			a.hostFilter = utils.NewHostsFilter()
		}
		a.hostFilter.Add(a.IPv4CIDR)
	}

	if a.IPv6 != "" {
		if a.hostFilter == nil {
			a.hostFilter = utils.NewHostsFilter()
		}
		a.hostFilter.Add(a.IPv6)
	}

	if a.hostFilter != nil {
		if a.hostFilter.Contains(i) {
			return true
		}
	}
	return false
}

func (a *AddressRule) Match(i string) bool {
	if a == nil {
		return false
	}

	if a.Negative {
		return !a._matchWithoutNegative(i)
	}
	return a._matchWithoutNegative(i)
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
	addr := &AddressRule{envtable: v.Environment}
	if i.Any() != nil {
		addr.Any = true
		return addr
	}
	if i.Negative() != nil {
		addr.Negative = true
		addr.SubRules = append(addr.SubRules, v.VisitAddress(i.Address(0).(*parser.AddressContext)))
		return addr
	}
	switch true {
	case i.Ipv4() != nil:
		addr.IPv4CIDR = trim(i.Ipv4().GetText())
		return addr
	case i.Ipv6() != nil:
		addr.IPv6 = trim(i.Ipv6().GetText())
		return addr
	case i.Environment_var() != nil:
		addr.Env = strings.Trim(trim(i.Environment_var().GetText()), "${}")
		return addr
	case len(i.AllAddress()) > 0:
		var subs []*AddressRule
		for _, r := range i.AllAddress() {
			if r == nil {
				continue
			}
			subs = append(subs, v.VisitAddress(r.(*parser.AddressContext)))
		}
		addr.SubRules = subs
		return addr
	default:
		log.Errorf("unhandled unit: %v", i.GetText())
		return nil
	}
}
