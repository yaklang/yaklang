package ssaapi_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestParseAPI(t *testing.T) {
	t.Run("test raw language", func(t *testing.T) {
		code := `
		print("hello world")
		`
		var (
			prog *ssaapi.Program
			err  error
		)
		// support language
		prog, err = ssaapi.Parse(code, ssaapi.WithRawLanguage("yak"))
		require.NoError(t, err)
		require.NotNil(t, prog)

		// support language with other word
		prog, err = ssaapi.Parse(code, ssaapi.WithRawLanguage("yaklang"))
		require.NoError(t, err)
		require.NotNil(t, prog)

		// unsupported language
		prog, err = ssaapi.Parse(code, ssaapi.WithRawLanguage("ja"))
		require.Nil(t, prog)
		require.Error(t, err)
		require.Equal(t, "unsupported language: ja", err.Error())
	})
}

func TestParseMemory(t *testing.T) {
	t.Run("memory program save in cache", func(t *testing.T) {
		code := `
		print("hello world")
		`
		var (
			prog *ssaapi.Program
			err  error
		)

		progName := uuid.NewString()
		prog, err = ssaapi.Parse(code,
			ssaapi.WithRawLanguage("yak"),
			ssaapi.WithProgramName(progName),
			ssaapi.WithMemory(),
			ssaapi.WithEnableCache(),
		)
		require.NoError(t, err)
		require.NotNil(t, prog)

		progFromCache, err := ssaapi.FromDatabase(progName)
		require.NoError(t, err)
		require.Equal(t, progName, progFromCache.GetProgramName())

		progFromDB, err := ssa.GetProgram(progName, ssa.Application)
		require.Error(t, err)
		_ = progFromDB
	})
}

