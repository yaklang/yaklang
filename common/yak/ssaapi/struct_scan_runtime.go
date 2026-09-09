package ssaapi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type structScanRuntime struct {
	enableBuiltin bool
	extraDirs     []string
	extraRaw      []string
	rules         []*schema.SyntaxFlowRule
	riskCB        func(*schema.SSARisk)
	taskID        string
	timeout       time.Duration
	workLimit     int64
	errs          []error
	results       []*SyntaxFlowResult
	ranHashes     []string
	skipped       bool
	skipReason    string
}

func (s *structScanRuntime) enabled() bool {
	if s == nil || s.skipped {
		return false
	}
	return len(s.rules) > 0
}

func (s *structScanRuntime) wantsScan() bool {
	if s == nil {
		return false
	}
	return s.enableBuiltin || len(s.extraDirs) > 0 || len(s.extraRaw) > 0 || len(s.rules) > 0
}

func (c *Config) prepareStructScan(plan *UnitPlan) error {
	if c == nil || c.structScan == nil {
		return nil
	}
	s := c.structScan
	if s.riskCB != nil && !s.wantsScan() {
		return utils.Errorf("withStructRuleCallback requires withStructRule(true) or withStructRuleDir/Raw")
	}
	if !s.wantsScan() {
		return nil
	}
	if strings.TrimSpace(c.GetProgramName()) == "" {
		return utils.Errorf("struct scan requires withProgramName")
	}
	if c.GetEnableIncrementalCompile() || c.GetBaseProgramName() != "" {
		s.skipped = true
		s.skipReason = "incremental compile"
		log.Warnf("[struct_scan] skipped: %s", s.skipReason)
		return nil
	}
	if plan != nil && len(plan.Units) == 1 {
		if _, ok := plan.Units["unit:all"]; ok {
			s.skipped = true
			s.skipReason = "unit:all fallback"
			log.Warnf("[struct_scan] skipped: %s", s.skipReason)
			return nil
		}
	}
	if err := s.resolveRules(); err != nil {
		return err
	}
	if len(s.rules) == 0 {
		log.Warnf("[struct_scan] no struct rules loaded")
	}
	return nil
}

func (c *Config) ensureStructScan() *structScanRuntime {
	if c.structScan == nil {
		c.structScan = &structScanRuntime{
			taskID:    uuid.NewString(),
			workLimit: ssaconfig.DefaultScanRuleWorkLimit,
		}
	}
	return c.structScan
}

func (s *structScanRuntime) resolveRules() error {
	if s == nil {
		return nil
	}
	if len(s.rules) > 0 && !s.enableBuiltin && len(s.extraDirs) == 0 && len(s.extraRaw) == 0 {
		return s.validateRules()
	}
	var rules []*schema.SyntaxFlowRule
	if s.enableBuiltin {
		rules = append(rules, loadBuiltinStructRules()...)
	}
	for _, dir := range s.extraDirs {
		loaded, err := loadStructRulesFromDir(dir)
		if err != nil {
			return err
		}
		rules = append(rules, loaded...)
	}
	for _, raw := range s.extraRaw {
		rule, err := compileStructRuleContent(raw)
		if err != nil {
			return err
		}
		rules = append(rules, rule)
	}
	s.rules = append(s.rules, rules...)
	return s.validateRules()
}

func (s *structScanRuntime) validateRules() error {
	for _, rule := range s.rules {
		if rule == nil {
			continue
		}
		if !rule.IsStructMode() {
			return utils.Errorf("non-struct rule %s (mode=%s) cannot be used with withStructRule*", rule.RuleName, schema.ValidRuleMode(rule.Mode))
		}
	}
	return nil
}

func loadBuiltinStructRules() []*schema.SyntaxFlowRule {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil
	}
	var rules []*schema.SyntaxFlowRule
	_ = yakit.ApplySyntaxFlowRuleModeFilter(db.Model(&schema.SyntaxFlowRule{}), []string{string(schema.SFR_MODE_STRUCT)}).
		Find(&rules).Error
	return rules
}

func loadStructRulesFromDir(dir string) ([]*schema.SyntaxFlowRule, error) {
	var rules []*schema.SyntaxFlowRule
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil || info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".sf") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rule, err := compileStructRuleContent(string(raw))
		if err != nil {
			log.Warnf("[struct_scan] skip %s: %v", path, err)
			return nil
		}
		if rule.RuleName == "" {
			rule.RuleName = info.Name()
		}
		rules = append(rules, rule)
		return nil
	})
	return rules, err
}

