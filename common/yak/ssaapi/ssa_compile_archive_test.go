package ssaapi

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// A jar is the Java archive format, so the config settles the language even
// when the caller named none. Without that, the frontend that reads the classes
// would not be selected.
func TestJarCodeSourceSettlesJavaLanguage(t *testing.T) {
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

// jarCodeSourceConfig builds a Java compile over an existing jar file.
func jarCodeSourceConfig(t *testing.T, jarPath string, extra ...ssaconfig.Option) *Config {
	t.Helper()
	opts := append([]ssaconfig.Option{
		WithLanguage(ssaconfig.JAVA),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceJar),
		ssaconfig.WithCodeSourceLocalFile(jarPath),
	}, extra...)
	cfg, err := DefaultConfig(opts...)
	require.NoError(t, err)
	require.NotNil(t, cfg.LanguageBuilder)
	require.NotNil(t, cfg.fs)
	return cfg
}

// A jar code source is opened by the language registry, not by the pipeline: it
// arrives already readable as Java source.
func TestJarCodeSourceIsReadableThroughTheFrontend(t *testing.T) {
	jarPath := filepath.Join("test", "ssatest", "testfile", "test.jar")
	require.FileExists(t, jarPath)

	cfg := jarCodeSourceConfig(t, jarPath)
	unified, ok := cfg.fs.(*filesys.UnifiedFS)
	require.True(t, ok, "DefaultConfig normalizes the view, got %T", cfg.fs)
	_, isJarFS := unified.GetFileSystem().(*javaclassparser.JarFS)
	require.True(t, isJarFS, "the registered opener must use the jar reader, got %T",
		unified.GetFileSystem())

	langFS := cfg.swapLanguageFs(cfg.fs)
	walked := assertWalkPaths(t, langFS)
	require.Contains(t, walked, "com/java/main/Main.java",
		"class entries must be offered under source names, walked %v", walked)

	rawSrc, err := langFS.ReadFile("com/java/main/Main.java")
	require.NoError(t, err)
	require.Contains(t, string(rawSrc), "class Main",
		"the frontend must serve decompiled source, got %q", string(rawSrc))
}

// jar_recursive_parse is the "parse jars" setting. Turning it off removes
// archive files from the tree the frontend walks; an archive that is not parsed
// has no readable source, so the frontend must never be handed one.
func TestJarRecursiveParseOffSkipsArchiveFiles(t *testing.T) {
	jarBytes, err := os.ReadFile(filepath.Join("test", "ssatest", "testfile", "test.jar"))
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lib.jar"), jarBytes, 0o644))

	newConfig := func(extra ...ssaconfig.Option) *Config {
		opts := append([]ssaconfig.Option{WithLanguage(ssaconfig.JAVA), WithLocalFs(dir)}, extra...)
		cfg, err := DefaultConfig(opts...)
		require.NoError(t, err)
		return cfg
	}

	// Default: jars are parsed, so the archive becomes a directory and its
	// classes are readable as source.
	cfg := newConfig()
	langFS := cfg.swapLanguageFs(cfg.fs)
	expanded := assertWalkPaths(t, langFS)
	require.Contains(t, expanded, "lib.jar/com/java/main/Main.java",
		"an expanded jar exposes its classes, walked %v", expanded)
	info, err := langFS.Stat("lib.jar")
	require.NoError(t, err)
	require.True(t, info.IsDir(), "an expanded archive is enterable, got %T", info)

	// Parsing off: the archive is not part of the tree at all.
	off := newConfig(ssaconfig.WithCodeSourceJarRecursiveParse(false))
	walked := assertWalkPaths(t, off.swapLanguageFs(off.fs))
	require.Empty(t, walked, "a jar that is not parsed must not be walked, walked %v", walked)
	_, err = off.swapLanguageFs(off.fs).Stat("lib.jar")
	require.ErrorIs(t, err, fs.ErrNotExist, "a skipped archive must look absent")
}
