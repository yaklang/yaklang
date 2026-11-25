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
		{alias: "cpp", want: CPP},
		{alias: "c++", want: CPP},
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

// ECMAScript has one language value. js, ts, javascript and typescript are
// input aliases, so every accepted spelling resolves to TS and no spelling
// creates a second language.
func TestSameLanguageECMAScriptAliases(t *testing.T) {
	for _, alias := range []string{"js", "ts", "javascript", "typescript", "ecmascript"} {
		language, err := ValidateLanguage(alias)
		require.NoError(t, err, alias)
		require.Equal(t, TS, language, alias)
		require.Equal(t, TS, CanonicalLanguage(Language(alias)), alias)
	}
	require.True(t, SameLanguage(TS, TS))
	require.True(t, SameLanguage("js", "ts"))
	require.True(t, SameLanguage("javascript", TS))
	require.False(t, SameLanguage(TS, General))
	require.False(t, SameLanguage(TS, GO))
	require.False(t, SameLanguage("", TS))
}

// The engine language list names each frontend once: no js, and no second ts.
func TestSupportedLanguagesAreUnique(t *testing.T) {
	languages := GetAllSupportedLanguages()
	seen := make(map[string]bool, len(languages))
	for _, language := range languages {
		require.False(t, seen[language], "duplicate language %q", language)
		seen[language] = true
	}
	require.Contains(t, seen, string(TS))
	require.NotContains(t, seen, "js")
	require.NotContains(t, seen, "ts")
}
