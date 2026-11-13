package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServerQueryAIResourceRPCs(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(
		&schema.AIYakTool{},
		&schema.KnowledgeBaseInfo{},
		&schema.VectorStoreCollection{},
	).Error)

	tool := &schema.AIYakTool{
		Name:        "grpc-query-tool",
		Description: "queryable tool",
		Keywords:    "grpc,resource",
		IsFavorite:  true,
	}
	require.NoError(t, db.Create(tool).Error)

	kb := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "grpc-query-kb",
		KnowledgeBaseDescription: "queryable knowledge base",
		KnowledgeBaseType:        "guide",
		Tags:                     "grpc,resource",
		IsDefault:                true,
		CreatedFromUI:            true,
	}
	require.NoError(t, db.Create(kb).Error)

	server := &Server{profileDatabase: db}

	toolResp, err := server.QueryAITool(context.Background(), &ypb.QueryAIToolRequest{
		Filter: &ypb.AIToolFilter{
			IDs:           []int64{int64(tool.ID)},
			Keyword:       "resource",
			OnlyFavorites: true,
		},
		Pagination: &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "asc"},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, toolResp.GetTotal())
	require.Len(t, toolResp.GetData(), 1)
	require.Equal(t, tool.Name, toolResp.GetData()[0].GetName())
	require.EqualValues(t, 1, toolResp.GetPagination().GetPage())

	kbResp, err := server.QueryKnowledgeBase(context.Background(), &ypb.QueryKnowledgeBaseRequest{
		Filter: &ypb.KnowledgeBaseFilter{
			IDs:     []int64{int64(kb.ID)},
			Keyword: "queryable",
			Types:   []string{"guide"},
			Tags:    []string{"resource"},
		},
		Pagination: &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "asc"},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, kbResp.GetTotal())
	require.Len(t, kbResp.GetData(), 1)
	require.Equal(t, kb.KnowledgeBaseName, kbResp.GetData()[0].GetKnowledgeBaseName())
	require.EqualValues(t, 1, kbResp.GetPagination().GetPage())
}
