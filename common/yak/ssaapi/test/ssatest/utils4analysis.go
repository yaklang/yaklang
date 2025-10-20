package ssatest

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"

	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"

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

func CheckWithFS(fs fi.FileSystem, t require.TestingT, handler func(ssaapi.Programs) error, opt ...ssaapi.Option) {
	// only in memory
	{
		var astSequence ssareducer.ASTSequenceType
		for i := 0; i < 3; i++ {
			switch i {
			case 0:
				astSequence = ssareducer.Order
			case 1:
				astSequence = ssareducer.ReverseOrder
			case 2:
				astSequence = ssareducer.OutOfOrder
			}

			prog, err := ssaapi.ParseProjectWithFS(fs, append(opt, ssaapi.WithASTOrder(astSequence))...)
			require.Nil(t, err)

			log.Infof("only in memory ")
			err = handler(prog)
			require.Nil(t, err)
		}
	}

	programID := uuid.NewString()
	fmt.Println("------------------------------DEBUG PROGRAME ID------------------------------")
	log.Info("Program ID: ", programID)
	ssadb.DeleteProgram(ssadb.GetDB(), programID)
	fmt.Println("-----------------------------------------------------------------------------")
	// parse with database
	{
		opt = append(opt, ssaapi.WithProgramName(programID))
		prog, err := ssaapi.ParseProjectWithFS(fs, opt...)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programID)
		}()
		require.Nil(t, err)
		require.NotNil(t, prog)

		log.Infof("with database ")
		err = handler(prog)
		require.Nil(t, err)
	}

	// just use database
	{
		prog, err := ssaapi.FromDatabase(programID)
		require.Nil(t, err)

		log.Infof("only use database ")
		err = handler([]*ssaapi.Program{prog})
		require.Nil(t, err)
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
		require.Nil(t, err)
		_ = prog

		log.Infof("only in memory ")
		err = handler(prog)
		require.Nil(t, err)
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
		require.Nil(t, err)
		// prog.Show()

		log.Infof("with database ")
		_ = prog
		err = handler(prog)
		require.Nil(t, err)
	}

	// just use database
	{
		prog, err := ssaapi.FromDatabase(programID)
		require.Nil(t, err)

		log.Infof("only use database ")
		err = handler(prog)
		require.Nil(t, err)
	}
}

func CheckWithNameOnlyInMemory(
	name string,
	t *testing.T, code string,
	handler func(prog *ssaapi.Program) error,
	opt ...ssaapi.Option,
) {
	// only in memory
	{
		prog, err := ssaapi.Parse(code, opt...)
		require.Nil(t, err)

		log.Infof("only in memory ")
		err = handler(prog)
		require.Nil(t, err)
	}

	programID := uuid.NewString()
	if name != "" {
		programID = name
		ssadb.DeleteProgram(ssadb.GetDB(), programID)
	}
	fmt.Println("------------------------------DEBUG PROGRAME ID------------------------------")
	log.Info("Program ID: ", programID)
	fmt.Println("-----------------------------------------------------------------------------")
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
		require.NoError(t, handler(true, nil, start))
	}

	// only in memory
	{
		start := time.Now()
		prog, err := ssaapi.Parse(code, opt...)
		require.Nil(t, err)

		log.Infof("only in memory ")
		err = handler(true, prog, start)
		require.Nil(t, err)
	}

	programID := uuid.NewString()
	ssadb.DeleteProgram(ssadb.GetDB(), programID)
	defer ssadb.DeleteProgram(ssadb.GetDB(), programID)
	// parse with database
	{
		start := time.Now()
		opt = append(opt, ssaapi.WithProgramName(programID))
		prog, err := ssaapi.Parse(code, opt...)
		require.Nil(t, err)
		log.Infof("with database ")
		err = handler(false, prog, start)
		require.Nil(t, err)
	}
}

