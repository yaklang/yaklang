package ssatest

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"

	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type checkFunction func(*ssaapi.Program) error

type ParseStage int

const (
	OnlyMemory ParseStage = iota
	WithDatabase
	OnlyDatabase
)

func CheckWithFS(fs fi.FileSystem, t assert.TestingT, handler func(ssaapi.Programs) error, opt ...ssaapi.Option) {
	// only in memory
	{
		prog, err := ssaapi.ParseProject(fs, opt...)
		assert.Nil(t, err)

		log.Infof("only in memory ")
		err = handler(prog)
		assert.Nil(t, err)
	}

	programID := uuid.NewString()
	fmt.Println("------------------------------DEBUG PROGRAME ID------------------------------")
	log.Info("Program ID: ", programID)
	ssadb.DeleteProgram(ssadb.GetDB(), programID)
	fmt.Println("-----------------------------------------------------------------------------")
	// parse with database
	{
		opt = append(opt, ssaapi.WithProgramName(programID))
		prog, err := ssaapi.ParseProject(fs, opt...)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programID)
		}()
		assert.Nil(t, err)

		log.Infof("with database ")
		err = handler(prog)
		assert.Nil(t, err)
	}

	// just use database
	{
		prog, err := ssaapi.FromDatabase(programID)
		assert.Nil(t, err)

		log.Infof("only use database ")
		err = handler([]*ssaapi.Program{prog})
		assert.Nil(t, err)
	}
}

func CheckWithName(
	name string,
	t *testing.T, code string,
	handler func(prog *ssaapi.Program) error,
	opt ...ssaapi.Option,
) {
	// only in memory
	{
		prog, err := ssaapi.Parse(code, opt...)
		assert.Nil(t, err)

		log.Infof("only in memory ")
		err = handler(prog)
		assert.Nil(t, err)
	}

	programID := uuid.NewString()
	if name != "" {
		programID = name
		ssadb.DeleteProgram(ssadb.GetDB(), programID)
	}
	fmt.Println("------------------------------DEBUG PROGRAME ID------------------------------")
	log.Info("Program ID: ", programID)
	fmt.Println("-----------------------------------------------------------------------------")
	// parse with database
	{
		opt = append(opt, ssaapi.WithProgramName(programID))
		prog, err := ssaapi.Parse(code, opt...)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programID)
		}()
		assert.Nil(t, err)
		// prog.Show()

		log.Infof("with database ")
		err = handler(prog)
		assert.Nil(t, err)
	}

	// just use database
	{
		prog, err := ssaapi.FromDatabase(programID)
		assert.Nil(t, err)

		log.Infof("only use database ")
		err = handler(prog)
		assert.Nil(t, err)
	}
}

func Check(
	t *testing.T, code string,
	handler func(prog *ssaapi.Program) error,
	opt ...ssaapi.Option,
) {
	CheckWithName("", t, code, handler, opt...)
}

func CheckJava(
	t *testing.T, code string,
	handler func(prog *ssaapi.Program) error,
	opt ...ssaapi.Option,
) {
	opt = append(opt, ssaapi.WithLanguage(ssaapi.JAVA))
	CheckWithName("", t, code, handler, opt...)
}

