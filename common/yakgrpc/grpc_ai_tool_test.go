package yakgrpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_GetAIToolList(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	t.Run("GetAllTools", func(t *testing.T) {
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			Query: "",
			Pagination: &ypb.Paging{
				Page:    1,
				Limit:   10,
				OrderBy: "updated_at",
				Order:   "desc",
			},
		})
		require.NoError(t, err)
		assert.True(t, len(resp.Tools) == 10, "Should return at least 3 tools")
		assert.Equal(t, int64(1), resp.Pagination.Page)
		assert.Equal(t, int64(10), resp.Pagination.Limit)
	})

	t.Run("GetToolByExactName", func(t *testing.T) {
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: "zip_viewer",
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)
		assert.Len(t, resp.Tools, 1, "Should return exactly 1 tool")
		assert.Equal(t, "zip_viewer", resp.Tools[0].Name)
	})

	t.Run("GetToolByKeyword", func(t *testing.T) {
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			Query: "zip",
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		require.NoError(t, err)

		// Find the test tool with special keyword
		var found bool
		for _, tool := range resp.Tools {
			if tool.Name == "zip_viewer" {
				found = true
				break
			}
		}
		assert.True(t, found, "Should find the tool with 'zip' keyword")
	})

	t.Run("NonExistentTool", func(t *testing.T) {
		resp, err := c.GetAIToolList(ctx, &ypb.GetAIToolListRequest{
			ToolName: "nonexistent-tool-" + uuid.NewString(),
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		})
		// Should return empty results but not error
		require.NoError(t, err)
		assert.Len(t, resp.Tools, 0, "Should return no tools")
	})
}
