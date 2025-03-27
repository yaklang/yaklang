package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"strings"
	"testing"
)

func TestMUSTPASS_SyntaxFlowRuleUpdate(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	yakit.DelKey(consts.GetGormProfileDatabase(), consts.EmbedSfBuildInRuleKey)
	update, err := client.CheckSyntaxFlowRuleUpdate(context.Background(), &ypb.CheckSyntaxFlowRuleUpdateRequest{})
	require.NoError(t, err)
	require.True(t, update.GetNeedUpdate())
	stream, err := client.ApplySyntaxFlowRuleUpdate(context.Background(), &ypb.ApplySyntaxFlowRuleUpdateRequest{})
	require.NoError(t, err)
	var finalProcess float64
	finish := false
	for {
		rsp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		spew.Dump(rsp)
		finalProcess = rsp.GetPercent()
		if strings.Contains(rsp.GetMessage(), "更新SyntaxFlow内置规则成功！") {
			finish = true
		}
	}
	require.Equal(t, float64(1), finalProcess)
	require.True(t, finish)

	update, err = client.CheckSyntaxFlowRuleUpdate(context.Background(), &ypb.CheckSyntaxFlowRuleUpdateRequest{})
	require.NoError(t, err)
	require.False(t, update.GetNeedUpdate())
}
