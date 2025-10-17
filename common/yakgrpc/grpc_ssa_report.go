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
func (s *Server) GenerateSSAReport(ctx context.Context, req *ypb.GenerateSSAReportRequest) (*ypb.GenerateSSAReportResponse, error) {
	// 参数验证
	if req.GetTaskID() == "" {
		return nil, utils.Errorf("taskID is required")
	}
	// 根据TaskID获取扫描任务信息
	db := s.GetSSADatabase()
	task, err := schema.GetSyntaxFlowScanTaskById(db, req.GetTaskID())
	if err != nil {
		log.Errorf("get syntax flow scan task failed: %v", err)
		return nil, utils.Wrapf(err, "get syntax flow scan task failed")
	}

	// 设置报告名称
	reportName := req.GetReportName()
	if reportName == "" {
		reportName = fmt.Sprintf("%s_%s", "SSA项目扫描报告", time.Now().Format("20060102150405"))
	}

	// 生成SSA项目报告数据
	ssaReport, err := sfreport.GenerateSSAProjectReportFromTask(ctx, task)
	if err != nil {
		log.Errorf("generate ssa project report failed: %v", err)
		return nil, utils.Wrapf(err, "generate ssa project report failed")
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
