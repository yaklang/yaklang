package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"strings"
	"testing"
)

func TestMUSTPASS_UpdateSFBuildInRule(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	stream, err := client.UpdateSFBuildInRule(context.Background(), &ypb.UpdateSFBuildInRuleRequest{})
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
}
