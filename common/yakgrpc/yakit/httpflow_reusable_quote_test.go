package yakit

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
)

func reusableQuoteTestPackets() ([]byte, []byte) {
	requestBody := bytes.Repeat([]byte("request-token-"), 5*1024)
	request := append(
		[]byte(fmt.Sprintf(
			"POST /reusable-quote HTTP/1.1\r\nHost: example.test\r\nContent-Length: %d\r\n\r\n",
			len(requestBody),
		)),
		requestBody...,
	)
	responseBody := bytes.Repeat([]byte("response-token-"), 18*1024)
	response := append(
		[]byte(fmt.Sprintf(
			"HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n",
			len(responseBody),
		)),
		responseBody...,
	)
	return request, response
}

func TestReusablePacketQuotePersistsExactTextAndProto(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	request, response := reusableQuoteTestPackets()
	wantRequest := strconv.Quote(string(request))
	wantResponse := strconv.Quote(string(response))
	flow, err := CreateHTTPFlow(
		CreateHTTPFlowWithRequestRaw(request),
		CreateHTTPFlowWithResponseRaw(response),
		CreateHTTPFlowWithFixResponseRaw(response),
		CreateHTTPFlowWithSource(schema.HTTPFlow_SourceType_MITM),
		CreateHTTPFlowWithURL("http://example.test/reusable-quote"),
		CreateHTTPFlowWithRemoteAddr("127.0.0.1:80"),
		CreateHTTPFlowWithReusablePacketQuoteBuffer(),
	)
	require.NoError(t, err)
	require.Equal(t, wantRequest, flow.Request)
	require.Equal(t, wantResponse, flow.Response)
	wantHash := flow.Hash

	afterSaveCalled := false
	flow.AfterSaveHandlers = append(flow.AfterSaveHandlers, func(saved *schema.HTTPFlow) {
		afterSaveCalled = true
		require.Equal(t, wantRequest, saved.Request)
		require.Equal(t, wantResponse, saved.Response)
		require.Equal(t, request, []byte(saved.GetRequest()))
		require.Equal(t, response, []byte(saved.GetResponse()))
	})

	require.NoError(t, InsertHTTPFlow(db, flow))
	require.True(t, afterSaveCalled)
	require.NotZero(t, flow.ID)
	require.Empty(t, flow.Request)
	require.Empty(t, flow.Response)
	require.Empty(t, flow.AfterPersistCleanups)

	var storedRequest, storedResponse, requestType, responseType string
	require.NoError(t, db.Raw(
		"SELECT request, response, typeof(request), typeof(response) FROM http_flows WHERE id = ?",
		flow.ID,
	).Row().Scan(&storedRequest, &storedResponse, &requestType, &responseType))
	require.Equal(t, wantRequest, storedRequest)
	require.Equal(t, wantResponse, storedResponse)
	require.Equal(t, "text", requestType)
	require.Equal(t, "text", responseType)

	var likeCount int
	require.NoError(t, db.Model(&schema.HTTPFlow{}).
		Where("request LIKE ? AND response LIKE ?", "%request-token-%", "%response-token-%").
		Count(&likeCount).Error)
	require.Equal(t, 1, likeCount)

	stored, err := GetHTTPFlow(db, int64(flow.ID))
	require.NoError(t, err)
	require.Equal(t, wantRequest, stored.Request)
	require.Equal(t, wantResponse, stored.Response)
	require.Equal(t, wantHash, stored.Hash)

	grpcFlow, err := model.ToHTTPFlowGRPCModelFull(stored)
	require.NoError(t, err)
	require.Equal(t, request, grpcFlow.GetRequest())
	require.Equal(t, response, grpcFlow.GetResponse())
}

func TestReusablePacketQuoteCleanupOnPersistenceFailureAndDiscard(t *testing.T) {
	request, response := reusableQuoteTestPackets()
	createFlow := func(t *testing.T) *schema.HTTPFlow {
		t.Helper()
		flow, err := CreateHTTPFlow(
			CreateHTTPFlowWithRequestRaw(request),
			CreateHTTPFlowWithResponseRaw(response),
			CreateHTTPFlowWithFixResponseRaw(response),
			CreateHTTPFlowWithSource(schema.HTTPFlow_SourceType_MITM),
			CreateHTTPFlowWithURL("http://example.test/reusable-quote-cleanup"),
			CreateHTTPFlowWithReusablePacketQuoteBuffer(),
		)
		require.NoError(t, err)
		require.NotEmpty(t, flow.Request)
		require.NotEmpty(t, flow.Response)
		return flow
	}

	t.Run("persistence-failure", func(t *testing.T) {
		db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "closed.db"))
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)
		require.NoError(t, db.Close())

		flow := createFlow(t)
		require.Error(t, InsertHTTPFlow(db, flow))
		require.Empty(t, flow.Request)
		require.Empty(t, flow.Response)
		require.Empty(t, flow.AfterPersistCleanups)
	})

	t.Run("discard", func(t *testing.T) {
		flow := createFlow(t)
		ReleaseHTTPFlowPersistResources(flow)
		ReleaseHTTPFlowPersistResources(flow)
		require.Empty(t, flow.Request)
		require.Empty(t, flow.Response)
		require.Empty(t, flow.AfterPersistCleanups)
	})
}

func TestDefaultPacketQuoteRemainsOwnedAfterPersistence(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	request := []byte("GET /default-owned HTTP/1.1\r\nHost: example.test\r\n\r\n")
	response := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
	flow, err := CreateHTTPFlow(
		CreateHTTPFlowWithRequestRaw(request),
		CreateHTTPFlowWithResponseRaw(response),
		CreateHTTPFlowWithFixResponseRaw(response),
		CreateHTTPFlowWithSource(schema.HTTPFlow_SourceType_MITM),
		CreateHTTPFlowWithURL("http://example.test/default-owned"),
	)
	require.NoError(t, err)
	wantRequest, wantResponse := flow.Request, flow.Response
	require.NoError(t, InsertHTTPFlow(db, flow))
	require.Equal(t, wantRequest, flow.Request)
	require.Equal(t, wantResponse, flow.Response)
}
