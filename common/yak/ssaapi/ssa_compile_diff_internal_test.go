package ssaapi

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// diffWalkPaths returns the set of paths Recursive actually visits in the
// given VirtualFS, so tests can assert which entries survived the diff walk.
func diffWalkPaths(t *testing.T, vfs *filesys.VirtualFS) map[string]bool {
	t.Helper()
	paths := make(map[string]bool)
	err := filesys.Recursive(".", filesys.WithFileSystem(vfs), filesys.WithFileStat(func(pathname string, _ fs.FileInfo) error {
		if pathname == "" {
			return nil
		}
		paths[pathname] = true
		return nil
	}))
	require.NoError(t, err)
	return paths
}

// TestCalculateFileSystemDiff_RespectsUserExclude pins the regression where
// the incremental diff walked the tree without any exclusion predicates: a
// user-excluded directory (e.g. build artifacts) leaked into the diff as
// "added" files and triggered a full recompile of a clean tree.
func TestCalculateFileSystemDiff_RespectsUserExclude(t *testing.T) {
	baseFS := filesys.NewVirtualFs()
	baseFS.AddFile("src/Main.java", "public class Main {}")

	newFS := filesys.NewVirtualFs()
	newFS.AddFile("src/Main.java", "public class Main {}")
	// user-excluded dir: only present on the new side, must not be reported
	newFS.AddFile("target/generated.java", "public class Generated {}")

	// java whitelist: only .java is in the diff domain
	isSourceFile := resolveDiffSourceFilter(ssaconfig.JAVA, "")
	require.NotNil(t, isSourceFile)
	exclude := ssaconfig.BuildCompileExcludeFunc([]string{"target/**"}, ".", true)
	diffFS, fileHashMap, err := calculateFileSystemDiff(baseFS, newFS, exclude, nil, isSourceFile)
	require.NoError(t, err)
	require.NotNil(t, diffFS)
	require.Empty(t, fileHashMap["target/generated.java"])
	// an entirely clean tree yields an empty diff
	require.Empty(t, fileHashMap)
	require.Empty(t, diffWalkPaths(t, diffFS))
}

// TestCalculateFileSystemDiff_LanguageWhitelistDomain pins the diff domain =
// language whitelist (FilterFile ∪ FilterPreHandlerFile), applied
// symmetrically to both sides: out-of-domain entries are ignored on the new
// side (no phantom additions) and on the base side (no phantom deletions —
// the raw-disk fallback baseFS regression), while in-domain files still
// diff normally and the empty-file skip shared with ScanProjectFiles stays.
func TestCalculateFileSystemDiff_LanguageWhitelistDomain(t *testing.T) {
	// base side carries out-of-domain noise (raw fallback baseFS shape)
	baseFS := filesys.NewVirtualFs()
	baseFS.AddFile("src/Main.java", "public class Main {}")
	baseFS.AddFile("libs/dependency.jar", "PK\x03\x04raw")
	baseFS.AddFile("README.md", "docs")

	newFS := filesys.NewVirtualFs()
	newFS.AddFile("src/Main.java", "public class Main {}")
	newFS.AddFile("src/Helper.java", "public class Helper {}")
	// new-side out-of-domain entries: raw archives and non-source files
	newFS.AddFile("libs/webapp.war", "PK\x03\x04raw")
	newFS.AddFile("bundle.zip", "PK\x03\x04raw")
	newFS.AddFile("docs/notes.md", "notes")
	// empty files are skipped by the shared skipCompileFile predicate
	newFS.AddFile("empty.java", "")

	isSourceFile := resolveDiffSourceFilter(ssaconfig.JAVA, "")
	require.NotNil(t, isSourceFile)
	require.True(t, isSourceFile("src/Main.java"))
	require.False(t, isSourceFile("libs/dependency.jar"))
	require.False(t, isSourceFile("docs/notes.md"))

	diffFS, fileHashMap, err := calculateFileSystemDiff(baseFS, newFS, nil, nil, isSourceFile)
	require.NoError(t, err)
	require.NotNil(t, diffFS)
	require.Equal(t, 1, fileHashMap["src/Helper.java"], "in-domain addition must be reported")
	for _, ignored := range []string{
		// new side: out-of-domain entries are not additions
		"libs/webapp.war",
		"bundle.zip",
		"docs/notes.md",
		"empty.java",
		// base side: out-of-domain entries are not deletions
		"libs/dependency.jar",
		"README.md",
	} {
		require.Empty(t, fileHashMap[ignored], "out-of-domain entry leaked into diff: %s", ignored)
	}
	require.Equal(t, map[string]int{"src/Helper.java": 1}, fileHashMap)
	require.Equal(t, map[string]bool{"src/Helper.java": true}, diffWalkPaths(t, diffFS))
}

