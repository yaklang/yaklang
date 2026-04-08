package aihttp_test

import (
	"bufio"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var useProtoNames = protojson.MarshalOptions{UseProtoNames: true}

func postProtoRequest(t *testing.T, path string, msg proto.Message, gw interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}) *httptest.ResponseRecorder {
	t.Helper()

	body, err := useProtoNames.Marshal(msg)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	gw.ServeHTTP(w, req)
	return w
}

func queryAIForgeByName(t *testing.T, gw interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, forgeName string) *ypb.QueryAIForgeResponse {
	t.Helper()

	resp := postProtoRequest(t, "/agent/forge/query", &ypb.QueryAIForgeRequest{
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 20,
		},
		Filter: &ypb.AIForgeFilter{
			ForgeName: forgeName,
		},
	}, gw)
	require.Equal(t, http.StatusOK, resp.Code)

	var queryResp ypb.QueryAIForgeResponse
	require.NoError(t, protojson.Unmarshal(resp.Body.Bytes(), &queryResp))
	return &queryResp
}

func deleteAIForgeByName(t *testing.T, gw interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, forgeName string) {
	t.Helper()

	resp := postProtoRequest(t, "/agent/forge/delete", &ypb.AIForgeFilter{
		ForgeName: forgeName,
	}, gw)
	require.Equal(t, http.StatusOK, resp.Code)
}

func decodeGeneralProgressSSE(t *testing.T, raw string) []*ypb.GeneralProgress {
	t.Helper()

	progresses := make([]*ypb.GeneralProgress, 0)
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		var progress ypb.GeneralProgress
		require.NoError(t, protojson.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &progress))
		progresses = append(progresses, &progress)
	}
	require.NoError(t, scanner.Err())
	return progresses
}

func TestAIForgeCRUDPassthrough(t *testing.T) {
	gw := newTestGateway(t)

	forgeName := "forge-http-" + uuid.NewString()
	initialContent := "content-" + uuid.NewString()
	updatedContent := "content-" + uuid.NewString()

	t.Cleanup(func() {
		deleteAIForgeByName(t, gw, forgeName)
	})

	createResp := postProtoRequest(t, "/agent/forge/create", &ypb.AIForge{
		ForgeName:    forgeName,
		ForgeContent: initialContent,
	}, gw)
	require.Equal(t, http.StatusOK, createResp.Code)

	var createMsg ypb.DbOperateMessage
	require.NoError(t, protojson.Unmarshal(createResp.Body.Bytes(), &createMsg))
	require.Equal(t, "create", createMsg.GetOperation())
	require.Equal(t, int64(1), createMsg.GetEffectRows())
	require.NotZero(t, createMsg.GetCreateID())

	queryResp := queryAIForgeByName(t, gw, forgeName)
	require.Len(t, queryResp.GetData(), 1)
	require.Equal(t, initialContent, queryResp.GetData()[0].GetForgeContent())

	getResp := postProtoRequest(t, "/agent/forge/get", &ypb.GetAIForgeRequest{
		ForgeName: forgeName,
	}, gw)
	require.Equal(t, http.StatusOK, getResp.Code)

	var forge ypb.AIForge
	require.NoError(t, protojson.Unmarshal(getResp.Body.Bytes(), &forge))
	require.Equal(t, forgeName, forge.GetForgeName())
	require.Equal(t, initialContent, forge.GetForgeContent())

	updateResp := postProtoRequest(t, "/agent/forge/update", &ypb.AIForge{
		Id:           forge.GetId(),
		ForgeName:    forgeName,
		ForgeContent: updatedContent,
	}, gw)
	require.Equal(t, http.StatusOK, updateResp.Code)

	var updateMsg ypb.DbOperateMessage
	require.NoError(t, protojson.Unmarshal(updateResp.Body.Bytes(), &updateMsg))
	require.Equal(t, "update", updateMsg.GetOperation())
	require.Equal(t, int64(1), updateMsg.GetEffectRows())

	getResp = postProtoRequest(t, "/agent/forge/get", &ypb.GetAIForgeRequest{
		ForgeName: forgeName,
	}, gw)
	require.Equal(t, http.StatusOK, getResp.Code)
	require.NoError(t, protojson.Unmarshal(getResp.Body.Bytes(), &forge))
	require.Equal(t, updatedContent, forge.GetForgeContent())

	deleteResp := postProtoRequest(t, "/agent/forge/delete", &ypb.AIForgeFilter{
		ForgeName: forgeName,
	}, gw)
	require.Equal(t, http.StatusOK, deleteResp.Code)

	var deleteMsg ypb.DbOperateMessage
	require.NoError(t, protojson.Unmarshal(deleteResp.Body.Bytes(), &deleteMsg))
	require.Equal(t, "delete", deleteMsg.GetOperation())

	queryResp = queryAIForgeByName(t, gw, forgeName)
	require.Len(t, queryResp.GetData(), 0)
}

func TestAIForgeExportImportPassthrough(t *testing.T) {
	gw := newTestGateway(t)

	forgeName := "forge-http-export-" + uuid.NewString()
	outputName := "forge-http-export-package-" + uuid.NewString()
	exportPath := filepath.Join(consts.GetDefaultYakitProjectsDir(), outputName+".zip")

	t.Cleanup(func() {
		deleteAIForgeByName(t, gw, forgeName)
		_ = os.Remove(exportPath)
	})

	createResp := postProtoRequest(t, "/agent/forge/create", &ypb.AIForge{
		ForgeName:    forgeName,
		ForgeContent: "content-" + uuid.NewString(),
	}, gw)
	require.Equal(t, http.StatusOK, createResp.Code)

	_ = os.Remove(exportPath)

	exportResp := postProtoRequest(t, "/agent/forge/export", &ypb.ExportAIForgeRequest{
		ForgeNames: []string{forgeName},
		OutputName: outputName,
	}, gw)
	require.Equal(t, http.StatusOK, exportResp.Code)
	require.Contains(t, exportResp.Header().Get("Content-Type"), "text/event-stream")

	exportProgress := decodeGeneralProgressSSE(t, exportResp.Body.String())
	require.NotEmpty(t, exportProgress)
	require.Equal(t, float64(100), exportProgress[len(exportProgress)-1].GetPercent())
	require.Equal(t, "success", exportProgress[len(exportProgress)-1].GetMessageType())
	require.Equal(t, "export completed", exportProgress[len(exportProgress)-1].GetMessage())
	require.FileExists(t, exportPath)

	deleteAIForgeByName(t, gw, forgeName)
	require.Len(t, queryAIForgeByName(t, gw, forgeName).GetData(), 0)

	importResp := postProtoRequest(t, "/agent/forge/import", &ypb.ImportAIForgeRequest{
		InputPath: exportPath,
	}, gw)
	require.Equal(t, http.StatusOK, importResp.Code)
	require.Contains(t, importResp.Header().Get("Content-Type"), "text/event-stream")

	importProgress := decodeGeneralProgressSSE(t, importResp.Body.String())
	require.NotEmpty(t, importProgress)
	require.Equal(t, float64(100), importProgress[len(importProgress)-1].GetPercent())
	require.Equal(t, "import completed", importProgress[len(importProgress)-1].GetMessage())

	queryResp := queryAIForgeByName(t, gw, forgeName)
	require.Len(t, queryResp.GetData(), 1)
	require.Equal(t, forgeName, queryResp.GetData()[0].GetForgeName())
}