func ProfileJavaCheck(t *testing.T, code string, handler func(inMemory bool, prog *ssaapi.Program, start time.Time) error, opt ...ssaapi.Option) {
	opt = append(opt, ssaapi.WithLanguage(ssaapi.JAVA))

	{
		start := time.Now()
		errListener := antlr4util.NewErrorListener()
		lexer := javaparser.NewJavaLexer(antlr.NewInputStream(code))
		lexer.RemoveErrorListeners()
		lexer.AddErrorListener(errListener)
		tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
		parser := javaparser.NewJavaParser(tokenStream)
		parser.RemoveErrorListeners()
		parser.AddErrorListener(errListener)
		parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
		ast := parser.CompilationUnit()
		_ = ast
		assert.NoError(t, handler(true, nil, start))
	}

	// only in memory
	{
		start := time.Now()
		prog, err := ssaapi.Parse(code, opt...)
		assert.Nil(t, err)

		log.Infof("only in memory ")
		err = handler(true, prog, start)
		assert.Nil(t, err)
	}

	programID := uuid.NewString()
	ssadb.DeleteProgram(ssadb.GetDB(), programID)
	defer ssadb.DeleteProgram(ssadb.GetDB(), programID)
	// parse with database
	{
		start := time.Now()
		opt = append(opt, ssaapi.WithProgramName(programID))
		prog, err := ssaapi.Parse(code, opt...)
		assert.Nil(t, err)
		log.Infof("with database ")
		err = handler(false, prog, start)
		assert.Nil(t, err)
	}
}

func CheckProfileWithFS(fs fi.FileSystem, t assert.TestingT, handler func(p ParseStage, prog ssaapi.Programs, start time.Time) error, opt ...ssaapi.Option) {
	// only in memory
	{
		start := time.Now()
		prog, err := ssaapi.ParseProject(fs, opt...)
		assert.Nil(t, err)

		log.Infof("only in memory ")
		err = handler(OnlyMemory, prog, start)
		assert.Nil(t, err)
	}

	programID := uuid.NewString()
	fmt.Println("------------------------------DEBUG PROGRAME ID------------------------------")
	log.Info("Program ID: ", programID)
	ssadb.DeleteProgram(ssadb.GetDB(), programID)
	fmt.Println("-----------------------------------------------------------------------------")
	// parse with database
	{
		start := time.Now()
		opt = append(opt, ssaapi.WithProgramName(programID))
		prog, err := ssaapi.ParseProject(fs, opt...)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programID)
		}()
		assert.Nil(t, err)

		log.Infof("with database ")
		err = handler(WithDatabase, prog, start)
		assert.Nil(t, err)
	}

	// just use database
	{
		start := time.Now()
		prog, err := ssaapi.FromDatabase(programID)
		assert.Nil(t, err)
		log.Infof("only use database ")
		err = handler(OnlyDatabase, []*ssaapi.Program{prog}, start)
		assert.Nil(t, err)
	}
}

func CheckFSWithProgram(
	t *testing.T, programName string,
	codeFS, ruleFS fi.FileSystem, opt ...ssaapi.Option,
) {
	if programName == "" {
		programName = "test-" + uuid.New().String()
	}
	//ssadb.DeleteProgram(ssadb.GetDB(), programName)

	opt = append(opt, ssaapi.WithProgramName(programName))
	_, err := ssaapi.ParseProject(codeFS, opt...)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	program, err := ssaapi.FromDatabase(programName)
	if err != nil {
		t.Fatalf("get program from database failed: %v", err)
	}
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
	}()
	filesys.Recursive(".", filesys.WithFileSystem(ruleFS), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		if !strings.HasSuffix(s, ".sf") {
			log.Infof("skip file: %s", s)
			return nil
		}

		t.Run(fmt.Sprintf("case in %v", s), func(t *testing.T) {
			log.Infof("start to check file: %s", s)
			raw, err := ruleFS.ReadFile(s)
			if err != nil {
				t.Fatalf("read file[%s] failed: %v", s, err)
			}
			i, err := program.SyntaxFlowWithError(string(raw))
			if err != nil {
				t.Fatalf("exec syntaxflow failed: %v", err)
			}
			if len(i.GetErrors()) > 0 {
				log.Infof("result: %s", i.String())
				t.Fatalf("result has errors: %v", i.GetErrors())
			}
		})

		return nil
	}))
}

