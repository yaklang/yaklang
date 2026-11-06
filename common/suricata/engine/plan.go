package engine

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/suricata/match"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

type CompiledRulePlan struct {
	Index      uint64
	Rule       *rule.Rule
	Matcher    *match.Matcher
	Flow       *rule.FlowRule
	Tag        *rule.TagRule
	Options    []rule.StateOption
	Threshold  *rule.ThresholdingConfig
	StreamSize []*rule.StateMetricRule
	NoAlert    bool

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
// stateless rule the matcher supports, stateful HTTP over TCP, and stateful
// UDP/DNS over the UDP pseudo-flow.
func CompileStatefulRule(r *rule.Rule) (*CompiledRulePlan, error) {
	matcher, err := match.CompileStatefulRule(r)
	if err != nil {
		return nil, err
	}
	plan := &CompiledRulePlan{Rule: r, Matcher: matcher}
	if r.ContentRuleConfig != nil {
		plan.Flow = r.ContentRuleConfig.Flow
		plan.Tag = r.ContentRuleConfig.Tag
		plan.Threshold = r.ContentRuleConfig.Thresholding
	}
	plan.Options = append([]rule.StateOption(nil), r.StateOptions...)
	for _, option := range plan.Options {
		if option.NoAlert || option.FlowBits != nil && option.FlowBits.Operation == rule.FlowBitsNoAlert {
			plan.NoAlert = true
		}
		if option.Metric != nil {
			if option.Keyword != "stream_size" {
				return nil, fmt.Errorf("state option %s is parsed but not executable in M1", option.Keyword)
			}
			plan.StreamSize = append(plan.StreamSize, option.Metric)
		}
		if option.XBits != nil {
			if err := validateXBitsOption(option.XBits); err != nil {
				return nil, err
			}
		}
		if option.FlowInt != nil {
			if err := validateFlowIntOption(option.FlowInt); err != nil {
				return nil, err
			}
		}
	}
	if len(plan.StreamSize) == 0 && r.ContentRuleConfig != nil && r.ContentRuleConfig.StreamSize != nil {
		plan.StreamSize = append(plan.StreamSize, r.ContentRuleConfig.StreamSize)
	}
	if err := plan.validateProtocolState(); err != nil {
		return nil, err
	}
	plan.matcherPool.New = func() any {
		return matcher.Clone()
	}
	return plan, nil
}

func validateFlowIntOption(option *rule.StateValueOption) error {
	if option == nil {
		return fmt.Errorf("flowint option is nil")
	}
	if strings.TrimSpace(option.Name) == "" {
		return fmt.Errorf("flowint requires a name")
	}
	switch strings.ToLower(strings.TrimSpace(option.Operation)) {
	case "+", "-", "=", "==", "!=", "<", ">", "<=", ">=":
		if len(option.Arguments) != 1 {
			return fmt.Errorf("flowint %s requires exactly one integer argument", option.Operation)
		}
		if _, err := strconv.ParseInt(strings.TrimSpace(option.Arguments[0]), 10, 64); err != nil {
			return fmt.Errorf("flowint %s argument %q must be an integer", option.Operation, option.Arguments[0])
		}
	case "isset", "isnotset":
		if len(option.Arguments) != 0 {
			return fmt.Errorf("flowint %s does not accept an argument", option.Operation)
		}
	default:
		return fmt.Errorf("unsupported flowint operation %q", option.Operation)
	}
	return nil
}

func validateXBitsOption(option *rule.StateValueOption) error {
	if option == nil {
		return fmt.Errorf("xbits option is nil")
	}
	if strings.TrimSpace(option.Name) == "" {
		return fmt.Errorf("xbits requires a name")
	}
	switch option.Operation {
	case "set", "unset", "toggle", "isset", "isnotset":
	default:
		return fmt.Errorf("unsupported xbits operation %q", option.Operation)
	}
	switch strings.ToLower(strings.TrimSpace(option.Track)) {
	case "ip_src", "ip_dst", "ip_pair":
	default:
		return fmt.Errorf("xbits track %q must be ip_src, ip_dst or ip_pair", option.Track)
	}
	if option.Expire != nil && *option.Expire < 0 {
		return fmt.Errorf("xbits expire must be non-negative")
	}
	return nil
}

// validateProtocolState allows HTTP/TLS state over TCP stream reassembly,
// packet-scoped tcp-pkt state over the TCP flowRuntime, and UDP/DNS state over
// the bounded UDP pseudo-flow. Other protocols remain stateless until their
// flow tracker exists.
func (p *CompiledRulePlan) validateProtocolState() error {
	if p.Rule == nil {
		return nil
	}
	protocol := p.Rule.Protocol
	switch protocol {
	case "tcp", "http", "smtp", "ftp", "ftp-data", "smb", "ssh", "dcerpc", "tcp-stream":
		// These protocols are carried over the same TCP flowRuntime that drives
		// the handshake, direction, flowbits and threshold state.
		if len(p.StreamSize) > 0 && p.Flow != nil && (p.Flow.Stateless || p.Flow.NoStream) {
			return fmt.Errorf("%s rules cannot use stream_size with stateless/no_stream", protocol)
		}
		return nil
	case "tls":
		// TLS handshake and certificate records are parsed from the ordered
		// TCP stream so a certificate split across segments is still visible.
		if len(p.StreamSize) > 0 && p.Flow != nil && (p.Flow.Stateless || p.Flow.NoStream) {
			return fmt.Errorf("%s rules cannot use stream_size with stateless/no_stream", protocol)
		}
		return nil
	case "tcp-pkt":
		// tcp-pkt is packet-scoped by definition. stream_size uses the
		// sequence-deduplicated packet byte counters.
		if p.Flow != nil && p.Flow.OnlyStream {
			return fmt.Errorf("%s rules cannot use only_stream", protocol)
		}
		return nil
	case "ip":
		if len(p.StreamSize) > 0 {
			return fmt.Errorf("stream_size requires a TCP stream")
		}
		if p.Flow != nil {
			if p.Flow.OnlyStream || p.Flow.Established || p.Flow.NotEstablished || p.Flow.ToServer || p.Flow.ToClient {
				return fmt.Errorf("stateful flow tracking for ip rules is not supported")
			}
		}
		for _, option := range p.Options {
			if option.FlowInt != nil || option.XBits != nil {
				return fmt.Errorf("state option %s requires a TCP/UDP flow for ip rules", option.Keyword)
			}
		}
		return nil
	case "icmp", "icmpv6":
		if len(p.StreamSize) > 0 {
			return fmt.Errorf("stream_size requires a TCP stream")
		}
		if p.Flow != nil {
			if p.Flow.OnlyStream || p.Flow.Established || p.Flow.NotEstablished || p.Flow.ToServer || p.Flow.ToClient {
				return fmt.Errorf("stateful flow tracking for %s rules is not supported", protocol)
			}
		}
		for _, option := range p.Options {
			if option.FlowBits != nil || option.FlowInt != nil || option.XBits != nil {
				return fmt.Errorf("state option %s requires a TCP/UDP flow for %s rules", option.Keyword, protocol)
			}
		}
		return nil
	case "udp", "dns":
		if len(p.StreamSize) > 0 {
			return fmt.Errorf("stream_size requires a TCP stream")
		}
		if p.Flow != nil && p.Flow.OnlyStream {
			return fmt.Errorf("%s rules cannot use only_stream", protocol)
		}
		// UDP/DNS use the engine's bounded UDP pseudo-flow.
		return nil
	default:
		if p.Flow != nil {
			if p.Flow.OnlyStream {
				return fmt.Errorf("%s rules cannot use only_stream", protocol)
			}
			if p.Flow.Established || p.Flow.NotEstablished || p.Flow.ToServer || p.Flow.ToClient {
				return fmt.Errorf("stateful flow tracking for %s rules is not supported; only stateless %s rules are supported", protocol, protocol)
			}
		}
		if len(p.StreamSize) > 0 {
			return fmt.Errorf("stream_size requires a TCP stream")
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
}

// stateless reports whether the plan can run without a flowRuntime. Direct
// MatchPacket/MatchStream calls and packet-view inspection use this gate.
func (p *CompiledRulePlan) stateless() bool {
	if p.Threshold != nil || len(p.StreamSize) > 0 || p.Tag != nil {
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
