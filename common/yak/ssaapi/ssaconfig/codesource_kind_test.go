package ssaconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyLocalCodeSourceKind(t *testing.T) {
	cases := map[string]CodeSourceKind{
		"/tmp/project":          CodeSourceLocal,
		"/tmp/project.zip":      CodeSourceCompression,
		"/tmp/project.ZIP":      CodeSourceCompression,
		"/tmp/app.jar":          CodeSourceJar,
		"/tmp/app.war":          CodeSourceJar,
		"/tmp/source.tar.gz":    CodeSourceLocal,
		"https://x.com/src.zip": CodeSourceLocal,
		"":                      CodeSourceLocal,
	}
	for input, want := range cases {
		require.Equal(t, want, ClassifyLocalCodeSourceKind(input), "input=%q", input)
	}
}

func TestNormalizeCodeSourceKindRestoresArchiveKinds(t *testing.T) {
	t.Run("zip local file becomes compression", func(t *testing.T) {
		cfg, err := New(ModeCodeSource,
			WithCodeSourceKind(CodeSourceLocal),
			WithCodeSourceLocalFile("/tmp/project.zip"),
		)
		require.NoError(t, err)
		require.Equal(t, CodeSourceCompression, cfg.GetCodeSourceKind())
	})

	t.Run("jar local file becomes jar", func(t *testing.T) {
		cfg, err := New(ModeCodeSource,
			WithCodeSourceKind(CodeSourceLocal),
			WithCodeSourceLocalFile("/tmp/app.jar"),
		)
		require.NoError(t, err)
		require.Equal(t, CodeSourceJar, cfg.GetCodeSourceKind())
	})

	t.Run("omitted kind is classified from path", func(t *testing.T) {
		cfg, err := New(ModeCodeSource, WithCodeSourceLocalFile("/tmp/app.jar"))
		require.NoError(t, err)
		require.Equal(t, CodeSourceJar, cfg.GetCodeSourceKind())
	})

	t.Run("real directory stays local", func(t *testing.T) {
		cfg, err := New(ModeCodeSource,
			WithCodeSourceKind(CodeSourceLocal),
			WithCodeSourceLocalFile(t.TempDir()),
		)
		require.NoError(t, err)
		require.Equal(t, CodeSourceLocal, cfg.GetCodeSourceKind())
	})

	t.Run("directory named .zip stays local", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "archive.zip")
		require.NoError(t, os.Mkdir(dir, 0o755))
		cfg, err := New(ModeCodeSource,
			WithCodeSourceKind(CodeSourceLocal),
			WithCodeSourceLocalFile(dir),
		)
		require.NoError(t, err)
		require.Equal(t, CodeSourceLocal, cfg.GetCodeSourceKind())
	})

	t.Run("explicit git kind is authoritative", func(t *testing.T) {
		cfg, err := New(ModeCodeSource,
			WithCodeSourceKind(CodeSourceGit),
			WithCodeSourceLocalFile("/tmp/repo"),
			WithCodeSourceURL("https://example.com/repo.git"),
		)
		require.NoError(t, err)
		require.Equal(t, CodeSourceGit, cfg.GetCodeSourceKind())
	})
}

func TestUpdateNormalizesCodeSourceKind(t *testing.T) {
	cfg, err := New(ModeCodeSource)
	require.NoError(t, err)
	require.NoError(t, cfg.Update(
		WithCodeSourceKind(CodeSourceLocal),
		WithCodeSourceLocalFile("/tmp/project.zip"),
	))
	require.Equal(t, CodeSourceCompression, cfg.GetCodeSourceKind())
}
