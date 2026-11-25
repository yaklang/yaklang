package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
)

func TestParseBashGuardRegexRules(t *testing.T) {
	t.Run("empty-means-unused", func(t *testing.T) {
		rules, err := ParseBashGuardRegexRules("  ")
		require.NoError(t, err)
		require.Empty(t, rules)
	})

	t.Run("decodes-rows", func(t *testing.T) {
		raw := `[
			{"name":"block-rm-root","pattern":"rm\\s+-rf\\s+/","action":"deny","match":"regex","description":"禁止删根"},
			{"pattern":"curl\\s+http://","action":"ask"},
			{"pattern":"chmod\\s+777","action":"warn","disabled":true}
		]`
		rules, err := ParseBashGuardRegexRules(raw)
		require.NoError(t, err)
		require.Len(t, rules, 3)
		require.Equal(t, "block-rm-root", rules[0].Name)
		require.Equal(t, `rm\s+-rf\s+/`, rules[0].Pattern)
		require.Equal(t, bashCustomRegexMatchRegex, rules[0].Match)
		require.Equal(t, bashCustomRegexRuleActionDeny, rules[0].Action)
		require.NotNil(t, rules[2].Disabled)
		require.True(t, *rules[2].Disabled)
		require.NotNil(t, rules[0].Disabled)
		require.True(t, *rules[0].Disabled)
	})

	t.Run("rejects-broken-json", func(t *testing.T) {
		_, err := ParseBashGuardRegexRules("{not an array")
		require.Error(t, err)
	})
}

func TestCompileBashGuardRegexRuleToSyntaxFlow(t *testing.T) {
	enabled := false

	t.Run("exact-becomes-anchored-regex-sf", func(t *testing.T) {
		rule, err := CompileBashGuardRegexRuleToSyntaxFlow(BashGuardRegexRule{
			Name:     "deny-ls",
			Pattern:  "ls",
			Action:   bashCustomRegexRuleActionDeny,
			Match:    bashCustomRegexMatchExact,
			Disabled: &enabled,
		})
		require.NoError(t, err)
		require.NotNil(t, rule)
		require.Equal(t, schema.SFR_MODE_SSA, rule.Mode)
		require.Equal(t, "bash", string(rule.Language))
		require.Contains(t, rule.Content, `/^ls$/ as $hit`)
		require.Contains(t, rule.Content, `alert $hit`)
	})

	t.Run("prefix-becomes-prefix-regex-sf", func(t *testing.T) {
		rule, err := CompileBashGuardRegexRuleToSyntaxFlow(BashGuardRegexRule{
			Pattern:  "git status",
			Action:   bashCustomRegexRuleActionAsk,
			Match:    bashCustomRegexMatchPrefix,
			Disabled: &enabled,
		})
		require.NoError(t, err)
		require.NotNil(t, rule)
		require.Contains(t, rule.Content, `/^git status(\s|$)/ as $hit`)
		require.Equal(t, schema.SFR_SEVERITY_WARNING, rule.Severity)
	})

	t.Run("regex-keeps-user-pattern", func(t *testing.T) {
		rule, err := CompileBashGuardRegexRuleToSyntaxFlow(BashGuardRegexRule{
			Pattern:  `curl\s+http://`,
			Action:   bashCustomRegexRuleActionWarn,
			Match:    bashCustomRegexMatchRegex,
			Disabled: &enabled,
		})
		require.NoError(t, err)
		require.NotNil(t, rule)
		require.Contains(t, rule.Content, `/curl\s+http:\/\// as $hit`)
		require.Equal(t, schema.SFR_SEVERITY_LOW, rule.Severity)
	})

	t.Run("disabled-skipped", func(t *testing.T) {
		disabled := true
		rule, err := CompileBashGuardRegexRuleToSyntaxFlow(BashGuardRegexRule{
			Pattern:  "ls",
			Action:   bashCustomRegexRuleActionDeny,
			Disabled: &disabled,
		})
		require.NoError(t, err)
		require.Nil(t, rule)
	})

	t.Run("invalid-regex-errors", func(t *testing.T) {
		_, err := CompileBashGuardRegexRuleToSyntaxFlow(BashGuardRegexRule{
			Pattern:  "(unclosed",
			Action:   bashCustomRegexRuleActionDeny,
			Match:    bashCustomRegexMatchRegex,
			Disabled: &enabled,
		})
		require.Error(t, err)
	})
}
