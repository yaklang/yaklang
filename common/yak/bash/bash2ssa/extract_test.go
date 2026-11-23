package bash2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractShellCommands(t *testing.T) {
	t.Run("wrapper-chain", func(t *testing.T) {
		commands, err := ExtractShellCommands("FOO=bar sudo timeout 30 rm -rf /tmp/x")
		require.NoError(t, err)
		require.NotEmpty(t, commands)
		require.Equal(t, "rm -rf /tmp/x", commands[0].EffectiveText)
		require.Contains(t, commands[0].Wrapper, "sudo")
		require.Contains(t, commands[0].Wrapper, "timeout")
	})

	t.Run("compound-command", func(t *testing.T) {
		commands, err := ExtractShellCommands("git status && rm -rf /tmp/x")
		require.NoError(t, err)
		require.Len(t, commands, 2)
		require.Equal(t, "git status", commands[0].Text)
		require.Equal(t, "rm -rf /tmp/x", commands[1].Text)
	})

	t.Run("shell-c-payload", func(t *testing.T) {
		commands, err := ExtractShellCommands("sh -c 'rm -rf /tmp/x'")
		require.NoError(t, err)
		require.Len(t, commands, 2)
		require.Equal(t, "sh -c 'rm -rf /tmp/x'", commands[0].Text)
		require.True(t, commands[1].Nested)
		require.Equal(t, "rm -rf /tmp/x", commands[1].Text)
	})

	t.Run("xargs-and-find-exec", func(t *testing.T) {
		xargs, err := ExtractShellCommands("printf x | xargs -0 rm -rf")
		require.NoError(t, err)
		require.Condition(t, func() bool {
			for _, command := range xargs {
				if command.EffectiveText == "rm -rf" {
					return true
				}
			}
			return false
		})

		find, err := ExtractShellCommands("find . -exec rm -rf {} \\;")
		require.NoError(t, err)
		require.Condition(t, func() bool {
			for _, command := range find {
				if command.EffectiveText == "rm -rf {}" {
					return true
				}
			}
			return false
		})
	})

	t.Run("redirection-is-kept", func(t *testing.T) {
		commands, err := ExtractShellCommands("rm -rf /tmp/x > /tmp/log")
		require.NoError(t, err)
		require.Len(t, commands, 1)
		require.Contains(t, commands[0].Text, "> /tmp/log")
	})

	t.Run("command-query-is-not-unwrapped", func(t *testing.T) {
		commands, err := ExtractShellCommands("command -v ls")
		require.NoError(t, err)
		require.Len(t, commands, 1)
		require.Empty(t, commands[0].EffectiveText)
	})

	t.Run("unparsable-input-returns-error", func(t *testing.T) {
		_, err := ExtractShellCommands("if then")
		require.Error(t, err)
	})
}
