package sfpattern

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

// The blanket filter "*" means "code and configuration". A manifest lists
// dependencies, so a credential-shaped line inside package.json is not a
// finding; a rule that wants the manifest asks for it by name.
func TestBlanketFilterSkipsManifests(t *testing.T) {
	require.False(t, compilePathMatcher("*")("package.json"))
	require.False(t, compilePathMatcher("*")("web/package.json"))
	require.False(t, compilePathMatcher("*")("app/Package.JSON"))
	require.False(t, compilePathMatcher("*.*")("pom.xml"))
	require.True(t, compilePathMatcher("*")("src/app.go"))
	require.True(t, compilePathMatcher("*")(".env"))
	require.True(t, compilePathMatcher("*")("config/application.yml"))

	explicit := compilePathMatcher("*package.json")
	require.True(t, explicit("package.json"))
	require.True(t, explicit("web/package.json"))

	frames := sourceFrames(t, `${*}.pattern_regex(/"password"\s*:/) as $hit
alert $hit`)
	plan, err := NewSourceScanPlan(frames)
	require.NoError(t, err)
	defer plan.Close()
	files := map[string]string{
		"package.json":     `{"scripts": {"password": "x"}}`,
		"web/package.json": `{"password": "y"}`,
		"src/config.json":  `{"password": "z"}`,
	}
	runtime := plan.Start(context.Background(), files)
	defer runtime.Close()
	root := NewRoot(files)
	root.SetFileFilterMatcher(runtime.Matcher(context.Background()))

	blanket, err := root.FileFilter("*", "regexp", nil, []string{`"password"\s*:`})
	require.NoError(t, err)
	require.Equal(t, 1, sfvm.ValuesLen(blanket), "only the non-manifest file reports")

	explicitValues, err := root.FileFilter("*package.json", "regexp", nil, []string{`"password"\s*:`})
	require.NoError(t, err)
	require.Equal(t, 2, sfvm.ValuesLen(explicitValues), "an explicit manifest filter still reads both manifests")
}
