package service

import (
	"context"
)

// SSAService 统一的 SSA 服务接口
type SSAService interface {
	// Compile 编译项目
	Compile(ctx context.Context, req *SSACompileRequest) (*SSACompileResponse, error)

	// QueryPrograms 查询程序
	QueryPrograms(ctx context.Context, req *SSAQueryRequest) (*SSAQueryResponse, error)

	// SyntaxFlowQuery 执行SyntaxFlow查询
	SyntaxFlowQuery(ctx context.Context, req *SSASyntaxFlowQueryRequest) (*SSASyntaxFlowQueryResponse, error)
}

