package ssaapi_test

import (
	"testing"

	"github.com/stretchr/testify/require"
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

		// no set language
		prog, err = ssaapi.Parse(code, ssaapi.WithRawLanguage(""))
		require.Nil(t, prog)
		require.Error(t, err)
		require.Equal(t, "language is empty, please set language", err.Error())

		// unsupported language
		prog, err = ssaapi.Parse(code, ssaapi.WithRawLanguage("ja"))
		require.Nil(t, prog)
		require.Error(t, err)
		require.Equal(t, "unsupported language: ja", err.Error())

	})
}
