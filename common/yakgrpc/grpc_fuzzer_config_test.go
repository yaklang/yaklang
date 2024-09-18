package yakgrpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPCMUSTPASS_FuzzerConfig(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	var pageIds []string
	saveFuzzerConfig := func(num int) {
		var Data []*ypb.FuzzerConfig
		for i := 0; i < num; i++ {
			pageId := uuid.New().String()
			pageIds = append(pageIds, pageId)
			Data = append(Data, &ypb.FuzzerConfig{
				PageId: pageId,
				Type:   "group",
				Config: "{\"isHTTPS\":true",
			})
			req := &ypb.SaveFuzzerConfigRequest{
				Data: Data,
			}
			_, err = client.SaveFuzzerConfig(context.Background(), req)
			require.NoError(t, err)
		}
	}
	queryAll := &ypb.QueryFuzzerConfigRequest{Pagination: &ypb.Paging{Limit: -1}}
	originResult, err := client.QueryFuzzerConfig(context.Background(), queryAll)
	require.NoError(t, err)
	saveFuzzerConfig(100)

	newResult, err := client.QueryFuzzerConfig(context.Background(), queryAll)
	require.NoError(t, err)
	require.Equal(t, len(originResult.GetData())+100, len(newResult.GetData()))

	res, err := client.QueryFuzzerConfig(context.Background(), &ypb.QueryFuzzerConfigRequest{Pagination: &ypb.Paging{Limit: 10}})
	require.NoError(t, err)
	require.Equal(t, len(res.Data), 10)

	res, err = client.QueryFuzzerConfig(context.Background(), &ypb.QueryFuzzerConfigRequest{PageId: pageIds[:10]})
	require.NoError(t, err)
	require.Equal(t, len(res.Data), 10)

	res, err = client.QueryFuzzerConfig(context.Background(), &ypb.QueryFuzzerConfigRequest{PageId: pageIds[:10], Pagination: &ypb.Paging{Limit: 15}})
	require.NoError(t, err)
	require.Equal(t, len(res.Data), 10)

	_, err = client.DeleteFuzzerConfig(context.Background(), &ypb.DeleteFuzzerConfigRequest{PageId: pageIds})
	require.NoError(t, err)
	result, err := client.QueryFuzzerConfig(context.Background(), queryAll)
	require.Equal(t, len(originResult.GetData()), len(result.GetData()))
}
