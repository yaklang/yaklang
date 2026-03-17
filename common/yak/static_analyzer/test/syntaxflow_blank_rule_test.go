package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak"
)

func TestStaticAnalyze_SyntaxFlow_BlankRule_NoError(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		results := yak.StaticAnalyze("", yak.WithStaticAnalyzePluginType("syntaxflow"))
		require.Empty(t, results)
	})

	t.Run("whitespace", func(t *testing.T) {
		results := yak.StaticAnalyze(" \n\t ", yak.WithStaticAnalyzePluginType("syntaxflow"))
		require.Empty(t, results)
	})
}
