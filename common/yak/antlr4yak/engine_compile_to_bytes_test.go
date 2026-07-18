package antlr4yak

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalcHashTracksIncludedSource(t *testing.T) {
	includePath := filepath.Join(t.TempDir(), "dependency.yak")
	require.NoError(t, os.WriteFile(includePath, []byte(`value = "first"`), 0o600))
	code := fmt.Sprintf("include %q\nprintln(value)", includePath)

	first := calcHash(code, nil)
	require.NoError(t, os.WriteFile(includePath, []byte(`value = "second"`), 0o600))
	second := calcHash(code, nil)

	require.NotEqual(t, first, second, "changing an included file must invalidate the parent yakc cache")
}

func TestCalcHashIgnoresIncludeTextOutsideStatements(t *testing.T) {
	code := `println("include") // include "missing.yak"`
	require.Empty(t, includeCacheMaterial(code, make(map[string]struct{})))
}
