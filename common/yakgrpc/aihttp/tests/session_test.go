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
