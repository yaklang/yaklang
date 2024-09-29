package sfdb_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
)

func Test_import_rule(t *testing.T) {
	t.Run("test save same name", func(t *testing.T) {
		name := uuid.NewString() + ".sf"

		err := sfdb.ImportRuleWithoutValid(name, `
		a* as $a
		`, false)
		defer sfdb.DeleteRuleByRuleName(name)
		require.NoError(t, err)

		err2 := sfdb.ImportRuleWithoutValid(name, `
		b* as $b`, false)
		require.NoError(t, err2)

		rule, err := sfdb.GetRule(name)
		require.NoError(t, err)
		require.Contains(t, rule.Content, "$a")
	})

	t.Run("test save same content", func(t *testing.T) {
		name1 := uuid.NewString() + ".sf"
		name2 := uuid.NewString() + ".sf"

		err := sfdb.ImportRuleWithoutValid(name1, `
		a* as $a
		`, false)
		defer sfdb.DeleteRuleByRuleName(name1)
		require.NoError(t, err)

		err2 := sfdb.ImportRuleWithoutValid(name2, `
		a* as $a
		`, false)
		require.NoError(t, err2)

		rule, err := sfdb.GetRule(name1)
		require.NoError(t, err)
		require.Contains(t, rule.Content, "$a")

		rule2, err := sfdb.GetRule(name2)
		require.NoError(t, err)
		require.Contains(t, rule2.Content, "$a")
	})
}
