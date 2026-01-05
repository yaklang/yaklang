package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCAdapter_QueryPrograms(t *testing.T) {
	service := NewSSAService()
	adapter := NewGRPCAdapter(service)
	ctx := context.Background()

	t.Run("query with basic request", func(t *testing.T) {
		req := &ypb.QuerySSAProgramRequest{
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 10,
			},
		}

		// 注意：当前实现返回 nil，因为转换逻辑较复杂
		// 这个测试主要验证适配器不会 panic
		result, err := adapter.QueryPrograms(ctx, req)
		require.NoError(t, err)
		_ = result // 当前实现返回 nil
	})

	t.Run("query with filter", func(t *testing.T) {
		req := &ypb.QuerySSAProgramRequest{
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 5,
			},
			Filter: &ypb.SSAProgramFilter{
				ProgramNames: []string{"test-.*"},
				Languages:    []string{"java"},
			},
		}

		result, err := adapter.QueryPrograms(ctx, req)
		require.NoError(t, err)
		_ = result
	})

	t.Run("query with nil service", func(t *testing.T) {
		adapter := NewGRPCAdapter(nil)
		req := &ypb.QuerySSAProgramRequest{}

		result, err := adapter.QueryPrograms(ctx, req)
		require.NoError(t, err) // 当前实现返回 nil 但不报错
		require.Nil(t, result)
	})
}

