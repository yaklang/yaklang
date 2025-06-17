package yakgrpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
	"time"
)

func queryForge(ctx context.Context, client ypb.YakClient, filter *ypb.AIForgeFilter) ([]*ypb.AIForge, error) {
	resp, err := client.QueryAIForge(ctx, &ypb.QueryAIForgeRequest{
		Pagination: &ypb.Paging{},
		Filter:     filter,
	})
	return resp.GetData(), err
}

func TestAIForgeBaseCurd(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	name := uuid.New().String()
	content := uuid.New().String()

	_, err = client.CreateAIForge(ctx, &ypb.AIForge{
		ForgeName:    name,
		ForgeContent: content,
	})
	require.NoError(t, err)

	forge, err := queryForge(ctx, client, &ypb.AIForgeFilter{
		ForgeName: name,
	})
	require.NoError(t, err)
	require.Len(t, forge, 1)
	require.Equal(t, name, forge[0].ForgeName)
	require.Equal(t, content, forge[0].ForgeContent)

	forge, err = queryForge(ctx, client, &ypb.AIForgeFilter{
		Keyword: content,
	})
	require.NoError(t, err)
	require.Len(t, forge, 1)
	require.Equal(t, name, forge[0].ForgeName)
	require.Equal(t, content, forge[0].ForgeContent)

	newContent := uuid.New().String()
	_, err = client.UpdateAIForge(ctx, &ypb.AIForge{
		ForgeName:    name,
		ForgeContent: newContent,
	})
	require.NoError(t, err)

	forge, err = queryForge(ctx, client, &ypb.AIForgeFilter{
		ForgeName: name,
	})
	require.NoError(t, err)
	require.Len(t, forge, 1)
	require.Equal(t, name, forge[0].ForgeName)
	require.Equal(t, newContent, forge[0].ForgeContent)

	_, err = client.DeleteAIForge(ctx, &ypb.AIForgeFilter{
		ForgeName: name,
	})
	require.NoError(t, err)

	forge, err = queryForge(ctx, client, &ypb.AIForgeFilter{
		ForgeName: name,
	})
	require.NoError(t, err)
	require.Len(t, forge, 0)
}
