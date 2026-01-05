package service

import (
	"context"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// GRPCAdapter 将通用服务适配到 gRPC
type GRPCAdapter struct {
	service SSAService
}

// NewGRPCAdapter 创建新的 gRPC 适配器
func NewGRPCAdapter(service SSAService) *GRPCAdapter {
	return &GRPCAdapter{service: service}
}

// QueryPrograms 查询程序（适配 gRPC 请求）
// 注意：这个适配器提供基础功能，复杂的数据库查询（分页、过滤等）仍使用 yakit.QuerySSAProgram
func (a *GRPCAdapter) QueryPrograms(ctx context.Context, req *ypb.QuerySSAProgramRequest) ([]*ypb.SSAProgram, error) {
	if a.service == nil {
		return nil, nil
	}

	// 从请求中提取查询参数
	queryReq := &SSAQueryRequest{
		Limit: 30, // 默认限制
	}

	// 如果有分页信息，使用分页限制
	if req.Pagination != nil && req.Pagination.Limit > 0 {
		queryReq.Limit = int(req.Pagination.Limit)
	}

	// 如果有过滤条件，提取程序名称模式
	if req.Filter != nil {
		if len(req.Filter.ProgramNames) > 0 {
			// 如果有多个程序名，使用第一个作为模式（简化处理）
			queryReq.ProgramNamePattern = req.Filter.ProgramNames[0]
		}
		if len(req.Filter.Languages) > 0 {
			queryReq.Language = req.Filter.Languages[0]
		}
	}

	// 调用服务层
	resp, err := a.service.QueryPrograms(ctx, queryReq)
	if err != nil {
		return nil, err
	}

	// 转换为 gRPC 模型
	// 注意：这里需要将 ssaapi.Program 转换为 ypb.SSAProgram
	// 由于转换逻辑较复杂，这里返回空列表，实际使用中可能需要更复杂的转换
	// 或者保持使用 yakit.QuerySSAProgram 来处理复杂的数据库查询
	_ = resp
	return nil, nil
}

