package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// archiveOnlyBuilder is a frontend that can open an archive but has no
// preparation step of its own.
type archiveOnlyBuilder struct {
	ssa.Builder
	opened ssa.ArchiveCodeSource
}

func (b *archiveOnlyBuilder) PrepareFS(src ssa.ArchiveCodeSource) (fi.FileSystem, error) {
	b.opened = src
	vf := filesys.NewVirtualFs()
	vf.AddFile("A.java", "class A {}")
	return vf, nil
}

// A jar is the Java archive format, so the config settles the language even
// when the caller named none. Without that, opening the archive would need a
// frontend the pipeline is not allowed to name itself.
func TestArchiveCodeSourceSettlesJavaLanguage(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithCodeSourceMap(map[string]any{
		"kind":       "jar",
		"local_file": "/tmp/app.jar",
	}))
	require.NoError(t, err)
	require.Equal(t, ssaconfig.CodeSourceJar, cfg.GetCodeSourceKind())
	require.Equal(t, string(ssaconfig.JAVA), string(cfg.GetLanguage()),
		"a jar code source is Java by format")

	// The caller's language is never overridden.
	explicit, err := ssaconfig.New(ssaconfig.ModeSSACompile,
		ssaconfig.WithCodeSourceMap(map[string]any{"kind": "jar", "local_file": "/tmp/app.jar"}),
		ssaconfig.WithProjectLanguage(ssaconfig.JAVA),
	)
	require.NoError(t, err)
	require.Equal(t, string(ssaconfig.JAVA), string(explicit.GetLanguage()))

	// A zip is not a Java archive: nothing is settled for it.
	zipCfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithCodeSourceMap(map[string]any{
		"kind":       "compression",
		"local_file": "/tmp/app.zip",
	}))
	require.NoError(t, err)
	require.Empty(t, string(zipCfg.GetLanguage()), "a zip does not imply a language")
}

// The pipeline asks the frontend to open the archive; it never names one.
func TestPrepareArchiveFSDelegatesToFrontend(t *testing.T) {
	builder := &archiveOnlyBuilder{}
	c := &Config{
		Config:          &ssaconfig.Config{},
		LanguageBuilder: builder,
	}
	fsys, err := c.prepareArchiveFS("/tmp/whatever.jar")
	require.NoError(t, err)
	require.NotNil(t, fsys)
	require.Equal(t, "/tmp/whatever.jar", builder.opened.LocalFile,
		"the frontend receives the archive path instead of ssaapi opening it")
	// jar_recursive_parse defaults on, so the frontend is told to expand.
	require.True(t, builder.opened.Recursive)
}

// A frontend without archive support fails loudly instead of silently reading
// nothing.
func TestPrepareArchiveFSRejectsFrontendWithoutArchiveSupport(t *testing.T) {
	c := &Config{
		Config:          &ssaconfig.Config{},
		LanguageBuilder: &archiveOnlyBuilder{},
	}
	c.LanguageBuilder = nil
	_, err := c.prepareArchiveFS("/tmp/whatever.jar")
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot open an archive code source")
}
