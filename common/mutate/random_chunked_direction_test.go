package mutate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestRandomChunkedInfo_DirectionToGRPCModel 验证方向字段 Direction 能正确透传到
// gRPC 模型，这是修复"开启分块传输后请求 body 被当响应内容回显"的关键：
// 请求分块标记 REQUEST，SSE 响应增量标记 RESPONSE，未设置兜底为 RESPONSE。
func TestRandomChunkedInfo_DirectionToGRPCModel(t *testing.T) {
	t.Run("request direction is propagated", func(t *testing.T) {
		info := &RandomChunkedInfo{
			Index:     1,
			Data:      []byte("req-chunk"),
			Direction: ypb.ChunkedDataDirection_CHUNKED_DATA_DIRECTION_REQUEST,
		}
		m := info.ToGRPCModel()
		require.NotNil(t, m)
		assert.Equal(t, ypb.ChunkedDataDirection_CHUNKED_DATA_DIRECTION_REQUEST, m.Direction)
		assert.Equal(t, []byte("req-chunk"), m.Data)
	})

	t.Run("response direction is propagated", func(t *testing.T) {
		info := &RandomChunkedInfo{
			Index:     2,
			Data:      []byte("resp-delta"),
			Direction: ypb.ChunkedDataDirection_CHUNKED_DATA_DIRECTION_RESPONSE,
		}
		m := info.ToGRPCModel()
		require.NotNil(t, m)
		assert.Equal(t, ypb.ChunkedDataDirection_CHUNKED_DATA_DIRECTION_RESPONSE, m.Direction)
	})

	t.Run("unset direction defaults to RESPONSE for backward compat", func(t *testing.T) {
		// 兼容旧调用方：未显式设置方向时按响应增量处理，保持与历史 SSE 逻辑一致。
		info := &RandomChunkedInfo{
			Index: 3,
			Data:  []byte("legacy"),
		}
		m := info.ToGRPCModel()
		require.NotNil(t, m)
		assert.Equal(t, ypb.ChunkedDataDirection_CHUNKED_DATA_DIRECTION_RESPONSE, m.Direction)
	})

	t.Run("nil receiver returns nil", func(t *testing.T) {
		var info *RandomChunkedInfo
		assert.Nil(t, info.ToGRPCModel())
	})
}
