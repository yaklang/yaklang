package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SavePluginExecutionHistory 由前端在插件执行结束（finished）或用户主动停止（stopped）时调用。
// 前端把聚合好的 streamInfo 快照 + 执行参数 + runtimeId + resultStatus 一次性 POST 回后端，
// 后端只做存储，不参与执行期聚合（B 方案）。数据落在 project 库的 exec_histories 表。
func (s *Server) SavePluginExecutionHistory(ctx context.Context, req *ypb.SavePluginExecutionHistoryRequest) (*ypb.Empty, error) {
	if err := yakit.SavePluginExecutionHistory(s.GetProjectDatabase(), req); err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

// GetPluginExecutionUsageRanking 返回插件使用次数排行（按 plugin_id 分组 count 降序）。
// 供 MenuPlugin 侧边栏 / 插件列表按使用频次排序展示。
func (s *Server) GetPluginExecutionUsageRanking(ctx context.Context, req *ypb.Empty) (*ypb.PluginExecutionUsageRankingResponse, error) {
	data, err := yakit.QueryPluginExecutionUsageRanking(s.GetProjectDatabase(), 0)
	if err != nil {
		return nil, err
	}
	return &ypb.PluginExecutionUsageRankingResponse{Data: data}, nil
}