func CheckSyntaxFlowPrintWithPhp(t *testing.T, code string, wants []string) {
	checkSyntaxFlowEx(t, code, `println(* #-> * as $param)`, true, map[string][]string{"param": wants}, []ssaapi.Option{ssaapi.WithLanguage(ssaapi.PHP)}, nil)
}
func CheckSyntaxFlowContain(t *testing.T, code string, sf string, wants map[string][]string, opt ...ssaapi.Option) {
	checkSyntaxFlowEx(t, code, sf, true, wants, opt, nil)
}

func CheckSyntaxFlowWithFS(t *testing.T, fs fi.FileSystem, sf string, wants map[string][]string, contain bool, opt ...ssaapi.Option) {
	CheckWithFS(fs, t, func(p ssaapi.Programs) error {
		for _, program := range p {
			program.Show()
		}
		results, err := p.SyntaxFlowWithError(sf, sfvm.WithEnableDebug())
		assert.Nil(t, err)
		assert.NotNil(t, results)
		CompareResult(t, contain, results, wants)
		return nil
	}, opt...)
}

func CheckSyntaxFlow(t *testing.T, code string, sf string, wants map[string][]string, opt ...ssaapi.Option) {
	checkSyntaxFlowEx(t, code, sf, false, wants, opt, nil)
}

func CheckSyntaxFlowEx(t *testing.T, code string, sf string, contain bool, wants map[string][]string, opt ...ssaapi.Option) {
	checkSyntaxFlowEx(t, code, sf, contain, wants, opt, nil)
}

func CheckSyntaxFlowWithSFOption(t *testing.T, code string, sf string, wants map[string][]string, opt ...sfvm.Option) {
	checkSyntaxFlowEx(t, code, sf, false, wants, nil, opt)
}

func checkSyntaxFlowEx(t *testing.T, code string, sf string, contain bool, wants map[string][]string, ssaOpt []ssaapi.Option, sfOpt []sfvm.Option) {
	Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		sfOpt = append(sfOpt, sfvm.WithEnableDebug(true))
		results, err := prog.SyntaxFlowWithError(sf, sfOpt...)
		assert.Nil(t, err)
		assert.NotNil(t, results)
		CompareResult(t, contain, results, wants)
		return nil
	}, ssaOpt...)
}

func CompareResult(t *testing.T, contain bool, results *ssaapi.SyntaxFlowResult, wants map[string][]string) {
	results.Show()
	for k, want := range wants {
		gotVs := results.GetValues(k)
		assert.Greater(t, len(gotVs), 0, "key[%s] not found", k)
		got := lo.Map(gotVs, func(v *ssaapi.Value, _ int) string { return v.String() })
		sort.Strings(got)
		sort.Strings(want)
		if contain {
			// every want should be found in got
			for _, containSubStr := range want {
				match := false
				// should contain at least one
				for _, g := range got {
					if strings.Contains(g, containSubStr) {
						match = true
					}
				}
				if !match {
					t.Errorf("want[%s] not found in got[%v]", want, got)
				}
			}
		} else {
			assert.Equal(t, len(want), len(gotVs))
			assert.Equal(t, want, got)
		}
	}
}

func CheckBottomUser_Contain(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *ssaapi.Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			func() ssaapi.Values {
				return p.Ref(variable)
			},
			func(v *ssaapi.Value) ssaapi.Values { return v.GetBottomUses() },
			checkLength, want,
			func(v1 *ssaapi.Value, v2 string) bool {
				return strings.Contains(v1.String(), v2)
			},
		)
	}
}

func CheckBottomUserCall_Contain(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *ssaapi.Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			func() ssaapi.Values {
				lastIndex := strings.LastIndex(variable, ".")
				if lastIndex != -1 {
					member := variable[:lastIndex]
					key := variable[lastIndex+1:]
					return p.Ref(member).Ref(key)
				} else {
					return p.Ref(variable)
				}
			},
			func(v *ssaapi.Value) ssaapi.Values { return v.GetBottomUses() },
			checkLength, want,
			func(v1 *ssaapi.Value, v2 string) bool {
				return strings.Contains(v1.String(), v2)
			},
		)
	}
}

