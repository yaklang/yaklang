package yakit

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func TestHTTPFlowSQLiteTextExpressionsUseLargeFieldsOnly(t *testing.T) {
	flow := &schema.HTTPFlow{
		Request:  strings.Repeat("r", sqliteHTTPFlowTextBytesMinSize-1),
		Response: strings.Repeat("s", sqliteHTTPFlowTextBytesMinSize),
	}
	expressions := httpFlowSQLiteTextExpressions(flow)
	require.NotContains(t, expressions, "request")
	require.Contains(t, expressions, "response")
}

func TestInsertHTTPFlowSQLiteLargePacketsRemainText(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	requestToken := "phase60-request-token"
	responseToken := "phase60-response-token"
	flow := &schema.HTTPFlow{
		HiddenIndex: "phase60-large-text-bind",
		Request:     strings.Repeat("r", sqliteHTTPFlowTextBytesMinSize) + requestToken,
		Response:    strings.Repeat("s", 4*sqliteHTTPFlowTextBytesMinSize) + responseToken,
		SourceType:  schema.HTTPFlow_SourceType_MITM,
	}
	afterSaveCalls := 0
	flow.AfterSaveHandlers = append(flow.AfterSaveHandlers, func(saved *schema.HTTPFlow) {
		afterSaveCalls++
		require.Equal(t, flow, saved)
	})
	require.NoError(t, InsertHTTPFlow(db, flow))
	require.NotZero(t, flow.ID)
	require.Equal(t, 1, afterSaveCalls)

	var requestType, responseType string
	require.NoError(t, db.Raw(
		"SELECT typeof(request), typeof(response) FROM http_flows WHERE id = ?",
		flow.ID,
	).Row().Scan(&requestType, &responseType))
	require.Equal(t, "text", requestType)
	require.Equal(t, "text", responseType)

	var likeCount int
	require.NoError(t, db.Model(&schema.HTTPFlow{}).
		Where("request LIKE ? AND response LIKE ?", "%"+requestToken+"%", "%"+responseToken+"%").
		Count(&likeCount).Error)
	require.Equal(t, 1, likeCount)

	stored, err := GetHTTPFlow(db, int64(flow.ID))
	require.NoError(t, err)
	require.Equal(t, flow.Request, stored.Request)
	require.Equal(t, flow.Response, stored.Response)
	require.Equal(t, flow.Hash, stored.Hash)
}