func compileStructRuleContent(raw string) (*schema.SyntaxFlowRule, error) {
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(raw)
	if err != nil {
		return nil, err
	}
	rule := frame.GetRule()
	if rule == nil {
		return nil, utils.Error("compiled struct rule has no schema")
	}
	rule.Content = raw
	rule.NormalizeMode()
	if !rule.IsStructMode() {
		return nil, utils.Errorf("rule mode %s is not struct", rule.Mode)
	}
	return rule, nil
}

func (s *structScanRuntime) ScanStruct(progAPI *Program, unit *ssa.CompileUnit) {
	if s == nil || !s.enabled() || progAPI == nil || unit == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			err := utils.Errorf("struct scan panic on %s: %v", unit.Key, r)
			log.Errorf("%v", err)
			s.errs = append(s.errs, err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	compileCtx := context.Background()
	if progAPI.config != nil && progAPI.config.ctx != nil {
		compileCtx = progAPI.config.ctx
	}
	for _, rule := range s.rules {
		if rule == nil {
			continue
		}
		ruleCtx, cancel := context.WithCancel(compileCtx)
		if s.timeout > 0 {
			ruleCtx, cancel = context.WithTimeout(compileCtx, s.timeout)
		}
		var budget *sfvm.RuleWorkBudget
		if s.workLimit > 0 {
			budget = sfvm.NewRuleWorkBudget(s.workLimit, cancel)
		}
		target := NewStructQueryTarget(progAPI, unit, newStructBound(unit, progAPI.Program))
		res, err := QuerySyntaxflow(
			QueryWithValue(target),
			QueryWithResultProgram(progAPI),
			QueryWithStruct(unit),
			QueryWithRuleContent(rule.Content),
			QueryWithMemory(),
			QueryWithTaskID(s.taskID),
			QueryWithContext(ruleCtx),
			QueryWithWorkBudget(budget),
		)
		cancel()
		if err != nil {
			s.errs = append(s.errs, utils.Wrapf(err, "struct scan %s rule %s", unit.Key, rule.RuleName))
			log.Warnf("[struct_scan] unit=%s rule=%s err=%v", unit.Key, rule.RuleName, err)
			continue
		}
		if res != nil {
			s.results = append(s.results, res)
			s.ranHashes = append(s.ranHashes, ruleContentHash(rule))
			if s.riskCB != nil {
				risks := res.GetRisks()
				if len(risks) == 0 {
					for _, name := range res.GetAlertVariables() {
						for index, val := range res.GetValues(name) {
							if risk := buildSSARisk(res, name, index, val); risk != nil {
								risks = append(risks, risk)
							}
						}
					}
				}
				for _, risk := range risks {
					s.riskCB(risk)
				}
			}
			log.Infof("[struct_scan] package=%s rule=%s alerts=%d", unit.Key, rule.RuleName, len(res.GetAlertVariables()))
		}
		progAPI.ResetInterRuleState()
	}
}

func ruleContentHash(rule *schema.SyntaxFlowRule) string {
	if rule == nil {
		return ""
	}
	if rule.Hash != "" {
		return rule.Hash
	}
	return utils.CalcSha256(rule.RuleName, rule.Content)
}

func (s *structScanRuntime) persistAfterProgramMeta(progAPI *Program) {
	if s == nil || progAPI == nil || progAPI.Program == nil {
		return
	}
	if progAPI.Program.DatabaseKind == ssa.ProgramCacheMemory {
		return
	}
	for _, res := range s.results {
		if res == nil {
			continue
		}
		if _, err := res.Save(schema.SFResultKindScan, s.taskID); err != nil {
			log.Warnf("[struct_scan] persist result failed: %v", err)
			s.errs = append(s.errs, err)
		}
	}
}

func (p *Program) StructRulesAlreadyRan(rule *schema.SyntaxFlowRule) bool {
	if p == nil || p.config == nil || p.config.structScan == nil || rule == nil {
		return false
	}
	h := ruleContentHash(rule)
	for _, ran := range p.config.structScan.ranHashes {
		if ran == h {
			return true
		}
	}
	return false
}

func (p *Program) StructScanErrors() []error {
	if p == nil || p.config == nil || p.config.structScan == nil {
		return nil
	}
	return p.config.structScan.errs
}

func (p *Program) StructScanTaskID() string {
	if p == nil || p.config == nil || p.config.structScan == nil {
		return ""
	}
	return p.config.structScan.taskID
}
