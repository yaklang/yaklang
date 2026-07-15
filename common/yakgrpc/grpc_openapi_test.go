//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/yakurl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func setupOpenAPIGRPCTest(t *testing.T) {
	t.Helper()
	t.Setenv("YAKIT_HOME", t.TempDir())
	yakurl.ResetOpenAPIDocumentStoreForTest()
}

func readOpenAPIDemoFixture(t *testing.T) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", "swagger-demo.json"))
	require.NoError(t, err)
	return content
}

func TestGRPCMUSTPASS_OpenAPIYakURLUploadAndList(t *testing.T) {
	setupOpenAPIGRPCTest(t)
	client, err := NewLocalClient()
	require.NoError(t, err)

	content := readOpenAPIDemoFixture(t)
	uploadResp, err := client.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
		Method: "POST",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: "upload",
			Path:     "/",
		},
		Body: content,
	})
	require.NoError(t, err)
	require.NotEmpty(t, uploadResp.GetResources())
	docID := uploadResp.GetResources()[0].GetResourceName()
	require.NotEmpty(t, docID)

	listResp, err := client.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: docID,
			Path:     "/",
		},
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(listResp.GetResources()), 5)
	require.Equal(t, "Yakit OpenAPI Demo API", listResp.GetResources()[0].GetVerboseName())
}

func TestGRPCMUSTPASS_OpenAPIYakURLBuildOperation(t *testing.T) {
	setupOpenAPIGRPCTest(t)
	client, err := NewLocalClient()
	require.NoError(t, err)

	doc := readOpenAPIDemoFixture(t)
	uploadResp, err := client.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
		Method: "POST",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: "upload",
			Path:     "/",
		},
		Body: doc,
	})
	require.NoError(t, err)
	docID := uploadResp.GetResources()[0].GetResourceName()

	resp, err := client.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
		Method: "POST",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: docID,
			Path:     "/",
			Query: []*ypb.KVPair{
				{Key: "op", Value: "build"},
				{Key: "method", Value: "GET"},
				{Key: "path", Value: "/users/{id}"},
				{Key: "param.id", Value: "1"},
				{Key: "overrideIsHttps", Value: "true"},
			},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetResources())
	raw := ""
	for _, extra := range resp.GetResources()[0].GetExtra() {
		if extra.GetKey() == "request" {
			raw = extra.GetValue()
		}
	}
	require.Contains(t, raw, "/users/1")
	require.NotContains(t, raw, "//users")

	listResp, err := client.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
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
	listRaw := ""
	for _, extra := range listResp.GetResources()[0].GetExtra() {
		if extra.GetKey() == "request" {
			listRaw = extra.GetValue()
		}
	}
	require.Contains(t, listRaw, "/users")
	require.NotContains(t, listRaw, "//users")
}

func TestGRPCMUSTPASS_OpenAPIYakURLImportAll(t *testing.T) {
	setupOpenAPIGRPCTest(t)
	client, err := NewLocalClient()
	require.NoError(t, err)

	content := readOpenAPIDemoFixture(t)
	uploadResp, err := client.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
		Method: "POST",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: "upload",
			Path:     "/",
		},
		Body: content,
	})
	require.NoError(t, err)
	docID := uploadResp.GetResources()[0].GetResourceName()

	resp, err := client.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
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
}

func TestGRPCMUSTPASS_OpenAPIYakURLHistory(t *testing.T) {
	setupOpenAPIGRPCTest(t)
	client, err := NewLocalClient()
	require.NoError(t, err)

	content := readOpenAPIDemoFixture(t)
	for i := 0; i < 2; i++ {
		_, err := client.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
			Method: "POST",
			Url: &ypb.YakURL{
				Schema:   "openapi",
				Location: "upload",
				Path:     "/",
				Query: []*ypb.KVPair{
					{Key: "fileName", Value: "swagger-demo.json"},
				},
			},
			Body: content,
		})
		require.NoError(t, err)
	}

	historyResp, err := client.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: "history",
			Path:     "/",
		},
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(historyResp.GetResources()), 2)
	for _, resource := range historyResp.GetResources() {
		require.Equal(t, "openapi-document", resource.GetResourceType())
	}
}

func TestGRPCMUSTPASS_OpenAPIYakURLUploadCancel(t *testing.T) {
	setupOpenAPIGRPCTest(t)
	client, err := NewLocalClient()
	require.NoError(t, err)

	content := readOpenAPIDemoFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
		Method: "POST",
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: "upload",
			Path:     "/",
			Query: []*ypb.KVPair{
				{Key: "parse_task_id", Value: "test-cancel-task"},
			},
		},
		Body: content,
	})
	require.Error(t, err)
}