func CheckTopDef_Contain(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *ssaapi.Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			func() ssaapi.Values {
				return p.Ref(variable)
			},
			func(v *ssaapi.Value) ssaapi.Values { return v.GetTopDefs() },
			checkLength, want,
			func(v1 *ssaapi.Value, v2 string) bool {
				return strings.Contains(v1.String(), v2)
			},
		)
	}
}

func CheckTopDef_Equal(variable string, want []string, forceCheckLength ...bool) checkFunction {
	return func(p *ssaapi.Program) error {
		checkLength := false
		if len(forceCheckLength) > 0 && forceCheckLength[0] {
			checkLength = true
		}
		return checkFunctionEx(
			func() ssaapi.Values {
				return p.Ref(variable)
			},
			func(v *ssaapi.Value) ssaapi.Values { return v.GetTopDefs() },
			checkLength, want,
			func(v1 *ssaapi.Value, v2 string) bool {
				return v1.String() == v2
			},
		)
	}
}

func checkFunctionEx(
	variable func() ssaapi.Values, // variable  for test
	get func(*ssaapi.Value) ssaapi.Values, // getTop / getBottom
	checkLength bool,
	want []string,
	compare func(*ssaapi.Value, string) bool,
) error {
	values := variable()
	if len(values) != 1 {
		return fmt.Errorf("variable[%s] not len(1): %d", values, len(values))
	}
	value := values[0]
	vs := get(value)
	vs = lo.UniqBy(vs, func(v *ssaapi.Value) int64 { return v.GetId() })
	if checkLength {
		if len(vs) != len(want) {
			err := fmt.Errorf("variable[%v] got:%d: %v vs want: %d:%v", values, len(vs), vs, len(want), want)
			log.Info(err)
			return err
		}
	}
	mark := make([]bool, len(want))
	for _, value := range vs {
		log.Infof("value: %s", value.String())
		for j, w := range want {
			mark[j] = mark[j] || compare(value, w)
		}
	}
	for i, m := range mark {
		if !m {
			return fmt.Errorf("want[%d] %s not found", i, want[i])
		}
	}
	return nil
}

