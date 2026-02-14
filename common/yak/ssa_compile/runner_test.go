package ssa_compile

import "testing"

func TestShouldCompileInMemory(t *testing.T) {
	tests := []struct {
		name string
		conf *SSADetectConfig
		want bool
	}{
		{
			name: "nil config",
			conf: nil,
			want: false,
		},
		{
			name: "nil params",
			conf: &SSADetectConfig{},
			want: false,
		},
		{
			name: "missing memory param",
			conf: &SSADetectConfig{Params: map[string]any{"foo": "bar"}},
			want: false,
		},
		{
			name: "memory true bool",
			conf: &SSADetectConfig{Params: map[string]any{"memory": true}},
			want: true,
		},
		{
			name: "memory true string",
			conf: &SSADetectConfig{Params: map[string]any{"memory": "TRUE"}},
			want: true,
		},
		{
			name: "memory false",
			conf: &SSADetectConfig{Params: map[string]any{"memory": false}},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := shouldCompileInMemory(tc.conf)
			if got != tc.want {
				t.Fatalf("shouldCompileInMemory() = %v, want %v", got, tc.want)
			}
		})
	}
}
