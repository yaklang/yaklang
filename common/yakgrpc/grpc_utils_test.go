package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func QueryHTTPFlows(ctx context.Context, client ypb.YakClient, in *ypb.QueryHTTPFlowRequest, expectLen int) (*ypb.QueryHTTPFlowResponse, error) {
	var result *ypb.QueryHTTPFlowResponse
	err := utils.AttemptWithDelayFast(func() error {
		out, err := client.QueryHTTPFlows(ctx, in)
		if err != nil {
			return err
		}
		if len(out.Data) != expectLen {
			return utils.Errorf("expect %d, got %d", expectLen, len(out.Data))
		}
		result = out
		return nil
	})
	return result, err
}
