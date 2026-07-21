package mcp_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mcp"
	rawmcp "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

type recordingHTTPFlowClient struct {
	ypb.YakClient
	lastQuery *ypb.QueryHTTPFlowRequest
	queries   int
	setTags   int
	deletes   int
}

func (c *recordingHTTPFlowClient) GetProfileDatabase() *gorm.DB {
	return nil
}

func (c *recordingHTTPFlowClient) QueryHTTPFlows(ctx context.Context, req *ypb.QueryHTTPFlowRequest, _ ...grpc.CallOption) (*ypb.QueryHTTPFlowResponse, error) {
	c.lastQuery = req
	c.queries++
	return &ypb.QueryHTTPFlowResponse{}, nil
}

func (c *recordingHTTPFlowClient) SetTagForHTTPFlow(ctx context.Context, req *ypb.SetTagForHTTPFlowRequest, _ ...grpc.CallOption) (*ypb.Empty, error) {
	c.setTags++
	return &ypb.Empty{}, nil
}

func (c *recordingHTTPFlowClient) DeleteHTTPFlows(ctx context.Context, req *ypb.DeleteHTTPFlowRequest, _ ...grpc.CallOption) (*ypb.Empty, error) {
	c.deletes++
	return &ypb.Empty{}, nil
}

func (c *recordingHTTPFlowClient) GetCurrentProjectEx(ctx context.Context, req *ypb.GetCurrentProjectExRequest, _ ...grpc.CallOption) (*ypb.ProjectDescription, error) {
	return &ypb.ProjectDescription{
		ProjectName:  "test-project",
		DatabasePath: "/tmp/test-project.db",
	}, nil
}

