package ssa

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompileDiagnosticsBoundedAndDetached(t *testing.T) {
	prog := &Program{}
	for i := 0; i < 100; i++ {
		prog.RecordCompileDiagnostic("ast", "Bad.java", strings.Repeat("x", 1024))
	}
	d := prog.CompileDiagnostics()
	require.Equal(t, 100, d.ASTErrors)
	require.Len(t, d.Examples, 20)
	require.Len(t, d.Examples[0].Message, 512)
	d.Examples[0].Message = "changed"
	require.NotEqual(t, "changed", prog.CompileDiagnostics().Examples[0].Message)
}

func TestYAMLDiagnosticsDistinguishTemplatesFromMalformedConfig(t *testing.T) {
	prog := &Program{ProjectConfig: make(map[string]*ProjectConfig)}
	for _, tc := range []struct{ path, content, kind string }{
		{"chart/templates/pod.yaml", "metadata:\n  {{- include \"labels\" . }}", "template"},
		{"docker-compose.yml", "volumes:\n - {local_path_DB.sql}.sql:/data/dump.sql", "config"},
	} {
		err := prog.ParseProjectConfig([]byte(tc.content), tc.path, PROJECT_CONFIG_YAML)
		var typed *ProjectConfigError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, tc.kind, typed.Kind)
	}
	// A quoted template-like string is valid YAML and must not be skipped.
	require.NoError(t, prog.ParseProjectConfig([]byte("name: '{{ value }}'"), "chart/templates/valid.yaml", PROJECT_CONFIG_YAML))
	d := prog.CompileDiagnostics()
	require.Equal(t, 1, d.TemplatesSkipped)
	require.Equal(t, 1, d.ConfigErrors)
	require.Equal(t, "{{ value }}", prog.GetProjectConfigValue("name"))
}
