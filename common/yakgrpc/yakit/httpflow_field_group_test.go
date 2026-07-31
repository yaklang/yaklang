package yakit

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func TestHTTPFlowFieldGroupsUseProvidedDatabase(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	flows := []*schema.HTTPFlow{
		{Hash: "field-group-1", Tags: "alpha|beta|YAKIT_COLOR_RED", PathSuffix: ".js"},
		{Hash: "field-group-2", Tags: "alpha", PathSuffix: ".js"},
		{Hash: "field-group-3", Tags: "gamma", PathSuffix: ".css"},
		{Hash: "field-group-4"},
	}
	for _, flow := range flows {
		require.NoError(t, db.Create(flow).Error)
	}

	tags, err := QueryHTTPFlowTagsWithDB(db)
	require.NoError(t, err)
	tagsByValue := make(map[string]*TagAndStatusCode, len(tags))
	for _, tag := range tags {
		tagsByValue[tag.Value] = tag
	}
	require.Equal(t, 2, tagsByValue["alpha"].Count)
	require.Equal(t, 1, tagsByValue["beta"].Count)
	require.Equal(t, 1, tagsByValue["gamma"].Count)
	require.False(t, tagsByValue["alpha"].Builtin)
	require.NotContains(t, tagsByValue, "YAKIT_COLOR_RED")
	for builtin := range HTTPFlowBuiltinTags {
		tag, ok := tagsByValue[builtin]
		require.True(t, ok, "missing builtin tag %s", builtin)
		require.True(t, tag.Builtin)
		require.Zero(t, tag.Count)
	}

	suffixes, err := HTTPFlowSuffixesWithDB(db)
	require.NoError(t, err)
	suffixCounts := make(map[string]int, len(suffixes))
	for _, suffix := range suffixes {
		suffixCounts[suffix.Value] = suffix.Count
	}
	require.Equal(t, map[string]int{".css": 1, ".js": 2}, suffixCounts)
}
