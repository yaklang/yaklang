package ssa_bootstrapping

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type CheckKind int

const (
	invalid CheckKind = iota
	Checked
	NotChecked
)

var (
	ruleCache = utils.NewTTLCache[*schema.SyntaxFlowRule](time.Minute * 10)
)

type InputRule struct {
	RuleName    string
	RuleContent string
}
type RuleChecker struct {
	Name        string
	ConfigInfo  *ssaconfig.CodeSourceInfo
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

	//可能一行有多个
	Number int
}

func (r *RiskInfo) check(progName string) error {
	db := consts.GetGormDefaultSSADataBase().Debug()
	target := filepath.Join("/", progName, r.FileName)
	db = yakit.FilterSSARisk(db, &ypb.SSARisksFilter{
		ProgramName:   []string{progName},
		Severity:      []string{string(r.Severity)},
		FromRule:      []string{r.RuleName},
		CodeSourceUrl: []string{target},
	})
	db = bizhelper.ExactQueryString(db, "variable", r.RiskVariable)
	db = bizhelper.ExactQueryInt64(db, "line", r.Line)

	var risks []*schema.SSARisk
	if result := db.Find(&risks); result.Error != nil {
		log.Errorf("find risk error: %v", result.Error)
		return result.Error
	}
	exist := false
	for _, ssaRisk := range risks {
		rangeIf := ssaapi.CodeRange{}
		if err := json.Unmarshal([]byte(ssaRisk.CodeRange), &rangeIf); err != nil {
			log.Errorf("code range unmarshal fail: %v", err)
			continue
		}
		if rangeIf.StartLine != r.StartLine || rangeIf.EndLine != r.EndLine || rangeIf.StartColumn != r.StartColumn || rangeIf.EndColumn != r.EndColumn {
			continue
		}
		exist = true
		break
	}
	if !exist {
		return utils.Errorf("not found match result")
	}
	return nil
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
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progName)
	}()
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

	for _, info := range s.RiskInfo {
		if err := info.check(progName); err != nil {
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

func startCase(checker []RuleChecker) error {
	var errResult string
	startTime := time.Now()
	for _, ruleChecker := range checker {
		if err := ruleChecker.run(); err != nil {
			errResult += fmt.Sprintf("%s\n", err.Error())
		}
	}
	log.Infof("time Duration: %v", time.Now().Sub(startTime).Seconds())
	if errResult != "" {
		return fmt.Errorf("error: %s", errResult)
	}
	return nil
}