// TestCalculateFileSystemDiff_PreHandlerFileInDomain pins the union part of
// the domain predicate: pre-handler files (e.g. python's .yaml) build editors
// and persist ir_sources rows, so a FilterFile-only whitelist would
// phantom-delete them on every increment.
func TestCalculateFileSystemDiff_PreHandlerFileInDomain(t *testing.T) {
	baseFS := filesys.NewVirtualFs()
	baseFS.AddFile("app.py", "print(1)")
	baseFS.AddFile("config.yaml", "a: 1")

	newFS := filesys.NewVirtualFs()
	newFS.AddFile("app.py", "print(1)")
	newFS.AddFile("config.yaml", "a: 1")

	// python whitelist union: .py (FilterFile) plus .yaml (FilterPreHandlerFile)
	isSourceFile := resolveDiffSourceFilter(ssaconfig.PYTHON, "")
	require.NotNil(t, isSourceFile)
	require.True(t, isSourceFile("config.yaml"))

	diffFS, fileHashMap, err := calculateFileSystemDiff(baseFS, newFS, nil, nil, isSourceFile)
	require.NoError(t, err)
	require.NotNil(t, diffFS)
	// unchanged tree: the pre-handler file must NOT be reported as deleted
	require.Empty(t, fileHashMap)
	require.Empty(t, diffWalkPaths(t, diffFS))
}

// TestCalculateFileSystemDiff_FallbackWithoutLanguage pins the nil-filter
// fallback: when the language can't be resolved, the new side still skips
// raw archives via the blacklist (isRawArchiveDiffEntry) so the original
// "raw jar floods the diff compile back to a full one" incident can't recur.
func TestCalculateFileSystemDiff_FallbackWithoutLanguage(t *testing.T) {
	baseFS := filesys.NewVirtualFs()
	baseFS.AddFile("src/Main.java", "public class Main {}")

	newFS := filesys.NewVirtualFs()
	newFS.AddFile("src/Main.java", "public class Main {}")
	newFS.AddFile("libs/dependency.jar", "PK\x03\x04raw")
	newFS.AddFile("README.md", "docs")

	diffFS, fileHashMap, err := calculateFileSystemDiff(baseFS, newFS, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, diffFS)
	require.Empty(t, fileHashMap["libs/dependency.jar"], "raw archive leaked into diff")
	// no language → no whitelist: non-archive noise (README.md) keeps the
	// legacy added-then-rejected behavior, only archives are blacklisted
	require.Equal(t, 1, fileHashMap["README.md"])
	require.Equal(t, map[string]bool{"README.md": true}, diffWalkPaths(t, diffFS))
}

// assertWalkPaths returns the set of paths a plain Recursive walk visits on
// the given FS: on an archive-expansion view the jar Stats as a directory
// and the walk descends into it (com/example/Inside.java); without
// expansion the jar stays an opaque file the walk cannot enter.
func assertWalkPaths(t *testing.T, walkFS fi.FileSystem) map[string]bool {
	t.Helper()
	paths := make(map[string]bool)
	err := filesys.Recursive(".", filesys.WithFileSystem(walkFS), filesys.WithFileStat(func(pathname string, _ fs.FileInfo) error {
		if pathname == "" {
			return nil
		}
		paths[pathname] = true
		return nil
	}))
	require.NoError(t, err)
	return paths
}

// TestSwapLanguageFs_DisableArchiveExpansion pins the incremental compile
// contract at the FS level: with disableArchiveExpansion the language
// preprocessing must NOT turn archives into enterable directories (the walk
// never sees archive-interior entries), while a normal full compile expands
// them per jar_recursive_parse.
func TestSwapLanguageFs_DisableArchiveExpansion(t *testing.T) {
	// build a real zip on disk so the archive scan in MaybeWrapExpandedArchiveFS
	// finds something to wrap
	dir := t.TempDir()
	jarPath := filepath.Join(dir, "lib.jar")
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, err := zw.Create("com/example/Inside.class")
	require.NoError(t, err)
	_, err = w.Write([]byte("\xca\xfe\xba\xbe"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(jarPath, buf.Bytes(), 0o644))

	newConfig := func() *Config {
		cfg, err := DefaultConfig(
			WithLanguage(ssaconfig.JAVA),
			WithLocalFs(dir),
		)
		require.NoError(t, err)
		require.NotNil(t, cfg.LanguageBuilder)
		return cfg
	}

	// sanity: both sides start from the same wrapped cfg.fs; only the flag
	// differs, so the delta below is attributable to it alone
	require.False(t, newConfig().disableArchiveExpansion)

	// full compile, default jar_recursive_parse=true: the jar becomes an
	// enterable directory and the walk sees the in-archive class (mapped to
	// .java by the preprocessor's UnifiedFS ext map)
	wrapped := newConfig().swapLanguageFs(newConfig().fs)
	walked := assertWalkPaths(t, wrapped)
	require.Contains(t, walked, "lib.jar/com/example/Inside.java", "full compile should expand archive, walked %v", walked)

	// incremental sequence member: jar/zip stay opaque — no ExpandedZipFS,
	// no in-archive entries; only the raw jar file is visible
	cfg := newConfig()
	cfg.disableArchiveExpansion = true
	raw := cfg.swapLanguageFs(cfg.fs)
	_, isExpanded := raw.(*javaclassparser.ExpandedZipFS)
	require.False(t, isExpanded, "incremental compile must not wrap fs into archive expansion view, got %T", raw)
	inner, err := raw.Stat("lib.jar")
	require.NoError(t, err)
	require.False(t, inner.IsDir(), "with archive expansion disabled the jar must stay an opaque file, got %T", raw)
	walked = assertWalkPaths(t, raw)
	require.Contains(t, walked, "lib.jar")
	require.NotContains(t, walked, "lib.jar/com/example/Inside.java", "incremental compile must keep archives un-expanded, walked %v", walked)
}
