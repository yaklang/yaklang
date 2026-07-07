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
	require.NoError(t, db.AutoMigrate(&schema.AISession{}))

	srv := &Server{projectDatabase: db}

	marker := "query-session-page-" + uuid.NewString()
	sessionIDs := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		sessionID := fmt.Sprintf("%s-%d", marker, i)
		sessionIDs = append(sessionIDs, sessionID)
		_, err = yakit.CreateOrUpdateAISessionMeta(db, sessionID, fmt.Sprintf("%s-title-%d", marker, i))
		require.NoError(t, err)
		_, err = yakit.CreateOrUpdateAISessionMetaStartParams(db, sessionID, &ypb.AIStartParams{
			AIService:         "svc",
			AIModelName:       fmt.Sprintf("model-%d", i),
			TimelineSessionID: sessionID,
		})
		require.NoError(t, err)
		require.NoError(t, db.Model(&schema.AISession{}).
			Where("session_id = ?", sessionID).
			UpdateColumn("last_used_at", time.Unix(int64(2000+i), 0)).Error)
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
	require.Equal(t, "model-5", page1.GetData()[0].GetStartParams().GetAIModelName())
	require.Equal(t, int64(2005), page1.GetData()[0].GetLastUsedAt())

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
	require.NoError(t, db.AutoMigrate(&schema.AISession{}))

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

func TestServer_QueryAISession_FilterBySource(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}))

	srv := &Server{projectDatabase: db}

	marker := "src-filter-" + uuid.NewString()
	s1 := marker + "-1"
	s2 := marker + "-2"
	s3 := marker + "-3"

	_, err = yakit.CreateOrUpdateAISessionMeta(db, s1, marker+"-t1")
	require.NoError(t, err)
	_, err = yakit.CreateOrUpdateAISessionMeta(db, s2, marker+"-t2")
	require.NoError(t, err)
	_, err = yakit.CreateOrUpdateAISessionMeta(db, s3, marker+"-t3")
	require.NoError(t, err)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", s1).UpdateColumn("source", "alpha").Error)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", s2).UpdateColumn("source", "beta").Error)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", s3).UpdateColumn("source", "alpha").Error)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := srv.QueryAISession(ctx, &ypb.QueryAISessionRequest{
		Filter: &ypb.AISessionFilter{
			Keyword: marker,
			Source:  []string{"alpha"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), resp.GetTotal())
	ids := []string{resp.GetData()[0].GetSessionID(), resp.GetData()[1].GetSessionID()}
	require.Contains(t, ids, s1)
	require.Contains(t, ids, s3)
	for _, row := range resp.GetData() {
		require.Equal(t, "alpha", row.GetSource())
	}
}

// UpdateAISessionIMMeta 写入后，QueryAISession 应能返回 IMSourceMeta，
// 且 im_source 列存储的是可往返的 protojson。
func TestServer_UpdateAISessionIMMeta(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	srv := &Server{projectDatabase: db}

	sessionID := "im-session-" + uuid.NewString()
	// 先建一个 source=im 的空行（模拟 agent 启动写入）
	_, err = yakit.EnsureAISessionMeta(db, sessionID, "im")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// IM 引擎回写 IM 元数据
	meta := &ypb.IMSourceMeta{
		Platform:   "feishu",
		ChatType:   "private",
		ChatTitle:  "Feishu DM - 张三",
		SenderName: "张三",
		ThreadID:   "",
	}
	resp, err := srv.UpdateAISessionIMMeta(ctx, &ypb.UpdateAISessionIMMetaRequest{
		SessionID: sessionID,
		Meta:      meta,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), resp.GetEffectRows())

	// QueryAISession 应返回 IMSourceMeta
	qResp, err := srv.QueryAISession(ctx, &ypb.QueryAISessionRequest{
		Filter: &ypb.AISessionFilter{SessionID: []string{sessionID}},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), qResp.GetTotal())
	got := qResp.GetData()[0].GetIMSourceMeta()
	require.NotNil(t, got, "IMSourceMeta should be returned")
	require.Equal(t, "feishu", got.GetPlatform())
	require.Equal(t, "private", got.GetChatType())
	require.Equal(t, "Feishu DM - 张三", got.GetChatTitle())
	require.Equal(t, "张三", got.GetSenderName())

	// im_source 列持久化校验
	var row schema.AISession
	require.NoError(t, db.Where("session_id = ?", sessionID).First(&row).Error)
	require.NotEmpty(t, row.IMSource, "im_source column should be populated")
	parsed, err := yakit.UnmarshalAISessionIMSource(row.IMSource)
	require.NoError(t, err)
	require.Equal(t, "feishu", parsed.GetPlatform())

	// meta==nil 时清空列
	_, err = srv.UpdateAISessionIMMeta(ctx, &ypb.UpdateAISessionIMMetaRequest{
		SessionID: sessionID,
		Meta:      nil,
	})
	require.NoError(t, err)
	require.NoError(t, db.Where("session_id = ?", sessionID).First(&row).Error)
	require.Empty(t, row.IMSource, "im_source column should be cleared when meta is nil")
}

func TestServer_UpdateAISessionIMMeta_DoesNotInitializePrivateTitle(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	srv := &Server{projectDatabase: db}

	sessionID := "im-session-title-" + uuid.NewString()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = srv.UpdateAISessionIMMeta(ctx, &ypb.UpdateAISessionIMMetaRequest{
		SessionID: sessionID,
		Meta: &ypb.IMSourceMeta{
			Platform:   "feishu",
			ChatType:   "private",
			ChatTitle:  "私聊会话",
			SenderName: "xx",
		},
	})
	require.NoError(t, err)

	qResp, err := srv.QueryAISession(ctx, &ypb.QueryAISessionRequest{
		Filter: &ypb.AISessionFilter{SessionID: []string{sessionID}},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), qResp.GetTotal())

	got := qResp.GetData()[0]
	require.Equal(t, "im", got.GetSource())
	require.Equal(t, "<未命名>", got.GetTitle())
	require.False(t, got.GetTitleInitialized())
	require.Equal(t, "feishu", got.GetIMSourceMeta().GetPlatform())
}

func TestServer_UpdateAISessionIMMeta_InitializesGroupTitle(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	srv := &Server{projectDatabase: db}

	sessionID := "im-session-group-title-" + uuid.NewString()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = srv.UpdateAISessionIMMeta(ctx, &ypb.UpdateAISessionIMMetaRequest{
		SessionID: sessionID,
		Meta: &ypb.IMSourceMeta{
			Platform:  "dingtalk",
			ChatType:  "group",
			ChatTitle: "安全测试群",
		},
	})
	require.NoError(t, err)

	qResp, err := srv.QueryAISession(ctx, &ypb.QueryAISessionRequest{
		Filter: &ypb.AISessionFilter{SessionID: []string{sessionID}},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), qResp.GetTotal())

	got := qResp.GetData()[0]
	require.Equal(t, "im", got.GetSource())
	require.Equal(t, "安全测试群", got.GetTitle())
	require.True(t, got.GetTitleInitialized())
	require.Equal(t, "dingtalk", got.GetIMSourceMeta().GetPlatform())
}
