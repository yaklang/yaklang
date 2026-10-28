package fp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "yak-fp-rehs-")
	if err != nil {
		panic(err)
	}
	if err = os.Setenv("YAK_MINIREHS_CACHE", dir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestServicePrefilterWritesImage(t *testing.T) {
	t.Setenv("YAK_MINIREHS_CACHE", t.TempDir())
	rules := []*NmapMatch{ruleFromLine(t, `match http m|^HTTP| p/Http/`)}
	filter := compileNmapPrefilter(rules)
	require.NotNil(t, filter.db)
	t.Cleanup(func() { _ = filter.db.Close() })

	matches, err := filepath.Glob(filepath.Join(os.Getenv("YAK_MINIREHS_CACHE"), "service", "*.rehs"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	raw, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	require.Equal(t, "REHS", string(raw[:4]))
	require.Equal(t, "http", firstService(rules, "HTTP/1.0 200 OK"))
}