func CheckProfileWithFS(fs fi.FileSystem, t require.TestingT, handler func(p ParseStage, prog ssaapi.Programs, start time.Time) error, opt ...ssaapi.Option) {
	// only in memory
	{
		start := time.Now()
		prog, err := ssaapi.ParseProjectWithFS(fs, opt...)
		require.Nil(t, err)

		log.Infof("only in memory ")
		err = handler(OnlyMemory, prog, start)
		require.Nil(t, err)
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
		prog, err := ssaapi.ParseProjectWithFS(fs, opt...)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programID)
		}()
		require.Nil(t, err)

		log.Infof("with database ")
		err = handler(WithDatabase, prog, start)
		require.Nil(t, err)
	}

	// just use database
	{
		start := time.Now()
		prog, err := ssaapi.FromDatabase(programID)
		require.Nil(t, err)
		log.Infof("only use database ")
		err = handler(OnlyDatabase, []*ssaapi.Program{prog}, start)
		require.Nil(t, err)
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
	_, err := ssaapi.ParseProjectWithFS(codeFS, opt...)
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

func checkRuleResult(verifyFs *sfvm.VerifyFileSystem, rule *schema.SyntaxFlowRule, result *ssaapi.SyntaxFlowResult, isStrict bool) (errs error) {
	defer func() {
		if errs != nil {
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
				fileName := entry.Name()
				builder.WriteString(fileName)
				builder.WriteString(" | ")
			}
			errs = utils.Wrapf(errs, "checkResult failed in file: %s", builder.String())
		}
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
		alert_high = 0
		alert_mid  = 0
		alert_info = 0
	)

	for _, name := range result.GetAlertVariables() {
		alertCount += len(result.GetValues(name))
		count := len(result.GetValues(name))
		if info, b := result.GetAlertInfo(name); b {
			switch info.Severity {
			case "mid", "m", "middle":
				alert_mid += count
			case "high", "h":
				alert_high += count
			case "info", "low":
				alert_info += count
			}
		}
	}
	if alertCount <= 0 {
		errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is empty"))
		return
	}
	result.Show()

	ret := verifyFs.GetExtraInfoInt("alert_min", "vuln_min", "alertMin", "vulnMin")
	if ret > 0 {
		if alertCount < ret {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_min config: %v actual got: %v", ret, alertCount))
			return
		}
	}
	maxNum := verifyFs.GetExtraInfoInt("alert_max", "vuln_max", "alertMax", "vulnMax")
	if maxNum > 0 {
		if alertCount > maxNum {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is more than alert_max config: %v actual got: %v", maxNum, alertCount))
			return
		}
	}
	num := verifyFs.GetExtraInfoInt("alert_exact", "alertExact", "vulnExact", "alert_num", "vulnNum")
	if num > 0 {
		if alertCount != num {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is not equal alert_exact config: %v, actual got: %v", num, alertCount))
			return
		}
	}
	high := verifyFs.GetExtraInfoInt("alert_high", "alertHigh", "vulnHigh")
	if high > 0 {
		if alert_high != high {
			if isStrict && high < alert_high {
				errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_high config: %v, actual got: %v", high, alert_high))
				return
			}
		}
	}
	mid := verifyFs.GetExtraInfoInt("alert_mid", "alertMid", "vulnMid")
	if mid > 0 {
		if alert_mid < mid {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_mid config: %v, actual got: %v", mid, alert_mid))
			return
		}
	}
	low := verifyFs.GetExtraInfoInt("alert_low", "alertMid", "vulnMid", "alert_info")
	if low > 0 {
		if alert_info < low {
			errs = utils.JoinErrors(errs, utils.Errorf("alert symbol table is less than alert_low config: %v, actual got: %v", low, alert_info))
			return
		}
	}

	return
}
func EvaluateVerifyFilesystemWithRule(rule *schema.SyntaxFlowRule, t require.TestingT, isStrict bool) error {

	/*
		// TODO:
			analyzer := sfanalyzer.NewSyntaxFlowAnalyzer(string(code))
			result := analyzer.Analyze()
			var err error
			for _, prob := range result.Problems {
				err = utils.JoinErrors(err, utils.Errorf("syntax flow rule analyze failed: %v", prob))
			}
	*/

	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(rule.Content)
	if err != nil {
		return err
	}
	verifyFs, err := frame.ExtractVerifyFilesystemAndLanguage()
	if err != nil {
		return err
	}

	log.Infof("unsafe filesystem start")
	if verifyFs == nil && isStrict {
		return utils.Errorf("no negative filesystem found in rule: %s", rule.RuleName)
	}

	var errs error

	for _, f := range verifyFs {
		CheckWithFS(f.GetVirtualFs(), t, func(p ssaapi.Programs) error {
			// Use the program as the init input var,so that the lib rule which have `$input` can be tested.
			result, err := p.SyntaxFlowWithError(rule.Content, ssaapi.QueryWithInitInputVar(p[0]))
			if err != nil {
				return utils.Errorf("syntax flow content failed: %v", err)
			}
			if err := checkRuleResult(f, rule, result, isStrict); err != nil {
				errs = utils.JoinErrors(errs, err)
			}

			// in db
			result2, err := p.SyntaxFlowRule(rule, ssaapi.QueryWithInitInputVar(p[0]))
			if err != nil {
				return utils.Errorf("syntax flow rule failed: %v", err)
			}
			if err := checkRuleResult(f, rule, result2, isStrict); err != nil {
				errs = utils.JoinErrors(errs, err)
			}

			return nil
		}, ssaapi.WithLanguage(f.GetLanguage()))
	}
	if errs != nil {
		return errs
	}

	check := func(result *ssaapi.SyntaxFlowResult) error {
		if len(result.GetAlertVariables()) > 0 {
			for _, name := range result.GetAlertVariables() {
				vals := result.GetValues(name)
				return utils.Errorf("alert symbol table not empty, have: %v: %v", name, vals)
			}
		}
		return nil
	}

	negativeFs, _ := frame.ExtractNegativeFilesystemAndLanguage()
	if negativeFs == nil && isStrict {
		return utils.Errorf("no negative filesystem found in rule: %s", rule.RuleName)
	}
	log.Infof("safe filesystem start")
	if isStrict {
		for _, f := range negativeFs {
			CheckWithFS(f.GetVirtualFs(), t, func(programs ssaapi.Programs) error {
				result, err := programs.SyntaxFlowWithError(rule.Content, ssaapi.QueryWithEnableDebug(), ssaapi.QueryWithInitInputVar(programs[0]))
				if err != nil {
					return utils.Errorf("syntax flow content failed: %v", err)
				}
				if err := check(result); err != nil {
					errs = utils.JoinErrors(errs, err)
					return nil
				}
				result2, err := programs.SyntaxFlowRule(rule, ssaapi.QueryWithEnableDebug(), ssaapi.QueryWithInitInputVar(programs[0]))
				if err != nil {
					return utils.Errorf("syntax flow content failed: %v", err)
				}
				if err := check(result2); err != nil {
					errs = utils.JoinErrors(errs, err)
					return nil
				}
				return nil
			}, ssaapi.WithLanguage(f.GetLanguage()))
		}
	}
	if errs != nil {
		return errs
	}

	return nil
}

func EvaluateVerifyFilesystem(i string, t require.TestingT, isStrict bool) error {
	frame, err := sfvm.CompileRule(i)
	if err != nil {
		return err
	}

	return EvaluateVerifyFilesystemWithRule(frame.GetRule(), t, isStrict)
}
