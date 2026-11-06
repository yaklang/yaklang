package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// ensureLanguageBuilder maps a decided language onto its frontend. The language
// itself is decided once, by the project inspector.
func TestEnsureLanguageBuilderUsesDecidedLanguage(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("webapp/index.jsp", "<html></html>")

	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.JAVA), WithProgramPath("."))
	require.NoError(t, err)
	cfg.ensureLanguageBuilder()
	require.NotNil(t, cfg.LanguageBuilder)
	require.Equal(t, ssaconfig.JAVA, cfg.LanguageBuilder.GetLanguage())
}

// ensureLanguageBuilder never guesses. Without a decided language the builder
// stays nil, so the per-file fallback in checkLanguage keeps applying — the
// compile must not turn a tree into Java because it happens to hold a jar.
func TestEnsureLanguageBuilderDoesNotGuess(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("webapp/WEB-INF/lib/app.jar", "not really a jar")
	vf.AddFile("webapp/index.jsp", "<html></html>")

	cfg, err := DefaultConfig(WithFileSystem(vf), WithProgramPath("."))
	require.NoError(t, err)
	cfg.ensureLanguageBuilder()
	require.Nil(t, cfg.LanguageBuilder, "an undecided language must not become Java")
}

// An already resolved builder is never replaced.
func TestEnsureLanguageBuilderKeepsExistingBuilder(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("webapp/index.jsp", "<html></html>")

	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.PHP), WithProgramPath("."))
	require.NoError(t, err)
	existing := cfg.LanguageBuilder
	cfg.ensureLanguageBuilder()
	require.NotNil(t, cfg.LanguageBuilder, "the configured language resolves a builder")
	cfg.LanguageBuilder = existing
	cfg.ensureLanguageBuilder()
	require.Same(t, existing, cfg.LanguageBuilder)
}
