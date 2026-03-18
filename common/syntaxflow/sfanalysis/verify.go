package sfanalysis

import (
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type verifyConfig struct {
	requirePositive bool
	requireNegative bool
	verifyNegative  bool
}

func WithRequirePositive(v bool) VerifyOption {
	return func(c *verifyConfig) {
		c.requirePositive = v
	}
}

func WithRequireNegative(v bool) VerifyOption {
	return func(c *verifyConfig) {
		c.requireNegative = v
	}
}

func WithVerifyNegative(v bool) VerifyOption {
	return func(c *verifyConfig) {
		c.verifyNegative = v
	}
}

func WithStrictEmbeddedVerify() VerifyOption {
	return func(c *verifyConfig) {
		c.requirePositive = true
		c.requireNegative = true
		c.verifyNegative = true
	}
}

func EvaluateVerifyFilesystemWithFrame(frame *sfvm.SFFrame, opts ...VerifyOption) error {
	return runEmbeddedVerifyWithFrame(frame, opts...).Error
}

func EvaluateVerifyFilesystemWithRule(rule *schema.SyntaxFlowRule, opts ...VerifyOption) error {
	if rule == nil {
		return utils.Error("syntaxflow rule is nil")
	}
	if strings.TrimSpace(rule.Content) == "" {
		// Draft/empty rules: treat as "nothing to verify" to avoid unnecessary SSA parsing and
		// confusing error logs.
		return nil
	}
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(rule.Content)
	if err != nil {
		return err
	}
	return EvaluateVerifyFilesystemWithFrame(frame, opts...)
}

func runEmbeddedVerifyWithFrame(frame *sfvm.SFFrame, opts ...VerifyOption) *EmbeddedVerifyReport {
	cfg := verifyConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}

	report := &EmbeddedVerifyReport{}
	if frame == nil {
		report.Error = utils.Error("syntaxflow frame is nil")
		return report
	}

	rule := frame.GetRule()
	verifyFs, err := frame.ExtractVerifyFilesystemAndLanguage()
	if err != nil {
		report.Error = err
		return report
	}
	report.PositiveTestCount = len(verifyFs)
	if len(verifyFs) == 0 && cfg.requirePositive {
		report.Error = utils.Errorf("no positive filesystem found in rule: %s", rule.RuleName)
		return report
	}

	for _, f := range verifyFs {
		err = checkWithFS(f.GetVirtualFs(), func(programs ssaapi.Programs) error {
			opts := []ssaapi.QueryOption{
				ssaapi.QueryWithPrograms(programs),
				ssaapi.QueryWithFrame(frame),
				ssaapi.QueryWithInitInputVar(programs[0]),
			}
			result, err := ssaapi.QuerySyntaxflow(opts...)
			if err != nil {
				return utils.Errorf("syntax flow content failed: %v", err)
			}
			return checkPositiveResult(f, rule, result)
		}, ssaapi.WithLanguage(f.GetLanguage()))
		if err != nil {
			report.Error = err
			return report
		}
	}

	negativeFs, err := frame.ExtractNegativeFilesystemAndLanguage()
	if err != nil {
		report.Error = err
		return report
	}
	report.NegativeTestCount = len(negativeFs)
	if len(negativeFs) == 0 && cfg.requireNegative {
		report.Error = utils.Errorf("no negative filesystem found in rule: %s", rule.RuleName)
		return report
	}
	if cfg.verifyNegative {
		for _, f := range negativeFs {
			err = checkWithFS(f.GetVirtualFs(), func(programs ssaapi.Programs) error {
				queryOpts := []ssaapi.QueryOption{
					ssaapi.QueryWithPrograms(programs),
					ssaapi.QueryWithFrame(frame),
					ssaapi.QueryWithEnableDebug(),
					ssaapi.QueryWithInitInputVar(programs[0]),
				}
				result, err := ssaapi.QuerySyntaxflow(queryOpts...)
				if err != nil {
					return utils.Errorf("syntax flow content failed: %v", err)
				}
				return checkNegativeResult(result)
			}, ssaapi.WithLanguage(f.GetLanguage()))
			if err != nil {
				report.Error = err
				return report
			}
		}
	}

	report.Passed = true
	return report
}

