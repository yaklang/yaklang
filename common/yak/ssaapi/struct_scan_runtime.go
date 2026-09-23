package ssaapi

import (
	"context"
	"os"
	"path/filepath"
	"sort"
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
	ruleStats     map[string]*structRuleStat
	skipped       bool
	skipReason    string
}

// structRuleStat aggregates one struct-mode rule across compile units so
// 规则耗时 shows one row per rule instead of one row per package.
type structRuleStat struct {
	ruleName    string
	programName string
	startTime   int64
	endTime     int64
	riskCount   int64
	failed      bool
	err         string
}

// StructScanRuleStat is the per-rule timing ScanProject emits for 语义检测.
type StructScanRuleStat struct {
	RuleName    string
	ProgramName string
	StartTime   int64
	EndTime     int64
	RiskCount   int64
	Failed      bool
	Error       string
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
	return s.enableBuiltin || len(s.extraDirs) > 0 || len(s.extraRaw) > 0 || hasExplicitStructRules(s)
}

func hasExplicitStructRules(s *structScanRuntime) bool {
	if s == nil {
		return false
	}
	for _, rule := range s.rules {
		if rule != nil {
			return true
		}
	}
	return false
}

func (c *Config) prepareStructScan(plan *UnitPlan) error {
	if c == nil || c.structScan == nil {
		return nil
	}
	s := c.structScan
	if s.riskCB != nil && !s.wantsScan() {
		return utils.Errorf("withStructRuleCallback requires withStructRule(true|rule) or withStructRuleDir/Raw")
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
			// Builtin whole-program fallback has no package boundary.
			// Explicit withStructRule(rule)/Raw still scans that single unit.
			if s.enableBuiltin && len(s.extraRaw) == 0 && len(s.extraDirs) == 0 && !hasExplicitStructRules(s) {
				s.skipped = true
				s.skipReason = "unit:all fallback"
				log.Warnf("[struct_scan] skipped: %s", s.skipReason)
				return nil
			}
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
		start := time.Now().Unix()
		res, err := QuerySyntaxflow(
			QueryWithValue(target),
			QueryWithResultProgram(progAPI),
			QueryWithSSAConfig(progAPI.config.Config),
			QueryWithStruct(unit),
			QueryWithRuleContent(rule.Content),
			QueryWithMemory(),
			QueryWithTaskID(s.taskID),
			QueryWithContext(ruleCtx),
			QueryWithWorkBudget(budget),
		)
		cancel()
		end := time.Now().Unix()
		programName := ""
		if progAPI != nil {
			programName = strings.TrimSpace(progAPI.GetProgramName())
		}
		if err != nil {
			s.noteRule(rule, programName, start, end, 0, err)
			s.errs = append(s.errs, utils.Wrapf(err, "struct scan %s rule %s", unit.Key, rule.RuleName))
			log.Warnf("[struct_scan] unit=%s rule=%s err=%v", unit.Key, rule.RuleName, err)
			continue
		}
		if res != nil {
			s.results = append(s.results, res)
			s.ranHashes = append(s.ranHashes, ruleContentHash(rule))
			s.noteRule(rule, programName, start, end, int64(res.RiskCount()), nil)
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

func (p *Program) StructScanResults() []*SyntaxFlowResult {
	if p == nil || p.config == nil || p.config.structScan == nil {
		return nil
	}
	return p.config.structScan.results
}

// StructScanCounts is the number of struct-mode rules this compile actually
// loaded or ran, plus how many results they produced. ScanProject uses this
// for the 语义检测 stage's rule_count when the syntaxflow process monitor
// never saw those rules (they run inside compile, not StartScan).
func (p *Program) StructScanCounts() (rules int, results int) {
	if p == nil || p.config == nil || p.config.structScan == nil {
		return 0, 0
	}
	s := p.config.structScan
	rules = len(s.rules)
	if rules == 0 {
		rules = len(s.ranHashes)
	}
	return rules, len(s.results)
}

func (s *structScanRuntime) noteRule(rule *schema.SyntaxFlowRule, programName string, start, end int64, riskCount int64, err error) {
	if s == nil || rule == nil {
		return
	}
	if s.ruleStats == nil {
		s.ruleStats = map[string]*structRuleStat{}
	}
	name := strings.TrimSpace(rule.RuleName)
	if name == "" {
		return
	}
	key := name + "@" + programName
	stat := s.ruleStats[key]
	if stat == nil {
		stat = &structRuleStat{ruleName: name, programName: programName, startTime: start}
		s.ruleStats[key] = stat
	}
	if start > 0 && (stat.startTime == 0 || start < stat.startTime) {
		stat.startTime = start
	}
	elapsed := end - start
	if elapsed < 0 {
		elapsed = 0
	}
	already := stat.endTime - stat.startTime
	if already < 0 {
		already = 0
	}
	stat.endTime = stat.startTime + already + elapsed
	if riskCount > 0 {
		stat.riskCount += riskCount
	}
	if err != nil {
		stat.failed = true
		stat.err = err.Error()
	}
}

func (p *Program) StructScanRuleStats() []StructScanRuleStat {
	if p == nil || p.config == nil || p.config.structScan == nil {
		return nil
	}
	stats := p.config.structScan.ruleStats
	if len(stats) == 0 {
		return nil
	}
	out := make([]StructScanRuleStat, 0, len(stats))
	for _, stat := range stats {
		if stat == nil {
			continue
		}
		out = append(out, StructScanRuleStat{
			RuleName:    stat.ruleName,
			ProgramName: stat.programName,
			StartTime:   stat.startTime,
			EndTime:     stat.endTime,
			RiskCount:   stat.riskCount,
			Failed:      stat.failed,
			Error:       stat.err,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RuleName == out[j].RuleName {
			return out[i].ProgramName < out[j].ProgramName
		}
		return out[i].RuleName < out[j].RuleName
	})
	return out
}

func (p *Program) StructScanTaskID() string {
	if p == nil || p.config == nil || p.config.structScan == nil {
		return ""
	}
	return p.config.structScan.taskID
}

// ScanProgramStruct runs mode=struct rules on an already-compiled program.
// Each application/library is a separate unit: matches stay inside that
// program and do not cross into other applications or libraries.
func (p *Program) ScanProgramStruct(opts ...ssaconfig.Option) error {
	if p == nil || p.Program == nil {
		return utils.Error("nil program")
	}
	cfg := p.config
	if cfg == nil {
		cfg = &Config{}
		p.config = cfg
	}
	if cfg.Config == nil {
		sc, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(p.GetProgramName()))
		if err != nil {
			return err
		}
		cfg.Config = sc
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(cfg.Config); err != nil {
			return err
		}
	}
	ssaconfig.ApplyExtraOptions(cfg, cfg.Config)
	s := cfg.ensureStructScan()
	if !s.wantsScan() {
		return nil
	}
	if err := s.resolveRules(); err != nil {
		return err
	}
	if len(s.rules) == 0 {
		log.Warnf("[struct_scan] no struct rules loaded for program %s", p.GetProgramName())
		return nil
	}
	units := programStructUnits(p)
	if len(units) == 0 {
		log.Warnf("[struct_scan] program %s has no application/library unit", p.GetProgramName())
		return nil
	}
	for _, unit := range units {
		if unit == nil {
			continue
		}
		s.ScanStruct(p, unit)
	}
	s.persistAfterProgramMeta(p)
	return nil
}

func programStructUnits(prog *Program) []*ssa.CompileUnit {
	if prog == nil || prog.Program == nil {
		return nil
	}
	app := prog.Program.GetApplication()
	if app == nil {
		app = prog.Program
	}
	if len(app.CompileUnits) > 0 {
		return app.CompileUnits
	}
	var units []*ssa.CompileUnit
	if app.UpStream != nil {
		app.UpStream.ForEach(func(name string, lib *ssa.Program) bool {
			if lib == nil || lib.ProgramKind != ssa.Library {
				return true
			}
			files := libraryUnitFiles(app, name, lib)
			if len(files) == 0 {
				return true
			}
			units = append(units, &ssa.CompileUnit{
				Key:      "library:" + name,
				Files:    files,
				Language: lib.Language,
			})
			return true
		})
	}
	if len(units) > 0 {
		return units
	}
	files := fileListKeys(app)
	if len(files) == 0 {
		return nil
	}
	return []*ssa.CompileUnit{{
		Key:      "application:" + app.GetProgramName(),
		Files:    files,
		Language: app.Language,
	}}
}

func libraryUnitFiles(app *ssa.Program, name string, lib *ssa.Program) []string {
	if app != nil && app.LibraryFile != nil {
		if files := append([]string(nil), app.LibraryFile[name]...); len(files) > 0 {
			sort.Strings(files)
			return files
		}
	}
	return fileListKeys(lib)
}

func fileListKeys(prog *ssa.Program) []string {
	if prog == nil || len(prog.FileList) == 0 {
		return nil
	}
	files := make([]string, 0, len(prog.FileList))
	for path := range prog.FileList {
		if strings.TrimSpace(path) == "" {
			continue
		}
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}
