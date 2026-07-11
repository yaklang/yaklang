package mcp_test

import (
	"context"
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mcp"
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
	require.Equal(t, "updated_at", client.lastQuery.Pagination.OrderBy)
	require.Equal(t, "desc", client.lastQuery.Pagination.Order)
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

func insertHTTPFlowForMCPTest(db *gorm.DB, url string, ts time.Time) (*schema.HTTPFlow, error) {
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
		Request:    "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
		Response:   "HTTP/1.1 200 OK\r\n\r\n",
	}
	return flow, yakit.InsertHTTPFlow(db, flow)
}
