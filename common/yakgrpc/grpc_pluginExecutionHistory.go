package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SavePluginExecutionHistory 由前端在插件执行结束（finished）或用户主动停止（stopped）时调用。
// 前端把聚合好的 streamInfo 快照 + 执行参数 + runtimeId + resultStatus 一次性 POST 回后端。
//
// 双写：
//  1. project 库 exec_histories：存完整历史（含 streamInfo），按项目隔离，供"恢复现场"用
//  2. profile 库 plugin_usage_counts：全局计数 +1，供 QueryYakScript 按使用次数排序 / 排行用
func (s *Server) SavePluginExecutionHistory(ctx context.Context, req *ypb.SavePluginExecutionHistoryRequest) (*ypb.Empty, error) {
	// 1. 完整历史落 project 库
	if err := yakit.SavePluginExecutionHistory(s.GetProjectDatabase(), req); err != nil {
		return nil, err
	}
	// 2. 全局计数落 profile 库（plugin_id > 0 才计）
	if err := yakit.UpsertPluginUsageCount(
		s.GetProfileDatabase(),
		req.GetPluginId(),
		req.GetPluginName(),
		req.GetPluginUUID(),
		req.GetPluginType(),
		req.GetHeadImg(),
	); err != nil {
		// 计数失败不应阻断历史保存，记录日志即可
		// log.Errorf("upsert plugin usage count failed: %s", err)
	}
	return &ypb.Empty{}, nil
}

// GetPluginExecutionUsageRanking 返回插件使用次数排行（按 count 降序），查 profile 库 plugin_usage_counts 表。
// 供 MenuPlugin 侧边栏 / 插件列表按使用频次排序展示。全局计数，切项目不丢。
func (s *Server) GetPluginExecutionUsageRanking(ctx context.Context, req *ypb.Empty) (*ypb.PluginExecutionUsageRankingResponse, error) {
	data, err := yakit.QueryPluginUsageCountRanking(s.GetProfileDatabase(), 0)
	if err != nil {
		return nil, err
	}
	return &ypb.PluginExecutionUsageRankingResponse{Data: data}, nil
}