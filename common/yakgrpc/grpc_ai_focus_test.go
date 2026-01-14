package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestYak_GetAIReActLoopMetadata(t *testing.T) { // 如果后期有数据库存储的自定义 ai 专注模式 ，则需要调整此测试
	client, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)

	resp, err := client.QueryAIFocus(context.Background(), &ypb.QueryAIFocusRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.GetData())

	names := make([]string, 0, len(resp.GetData()))
	for _, meta := range resp.GetData() {
		require.NotEmpty(t, meta.GetName())
		names = append(names, meta.GetName())
	}

	var yaklangMeta *ypb.AIFocus
	for _, meta := range resp.GetData() {
		if meta.GetName() == schema.AI_REACT_LOOP_NAME_WRITE_YAKLANG {
			yaklangMeta = meta
			break
		}
	}
	require.NotNil(t, yaklangMeta)
	require.NotEmpty(t, yaklangMeta.GetDescription())
	require.NotEmpty(t, yaklangMeta.GetUsagePrompt())
}
