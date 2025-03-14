package ssa_bootstrapping

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"path/filepath"
	"strings"
	"time"
)

type CheckKind int

const (
	invalid CheckKind = iota
	Checked
	NotChecked
)

var ruleCache = utils.NewTTLCache[*schema.SyntaxFlowRule](time.Minute * 10)

type InputRule struct {
	RuleName    string
	RuleContent string
}
type RuleChecker struct {
	Name        string
	ConfigInfo  *ssaapi.ConfigInfo
	RiskInfo    []*RiskInfo
	RuleNames   []string
	InputRules  []*InputRule
	Language    string
	ExcludeFile string

	//RequiredExclude only language !=nil to check
	RequiredExclude bool

	errors []error
}

type RiskInfo struct {
	Kind     CheckKind
	RuleName string

	//FileName {progName}/filename
	FileName    string
	Line        int64
	StartLine   int64
	EndLine     int64
	StartColumn int64
	EndColumn   int64
	Severity    schema.SyntaxFlowSeverity

	//RiskVariable is rule alert variable
	RiskVariable string
	TipContent   string

	Number int
}

func (r *RiskInfo) validate(risk *schema.SSARisk) error {
	var rangeIf ssaapi.CodeRange
	err := json.Unmarshal([]byte(risk.CodeRange), &rangeIf)
	if err != nil {
		return err
	}
	if r.StartLine != rangeIf.StartLine {
		return utils.Errorf("start line not match: %v != %v", rangeIf.StartLine, r.StartLine)
	}
	if r.EndLine != rangeIf.EndLine {
		return utils.Errorf("end line not match: %v != %v", rangeIf.EndLine, r.EndLine)
	}
	if r.StartColumn != rangeIf.StartColumn {
		return utils.Errorf("start column not match: %v != %v", rangeIf.StartColumn, r.StartColumn)
	}
	if r.EndColumn != rangeIf.EndColumn {
		return utils.Errorf("end column not match: %v != %v", rangeIf.EndColumn, r.EndColumn)
	}
	target := filepath.Join(risk.ProgramName, r.FileName)
	if target != risk.CodeSourceUrl {
		return utils.Errorf("code source url not match: %v != %v", risk.CodeSourceUrl, target)
	}
	switch r.Kind {
	case Checked:
		return nil
	case NotChecked:
		return utils.Errorf("not checked rule: %v", r.RuleName)
	default:
		return utils.Errorf("unhandled default case")
	}
}
func (s *RuleChecker) getRule() []*schema.SyntaxFlowRule {
	var rules []*schema.SyntaxFlowRule
	for _, rule := range s.InputRules {
		schemaRule, err := sfdb.CheckSyntaxFlowRuleContent(rule.RuleContent)
		if err != nil {
			log.Errorf("check syntax flow rule content error: %v", err)
			continue
		}
		schemaRule.RuleName = rule.RuleName
		rules = append(rules, schemaRule)
	}
	for _, ruleName := range s.RuleNames {
		rule, exists := ruleCache.Get(ruleName)
		if exists {
			rules = append(rules, rule)
			continue
		}
		rule, err := sfdb.GetRule(ruleName)
		if err != nil {
			log.Errorf("get rule %s error: %v", ruleName, err)
			continue
		}
		rules = append(rules, rule)
		ruleCache.Set(ruleName, rule)
	}
	if s.RuleNames == nil || len(s.RuleNames) == 0 {
		allRules, err := sfdb.GetRuleByLanguage(strings.ToLower(s.Language))
		if err != nil {
			log.Errorf("get all rules error: %v", err)
			return nil
		}
		rules = append(rules, allRules...)
		for _, rule := range allRules {
			ruleCache.Set(rule.RuleName, rule)
		}
	}
	return rules
}
func (s *RuleChecker) run() error {
	progName := uuid.NewString()
	rawConfig, err := json.Marshal(s.ConfigInfo)
	if err != nil {
		return err
	}
	if s.ConfigInfo == nil {
		return utils.Error("config info is nil")
	}
	if s.Language == "" {
		log.Infof("language is empty, need to detect")
		return utils.Error("language is empty")
	}
	opts := []ssaapi.Option{
		ssaapi.WithProgramName(progName),
		ssaapi.WithRawLanguage(s.Language),
		ssaapi.WithConfigInfoRaw(string(rawConfig)),
		ssaapi.WithProcess(func(msg string, process float64) {
			log.Infof("msg: %v", msg)
			log.Infof("process: %v", process)
		}),
	}
	var defaultExclude []string
	if s.ExcludeFile != "" {
		defaultExclude = strings.Split(s.ExcludeFile, ",")
	} else if s.RequiredExclude {
		switch strings.ToLower(s.Language) {
		case string(consts.PHP):
			defaultExclude = strings.Split("**vendor**,vendor**,lib**,**lib**", ",")
		case string(consts.JAVA):
			defaultExclude = strings.Split("**/classes/**,**/target/**", ",")
		}
	}
	excludeOptions, err := ssaapi.DefaultExcludeFunc(defaultExclude)
	if err != nil {
		return err
	}
	opts = append(opts, excludeOptions)
	opts = append(opts, ssaapi.WithSaveToProfile(true))
	_, err = ssaapi.ParseProject(opts...)
	if err != nil {
		return err
	}
	prog, err := ssaapi.FromDatabase(progName)
	if err != nil {
		return err
	}
	for _, rule := range s.getRule() {
		_, err := prog.SyntaxFlowRule(rule, ssaapi.QueryWithSave(schema.SFResultKindQuery))
		if err != nil {
			log.Errorf("check rule %s error: %v", rule.RuleName, err)
			continue
		}
	}

	db := consts.GetGormDefaultSSADataBase().Debug()
	//validate
	for _, info := range s.RiskInfo {
		target := filepath.Join(progName, info.FileName)
		db = yakit.FilterSSARisk(db, &ypb.SSARisksFilter{
			ProgramName:   []string{progName},
			Severity:      []string{string(info.Severity)},
			FromRule:      []string{info.RuleName},
			CodeSourceUrl: []string{target},
		})
		db = bizhelper.ExactQueryString(db, "variable", info.RiskVariable)
		db = bizhelper.ExactQueryInt64(db, "line", info.Line)
		var risk = &schema.SSARisk{}
		if result := db.Find(&risk); result.Error != nil {
			log.Errorf("find risk error: %v", result.Error)
			s.errors = append(s.errors, utils.Errorf("find risk error: %v,rule: %s", result.Error, info.RuleName))
			continue
		}
		if err := info.validate(risk); err != nil {
			s.errors = append(s.errors, utils.Errorf("validate risk error: %v,rule: %s", err, info.RuleName))
			continue
		}
	}
	if s.errors == nil {
		return nil
	}
	var strError string
	for _, err2 := range s.errors {
		strError = strError + "\n" + err2.Error()
	}
	return errors.New(strError)
}
