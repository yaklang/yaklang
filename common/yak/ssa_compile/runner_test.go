package ssa_compile

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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
