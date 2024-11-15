package rule

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"math/rand"
	"slices"
	"sort"
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

func (p *PortRule) GenerateWithDefault(def uint32) uint32 {
	if p == nil || p.Any {
		return def
	}
	return p.GetAvailablePort()
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
		if ok && codec.Atoi(result) == i {
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

func (p *PortRule) GetAvailablePort() uint32 {
	if p == nil || p.Any {
		return uint32(getHighPort())
	}

	if strings.Contains(strings.ToLower(p.Env), "ssh") {
		return 22
	} else if p.Env != "" {
		return uint32(getHighPort())
	}

	if len(p.Ports) > 0 && !p.Negative {
		return uint32(p.Ports[rand.Intn(len(p.Ports))])
	}

	var count int = 1000
	for len(p.Ports) > 0 && p.Negative && count <= 30000 {
		matched := false
		for _, p := range p.Ports {
			if p == count {
				matched = true
				break
			}
		}
		if matched {
			return uint32(count)
		}
		count++
	}
	if p.Negative {
		allPorts := []uint32{}
		lo.ForEach(p.Rules, func(item *PortRule, index int) {
			lo.ForEach(item.Ports, func(item int, index int) {
				allPorts = append(allPorts, uint32(item))
			})
		})
		sort.Slice(allPorts, func(i, j int) bool {
			return allPorts[i] < allPorts[j]
		})
		if len(allPorts) > 0 {
			for i := 0; i < 10000; i++ {
				n := rand.Intn(65535-1000) + 1000
				if !slices.Contains(allPorts, uint32(n)) {
					return uint32(n)
				}
			}
			return 0
		}
	}
	return p.Rules[rand.Intn(len(p.Rules))].GetAvailablePort()
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
