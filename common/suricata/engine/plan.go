package engine

import (
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/suricata/match"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

type CompiledRulePlan struct {
	Index     uint64
	Rule      *rule.Rule
	Matcher   *match.Matcher
	Flow      *rule.FlowRule
	Options   []rule.StateOption
	Threshold *rule.ThresholdingConfig
	NoAlert   bool

	// matcherPool provides independent matchContexts. A Matcher serializes
	// calls through its own mutex; pooling clones lets different flows evaluate
	// the same rule concurrently.
	matcherPool sync.Pool
}

func (p *CompiledRulePlan) streamOverlap() int {
	maxLength := 0
	if p.Rule == nil || p.Rule.ContentRuleConfig == nil {
		return 0
	}
	for _, content := range p.Rule.ContentRuleConfig.ContentRules {
		if len(content.Content) > maxLength {
			maxLength = len(content.Content)
		}
	}
	if maxLength > 0 {
		return maxLength - 1
	}
	return 0
}

// CompileStatefulRule compiles a rule for the unified engine. It accepts every
// stateless rule the matcher supports; non-TCP rules that require flow state
// are rejected until non-TCP flow tracking exists.
func CompileStatefulRule(r *rule.Rule) (*CompiledRulePlan, error) {
	matcher, err := match.CompileStatefulRule(r)
	if err != nil {
		return nil, err
	}
	plan := &CompiledRulePlan{Rule: r, Matcher: matcher}
	if r.ContentRuleConfig != nil {
		plan.Flow = r.ContentRuleConfig.Flow
		plan.Threshold = r.ContentRuleConfig.Thresholding
	}
	plan.Options = append([]rule.StateOption(nil), r.StateOptions...)
	for _, option := range plan.Options {
		if option.NoAlert || option.FlowBits != nil && option.FlowBits.Operation == rule.FlowBitsNoAlert {
			plan.NoAlert = true
		}
		if option.FlowInt != nil || option.XBits != nil || option.Metric != nil {
			return nil, fmt.Errorf("state option %s is parsed but not executable in M1", option.Keyword)
		}
	}
	if err := plan.validateProtocolState(); err != nil {
		return nil, err
	}
	plan.matcherPool.New = func() any {
		return matcher.Clone()
	}
	return plan, nil
}

// validateProtocolState keeps non-TCP rules fail-closed. The matcher can
// evaluate them statelessly, but flow direction, connection state, thresholds
// and cross-packet state still require a non-TCP flow tracker that M1 does not
// have.
func (p *CompiledRulePlan) validateProtocolState() error {
	if p.Rule == nil || p.Rule.Protocol == "tcp" {
		return nil
	}
	protocol := p.Rule.Protocol
	if p.Flow != nil {
		if p.Flow.OnlyStream {
			return fmt.Errorf("%s rules cannot use only_stream", protocol)
		}
		if p.Flow.Established || p.Flow.NotEstablished || p.Flow.ToServer || p.Flow.ToClient {
			return fmt.Errorf("stateful flow tracking for %s rules is not supported; only stateless %s rules are supported", protocol, protocol)
		}
	}
	if p.Threshold != nil {
		return fmt.Errorf("threshold for %s rules requires non-TCP flow state", protocol)
	}
	for _, option := range p.Options {
		if option.FlowBits != nil || option.FlowInt != nil || option.XBits != nil || option.NoAlert {
			return fmt.Errorf("state option %s for %s rules requires non-TCP flow state", option.Keyword, protocol)
		}
	}
	return nil
}

// stateless reports whether the plan can run without a flowRuntime. Direct
// MatchPacket/MatchStream calls and packet-view inspection use this gate.
func (p *CompiledRulePlan) stateless() bool {
	if p.Threshold != nil {
		return false
	}
	if p.Flow != nil {
		if p.Flow.Established || p.Flow.NotEstablished || p.Flow.ToServer || p.Flow.ToClient || p.Flow.OnlyStream {
			return false
		}
	}
	for _, option := range p.Options {
		if option.FlowBits != nil || option.FlowInt != nil || option.XBits != nil || option.NoAlert {
			return false
		}
	}
	return true
}

func (p *CompiledRulePlan) acquireMatcher() *match.Matcher {
	if pooled, ok := p.matcherPool.Get().(*match.Matcher); ok && pooled != nil {
		return pooled
	}
	return p.Matcher.Clone()
}

func (p *CompiledRulePlan) releaseMatcher(matcher *match.Matcher) {
	if matcher == nil {
		return
	}
	matcher.Reset()
	p.matcherPool.Put(matcher)
}
