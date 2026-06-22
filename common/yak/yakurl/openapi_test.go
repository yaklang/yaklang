package yakurl_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/yakurl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func readOpenAPIDemoFixture(t *testing.T) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "yakgrpc", "testdata", "swagger-demo.json"))
	require.NoError(t, err)
	return content
}

func postOpenAPIUpload(t *testing.T, content []byte) string {
	t.Helper()
	action := yakurl.GetActionService().GetAction("openapi")
	require.NotNil(t, action)

	resp, err := action.Post(&ypb.RequestYakURLParams{
		Method: "POST",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: "upload",
			Path:     "/",
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
	content := readOpenAPIDemoFixture(t)
	docID := postOpenAPIUpload(t, content)

	rsp, err := yakurl.LoadGetResource("openapi://" + docID + "/")
	require.NoError(t, err)
	require.True(t, strings.Contains(rsp.GetResources()[0].GetVerboseName(), "OpenAPI"))
}
