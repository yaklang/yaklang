package syntaxflow_scan

import (
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// rulePreparation belongs to one product scan, not to the process/profile DB.
// Its nested stages share parsed rules and immutable bytecode; task-local
// metadata/libraries never leak into another scan or into the profile DB.
type rulePreparation struct {
	once      sync.Once
	rules     []*schema.SyntaxFlowRule
	libraries map[string]*schema.SyntaxFlowRule
	err       error
	loader    sfvm.RuleLoader
}

func (c *Config) rulePreparationState() *rulePreparation {
	if c.rulePreparation == nil {
		c.rulePreparation = &rulePreparation{}
	}
	return c.rulePreparation
}

func (c *Config) preparedRuleInputs() ([]*schema.SyntaxFlowRule, map[string]*schema.SyntaxFlowRule, error) {
	p := c.rulePreparationState()
	p.once.Do(func() {
		if c.IsTaskLocalRuleInput() {
			p.rules, p.libraries, p.err = loadTaskLocalSyntaxFlowRules(c.SyntaxFlowRule)
			return
		}
		inputs := c.GetRuleInput()
		texts := make([]string, len(inputs))
		for i, input := range inputs {
			texts[i] = input.GetContent()
		}
		compiled, errs := sfvm.CompileRules(texts)
		for i, input := range inputs {
			if errs[i] != nil {
				log.Warnf("compile scan rule input %d: %v", i, errs[i])
				continue
			}
			rule, err := yakit.ApplySyntaxFlowRuleInput(input, compiled[i])
			if err != nil {
				log.Warnf("prepare scan rule input %d: %v", i, err)
				continue
			}
			p.rules = append(p.rules, rule)
		}
	})
	return p.rules, p.libraries, p.err
}

func withRulePreparation(p *rulePreparation) ssaconfig.Option {
	return ssaconfig.SetOption("syntaxflow-scan/rulePreparation", func(c *Config, value *rulePreparation) {
		c.rulePreparation = value
	})(p)
}
