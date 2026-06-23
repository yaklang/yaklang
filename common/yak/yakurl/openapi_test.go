package yakurl_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/yakurl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func setupOpenAPITest(t *testing.T) {
	t.Helper()
	t.Setenv("YAKIT_HOME", t.TempDir())
	yakurl.ResetOpenAPIDocumentStoreForTest()
}

func readOpenAPIDemoFixture(t *testing.T) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "yakgrpc", "testdata", "swagger-demo.json"))
	require.NoError(t, err)
	return content
}

func postOpenAPIUpload(t *testing.T, content []byte) string {
	return postOpenAPIUploadWithFileName(t, content, "")
}

func postOpenAPIUploadWithFileName(t *testing.T, content []byte, fileName string) string {
	t.Helper()
	action := yakurl.GetActionService().GetAction("openapi")
	require.NotNil(t, action)

	query := []*ypb.KVPair{}
	if fileName != "" {
		query = append(query, &ypb.KVPair{Key: "fileName", Value: fileName})
	}

	resp, err := action.Post(&ypb.RequestYakURLParams{
		Method: "POST",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: "upload",
			Path:     "/",
			Query:    query,
		},
		Body: content,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetResources())
	docID := resp.GetResources()[0].GetResourceName()
	require.NotEmpty(t, docID)
	return docID
}

func TestOpenAPIYakURLUploadAndList(t *testing.T) {
	setupOpenAPITest(t)
	docID := postOpenAPIUpload(t, readOpenAPIDemoFixture(t))

	action := yakurl.GetActionService().GetAction("openapi")
	resp, err := action.Get(&ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: docID,
			Path:     "/",
		},
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(resp.GetResources()), 5)
	require.Equal(t, "openapi-document", resp.GetResources()[0].GetResourceType())
	require.Equal(t, "Yakit OpenAPI Demo API", resp.GetResources()[0].GetVerboseName())
}

func TestOpenAPIYakURLBuildOperation(t *testing.T) {
	setupOpenAPITest(t)
	content := readOpenAPIDemoFixture(t)
	docID := postOpenAPIUpload(t, content)
	action := yakurl.GetActionService().GetAction("openapi")

	buildBody, err := json.Marshal(map[string]any{
		"parameterValues": map[string]string{"id": "1"},
		"overrideIsHttps": true,
	})
	require.NoError(t, err)

	resp, err := action.Post(&ypb.RequestYakURLParams{
		Method: "POST",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: docID,
			Path:     "/",
			Query: []*ypb.KVPair{
				{Key: "op", Value: "build"},
				{Key: "method", Value: "GET"},
				{Key: "path", Value: "/users/{id}"},
			},
		},
		Body: buildBody,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetResources())
	raw := yakurl.GetQueryParam(resp.GetResources()[0].GetExtra(), "request")
	require.Contains(t, raw, "/users/1")
	require.NotContains(t, raw, "//users")
	require.Equal(t, "true", yakurl.GetQueryParam(resp.GetResources()[0].GetExtra(), "is_https"))

	listResp, err := action.Post(&ypb.RequestYakURLParams{
		Method: "POST",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: docID,
			Path:     "/",
			Query: []*ypb.KVPair{
				{Key: "op", Value: "build"},
				{Key: "method", Value: "GET"},
				{Key: "path", Value: "/users"},
				{Key: "overrideIsHttps", Value: "true"},
			},
		},
	})
	require.NoError(t, err)
	listRaw := yakurl.GetQueryParam(listResp.GetResources()[0].GetExtra(), "request")
	require.Contains(t, listRaw, "/users")
	require.NotContains(t, listRaw, "//users")
}

func TestOpenAPIYakURLImportAll(t *testing.T) {
	setupOpenAPITest(t)
	docID := postOpenAPIUpload(t, readOpenAPIDemoFixture(t))
	action := yakurl.GetActionService().GetAction("openapi")

	resp, err := action.Post(&ypb.RequestYakURLParams{
		Method: "POST",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: docID,
			Path:     "/",
			Query: []*ypb.KVPair{
				{Key: "op", Value: "import-all"},
				{Key: "overrideIsHttps", Value: "true"},
			},
		},
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(resp.GetResources()), 4)
	for _, resource := range resp.GetResources() {
		require.Equal(t, "fuzzer-request", resource.GetResourceType())
		require.NotEmpty(t, yakurl.GetQueryParam(resource.GetExtra(), "request"))
	}
}

func TestOpenAPIYakURLFromRaw(t *testing.T) {
	setupOpenAPITest(t)
	content := readOpenAPIDemoFixture(t)
	docID := postOpenAPIUpload(t, content)

	rsp, err := yakurl.LoadGetResource("openapi://" + docID + "/")
	require.NoError(t, err)
	require.True(t, strings.Contains(rsp.GetResources()[0].GetVerboseName(), "OpenAPI"))
}

func TestOpenAPIYakURLHistory(t *testing.T) {
	setupOpenAPITest(t)
	action := yakurl.GetActionService().GetAction("openapi")
	require.NotNil(t, action)

	content := readOpenAPIDemoFixture(t)
	docID1 := postOpenAPIUpload(t, content)
	docID2 := postOpenAPIUpload(t, content)

	historyResp, err := action.Get(&ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: "history",
			Path:     "/",
		},
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(historyResp.GetResources()), 2)

	docIDs := map[string]bool{}
	for _, resource := range historyResp.GetResources() {
		require.Equal(t, "openapi-document", resource.GetResourceType())
		docIDs[resource.GetResourceName()] = true
		require.NotEmpty(t, yakurl.GetQueryParam(resource.GetExtra(), "last_used_at"))
		require.NotEmpty(t, yakurl.GetQueryParam(resource.GetExtra(), "session_id"))
	}
	require.True(t, docIDs[docID1])
	require.True(t, docIDs[docID2])

	_, err = action.Delete(&ypb.RequestYakURLParams{
		Method: "DELETE",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: docID1,
			Path:     "/",
		},
	})
	require.NoError(t, err)

	historyResp, err = action.Get(&ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: "history",
			Path:     "/",
		},
	})
	require.NoError(t, err)
	found := false
	for _, resource := range historyResp.GetResources() {
		if resource.GetResourceName() == docID1 {
			found = true
		}
	}
	require.False(t, found)
	require.True(t, docIDs[docID2])
}

func TestOpenAPIYakURLPersistence(t *testing.T) {
	setupOpenAPITest(t)
	content := readOpenAPIDemoFixture(t)
	docID := postOpenAPIUploadWithFileName(t, content, "swagger-demo.json")

	docDir := filepath.Join(consts.GetDefaultYakitOpenAPIDocumentsDir(), docID)
	require.DirExists(t, docDir)
	require.FileExists(t, filepath.Join(docDir, "session.json"))
	require.FileExists(t, filepath.Join(docDir, "swagger-demo.json"))

	yakurl.ResetOpenAPIDocumentStoreForTest()

	action := yakurl.GetActionService().GetAction("openapi")
	resp, err := action.Get(&ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: docID,
			Path:     "/",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "Yakit OpenAPI Demo API", resp.GetResources()[0].GetVerboseName())

	historyResp, err := action.Get(&ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: "history",
			Path:     "/",
		},
	})
	require.NoError(t, err)
	require.Len(t, historyResp.GetResources(), 1)
	require.Equal(t, docID, historyResp.GetResources()[0].GetResourceName())
	require.Equal(t, "swagger-demo.json", yakurl.GetQueryParam(historyResp.GetResources()[0].GetExtra(), "file_name"))
}
