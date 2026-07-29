package aihttp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/aihttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestCreateSessionValidation(t *testing.T) {
	gw := newTestGateway(t)

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/agent/session", bytes.NewReader([]byte(`{`)))
		req.Header.Set("Content-Type", "application/json")
		w := performRequest(gw, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestCreateSession(t *testing.T) {
	gw := newTestGateway(t)

	body, _ := json.Marshal(aihttp.CreateSessionRequest{RunID: "session-create-test"})
	req := httptest.NewRequest("POST", "/agent/session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := performRequest(gw, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var resp aihttp.CreateSessionResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, "session-create-test", resp.RunID)
}

func TestListAllSessions(t *testing.T) {
	gw := newTestGateway(t)
	req := httptest.NewRequest("GET", "/agent/session/all", nil)
	w := performRequest(gw, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestDeleteSession(t *testing.T) {
	gw := newTestGateway(t, aihttp.WithDatabase(consts.GetGormProjectDatabase()))

	body, _ := json.Marshal(aihttp.CreateSessionRequest{RunID: "session-delete-test"})
	createReq := httptest.NewRequest("POST", "/agent/session", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := performRequest(gw, createReq)
	require.Equal(t, http.StatusCreated, createResp.Code)

	deleteBody, err := protojson.Marshal(&ypb.DeleteAISessionFilter{
		SessionID: []string{"session-delete-test"},
	})
	require.NoError(t, err)

	deleteReq := httptest.NewRequest("POST", "/agent/session/del", bytes.NewReader(deleteBody))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteResp := performRequest(gw, deleteReq)
	require.Equal(t, http.StatusOK, deleteResp.Code)
	var deleteMsg ypb.DbOperateMessage
	err = protojson.Unmarshal(deleteResp.Body.Bytes(), &deleteMsg)
	require.NoError(t, err)
	require.Equal(t, "delete", deleteMsg.GetOperation())

	listReq := httptest.NewRequest("GET", "/agent/session/all", nil)
	listResp := performRequest(gw, listReq)
	require.Equal(t, http.StatusOK, listResp.Code)

	var sessions aihttp.SessionListResponse
	err = json.Unmarshal(listResp.Body.Bytes(), &sessions)
	require.NoError(t, err)
	for _, item := range sessions.Sessions {
		require.NotEqual(t, "session-delete-test", item.RunID)
	}
}

func TestDeleteSessionPassthroughDeleteAISessionFilter(t *testing.T) {
	gw := newTestGateway(t, aihttp.WithDatabase(consts.GetGormProjectDatabase()))

	oldRunID := "session-filter-old-" + uuid.NewString()
	newRunID := "session-filter-new-" + uuid.NewString()

	for _, runID := range []string{oldRunID, newRunID} {
		body, _ := json.Marshal(aihttp.CreateSessionRequest{RunID: runID})
		req := httptest.NewRequest("POST", "/agent/session", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := performRequest(gw, req)
		require.Equal(t, http.StatusCreated, resp.Code)
	}

	db := consts.GetGormProjectDatabase()
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", oldRunID).UpdateColumn("updated_at", time.Unix(1000, 0)).Error)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", newRunID).UpdateColumn("updated_at", time.Unix(2000, 0)).Error)

	filterBody, err := protojson.Marshal(&ypb.DeleteAISessionFilter{
		BeforeTimestamp: 1500,
	})
	require.NoError(t, err)

	deleteReq := httptest.NewRequest("POST", "/agent/session/del", bytes.NewReader(filterBody))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteResp := performRequest(gw, deleteReq)
	require.Equal(t, http.StatusOK, deleteResp.Code)

	listReq := httptest.NewRequest("GET", "/agent/session/all", nil)
	listResp := performRequest(gw, listReq)
	require.Equal(t, http.StatusOK, listResp.Code)

	var sessions aihttp.SessionListResponse
	err = json.Unmarshal(listResp.Body.Bytes(), &sessions)
	require.NoError(t, err)

	foundOld := false
	foundNew := false
	for _, item := range sessions.Sessions {
		if item.RunID == oldRunID {
			foundOld = true
		}
		if item.RunID == newRunID {
			foundNew = true
		}
	}
	require.False(t, foundOld)
	require.True(t, foundNew)
}

// TestDeleteSessionPlatformFilterAccepted verifies that a delete request
// carrying only a Platform filter (no SessionID / timestamp / Source) is
// accepted by the gateway validator and forwarded (not rejected as 400).
func TestDeleteSessionPlatformFilterAccepted(t *testing.T) {
	gw := newTestGateway(t, aihttp.WithDatabase(consts.GetGormProjectDatabase()))

	feishuID := "platform-filter-feishu-" + uuid.NewString()
	dingtalkID := "platform-filter-dingtalk-" + uuid.NewString()
	for _, sid := range []string{feishuID, dingtalkID} {
		body, _ := json.Marshal(aihttp.CreateSessionRequest{RunID: sid})
		req := httptest.NewRequest("POST", "/agent/session", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		require.Equal(t, http.StatusCreated, performRequest(gw, req).Code)
	}
	db := consts.GetGormProjectDatabase()
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", feishuID).
		UpdateColumns(map[string]any{"source": "im", "im_source": `{"platform":"feishu"}`}).Error)
	require.NoError(t, db.Model(&schema.AISession{}).Where("session_id = ?", dingtalkID).
		UpdateColumns(map[string]any{"source": "im", "im_source": `{"platform":"dingtalk"}`}).Error)

	filterBody, err := protojson.Marshal(&ypb.DeleteAISessionFilter{
		// Scope deletion to the two test sessions (AND) so we never touch
		// unrelated real IM sessions in the project database.
		SessionID: []string{feishuID, dingtalkID},
		Source:    []string{"im"},
		Platform:  []string{"feishu"},
	})
	require.NoError(t, err)
	deleteReq := httptest.NewRequest("POST", "/agent/session/del", bytes.NewReader(filterBody))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteResp := performRequest(gw, deleteReq)
	require.Equal(t, http.StatusOK, deleteResp.Code, "platform-only filter should not be 400: %s", deleteResp.Body.String())

	// Only dingtalk should remain among the two IM sessions.
	listReq := httptest.NewRequest("GET", "/agent/session/all", nil)
	listResp := performRequest(gw, listReq)
	require.Equal(t, http.StatusOK, listResp.Code)
	var sessions aihttp.SessionListResponse
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &sessions))
	ids := make(map[string]bool)
	for _, item := range sessions.Sessions {
		ids[item.RunID] = true
	}
	require.False(t, ids[feishuID], "feishu session should be deleted")
	require.True(t, ids[dingtalkID], "dingtalk session should remain")
}

// TestDeleteSessionRejectsEmptyFilter ensures an empty filter still 400s.
func TestDeleteSessionRejectsEmptyFilter(t *testing.T) {
	gw := newTestGateway(t)

	filterBody, err := protojson.Marshal(&ypb.DeleteAISessionFilter{})
	require.NoError(t, err)
	deleteReq := httptest.NewRequest("POST", "/agent/session/del", bytes.NewReader(filterBody))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteResp := performRequest(gw, deleteReq)
	require.Equal(t, http.StatusBadRequest, deleteResp.Code)
}
