package yakgrpc

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestAnalyzeHTTPFlow(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	result, err := client.AnalyzeHTTPFlow(
		context.Background(),
		&ypb.AnalyzeHTTPFlowRequest{},
	)
	require.NoError(t, err)
	fmt.Println("analyzedId: " + result.AnalyzeId)

	var ids []int64
	{
		// query rule data
		ruleData, err := client.QueryAnalyzedHTTPFlowRule(context.Background(), &ypb.QueryAnalyzedHTTPFlowRuleRequest{
			AnalyzeIds: []string{result.AnalyzeId},
			Pagination: nil,
		})
		require.NoError(t, err)
		fmt.Println("ruleData: ", ruleData)
		for _, data := range ruleData.Data {
			ids = append(ids, data.HTTPFlowIds...)
		}
	}

	{
		// query http flow
		httpFlows, err := client.GetHTTPFlowByIds(context.Background(), &ypb.GetHTTPFlowByIdsRequest{
			Ids: []int64{ids[0]},
		})
		require.NoError(t, err)
		fmt.Println("httpFlows: ", httpFlows.GetData()[0])
	}
}
