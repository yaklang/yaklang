package ssaapi

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// WithStructRule enables builtin struct rules (bool) or appends an explicit
// *schema.SyntaxFlowRule to run during compile (after LazyBuildForUnits,
// before FlushCompileUnit).
func WithStructRule(v any) ssaconfig.Option {
	switch t := v.(type) {
	case bool:
		return ssaconfig.SetOption("ssa_compile/struct_rule_enable", func(c *Config, enable bool) {
			c.ensureStructScan().enableBuiltin = enable
		})(t)
	case *schema.SyntaxFlowRule:
		return WithStructRules(t)
	case schema.SyntaxFlowRule:
		return WithStructRules(&t)
	default:
		return func(*ssaconfig.Config) error {
			return utils.Errorf("withStructRule: want bool or *schema.SyntaxFlowRule, got %T", v)
		}
	}
}

// WithStructRules appends compiled struct-mode rules to the compile-time scan.
func WithStructRules(rules ...*schema.SyntaxFlowRule) ssaconfig.Option {
	return ssaconfig.SetOption("ssa_compile/struct_rules", func(c *Config, v []*schema.SyntaxFlowRule) {
		s := c.ensureStructScan()
		s.rules = append(s.rules, v...)
	})(rules)
}

func WithStructRuleDir(dir string) ssaconfig.Option {
	return ssaconfig.SetOption("ssa_compile/struct_rule_dir", func(c *Config, v string) {
		if strings.TrimSpace(v) == "" {
			return
		}
		s := c.ensureStructScan()
		s.extraDirs = append(s.extraDirs, v)
	})(dir)
}

func WithStructRuleRaw(raw string) ssaconfig.Option {
	return ssaconfig.SetOption("ssa_compile/struct_rule_raw", func(c *Config, v string) {
		if strings.TrimSpace(v) == "" {
			return
		}
		s := c.ensureStructScan()
		s.extraRaw = append(s.extraRaw, v)
	})(raw)
}

func WithStructRuleCallback(cb func(*schema.SSARisk)) ssaconfig.Option {
	return ssaconfig.SetOption("ssa_compile/struct_rule_callback", func(c *Config, v func(*schema.SSARisk)) {
		c.ensureStructScan().riskCB = v
	})(cb)
}

func WithStructRuleTimeout(d time.Duration) ssaconfig.Option {
	return ssaconfig.SetOption("ssa_compile/struct_rule_timeout", func(c *Config, v time.Duration) {
		c.ensureStructScan().timeout = v
	})(d)
}

func WithStructRuleWorkLimit(n int64) ssaconfig.Option {
	return ssaconfig.SetOption("ssa_compile/struct_rule_work_limit", func(c *Config, v int64) {
		c.ensureStructScan().workLimit = v
	})(n)
}
