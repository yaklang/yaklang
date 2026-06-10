package ssa_compile

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestShouldCompileInMemory(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ssaconfig.Config
		want bool
	}{
		{
			name: "nil",
			cfg:  nil,
			want: false,
		},
		{
			name: "empty config",
			cfg:  &ssaconfig.Config{},
			want: false,
		},
		{
			name: "memory true",
			cfg: &ssaconfig.Config{
				SSACompile: &ssaconfig.SSACompileConfig{
					MemoryCompile: true,
				},
			},
			want: true,
		},
		{
			name: "memory false",
			cfg: &ssaconfig.Config{
				SSACompile: &ssaconfig.SSACompileConfig{
					MemoryCompile: false,
				},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := shouldCompileInMemory(tc.cfg)
			if got != tc.want {
				t.Fatalf("shouldCompileInMemory() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCompilePluginResultCapturesCompileFailure(t *testing.T) {
	var result compilePluginResult

	require.NoError(t, result.handle(newCompilePluginLogExecResult(t, "text", "编译错误信息:\nroot cause")))
	require.NoError(t, result.handle(newCompilePluginLogExecResult(t, "error", "编译失败")))

	require.Empty(t, result.programName)
	require.Contains(t, result.failureText(), "编译错误信息")
	require.Contains(t, result.failureText(), "root cause")
	require.Contains(t, result.failureText(), "编译失败")
}

func TestCompilePluginResultParsesProgramName(t *testing.T) {
	var result compilePluginResult

	require.NoError(t, result.handle(newCompilePluginLogExecResult(t, "code", `{"program_name":"program-a"}`)))

	require.Equal(t, "program-a", result.programName)
	require.Empty(t, result.failureText())
}

func newCompilePluginLogExecResult(t *testing.T, level, data string) *ypb.ExecResult {
	t.Helper()

	raw, err := json.Marshal(map[string]any{
		"type": "log",
		"content": map[string]any{
			"level": level,
			"data":  data,
		},
	})
	require.NoError(t, err)

	return &ypb.ExecResult{
		IsMessage: true,
		Message:   raw,
	}
}
