package sfvm_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func CheckFormatDesc(t *testing.T, input string, expected string) {
	result, err := sfvm.FormatRule(input, sfvm.RuleFormatWithRuleID("id"))
	require.NoError(t, err)
	require.Equal(t, expected, result)
}

func TestFormatDesc(t *testing.T) {
	log.SetLevel(log.DebugLevel)

	t.Run("test empty description", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc() 
`,
			`
desc(
	rule_id: "id"
)
`,
		)
	})

	t.Run("test description ", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc(
	Title: "title",
)
`,
			`
desc(
	Title: "title"
	rule_id: "id"
)
`,
		)
	})

	t.Run("test description with rule id not add ", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc(
	Title: "title",
	rule_id: "id",
)
`,
			`
desc(
	Title: "title"
	rule_id: "id"
)
`,
		)
	})

	t.Run("test normal", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc() 
a #-> *as  b
`,
			`
desc(
	rule_id: "id"
)
a #-> *as  b
`,
		)
	})

	t.Run("multiple desc", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc() 
desc()
`,
			`
desc(
	rule_id: "id"
)
desc(
)
`,
		)
	})

	t.Run("multiple desc with {}", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc() 
desc{}
`,
			`
desc(
	rule_id: "id"
)
desc(
)
`,
		)
	})

	t.Run("multiple desc line in empty desc", func(t *testing.T) {
		CheckFormatDesc(t,
			`
desc(
)
desc{
}
`,
			`
desc(
	rule_id: "id"
)
desc(
)
`,
		)
	})
}
func TestName(t *testing.T) {

}

func TestRealFormatCheck(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	t.Run("duplicate desc", func(t *testing.T) {

	})
}
