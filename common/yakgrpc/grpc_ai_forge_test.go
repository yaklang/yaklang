package yakgrpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

	newContent = uuid.New().String()
	_, err = client.UpdateAIForge(ctx, &ypb.AIForge{
		Id:           forge[0].GetId(),
		ForgeContent: newContent,
	})
	require.NoError(t, err)

	forge, err = queryForge(ctx, client, &ypb.AIForgeFilter{
		Id: forge[0].GetId(),
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

func TestGetAIForgeByName(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := uuid.New().String()
	content := uuid.New().String()

	_, err = client.CreateAIForge(ctx, &ypb.AIForge{
		ForgeName:    name,
		ForgeContent: content,
	})
	require.NoError(t, err)

	// Test GetAIForge by name
	forge, err := client.GetAIForge(ctx, &ypb.GetAIForgeRequest{
		ForgeName: name,
	})
	require.NoError(t, err)
	require.NotNil(t, forge)
	require.Equal(t, name, forge.ForgeName)
	require.Equal(t, content, forge.ForgeContent)

	// Clean up
	_, err = client.DeleteAIForge(ctx, &ypb.AIForgeFilter{
		ForgeName: name,
	})
	require.NoError(t, err)
}

func TestUpdateAIForgeWithZeroField(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := uuid.New().String()
	content := uuid.New().String()

	forgeIns := &ypb.AIForge{
		ForgeName:    name,
		ForgeContent: content,
	}
	_, err = client.CreateAIForge(ctx, forgeIns)
	require.NoError(t, err)
	defer func() {
		_, err = client.DeleteAIForge(ctx, &ypb.AIForgeFilter{
			ForgeName: name,
		})
		require.NoError(t, err)
	}()
	_, err = client.UpdateAIForge(ctx, &ypb.AIForge{
		ForgeName:    name,
		ForgeContent: "",
	})
	require.NoError(t, err)

	forge, err := client.GetAIForge(ctx, &ypb.GetAIForgeRequest{
		ForgeName: name,
	})
	require.NoError(t, err)
	require.NotNil(t, forge)
	require.Equal(t, name, forge.ForgeName)
	require.Equal(t, "", forge.ForgeContent)
}
