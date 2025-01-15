package ssa_bootstrapping

import (
	"embed"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"testing"
)

//go:embed ci_rule/**
var ciRules embed.FS

func TestCiRule(t *testing.T) {
	dir, err := ciRules.ReadDir("ci_rule")
	require.NoError(t, err)
	for _, entry := range dir {
		raw, err2 := ciRules.ReadFile(fmt.Sprintf("ci_rule/%s", entry.Name()))
		require.NoError(t, err2)
		_, err2 = sfdb.CheckSyntaxFlowRuleContent(string(raw))
		require.NoError(t, err2)
	}
}