func TestQueryHTTPFlowDefaultsToNewestWhenPaginationOmitsOrder(t *testing.T) {
	client := &recordingHTTPFlowClient{}
	srv, err := mcp.NewMCPServer(
		mcp.WithEnableHTTPFlowToolSet(),
		mcp.WithGRPCClient(client),
	)
	require.NoError(t, err)

	_, err = mcp.CallBuiltinTool(srv, context.Background(), "query_http_flow", map[string]any{
		"pagination": map[string]any{
			"page":  1,
			"limit": 5,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, client.lastQuery)
	require.NotNil(t, client.lastQuery.Pagination)
	require.Equal(t, schema.HTTPFlow_SourceType_MITM, client.lastQuery.SourceType)
	require.Equal(t, "updated_at", client.lastQuery.Pagination.OrderBy)
	require.Equal(t, "desc", client.lastQuery.Pagination.Order)
}

func TestQueryHTTPFlowDefaultScopeMatchesCurrentMITMTraffic(t *testing.T) {
	db, err := consts.CreateProjectDatabase(t.TempDir() + "/project.db")
	require.NoError(t, err)
	_, err = insertHTTPFlowForMCPTest(db, "https://mitm.example/old", time.Now().Add(-time.Minute))
	require.NoError(t, err)
	_, err = insertHTTPFlowForMCPTest(db, "https://scan.example/ignored", time.Now().Add(time.Minute), schema.HTTPFlow_SourceType_SCAN)
	require.NoError(t, err)
	_, err = insertHTTPFlowForMCPTest(db, "https://mitm.example/new", time.Now())
	require.NoError(t, err)

	client := &recordingHTTPFlowClient{}
	srv, err := mcp.NewMCPServer(
		mcp.WithEnableHTTPFlowToolSet(),
		mcp.WithGRPCClient(client),
		mcp.WithDatabases(nil, db),
	)
	require.NoError(t, err)

	result, err := mcp.CallBuiltinTool(srv, context.Background(), "query_http_flow", map[string]any{})
	require.NoError(t, err)

	var payload struct {
		Flows []struct {
			Url string `json:"Url"`
		} `json:"flows"`
		ReturnedCount     int   `json:"returned_count"`
		TotalMatchedCount int64 `json:"total_matched_count"`
		Total             *int  `json:"total"`
		EffectiveFilter   struct {
			SourceType string `json:"source_type"`
		} `json:"effective_filter"`
	}
	decodeToolResultJSON(t, toolResultText(t, result), &payload)
	require.Nil(t, payload.Total, "ambiguous total field should not be exposed to MCP clients")
	require.Equal(t, int64(2), payload.TotalMatchedCount)
	require.Equal(t, 2, payload.ReturnedCount)
	require.Equal(t, schema.HTTPFlow_SourceType_MITM, payload.EffectiveFilter.SourceType)
	require.Len(t, payload.Flows, 2)
	require.Equal(t, "https://mitm.example/new", payload.Flows[0].Url)
	require.Equal(t, "https://mitm.example/old", payload.Flows[1].Url)
}

func TestQueryHTTPFlowEmptyResultHintDoesNotSuggestUnrelatedTools(t *testing.T) {
	client := &recordingHTTPFlowClient{}
	srv, err := mcp.NewMCPServer(
		mcp.WithEnableHTTPFlowToolSet(),
		mcp.WithGRPCClient(client),
	)
	require.NoError(t, err)

	result, err := mcp.CallBuiltinTool(srv, context.Background(), "query_http_flow", map[string]any{})
	require.NoError(t, err)

	var payload struct {
		Hint string `json:"hint"`
	}
	decodeToolResultJSON(t, toolResultText(t, result), &payload)
	require.NotEmpty(t, payload.Hint)
	require.NotContains(t, payload.Hint, "list_project_databases")
	require.NotContains(t, payload.Hint, "switch_current_project_database")
	require.True(t, strings.Contains(payload.Hint, "Do not inspect risks, ports, or MITM rules"))
}

func TestQueryHTTPFlowUsesBoundProjectDatabase(t *testing.T) {
	db, err := consts.CreateProjectDatabase(t.TempDir() + "/project.db")
	require.NoError(t, err)
	_, err = insertHTTPFlowForMCPTest(db, "https://old.example", time.Now().Add(-time.Hour))
	require.NoError(t, err)
	_, err = insertHTTPFlowForMCPTest(db, "https://new.example", time.Now())
	require.NoError(t, err)

	client := &recordingHTTPFlowClient{}
	srv, err := mcp.NewMCPServer(
		mcp.WithEnableHTTPFlowToolSet(),
		mcp.WithGRPCClient(client),
		mcp.WithDatabases(nil, db),
	)
	require.NoError(t, err)

	result, err := mcp.CallBuiltinTool(srv, context.Background(), "query_http_flow", map[string]any{
		"pagination": map[string]any{
			"page":  1,
			"limit": 1,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 0, client.queries, "bound project DB should avoid querying the fallback local client")

	var payload struct {
		Flows []struct {
			Url string `json:"Url"`
		} `json:"flows"`
	}
	decodeToolResultJSON(t, toolResultText(t, result), &payload)
	require.Len(t, payload.Flows, 1)
	require.Equal(t, "https://new.example", payload.Flows[0].Url)
}

func TestQueryHTTPFlowUsesLatestProjectDatabaseProvider(t *testing.T) {
	db1, err := consts.CreateProjectDatabase(t.TempDir() + "/project-1.db")
	require.NoError(t, err)
	_, err = insertHTTPFlowForMCPTest(db1, "https://old-project.example", time.Now())
	require.NoError(t, err)

	db2, err := consts.CreateProjectDatabase(t.TempDir() + "/project-2.db")
	require.NoError(t, err)
	_, err = insertHTTPFlowForMCPTest(db2, "https://new-project.example", time.Now())
	require.NoError(t, err)

	currentDB := db1
	client := &recordingHTTPFlowClient{}
	srv, err := mcp.NewMCPServer(
		mcp.WithEnableHTTPFlowToolSet(),
		mcp.WithGRPCClient(client),
		mcp.WithDatabaseProvider(nil, func() *gorm.DB {
			return currentDB
		}),
	)
	require.NoError(t, err)

	result, err := mcp.CallBuiltinTool(srv, context.Background(), "query_http_flow", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 0, client.queries, "provider project DB should avoid querying the fallback local client")
	requireHTTPFlowURL(t, result, "https://old-project.example")

	currentDB = db2
	result, err = mcp.CallBuiltinTool(srv, context.Background(), "query_http_flow", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 0, client.queries, "provider should be resolved on every tool call")
	requireHTTPFlowURL(t, result, "https://new-project.example")
}

func TestSetTagHTTPFlowUsesBoundProjectDatabase(t *testing.T) {
	db, err := consts.CreateProjectDatabase(t.TempDir() + "/project.db")
	require.NoError(t, err)
	flow, err := insertHTTPFlowForMCPTest(db, "https://tag.example", time.Now())
	require.NoError(t, err)

	client := &recordingHTTPFlowClient{}
	srv, err := mcp.NewMCPServer(
		mcp.WithEnableHTTPFlowToolSet(),
		mcp.WithGRPCClient(client),
		mcp.WithDatabases(nil, db),
	)
	require.NoError(t, err)

	_, err = mcp.CallBuiltinTool(srv, context.Background(), "set_tag_for_http_flow", map[string]any{
		"id":   int64(flow.ID),
		"tags": []string{"current"},
	})
	require.NoError(t, err)
	require.Equal(t, 0, client.setTags, "bound project DB should avoid setting tags through the fallback local client")

	updated, err := yakit.GetHTTPFlow(db, int64(flow.ID))
	require.NoError(t, err)
	require.Equal(t, "current", updated.Tags)
}

func TestDeleteHTTPFlowUsesBoundProjectDatabase(t *testing.T) {
	db, err := consts.CreateProjectDatabase(t.TempDir() + "/project.db")
	require.NoError(t, err)
	flow, err := insertHTTPFlowForMCPTest(db, "https://delete.example", time.Now())
	require.NoError(t, err)

	client := &recordingHTTPFlowClient{}
	srv, err := mcp.NewMCPServer(
		mcp.WithEnableHTTPFlowToolSet(),
		mcp.WithGRPCClient(client),
		mcp.WithDatabases(nil, db),
	)
	require.NoError(t, err)

	_, err = mcp.CallBuiltinTool(srv, context.Background(), "delete_http_flow", map[string]any{
		"id": []int64{int64(flow.ID)},
	})
	require.NoError(t, err)
	require.Equal(t, 0, client.deletes, "bound project DB should avoid deleting through the fallback local client")

	_, err = yakit.GetHTTPFlow(db, int64(flow.ID))
	require.Error(t, err)
}

func insertHTTPFlowForMCPTest(db *gorm.DB, url string, ts time.Time, sourceType ...string) (*schema.HTTPFlow, error) {
	st := schema.HTTPFlow_SourceType_MITM
	if len(sourceType) > 0 && sourceType[0] != "" {
		st = sourceType[0]
	}
	flow := &schema.HTTPFlow{
		Model: gorm.Model{
			CreatedAt: ts,
			UpdatedAt: ts,
		},
		Hash:       url,
		Url:        url,
		Path:       "/",
		Method:     "GET",
		StatusCode: 200,
		SourceType: st,
		Request:    "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
		Response:   "HTTP/1.1 200 OK\r\n\r\n",
	}
	return flow, yakit.InsertHTTPFlow(db, flow)
}

func requireHTTPFlowURL(t *testing.T, result *rawmcp.CallToolResult, expected string) {
	t.Helper()
	var payload struct {
		Flows []struct {
			Url string `json:"Url"`
		} `json:"flows"`
	}
	decodeToolResultJSON(t, toolResultText(t, result), &payload)
	require.Len(t, payload.Flows, 1)
	require.Equal(t, expected, payload.Flows[0].Url)
}
