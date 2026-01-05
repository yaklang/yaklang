package service

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

// CLIAdapter 将通用服务适配到 CLI
type CLIAdapter struct {
	service SSAService
}

// NewCLIAdapter 创建新的 CLI 适配器
func NewCLIAdapter(service SSAService) *CLIAdapter {
	return &CLIAdapter{service: service}
}

// CompileAndShow 编译项目并显示结果
func (a *CLIAdapter) CompileAndShow(ctx context.Context, req *SSACompileRequest) error {
	if a.service == nil {
		return fmt.Errorf("service is nil")
	}

	resp, err := a.service.Compile(ctx, req)
	if err != nil {
		return err
	}

	if resp.Error != nil {
		return resp.Error
	}

	log.Infof("finished compiling..., results: %v", len(resp.Programs))
	return nil
}

// QueryProgramsAndShow 查询程序并显示结果
func (a *CLIAdapter) QueryProgramsAndShow(ctx context.Context, pattern string) error {
	if a.service == nil {
		return fmt.Errorf("service is nil")
	}

	req := &SSAQueryRequest{
		ProgramNamePattern: pattern,
	}

	resp, err := a.service.QueryPrograms(ctx, req)
	if err != nil {
		return err
	}

	if resp.Error != nil {
		return resp.Error
	}

	// 显示结果
	if len(resp.Programs) == 0 {
		fmt.Printf("Program match: %s\n", pattern)
		fmt.Printf("\tno program found\n")
		return nil
	}

	fmt.Printf("Program match: %s\n", pattern)
	for _, p := range resp.Programs {
		fmt.Printf("\t[%6s]:\t%s \n",
			p.GetLanguage(),
			p.GetProgramName(),
		)
	}

	return nil
}

// SyntaxFlowQueryAndShow 执行SyntaxFlow查询并显示结果
func (a *CLIAdapter) SyntaxFlowQueryAndShow(ctx context.Context, req *SSASyntaxFlowQueryRequest) error {
	if a.service == nil {
		return fmt.Errorf("service is nil")
	}

	resp, err := a.service.SyntaxFlowQuery(ctx, req)
	if err != nil {
		return err
	}

	if resp.Error != nil {
		// 即使有错误，也尝试显示结果
		log.Errorf("syntax flow query error: %v", resp.Error)
	}

	if resp.Result == nil {
		return resp.Error
	}

	// 显示结果
	log.Infof("syntax flow query result:")
	resp.Result.Show(
		sfvm.WithShowAll(req.Debug),
		sfvm.WithShowCode(req.WithCode),
		sfvm.WithShowDot(req.ShowDot),
	)

	return resp.Error
}

