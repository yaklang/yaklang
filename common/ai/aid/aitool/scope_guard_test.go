package aitool

import (
	"os"
	"testing"
)

func TestCheckToolScope(t *testing.T) {
	target := "/tmp/eval/target"
	work := "/tmp/eval/work"
	t.Setenv(EnvAuditTargetPath, target)
	t.Setenv(EnvAuditWorkDir, work)

	cases := []struct {
		name    string
		tool    string
		params  map[string]any
		wantErr bool
	}{
		{
			name:    "read file inside target",
			tool:    "read_file",
			params:  map[string]any{"path": target + "/main.go"},
			wantErr: false,
		},
		{
			name:    "read file outside target",
			tool:    "read_file",
			params:  map[string]any{"path": "/etc/passwd"},
			wantErr: true,
		},
		{
			name:    "bash with allowed target path",
			tool:    "bash",
			params:  map[string]any{"command": "grep -r exec.Command " + target + "/plugins"},
			wantErr: false,
		},
		{
			name:    "bash with outside path",
			tool:    "bash",
			params:  map[string]any{"command": "grep -r exec.Command /home/user/other-project"},
			wantErr: true,
		},
		{
			name:    "bash with system binary only",
			tool:    "bash",
			params:  map[string]any{"command": "/usr/bin/python3 -c 'print(1)'"},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckToolScope(tc.tool, tc.params)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCheckToolScopeNoEnv(t *testing.T) {
	os.Unsetenv(EnvAuditTargetPath)
	os.Unsetenv(EnvAuditWorkDir)
	// Without an explicit audit scope, the guard is a no-op so other AI flows
	// are not affected.
	if err := CheckToolScope("read_file", map[string]any{"path": "/etc/passwd"}); err != nil {
		t.Fatalf("expected no restriction without explicit scope, got %v", err)
	}
}
