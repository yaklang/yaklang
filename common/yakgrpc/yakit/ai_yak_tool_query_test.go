package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestQueryAIYakToolsFiltersAndPagination(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AIYakTool{}).Error)

	custom := &schema.AIYakTool{
		Name:        "query-custom-tool",
		Description: "custom resource",
		Keywords:    "alpha,shared",
		IsFavorite:  true,
	}
	builtin := &schema.AIYakTool{
		Name:        "query-builtin-tool",
		Description: "builtin resource",
		Keywords:    "beta,shared",
		IsBuiltin:   true,
	}
	require.NoError(t, db.Create(custom).Error)
	require.NoError(t, db.Create(builtin).Error)

	paginator, tools, err := QueryAIYakTools(db, &ypb.AIToolFilter{
		IDs:           []int64{int64(custom.ID), int64(builtin.ID)},
		OnlyFavorites: true,
	}, &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "asc"})
	require.NoError(t, err)
	require.Equal(t, 1, paginator.TotalRecord)
	require.Len(t, tools, 1)
	require.Equal(t, custom.ID, tools[0].ID)

	paginator, tools, err = QueryAIYakTools(db, &ypb.AIToolFilter{
		ToolName:  builtin.Name,
		ToolNames: []string{"missing-tool"},
		Keyword:   "shared",
	}, &ypb.Paging{Page: 1, Limit: 1, OrderBy: "id", Order: "asc"})
	require.NoError(t, err)
	require.Equal(t, 1, paginator.TotalRecord)
	require.Len(t, tools, 1)
	require.Equal(t, builtin.ID, tools[0].ID)

	paginator, tools, err = QueryAIYakTools(db, nil, &ypb.Paging{
		Page: 1, Limit: 1, OrderBy: "id", Order: "asc",
	})
	require.NoError(t, err)
	require.Equal(t, 2, paginator.TotalRecord)
	require.Len(t, tools, 1)
	require.Equal(t, custom.ID, tools[0].ID)
}
