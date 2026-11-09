package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestQueryKnowledgeBaseByFilter(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.KnowledgeBaseInfo{}).Error)

	alpha := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "query-alpha-kb",
		KnowledgeBaseDescription: "alpha security notes",
		KnowledgeBaseType:        "guide",
		Tags:                     "red,security",
		IsDefault:                true,
		CreatedFromUI:            true,
	}
	beta := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "query-beta-kb",
		KnowledgeBaseDescription: "beta engineering notes",
		KnowledgeBaseType:        "reference",
		Tags:                     "blue,engineering",
	}
	require.NoError(t, db.Create(alpha).Error)
	require.NoError(t, db.Create(beta).Error)

	paginator, items, err := QueryKnowledgeBaseByFilter(db, &ypb.KnowledgeBaseFilter{
		IDs:     []int64{int64(alpha.ID), int64(beta.ID)},
		Keyword: "alpha",
		Types:   []string{"guide"},
		Tags:    []string{"security"},
	}, &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "asc"})
	require.NoError(t, err)
	require.Equal(t, 1, paginator.TotalRecord)
	require.Len(t, items, 1)
	require.Equal(t, alpha.ID, items[0].ID)

	paginator, items, err = QueryKnowledgeBaseByFilter(db, &ypb.KnowledgeBaseFilter{
		Names: []string{beta.KnowledgeBaseName},
	}, &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "asc"})
	require.NoError(t, err)
	require.Equal(t, 1, paginator.TotalRecord)
	require.Len(t, items, 1)
	require.Equal(t, beta.ID, items[0].ID)

	paginator, items, err = QueryKnowledgeBaseByFilter(db, nil, &ypb.Paging{
		Page: 1, Limit: 1, OrderBy: "id", Order: "asc",
	})
	require.NoError(t, err)
	require.Equal(t, 2, paginator.TotalRecord)
	require.Len(t, items, 1)
	require.Equal(t, alpha.ID, items[0].ID)
}
