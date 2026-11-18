package ssaconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONConfigNormalizesLanguageAlias(t *testing.T) {
	tests := []struct {
		name string
		opt  Option
	}{
		{
			name: "raw config",
			opt:  WithJsonRawConfig([]byte(`{"BaseInfo":{"language":"go"}}`)),
		},
		{
			name: "config json",
			opt:  WithConfigJson(`{"BaseInfo":{"language":"go"}}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := New(ModeAll, tt.opt)
			require.NoError(t, err)
			require.Equal(t, GO, cfg.GetLanguage())
		})
	}
}

func TestJSONConfigNormalizesNewLanguageAliases(t *testing.T) {
	tests := []struct {
		alias string
		want  Language
	}{
		{alias: "lua", want: LUA},
		{alias: "rs", want: RUST},
		{alias: "kT", want: KOTLIN},
		{alias: "sc", want: SCALA},
	}
	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			cfg, err := New(ModeAll, WithJsonRawConfig([]byte(`{"BaseInfo":{"language":"`+tt.alias+`"}}`)))
			require.NoError(t, err)
			require.Equal(t, tt.want, cfg.GetLanguage())
		})
	}
}

func TestJSONConfigRejectsUnknownLanguage(t *testing.T) {
	_, err := New(ModeAll, WithJsonRawConfig([]byte(`{"BaseInfo":{"language":"cobol"}}`)))
	require.Error(t, err)
}
