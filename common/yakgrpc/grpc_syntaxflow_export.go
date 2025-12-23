//go:build !irify_exclude

package yakgrpc

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) ExportSyntaxFlows(req *ypb.ExportSyntaxFlowsRequest, stream ypb.Yak_ExportSyntaxFlowsServer) error {
	db := s.GetProfileDatabase()
	ruleDB := yakit.FilterSyntaxFlowRule(db, req.GetFilter())

	opts := make([]sfdb.RuleExportOption, 0)

	// 密码保护
	if req.GetPassword() != "" {
		opts = append(opts, sfdb.WithExportPassword(req.GetPassword()))
	}

	// 进度回调
	opts = append(opts, sfdb.WithExportProgress(func(current, total int) {
		stream.Send(&ypb.SyntaxflowsProgress{
			Progress: float64(current) / float64(total),
		})
	}))

	// 使用 sfdb 导出
	_, err := sfdb.ExportRulesToZip(stream.Context(), ruleDB, req.GetTargetPath(), opts...)
	if err != nil {
		return utils.Wrap(err, "export syntax flow rules failed")
	}

	return nil
}

func (s *Server) ImportSyntaxFlows(req *ypb.ImportSyntaxFlowsRequest, stream ypb.Yak_ImportSyntaxFlowsServer) error {
	db := s.GetProfileDatabase()

	opts := make([]sfdb.RuleImportOption, 0)

	// 密码保护
	if req.GetPassword() != "" {
		opts = append(opts, sfdb.WithImportPassword(req.GetPassword()))
	}

	// 进度回调
	opts = append(opts, sfdb.WithImportProgress(func(current, total int) {
		stream.Send(&ypb.SyntaxflowsProgress{
			Progress: float64(current) / float64(total),
		})
	}))

	// 使用 sfdb 导入
	_, err := sfdb.ImportRulesFromZip(stream.Context(), db, req.GetInputPath(), opts...)
	if err != nil {
		return utils.Wrap(err, "import syntax flow rules failed")
	}

	return nil
}