func checkWithFS(fs fi.FileSystem, handler func(ssaapi.Programs) error, opt ...ssaconfig.Option) error {
	prog, err := ssaapi.ParseProjectWithFS(fs, opt...)
	if err != nil {
		return err
	}
	return handler(prog)
}

func checkNegativeResult(result *ssaapi.SyntaxFlowResult) error {
	if len(result.GetAlertVariables()) > 0 {
		for _, name := range result.GetAlertVariables() {
			vals := result.GetValues(name)
			return utils.Errorf("alert symbol table not empty, have: %v: %v", name, vals)
		}
	}
	return nil
}

func checkPositiveResult(verifyFs *sfvm.VerifyFileSystem, rule *schema.SyntaxFlowRule, result *ssaapi.SyntaxFlowResult) (errs error) {
	defer func() {
		if errs == nil {
			return
		}
		fs := verifyFs.GetVirtualFs()
		builder := &strings.Builder{}
		entrys, err := fs.ReadDir(".")
		if err != nil {
			return
		}
		for _, entry := range entrys {
			if entry.IsDir() {
				continue
			}
			builder.WriteString(entry.Name())
			builder.WriteString(" | ")
		}
		errs = utils.Wrapf(errs, "checkResult failed in file: %s", builder.String())
	}()

	result.Show(sfvm.WithShowAll())
	if len(result.GetErrors()) > 0 {
		for _, e := range result.GetErrors() {
			errs = utils.JoinErrors(errs, utils.Errorf("syntax flow failed: %v", e))
		}
		return utils.Errorf("syntax flow failed: %v", strings.Join(result.GetErrors(), "\n"))
	}
	if len(result.GetAlertVariables()) <= 0 {
		errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is empty"))
		return errs
	}
	if rule.AllowIncluded {
		libOutput := result.GetValues("output")
		if libOutput == nil {
			errs = utils.JoinErrors(errs, utils.Errorf("lib: %v is not exporting output in `alert`", result.Name()))
		}
		if len(libOutput) <= 0 {
			errs = utils.JoinErrors(errs, utils.Errorf("lib: %v is not exporting output in `alert` (empty result)", result.Name()))
		}
	}

	var (
		alertCount = 0
		alertHigh  = 0
		alertMid   = 0
		alertInfo  = 0
	)
	for _, name := range result.GetAlertVariables() {
		alertCount += len(result.GetValues(name))
		count := len(result.GetValues(name))
		if info, ok := result.GetAlertInfo(name); ok {
			switch info.Severity {
			case "mid", "m", "middle":
				alertMid += count
			case "high", "h":
				alertHigh += count
			case "info", "low":
				alertInfo += count
			}
		}
	}
	if alertCount <= 0 {
		errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is empty"))
		return errs
	}

	ret := verifyFs.GetExtraInfoInt("alert_min", "vuln_min", "alertMin", "vulnMin")
	if ret > 0 && alertCount < ret {
		return utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_min config: %v actual got: %v", ret, alertCount))
	}
	maxNum := verifyFs.GetExtraInfoInt("alert_max", "vuln_max", "alertMax", "vulnMax")
	if maxNum > 0 && alertCount > maxNum {
		return utils.JoinErrors(errs, utils.Errorf("alert symbol table is more than alert_max config: %v actual got: %v", maxNum, alertCount))
	}
	num := verifyFs.GetExtraInfoInt("alert_exact", "alertExact", "vulnExact", "alert_num", "vulnNum")
	if num > 0 && alertCount != num {
		return utils.JoinErrors(errs, utils.Errorf("alert symbol table is not equal alert_exact config: %v, actual got: %v", num, alertCount))
	}
	high := verifyFs.GetExtraInfoInt("alert_high", "alertHigh", "vulnHigh")
	if high > 0 && alertHigh != high {
		return utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_high config: %v, actual got: %v", high, alertHigh))
	}
	mid := verifyFs.GetExtraInfoInt("alert_mid", "alertMid", "vulnMid")
	if mid > 0 && alertMid < mid {
		return utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_mid config: %v, actual got: %v", mid, alertMid))
	}
	low := verifyFs.GetExtraInfoInt("alert_low", "alertMid", "vulnMid", "alert_info")
	if low > 0 && alertInfo < low {
		return utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_low config: %v, actual got: %v", low, alertInfo))
	}
	return errs
}
