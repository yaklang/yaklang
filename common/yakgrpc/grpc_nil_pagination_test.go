package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestQueryRPCsAcceptNilPagination(t *testing.T) {
	client, server, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)
	require.NoError(t, server.GetProfileDatabase().AutoMigrate(&schema.AIAgentRuntime{}).Error)

	ctx := context.Background()
	tests := []struct {
		name  string
		query func() (*ypb.Paging, error)
	}{
		{
			name: "chaos maker rules",
			query: func() (*ypb.Paging, error) {
				response, err := client.QueryChaosMakerRule(ctx, &ypb.QueryChaosMakerRuleRequest{})
				return response.GetPagination(), err
			},
		},
		{
			name: "HTTP fuzzer history",
			query: func() (*ypb.Paging, error) {
				response, err := client.QueryHistoryHTTPFuzzerTaskEx(ctx, &ypb.QueryHistoryHTTPFuzzerTaskExParams{})
				return response.GetPagination(), err
			},
		},
		{
			name: "AI tasks",
			query: func() (*ypb.Paging, error) {
				response, err := client.QueryAITask(ctx, &ypb.AITaskQueryRequest{})
				return response.GetPagination(), err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pagination, err := test.query()
			require.NoError(t, err)
			require.NotNil(t, pagination)
			require.Equal(t, int64(1), pagination.GetPage())
			require.Equal(t, int64(10), pagination.GetLimit())
		})
	}
}
