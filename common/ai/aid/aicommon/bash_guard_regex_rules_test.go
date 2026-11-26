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
		require.Equal(t, bashGuardMatchRegex, rules[0].Match)
		require.Equal(t, "deny", rules[0].Action)
		require.NotNil(t, rules[0].Disabled)
		require.True(t, *rules[0].Disabled)
		require.NotNil(t, rules[2].Disabled)
		require.True(t, *rules[2].Disabled)
	})

	t.Run("rejects-broken-json", func(t *testing.T) {
		_, err := ParseBashGuardRegexRules("{not an array")
		require.Error(t, err)
	})
}

func TestCompileBashGuardRegexRules(t *testing.T) {
	enabled := false
	disabled := true
	rules := compileBashGuardRegexRules([]BashGuardRegexRule{
		{Pattern: "ls", Action: "deny", Match: "exact", Disabled: &enabled},
		{Pattern: "git", Action: "ask", Match: "prefix", Disabled: &enabled},
		{Pattern: `rm\s+-rf\s+/`, Action: "warn", Match: "regex", Disabled: &enabled},
		{Pattern: "ignored", Action: "deny", Match: "exact", Disabled: &disabled},
		{Pattern: "(unclosed", Action: "deny", Match: "regex", Disabled: &enabled},
	})
	require.Len(t, rules, 3)
	require.Equal(t, schema.SFR_ACTION_DENY, rules[0].action)
	require.Equal(t, schema.SFR_ACTION_ASK, rules[1].action)
	require.Equal(t, schema.SFR_ACTION_WARN, rules[2].action)

	require.True(t, rules[0].matches("ls"))
	require.False(t, rules[0].matches("ls -la"))

	require.True(t, rules[1].matches("git status"))
	require.True(t, rules[1].matches("git"))
	require.False(t, rules[1].matches("gitlab"))

	texts := bashCommandTexts("echo ok && rm -rf /tmp/x")
	matched := false
	for _, text := range texts {
		if rules[2].matches(text) {
			matched = true
			break
		}
	}
	require.True(t, matched, "regex should match a parsed subcommand: %v", texts)
}

func TestBashGuardRegexActionDefaults(t *testing.T) {
	require.Equal(t, schema.SFR_ACTION_DENY, bashGuardRegexAction(""))
	require.Equal(t, schema.SFR_ACTION_DENY, bashGuardRegexAction("typo"))
	require.Equal(t, schema.SFR_ACTION_ASK, bashGuardRegexAction("ASK"))
	require.Equal(t, schema.SFR_ACTION_WARN, bashGuardRegexAction("warn"))
	require.Equal(t, bashGuardMatchExact, bashGuardRegexMatchMode("exact"))
	require.Equal(t, bashGuardMatchPrefix, bashGuardRegexMatchMode("prefix"))
	require.Equal(t, bashGuardMatchRegex, bashGuardRegexMatchMode(""))
}
