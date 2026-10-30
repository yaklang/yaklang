package yakit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestDeprecatedAIToolsHiddenFromStoredInventory(t *testing.T) {
	db := newAIYakToolTestDB(t)
	// SQLite :memory: is private to each connection; the yielders run in
	// goroutines and must share the connection that owns the fixture tables.
	db.DB().SetMaxOpenConns(1)
	// Model pre-upgrade rows, including an edited builtin (IsBuiltin=false).
	for i, name := range schema.DeprecatedAIToolNames() {
		old := &schema.AIYakTool{Name: name, Path: "pcap/" + name, Keywords: "pcap", IsBuiltin: i != 1, IsFavorite: true}
		require.NoError(t, db.Create(old).Error)
		_, err := GetAIYakTool(db, name)
		require.ErrorContains(t, err, "query_pcapdb")
		_, err = GetAIYakToolByID(db, old.ID)
		require.ErrorContains(t, err, "load_pcapfile")
		_, err = ToggleAIYakToolFavoriteByID(db, old.ID)
		require.ErrorContains(t, err, "pcapdb")
	}
	for _, name := range []string{"load_pcapfile", "query_pcapdb", "manage_pcapdb"} {
		_, err := CreateAIYakTool(db, &schema.AIYakTool{Name: name, Path: "pcap/" + name, Keywords: "pcap", IsFavorite: true})
		require.NoError(t, err)
	}
	check := func(tools []*schema.AIYakTool, err error) {
		t.Helper()
		require.NoError(t, err)
		require.Len(t, tools, 3)
		for _, tool := range tools {
			require.NoError(t, schema.AIToolDeprecationError(tool.Name))
		}
	}
	check(SearchAIYakTool(db, ""))
	check(SearchAIYakTool(db, "pcap"))
	check(SearchAIYakToolByPath(db, "pcap"))
	count, err := CountAIYakTools(db, nil)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)
	count, err = CountAIYakTools(db, &ypb.AIToolFilter{ToolNames: schema.DeprecatedAIToolNames()})
	require.NoError(t, err)
	require.Zero(t, count)
	page, tools, err := SearchAIYakToolWithPagination(db, "pcap", true, &ypb.Paging{Page: 1, Limit: 10})
	check(tools, err)
	require.Equal(t, 3, page.TotalRecord)
	for _, rows := range []chan *schema.AIYakTool{YieldAllAITools(context.Background(), db), YieldAIYakTools(context.Background(), db, nil)} {
		var tools []*schema.AIYakTool
		for tool := range rows {
			tools = append(tools, tool)
		}
		check(tools, nil)
	}
	check(SearchAIYakToolBM25(db, &AIYakToolFilter{Keywords: []string{"pcap"}}, 10, 0))
	// Exclusion must happen before ranking/pagination, including the FTS path.
	require.NoError(t, EnsureAIYakToolFTS5(db))
	check(SearchAIYakToolBM25(db, &AIYakToolFilter{Keywords: []string{"pcap"}}, 10, 0))
}

func TestDeprecatedAIToolsCannotBeRecreated(t *testing.T) {
	db := newAIYakToolTestDB(t)
	current := &schema.AIYakTool{Name: "query_pcapdb"}
	_, err := CreateAIYakTool(db, current)
	require.NoError(t, err)
	for _, name := range schema.DeprecatedAIToolNames() {
		_, err := CreateAIYakTool(db, &schema.AIYakTool{Name: name})
		require.ErrorContains(t, err, "pcapdb")
		_, err = SaveAIYakTool(db, &schema.AIYakTool{Name: name})
		require.ErrorContains(t, err, "pcapdb")
		renamed := *current
		renamed.Name = name
		_, err = UpdateAIYakToolByID(db, &renamed)
		require.ErrorContains(t, err, "pcapdb")
	}
	_, err = GetAIYakTool(db, "query_pcapdb")
	require.NoError(t, err)
}