func checkResult(frame *sfvm.SFFrame, rule *schema.SyntaxFlowRule, result *ssaapi.SyntaxFlowResult) (errs error) {
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
		alert_high = 0
		alert_mid  = 0
		alert_info = 0
	)

	for _, name := range result.GetAlertVariables() {
		alertCount += len(result.GetValues(name))
		if info, b := result.GetAlertInfo(name); b {
			switch info.Severity {
			case "mid", "m", "middle":
				alert_mid++
			case "high", "h":
				alert_high++
			case "info", "low":
				alert_info++
			}
		}
	}
	if alertCount <= 0 {
		errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is empty"))
		return
	}
	result.Show()

	ret := frame.GetExtraInfoInt("alert_min", "vuln_min", "alertMin", "vulnMin")
	if ret > 0 {
		if alertCount < ret {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_min config: %v actual got: %v", ret, alertCount))
			return
		}
	}
	maxNum := frame.GetExtraInfoInt("alert_max", "vuln_max", "alertMax", "vulnMax")
	if maxNum > 0 {
		if alertCount > maxNum {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is more than alert_max config: %v actual got: %v", maxNum, alertCount))
			return
		}
	}
	high := frame.GetExtraInfoInt("alert_high", "alertHigh", "vulnHigh")
	if high > 0 {
		if alert_high < high {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_high config: %v, actual got: %v", high, alert_high))
			return
		}
	}
	mid := frame.GetExtraInfoInt("alert_mid", "alertMid", "vulnMid")
	if mid > 0 {
		if alert_mid < mid {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_mid config: %v, actual got: %v", mid, alert_mid))
			return
		}
	}
	low := frame.GetExtraInfoInt("alert_low", "alertMid", "vulnMid", "alert_info")
	if low > 0 {
		if alert_info < low {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_low config: %v, actual got: %v", low, alert_info))
			return
		}
	}
	exact := frame.GetExtraInfoInt("alert_exact", "alertExact", "vulnExact", "alert_num", "vulnNum")
	if exact > 0 {
		if alert_info != exact {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is not equal alert_exact config: %v, actual got: %v", exact, alert_info))
			return
		}
	}
	return
}
func EvaluateVerifyFilesystemWithRule(rule *schema.SyntaxFlowRule, t *testing.T) error {
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(rule.Content)
	if err != nil {
		return err
	}
	l, vfs, err := frame.ExtractVerifyFilesystemAndLanguage()
	if err != nil {
		return err
	}
	log.Infof("unsafe filesystem start")
	CheckWithFS(vfs, t, func(p ssaapi.Programs) error {
		result, err := p.SyntaxFlowWithError(rule.Content)
		if err != nil {
			return utils.Errorf("syntax flow content failed: %v", err)
		}
		if err := checkResult(frame, rule, result); err != nil {
			return err
		}

		// in db
		result2, err := p.SyntaxFlowRule(rule)
		if err != nil {
			return utils.Errorf("syntax flow rule failed: %v", err)
		}
		if err := checkResult(frame, rule, result2); err != nil {
			return err
		}

		return nil
	}, ssaapi.WithLanguage(l))

	check := func(result *ssaapi.SyntaxFlowResult) error {
		if len(result.GetAlertVariables()) > 0 {
			for _, name := range result.GetAlertVariables() {
				vals := result.GetValues(name)
				return utils.Errorf("alert symbol table not empty, have: %v: %v", name, vals)
			}
		}
		return nil
	}

	l, vfs, _ = frame.ExtractNegativeFilesystemAndLanguage()
	if vfs != nil && l != "" {
		log.Infof("safe filesystem start")
		CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
			result, err := programs.SyntaxFlowWithError(rule.Content, sfvm.WithEnableDebug())
			if err != nil {
				return utils.Errorf("syntax flow content failed: %v", err)
			}
			if err := check(result); err != nil {
				return utils.Errorf("check content failed: %v", err)
			}
			result2, err := programs.SyntaxFlowRule(rule, sfvm.WithEnableDebug())
			if err != nil {
				return utils.Errorf("syntax flow rule failed: %v", err)
			}
			if err := check(result2); err != nil {
				return utils.Errorf("check rule failed: %v", err)
			}
			return nil
		})
	}

	return nil
}

func EvaluateVerifyFilesystem(i string, t assert.TestingT) error {
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(i)
	if err != nil {
		return err
	}
	l, vfs, err := frame.ExtractVerifyFilesystemAndLanguage()
	if err != nil {
		return err
	}

	var errs error
	CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		result, err := programs.SyntaxFlowWithError(i, sfvm.WithEnableDebug(false))
		if err != nil {
			errs = utils.JoinErrors(errs, err)
			return err
		}
		if err := checkResult(frame, frame.GetRule(), result); err != nil {
			errs = utils.JoinErrors(errs, err)
		}
		return nil
	}, ssaapi.WithLanguage(l))
	if (errs) != nil {
		return errs
	}

	l, vfs, _ = frame.ExtractNegativeFilesystemAndLanguage()
	if vfs != nil && l != "" {
		CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
			result, err := programs.SyntaxFlowWithError(i, sfvm.WithEnableDebug(false))
			if err != nil {
				if errors.Is(err, sfvm.CriticalError) {
					errs = utils.JoinErrors(errs, err)
					return err
				}
			}
			if result != nil {
				if len(result.GetErrors()) > 0 {
					return nil
				}
				if len(result.GetAlertVariables()) > 0 {
					for _, name := range result.GetAlertVariables() {
						vals := result.GetValues(name)
						errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table not empty, have: %v: %v", name, vals))
					}
				}
			}
			return nil
		})
	}

	if errs != nil {
		return errs
	}

	return nil
}
