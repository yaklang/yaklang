package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestClaudeCodePermissionPolicy(t *testing.T) {
	gate, err := CompileClaudeCodePermissions(`{
		"permissions": {
			"allow": ["Bash(git *)", "Bash(ls *)", "Read(./public/**)"],
			"ask": ["Bash(git push:*)"],
			"deny": [
				"Bash(rm -rf *)",
				"Bash(git push --force *)",
				"Read(./.env)",
				"WebFetch(domain:*.example.com)",
				"mcp__danger__exec"
			]
		}
	}`)
	require.NoError(t, err)
	require.NotNil(t, gate)

	cases := []struct {
		name       string
		tool       string
		params     aitool.InvokeParams
		wantAction schema.SyntaxFlowAction
	}{
		{
			name:       "deny-rm-rf",
			tool:       "bash",
			params:     aitool.InvokeParams{"command": "rm -rf /tmp/x"},
			wantAction: schema.SFR_ACTION_DENY,
		},
		{
			name:       "deny-rm-rf-through-sudo",
			tool:       "bash",
			params:     aitool.InvokeParams{"command": "sudo rm -rf /tmp/x"},
			wantAction: schema.SFR_ACTION_DENY,
		},
		{
			name:       "deny-rm-rf-through-timeout",
			tool:       "bash",
			params:     aitool.InvokeParams{"command": "timeout 30 rm -rf /tmp/x"},
			wantAction: schema.SFR_ACTION_DENY,
		},
		{
			name:       "allow-git-status",
			tool:       "bash",
			params:     aitool.InvokeParams{"command": "git status"},
			wantAction: schema.SFR_ACTION_ALLOW,
		},
		{
			name:       "ask-git-push",
			tool:       "bash",
			params:     aitool.InvokeParams{"command": "git push origin main"},
			wantAction: schema.SFR_ACTION_ASK,
		},
		{
			name:       "deny-beats-ask",
			tool:       "bash",
			params:     aitool.InvokeParams{"command": "git push --force origin main"},
			wantAction: schema.SFR_ACTION_DENY,
		},
		{
			name:       "allow-all-compound-subcommands",
			tool:       "bash",
			params:     aitool.InvokeParams{"command": "git status && git diff"},
			wantAction: schema.SFR_ACTION_ALLOW,
		},
		{
			name:       "allow-does-not-cover-other-subcommand",
			tool:       "bash",
			params:     aitool.InvokeParams{"command": "git status && echo ok"},
			wantAction: "",
		},
		{
			name:       "allow-does-not-strip-sudo",
			tool:       "bash",
			params:     aitool.InvokeParams{"command": "sudo ls"},
			wantAction: "",
		},
		{
			name:       "allow-ls-with-arguments",
			tool:       "bash",
			params:     aitool.InvokeParams{"command": "ls -la"},
			wantAction: schema.SFR_ACTION_ALLOW,
		},
		{
			name:       "deny-read-env-absolute-windows-path",
			tool:       "read_file",
			params:     aitool.InvokeParams{"file": `C:\work\project\.env`},
			wantAction: schema.SFR_ACTION_DENY,
		},
		{
			name:       "read-env-does-not-match-suffix",
			tool:       "read_file",
			params:     aitool.InvokeParams{"file": `C:\work\project\.env.local`},
			wantAction: "",
		},
		{
			name:       "allow-read-public",
			tool:       "read_file",
			params:     aitool.InvokeParams{"file": `C:\work\project\public\a.txt`},
			wantAction: schema.SFR_ACTION_ALLOW,
		},
		{
			name:       "deny-webfetch-subdomain",
			tool:       "do_http_request",
			params:     aitool.InvokeParams{"url": "https://api.example.com/v1"},
			wantAction: schema.SFR_ACTION_DENY,
		},
		{
			name:       "webfetch-wildcard-does-not-match-apex",
			tool:       "do_http_request",
			params:     aitool.InvokeParams{"url": "https://example.com/v1"},
			wantAction: "",
		},
		{
			name:       "deny-mcp-exact-name",
			tool:       "mcp__danger__exec",
			params:     aitool.InvokeParams{},
			wantAction: schema.SFR_ACTION_DENY,
		},
		{
			name:       "mcp-does-not-match-similar-name",
			tool:       "mcp__danger__safe",
			params:     aitool.InvokeParams{},
			wantAction: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := gate(context.Background(), tc.tool, tc.params)
			if tc.wantAction == "" {
				require.Nil(t, decision)
				return
			}
			require.NotNil(t, decision)
			require.Equal(t, tc.wantAction, decision.Action)
			require.NotEmpty(t, decision.RuleID)
			require.NotEmpty(t, decision.RuleName)
		})
	}

	t.Run("deny-shell-payload-through-bash2ssa", func(t *testing.T) {
		decision := gate(context.Background(), "bash", aitool.InvokeParams{"command": "sh -c 'rm -rf /tmp/x'"})
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("deny-xargs-effective-command", func(t *testing.T) {
		decision := gate(context.Background(), "bash", aitool.InvokeParams{"command": "printf x | xargs -0 rm -rf /tmp/x"})
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("allow-must-cover-nested-payload", func(t *testing.T) {
		shGate, err := CompileClaudeCodePermissions(`{"permissions":{"allow":["Bash(sh *)"]}}`)
		require.NoError(t, err)
		decision := shGate(context.Background(), "bash", aitool.InvokeParams{"command": "sh -c 'echo ok'"})
		require.Nil(t, decision)
	})

	t.Run("command-query-is-not-an-effective-command", func(t *testing.T) {
		lsGate, err := CompileClaudeCodePermissions(`{"permissions":{"deny":["Bash(ls *)"]}}`)
		require.NoError(t, err)
		require.Nil(t, lsGate(context.Background(), "bash", aitool.InvokeParams{"command": "command -v ls"}))
	})

	t.Run("splice-configs-keeps-deny-precedence", func(t *testing.T) {
		spliced, err := CompileClaudeCodePermissions(
			`{"permissions":{"allow":["Bash(git *)"]}}`,
			`{"permissions":{"deny":["Bash(git push *)"]}}`,
		)
		require.NoError(t, err)
		require.NotNil(t, spliced)

		decision := spliced(context.Background(), "bash", aitool.InvokeParams{"command": "git push origin main"})
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("compose-cannot-relax-builtin-deny", func(t *testing.T) {
		builtin := func(context.Context, string, aitool.InvokeParams) *ToolCallRuleDecision {
			return &ToolCallRuleDecision{Action: schema.SFR_ACTION_DENY, RuleName: "builtin"}
		}
		userAllow := func(context.Context, string, aitool.InvokeParams) *ToolCallRuleDecision {
			return &ToolCallRuleDecision{Action: schema.SFR_ACTION_ALLOW, RuleName: "user-allow"}
		}

		for _, gates := range [][]ToolCallRuleGate{
			{builtin, userAllow},
			{userAllow, builtin},
		} {
			decision := ComposeToolCallRuleGates(gates...)(context.Background(), "bash", nil)
			require.NotNil(t, decision)
			require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
		}
	})

	t.Run("invalid-rule-is-rejected", func(t *testing.T) {
		_, err := ParseClaudeCodePermissions(`{"permissions":{"deny":["Bash(rm -rf *"]}}`)
		require.Error(t, err)
	})

	t.Run("rule-id-is-stable", func(t *testing.T) {
		first, err := ParseClaudeCodePermissions(`{"permissions":{"deny":["Bash(rm -rf *)"]}}`)
		require.NoError(t, err)
		second, err := ParseClaudeCodePermissions(`{"permissions":{"deny":["Bash(rm -rf *)"]}}`)
		require.NoError(t, err)
		require.Len(t, first, 1)
		require.Len(t, second, 1)
		require.Equal(t, first[0].RuleID, second[0].RuleID)
	})

	t.Run("empty-specifier-matches-every-call", func(t *testing.T) {
		gate, err := CompileClaudeCodePermissions(`{"permissions":{"deny":["Read","Write","WebFetch"]}}`)
		require.NoError(t, err)
		require.NotNil(t, gate)

		for _, tc := range []struct {
			tool   string
			params aitool.InvokeParams
		}{
			{tool: "read_file", params: aitool.InvokeParams{"file": "/tmp/a"}},
			{tool: "write_file", params: aitool.InvokeParams{"file": "/tmp/a"}},
			{tool: "do_http_request", params: aitool.InvokeParams{"url": "https://example.com"}},
		} {
			decision := gate(context.Background(), tc.tool, tc.params)
			require.NotNil(t, decision, tc.tool)
			require.Equal(t, schema.SFR_ACTION_DENY, decision.Action, tc.tool)
		}
		require.Nil(t, gate(context.Background(), "bash", aitool.InvokeParams{"command": "echo ok"}))
	})
}
