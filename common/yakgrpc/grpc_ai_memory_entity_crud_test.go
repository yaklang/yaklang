package yakgrpc

import (
	"context"
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestAIMemoryEntity_CRUD_DBOnly(t *testing.T) {
	// isolate yakit dirs

	client, srv, err := NewLocalClientAndServerWithTempDatabase(t)

	require.NoError(t, err)
	ctx := context.Background()

	db := srv.GetProjectDatabase()
	sessionID := "test-session"
	now := time.Now()

	// Create (DB)
	e1 := &schema.AIMemoryEntity{
		Model: gorm.Model{
			CreatedAt: now.Add(-2 * time.Minute),
			UpdatedAt: now.Add(-2 * time.Minute),
		},
		MemoryID:           "m1",
		SessionID:          sessionID,
		Content:            "hello yaklang",
		Tags:               schema.StringArray{"yaklang", "grpc"},
		PotentialQuestions: schema.StringArray{"what is yaklang?"},
	}
	e2 := &schema.AIMemoryEntity{
		Model: gorm.Model{
			CreatedAt: now.Add(-1 * time.Minute),
			UpdatedAt: now.Add(-1 * time.Minute),
		},
		MemoryID:  "m2",
		SessionID: sessionID,
		Content:   "another memory",
		Tags:      schema.StringArray{"grpc"},
	}
	require.NoError(t, db.Create(e1).Error)
	require.NoError(t, db.Create(e2).Error)

	// Read (gRPC Get)
	got, err := client.GetAIMemoryEntity(ctx, &ypb.GetAIMemoryEntityRequest{
		SessionID: sessionID,
		MemoryID:  "m1",
	})
	require.NoError(t, err)
	require.Equal(t, "m1", got.GetMemoryID())
	require.Equal(t, sessionID, got.GetSessionID())
	require.Equal(t, "hello yaklang", got.GetContent())
	require.ElementsMatch(t, []string{"yaklang", "grpc"}, got.GetTags())

	// Query (gRPC Query, non-vector path)
	q1, err := client.QueryAIMemoryEntity(ctx, &ypb.QueryAIMemoryEntityRequest{
		Pagination: &ypb.Paging{Page: 1, Limit: 10, OrderBy: "memory_id", Order: "asc"},
		Filter: &ypb.AIMemoryEntityFilter{
			SessionID:                sessionID,
			ContentKeyword:           "yaklang",
			TagMatchAll:              false,
			PotentialQuestionKeyword: "",
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), q1.GetTotal())
	require.Len(t, q1.GetData(), 1)
	require.Equal(t, "m1", q1.GetData()[0].GetMemoryID())

	// Update (gRPC Update)
	_, err = client.UpdateAIMemoryEntity(ctx, &ypb.AIMemoryEntity{
		MemoryID:  "m1",
		SessionID: sessionID,
		Content:   "hello yaklang updated",
		Tags:      []string{"yaklang", "crud"},
	})
	require.NoError(t, err)

	got2, err := client.GetAIMemoryEntity(ctx, &ypb.GetAIMemoryEntityRequest{
		SessionID: sessionID,
		MemoryID:  "m1",
	})
	require.NoError(t, err)
	require.Equal(t, "hello yaklang updated", got2.GetContent())
	require.ElementsMatch(t, []string{"yaklang", "crud"}, got2.GetTags())

	// Delete (gRPC Delete)
	_, err = client.DeleteAIMemoryEntity(ctx, &ypb.DeleteAIMemoryEntityRequest{
		Filter: &ypb.AIMemoryEntityFilter{
			SessionID: sessionID,
			MemoryID:  []string{"m1"},
		},
	})
	require.NoError(t, err)

	_, err = client.GetAIMemoryEntity(ctx, &ypb.GetAIMemoryEntityRequest{
		SessionID: sessionID,
		MemoryID:  "m1",
	})
	require.Error(t, err)
}

func TestAIMemoryEntity_CountTags_DBOnly(t *testing.T) {
	client, srv, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)
	ctx := context.Background()

	db := srv.GetProjectDatabase()
	sessionID := "test-session"

	require.NoError(t, db.Create(&schema.AIMemoryEntity{
		MemoryID:  "m1",
		SessionID: sessionID,
		Content:   "hello yaklang",
		Tags:      schema.StringArray{"yaklang", "grpc"},
	}).Error)
	require.NoError(t, db.Create(&schema.AIMemoryEntity{
		MemoryID:  "m2",
		SessionID: sessionID,
		Content:   "another memory",
		Tags:      schema.StringArray{"grpc"},
	}).Error)
	require.NoError(t, db.Create(&schema.AIMemoryEntity{
		MemoryID:  "m3",
		SessionID: sessionID,
		Content:   "dup tags",
		Tags:      schema.StringArray{"grpc", "tag2"},
	}).Error)

	resp, err := client.CountAIMemoryEntityTags(ctx, &ypb.CountAIMemoryEntityTagsRequest{
		SessionID: sessionID,
	})
	require.NoError(t, err)

	require.Equal(t, []*ypb.TagsCode{
		{Value: "grpc", Total: 3},
		{Value: "tag2", Total: 1},
		{Value: "yaklang", Total: 1},
	}, resp.GetTagsCount())
}
