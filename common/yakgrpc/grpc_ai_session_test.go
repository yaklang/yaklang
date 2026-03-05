package yakgrpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_QueryAISession_Pagination(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	srv := &Server{projectDatabase: db}

	marker := "query-session-page-" + uuid.NewString()
	sessionIDs := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		sessionID := fmt.Sprintf("%s-%d", marker, i)
		sessionIDs = append(sessionIDs, sessionID)
		_, err = yakit.CreateOrUpdateAISessionMeta(db, sessionID, fmt.Sprintf("%s-title-%d", marker, i))
		require.NoError(t, err)
		require.NoError(t, db.Model(&schema.AISession{}).
			Where("session_id = ?", sessionID).
			UpdateColumn("updated_at", time.Unix(int64(1000+i), 0)).Error)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	page1, err := srv.QueryAISession(ctx, &ypb.QueryAISessionRequest{
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   2,
			OrderBy: "updated_at",
			Order:   "desc",
		},
		Filter: &ypb.AISessionFilter{
			Keyword: marker,
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(5), page1.GetTotal())
	require.Len(t, page1.GetData(), 2)
	require.Equal(t, sessionIDs[4], page1.GetData()[0].GetSessionID())
	require.Equal(t, sessionIDs[3], page1.GetData()[1].GetSessionID())

	page2, err := srv.QueryAISession(ctx, &ypb.QueryAISessionRequest{
		Pagination: &ypb.Paging{
			Page:    2,
			Limit:   2,
			OrderBy: "updated_at",
			Order:   "desc",
		},
		Filter: &ypb.AISessionFilter{
			Keyword: marker,
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(5), page2.GetTotal())
	require.Len(t, page2.GetData(), 2)
	require.Equal(t, sessionIDs[2], page2.GetData()[0].GetSessionID())
	require.Equal(t, sessionIDs[1], page2.GetData()[1].GetSessionID())

	page3, err := srv.QueryAISession(ctx, &ypb.QueryAISessionRequest{
		Pagination: &ypb.Paging{
			Page:    3,
			Limit:   2,
			OrderBy: "updated_at",
			Order:   "desc",
		},
		Filter: &ypb.AISessionFilter{
			Keyword: marker,
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(5), page3.GetTotal())
	require.Len(t, page3.GetData(), 1)
	require.Equal(t, sessionIDs[0], page3.GetData()[0].GetSessionID())
}

func TestServer_QueryAISession_DefaultPagination(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	srv := &Server{projectDatabase: db}

	marker := "query-session-default-" + uuid.NewString()
	for i := 0; i < 3; i++ {
		_, err = yakit.CreateOrUpdateAISessionMeta(db, fmt.Sprintf("%s-%d", marker, i), marker)
		require.NoError(t, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := srv.QueryAISession(ctx, &ypb.QueryAISessionRequest{
		Filter: &ypb.AISessionFilter{
			Keyword: marker,
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), resp.GetTotal())
	require.Equal(t, int64(1), resp.GetPagination().GetPage())
	require.Equal(t, int64(30), resp.GetPagination().GetLimit())
	require.Equal(t, "updated_at", resp.GetPagination().GetOrderBy())
	require.Equal(t, "desc", resp.GetPagination().GetOrder())
}
