package imagecache

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/minirehs"
)

func TestOpenReusesImageAndRepairsCorruptFile(t *testing.T) {
	t.Setenv("YAK_MINIREHS_CACHE", t.TempDir())
	patterns := []minirehs.Pattern{
		{ID: 1, Expr: `nginx/[\d.]+`},
		{ID: 2, Expr: `Apache`, Flags: minirehs.FlagCaseless},
	}
	opts := []minirehs.Option{
		minirehs.WithBackend(minirehs.BackendMVS),
		minirehs.WithReportLocation(false),
	}
	first, err := Open("service", patterns, opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })

	path, err := patternFile("service", patterns)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(4))
	magic, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "REHS", string(magic[:4]))

	second, err := Open("service", patterns, opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
	again, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, info.ModTime().Equal(again.ModTime()))
	require.Equal(t, info.Size(), again.Size())
	require.Equal(t, []uint32{1}, hitIDs(t, second, []byte("server: nginx/1.24.0")))

	require.NoError(t, os.WriteFile(path, []byte("not-an-image"), 0o644))
	repaired, err := Open("service", patterns, opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repaired.Close() })
	fixed, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "REHS", string(fixed[:4]))
	require.Equal(t, []uint32{1}, hitIDs(t, repaired, []byte("server: nginx/1.24.0")))
}

func hitIDs(t *testing.T, db minirehs.Database, data []byte) []uint32 {
	t.Helper()
	scratch, err := db.NewScratch()
	require.NoError(t, err)
	t.Cleanup(func() { _ = scratch.Close() })
	var ids []uint32
	require.NoError(t, db.Scan(data, scratch, func(m minirehs.Match) bool {
		ids = append(ids, uint32(m.ID))
		return true
	}))
	return ids
}
