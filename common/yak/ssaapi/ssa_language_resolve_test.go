package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestResolveLanguageBuilderWithoutConfiguredLanguage is the coremail case: a
// target compiled without an explicit language used to reach the filesystem
// wrap with a nil builder, so WrapWithPreprocessedFS (the jar/zip expansion)
// never ran and every archive member stayed invisible.
func TestResolveLanguageBuilderWithoutConfiguredLanguage(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("webapp/index.jsp", "<html><body>hi</body></html>")

	cfg, err := DefaultConfig(WithFileSystem(vf), WithProgramPath("."))
	require.NoError(t, err)
	require.Nil(t, cfg.LanguageBuilder, "no language configured, no builder yet")

	cfg.resolveLanguageBuilder(vf)
	require.NotNil(t, cfg.LanguageBuilder, "the wrap needs a builder to expand archives")
	require.Equal(t, ssaconfig.JAVA, cfg.LanguageBuilder.GetLanguage())
}

// A configured language wins over probing.
func TestResolveLanguageBuilderUsesConfiguredLanguage(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.go", "package main")

	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.GO), WithProgramPath("."))
	require.NoError(t, err)
	cfg.resolveLanguageBuilder(vf)
	require.NotNil(t, cfg.LanguageBuilder)
	require.Equal(t, ssaconfig.GO, cfg.LanguageBuilder.GetLanguage())
}

// A tree with nothing recognizable keeps the builder nil so the per-file
// fallback in checkLanguage still applies. .txt is claimed by a pre-handler but
// says nothing about the project's language.
func TestResolveLanguageBuilderLeavesUnknownTreeAlone(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("notes.txt", "hello")
	vf.AddFile("config.json", "{}")

	cfg, err := DefaultConfig(WithFileSystem(vf), WithProgramPath("."))
	require.NoError(t, err)
	cfg.resolveLanguageBuilder(vf)
	require.Nil(t, cfg.LanguageBuilder)
}

// A Java webapp carries a lot of JavaScript. The bundled .js must not outvote
// the .jsp pages, or the archive wrap is skipped for a Java project.
func TestResolveLanguageBuilderPrefersJavaOverBundledJS(t *testing.T) {
	vf := filesys.NewVirtualFs()
	for _, name := range []string{"static/a.js", "static/b.js", "static/c.js"} {
		vf.AddFile(name, "var a = 1;")
	}
	vf.AddFile("webapp/WEB-INF/views/index.jsp", "<html></html>")

	cfg, err := DefaultConfig(WithFileSystem(vf), WithProgramPath("."))
	require.NoError(t, err)
	cfg.resolveLanguageBuilder(vf)
	require.NotNil(t, cfg.LanguageBuilder)
	require.Equal(t, ssaconfig.JAVA, cfg.LanguageBuilder.GetLanguage())
}

// An already resolved builder is never replaced.
func TestResolveLanguageBuilderKeepsExistingBuilder(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("webapp/index.jsp", "<html></html>")

	cfg, err := DefaultConfig(WithFileSystem(vf), WithLanguage(ssaconfig.PHP), WithProgramPath("."))
	require.NoError(t, err)
	existing := cfg.LanguageBuilder
	cfg.resolveLanguageBuilder(vf)
	require.Same(t, existing, cfg.LanguageBuilder)
}
