package rule

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/config"
	"github.com/yaklang/yaklang/common/suricata/parser"
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

type portRange struct {
	min uint16
	max uint16
}

type PortRule struct {
	Any      bool
	Negative bool

	// Compact sorted inclusive port ranges (avoids expanding 1024:65535 into
	// 64k-element slices and maps per rule).
	portRanges []portRange
	// Ports is retained for backwards compatibility when callers construct
	// PortRule literals directly; Prepare folds any entries into portRanges.
	Ports []int

	Config     *config.Config
	envMatcher *config.VarMatcher
	// rule cache
	negativeRules []*PortRule
	positiveRules []*PortRule
	Rules         []*PortRule
	prepared      bool

	Env string
}

func mergePortRanges(ranges []portRange) []portRange {
	if len(ranges) <= 1 {
		return ranges
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].min != ranges[j].min {
			return ranges[i].min < ranges[j].min
		}
		return ranges[i].max < ranges[j].max
	})
	merged := ranges[:1]
	for _, r := range ranges[1:] {
		last := &merged[len(merged)-1]
		if uint32(r.min) <= uint32(last.max)+1 {
			if r.max > last.max {
				last.max = r.max
			}
		} else {
			merged = append(merged, r)
		}
	}
	return merged
}

func containsPortRange(ranges []portRange, port int) bool {
	if port < 0 || port > 65535 {
		return false
	}
	p := uint16(port)
	if len(ranges) <= 4 {
		for _, r := range ranges {
			if p >= r.min && p <= r.max {
				return true
			}
		}
		return false
	}
	lo, hi := 0, len(ranges)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if ranges[mid].max < p {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo < len(ranges) && ranges[lo].min <= p && p <= ranges[lo].max
}

func (p *PortRule) GenerateWithDefault(def uint32) uint32 {
	if p == nil || p.Any {
		return def
	}
	return p.GetAvailablePort()
}

func (p *PortRule) postHandle() {
	if len(p.Ports) > 0 {
		for _, port := range p.Ports {
			if port >= 0 && port <= 65535 {
				u := uint16(port)
				p.portRanges = append(p.portRanges, portRange{min: u, max: u})
			}
		}
		p.portRanges = mergePortRanges(p.portRanges)
	}

	if len(p.Rules) > 0 && len(p.negativeRules) == 0 && len(p.positiveRules) == 0 {
		for _, r := range p.Rules {
			if r.Negative {
				p.negativeRules = append(p.negativeRules, r)
			} else {
				p.positiveRules = append(p.positiveRules, r)
			}
		}
	}
	if p.Env != "" && p.Config != nil {
		p.envMatcher = p.Config.LookupVar(p.Env)
	}
	p.prepared = true
}

// Prepare materializes all matcher state before a Rule is shared by matchers.
func (p *PortRule) Prepare() {
	if p == nil || p.prepared {
		return
	}
	for _, sub := range p.Rules {
		sub.Prepare()
	}
	p.postHandle()
}

func (p *PortRule) _matchWithoutNegative(i int) bool {
	if p == nil {
		return false
	}
	if p.Any {
		return true
	}

	if len(p.portRanges) > 0 {
		if containsPortRange(p.portRanges, i) {
			return true
		}
	} else if len(p.Ports) > 0 {
		for _, pInt := range p.Ports {
			if pInt == i {
				return true
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
	if p.Env != "" && p.Config != nil {
		if p.envMatcher != nil {
			return p.envMatcher.MatchPort(i)
		}
		if !p.Config.HasVar(p.Env) {
			log.Warnf("suricata env %s not found", p.Env)
			return false
		}
		return p.Config.MatchVar(p.Env, i)
	}
	return false
}

func (p *PortRule) Match(i int) bool {
	if p == nil {
		return false
	}
	if p.Negative {
		return !p._matchWithoutNegative(i)
	}
	return p._matchWithoutNegative(i)
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

	if len(p.portRanges) > 0 && !p.Negative {
		r := p.portRanges[rand.Intn(len(p.portRanges))]
		if r.max <= r.min {
			return uint32(r.min)
		}
		return uint32(r.min) + uint32(rand.Intn(int(r.max-r.min)+1))
	}
	if len(p.Ports) > 0 && !p.Negative {
		return uint32(p.Ports[rand.Intn(len(p.Ports))])
	}

	if p.Negative {
		for i := 0; i < 10000; i++ {
			n := rand.Intn(65535-1000) + 1000
			if p.Match(n) {
				return uint32(n)
			}
		}
		return 0
	}
	if len(p.Rules) > 0 {
		return p.Rules[rand.Intn(len(p.Rules))].GetAvailablePort()
	}
	return uint32(getHighPort())
}

func parsePortRangeExpr(raw string) (portRange, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return portRange{}, false
	}
	left, right, hasColon := strings.Cut(raw, ":")
	if !hasColon {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 || v > 65535 {
			return portRange{}, false
		}
		return portRange{min: uint16(v), max: uint16(v)}, true
	}
	lo, hi := 0, 65535
	if strings.TrimSpace(left) != "" {
		v, err := strconv.Atoi(strings.TrimSpace(left))
		if err != nil || v < 0 || v > 65535 {
			return portRange{}, false
		}
		lo = v
	}
	if strings.TrimSpace(right) != "" {
		v, err := strconv.Atoi(strings.TrimSpace(right))
		if err != nil || v < 0 || v > 65535 {
			return portRange{}, false
		}
		hi = v
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	return portRange{min: uint16(lo), max: uint16(hi)}, true
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
	r := &PortRule{Config: v.Config}
	defer r.Prepare()
	if i.Any() != nil {
		r.Any = true
		return r
	}

	if i.Environment_var() != nil {
		r.Env = strings.Trim(trim(i.Environment_var().GetText()), "${}")
		return r
	}

	if i.Negative() != nil {
		r.Negative = true
		r.Rules = append(r.Rules, v.VisitPortRule(i.Port(0).(*parser.PortContext)))
		return r
	}

	if i.Colon() == nil {
		if len(i.AllINT()) == 1 && i.INT(0) != nil {
			port := atoi(strings.TrimSpace(i.INT(0).GetText()))
			if port >= 0 && port <= 65535 {
				u := uint16(port)
				r.portRanges = []portRange{{min: u, max: u}}
			}
			return r
		}
	} else {
		if pr, ok := parsePortRangeExpr(i.GetText()); ok {
			r.portRanges = []portRange{pr}
		}
		return r
	}

	var rules []*PortRule
	for _, subRule := range i.AllPort() {
		rules = append(rules, v.VisitPortRule(subRule.(*parser.PortContext)))
	}
	r.Rules = rules
	return r
}
