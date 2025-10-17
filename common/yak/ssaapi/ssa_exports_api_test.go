package ssaapi_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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