func TestParseWithStopOnCliCheck(t *testing.T) {
	t.Run("test WithStopOnCliCheck truncates code before cli.check()", func(t *testing.T) {
		code := `
entry := cli.FileNames("entry", cli.setVerboseName("项目入口文件"), cli.setCliGroup("compile"))
strictMode = cli.Bool("StrictMode", cli.setVerboseName("严格模式"), cli.setCliGroup("compile"), cli.setDefault(false))
reCompile := cli.Bool("re-compile", cli.setVerboseName("是否重新编译"), cli.setCliGroup("compile"), cli.setDefault(true))
cli.check()
afterCliCheck = "this should not be parsed"
someComplexCode = "this should not be parsed either"
`

		prog, err := ssaapi.Parse(code,
			ssaapi.WithRawLanguage("yak"),
			ssaconfig.WithStopOnCliCheck(true),
		)
		require.NoError(t, err)
		require.NotNil(t, prog)

		entryRefs := prog.Ref("entry")
		require.NotEmpty(t, entryRefs, "entry should be found before cli.check()")

		strictModeRefs := prog.Ref("strictMode")
		require.NotEmpty(t, strictModeRefs, "strictMode should be found before cli.check()")

		reCompileRefs := prog.Ref("reCompile")
		require.NotEmpty(t, reCompileRefs, "reCompile should be found before cli.check()")

		afterCliCheckRefs := prog.Ref("afterCliCheck")
		require.Empty(t, afterCliCheckRefs, "afterCliCheck should not be found as it's after cli.check()")

		someComplexCodeRefs := prog.Ref("someComplexCode")
		require.Empty(t, someComplexCodeRefs, "someComplexCode should not be found as it's after cli.check()")
	})

	t.Run("test WithStopOnCliCheck without cli.check()", func(t *testing.T) {
		code := `
entry := cli.FileNames("entry", cli.setVerboseName("项目入口文件"))
strictMode = cli.Bool("StrictMode", cli.setVerboseName("严格模式"), cli.setDefault(false))
afterCode := cli.String("afterCode", cli.setVerboseName("后续参数"))
`

		prog, err := ssaapi.Parse(code,
			ssaapi.WithRawLanguage("yak"),
			ssaconfig.WithStopOnCliCheck(true),
		)
		require.NoError(t, err)
		require.NotNil(t, prog)

		entryRefs := prog.Ref("entry")
		require.NotEmpty(t, entryRefs, "entry should be found")

		afterCodeRefs := prog.Ref("afterCode")
		require.NotEmpty(t, afterCodeRefs, "afterCode should be found when there's no cli.check()")
	})

	t.Run("test WithStopOnCliCheck with multiple cli.check()", func(t *testing.T) {
		code := `
entry := cli.FileNames("entry", cli.setVerboseName("项目入口文件"))
cli.check()
anotherCheck = cli.String("another")
cli.check()
afterSecondCheck = "this should not be parsed"
`

		prog, err := ssaapi.Parse(code,
			ssaapi.WithRawLanguage("yak"),
			ssaconfig.WithStopOnCliCheck(true),
		)
		require.NoError(t, err)
		require.NotNil(t, prog)

		entryRefs := prog.Ref("entry")
		require.NotEmpty(t, entryRefs, "entry should be found before first cli.check()")

		anotherCheckRefs := prog.Ref("anotherCheck")
		require.Empty(t, anotherCheckRefs, "anotherCheck should not be found as it's after first cli.check()")

		afterSecondCheckRefs := prog.Ref("afterSecondCheck")
		require.Empty(t, afterSecondCheckRefs, "afterSecondCheck should not be found as it's after first cli.check()")
	})

	t.Run("test WithStopOnCliCheck verifies code truncation by checking source", func(t *testing.T) {
		code := `
entry := cli.FileNames("entry")
cli.check()
afterCheck := "should not be in source"
`

		prog, err := ssaapi.Parse(code,
			ssaapi.WithRawLanguage("yak"),
			ssaconfig.WithStopOnCliCheck(true),
		)
		require.NoError(t, err)
		require.NotNil(t, prog)

		afterCheckRefs := prog.Ref("afterCheck")
		require.Empty(t, afterCheckRefs, "afterCheck should not be found")

		entryRefs := prog.Ref("entry")
		require.NotEmpty(t, entryRefs, "entry should be found")
	})

	t.Run("test WithStopOnCliCheck false does not truncate", func(t *testing.T) {
		code := `
entry := cli.FileNames("entry")
cli.check()
afterCheck := "should be in source"
`

		prog, err := ssaapi.Parse(code,
			ssaapi.WithRawLanguage("yak"),
			ssaconfig.WithStopOnCliCheck(false),
		)
		require.NoError(t, err)
		require.NotNil(t, prog)

		entryRefs := prog.Ref("entry")
		require.NotEmpty(t, entryRefs, "entry should be found")

		afterCheckRefs := prog.Ref("afterCheck")
		require.NotEmpty(t, afterCheckRefs, "afterCheck should be found when StopOnCliCheck is false")
	})

	t.Run("test WithStopOnCliCheck with cli.check() in string literal", func(t *testing.T) {
		code := `
entry := cli.FileNames("entry")
str := "some text with cli.check() in it"
cli.check()
afterCheck := cli.String("afterCheck")
`

		prog, err := ssaapi.Parse(code,
			ssaapi.WithRawLanguage("yak"),
			ssaconfig.WithStopOnCliCheck(true),
		)

		if err != nil {
			require.Contains(t, err.Error(), "syntax", "expected syntax error when truncating in string literal")
			return
		}

		require.NotNil(t, prog)

		entryRefs := prog.Ref("entry")
		require.NotEmpty(t, entryRefs, "entry should be found")

		afterCheckRefs := prog.Ref("afterCheck")
		require.Empty(t, afterCheckRefs, "afterCheck should not be found as it's after cli.check()")
	})

	t.Run("test WithStopOnCliCheck performance with large code after cli.check()", func(t *testing.T) {
		var codeBuilder strings.Builder
		codeBuilder.WriteString("entry := cli.FileNames(\"entry\")\n")
		codeBuilder.WriteString("cli.check()\n")
		for i := 0; i < 1000; i++ {
			codeBuilder.WriteString("var")
			codeBuilder.WriteString(string(rune('0' + i%10)))
			codeBuilder.WriteString(" = ")
			codeBuilder.WriteString(string(rune('0' + i%10)))
			codeBuilder.WriteString("\n")
		}

		code := codeBuilder.String()

		prog, err := ssaapi.Parse(code,
			ssaapi.WithRawLanguage("yak"),
			ssaconfig.WithStopOnCliCheck(true),
		)
		require.NoError(t, err)
		require.NotNil(t, prog)

		entryRefs := prog.Ref("entry")
		require.NotEmpty(t, entryRefs, "entry should be found")

		var0Refs := prog.Ref("var0")
		require.Empty(t, var0Refs, "var0 should not be found as it's after cli.check()")
	})
}
