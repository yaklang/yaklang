package ssatest

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfanalysis"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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

func CheckWithFS(fs fi.FileSystem, t require.TestingT, handler func(ssaapi.Programs) error, opt ...ssaconfig.Option) {
	// only in memory
	{
		var astSequence ssareducer.ASTSequenceType
		for i := 0; i < 3; i++ {
			switch i {
			case 0:
				log.Infof("current: ssareducer.Order")
				astSequence = ssareducer.Order
			case 1:
				log.Infof("current: ssareducer.ReverseOrder")
				astSequence = ssareducer.ReverseOrder
			case 2:
				log.Infof("current: ssareducer.OutOfOrder")
				astSequence = ssareducer.OutOfOrder
			}

			prog, err := ssaapi.ParseProjectWithFS(fs, append(opt, ssaapi.WithASTOrder(astSequence))...)
			require.Nil(t, err)

			log.Infof("only in memory")
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
	opt ...ssaconfig.Option,
) {
	// only in memory
	if false {
		prog, err := ssaapi.Parse(code, opt...)
		require.Nil(t, err)
		_ = prog

		log.Infof("compiled only in memory ")
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

		log.Infof("compiled with database ")
		_ = prog
		// err = handler(prog)
		require.Nil(t, err)
	}

	// just use database
	{
		prog, err := ssaapi.FromDatabase(programID)
		require.Nil(t, err)

		log.Infof("loaded from database ")
		err = handler(prog)
		_ = prog
		require.Nil(t, err)
	}
}

func CheckWithNameOnlyInMemory(
	name string,
	t *testing.T, code string,
	handler func(prog *ssaapi.Program) error,
	opt ...ssaconfig.Option,
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
	opt ...ssaconfig.Option,
) {
	CheckWithName("", t, code, handler, opt...)
}

func CheckJava(
	t *testing.T, code string,
	handler func(prog *ssaapi.Program) error,
	opt ...ssaconfig.Option,
) {
	opt = append(opt, ssaapi.WithLanguage(ssaconfig.JAVA))
	CheckWithName("", t, code, handler, opt...)
}

func ProfileJavaCheck(t *testing.T, code string, handler func(inMemory bool, prog *ssaapi.Program, start time.Time) error, opt ...ssaconfig.Option) {
	opt = append(opt, ssaapi.WithLanguage(ssaconfig.JAVA))

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

func CheckProfileWithFS(fs fi.FileSystem, t require.TestingT, handler func(p ParseStage, prog ssaapi.Programs, start time.Time) error, opt ...ssaconfig.Option) {
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
	codeFS, ruleFS fi.FileSystem, opt ...ssaconfig.Option,
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

func EvaluateVerifyFilesystemWithRule(rule *schema.SyntaxFlowRule, t require.TestingT, isStrict bool, opts ...sfvm.Option) error {
	if isStrict {
		return sfanalysis.EvaluateVerifyFilesystemWithRule(rule, sfanalysis.WithStrictEmbeddedVerify())
	}
	return sfanalysis.EvaluateVerifyFilesystemWithRule(rule)
}

func EvaluateVerifyFilesystem(i string, t require.TestingT, isStrict bool) error {
	frame, err := sfvm.CompileRule(i)
	if err != nil {
		return err
	}

	return EvaluateVerifyFilesystemWithRule(frame.GetRule(), t, isStrict)
}
