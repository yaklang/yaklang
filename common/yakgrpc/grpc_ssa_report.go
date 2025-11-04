package yakgrpc

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// GenerateSSAReport 生成SSA扫描报告
// 支持两种生成方式：
// 1. 基于TaskID：生成整个扫描任务的完整报告
// 2. 基于RiskIDs：生成用户选择的Risk的报告
func (s *Server) GenerateSSAReport(ctx context.Context, req *ypb.GenerateSSAReportRequest) (*ypb.GenerateSSAReportResponse, error) {
	// 参数验证：TaskID和RiskIDs至少需要一个
	if req.GetTaskID() == "" && len(req.GetRiskIDs()) == 0 {
		return nil, utils.Errorf("taskID or riskIDs is required")
	}

	// 设置报告名称
	reportName := req.GetReportName()
	if reportName == "" {
		reportName = fmt.Sprintf("%s_%s", "SSA项目扫描报告", time.Now().Format("20060102150405"))
	}

	var ssaReport *sfreport.SSAProjectReport
	var err error

	// 两种独立的报告生成路径
	if len(req.GetRiskIDs()) > 0 {
		// 路径1：基于用户选择的RiskIDs生成报告（新功能）
		log.Infof("generating report from %d selected risk IDs", len(req.GetRiskIDs()))
		ssaReport, err = sfreport.GenerateSSAProjectReportFromRiskIDs(ctx, req.GetRiskIDs())
		if err != nil {
			log.Errorf("generate ssa project report from risk ids failed: %v", err)
			return nil, utils.Wrapf(err, "generate ssa project report from risk ids failed")
		}
	} else {
		// 路径2：基于TaskID生成完整扫描任务报告（原有功能）
		log.Infof("generating report from task ID: %s", req.GetTaskID())
		db := s.GetSSADatabase()
		task, err := schema.GetSyntaxFlowScanTaskById(db, req.GetTaskID())
		if err != nil {
			log.Errorf("get syntax flow scan task failed: %v", err)
			return nil, utils.Wrapf(err, "get syntax flow scan task failed")
		}

		ssaReport, err = sfreport.GenerateSSAProjectReportFromTask(ctx, task)
		if err != nil {
			log.Errorf("generate ssa project report failed: %v", err)
			return nil, utils.Wrapf(err, "generate ssa project report failed")
		}
	}

	// 创建IRify报告实例
	reportInstance := yakit.NewReport()
	reportInstance.From("ssa-scan")
	reportInstance.Title(reportName)

	// 生成报告内容
	err = sfreport.GenerateYakitReportContent(reportInstance, ssaReport)
	if err != nil {
		log.Errorf("generate yakit report content failed: %v", err)
		return nil, utils.Wrapf(err, "generate yakit report content failed")
	}

	// 保存报告
	reportID := reportInstance.SaveForIRify()
	if reportID == 0 {
		return nil, utils.Errorf("save report failed")
	}

	return &ypb.GenerateSSAReportResponse{
		ReportData: strconv.Itoa(reportID),
		Success:    true,
		Message:    "SSA扫描报告生成成功",
	}, nil
}
