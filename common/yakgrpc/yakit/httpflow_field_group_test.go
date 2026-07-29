package yakit

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/yaklang/gorm"
	"github.com/stretchr/testify/require"
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
	tagValues := make([]string, 0, len(tags))
	for _, tag := range tags {
		tagValues = append(tagValues, tag.Value)
	}
	sort.Strings(tagValues)
	require.Equal(t, []string{"alpha", "beta", "gamma"}, tagValues)

	suffixes, err := HTTPFlowSuffixesWithDB(db)
	require.NoError(t, err)
	suffixCounts := make(map[string]int, len(suffixes))
	for _, suffix := range suffixes {
		suffixCounts[suffix.Value] = suffix.Count
	}
	require.Equal(t, map[string]int{".css": 1, ".js": 2}, suffixCounts)
}
