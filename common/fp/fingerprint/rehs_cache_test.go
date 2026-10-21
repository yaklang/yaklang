package fingerprint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "yak-web-rehs-")
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

func TestWebPrefilterWritesImage(t *testing.T) {
	t.Setenv("YAK_MINIREHS_CACHE", t.TempDir())
	rules := []*rule.FingerPrintRule{
		webRegexpRule("nginx", `nginx/([\d.]+)`, ""),
	}
	filter := compileWebPrefilter(rules)
	require.NotNil(t, filter.db)
	t.Cleanup(func() { _ = filter.db.Close() })

	matches, err := filepath.Glob(filepath.Join(os.Getenv("YAK_MINIREHS_CACHE"), "web", "*.rehs"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	raw, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	require.Equal(t, "REHS", string(raw[:4]))
	kept := filter.allows([]byte("nginx/1.24.0"), rules)
	require.Len(t, kept, 1)
	require.Equal(t, "nginx", kept[0].MatchParam.Info.Product)
}
