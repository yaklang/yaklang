package yakit

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestApplyAISessionPlatformFilter verifies the platform filter narrows IM
// sessions by the platform stored inside the im_source JSON column, and is
// a no-op when no platforms are provided.
func TestApplyAISessionPlatformFilter(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	feishu := "pf-feishu-" + uuid.NewString()
	dingtalk := "pf-dingtalk-" + uuid.NewString()
	for _, sid := range []string{feishu, dingtalk} {
		_, err = EnsureAISessionMeta(db, sid, "im")
		require.NoError(t, err)
	}
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", feishu).
		UpdateColumn("im_source", `{"platform":"feishu","chatType":"private"}`).Error)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", dingtalk).
		UpdateColumn("im_source", `{"platform":"dingtalk","chatType":"group"}`).Error)

	query := func(platforms []string) []string {
		var ids []string
		q := applyAISessionPlatformFilter(db.Model(&schema.AISession{}).Where("source = ?", "im"), platforms)
		require.NoError(t, q.Pluck("session_id", &ids).Error)
		return ids
	}

	// No platform filter: returns both (no-op).
	ids := query(nil)
	require.ElementsMatch(t, []string{feishu, dingtalk}, ids)

	// feishu only.
	ids = query([]string{"feishu"})
	require.Equal(t, []string{feishu}, ids)

	// dingtalk only.
	ids = query([]string{"dingtalk"})
	require.Equal(t, []string{dingtalk}, ids)

	// case-insensitive + dedup.
	ids = query([]string{"Feishu", "feishu"})
	require.Equal(t, []string{feishu}, ids)

	// multiple platforms OR.
	ids = query([]string{"feishu", "dingtalk"})
	require.ElementsMatch(t, []string{feishu, dingtalk}, ids)
}

// TestFilterAISessionMeta_Platform exercises the public FilterAISessionMeta
// entry point combining source + platform.
func TestFilterAISessionMeta_Platform(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	feishu := "fm-feishu-" + uuid.NewString()
	dingtalk := "fm-dingtalk-" + uuid.NewString()
	for _, sid := range []string{feishu, dingtalk} {
		_, err = EnsureAISessionMeta(db, sid, "im")
		require.NoError(t, err)
	}
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", feishu).
		UpdateColumn("im_source", `{"platform":"feishu"}`).Error)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", dingtalk).
		UpdateColumn("im_source", `{"platform":"dingtalk"}`).Error)

	var ids []string
	q := FilterAISessionMeta(db, &ypb.AISessionFilter{Source: []string{"im"}, Platform: []string{"feishu"}})
	require.NoError(t, q.Pluck("session_id", &ids).Error)
	require.Equal(t, []string{feishu}, ids)
}

// TestQueryAISessionIDsForDelete_Platform verifies deletion lookup narrows by
// platform so only the matching IM sessions are targeted.
func TestQueryAISessionIDsForDelete_Platform(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)

	feishu := "pd-feishu-" + uuid.NewString()
	dingtalk := "pd-dingtalk-" + uuid.NewString()
	for _, sid := range []string{feishu, dingtalk} {
		_, err = EnsureAISessionMeta(db, sid, "im")
		require.NoError(t, err)
	}
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", feishu).
		UpdateColumn("im_source", `{"platform":"feishu"}`).Error)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", dingtalk).
		UpdateColumn("im_source", `{"platform":"dingtalk"}`).Error)

	ids, err := QueryAISessionIDsForDelete(db, &ypb.DeleteAISessionFilter{
		Source:   []string{"im"},
		Platform: []string{"feishu"},
	}, false)
	require.NoError(t, err)
	require.Equal(t, []string{feishu}, ids)

	// Platform-only filter (no source) is accepted as a valid condition.
	ids, err = QueryAISessionIDsForDelete(db, &ypb.DeleteAISessionFilter{
		Platform: []string{"dingtalk"},
	}, false)
	require.NoError(t, err)
	require.Equal(t, []string{dingtalk}, ids)
}